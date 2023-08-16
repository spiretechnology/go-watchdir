package watchdir

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"time"

	"github.com/spiretechnology/go-watchdir/v2/internal/tree"
)

func New(fsys fs.FS, options ...Option) WatchDir {
	wd := &watchDir{
		fsys:                    fsys,
		eventsMask:              AllEvents,
		filter:                  nil,
		pollInterval:            DefaultPollInterval,
		maxDepth:                DefaultMaxDepth,
		writeStabilityThreshold: DefaultWriteStabilityThreshold,
	}
	for _, option := range options {
		option(wd)
	}
	return wd
}

type watchDir struct {
	fsys                    fs.FS
	eventsMask              EventType
	filter                  Filter
	pollInterval            time.Duration
	maxDepth                uint
	writeStabilityThreshold time.Duration
}

// Watch begins scanning the watch directory
func (wd *watchDir) Watch(ctx context.Context, handler Handler) error {
	if wd.fsys == nil {
		return errors.New("cannot watch nil file system")
	}

	// Keep track of all the files we've already seen, and when we saw them last
	fileTree := tree.NewTree()

	// Loop until we're told to stop
	for {
		// Perform the sweep iteration
		if err := wd.sweep(ctx, fileTree, handler, 0, "."); err != nil {
			// If the context was cancelled, return that error
			if errors.Is(err, context.Canceled) {
				return err
			}

			// Other errors should not cause the watch process to end, so we just
			// log them and continue
			fmt.Printf("watchdir: sweep error: %s\n", err)
		}

		// Sleep until the next sweep
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wd.pollInterval):
		}
	}
}

func (wd *watchDir) sweep(ctx context.Context, dirTree *tree.Node, handler Handler, depth uint, pathPrefix string) error {
	// Breakout if the context is cancelled. This is placed here because this function is
	// recursive so it doesn't matter where we put this check.
	if err := ctx.Err(); err != nil {
		return err
	}

	// Return if the depth is too deep
	if depth >= wd.maxDepth {
		fmt.Println("watchdir: hit max depth")
		return nil
	}

	// List all the files in the directory
	entries, err := fs.ReadDir(wd.fsys, pathPrefix)
	if err != nil {
		fmt.Println("watchdir: error reading directory: ", err)
		return err
	}

	// Create a map of entries
	entriesMap := make(map[string]fs.DirEntry)
	for _, entry := range entries {
		entriesMap[entry.Name()] = entry
	}

	// Find all the entries that have been removed
	for entryName, childNode := range dirTree.Children {
		if _, ok := entriesMap[entryName]; !ok {
			delete(dirTree.Children, entryName)
			if wd.eventsMask&FileRemoved != 0 {
				if err := wd.handleRemovedFile(ctx, childNode, handler, pathPrefix); err != nil {
					return err
				}
			}
		}
	}

	// Loop through all the entries
	for _, entry := range entries {
		// Create the full path to the entry
		entryName := entry.Name()
		entryPath := path.Join(pathPrefix, entryName)

		// Allow the filter a chance to ignore the file
		if !entry.IsDir() && wd.filter != nil {
			keep, err := wd.filter.Filter(ctx, entryPath)
			if err != nil {
				return err
			}
			if !keep {
				continue
			}
		}

		// If the entry doesn't exist, create it
		childNode, ok := dirTree.Children[entryName]
		if !ok {
			childNode, err = wd.handleNewFile(ctx, handler, entry, pathPrefix)
			if err != nil {
				return err
			}
			if childNode == nil {
				continue
			}
			dirTree.Children[entryName] = childNode
		}

		// If the entry is a directory, sweep it recursively too
		if entry.IsDir() {
			if err := wd.sweep(ctx, childNode, handler, depth+1, entryPath); err != nil {
				return err
			}
		}
	}
	return nil
}

func (wd *watchDir) handleNewFile(ctx context.Context, handler Handler, entry fs.DirEntry, pathPrefix string) (*tree.Node, error) {
	// Create the new child node
	var childNode *tree.Node
	if entry.IsDir() {
		childNode = tree.NewNode(entry.Name(), tree.NodeTypeFolder)
	} else {
		childNode = tree.NewNode(entry.Name(), tree.NodeTypeFile)
	}

	// If the entry is a directory, nothing more to do here
	if entry.IsDir() {
		return childNode, nil
	}

	// Ensure the file has passed the write stability threshold
	entryStat, err := entry.Info()
	if err != nil {
		return nil, err
	}
	if entryStat.ModTime().Add(wd.writeStabilityThreshold).After(time.Now()) {
		return nil, nil
	}

	// Trigger the event handler
	if wd.eventsMask&FileAdded != 0 {
		if err := handler.WatchEvent(ctx, Event{
			Type: FileAdded,
			File: path.Join(pathPrefix, entry.Name()),
		}); err != nil {
			return nil, err
		}
	}
	return childNode, nil
}

func (wd *watchDir) handleRemovedFile(ctx context.Context, node *tree.Node, handler Handler, pathPrefix string) error {
	// If it's a file, trigger the handler for it
	if node.Type == tree.NodeTypeFile {
		return handler.WatchEvent(ctx, Event{
			Type: FileRemoved,
			File: path.Join(pathPrefix, node.Name),
		})
	}

	// If it's a directory, recursively trigger the handler for all its children
	for entryName, childNode := range node.Children {
		if err := wd.handleRemovedFile(ctx, childNode, handler, path.Join(pathPrefix, entryName)); err != nil {
			return err
		}
	}
	return nil
}

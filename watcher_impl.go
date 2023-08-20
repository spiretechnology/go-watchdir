package watchdir

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"time"

	"github.com/spiretechnology/go-watchdir/v2/internal/tree"
)

func New(fsys fs.FS, options ...Option) Watcher {
	wd := &watcher{
		fsys:                    fsys,
		fileTree:                tree.NewTree(),
		eventsMask:              AllEvents,
		fileFilter:              nil,
		dirFilter:               nil,
		sleepFunc:               nil,
		maxDepth:                DefaultMaxDepth,
		writeStabilityThreshold: DefaultWriteStabilityThreshold,
	}
	WithPollInterval(DefaultPollInterval)(wd)
	for _, option := range options {
		option(wd)
	}
	return wd
}

type watcher struct {
	fsys                    fs.FS
	subRoot                 string
	fileTree                *tree.Node
	eventsMask              EventType
	fileFilter              Filter
	dirFilter               Filter
	sleepFunc               func(context.Context) error
	maxDepth                uint
	writeStabilityThreshold time.Duration
}

func (wd *watcher) Watch(ctx context.Context, handler Handler) error {
	if wd.fsys == nil {
		return errors.New("cannot watch nil file system")
	}

	// Loop until we're told to stop
	for {
		// Perform the sweep iteration
		if err := wd.Sweep(ctx, handler); err != nil {
			// If the context was cancelled, return that error
			if errors.Is(err, context.Canceled) {
				return err
			}

			// Other errors should not cause the watch process to end, so we just
			// log them and continue
			fmt.Printf("watchdir: sweep error: %s\n", err)
		}

		// Sleep until the next sweep
		if err := wd.sleepFunc(ctx); err != nil {
			return err
		}
	}
}

func (wd *watcher) getSweepFS() (fs.FS, error) {
	// If there is no sub-root configured, use the root fs
	if wd.subRoot == "" {
		return wd.fsys, nil
	}

	// If the sub-root doesn't exist, return an error
	if _, err := fs.Stat(wd.fsys, wd.subRoot); os.IsNotExist(err) {
		return nil, err
	}

	// Replace the fsys (for this scan) with the sub-root fs
	return fs.Sub(wd.fsys, wd.subRoot)
}

func (wd *watcher) Sweep(ctx context.Context, handler Handler) error {
	// Get the fsys for the sweep, which can be a sub-fs
	fsys, err := wd.getSweepFS()
	if err != nil {
		return err
	}

	// Sweep the fsys recursively
	return wd.sweep(ctx, fsys, wd.fileTree, handler, 0, ".")
}

func (wd *watcher) sweep(ctx context.Context, fsys fs.FS, dirTree *tree.Node, handler Handler, depth uint, pathPrefix string) error {
	// Breakout if the context is cancelled.
	if err := ctx.Err(); err != nil {
		return err
	}

	// If this directory is excluded, skip it
	if wd.dirFilter != nil {
		include, err := wd.dirFilter.Filter(ctx, wd.prependSubRoot(pathPrefix))
		if err != nil {
			return err
		}
		if !include {
			return nil
		}
	}

	// Return if the depth is too deep
	if depth >= wd.maxDepth {
		fmt.Println("watchdir: hit max depth")
		return nil
	}

	// List all the files in the directory
	entries, err := fs.ReadDir(fsys, pathPrefix)
	if err != nil {
		fmt.Printf("watchdir: error reading directory %q: %s\n", pathPrefix, err)
		return err
	}

	// Create a map of entries
	entriesMap := make(map[string]fs.DirEntry)
	for _, entry := range entries {
		entriesMap[entry.Name()] = entry
	}

	// Find all the entries that have been removed
	deletedEntries := make(map[string]*tree.Node)
	for entryName, childNode := range dirTree.Children {
		if _, ok := entriesMap[entryName]; !ok {
			deletedEntries[entryName] = childNode
		}
	}

	// Delete all the removed entries
	for entryName, childNode := range deletedEntries {
		delete(dirTree.Children, entryName)
		if wd.eventsMask&FileRemoved != 0 {
			if err := wd.handleRemovedFile(ctx, childNode, handler, pathPrefix); err != nil {
				return err
			}
		}
	}
	deletedEntries = nil

	// Loop through all the entries
	for _, entry := range entries {
		// Create the full path to the entry
		entryName := entry.Name()
		entryPath := path.Join(pathPrefix, entryName)

		// Allow the filter a chance to ignore the file
		if !entry.IsDir() && wd.fileFilter != nil {
			keep, err := wd.fileFilter.Filter(ctx, wd.prependSubRoot(entryPath))
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
			if err := wd.sweep(ctx, fsys, childNode, handler, depth+1, entryPath); err != nil {
				return err
			}
		}
	}
	return nil
}

func (wd *watcher) handleNewFile(ctx context.Context, handler Handler, entry fs.DirEntry, pathPrefix string) (*tree.Node, error) {
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
			File: wd.prependSubRoot(path.Join(pathPrefix, entry.Name())),
		}); err != nil {
			return nil, err
		}
	}
	return childNode, nil
}

func (wd *watcher) handleRemovedFile(ctx context.Context, node *tree.Node, handler Handler, pathPrefix string) error {
	// Get the relative path to the node
	nodePath := path.Join(pathPrefix, node.Name)

	// If it's a file, trigger the handler for it
	if node.Type == tree.NodeTypeFile {
		return handler.WatchEvent(ctx, Event{
			Type: FileRemoved,
			File: wd.prependSubRoot(nodePath),
		})
	}

	// If it's a directory, recursively trigger the handler for all its children
	for _, childNode := range node.Children {
		if err := wd.handleRemovedFile(ctx, childNode, handler, nodePath); err != nil {
			return err
		}
	}
	return nil
}

func (wd *watcher) prependSubRoot(name string) string {
	if wd.subRoot == "" {
		return name
	}
	return path.Join(wd.subRoot, name)
}

package watchdir

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path"
	"time"
)

func New(fsys fs.FS, options ...Option) Watcher {
	wd := &watcher{
		fsys:                    fsys,
		eventsMask:              AllEvents,
		fileFilter:              nil,
		dirFilter:               nil,
		maxDepth:                DefaultMaxDepth,
		writeStabilityThreshold: DefaultWriteStabilityThreshold,
		logger:                  log.New(os.Stdout, "[watchdir] ", log.LstdFlags),
		cachedEntries:           make(map[string]map[string]fs.FileInfo),
	}
	for _, option := range options {
		option(wd)
	}
	return wd
}

type watcher struct {
	fsys                    fs.FS
	subRoot                 string
	eventsMask              EventType
	fileFilter              Filter
	dirFilter               Filter
	maxDepth                uint
	writeStabilityThreshold time.Duration
	logger                  *log.Logger

	cachedEntries map[string]map[string]fs.FileInfo
}

func (wd *watcher) getSweepFS() (fs.FS, error) {
	// If there is no file system, return an error
	if wd.fsys == nil {
		return nil, errors.New("cannot watch nil file system")
	}

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

func (wd *watcher) Sweep(ctx context.Context, chanEvents chan<- Event) (reterr error) {
	startTime := time.Now()
	wd.logger.Println("sweep started")
	defer func() {
		duration := time.Since(startTime)
		if reterr != nil {
			wd.logger.Printf("sweep took %s, returned error: %+v", duration, reterr)
		} else {
			wd.logger.Printf("sweep took %s, completed successfully", duration)
		}
	}()

	// Get the fsys for the sweep, which can be a sub-fs
	fsys, err := wd.getSweepFS()
	if err != nil {
		return err
	}

	// Sweep the file system recursively
	return wd.sweep(ctx, fsys, chanEvents, 0, ".")
}

func readDirStats(fsys fs.FS, pathPrefix string) (map[string]fs.FileInfo, error) {
	// Read the directory entries
	entries, err := fs.ReadDir(fsys, pathPrefix)
	if err != nil {
		return nil, fmt.Errorf("error reading directory %q: %w", pathPrefix, err)
	}

	// Create a map to hold the file info
	fileInfo := make(map[string]fs.FileInfo)
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return nil, fmt.Errorf("error getting info for entry %q: %w", entry.Name(), err)
		}
		fileInfo[entry.Name()] = info
	}
	return fileInfo, nil
}

func (wd *watcher) sweep(ctx context.Context, fsys fs.FS, chanEvents chan<- Event, depth uint, pathPrefix string) error {
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
		wd.logger.Printf("hit max depth %d", depth)
		return nil
	}

	// Read the entries in the directory
	entries, err := readDirStats(fsys, pathPrefix)
	if err != nil {
		return fmt.Errorf("error reading directory %q: %w", pathPrefix, err)
	}

	// Delete any entries that are not yet considered stable (recently modified)
	for name, entry := range entries {
		if entry.ModTime().Add(wd.writeStabilityThreshold).After(time.Now()) {
			delete(entries, name)
		}
	}

	// Get the previous sweep data for this directory
	prevEntries := wd.cachedEntries[pathPrefix]

	// Find entries that are newly added (didn't previously exist)
	if wd.eventsMask&FileAdded != 0 {
		for name, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if prevEntries != nil && prevEntries[name] != nil {
				continue
			}
			// If this file is excluded, skip it
			if wd.fileFilter != nil {
				include, err := wd.fileFilter.Filter(ctx, wd.prependSubRoot(path.Join(pathPrefix, name)))
				if err != nil {
					return err
				}
				if !include {
					continue
				}
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case chanEvents <- Event{
				Type: FileAdded,
				File: wd.prependSubRoot(path.Join(pathPrefix, name)),
			}:
			}
		}
	}

	// Find entries that were removed (existed previously but not now)
	if prevEntries != nil && wd.eventsMask&FileRemoved != 0 {
		for name, prevEntry := range prevEntries {
			if _, stillExists := entries[name]; stillExists {
				continue
			}
			if prevEntry.IsDir() {
				if err := wd.sweepDeleted(ctx, fsys, chanEvents, path.Join(pathPrefix, name)); err != nil {
					return fmt.Errorf("error sweeping deleted directory %q: %w", path.Join(pathPrefix, name), err)
				}
			} else {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case chanEvents <- Event{
					Type: FileRemoved,
					File: wd.prependSubRoot(path.Join(pathPrefix, name)),
				}:
				}
			}
		}
	}

	// Sweep all child directories
	for name, entry := range entries {
		if entry.IsDir() {
			if err := wd.sweep(ctx, fsys, chanEvents, depth+1, path.Join(pathPrefix, name)); err != nil {
				return err
			}
		}
	}

	// Update the previous entries cache
	wd.cachedEntries[pathPrefix] = entries
	return nil
}

func (wd *watcher) sweepDeleted(ctx context.Context, fsys fs.FS, chanEvents chan<- Event, pathPrefix string) error {
	// Get the previous sweep data for this directory
	prevEntries := wd.cachedEntries[pathPrefix]
	if prevEntries == nil {
		return nil // Nothing to sweep
	}

	// Loop over all of the entries that were previously cached
	for name, prevEntry := range prevEntries {
		if prevEntry.IsDir() {
			if err := wd.sweepDeleted(ctx, fsys, chanEvents, path.Join(pathPrefix, name)); err != nil {
				return fmt.Errorf("error sweeping deleted directory %q: %w", path.Join(pathPrefix, name), err)
			}
		} else if wd.eventsMask&FileRemoved != 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case chanEvents <- Event{
				Type: FileRemoved,
				File: wd.prependSubRoot(path.Join(pathPrefix, name)),
			}:
			}
		}
	}

	// Delete the cached entries
	delete(wd.cachedEntries, pathPrefix)
	return nil
}

func (wd *watcher) prependSubRoot(name string) string {
	if wd.subRoot == "" {
		return name
	}
	return path.Join(wd.subRoot, name)
}

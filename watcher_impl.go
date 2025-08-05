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

	"golang.org/x/sync/errgroup"
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
		cache:                   newDirCache(),
	}
	for _, option := range options {
		option(wd)
	}
	return wd
}

type dirCache struct {
	entries  map[string]fs.DirEntry
	children map[string]*dirCache
}

func newDirCache() *dirCache {
	return &dirCache{
		entries:  make(map[string]fs.DirEntry),
		children: make(map[string]*dirCache),
	}
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

	cache *dirCache
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
	return wd.sweep(ctx, fsys, chanEvents, 0, ".", wd.cache)
}

func readDir(fsys fs.FS, pathPrefix string) (map[string]fs.DirEntry, error) {
	// Read the directory entries
	entries, err := fs.ReadDir(fsys, pathPrefix)
	if err != nil {
		return nil, fmt.Errorf("readdir: %w", err)
	}

	// Convert it to a map for easier lookups
	entriesMap := make(map[string]fs.DirEntry, len(entries))
	for _, entry := range entries {
		entriesMap[entry.Name()] = entry
	}
	return entriesMap, nil
}

func (wd *watcher) sweep(ctx context.Context, fsys fs.FS, chanEvents chan<- Event, depth uint, pathPrefix string, cache *dirCache) error {
	// Breakout if the context is cancelled.
	if err := ctx.Err(); err != nil {
		return err
	}

	// If this directory is excluded, skip it
	if wd.dirFilter != nil {
		include, err := wd.dirFilter.Filter(ctx, wd.prependSubRoot(pathPrefix))
		if err != nil {
			return fmt.Errorf("filter dir %q: %w", pathPrefix, err)
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
	entries, err := readDir(fsys, pathPrefix)
	if err != nil {
		return fmt.Errorf("read dir %q: %w", pathPrefix, err)
	}

	// Remove any file entries that are excluded by the file filter
	if wd.fileFilter != nil {
		for name, entry := range entries {
			if entry.IsDir() {
				continue
			}
			include, err := wd.fileFilter.Filter(ctx, wd.prependSubRoot(path.Join(pathPrefix, name)))
			if err != nil {
				return fmt.Errorf("filter file %q: %w", name, err)
			}
			if !include {
				delete(entries, name)
			}
		}
	}

	// Find entries that are newly added (didn't previously exist)
	for name, entry := range entries {
		if entry.IsDir() {
			continue
		}
		// If the file already exists in the cache, skip it
		if cache.entries[name] != nil {
			continue
		}
		// Ignore the file if it doesn't pass the file filter
		if wd.fileFilter != nil {
			include, err := wd.fileFilter.Filter(ctx, wd.prependSubRoot(path.Join(pathPrefix, name)))
			if err != nil {
				return fmt.Errorf("filter file %q: %w", name, err)
			}
			if !include {
				delete(entries, name)
				continue
			}
		}
		// Ignore the file if it fails the write stability threshold
		if wd.writeStabilityThreshold > 0 {
			stat, err := entry.Info()
			if err != nil {
				return fmt.Errorf("stat entry %q: %w", name, err)
			}
			if stat.ModTime().Add(wd.writeStabilityThreshold).After(time.Now()) {
				delete(entries, name)
				continue
			}
		}
		// If the file is new, send an event
		if wd.eventsMask&FileAdded != 0 {
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
	for name, prevEntry := range cache.entries {
		if _, stillExists := entries[name]; stillExists {
			continue
		}
		if prevEntry.IsDir() {
			if err := wd.sweepDeleted(ctx, fsys, chanEvents, path.Join(pathPrefix, name), cache.children[name]); err != nil {
				return fmt.Errorf("sweep deleted directory %q: %w", path.Join(pathPrefix, name), err)
			}
			delete(cache.children, name)
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

	// Update the cache with the current entries
	cache.entries = entries

	eg, ctx := errgroup.WithContext(ctx)

	// Quickly update the children map to ensure it has entries for all current directories
	// This cannot be done concurrently due to map access
	for name, entry := range entries {
		if entry.IsDir() {
			// Create the child cache if it doesn't exist
			if cache.children[name] == nil {
				cache.children[name] = newDirCache()
			}
		}
	}

	// Sweep all child directories
	for name, entry := range entries {
		if entry.IsDir() {
			eg.Go(func() error {
				// Recursively sweep the child directory, creating the new cache for it
				if err := wd.sweep(ctx, fsys, chanEvents, depth+1, path.Join(pathPrefix, name), cache.children[name]); err != nil {
					return fmt.Errorf("sweep directory %q: %w", path.Join(pathPrefix, name), err)
				}
				return nil
			})
		}
	}

	// Wait for all of the goroutines to complete
	if err := eg.Wait(); err != nil {
		return fmt.Errorf("wait for sweep goroutines: %w", err)
	}
	return nil
}

func (wd *watcher) sweepDeleted(ctx context.Context, fsys fs.FS, chanEvents chan<- Event, pathPrefix string, cache *dirCache) error {
	// Get the previous sweep data for this directory
	if cache == nil {
		return nil // Nothing to sweep
	}

	// Loop over all of the entries that were previously cached
	for name, prevEntry := range cache.entries {
		if prevEntry.IsDir() {
			if err := wd.sweepDeleted(ctx, fsys, chanEvents, path.Join(pathPrefix, name), cache.children[name]); err != nil {
				return fmt.Errorf("sweep deleted directory %q: %w", path.Join(pathPrefix, name), err)
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
	return nil
}

func (wd *watcher) prependSubRoot(name string) string {
	if wd.subRoot == "" {
		return name
	}
	return path.Join(wd.subRoot, name)
}

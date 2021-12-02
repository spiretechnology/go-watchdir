package watchdir

import (
	"context"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"time"
)

type LogLevel uint8

const (
	INFO  = LogLevel(1 << 0)
	WARN  = LogLevel(1 << 1)
	ERROR = LogLevel(1 << 2)
)

// WatchDir represents a watch directory instance
type WatchDir struct {

	// Fs is an optional file system implementation to use instead of the OS runtime file system. If implemented
	// and provided here, it enables some interesting things, such as watch directories over the network, or
	// for in-memory file systems.
	FS fs.FS

	// Dir the directory to watch, relative to the root of the file system
	Dir string

	// Caching is an optional caching service which can be attached to the watch directory. It can be used to
	// prevent the re-indexing of files across multiple sessions, and over time.
	Caching Cacher

	// PollTimeout is the number of milliseconds to wait after a polling sweep has concluded before beginning
	// the next polling sweep.
	PollTimeout time.Duration

	// MaxDepth is the maximum depth to recursively index. There is no way to turn off this limit, as it would
	// introduce a security vulnerability. If set to zero, recursive search will be disabled.
	MaxDepth uint8

	// WriteStabilityThreshold is the number of milliseconds a file's size must remain the same before it is
	// indexed. This ensures files aren't indexed until they are fully finished writing to the file system
	WriteStabilityThreshold time.Duration

	// Logger is the logger to print to
	Logger *log.Logger

	// LogLevel is the level of logging to actuall print to the logger. The default is WARN, which means
	// logs of type WARN or ERROR will be printed to the logger
	LogLevel LogLevel

	// Operations is a bitmask for the operations we're interested in
	Operations Op
}

// Event represents a file event
type Event struct {
	Operation Op
	File      FoundFile
}

// indexedFile represents a file that has been indexed already
type indexedFile struct {
	// Path is the absolute path to the file
	Path string
	// SweepIndex is the sweep index when the file was last seen. This will increase with each sweep performed by
	// the watcher. This allows the watcher to identify files that were removed
	SweepIndex uint64
	// File is the stat info about the file
	File FoundFile
}

// Watch begins scanning the watch directory
func (wd *WatchDir) Watch(
	ctx context.Context,
	eventChan chan<- Event,
) error {

	// If there is no file system, use the default
	if wd.FS == nil {
		wd.FS = os.DirFS("")
	}

	// Keep track of all the files we've already seen
	indexedFiles := make(map[string]*indexedFile)

	// Defer the cleanup function
	defer wd.cleanup()

	// Loop until we're told to stop
	for sweepIndex := uint64(0); true; sweepIndex++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			if err := wd.performSweepIteration(
				ctx,
				eventChan,
				sweepIndex,
				indexedFiles,
			); err != nil {
				return err
			}
		}
	}

	// Return without error
	return nil

}

// createFoundFile creates a found file instance from the directory and filename
func (wd *WatchDir) createFoundFile(dir, name string) (*FoundFile, error) {
	fullPath := filepath.Join(dir, name)
	info, err := fs.Stat(wd.FS, fullPath)
	if err != nil {
		return nil, err
	}
	file := &FoundFile{
		Dir:  wd.Dir,
		Path: fullPath,
		Info: info,
	}
	return file, nil
}

// sweepCallback creates a callback function that is called whenever files are found
func (wd *WatchDir) sweepCallback(newFileEvents *[]Event, chanFiles chan<- Event) func(*FoundFile) {
	return func(file *FoundFile) {

		// If caching is enabled, give it the opportunity to skip this file
		if wd.Caching != nil {
			isNew, err := wd.Caching.Found(file)
			if err != nil {
				wd.log(ERROR, "watch directory caching error: ", err)
				return
			}
			if !isNew {
				return
			}
		}

		// Create the event instance
		fileEvent := Event{
			Operation: Add,
			File:      *file,
		}

		// Send the file over the channel
		chanFiles <- fileEvent

	}
}

// performSweepIteration performs one iteration of the directory sweeper
func (wd *WatchDir) performSweepIteration(
	ctx context.Context,
	chanFiles chan<- Event,
	sweepIndex uint64,
	indexedFiles map[string]*indexedFile,
) error {

	// Create the slice of new files
	var newFileEvents []Event

	// Sweep the entire directory recursively
	if err := wd.sweepRecursive(
		ctx,
		wd.Dir,
		0,
		sweepIndex,
		indexedFiles,
		wd.sweepCallback(&newFileEvents, chanFiles),
	); err != nil {
		return err
	}

	// The list of removed file keys
	var removedKeys []string

	// Look for all of the removed files
	for k, v := range indexedFiles {
		if v.SweepIndex != sweepIndex {

			// Create the removed event
			removedEvent := Event{
				Operation: Remove,
				File:      v.File,
			}

			// Send the event on the channel
			chanFiles <- removedEvent

			// Remove the file from the map
			removedKeys = append(removedKeys, k)

		}
	}

	// Remove all of the keys
	for _, k := range removedKeys {
		delete(indexedFiles, k)
	}

	// Grab the timestamp when the sweep ends
	endTime := time.Now()

	// Calculate the remaining amount of time we need to sleep
	sleepTime := wd.PollTimeout - time.Since(endTime)
	if sleepTime > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(sleepTime):
		}
	}

	// Return without error
	return nil

}

func (wd *WatchDir) sweepRecursive(
	ctx context.Context,
	path string,
	depth uint8,
	sweepIndex uint64,
	indexedFiles map[string]*indexedFile,
	foundFile func(*FoundFile),
) error {

	// Get the stats about the path
	info, err := fs.Stat(wd.FS, path)
	if err != nil {
		wd.logf(WARN, "warning stat %q: %s", path, err)
		// This error can actually be ignored, as it usually means a file was just recently moved,
		// as opposed to anything more concerning.
		return nil
	}

	// If the path is a file
	if !info.IsDir() {

		// Create the found file instance
		file, err := wd.createFoundFile(
			filepath.Dir(path),
			filepath.Base(path),
		)
		if err != nil {
			return err
		}

		// If the modified time is too recent, return for now. We'll index it again on the next sweep
		if file.Info.ModTime().Add(time.Millisecond * wd.WriteStabilityThreshold).After(time.Now()) {
			return nil
		}

		// Get the existing entry for the file
		existingEntry, alreadyIndexed := indexedFiles[path]

		// If the value is not in the map, or the value is out of date
		if !alreadyIndexed {

			// Add the file to the map
			indexedFiles[path] = &indexedFile{
				Path:       path,
				SweepIndex: sweepIndex,
				File:       *file,
			}

			// We found the file!
			foundFile(file)

		} else {

			// Update the sweep index, telling the watcher that the file is still here
			existingEntry.SweepIndex = sweepIndex

		}

	} else {

		// If we're already too deep
		if depth >= wd.MaxDepth {
			return nil
		}

		// List all of the files
		entries, err := fs.ReadDir(wd.FS, path)
		if err != nil {
			wd.log(ERROR, "error sweeping directory: ", err)
			return err
		}

		// Loop through the files list
		for _, entry := range entries {

			// Sweep the child
			if err := wd.sweepRecursive(
				ctx,
				filepath.Join(path, entry.Name()),
				depth+1,
				sweepIndex,
				indexedFiles,
				foundFile,
			); err != nil {
				wd.log(WARN, "error sweeping subdir: ", err)
				// Don't return here, because we don't want to prevent sibling directories from being swept (the loop)
			}

		}

	}

	// Return without error
	return nil

}

// cleanup performs cleanup for the directory watcher just before it stops watching
func (wd *WatchDir) cleanup() {

	// If there is a cacher
	if wd.Caching != nil {
		err := wd.Caching.Cleanup()
		if err != nil {
			wd.log(ERROR, "cacher cleanup error: ", err)
		}
	}

}

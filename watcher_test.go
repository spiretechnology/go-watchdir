package watchdir_test

import (
	"context"
	"testing"

	"github.com/spiretechnology/go-memfs"
	"github.com/spiretechnology/go-watchdir/v2"
	"github.com/spiretechnology/go-watchdir/v2/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

func sweepAndCollectEvents(t *testing.T, wd watchdir.Watcher) (map[watchdir.EventType][]string, error) {
	t.Helper()

	var eg errgroup.Group

	// In one goroutine, perform the sweep and send events to the channel
	chanEvents := make(chan watchdir.Event)
	eg.Go(func() error {
		defer close(chanEvents)
		return wd.Sweep(context.Background(), chanEvents)
	})

	// In another goroutine, collect the events into a map
	events := make(map[watchdir.EventType][]string)
	eg.Go(func() error {
		for event := range chanEvents {
			events[event.Type] = append(events[event.Type], event.File)
		}
		return nil
	})

	// Wait for both goroutines to finish, then return the collected events
	err := eg.Wait()
	return events, err
}

func TestWatchDir(t *testing.T) {
	t.Run("detects added and removed files", func(t *testing.T) {
		fsys := memfs.FS{
			"foo": memfs.File("hello"),
		}
		wd := watchdir.New(fsys, watchdir.WithWriteStabilityThreshold(0))

		// Initial sweep. Should find one file.
		events, err := sweepAndCollectEvents(t, wd)
		require.NoError(t, err, "error sweeping")
		require.Len(t, events[watchdir.FileAdded], 1, "wrong number of add events")
		require.Len(t, events[watchdir.FileRemoved], 0, "wrong number of remove events")
		require.ElementsMatch(t, []string{"foo"}, events[watchdir.FileAdded], "wrong file added")

		// Add a file to the FS
		fsys["bar"] = memfs.File("world")

		// Second sweep. Should find the new file
		events, err = sweepAndCollectEvents(t, wd)
		require.NoError(t, err, "error sweeping")
		require.Len(t, events[watchdir.FileAdded], 1, "wrong number of add events")
		require.Len(t, events[watchdir.FileRemoved], 0, "wrong number of remove events")
		require.ElementsMatch(t, []string{"bar"}, events[watchdir.FileAdded], "wrong file added")

		// Add a directory
		fsys["somedir"] = memfs.Dir{}

		// Third sweep. Should find nothing new.
		events, err = sweepAndCollectEvents(t, wd)
		require.NoError(t, err, "error sweeping")
		require.Len(t, events[watchdir.FileAdded], 0, "wrong number of add events")
		require.Len(t, events[watchdir.FileRemoved], 0, "wrong number of remove events")

		// Remove some files
		delete(fsys, "foo")
		delete(fsys, "bar")

		// Fourth sweep. Should find the removed files.
		events, err = sweepAndCollectEvents(t, wd)
		require.NoError(t, err, "error sweeping")
		require.Len(t, events[watchdir.FileAdded], 0, "wrong number of add events")
		require.Len(t, events[watchdir.FileRemoved], 2, "wrong number of remove events")
		require.ElementsMatch(t, []string{"foo", "bar"}, events[watchdir.FileRemoved], "wrong files removed")
	})
	t.Run("delete entire directory of files", func(t *testing.T) {
		fsys := memfs.FS{
			"hello/foo": memfs.File("hello"),
			"hello/bar": memfs.File("world"),
			"hello/baz": memfs.File("golang"),
		}
		wd := watchdir.New(fsys, watchdir.WithWriteStabilityThreshold(0))

		// Initial sweep. Should find three files.
		events, err := sweepAndCollectEvents(t, wd)
		require.NoError(t, err, "error sweeping")
		require.Len(t, events[watchdir.FileAdded], 3, "wrong number of add events")
		require.Len(t, events[watchdir.FileRemoved], 0, "wrong number of remove events")
		require.ElementsMatch(t, []string{"hello/foo", "hello/bar", "hello/baz"}, events[watchdir.FileAdded], "wrong files added")

		// Delete all the files
		delete(fsys, "hello/foo")
		delete(fsys, "hello/bar")
		delete(fsys, "hello/baz")

		// Second sweep. Should detect all three files deleted
		events, err = sweepAndCollectEvents(t, wd)
		require.NoError(t, err, "error sweeping")
		require.Len(t, events[watchdir.FileAdded], 0, "wrong number of add events")
		require.Len(t, events[watchdir.FileRemoved], 3, "wrong number of remove events")
		require.ElementsMatch(t, []string{"hello/foo", "hello/bar", "hello/baz"}, events[watchdir.FileRemoved], "wrong files removed")
	})
	t.Run("delete directory with nested directories", func(t *testing.T) {
		fsys := memfs.FS{
			"hello/foo/a":       memfs.File(""),
			"hello/foo/bar/a":   memfs.File(""),
			"hello/foo/bar/baz": memfs.Dir{},
			"hello/bar/a":       memfs.File(""),
		}
		wd := watchdir.New(fsys, watchdir.WithWriteStabilityThreshold(0))

		// Initial sweep. Should find three files.
		events, err := sweepAndCollectEvents(t, wd)
		require.NoError(t, err, "error sweeping")
		require.Len(t, events[watchdir.FileAdded], 3, "wrong number of add events")
		require.Len(t, events[watchdir.FileRemoved], 0, "wrong number of remove events")
		require.ElementsMatch(t, []string{"hello/foo/a", "hello/foo/bar/a", "hello/bar/a"}, events[watchdir.FileAdded], "wrong files added")

		// Delete all the files
		delete(fsys, "hello/foo/a")
		delete(fsys, "hello/foo/bar/a")
		delete(fsys, "hello/bar/a")

		// Second sweep. Should detect all three files deleted
		events, err = sweepAndCollectEvents(t, wd)
		require.NoError(t, err, "error sweeping")
		require.Len(t, events[watchdir.FileAdded], 0, "wrong number of add events")
		require.Len(t, events[watchdir.FileRemoved], 3, "wrong number of remove events")
		require.ElementsMatch(t, []string{"hello/foo/a", "hello/foo/bar/a", "hello/bar/a"}, events[watchdir.FileRemoved], "wrong files removed")
	})
	t.Run("consecutive sweeps with no changes", func(t *testing.T) {
		fsys := memfs.FS{
			"hello/foo/a":       memfs.File(""),
			"hello/foo/bar/a":   memfs.File(""),
			"hello/foo/bar/baz": memfs.Dir{},
			"hello/bar/a":       memfs.File(""),
		}
		wd := watchdir.New(fsys, watchdir.WithWriteStabilityThreshold(0))

		// Initial sweep. Should find three files.
		events, err := sweepAndCollectEvents(t, wd)
		require.NoError(t, err, "error sweeping")
		require.Len(t, events[watchdir.FileAdded], 3, "wrong number of add events")
		require.Len(t, events[watchdir.FileRemoved], 0, "wrong number of remove events")
		require.ElementsMatch(t, []string{"hello/foo/a", "hello/foo/bar/a", "hello/bar/a"}, events[watchdir.FileAdded], "wrong files added")

		// Sweep again. Should find nothing new.
		events, err = sweepAndCollectEvents(t, wd)
		require.NoError(t, err, "error sweeping")
		require.Len(t, events[watchdir.FileAdded], 0, "wrong number of add events")
		require.Len(t, events[watchdir.FileRemoved], 0, "wrong number of remove events")
	})
	t.Run("directory exclusions", func(t *testing.T) {
		fsys := memfs.FS{
			"hello/foo/a":       memfs.File(""),
			"hello/foo/bar/a":   memfs.File(""),
			"hello/foo/bar/baz": memfs.Dir{},
			"hello/bar/a":       memfs.File(""),
			"world/baz/a":       memfs.File(""),
		}
		wd := watchdir.New(fsys,
			watchdir.WithWriteStabilityThreshold(0),
			watchdir.WithExcludeDirs("hello/foo"),
		)

		// Initial sweep. Should find three files.
		events, err := sweepAndCollectEvents(t, wd)
		require.NoError(t, err, "error sweeping")
		require.Len(t, events[watchdir.FileAdded], 2, "wrong number of add events")
		require.Len(t, events[watchdir.FileRemoved], 0, "wrong number of remove events")
		require.ElementsMatch(t, []string{"hello/bar/a", "world/baz/a"}, events[watchdir.FileAdded], "wrong files added")
	})
	t.Run("sub root", func(t *testing.T) {
		fsys := memfs.FS{
			"hello/foo/a":       memfs.File(""),
			"hello/foo/bar/a":   memfs.File(""),
			"hello/foo/bar/baz": memfs.Dir{},
			"hello/bar/a":       memfs.File(""),
			"world/baz/a":       memfs.File(""),
			"world/baz/b":       memfs.File(""),
			"world/baz/c":       memfs.File(""),
		}
		mockDirFilter := &mocks.MockFilter{}
		mockFileFilter := &mocks.MockFilter{}
		wd := watchdir.New(fsys,
			watchdir.WithWriteStabilityThreshold(0),
			watchdir.WithSubRoot("hello"),
			watchdir.WithDirFilter(mockDirFilter),
			watchdir.WithFileFilter(mockFileFilter),
		)

		// Setup expectations for dir filter
		mockDirFilter.On("Filter", mock.Anything, "hello").Return(true, nil)
		mockDirFilter.On("Filter", mock.Anything, "hello/foo").Return(true, nil)
		mockDirFilter.On("Filter", mock.Anything, "hello/foo/bar").Return(true, nil)
		mockDirFilter.On("Filter", mock.Anything, "hello/foo/bar/baz").Return(true, nil)
		mockDirFilter.On("Filter", mock.Anything, "hello/bar").Return(true, nil)

		// Setup expectations for file filter
		mockFileFilter.On("Filter", mock.Anything, "hello/foo/a").Return(true, nil)
		mockFileFilter.On("Filter", mock.Anything, "hello/foo/bar/a").Return(true, nil)
		mockFileFilter.On("Filter", mock.Anything, "hello/bar/a").Return(true, nil)

		// Initial sweep. Should find three files.
		events, err := sweepAndCollectEvents(t, wd)
		require.NoError(t, err, "error sweeping")
		require.Len(t, events[watchdir.FileAdded], 3, "wrong number of add events")
		require.Len(t, events[watchdir.FileRemoved], 0, "wrong number of remove events")
		require.ElementsMatch(t, []string{"hello/foo/a", "hello/foo/bar/a", "hello/bar/a"}, events[watchdir.FileAdded], "wrong files added")
		mockDirFilter.AssertExpectations(t)
		mockFileFilter.AssertExpectations(t)
	})
	t.Run("sub root that doesn't exist", func(t *testing.T) {
		fsys := memfs.FS{
			"hello/a": memfs.File(""),
		}
		mockDirFilter := &mocks.MockFilter{}
		mockFileFilter := &mocks.MockFilter{}
		wd := watchdir.New(fsys,
			watchdir.WithWriteStabilityThreshold(0),
			watchdir.WithSubRoot("world"),
			watchdir.WithDirFilter(mockDirFilter),
			watchdir.WithFileFilter(mockFileFilter),
		)

		// Initial sweep. Should error because the root doesn't exist.
		events, err := sweepAndCollectEvents(t, wd)
		require.Error(t, err, "should error sweeping")
		require.Len(t, events[watchdir.FileAdded], 0, "wrong number of add events")
		require.Len(t, events[watchdir.FileRemoved], 0, "wrong number of remove events")
		mockDirFilter.AssertExpectations(t)
		mockFileFilter.AssertExpectations(t)

		// Now, add the sub-root and try again
		fsys["world/a"] = memfs.File("")
		fsys["world/b"] = memfs.File("")
		fsys["world/c"] = memfs.File("")
		fsys["world/child/a"] = memfs.File("")
		fsys["world/child/grandchild"] = memfs.Dir{}

		// Setup expectations for dir filter
		mockDirFilter.On("Filter", mock.Anything, "world").Return(true, nil)
		mockDirFilter.On("Filter", mock.Anything, "world/child").Return(true, nil)
		mockDirFilter.On("Filter", mock.Anything, "world/child/grandchild").Return(true, nil)

		// Setup expectations for file filter
		mockFileFilter.On("Filter", mock.Anything, "world/a").Return(true, nil)
		mockFileFilter.On("Filter", mock.Anything, "world/b").Return(true, nil)
		mockFileFilter.On("Filter", mock.Anything, "world/c").Return(true, nil)
		mockFileFilter.On("Filter", mock.Anything, "world/child/a").Return(true, nil)

		// Initial sweep. Should error because the root doesn't exist.
		events, err = sweepAndCollectEvents(t, wd)
		require.NoError(t, err, "error sweeping")
		require.Len(t, events[watchdir.FileAdded], 4, "wrong number of add events")
		require.Len(t, events[watchdir.FileRemoved], 0, "wrong number of remove events")
		require.ElementsMatch(t, []string{"world/a", "world/b", "world/c", "world/child/a"}, events[watchdir.FileAdded], "wrong files added")
		mockDirFilter.AssertExpectations(t)
		mockFileFilter.AssertExpectations(t)
	})
}

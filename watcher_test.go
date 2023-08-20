package watchdir_test

import (
	"context"
	"testing"

	"github.com/spiretechnology/go-memfs"
	"github.com/spiretechnology/go-watchdir/v2"
	"github.com/spiretechnology/go-watchdir/v2/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type spyHandler struct {
	events map[watchdir.EventType][]string
}

func (h *spyHandler) Clear() {
	h.events = nil
}

func (h *spyHandler) WatchEvent(ctx context.Context, event watchdir.Event) error {
	if h.events == nil {
		h.events = make(map[watchdir.EventType][]string)
	}
	h.events[event.Type] = append(h.events[event.Type], event.File)
	return nil
}

func TestWatchDir(t *testing.T) {
	t.Run("detects added and removed files", func(t *testing.T) {
		fsys := memfs.FS{
			"foo": memfs.File("hello"),
		}
		wd := watchdir.New(fsys, watchdir.WithWriteStabilityThreshold(0))
		handler := &spyHandler{}

		// Initial sweep. Should find one file.
		err := wd.Sweep(context.Background(), handler)
		require.NoError(t, err, "error sweeping")
		require.Len(t, handler.events[watchdir.FileAdded], 1, "wrong number of add events")
		require.Len(t, handler.events[watchdir.FileRemoved], 0, "wrong number of remove events")
		require.ElementsMatch(t, []string{"foo"}, handler.events[watchdir.FileAdded], "wrong file added")
		handler.Clear()

		// Add a file to the FS
		fsys["bar"] = memfs.File("world")

		// Second sweep. Should find the new file
		err = wd.Sweep(context.Background(), handler)
		require.NoError(t, err, "error sweeping")
		require.Len(t, handler.events[watchdir.FileAdded], 1, "wrong number of add events")
		require.Len(t, handler.events[watchdir.FileRemoved], 0, "wrong number of remove events")
		require.ElementsMatch(t, []string{"bar"}, handler.events[watchdir.FileAdded], "wrong file added")
		handler.Clear()

		// Add a directory
		fsys["somedir"] = memfs.Dir{}

		// Third sweep. Should find nothing new.
		err = wd.Sweep(context.Background(), handler)
		require.NoError(t, err, "error sweeping")
		require.Len(t, handler.events[watchdir.FileAdded], 0, "wrong number of add events")
		require.Len(t, handler.events[watchdir.FileRemoved], 0, "wrong number of remove events")
		handler.Clear()

		// Remove some files
		delete(fsys, "foo")
		delete(fsys, "bar")

		// Fourth sweep. Should find the removed files.
		err = wd.Sweep(context.Background(), handler)
		require.NoError(t, err, "error sweeping")
		require.Len(t, handler.events[watchdir.FileAdded], 0, "wrong number of add events")
		require.Len(t, handler.events[watchdir.FileRemoved], 2, "wrong number of remove events")
		require.ElementsMatch(t, []string{"foo", "bar"}, handler.events[watchdir.FileRemoved], "wrong files removed")
		handler.Clear()
	})
	t.Run("delete entire directory of files", func(t *testing.T) {
		fsys := memfs.FS{
			"hello/foo": memfs.File("hello"),
			"hello/bar": memfs.File("world"),
			"hello/baz": memfs.File("golang"),
		}
		wd := watchdir.New(fsys, watchdir.WithWriteStabilityThreshold(0))
		handler := &spyHandler{}

		// Initial sweep. Should find three files.
		err := wd.Sweep(context.Background(), handler)
		require.NoError(t, err, "error sweeping")
		require.Len(t, handler.events[watchdir.FileAdded], 3, "wrong number of add events")
		require.Len(t, handler.events[watchdir.FileRemoved], 0, "wrong number of remove events")
		require.ElementsMatch(t, []string{"hello/foo", "hello/bar", "hello/baz"}, handler.events[watchdir.FileAdded], "wrong files added")
		handler.Clear()

		// Delete all the files
		delete(fsys, "hello/foo")
		delete(fsys, "hello/bar")
		delete(fsys, "hello/baz")

		// Second sweep. Should detect all three files deleted
		err = wd.Sweep(context.Background(), handler)
		require.NoError(t, err, "error sweeping")
		require.Len(t, handler.events[watchdir.FileAdded], 0, "wrong number of add events")
		require.Len(t, handler.events[watchdir.FileRemoved], 3, "wrong number of remove events")
		require.ElementsMatch(t, []string{"hello/foo", "hello/bar", "hello/baz"}, handler.events[watchdir.FileRemoved], "wrong files removed")
		handler.Clear()
	})
	t.Run("delete directory with nested directories", func(t *testing.T) {
		fsys := memfs.FS{
			"hello/foo/a":       memfs.File(""),
			"hello/foo/bar/a":   memfs.File(""),
			"hello/foo/bar/baz": memfs.Dir{},
			"hello/bar/a":       memfs.File(""),
		}
		wd := watchdir.New(fsys, watchdir.WithWriteStabilityThreshold(0))
		handler := &spyHandler{}

		// Initial sweep. Should find three files.
		err := wd.Sweep(context.Background(), handler)
		require.NoError(t, err, "error sweeping")
		require.Len(t, handler.events[watchdir.FileAdded], 3, "wrong number of add events")
		require.Len(t, handler.events[watchdir.FileRemoved], 0, "wrong number of remove events")
		require.ElementsMatch(t, []string{"hello/foo/a", "hello/foo/bar/a", "hello/bar/a"}, handler.events[watchdir.FileAdded], "wrong files added")
		handler.Clear()

		// Delete all the files
		delete(fsys, "hello/foo/a")
		delete(fsys, "hello/foo/bar/a")
		delete(fsys, "hello/bar/a")

		// Second sweep. Should detect all three files deleted
		err = wd.Sweep(context.Background(), handler)
		require.NoError(t, err, "error sweeping")
		require.Len(t, handler.events[watchdir.FileAdded], 0, "wrong number of add events")
		require.Len(t, handler.events[watchdir.FileRemoved], 3, "wrong number of remove events")
		require.ElementsMatch(t, []string{"hello/foo/a", "hello/foo/bar/a", "hello/bar/a"}, handler.events[watchdir.FileRemoved], "wrong files removed")
		handler.Clear()
	})
	t.Run("consecutive sweeps with no changes", func(t *testing.T) {
		fsys := memfs.FS{
			"hello/foo/a":       memfs.File(""),
			"hello/foo/bar/a":   memfs.File(""),
			"hello/foo/bar/baz": memfs.Dir{},
			"hello/bar/a":       memfs.File(""),
		}
		wd := watchdir.New(fsys, watchdir.WithWriteStabilityThreshold(0))
		handler := &spyHandler{}

		// Initial sweep. Should find three files.
		err := wd.Sweep(context.Background(), handler)
		require.NoError(t, err, "error sweeping")
		require.Len(t, handler.events[watchdir.FileAdded], 3, "wrong number of add events")
		require.Len(t, handler.events[watchdir.FileRemoved], 0, "wrong number of remove events")
		require.ElementsMatch(t, []string{"hello/foo/a", "hello/foo/bar/a", "hello/bar/a"}, handler.events[watchdir.FileAdded], "wrong files added")
		handler.Clear()

		// Sweep again. Should find nothing new.
		err = wd.Sweep(context.Background(), handler)
		require.NoError(t, err, "error sweeping")
		require.Len(t, handler.events[watchdir.FileAdded], 0, "wrong number of add events")
		require.Len(t, handler.events[watchdir.FileRemoved], 0, "wrong number of remove events")
		handler.Clear()
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
		handler := &spyHandler{}

		// Initial sweep. Should find three files.
		err := wd.Sweep(context.Background(), handler)
		require.NoError(t, err, "error sweeping")
		require.Len(t, handler.events[watchdir.FileAdded], 2, "wrong number of add events")
		require.Len(t, handler.events[watchdir.FileRemoved], 0, "wrong number of remove events")
		require.ElementsMatch(t, []string{"hello/bar/a", "world/baz/a"}, handler.events[watchdir.FileAdded], "wrong files added")
		handler.Clear()
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
		handler := &spyHandler{}

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
		err := wd.Sweep(context.Background(), handler)
		require.NoError(t, err, "error sweeping")
		require.Len(t, handler.events[watchdir.FileAdded], 3, "wrong number of add events")
		require.Len(t, handler.events[watchdir.FileRemoved], 0, "wrong number of remove events")
		require.ElementsMatch(t, []string{"hello/foo/a", "hello/foo/bar/a", "hello/bar/a"}, handler.events[watchdir.FileAdded], "wrong files added")
		handler.Clear()
		mockDirFilter.AssertExpectations(t)
		mockFileFilter.AssertExpectations(t)
	})
}

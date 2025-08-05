package watchdir

import (
	"context"
	"log"
	"time"
)

type Option func(wd *watcher)

func WithEvents(mask EventType) Option {
	return func(wd *watcher) {
		wd.eventsMask = mask
	}
}

func WithPollInterval(interval time.Duration) Option {
	return func(wd *watcher) {
		wd.sleepFunc = func(ctx context.Context) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(interval):
			}
			return nil
		}
	}
}

func WithMaxDepth(maxDepth uint) Option {
	return func(wd *watcher) {
		wd.maxDepth = maxDepth
	}
}

func WithWriteStabilityThreshold(threshold time.Duration) Option {
	return func(wd *watcher) {
		wd.writeStabilityThreshold = threshold
	}
}

func WithFileFilter(filter Filter) Option {
	return func(wd *watcher) {
		wd.fileFilter = filter
	}
}

func WithDirFilter(filter Filter) Option {
	return func(wd *watcher) {
		wd.dirFilter = filter
	}
}

func WithExcludeDirs(dirs ...string) Option {
	dirsMap := make(map[string]struct{})
	for _, dir := range dirs {
		dir = normalizePath(dir)
		dirsMap[dir] = struct{}{}
	}
	return WithDirFilter(FilterFunc(func(ctx context.Context, dir string) (bool, error) {
		_, ok := dirsMap[dir]
		return !ok, nil
	}))
}

func WithSubRoot(root string) Option {
	return func(wd *watcher) {
		wd.subRoot = normalizePath(root)
	}
}

func WithLogger(logger *log.Logger) Option {
	return func(wd *watcher) {
		wd.logger = logger
	}
}

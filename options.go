package watchdir

import (
	"context"
	"time"
)

type Option func(wd *watchDir)

func WithEvents(mask EventType) Option {
	return func(wd *watchDir) {
		wd.eventsMask = mask
	}
}

func WithFilter(filter Filter) Option {
	return func(wd *watchDir) {
		wd.filter = filter
	}
}

func WithPollInterval(interval time.Duration) Option {
	return func(wd *watchDir) {
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
	return func(wd *watchDir) {
		wd.maxDepth = maxDepth
	}
}

func WithWriteStabilityThreshold(threshold time.Duration) Option {
	return func(wd *watchDir) {
		wd.writeStabilityThreshold = threshold
	}
}

package watchdir

import (
	"context"
	"errors"
	"time"
)

const (
	// DefaultMaxDepth is the default maximum depth to sweep to, if nothing else is specified.
	DefaultMaxDepth = 10

	// DefaultWriteStabilityThreshold is the default amount of time since last modification before a file is detected.
	DefaultWriteStabilityThreshold = 15 * time.Second
)

type Watcher interface {
	// Sweep performs a single sweep of the directory and calls the handler on each change.
	Sweep(ctx context.Context, chanEvents chan<- Event) error
}

// EventType defines an operation that took place on the watch directory
type EventType uint8

const (
	FileAdded   = EventType(1 << 0)
	FileRemoved = EventType(1 << 1)
	AllEvents   = 0b11111111
)

// Event represents a file event
type Event struct {
	Type EventType
	File string
}

// Watch performs a periodic sweep of a given directory and sends events to the provided channel.
func Watch(
	ctx context.Context,
	w Watcher,
	sweepInterval time.Duration,
	chanEvents chan<- Event,
) error {
	for {
		// Perform the sweep iteration
		if err := w.Sweep(ctx, chanEvents); err != nil {
			// If the context was cancelled, return that error
			if errors.Is(err, context.Canceled) {
				return err
			}
		}

		// Sleep for the configured interval
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(sweepInterval):
		}
	}
}

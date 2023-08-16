package watchdir

import (
	"context"
	"time"
)

const (
	// DefaultMaxDepth is the default maximum depth to sweep to, if nothing else is specified.
	DefaultMaxDepth = 10

	// DefaultPollInterval is the default timeout between polling interations, if nothing else is specified.
	DefaultPollInterval = 15 * time.Second

	// DefaultWriteStabilityThreshold is the default amount of time since last modification before a file is detected.
	DefaultWriteStabilityThreshold = 15 * time.Second
)

type WatchDir interface {
	Watch(ctx context.Context, handler Handler) error
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

type Handler interface {
	WatchEvent(ctx context.Context, event Event) error
}

type HandlerFunc func(ctx context.Context, event Event) error

func (h HandlerFunc) WatchEvent(ctx context.Context, event Event) error {
	return h(ctx, event)
}

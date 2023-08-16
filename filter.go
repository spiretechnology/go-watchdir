package watchdir

import "context"

// Filter is an interface that can be implemented to instruct the watcher to ignore certain files entirely.
type Filter interface {
	// Filter returns true if the file should be scanned, and false if it should be ignored.
	Filter(ctx context.Context, filename string) (bool, error)
}

type FilterFunc func(ctx context.Context, filename string) (bool, error)

func (f FilterFunc) Filter(ctx context.Context, filename string) (bool, error) {
	return f(ctx, filename)
}

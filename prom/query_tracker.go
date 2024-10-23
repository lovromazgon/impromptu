package prom

import (
	"context"
)

// SequentialQueryTracker is a query tracker that allows only one query to be
// active at a time.
type SequentialQueryTracker chan struct{}

func NewSequentialQueryTracker() SequentialQueryTracker {
	return make(SequentialQueryTracker, 1)
}

func (s SequentialQueryTracker) GetMaxConcurrent() int {
	return 1
}
func (s SequentialQueryTracker) Delete(int)   { <-s }
func (s SequentialQueryTracker) Close() error { return nil }

func (s SequentialQueryTracker) Insert(ctx context.Context, _ string) (int, error) {
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case s <- struct{}{}:
		return 1, nil
	}
}

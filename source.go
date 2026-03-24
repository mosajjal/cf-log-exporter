package main

import (
	"context"
	"time"
)

// Source is implemented by each log data source.
type Source interface {
	// Name returns a unique key used for state tracking (e.g. "firewall:zone123").
	Name() string
	// Poll fetches events since the given time.
	// Returns the events, the next "since" cursor, and any error.
	Poll(ctx context.Context, since time.Time) (events []Event, next time.Time, err error)
}

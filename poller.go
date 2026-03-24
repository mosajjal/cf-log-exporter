package main

import (
	"context"
	"log/slog"
	"time"
)

// runPoller polls src on interval until ctx is cancelled.
// On each tick it calls src.Poll, forwards events to out, and updates state.
func runPoller(ctx context.Context, src Source, st *State, interval time.Duration, out chan<- Event) {
	pollOnce(ctx, src, st, out)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pollOnce(ctx, src, st, out)
		}
	}
}

func pollOnce(ctx context.Context, src Source, st *State, out chan<- Event) {
	since := st.Get(src.Name())
	if since.IsZero() {
		// Default: look back 5 minutes on first run.
		since = time.Now().Add(-5 * time.Minute)
	}

	events, next, err := src.Poll(ctx, since)
	if err != nil {
		slog.Error("poll failed", "source", src.Name(), "err", err)
		return
	}

	for _, e := range events {
		select {
		case out <- e:
		case <-ctx.Done():
			return
		}
	}

	if !next.IsZero() {
		st.Set(src.Name(), next)
	}
	if len(events) > 0 {
		slog.Info("polled", "source", src.Name(), "count", len(events))
	}
}

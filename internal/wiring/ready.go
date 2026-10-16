package wiring

import (
	"context"
	"fmt"
	"time"
)

// Probe returns nil once an app's API answers.
type Probe func(ctx context.Context) error

// WaitReady polls probe every interval until it succeeds or timeout passes.
func WaitReady(ctx context.Context, probe Probe, timeout, interval time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	t := time.NewTicker(interval)
	defer t.Stop()
	var last error
	for {
		if last = probe(ctx); last == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out after %s: %w", timeout, last)
		case <-t.C:
		}
	}
}

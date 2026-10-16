package wiring

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestWaitReadySucceedsAfterRetries(t *testing.T) {
	n := 0
	err := WaitReady(context.Background(), func(context.Context) error {
		n++
		if n < 3 {
			return errors.New("connection refused")
		}
		return nil
	}, time.Second, time.Millisecond)
	if err != nil || n != 3 {
		t.Fatalf("err=%v attempts=%d", err, n)
	}
}

func TestWaitReadyTimesOut(t *testing.T) {
	err := WaitReady(context.Background(), func(context.Context) error {
		return errors.New("connection refused")
	}, 30*time.Millisecond, 5*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "timed out") || !strings.Contains(err.Error(), "connection refused") {
		t.Fatalf("want timeout wrapping last error, got %v", err)
	}
}

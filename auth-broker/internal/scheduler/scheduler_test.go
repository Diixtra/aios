package scheduler

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestPeriodic_FiresAtInterval(t *testing.T) {
	var calls int32
	s := New(10*time.Millisecond, func(_ context.Context) error {
		atomic.AddInt32(&calls, 1)
		return nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 55*time.Millisecond)
	defer cancel()
	s.Run(ctx)
	got := atomic.LoadInt32(&calls)
	if got < 4 || got > 6 {
		t.Fatalf("got %d calls, want ~5", got)
	}
}

func TestPeriodic_ContinuesAfterJobError(t *testing.T) {
	var calls int32
	s := New(10*time.Millisecond, func(_ context.Context) error {
		atomic.AddInt32(&calls, 1)
		return errors.New("boom")
	})
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Millisecond)
	defer cancel()
	s.Run(ctx)
	got := atomic.LoadInt32(&calls)
	if got < 2 {
		t.Fatalf("got %d, want >=2 (errors should not stop the loop)", got)
	}
}

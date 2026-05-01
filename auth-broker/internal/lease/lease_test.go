package lease

import (
	"context"
	"testing"
	"time"
)

func TestAcquire_RespectsCap(t *testing.T) {
	mgr := New(2, time.Hour)

	a, err := mgr.Acquire(context.Background(), "agent-1")
	if err != nil {
		t.Fatal(err)
	}
	b, err := mgr.Acquire(context.Background(), "agent-2")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := mgr.Acquire(ctx, "agent-3"); err == nil {
		t.Fatal("expected timeout")
	}

	if err := mgr.Release(a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Acquire(context.Background(), "agent-4"); err != nil {
		t.Fatalf("should be able to acquire after release: %v", err)
	}
	_ = b
}

func TestAcquire_ExpiresStaleLease(t *testing.T) {
	mgr := New(1, 10*time.Millisecond)
	if _, err := mgr.Acquire(context.Background(), "agent-1"); err != nil {
		t.Fatal(err)
	}
	// Wait past expiry; new acquire should succeed.
	time.Sleep(20 * time.Millisecond)
	if _, err := mgr.Acquire(context.Background(), "agent-2"); err != nil {
		t.Fatalf("should reclaim stale lease: %v", err)
	}
}

func TestRelease_RejectsUnknownID(t *testing.T) {
	mgr := New(1, time.Hour)
	if err := mgr.Release("nope"); err == nil {
		t.Fatal("expected error for unknown lease id")
	}
}

func TestActive_ReportsCount(t *testing.T) {
	mgr := New(3, time.Hour)
	if got := mgr.Active(); got != 0 {
		t.Fatalf("got %d, want 0", got)
	}
	_, _ = mgr.Acquire(context.Background(), "x")
	_, _ = mgr.Acquire(context.Background(), "y")
	if got := mgr.Active(); got != 2 {
		t.Fatalf("got %d, want 2", got)
	}
}

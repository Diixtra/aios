package state

import (
	"context"
	"testing"
	"time"
)

func TestStoreAddAndGetMapping(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	m := Mapping{
		TickTickProjectID: "p1",
		TickTickTaskID:    "t1",
		GitHubRepo:        "Diixtra/aios",
		GitHubIssueNumber: 42,
		LastSyncedAt:      time.Now(),
	}

	if err := s.AddMapping(ctx, m); err != nil {
		t.Fatalf("AddMapping error: %v", err)
	}

	got := s.FindByTickTick(ctx, "p1", "t1")
	if got == nil {
		t.Fatal("FindByTickTick returned nil")
	}
	if got.GitHubIssueNumber != 42 {
		t.Errorf("issue number = %d, want 42", got.GitHubIssueNumber)
	}

	got2 := s.FindByGitHub(ctx, "Diixtra/aios", 42)
	if got2 == nil {
		t.Fatal("FindByGitHub returned nil")
	}
	if got2.TickTickTaskID != "t1" {
		t.Errorf("task ID = %q, want %q", got2.TickTickTaskID, "t1")
	}
}

func TestStoreNotFound(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	if s.FindByTickTick(ctx, "nope", "nope") != nil {
		t.Error("expected nil for unknown TickTick ref")
	}
	if s.FindByGitHub(ctx, "nope/nope", 999) != nil {
		t.Error("expected nil for unknown GitHub ref")
	}
}

func TestStoreDuplicateMapping(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	m := Mapping{
		TickTickProjectID: "p1",
		TickTickTaskID:    "t1",
		GitHubRepo:        "Diixtra/aios",
		GitHubIssueNumber: 42,
		LastSyncedAt:      time.Now(),
	}
	s.AddMapping(ctx, m)

	m.LastSyncedAt = time.Now().Add(time.Hour)
	s.AddMapping(ctx, m)

	all := s.AllMappings(ctx)
	if len(all) != 1 {
		t.Errorf("got %d mappings, want 1", len(all))
	}
}

func TestStorePollTimestamps(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	now := time.Now().Truncate(time.Second)
	s.SetLastTickTickPoll(ctx, now)
	s.SetLastGitHubPoll(ctx, now)

	if got := s.LastTickTickPoll(ctx); !got.Equal(now) {
		t.Errorf("TickTick poll = %v, want %v", got, now)
	}
	if got := s.LastGitHubPoll(ctx); !got.Equal(now) {
		t.Errorf("GitHub poll = %v, want %v", got, now)
	}
}

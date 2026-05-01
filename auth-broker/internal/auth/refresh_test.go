package auth

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRefresh_InvokesPiAndReturnsNilOnSuccess(t *testing.T) {
	dir := t.TempDir()
	fakePi := filepath.Join(dir, "pi-fake")
	if err := os.WriteFile(fakePi, []byte("#!/usr/bin/env bash\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	r := NewRefresher(fakePi, dir)
	if err := r.Refresh(context.Background()); err != nil {
		t.Fatalf("refresh: %v", err)
	}
}

func TestRefresh_ReturnsErrorWhenPiFails(t *testing.T) {
	dir := t.TempDir()
	fakePi := filepath.Join(dir, "pi-fake")
	if err := os.WriteFile(fakePi, []byte("#!/usr/bin/env bash\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	r := NewRefresher(fakePi, dir)
	if err := r.Refresh(context.Background()); err == nil {
		t.Fatal("expected error")
	}
}

func TestRefresh_RejectsEmptyConfig(t *testing.T) {
	r := NewRefresher("", "")
	if err := r.Refresh(context.Background()); err == nil {
		t.Fatal("expected error for empty config")
	}
}

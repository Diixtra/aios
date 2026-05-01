package auth

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestValidate_ReportsSuccessOnZeroExit(t *testing.T) {
	dir := t.TempDir()
	fakePi := filepath.Join(dir, "pi-fake")
	if err := os.WriteFile(fakePi, []byte("#!/usr/bin/env bash\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	v := NewValidator(fakePi, dir)
	if err := v.Validate(context.Background()); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidate_ReportsFailureOnNonZeroExit(t *testing.T) {
	dir := t.TempDir()
	fakePi := filepath.Join(dir, "pi-fake")
	if err := os.WriteFile(fakePi, []byte("#!/usr/bin/env bash\necho 'auth error' >&2; exit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	v := NewValidator(fakePi, dir)
	if err := v.Validate(context.Background()); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestValidate_RejectsEmptyConfig(t *testing.T) {
	v := NewValidator("", "")
	if err := v.Validate(context.Background()); err == nil {
		t.Fatal("expected error")
	}
}

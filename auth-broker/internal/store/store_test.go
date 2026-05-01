package store

import (
	"path/filepath"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := New(filepath.Join(dir, "auth.json"))

	want := Bundle{Raw: []byte(`{"refresh_token":"abc","expires_at":"2099-01-01T00:00:00Z"}`)}
	if err := s.Write(want); err != nil {
		t.Fatal(err)
	}
	got, err := s.Read()
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Raw) != string(want.Raw) {
		t.Fatalf("mismatch")
	}
}

func TestAtomicWrite_NoLeftoverTmp(t *testing.T) {
	dir := t.TempDir()
	s := New(filepath.Join(dir, "auth.json"))
	if err := s.Write(Bundle{Raw: []byte("v1")}); err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.tmp"))
	if len(files) != 0 {
		t.Fatalf("leftover .tmp file: %v", files)
	}
}

func TestWrite_RejectsEmpty(t *testing.T) {
	dir := t.TempDir()
	s := New(filepath.Join(dir, "auth.json"))
	if err := s.Write(Bundle{Raw: nil}); err == nil {
		t.Fatal("expected error for empty bundle")
	}
}

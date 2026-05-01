package server

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Diixtra/aios/auth-broker/internal/lease"
	"github.com/Diixtra/aios/auth-broker/internal/store"
)

func TestHealthz_OK(t *testing.T) {
	srv := New(Config{})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestAcquireLease_HappyPath(t *testing.T) {
	srv := New(Config{Lease: lease.New(2, time.Hour)})
	body, _ := json.Marshal(map[string]string{"holder": "agent-1"})
	req := httptest.NewRequest(http.MethodPost, "/v1/leases/acquire", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, body=%s", rec.Code, rec.Body)
	}
	var resp struct {
		LeaseID   string    `json:"lease_id"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.LeaseID == "" {
		t.Fatal("lease_id empty")
	}
}

func TestAcquireLease_RejectsMissingHolder(t *testing.T) {
	srv := New(Config{Lease: lease.New(1, time.Hour)})
	req := httptest.NewRequest(http.MethodPost, "/v1/leases/acquire", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestReleaseLease_RejectsUnknownID(t *testing.T) {
	srv := New(Config{Lease: lease.New(1, time.Hour)})
	body, _ := json.Marshal(map[string]string{"lease_id": "nope"})
	req := httptest.NewRequest(http.MethodPost, "/v1/leases/release", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestAuthBundle_ReturnsRawBundle(t *testing.T) {
	dir := t.TempDir()
	st := store.New(filepath.Join(dir, "auth.json"))
	if err := st.Write(store.Bundle{Raw: []byte(`{"chatgpt":{"type":"oauth"}}`)}); err != nil {
		t.Fatal(err)
	}
	srv := New(Config{Store: st})
	req := httptest.NewRequest(http.MethodGet, "/v1/auth/bundle", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d", rec.Code)
	}
	if rec.Body.String() != `{"chatgpt":{"type":"oauth"}}` {
		t.Fatalf("got %q", rec.Body.String())
	}
}

func TestAuthBundle_NotFoundWhenStoreEmpty(t *testing.T) {
	dir := t.TempDir()
	st := store.New(filepath.Join(dir, "auth.json"))
	srv := New(Config{Store: st})
	req := httptest.NewRequest(http.MethodGet, "/v1/auth/bundle", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("got %d", rec.Code)
	}
}

type stubOrchestrator struct{ err error }

func (s *stubOrchestrator) OnBundleUploaded(_ context.Context) error { return s.err }

func TestRevalidate_RunsOrchestratorAndReturnsState(t *testing.T) {
	srv := New(Config{Orchestrator: &stubOrchestrator{}})
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/revalidate", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, body=%s", rec.Code, rec.Body)
	}
}

func TestPostRunBundle_AcceptsNewerExpires(t *testing.T) {
	dir := t.TempDir()
	st := store.New(filepath.Join(dir, "auth.json"))
	if err := st.Write(store.Bundle{Raw: []byte(`{"openai-codex":{"expires":1000}}`)}); err != nil {
		t.Fatal(err)
	}
	srv := New(Config{Store: st})

	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	fw, _ := w.CreateFormFile("bundle", "auth.json")
	_, _ = fw.Write([]byte(`{"openai-codex":{"expires":2000}}`))
	_ = w.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/auth/bundle/post-run", body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, body=%s", rec.Code, rec.Body)
	}
	got, _ := st.Read()
	if string(got.Raw) != `{"openai-codex":{"expires":2000}}` {
		t.Fatalf("store not updated: %s", got.Raw)
	}
}

func TestPostRunBundle_RejectsOlderExpires(t *testing.T) {
	dir := t.TempDir()
	st := store.New(filepath.Join(dir, "auth.json"))
	if err := st.Write(store.Bundle{Raw: []byte(`{"openai-codex":{"expires":2000}}`)}); err != nil {
		t.Fatal(err)
	}
	srv := New(Config{Store: st})

	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	fw, _ := w.CreateFormFile("bundle", "auth.json")
	_, _ = fw.Write([]byte(`{"openai-codex":{"expires":1000}}`))
	_ = w.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/auth/bundle/post-run", body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"accepted":false`) {
		t.Fatalf("expected accepted:false body, got %s", rec.Body)
	}
	got, _ := st.Read()
	if string(got.Raw) != `{"openai-codex":{"expires":2000}}` {
		t.Fatalf("store mutated when it should not: %s", got.Raw)
	}
}

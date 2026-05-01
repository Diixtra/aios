package server

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/Diixtra/aios/auth-broker/internal/store"
)

func TestUploadBundle_Persists(t *testing.T) {
	dir := t.TempDir()
	st := store.New(filepath.Join(dir, "auth.json"))
	h := NewBundleHandler(st, func() {})

	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	fw, _ := w.CreateFormFile("bundle", "auth.json")
	_, _ = fw.Write([]byte(`{"chatgpt":{"type":"oauth","refresh_token":"r1"}}`))
	_ = w.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/auth/bundle", body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("got %d, want 202; body=%s", rec.Code, rec.Body)
	}
	got, err := st.Read()
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Raw) != `{"chatgpt":{"type":"oauth","refresh_token":"r1"}}` {
		t.Fatalf("bundle not persisted; got %s", got.Raw)
	}
}

func TestUploadBundle_RejectsNonJSON(t *testing.T) {
	dir := t.TempDir()
	st := store.New(filepath.Join(dir, "auth.json"))
	h := NewBundleHandler(st, func() {})

	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	fw, _ := w.CreateFormFile("bundle", "auth.json")
	_, _ = fw.Write([]byte(`not json`))
	_ = w.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/auth/bundle", body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", rec.Code)
	}
}

func TestUploadBundle_RejectsOversize(t *testing.T) {
	dir := t.TempDir()
	st := store.New(filepath.Join(dir, "auth.json"))
	h := NewBundleHandler(st, func() {})

	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	fw, _ := w.CreateFormFile("bundle", "auth.json")
	_, _ = fw.Write(bytes.Repeat([]byte("x"), 2*1024*1024)) // 2MB > limit
	_ = w.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/auth/bundle", body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("got %d, want 413", rec.Code)
	}
}

func TestUploadBundle_TriggersValidate(t *testing.T) {
	dir := t.TempDir()
	st := store.New(filepath.Join(dir, "auth.json"))
	called := false
	h := NewBundleHandler(st, func() { called = true })

	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	fw, _ := w.CreateFormFile("bundle", "auth.json")
	_, _ = fw.Write([]byte(`{"x":1}`))
	_ = w.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/auth/bundle", body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("got %d", rec.Code)
	}
	if !called {
		t.Fatal("triggerValidate not invoked")
	}
}

package paperless

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// mockPaperlessAPI returns a test server that serves all needed Paperless endpoints.
func mockPaperlessAPI(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/documents/42/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			w.WriteHeader(http.StatusOK)
			return
		}
		json.NewEncoder(w).Encode(apiDocumentResponse{
			ID:            42,
			Title:         "March Invoice",
			Content:       "Invoice for services rendered in March 2026.",
			Created:       "2026-03-01T00:00:00Z",
			Added:         "2026-03-30T10:00:00Z",
			Correspondent: 5,
			Tags:          []int{1},
		})
	})
	mux.HandleFunc("/api/correspondents/5/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"id": 5, "name": "Acme Corp"})
	})
	mux.HandleFunc("/api/tags/1/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"id": 1, "name": "business"})
	})
	mux.HandleFunc("/api/tags/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{{"id": 8, "name": "invoice"}},
			})
			return
		}
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]any{"id": 9, "name": "new-tag"})
		}
	})
	return httptest.NewServer(mux)
}

// mockLocalAI returns a test server for LocalAI classification.
func mockLocalAI(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": `["invoice"]`}},
			},
		})
	}))
}

// mockSearchAPI returns a test server for aios-search.
func mockSearchAPI(t *testing.T, vaultDir string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{
					"file_path": filepath.Join(vaultDir, "Projects", "acme-project.md"),
					"score":     0.85,
				},
			},
		})
	}))
}

func TestHandler_HappyPath(t *testing.T) {
	paperlessAPI := mockPaperlessAPI(t)
	defer paperlessAPI.Close()
	localAI := mockLocalAI(t)
	defer localAI.Close()
	vaultDir := t.TempDir()

	// Create a related note that will be auto-linked
	projectDir := filepath.Join(vaultDir, "Projects")
	os.MkdirAll(projectDir, 0o755)
	os.WriteFile(filepath.Join(projectDir, "acme-project.md"), []byte("---\ntitle: Acme Project\ntype: project\nstatus: active\n---\n\n# Acme Project\n"), 0o644)

	searchAPI := mockSearchAPI(t, vaultDir)
	defer searchAPI.Close()

	h := NewHandler(
		"test-secret",
		paperlessAPI.URL,
		"test-token",
		"https://paperless.lab.kazie.co.uk",
		localAI.URL,
		"test-model",
		vaultDir,
		searchAPI.URL,
	)

	payload := `{"document_id": 42}`
	req := httptest.NewRequest(http.MethodPost, "/webhook/paperless", bytes.NewBufferString(payload))
	req.Header.Set("X-Paperless-Secret", "test-secret")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify stub note was created
	stubPath := filepath.Join(vaultDir, "Knowledge", "Paperless", "Acme Corp", "march-invoice.md")
	if _, err := os.Stat(stubPath); os.IsNotExist(err) {
		t.Error("stub note was not created")
	}

	// Verify auto-link was added to related note
	content, _ := os.ReadFile(filepath.Join(projectDir, "acme-project.md"))
	if !bytes.Contains(content, []byte("paperless:")) {
		t.Error("auto-link not added to related note")
	}
}

func TestHandler_InvalidSecret(t *testing.T) {
	h := NewHandler("correct-secret", "", "", "", "", "", "", "")

	req := httptest.NewRequest(http.MethodPost, "/webhook/paperless", bytes.NewBufferString(`{"document_id": 1}`))
	req.Header.Set("X-Paperless-Secret", "wrong-secret")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestHandler_MissingDocumentID(t *testing.T) {
	h := NewHandler("secret", "", "", "", "", "", "", "")

	req := httptest.NewRequest(http.MethodPost, "/webhook/paperless", bytes.NewBufferString(`{}`))
	req.Header.Set("X-Paperless-Secret", "secret")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

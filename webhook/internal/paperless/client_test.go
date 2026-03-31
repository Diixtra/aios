package paperless

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_GetDocument(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/documents/42/", func(w http.ResponseWriter, r *http.Request) {
		resp := apiDocumentResponse{
			ID:            42,
			Title:         "Tax Return 2024",
			Content:       "Dear Mr. Sherlock, your tax return...",
			Created:       "2025-01-15T00:00:00Z",
			Added:         "2026-03-30T10:00:00Z",
			Correspondent: 5,
			Tags:          []int{1, 3},
		}
		json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("/api/correspondents/5/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"id": 5, "name": "HMRC"})
	})
	mux.HandleFunc("/api/tags/1/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"id": 1, "name": "tax"})
	})
	mux.HandleFunc("/api/tags/3/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"id": 3, "name": "self-assessment"})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	c := NewClient(server.URL, "test-token", "https://paperless.lab.kazie.co.uk")
	doc, err := c.GetDocument(context.Background(), 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if doc.ID != 42 {
		t.Errorf("expected ID 42, got %d", doc.ID)
	}
	if doc.Title != "Tax Return 2024" {
		t.Errorf("expected title 'Tax Return 2024', got %q", doc.Title)
	}
	if doc.Correspondent != "HMRC" {
		t.Errorf("expected correspondent 'HMRC', got %q", doc.Correspondent)
	}
	if len(doc.Tags) != 2 || doc.Tags[0] != "tax" || doc.Tags[1] != "self-assessment" {
		t.Errorf("unexpected tags: %v", doc.Tags)
	}
	if doc.Content != "Dear Mr. Sherlock, your tax return..." {
		t.Errorf("unexpected content: %q", doc.Content)
	}
	if doc.OriginalURL != "https://paperless.lab.kazie.co.uk/documents/42" {
		t.Errorf("unexpected URL: %q", doc.OriginalURL)
	}
}

func TestClient_GetDocument_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	c := NewClient(server.URL, "test-token", "https://paperless.lab.kazie.co.uk")
	_, err := c.GetDocument(context.Background(), 999)
	if err == nil {
		t.Fatal("expected error for 404")
	}
}

func TestClient_GetOrCreateTag_Existing(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/tags/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{
					{"id": 7, "name": "invoice"},
				},
			})
			return
		}
		t.Error("unexpected POST to tags")
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	c := NewClient(server.URL, "test-token", "https://paperless.lab.kazie.co.uk")
	id, err := c.GetOrCreateTag(context.Background(), "invoice")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 7 {
		t.Errorf("expected tag ID 7, got %d", id)
	}
}

func TestClient_GetOrCreateTag_New(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/tags/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(map[string]any{"results": []any{}})
			return
		}
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]any{"id": 12, "name": "new-tag"})
			return
		}
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	c := NewClient(server.URL, "test-token", "https://paperless.lab.kazie.co.uk")
	id, err := c.GetOrCreateTag(context.Background(), "new-tag")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 12 {
		t.Errorf("expected tag ID 12, got %d", id)
	}
}

func TestClient_UpdateTags(t *testing.T) {
	var capturedTags []int
	mux := http.NewServeMux()
	mux.HandleFunc("/api/documents/42/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("expected PATCH, got %s", r.Method)
		}
		var body struct {
			Tags []int `json:"tags"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		capturedTags = body.Tags
		w.WriteHeader(http.StatusOK)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	c := NewClient(server.URL, "test-token", "https://paperless.lab.kazie.co.uk")
	err := c.UpdateTags(context.Background(), 42, []int{1, 3, 7})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(capturedTags) != 3 || capturedTags[0] != 1 || capturedTags[1] != 3 || capturedTags[2] != 7 {
		t.Errorf("unexpected tags sent: %v", capturedTags)
	}
}

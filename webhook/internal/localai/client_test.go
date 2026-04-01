package localai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Diixtra/aios/webhook/internal/document"
)

func TestClient_ClassifyDocument(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		var req chatRequest
		json.NewDecoder(r.Body).Decode(&req)

		if req.Model == "" {
			t.Error("model should not be empty")
		}

		resp := chatResponse{
			Choices: []chatChoice{
				{Message: chatMessage{Content: `["invoice", "utility-bill"]`}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewClient(server.URL, "test-model")
	doc := &document.Document{
		Title:         "March 2026 Electricity Bill",
		Correspondent: "British Gas",
		Content:       "Your electricity bill for March 2026 is £142.50",
		Tags:          []string{},
	}

	tags, err := c.ClassifyDocument(context.Background(), doc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tags) != 2 || tags[0] != "invoice" || tags[1] != "utility-bill" {
		t.Errorf("unexpected tags: %v", tags)
	}
}

func TestClient_ClassifyDocument_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := chatResponse{
			Choices: []chatChoice{
				{Message: chatMessage{Content: "not json"}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewClient(server.URL, "test-model")
	doc := &document.Document{Title: "Test", Content: "test"}

	_, err := c.ClassifyDocument(context.Background(), doc)
	if err == nil {
		t.Fatal("expected error for invalid JSON response")
	}
}

func TestClient_ClassifyDocument_ContentTruncation(t *testing.T) {
	var capturedBody chatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedBody)
		resp := chatResponse{
			Choices: []chatChoice{
				{Message: chatMessage{Content: `["test"]`}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewClient(server.URL, "test-model")
	longContent := make([]byte, 5000)
	for i := range longContent {
		longContent[i] = 'a'
	}
	doc := &document.Document{Title: "Test", Content: string(longContent)}

	_, err := c.ClassifyDocument(context.Background(), doc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	userMsg := capturedBody.Messages[1].Content
	if len(userMsg) > 2200 {
		t.Errorf("content not truncated: length %d", len(userMsg))
	}
}

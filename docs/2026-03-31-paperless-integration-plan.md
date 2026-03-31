# Paperless-ngx Integration — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When a document is consumed by Paperless-ngx, auto-tag it via LocalAI, create a stub note in the Obsidian vault, and auto-link it to related vault notes.

**Architecture:** New `/webhook/paperless` route in the existing aios-webhook Go service. Three new internal packages (paperless API client, LocalAI client, vault writer) plus a handler that orchestrates them. K8s manifest updates for secrets, network policies, and deployment env vars.

**Tech Stack:** Go 1.25, net/http, Paperless-ngx REST API, LocalAI OpenAI-compatible API, YAML frontmatter, Kustomize

---

### Task 1: Paperless API Client

**Files:**
- Create: `webhook/internal/paperless/client.go`
- Create: `webhook/internal/paperless/client_test.go`

- [ ] **Step 1: Write the failing test for GetDocument**

Create `webhook/internal/paperless/client_test.go`:

```go
package paperless

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_GetDocument(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/documents/42/" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Token test-token" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}

		resp := apiDocumentResponse{
			ID:      42,
			Title:   "Tax Return 2024",
			Content: "Dear Mr. Sherlock, your tax return...",
			Created: "2025-01-15T00:00:00Z",
			Added:   "2026-03-30T10:00:00Z",
			Correspondent: 5,
			Tags:          []int{1, 3},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Need a second handler for correspondent and tag lookups
	// We'll use a mux instead
	server.Close()

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
	server = httptest.NewServer(mux)
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd webhook && go test ./internal/paperless/ -v -run TestClient_GetDocument`
Expected: FAIL — package does not exist

- [ ] **Step 3: Write the Document type and client GetDocument implementation**

Create `webhook/internal/paperless/client.go`:

```go
package paperless

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Document represents a Paperless-ngx document with resolved names.
type Document struct {
	ID            int       `json:"id"`
	Title         string    `json:"title"`
	Content       string    `json:"content"`
	Correspondent string    `json:"correspondent"`
	Tags          []string  `json:"tags"`
	Created       time.Time `json:"created"`
	Added         time.Time `json:"added"`
	OriginalURL   string    `json:"original_url"`
}

// Client talks to the Paperless-ngx REST API.
type Client struct {
	baseURL    string
	token      string
	externalURL string
	httpClient *http.Client
}

// NewClient creates a Paperless API client.
// baseURL is the internal service URL (e.g. http://paperless-paperless-ngx.paperless.svc:8000).
// externalURL is the public-facing URL for links (e.g. https://paperless.lab.kazie.co.uk).
func NewClient(baseURL, token, externalURL string) *Client {
	return &Client{
		baseURL:     baseURL,
		token:       token,
		externalURL: externalURL,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
	}
}

// apiDocumentResponse matches the Paperless API JSON for GET /api/documents/{id}/.
type apiDocumentResponse struct {
	ID            int    `json:"id"`
	Title         string `json:"title"`
	Content       string `json:"content"`
	Correspondent int    `json:"correspondent"`
	Tags          []int  `json:"tags"`
	Created       string `json:"created"`
	Added         string `json:"added"`
}

// GetDocument fetches a document by ID, resolving correspondent and tag names.
func (c *Client) GetDocument(ctx context.Context, id int) (*Document, error) {
	var apiDoc apiDocumentResponse
	if err := c.get(ctx, fmt.Sprintf("/api/documents/%d/", id), &apiDoc); err != nil {
		return nil, fmt.Errorf("fetch document %d: %w", id, err)
	}

	correspondent := ""
	if apiDoc.Correspondent > 0 {
		name, err := c.getCorrespondentName(ctx, apiDoc.Correspondent)
		if err != nil {
			return nil, fmt.Errorf("resolve correspondent %d: %w", apiDoc.Correspondent, err)
		}
		correspondent = name
	}

	tags := make([]string, 0, len(apiDoc.Tags))
	for _, tagID := range apiDoc.Tags {
		name, err := c.getTagName(ctx, tagID)
		if err != nil {
			return nil, fmt.Errorf("resolve tag %d: %w", tagID, err)
		}
		tags = append(tags, name)
	}

	created, _ := time.Parse(time.RFC3339, apiDoc.Created)
	added, _ := time.Parse(time.RFC3339, apiDoc.Added)

	return &Document{
		ID:            apiDoc.ID,
		Title:         apiDoc.Title,
		Content:       apiDoc.Content,
		Correspondent: correspondent,
		Tags:          tags,
		Created:       created,
		Added:         added,
		OriginalURL:   fmt.Sprintf("%s/documents/%d", c.externalURL, apiDoc.ID),
	}, nil
}

func (c *Client) getCorrespondentName(ctx context.Context, id int) (string, error) {
	var resp struct {
		Name string `json:"name"`
	}
	if err := c.get(ctx, fmt.Sprintf("/api/correspondents/%d/", id), &resp); err != nil {
		return "", err
	}
	return resp.Name, nil
}

func (c *Client) getTagName(ctx context.Context, id int) (string, error) {
	var resp struct {
		Name string `json:"name"`
	}
	if err := c.get(ctx, fmt.Sprintf("/api/tags/%d/", id), &resp); err != nil {
		return "", err
	}
	return resp.Name, nil
}

// get performs an authenticated GET request and decodes the JSON response.
func (c *Client) get(ctx context.Context, path string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Token "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d for %s", resp.StatusCode, path)
	}

	return json.NewDecoder(resp.Body).Decode(dest)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd webhook && go test ./internal/paperless/ -v -run TestClient_GetDocument`
Expected: PASS (both subtests)

- [ ] **Step 5: Write failing tests for GetOrCreateTag and UpdateTags**

Append to `webhook/internal/paperless/client_test.go`:

```go
func TestClient_GetOrCreateTag_Existing(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/tags/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			// List tags filtered by name
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
```

- [ ] **Step 6: Run tests to verify they fail**

Run: `cd webhook && go test ./internal/paperless/ -v`
Expected: FAIL — `GetOrCreateTag` and `UpdateTags` undefined

- [ ] **Step 7: Implement GetOrCreateTag and UpdateTags**

Append to `webhook/internal/paperless/client.go`:

```go
// GetOrCreateTag finds a tag by name, creating it if it doesn't exist.
func (c *Client) GetOrCreateTag(ctx context.Context, name string) (int, error) {
	// Search for existing tag
	var listResp struct {
		Results []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"results"`
	}
	if err := c.get(ctx, fmt.Sprintf("/api/tags/?name__iexact=%s", name), &listResp); err != nil {
		return 0, fmt.Errorf("search tag %q: %w", name, err)
	}
	if len(listResp.Results) > 0 {
		return listResp.Results[0].ID, nil
	}

	// Create new tag
	body := map[string]string{"name": name}
	var createResp struct {
		ID int `json:"id"`
	}
	if err := c.post(ctx, "/api/tags/", body, &createResp); err != nil {
		return 0, fmt.Errorf("create tag %q: %w", name, err)
	}
	return createResp.ID, nil
}

// UpdateTags sets the tag IDs on a document.
func (c *Client) UpdateTags(ctx context.Context, docID int, tagIDs []int) error {
	body := map[string]any{"tags": tagIDs}
	return c.patch(ctx, fmt.Sprintf("/api/documents/%d/", docID), body)
}

func (c *Client) post(ctx context.Context, path string, body any, dest any) error {
	encoded, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(encoded))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Token "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d for POST %s", resp.StatusCode, path)
	}

	if dest != nil {
		return json.NewDecoder(resp.Body).Decode(dest)
	}
	return nil
}

func (c *Client) patch(ctx context.Context, path string, body any) error {
	encoded, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, c.baseURL+path, bytes.NewReader(encoded))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Token "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d for PATCH %s", resp.StatusCode, path)
	}
	return nil
}
```

Add `"bytes"` to the imports.

- [ ] **Step 8: Run all tests to verify they pass**

Run: `cd webhook && go test ./internal/paperless/ -v`
Expected: PASS (all 5 tests)

- [ ] **Step 9: Commit**

```bash
cd webhook
git add internal/paperless/
git commit -m "feat(webhook): add Paperless-ngx API client

GetDocument, GetOrCreateTag, UpdateTags with token auth."
```

---

### Task 2: LocalAI Classification Client

**Files:**
- Create: `webhook/internal/localai/client.go`
- Create: `webhook/internal/localai/client_test.go`

- [ ] **Step 1: Write the failing test**

Create `webhook/internal/localai/client_test.go`:

```go
package localai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Diixtra/aios/webhook/internal/paperless"
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

		// Return a valid classification response
		resp := chatResponse{
			Choices: []chatChoice{
				{Message: chatMessage{Content: `["invoice", "utility-bill"]`}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewClient(server.URL, "test-model")
	doc := &paperless.Document{
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
	doc := &paperless.Document{Title: "Test", Content: "test"}

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
	doc := &paperless.Document{Title: "Test", Content: string(longContent)}

	_, err := c.ClassifyDocument(context.Background(), doc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// User message should contain truncated content (max 2000 chars)
	userMsg := capturedBody.Messages[1].Content
	if len(userMsg) > 2200 { // some overhead for title/correspondent prefix
		t.Errorf("content not truncated: length %d", len(userMsg))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd webhook && go test ./internal/localai/ -v`
Expected: FAIL — package does not exist

- [ ] **Step 3: Implement the LocalAI client**

Create `webhook/internal/localai/client.go`:

```go
package localai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/Diixtra/aios/webhook/internal/paperless"
)

const maxContentChars = 2000

const systemPrompt = `You are a document classifier. Given the document metadata and content below, return a JSON array of lowercase tag names that describe this document.
Use specific, consistent tags like: invoice, receipt, tax, contract, letter, bank-statement, insurance, medical, utility-bill, payslip.
Return only the JSON array, no other text.`

// Client calls LocalAI's OpenAI-compatible chat completions endpoint.
type Client struct {
	baseURL    string
	model      string
	httpClient *http.Client
}

// NewClient creates a LocalAI client.
func NewClient(baseURL, model string) *Client {
	return &Client{
		baseURL:    baseURL,
		model:      model,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []chatChoice `json:"choices"`
}

type chatChoice struct {
	Message chatMessage `json:"message"`
}

// ClassifyDocument returns suggested tags for a document.
func (c *Client) ClassifyDocument(ctx context.Context, doc *paperless.Document) ([]string, error) {
	content := doc.Content
	if len(content) > maxContentChars {
		content = content[:maxContentChars]
	}

	userMessage := fmt.Sprintf("Title: %s\nCorrespondent: %s\nExisting tags: %v\n\nContent:\n%s",
		doc.Title, doc.Correspondent, doc.Tags, content)

	reqBody := chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userMessage},
		},
	}

	encoded, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions", bytes.NewReader(encoded))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("localai request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("localai returned status %d", resp.StatusCode)
	}

	var chatResp chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	var tags []string
	if err := json.Unmarshal([]byte(chatResp.Choices[0].Message.Content), &tags); err != nil {
		return nil, fmt.Errorf("parse tags from response %q: %w", chatResp.Choices[0].Message.Content, err)
	}

	return tags, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd webhook && go test ./internal/localai/ -v`
Expected: PASS (all 3 tests)

- [ ] **Step 5: Commit**

```bash
cd webhook
git add internal/localai/
git commit -m "feat(webhook): add LocalAI classification client

Calls /v1/chat/completions to classify Paperless documents
and return suggested tags. Truncates content to 2000 chars."
```

---

### Task 3: Vault Stub Note Writer

**Files:**
- Create: `webhook/internal/vault/writer.go`
- Create: `webhook/internal/vault/writer_test.go`

- [ ] **Step 1: Write the failing test for WriteStub**

Create `webhook/internal/vault/writer_test.go`:

```go
package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Diixtra/aios/webhook/internal/paperless"
)

func TestWriter_WriteStub(t *testing.T) {
	tmpDir := t.TempDir()
	w := NewWriter(tmpDir)

	doc := &paperless.Document{
		ID:            42,
		Title:         "Self Assessment Tax Return 2024-25",
		Content:       "Dear Mr. Sherlock, your tax return...",
		Correspondent: "HMRC",
		Tags:          []string{"tax", "self-assessment"},
		Created:       time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC),
		Added:         time.Date(2026, 3, 30, 0, 0, 0, 0, time.UTC),
		OriginalURL:   "https://paperless.lab.kazie.co.uk/documents/42",
	}

	path, err := w.WriteStub(doc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedPath := filepath.Join(tmpDir, "Knowledge", "Paperless", "HMRC", "self-assessment-tax-return-2024-25.md")
	if path != expectedPath {
		t.Errorf("expected path %q, got %q", expectedPath, path)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read stub: %v", err)
	}

	s := string(content)
	if !strings.Contains(s, "title: Self Assessment Tax Return 2024-25") {
		t.Error("missing title in frontmatter")
	}
	if !strings.Contains(s, "type: paperless-document") {
		t.Error("missing type in frontmatter")
	}
	if !strings.Contains(s, "paperless_id: 42") {
		t.Error("missing paperless_id in frontmatter")
	}
	if !strings.Contains(s, "paperless_url: https://paperless.lab.kazie.co.uk/documents/42") {
		t.Error("missing paperless_url")
	}
	if !strings.Contains(s, "correspondent: HMRC") {
		t.Error("missing correspondent")
	}
	if !strings.Contains(s, "tags: [tax, self-assessment]") {
		t.Error("missing tags")
	}
	if !strings.Contains(s, "Dear Mr. Sherlock, your tax return...") {
		t.Error("missing OCR content in body")
	}
	if !strings.Contains(s, "[View in Paperless]") {
		t.Error("missing Paperless link")
	}
}

func TestWriter_WriteStub_NoCorrespondent(t *testing.T) {
	tmpDir := t.TempDir()
	w := NewWriter(tmpDir)

	doc := &paperless.Document{
		ID:          99,
		Title:       "Unknown Document",
		Content:     "Some content",
		Tags:        []string{},
		Created:     time.Now(),
		Added:       time.Now(),
		OriginalURL: "https://paperless.lab.kazie.co.uk/documents/99",
	}

	path, err := w.WriteStub(doc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedPath := filepath.Join(tmpDir, "Knowledge", "Paperless", "Uncategorised", "unknown-document.md")
	if path != expectedPath {
		t.Errorf("expected path %q, got %q", expectedPath, path)
	}
}

func TestWriter_WriteStub_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()
	w := NewWriter(tmpDir)

	doc := &paperless.Document{
		ID:          42,
		Title:       "Test Doc",
		Content:     "Version 1",
		Tags:        []string{},
		Created:     time.Now(),
		Added:       time.Now(),
		OriginalURL: "https://paperless.lab.kazie.co.uk/documents/42",
	}

	path1, _ := w.WriteStub(doc)

	doc.Content = "Version 2"
	path2, _ := w.WriteStub(doc)

	if path1 != path2 {
		t.Error("paths should be identical for same document")
	}

	content, _ := os.ReadFile(path2)
	if !strings.Contains(string(content), "Version 2") {
		t.Error("stub should be overwritten with new content")
	}
}

func TestSanitiseFilename(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Hello World", "hello-world"},
		{"Invoice #123 (March)", "invoice-123-march"},
		{"  Multiple   Spaces  ", "multiple-spaces"},
		{"Special/Chars\\Here", "specialcharshere"},
		{strings.Repeat("a", 150), strings.Repeat("a", 100)},
	}

	for _, tt := range tests {
		got := sanitiseFilename(tt.input)
		if got != tt.expected {
			t.Errorf("sanitiseFilename(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd webhook && go test ./internal/vault/ -v`
Expected: FAIL — package does not exist

- [ ] **Step 3: Implement the vault writer**

Create `webhook/internal/vault/writer.go`:

```go
package vault

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Diixtra/aios/webhook/internal/paperless"
)

// Writer creates and updates stub notes in the Obsidian vault.
type Writer struct {
	vaultPath string
}

// NewWriter creates a vault writer rooted at the given path.
func NewWriter(vaultPath string) *Writer {
	return &Writer{vaultPath: vaultPath}
}

// WriteStub creates a stub markdown note for a Paperless document.
// Returns the absolute path of the written file.
// If a stub already exists for the same document, it is overwritten.
func (w *Writer) WriteStub(doc *paperless.Document) (string, error) {
	correspondent := doc.Correspondent
	if correspondent == "" {
		correspondent = "Uncategorised"
	}

	filename := sanitiseFilename(doc.Title) + ".md"
	dir := filepath.Join(w.vaultPath, "Knowledge", "Paperless", correspondent)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create directory %s: %w", dir, err)
	}

	path := filepath.Join(dir, filename)

	tagsFormatted := "[]"
	if len(doc.Tags) > 0 {
		tagsFormatted = "[" + strings.Join(doc.Tags, ", ") + "]"
	}

	content := fmt.Sprintf(`---
title: %s
type: paperless-document
source: paperless
paperless_id: %d
paperless_url: %s
correspondent: %s
tags: %s
created: %s
added: %s
entity: []
status: active
---

# %s

[View in Paperless](%s)

## Content

%s
`,
		doc.Title,
		doc.ID,
		doc.OriginalURL,
		correspondent,
		tagsFormatted,
		doc.Created.Format(time.DateOnly),
		doc.Added.Format(time.DateOnly),
		doc.Title,
		doc.OriginalURL,
		doc.Content,
	)

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write stub %s: %w", path, err)
	}

	return path, nil
}

var nonAlphanumeric = regexp.MustCompile(`[^a-z0-9\-]+`)
var multiDash = regexp.MustCompile(`-{2,}`)

// sanitiseFilename converts a title to a safe, lowercase, hyphenated filename (without extension).
func sanitiseFilename(title string) string {
	s := strings.ToLower(strings.TrimSpace(title))
	s = strings.ReplaceAll(s, " ", "-")
	s = nonAlphanumeric.ReplaceAllString(s, "")
	s = multiDash.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 100 {
		s = s[:100]
	}
	return s
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd webhook && go test ./internal/vault/ -v`
Expected: PASS (all 4 tests)

- [ ] **Step 5: Write failing test for AddPaperlessLink (auto-link frontmatter update)**

Append to `webhook/internal/vault/writer_test.go`:

```go
func TestWriter_AddPaperlessLink(t *testing.T) {
	tmpDir := t.TempDir()
	w := NewWriter(tmpDir)

	// Create a note with existing frontmatter
	notePath := filepath.Join(tmpDir, "Projects", "tax-project.md")
	os.MkdirAll(filepath.Dir(notePath), 0o755)
	os.WriteFile(notePath, []byte(`---
title: Tax Project 2025
type: project
status: active
---

# Tax Project 2025

Some content here.
`), 0o644)

	err := w.AddPaperlessLink(notePath, "https://paperless.lab.kazie.co.uk/documents/42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, _ := os.ReadFile(notePath)
	s := string(content)

	if !strings.Contains(s, "paperless:") {
		t.Error("missing paperless key in frontmatter")
	}
	if !strings.Contains(s, "  - https://paperless.lab.kazie.co.uk/documents/42") {
		t.Error("missing paperless URL in frontmatter")
	}
	// Original content should be preserved
	if !strings.Contains(s, "Some content here.") {
		t.Error("original content lost")
	}
}

func TestWriter_AddPaperlessLink_ExistingList(t *testing.T) {
	tmpDir := t.TempDir()
	w := NewWriter(tmpDir)

	notePath := filepath.Join(tmpDir, "note.md")
	os.WriteFile(notePath, []byte(`---
title: Test
paperless:
  - https://paperless.lab.kazie.co.uk/documents/10
---

Content.
`), 0o644)

	err := w.AddPaperlessLink(notePath, "https://paperless.lab.kazie.co.uk/documents/42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, _ := os.ReadFile(notePath)
	s := string(content)

	if !strings.Contains(s, "  - https://paperless.lab.kazie.co.uk/documents/10") {
		t.Error("existing link lost")
	}
	if !strings.Contains(s, "  - https://paperless.lab.kazie.co.uk/documents/42") {
		t.Error("new link not added")
	}
}

func TestWriter_AddPaperlessLink_Duplicate(t *testing.T) {
	tmpDir := t.TempDir()
	w := NewWriter(tmpDir)

	notePath := filepath.Join(tmpDir, "note.md")
	os.WriteFile(notePath, []byte(`---
title: Test
paperless:
  - https://paperless.lab.kazie.co.uk/documents/42
---

Content.
`), 0o644)

	err := w.AddPaperlessLink(notePath, "https://paperless.lab.kazie.co.uk/documents/42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, _ := os.ReadFile(notePath)
	if strings.Count(string(content), "documents/42") != 1 {
		t.Error("duplicate link added")
	}
}
```

- [ ] **Step 6: Run tests to verify they fail**

Run: `cd webhook && go test ./internal/vault/ -v -run TestWriter_AddPaperlessLink`
Expected: FAIL — `AddPaperlessLink` undefined

- [ ] **Step 7: Implement AddPaperlessLink**

Append to `webhook/internal/vault/writer.go`:

```go
// AddPaperlessLink adds a Paperless URL to a vault note's frontmatter.
// If the note already has a paperless list, the URL is appended (if not duplicate).
// If not, a new paperless list is created in the frontmatter.
func (w *Writer) AddPaperlessLink(notePath, paperlessURL string) error {
	content, err := os.ReadFile(notePath)
	if err != nil {
		return fmt.Errorf("read note %s: %w", notePath, err)
	}

	s := string(content)

	// Check for duplicate
	if strings.Contains(s, paperlessURL) {
		return nil
	}

	// Find frontmatter boundaries
	if !strings.HasPrefix(s, "---\n") {
		return fmt.Errorf("note %s has no frontmatter", notePath)
	}
	endIdx := strings.Index(s[4:], "\n---")
	if endIdx == -1 {
		return fmt.Errorf("note %s has unclosed frontmatter", notePath)
	}
	endIdx += 4 // offset for the initial "---\n"

	frontmatter := s[4:endIdx]
	body := s[endIdx+4:] // skip "\n---"

	entry := fmt.Sprintf("  - %s", paperlessURL)

	if strings.Contains(frontmatter, "paperless:") {
		// Append to existing list — find the paperless: line and its list items
		lines := strings.Split(frontmatter, "\n")
		var result []string
		inserted := false
		for i, line := range lines {
			result = append(result, line)
			if strings.TrimSpace(line) == "paperless:" && !inserted {
				// Find the end of the list (next line that doesn't start with "  - ")
				j := i + 1
				for j < len(lines) && strings.HasPrefix(lines[j], "  - ") {
					j++
				}
				// Insert before position j (but we've already appended current line)
				// The loop will continue adding remaining items; we insert after the last "  - " item
				// Actually, let's just append at the end of the list section
				// We need to find where the list ends and insert there
			}
		}
		// Simpler approach: insert the entry right before the closing ---
		// Find last "  - " line after "paperless:" and insert after it
		frontmatter = insertAfterPaperlessList(frontmatter, entry)
	} else {
		// Add new paperless key before the closing ---
		frontmatter = frontmatter + "\npaperless:\n" + entry
	}

	result := "---\n" + frontmatter + "\n---" + body
	return os.WriteFile(notePath, []byte(result), 0o644)
}

func insertAfterPaperlessList(frontmatter, entry string) string {
	lines := strings.Split(frontmatter, "\n")
	var result []string
	inPaperless := false
	inserted := false

	for _, line := range lines {
		if strings.TrimSpace(line) == "paperless:" {
			inPaperless = true
			result = append(result, line)
			continue
		}

		if inPaperless && strings.HasPrefix(line, "  - ") {
			result = append(result, line)
			continue
		}

		if inPaperless && !strings.HasPrefix(line, "  - ") {
			// End of paperless list — insert new entry
			result = append(result, entry)
			inserted = true
			inPaperless = false
		}

		result = append(result, line)
	}

	// If paperless was the last key in frontmatter
	if inPaperless && !inserted {
		result = append(result, entry)
	}

	return strings.Join(result, "\n")
}
```

- [ ] **Step 8: Run all vault tests**

Run: `cd webhook && go test ./internal/vault/ -v`
Expected: PASS (all 7 tests)

- [ ] **Step 9: Commit**

```bash
cd webhook
git add internal/vault/
git commit -m "feat(webhook): add vault stub note writer and auto-linker

WriteStub creates markdown notes with frontmatter + OCR content.
AddPaperlessLink updates related notes' frontmatter with document URLs."
```

---

### Task 4: Paperless Webhook Handler

**Files:**
- Create: `webhook/internal/paperless/handler.go`
- Create: `webhook/internal/paperless/handler_test.go` (append)

- [ ] **Step 1: Write the failing test for the handler**

Append to `webhook/internal/paperless/client_test.go` — actually, create a new test file for the handler:

Create `webhook/internal/paperless/handler_test.go`:

```go
package paperless

import (
	"bytes"
	"context"
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
		// Return a related note path
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
	os.WriteFile(filepath.Join(projectDir, "acme-project.md"), []byte(`---
title: Acme Project
type: project
status: active
---

# Acme Project
`), 0o644)

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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd webhook && go test ./internal/paperless/ -v -run TestHandler`
Expected: FAIL — `NewHandler` undefined

- [ ] **Step 3: Implement the handler**

Create `webhook/internal/paperless/handler.go`:

```go
package paperless

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/Diixtra/aios/webhook/internal/localai"
	"github.com/Diixtra/aios/webhook/internal/vault"
)

// Handler processes Paperless-ngx webhook events.
type Handler struct {
	secret          string
	paperlessClient *Client
	localaiClient   *localai.Client
	vaultWriter     *vault.Writer
	searchURL       string
}

// NewHandler creates a Paperless webhook handler.
func NewHandler(
	secret string,
	paperlessURL string,
	paperlessToken string,
	paperlessDomain string,
	localaiURL string,
	localaiModel string,
	vaultPath string,
	searchURL string,
) *Handler {
	return &Handler{
		secret:          secret,
		paperlessClient: NewClient(paperlessURL, paperlessToken, paperlessDomain),
		localaiClient:   localai.NewClient(localaiURL, localaiModel),
		vaultWriter:     vault.NewWriter(vaultPath),
		searchURL:       searchURL,
	}
}

type webhookPayload struct {
	DocumentID int `json:"document_id"`
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Validate secret
	if r.Header.Get("X-Paperless-Secret") != h.secret {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var payload webhookPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if payload.DocumentID == 0 {
		http.Error(w, "missing document_id", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// 1. Fetch document from Paperless
	doc, err := h.paperlessClient.GetDocument(ctx, payload.DocumentID)
	if err != nil {
		log.Printf("ERROR fetch document %d: %v", payload.DocumentID, err)
		http.Error(w, "failed to fetch document", http.StatusBadGateway)
		return
	}

	// 2. Classify via LocalAI (best-effort)
	suggestedTags, err := h.localaiClient.ClassifyDocument(ctx, doc)
	if err != nil {
		log.Printf("WARN classify document %d: %v (continuing without auto-tag)", doc.ID, err)
	} else {
		if err := h.applyTags(ctx, doc, suggestedTags); err != nil {
			log.Printf("WARN apply tags to document %d: %v", doc.ID, err)
		}
	}

	// 3. Write stub note to vault
	stubPath, err := h.vaultWriter.WriteStub(doc)
	if err != nil {
		log.Printf("ERROR write stub for document %d: %v", doc.ID, err)
		http.Error(w, "failed to write stub note", http.StatusInternalServerError)
		return
	}
	log.Printf("INFO stub created: %s (document %d)", stubPath, doc.ID)

	// 4. Auto-link to related vault notes (best-effort)
	autolinks, err := h.autoLink(ctx, doc)
	if err != nil {
		log.Printf("WARN auto-link document %d: %v", doc.ID, err)
	} else if autolinks > 0 {
		log.Printf("INFO auto-linked document %d to %d notes", doc.ID, autolinks)
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "processed document %d", doc.ID)
}

// applyTags resolves suggested tag names to IDs and updates the document.
func (h *Handler) applyTags(ctx context.Context, doc *Document, suggestedTags []string) error {
	// Deduplicate: skip tags the document already has
	existing := make(map[string]bool, len(doc.Tags))
	for _, t := range doc.Tags {
		existing[t] = true
	}

	var newTagIDs []int
	for _, tagName := range suggestedTags {
		if existing[tagName] {
			continue
		}
		id, err := h.paperlessClient.GetOrCreateTag(ctx, tagName)
		if err != nil {
			return fmt.Errorf("resolve tag %q: %w", tagName, err)
		}
		newTagIDs = append(newTagIDs, id)
	}

	if len(newTagIDs) == 0 {
		return nil
	}

	// We need the existing tag IDs too — re-fetch the raw document to get them
	var rawDoc apiDocumentResponse
	if err := h.paperlessClient.get(ctx, fmt.Sprintf("/api/documents/%d/", doc.ID), &rawDoc); err != nil {
		return fmt.Errorf("re-fetch document: %w", err)
	}

	allTagIDs := append(rawDoc.Tags, newTagIDs...)
	return h.paperlessClient.UpdateTags(ctx, doc.ID, allTagIDs)
}

type searchResponse struct {
	Results []searchResult `json:"results"`
}

type searchResult struct {
	FilePath string  `json:"file_path"`
	Score    float64 `json:"score"`
}

// autoLink queries aios-search for related vault notes and adds paperless links.
func (h *Handler) autoLink(ctx context.Context, doc *Document) (int, error) {
	query := doc.Title
	if doc.Correspondent != "" {
		query += " " + doc.Correspondent
	}
	for _, tag := range doc.Tags {
		query += " " + tag
	}

	searchBody, _ := json.Marshal(map[string]any{
		"query": query,
		"top_k": 5,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.searchURL+"/search", bytes.NewReader(searchBody))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("search request: %w", err)
	}
	defer resp.Body.Close()

	var searchResp searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return 0, fmt.Errorf("decode search response: %w", err)
	}

	count := 0
	for _, result := range searchResp.Results {
		if result.Score < 0.7 {
			continue
		}
		// Skip stub notes (don't self-link)
		if contains(result.FilePath, "Knowledge/Paperless/") {
			continue
		}
		if err := h.vaultWriter.AddPaperlessLink(result.FilePath, doc.OriginalURL); err != nil {
			log.Printf("WARN add link to %s: %v", result.FilePath, err)
			continue
		}
		count++
	}

	return count, nil
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
```

Add `"bytes"` and `"strings"` to the imports. Use `strings.Contains` instead of the custom `contains` function — remove the `contains` and `containsSubstring` helper functions and replace the call with:

```go
if strings.Contains(result.FilePath, "Knowledge/Paperless/") {
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd webhook && go test ./internal/paperless/ -v -run TestHandler`
Expected: PASS (all 3 handler tests)

- [ ] **Step 5: Run ALL tests to check nothing is broken**

Run: `cd webhook && go test ./... -v -race`
Expected: PASS (all tests across all packages)

- [ ] **Step 6: Commit**

```bash
cd webhook
git add internal/paperless/handler.go internal/paperless/handler_test.go
git commit -m "feat(webhook): add Paperless webhook handler

Orchestrates: fetch doc → classify via LocalAI → apply tags →
write stub note → auto-link related vault notes.
LocalAI and auto-link are best-effort (failures don't block stub creation)."
```

---

### Task 5: Wire Handler into main.go

**Files:**
- Modify: `webhook/cmd/main.go`

- [ ] **Step 1: Add new env vars and Paperless handler to main.go**

Add the following to `webhook/cmd/main.go`:

After the existing `GITHUB_WEBHOOK_SECRET` env var loading, add:

```go
	// Paperless webhook config (optional — only enabled if PAPERLESS_API_URL is set)
	paperlessAPIURL := os.Getenv("PAPERLESS_API_URL")
	if paperlessAPIURL != "" {
		paperlessToken := os.Getenv("PAPERLESS_API_TOKEN")
		if paperlessToken == "" {
			log.Fatal("PAPERLESS_API_TOKEN required when PAPERLESS_API_URL is set")
		}
		paperlessSecret := os.Getenv("PAPERLESS_WEBHOOK_SECRET")
		if paperlessSecret == "" {
			log.Fatal("PAPERLESS_WEBHOOK_SECRET required when PAPERLESS_API_URL is set")
		}
		paperlessDomain := os.Getenv("PAPERLESS_DOMAIN")
		if paperlessDomain == "" {
			log.Fatal("PAPERLESS_DOMAIN required when PAPERLESS_API_URL is set")
		}
		localaiURL := os.Getenv("LOCALAI_URL")
		if localaiURL == "" {
			log.Fatal("LOCALAI_URL required when PAPERLESS_API_URL is set")
		}
		localaiModel := os.Getenv("LOCALAI_MODEL")
		if localaiModel == "" {
			localaiModel = "phi-3-mini"
		}
		vaultPath := os.Getenv("VAULT_PATH")
		if vaultPath == "" {
			log.Fatal("VAULT_PATH required when PAPERLESS_API_URL is set")
		}
		searchURL := os.Getenv("AIOS_SEARCH_URL")
		if searchURL == "" {
			log.Fatal("AIOS_SEARCH_URL required when PAPERLESS_API_URL is set")
		}

		paperlessHandler := paperless.NewHandler(
			paperlessSecret,
			paperlessAPIURL,
			paperlessToken,
			paperlessDomain,
			localaiURL,
			localaiModel,
			vaultPath,
			searchURL,
		)
		mux.Handle("/webhook/paperless", paperlessHandler)
		log.Printf("Paperless webhook enabled at /webhook/paperless")
	}
```

Add the import:

```go
	"github.com/Diixtra/aios/webhook/internal/paperless"
```

- [ ] **Step 2: Run all tests**

Run: `cd webhook && go test ./... -v -race`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
cd webhook
git add cmd/main.go
git commit -m "feat(webhook): wire Paperless handler into main.go

New route: POST /webhook/paperless. Enabled when PAPERLESS_API_URL is set.
Requires: PAPERLESS_API_TOKEN, PAPERLESS_WEBHOOK_SECRET, PAPERLESS_DOMAIN,
LOCALAI_URL, VAULT_PATH, AIOS_SEARCH_URL."
```

---

### Task 6: K8s Manifest Updates

**Files:**
- Create: `k8s/base/secrets/paperless-api.yaml`
- Modify: `k8s/base/webhook/deployment.yaml`
- Modify: `k8s/base/policies/network-policies.yaml`
- Modify: `k8s/base/kustomization.yaml`

- [ ] **Step 1: Create OnePasswordItem for Paperless secrets**

Create `k8s/base/secrets/paperless-api.yaml`:

```yaml
apiVersion: onepassword.com/v1
kind: OnePasswordItem
metadata:
  name: aios-paperless-api
  namespace: aios
spec:
  itemPath: "vaults/Homelab/items/aios-paperless-api"
```

- [ ] **Step 2: Add env vars and volume mount to webhook deployment**

Update `k8s/base/webhook/deployment.yaml` — add these env vars to the container spec after the existing `GITHUB_WEBHOOK_SECRET`:

```yaml
            - name: PAPERLESS_API_URL
              value: "http://paperless-paperless-ngx.paperless.svc:8000"
            - name: PAPERLESS_API_TOKEN
              valueFrom:
                secretKeyRef:
                  name: aios-paperless-api
                  key: api-token
            - name: PAPERLESS_WEBHOOK_SECRET
              valueFrom:
                secretKeyRef:
                  name: aios-paperless-api
                  key: webhook-secret
            - name: PAPERLESS_DOMAIN
              value: "https://paperless.lab.kazie.co.uk"
            - name: LOCALAI_URL
              value: "http://local-ai.local-ai.svc:8080"
            - name: LOCALAI_MODEL
              value: "phi-3-mini"
            - name: VAULT_PATH
              value: "/vault"
            - name: AIOS_SEARCH_URL
              value: "http://aios-search.aios.svc:8080"
```

Add volume and volume mount:

```yaml
          volumeMounts:
            - name: vault
              mountPath: /vault
      volumes:
        - name: vault
          hostPath:
            path: /mnt/syncthing/obsidian-vault
            type: Directory
```

Increase memory limit to `256Mi` (OCR text processing needs more):

```yaml
          resources:
            requests:
              cpu: 50m
              memory: 64Mi
            limits:
              cpu: 200m
              memory: 256Mi
```

- [ ] **Step 3: Add network policy for webhook egress**

Append to `k8s/base/policies/network-policies.yaml`:

```yaml
---
# Webhook: allow egress to Paperless, LocalAI, aios-search, and DNS
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: webhook-egress
  namespace: aios
  labels:
    app.kubernetes.io/part-of: aios
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/name: aios-webhook
  policyTypes:
    - Egress
  egress:
    # Allow DNS
    - ports:
        - port: 53
          protocol: UDP
        - port: 53
          protocol: TCP
    # Allow Paperless (port 8000 in paperless namespace)
    - to:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: paperless
      ports:
        - port: 8000
          protocol: TCP
    # Allow LocalAI (port 8080 in local-ai namespace)
    - to:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: local-ai
      ports:
        - port: 8080
          protocol: TCP
    # Allow aios-search (same namespace, port 8080)
    - to:
        - podSelector:
            matchLabels:
              app.kubernetes.io/name: aios-search
      ports:
        - port: 8080
          protocol: TCP
    # Allow K8s API (for RBAC/auth)
    - ports:
        - port: 6443
          protocol: TCP
```

Also add ingress from Paperless namespace to the existing `webhook-ingress` policy — update it to allow from both Traefik and Paperless:

```yaml
  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: traefik
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: paperless
      ports:
        - port: 8080
          protocol: TCP
```

- [ ] **Step 4: Add OnePasswordItem to kustomization.yaml**

Add to the secrets section of `k8s/base/kustomization.yaml`:

```yaml
  - secrets/paperless-api.yaml
```

- [ ] **Step 5: Commit**

```bash
git add k8s/base/secrets/paperless-api.yaml k8s/base/webhook/deployment.yaml k8s/base/policies/network-policies.yaml k8s/base/kustomization.yaml
git commit -m "feat(k8s): add Paperless integration manifests

OnePasswordItem for API token, webhook deployment env vars + vault mount,
network policies for Paperless/LocalAI/aios-search egress."
```

---

### Task 7: GitHub Actions Workflow

**Files:**
- Create: `.github/workflows/webhook-build.yaml`

- [ ] **Step 1: Create the CI workflow**

Create `.github/workflows/webhook-build.yaml`:

```yaml
name: Build webhook

on:
  push:
    branches: [main]
    paths:
      - "webhook/**"
  pull_request:
    paths:
      - "webhook/**"

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.25"
          cache-dependency-path: webhook/go.sum
      - name: Test
        working-directory: webhook
        run: go test ./... -v -race -cover

  build:
    needs: test
    if: github.ref == 'refs/heads/main'
    runs-on: ubuntu-latest
    permissions:
      contents: read
      packages: write
    steps:
      - uses: actions/checkout@v4
      - uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}
      - uses: docker/setup-buildx-action@v3
      - uses: docker/build-push-action@v6
        with:
          context: webhook
          push: true
          tags: ghcr.io/diixtra/aios-webhook:latest
          cache-from: type=gha
          cache-to: type=gha,mode=max
```

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/webhook-build.yaml
git commit -m "ci: add webhook build and test workflow"
```

---

### Task 8: Manual Verification

- [ ] **Step 1: Run full test suite**

Run: `cd webhook && go test ./... -v -race -cover`
Expected: PASS with >80% coverage on new packages

- [ ] **Step 2: Build the Docker image locally**

Run: `cd webhook && docker build -t aios-webhook:test .`
Expected: Image builds successfully

- [ ] **Step 3: Document Paperless UI workflow setup**

Add a note to the spec or README: After deploying, configure the Paperless workflow in the UI:
1. Go to Paperless Admin → Workflows
2. Create new workflow
3. Trigger: "Consumption finished"
4. Action: Webhook
5. URL: `http://aios-webhook.aios.svc:8080/webhook/paperless`
6. Headers: `X-Paperless-Secret: <value from 1Password>`

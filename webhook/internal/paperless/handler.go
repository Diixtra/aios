package paperless

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/Diixtra/aios/webhook/internal/document"
	"github.com/Diixtra/aios/webhook/internal/localai"
	"github.com/Diixtra/aios/webhook/internal/vault"
)

var searchClient = &http.Client{Timeout: 10 * time.Second}

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

	// Validate secret (constant-time comparison to prevent timing attacks)
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Paperless-Secret")), []byte(h.secret)) != 1 {
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
func (h *Handler) applyTags(ctx context.Context, doc *document.Document, suggestedTags []string) error {
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

	// Re-fetch the raw document to get existing tag IDs
	var rawDoc apiDocumentResponse
	if err := h.paperlessClient.get(ctx, fmt.Sprintf("/api/documents/%d/", doc.ID), &rawDoc); err != nil {
		return fmt.Errorf("re-fetch document: %w", err)
	}

	allTagIDs := make([]int, 0, len(rawDoc.Tags)+len(newTagIDs))
	allTagIDs = append(allTagIDs, rawDoc.Tags...)
	allTagIDs = append(allTagIDs, newTagIDs...)
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
func (h *Handler) autoLink(ctx context.Context, doc *document.Document) (int, error) {
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

	resp, err := searchClient.Do(req)
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
		if strings.Contains(result.FilePath, "Knowledge/Paperless/") {
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

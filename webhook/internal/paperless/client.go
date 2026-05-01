package paperless

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/Diixtra/aios/webhook/internal/document"
)

// Client talks to the Paperless-ngx REST API.
type Client struct {
	baseURL     string
	token       string
	externalURL string
	httpClient  *http.Client
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
func (c *Client) GetDocument(ctx context.Context, id int) (*document.Document, error) {
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

	return &document.Document{
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

// GetOrCreateTag finds a tag by name, creating it if it doesn't exist.
func (c *Client) GetOrCreateTag(ctx context.Context, name string) (int, error) {
	var listResp struct {
		Results []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"results"`
	}
	if err := c.get(ctx, "/api/tags/?"+url.Values{"name__iexact": {name}}.Encode(), &listResp); err != nil {
		return 0, fmt.Errorf("search tag %q: %w", name, err)
	}
	if len(listResp.Results) > 0 {
		return listResp.Results[0].ID, nil
	}

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
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d for %s", resp.StatusCode, path)
	}

	return json.NewDecoder(resp.Body).Decode(dest)
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
	defer func() { _ = resp.Body.Close() }()

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
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d for PATCH %s", resp.StatusCode, path)
	}
	return nil
}

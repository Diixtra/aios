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

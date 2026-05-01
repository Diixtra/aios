package ticktick

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

const defaultBaseURL = "https://api.ticktick.com"

// Client is a TickTick Open API client.
type Client struct {
	baseURL      string
	token        string
	refreshToken string
	clientID     string
	clientSecret string
	mu           sync.Mutex
	httpClient   *http.Client
}

// NewClient creates a new TickTick API client without refresh support.
func NewClient(baseURL, accessToken string) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      accessToken,
		httpClient: http.DefaultClient,
	}
}

// NewDefaultClient creates a client pointing at the real TickTick API.
func NewDefaultClient(accessToken string) *Client {
	return NewClient(defaultBaseURL, accessToken)
}

// NewClientWithRefresh creates a client that auto-refreshes expired OAuth tokens.
func NewClientWithRefresh(baseURL, accessToken, refreshToken, clientID, clientSecret string) *Client {
	c := NewClient(baseURL, accessToken)
	c.refreshToken = refreshToken
	c.clientID = clientID
	c.clientSecret = clientSecret
	return c
}

// NewDefaultClientWithRefresh creates a refresh-capable client pointing at the real TickTick API.
func NewDefaultClientWithRefresh(accessToken, refreshToken, clientID, clientSecret string) *Client {
	return NewClientWithRefresh(defaultBaseURL, accessToken, refreshToken, clientID, clientSecret)
}

// ListTasks returns all tasks in a project.
func (c *Client) ListTasks(ctx context.Context, projectID string) ([]Task, error) {
	path := fmt.Sprintf("/open/v1/project/%s/data", projectID)
	var data ProjectData
	if err := c.get(ctx, path, &data); err != nil {
		return nil, fmt.Errorf("list tasks for project %s: %w", projectID, err)
	}
	return data.Tasks, nil
}

// ListAgentTasks returns only tasks with the "agent" tag in a project.
func (c *Client) ListAgentTasks(ctx context.Context, projectID string) ([]Task, error) {
	tasks, err := c.ListTasks(ctx, projectID)
	if err != nil {
		return nil, err
	}
	var agent []Task
	for _, t := range tasks {
		if t.HasTag("agent") {
			agent = append(agent, t)
		}
	}
	return agent, nil
}

// CreateTask creates a new task and returns it.
func (c *Client) CreateTask(ctx context.Context, req CreateTaskRequest) (*Task, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal create request: %w", err)
	}
	var task Task
	if err := c.post(ctx, "/open/v1/task", body, &task); err != nil {
		return nil, fmt.Errorf("create task: %w", err)
	}
	return &task, nil
}

// CompleteTask marks a task as complete.
func (c *Client) CompleteTask(ctx context.Context, projectID, taskID string) error {
	path := fmt.Sprintf("/open/v1/project/%s/task/%s/complete", projectID, taskID)
	resp, err := c.doRequest(ctx, http.MethodPost, path, nil)
	if err != nil {
		return fmt.Errorf("complete task: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized && c.refreshToken != "" {
		_ = resp.Body.Close()
		if err := c.refreshAccessToken(ctx); err != nil {
			return fmt.Errorf("token refresh failed: %w", err)
		}
		resp, err = c.doRequest(ctx, http.MethodPost, path, nil)
		if err != nil {
			return fmt.Errorf("complete task after refresh: %w", err)
		}
		defer func() { _ = resp.Body.Close() }()
	}

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("complete task %s: status %d: %s", taskID, resp.StatusCode, string(b))
	}
	return nil
}

func (c *Client) get(ctx context.Context, path string, out interface{}) error {
	resp, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized && c.refreshToken != "" {
		_ = resp.Body.Close()
		if err := c.refreshAccessToken(ctx); err != nil {
			return fmt.Errorf("token refresh failed: %w", err)
		}
		resp, err = c.doRequest(ctx, http.MethodGet, path, nil)
		if err != nil {
			return err
		}
		defer func() { _ = resp.Body.Close() }()
	}

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GET %s: status %d: %s", path, resp.StatusCode, string(b))
	}

	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) post(ctx context.Context, path string, body []byte, out interface{}) error {
	resp, err := c.doRequest(ctx, http.MethodPost, path, body)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized && c.refreshToken != "" {
		_ = resp.Body.Close()
		if err := c.refreshAccessToken(ctx); err != nil {
			return fmt.Errorf("token refresh failed: %w", err)
		}
		resp, err = c.doRequest(ctx, http.MethodPost, path, body)
		if err != nil {
			return err
		}
		defer func() { _ = resp.Body.Close() }()
	}

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("POST %s: status %d: %s", path, resp.StatusCode, string(b))
	}

	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func (c *Client) doRequest(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = strings.NewReader(string(body))
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	req.Header.Set("Authorization", "Bearer "+c.token)
	c.mu.Unlock()
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.httpClient.Do(req)
}

func (c *Client) refreshAccessToken(ctx context.Context) error {
	if c.refreshToken == "" {
		return fmt.Errorf("no refresh token configured")
	}

	data := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {c.refreshToken},
		"client_id":     {c.clientID},
		"client_secret": {c.clientSecret},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/oauth2/token",
		strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("token refresh failed: %d: %s", resp.StatusCode, string(b))
	}

	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	c.mu.Lock()
	c.token = result.AccessToken
	if result.RefreshToken != "" {
		c.refreshToken = result.RefreshToken
	}
	c.mu.Unlock()

	return nil
}

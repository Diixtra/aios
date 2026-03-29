package ticktick

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const defaultBaseURL = "https://api.ticktick.com"

// Client is a TickTick Open API client.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewClient creates a new TickTick API client.
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
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("complete task: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("complete task %s: status %d: %s", taskID, resp.StatusCode, string(b))
	}
	return nil
}

func (c *Client) get(ctx context.Context, path string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GET %s: status %d: %s", path, resp.StatusCode, string(b))
	}

	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) post(ctx context.Context, path string, body []byte, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("POST %s: status %d: %s", path, resp.StatusCode, string(b))
	}

	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

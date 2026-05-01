package github

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// TaskRequest contains the extracted data from a GitHub issue labeled event.
type TaskRequest struct {
	Repo        string   `json:"repo"`
	IssueNumber int      `json:"issueNumber"`
	Title       string   `json:"title"`
	Body        string   `json:"body"`
	Labels      []string `json:"labels"`
}

// CreateTaskFunc is a callback invoked when a valid agent-labeled issue event is received.
type CreateTaskFunc func(req TaskRequest) error

// Handler processes GitHub webhook events.
type Handler struct {
	secret         []byte
	createTaskFunc CreateTaskFunc
}

// NewHandler creates a new GitHub webhook handler.
// secret is the HMAC-SHA256 shared secret for signature validation.
// createTaskFunc is called when a valid agent-labeled issue event is received.
func NewHandler(secret []byte, createTaskFunc CreateTaskFunc) *Handler {
	return &Handler{
		secret:         secret,
		createTaskFunc: createTaskFunc,
	}
}

// issueLabel represents a single label from a GitHub issue.
type issueLabel struct {
	Name string `json:"name"`
}

// issueEvent represents the relevant fields of a GitHub issues webhook payload.
type issueEvent struct {
	Action string `json:"action"`
	Label  struct {
		Name string `json:"name"`
	} `json:"label"`
	Issue struct {
		Number int          `json:"number"`
		Title  string       `json:"title"`
		Body   string       `json:"body"`
		Labels []issueLabel `json:"labels"`
	} `json:"issue"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
}

// ServeHTTP handles incoming GitHub webhook requests.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}
	defer func() { _ = r.Body.Close() }()

	// Validate signature
	signature := r.Header.Get("X-Hub-Signature-256")
	if !h.validSignature(body, signature) {
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	// Only handle issue events
	eventType := r.Header.Get("X-GitHub-Event")
	if eventType != "issues" {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "ignored event")
		return
	}

	var event issueEvent
	if err := json.Unmarshal(body, &event); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	// Only handle labeled action with "agent" label
	if event.Action != "labeled" || event.Label.Name != "agent" {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "ignored event")
		return
	}

	labels := make([]string, 0, len(event.Issue.Labels))
	for _, l := range event.Issue.Labels {
		labels = append(labels, l.Name)
	}

	taskReq := TaskRequest{
		Repo:        event.Repository.FullName,
		IssueNumber: event.Issue.Number,
		Title:       event.Issue.Title,
		Body:        event.Issue.Body,
		Labels:      labels,
	}

	if err := h.createTaskFunc(taskReq); err != nil {
		http.Error(w, "failed to create task", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	_, _ = fmt.Fprint(w, "task created")
}

// validSignature checks the HMAC-SHA256 signature of the payload.
func (h *Handler) validSignature(payload []byte, signature string) bool {
	if len(signature) < 7 || signature[:7] != "sha256=" {
		return false
	}

	sig, err := hex.DecodeString(signature[7:])
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, h.secret)
	mac.Write(payload)
	expected := mac.Sum(nil)

	return hmac.Equal(sig, expected)
}

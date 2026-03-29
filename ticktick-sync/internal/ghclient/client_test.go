package ghclient

import (
	"context"
	"testing"
)

// MockIssueService implements IssueService for testing.
type MockIssueService struct {
	Issues       []Issue
	CreatedIssue *Issue
	ClosedRepo   string
	ClosedNumber int
}

func (m *MockIssueService) ListByLabel(ctx context.Context, owner, repo, label string) ([]Issue, error) {
	return m.Issues, nil
}

func (m *MockIssueService) Create(ctx context.Context, owner, repo string, req CreateIssueRequest) (*Issue, error) {
	issue := &Issue{
		Number: 99,
		Title:  req.Title,
		Body:   req.Body,
		State:  "open",
		Labels: req.Labels,
	}
	m.CreatedIssue = issue
	return issue, nil
}

func (m *MockIssueService) Close(ctx context.Context, owner, repo string, number int) error {
	m.ClosedRepo = owner + "/" + repo
	m.ClosedNumber = number
	return nil
}

func (m *MockIssueService) UpdateBody(ctx context.Context, owner, repo string, number int, body string) error {
	return nil
}

func TestClientListAgentIssues(t *testing.T) {
	mock := &MockIssueService{
		Issues: []Issue{
			{Number: 1, Title: "Agent issue", State: "open", Labels: []string{"agent"}},
			{Number: 2, Title: "Synced issue", State: "open", Labels: []string{"agent", "ticktick-sync"}},
		},
	}
	c := &Client{issues: mock}

	got, err := c.ListAgentIssues(context.Background(), "Diixtra", "aios")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d issues, want 2", len(got))
	}
}

func TestClientCreateAgentIssue(t *testing.T) {
	mock := &MockIssueService{}
	c := &Client{issues: mock}

	issue, err := c.CreateAgentIssue(context.Background(), "Diixtra", "aios", "Fix bug", "Description\n\n<!-- ticktick:p1/t1 -->")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if issue.Number != 99 {
		t.Errorf("number = %d, want 99", issue.Number)
	}
	if mock.CreatedIssue == nil {
		t.Fatal("issue not created")
	}
	labels := mock.CreatedIssue.Labels
	hasAgent, hasSync := false, false
	for _, l := range labels {
		if l == "agent" {
			hasAgent = true
		}
		if l == "ticktick-sync" {
			hasSync = true
		}
	}
	if !hasAgent || !hasSync {
		t.Errorf("labels = %v, want agent + ticktick-sync", labels)
	}
}

func TestClientCloseIssue(t *testing.T) {
	mock := &MockIssueService{}
	c := &Client{issues: mock}

	err := c.CloseIssue(context.Background(), "Diixtra", "aios", 42)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if mock.ClosedRepo != "Diixtra/aios" || mock.ClosedNumber != 42 {
		t.Errorf("closed %s#%d, want Diixtra/aios#42", mock.ClosedRepo, mock.ClosedNumber)
	}
}

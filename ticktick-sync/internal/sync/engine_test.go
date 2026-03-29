package sync

import (
	"context"
	"testing"

	"github.com/Diixtra/aios/ticktick-sync/internal/ghclient"
	"github.com/Diixtra/aios/ticktick-sync/internal/state"
	"github.com/Diixtra/aios/ticktick-sync/internal/ticktick"
)

// --- Mocks ---

type mockTickTick struct {
	tasks        []ticktick.Task
	created      []ticktick.CreateTaskRequest
	completedIDs []string
}

func (m *mockTickTick) ListAgentTasks(ctx context.Context, projectID string) ([]ticktick.Task, error) {
	return m.tasks, nil
}

func (m *mockTickTick) CreateTask(ctx context.Context, req ticktick.CreateTaskRequest) (*ticktick.Task, error) {
	m.created = append(m.created, req)
	return &ticktick.Task{ID: "new-tt-id", ProjectID: req.ProjectID, Title: req.Title, Content: req.Content}, nil
}

func (m *mockTickTick) CompleteTask(ctx context.Context, projectID, taskID string) error {
	m.completedIDs = append(m.completedIDs, taskID)
	return nil
}

type mockGitHub struct {
	created       []ghclient.Issue
	closedIssues  []int
	updatedBodies map[int]string
}

func newMockGitHub() *mockGitHub {
	return &mockGitHub{
		updatedBodies: make(map[int]string),
	}
}

func (m *mockGitHub) CreateAgentIssue(ctx context.Context, owner, repo, title, body string) (*ghclient.Issue, error) {
	issue := ghclient.Issue{Number: 100 + len(m.created), Title: title, Body: body, State: "open", Labels: []string{"agent", "ticktick-sync"}}
	m.created = append(m.created, issue)
	return &issue, nil
}

func (m *mockGitHub) CloseIssue(ctx context.Context, owner, repo string, number int) error {
	m.closedIssues = append(m.closedIssues, number)
	return nil
}

func (m *mockGitHub) UpdateIssueBody(ctx context.Context, owner, repo string, number int, body string) error {
	m.updatedBodies[number] = body
	return nil
}

// --- Poll Tests (TickTick → GitHub) ---

func TestPollTickTick_NewTask(t *testing.T) {
	tt := &mockTickTick{
		tasks: []ticktick.Task{
			{ID: "t1", ProjectID: "p1", Title: "Fix auth bug", Content: "Details here", Tags: []string{"agent"}, Status: 0},
		},
	}
	gh := newMockGitHub()
	store := state.NewMemoryStore()

	e := NewEngine(tt, gh, store, "p1", []string{"Diixtra/aios"})
	err := e.PollTickTick(context.Background())
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if len(gh.created) != 1 {
		t.Fatalf("created %d issues, want 1", len(gh.created))
	}
	if gh.created[0].Title != "Fix auth bug" {
		t.Errorf("title = %q, want %q", gh.created[0].Title, "Fix auth bug")
	}

	m := store.FindByTickTick(context.Background(), "p1", "t1")
	if m == nil {
		t.Fatal("mapping not stored")
	}
}

func TestPollTickTick_AlreadySynced(t *testing.T) {
	tt := &mockTickTick{
		tasks: []ticktick.Task{
			{ID: "t1", ProjectID: "p1", Title: "Fix auth bug",
				Content: "Details\n\n<!-- github:Diixtra/aios#42 -->",
				Tags: []string{"agent"}, Status: 0},
		},
	}
	gh := newMockGitHub()
	store := state.NewMemoryStore()
	store.AddMapping(context.Background(), state.Mapping{
		TickTickProjectID: "p1", TickTickTaskID: "t1",
		GitHubRepo: "Diixtra/aios", GitHubIssueNumber: 42,
	})

	e := NewEngine(tt, gh, store, "p1", []string{"Diixtra/aios"})
	err := e.PollTickTick(context.Background())
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if len(gh.created) != 0 {
		t.Errorf("created %d issues, want 0 (already synced)", len(gh.created))
	}
}

func TestPollTickTick_CompletedClosesIssue(t *testing.T) {
	tt := &mockTickTick{
		tasks: []ticktick.Task{
			{ID: "t1", ProjectID: "p1", Title: "Done task", Tags: []string{"agent"}, Status: 2},
		},
	}
	gh := newMockGitHub()
	store := state.NewMemoryStore()
	store.AddMapping(context.Background(), state.Mapping{
		TickTickProjectID: "p1", TickTickTaskID: "t1",
		GitHubRepo: "Diixtra/aios", GitHubIssueNumber: 42,
	})

	e := NewEngine(tt, gh, store, "p1", []string{"Diixtra/aios"})
	err := e.PollTickTick(context.Background())
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if len(gh.closedIssues) != 1 || gh.closedIssues[0] != 42 {
		t.Errorf("closed = %v, want [42]", gh.closedIssues)
	}
}

// --- Event Tests (GitHub → TickTick via webhook) ---

func TestHandleIssueClosed_CompletesTask(t *testing.T) {
	tt := &mockTickTick{}
	gh := newMockGitHub()
	store := state.NewMemoryStore()
	store.AddMapping(context.Background(), state.Mapping{
		TickTickProjectID: "p1", TickTickTaskID: "t1",
		GitHubRepo: "Diixtra/aios", GitHubIssueNumber: 42,
	})

	e := NewEngine(tt, gh, store, "p1", []string{"Diixtra/aios"})
	err := e.HandleIssueClosed(context.Background(), "Diixtra/aios", 42)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if len(tt.completedIDs) != 1 || tt.completedIDs[0] != "t1" {
		t.Errorf("completed = %v, want [t1]", tt.completedIDs)
	}
}

func TestHandleIssueClosed_NoMapping(t *testing.T) {
	tt := &mockTickTick{}
	gh := newMockGitHub()
	store := state.NewMemoryStore()

	e := NewEngine(tt, gh, store, "p1", []string{"Diixtra/aios"})
	err := e.HandleIssueClosed(context.Background(), "Diixtra/aios", 999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tt.completedIDs) != 0 {
		t.Errorf("completed = %v, want empty", tt.completedIDs)
	}
}

func TestHandleIssueLabeled_CreatesTask(t *testing.T) {
	tt := &mockTickTick{}
	gh := newMockGitHub()
	store := state.NewMemoryStore()

	e := NewEngine(tt, gh, store, "p1", []string{"Diixtra/aios"})
	err := e.HandleIssueLabeled(context.Background(), "Diixtra/aios", 10, "New feature", "Build it")
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if len(tt.created) != 1 {
		t.Fatalf("created %d tasks, want 1", len(tt.created))
	}
	if tt.created[0].Title != "New feature" {
		t.Errorf("title = %q, want %q", tt.created[0].Title, "New feature")
	}

	m := store.FindByGitHub(context.Background(), "Diixtra/aios", 10)
	if m == nil {
		t.Fatal("mapping not stored")
	}
}

func TestHandleIssueLabeled_SkipsAlreadySynced(t *testing.T) {
	tt := &mockTickTick{}
	gh := newMockGitHub()
	store := state.NewMemoryStore()
	store.AddMapping(context.Background(), state.Mapping{
		TickTickProjectID: "p1", TickTickTaskID: "t1",
		GitHubRepo: "Diixtra/aios", GitHubIssueNumber: 10,
	})

	e := NewEngine(tt, gh, store, "p1", []string{"Diixtra/aios"})
	err := e.HandleIssueLabeled(context.Background(), "Diixtra/aios", 10, "Already synced", "body")
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if len(tt.created) != 0 {
		t.Errorf("created %d tasks, want 0", len(tt.created))
	}
}

func TestHandleIssueLabeled_SkipsTickTickMarker(t *testing.T) {
	tt := &mockTickTick{}
	gh := newMockGitHub()
	store := state.NewMemoryStore()

	e := NewEngine(tt, gh, store, "p1", []string{"Diixtra/aios"})
	err := e.HandleIssueLabeled(context.Background(), "Diixtra/aios", 10, "From TickTick", "body\n\n<!-- ticktick:p1/t1 -->")
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if len(tt.created) != 0 {
		t.Errorf("created %d tasks, want 0 (has ticktick marker)", len(tt.created))
	}
}

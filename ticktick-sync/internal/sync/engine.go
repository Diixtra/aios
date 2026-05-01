package sync

import (
	"context"
	"log/slog"
	"time"

	"github.com/Diixtra/aios/ticktick-sync/internal/ghclient"
	"github.com/Diixtra/aios/ticktick-sync/internal/state"
	"github.com/Diixtra/aios/ticktick-sync/internal/ticktick"
)

// TickTickClient defines the TickTick operations needed by the sync engine.
type TickTickClient interface {
	ListAgentTasks(ctx context.Context, projectID string) ([]ticktick.Task, error)
	CreateTask(ctx context.Context, req ticktick.CreateTaskRequest) (*ticktick.Task, error)
	CompleteTask(ctx context.Context, projectID, taskID string) error
}

// GitHubClient defines the GitHub operations needed by the sync engine.
type GitHubClient interface {
	CreateAgentIssue(ctx context.Context, owner, repo, title, body string) (*ghclient.Issue, error)
	CloseIssue(ctx context.Context, owner, repo string, number int) error
	UpdateIssueBody(ctx context.Context, owner, repo string, number int, body string) error
}

// Engine performs hybrid sync between TickTick and GitHub Issues.
// TickTick → GitHub is poll-based (PollTickTick).
// GitHub → TickTick is event-based (HandleIssueClosed, HandleIssueLabeled).
type Engine struct {
	tt        TickTickClient
	gh        GitHubClient
	store     state.Store
	projectID string
	repos     []string
}

// NewEngine creates a new sync engine.
func NewEngine(tt TickTickClient, gh GitHubClient, store state.Store, projectID string, repos []string) *Engine {
	return &Engine{
		tt:        tt,
		gh:        gh,
		store:     store,
		projectID: projectID,
		repos:     repos,
	}
}

// PollTickTick polls TickTick for agent tasks and syncs new ones to GitHub.
// Also detects completed tasks and closes corresponding GitHub issues.
func (e *Engine) PollTickTick(ctx context.Context) error {
	slog.Info("polling ticktick for agent tasks")

	tasks, err := e.tt.ListAgentTasks(ctx, e.projectID)
	if err != nil {
		return err
	}

	for _, task := range tasks {
		if m := e.store.FindByTickTick(ctx, task.ProjectID, task.ID); m != nil {
			if task.IsCompleted() && !m.Closed {
				owner, repo, err := ghclient.ParseRepo(m.GitHubRepo)
				if err != nil {
					slog.Error("invalid repo in mapping", "repo", m.GitHubRepo, "error", err)
					continue
				}
				slog.Info("closing github issue (ticktick task completed)",
					"repo", m.GitHubRepo, "issue", m.GitHubIssueNumber, "task", task.ID)
				if err := e.gh.CloseIssue(ctx, owner, repo, m.GitHubIssueNumber); err != nil {
					slog.Error("failed to close issue", "error", err)
				} else {
					m.Closed = true
					if err := e.store.AddMapping(ctx, *m); err != nil {
						slog.Error("add mapping failed", "error", err)
					}
				}
			}
			continue
		}

		if ghclient.ParseGitHubMarker(task.Content) != nil {
			continue
		}

		if task.IsCompleted() {
			continue
		}

		if len(e.repos) == 0 {
			continue
		}
		targetRepo := e.repos[0]
		owner, repo, err := ghclient.ParseRepo(targetRepo)
		if err != nil {
			slog.Error("invalid repo config", "repo", targetRepo, "error", err)
			continue
		}

		body := ghclient.AppendMarker(task.Content, ghclient.MakeTickTickMarker(task.ProjectID, task.ID))

		slog.Info("creating github issue from ticktick task",
			"task", task.ID, "title", task.Title, "repo", targetRepo)

		issue, err := e.gh.CreateAgentIssue(ctx, owner, repo, task.Title, body)
		if err != nil {
			slog.Error("failed to create issue", "error", err)
			continue
		}

		if err := e.store.AddMapping(ctx, state.Mapping{
			TickTickProjectID: task.ProjectID,
			TickTickTaskID:    task.ID,
			GitHubRepo:        targetRepo,
			GitHubIssueNumber: issue.Number,
			LastSyncedAt:      time.Now(),
		}); err != nil {
			slog.Error("add mapping failed", "error", err)
		}
	}

	e.store.SetLastTickTickPoll(ctx, time.Now())
	if err := e.store.Flush(ctx); err != nil {
		slog.Error("state flush failed", "error", err)
	}
	return nil
}

// HandleIssueClosed completes the corresponding TickTick task when a GitHub issue is closed.
func (e *Engine) HandleIssueClosed(ctx context.Context, repo string, issueNumber int) error {
	m := e.store.FindByGitHub(ctx, repo, issueNumber)
	if m == nil {
		slog.Debug("no mapping for closed issue", "repo", repo, "issue", issueNumber)
		return nil
	}

	slog.Info("completing ticktick task (github issue closed)",
		"task", m.TickTickTaskID, "issue", issueNumber, "repo", repo)

	if err := e.tt.CompleteTask(ctx, m.TickTickProjectID, m.TickTickTaskID); err != nil {
		return err
	}

	m.Closed = true
	if err := e.store.AddMapping(ctx, *m); err != nil {
		slog.Error("add mapping failed", "error", err)
	}

	if err := e.store.Flush(ctx); err != nil {
		slog.Error("state flush failed", "error", err)
	}
	return nil
}

// HandleIssueLabeled creates a TickTick task when a GitHub issue gets the "agent" label.
func (e *Engine) HandleIssueLabeled(ctx context.Context, repo string, issueNumber int, title, body string) error {
	if e.store.FindByGitHub(ctx, repo, issueNumber) != nil {
		return nil
	}

	if ghclient.ParseTickTickMarker(body) != nil {
		return nil
	}

	owner, repoName, err := ghclient.ParseRepo(repo)
	if err != nil {
		return err
	}

	ghMarker := ghclient.MakeGitHubMarker(repo, issueNumber)
	content := ghclient.AppendMarker(body, ghMarker)

	slog.Info("creating ticktick task from github issue",
		"issue", issueNumber, "title", title, "repo", repo)

	task, err := e.tt.CreateTask(ctx, ticktick.CreateTaskRequest{
		Title:     title,
		Content:   content,
		ProjectID: e.projectID,
		Tags:      []string{"agent"},
	})
	if err != nil {
		return err
	}

	ttMarker := ghclient.MakeTickTickMarker(e.projectID, task.ID)
	updatedBody := ghclient.AppendMarker(body, ttMarker)
	if err := e.gh.UpdateIssueBody(ctx, owner, repoName, issueNumber, updatedBody); err != nil {
		slog.Error("failed to update issue body with marker", "error", err)
	}

	if err := e.store.AddMapping(ctx, state.Mapping{
		TickTickProjectID: e.projectID,
		TickTickTaskID:    task.ID,
		GitHubRepo:        repo,
		GitHubIssueNumber: issueNumber,
		LastSyncedAt:      time.Now(),
	}); err != nil {
		slog.Error("add mapping failed", "error", err)
	}

	if err := e.store.Flush(ctx); err != nil {
		slog.Error("state flush failed", "error", err)
	}
	return nil
}

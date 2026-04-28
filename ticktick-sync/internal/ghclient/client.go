package ghclient

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/go-github/v85/github"
	"golang.org/x/oauth2"
)

// Issue is a simplified GitHub issue.
type Issue struct {
	Number int
	Title  string
	Body   string
	State  string
	Labels []string
}

// CreateIssueRequest holds parameters for creating an issue.
type CreateIssueRequest struct {
	Title  string
	Body   string
	Labels []string
}

// IssueService defines the GitHub issue operations needed by the sync engine.
type IssueService interface {
	ListByLabel(ctx context.Context, owner, repo, label string) ([]Issue, error)
	Create(ctx context.Context, owner, repo string, req CreateIssueRequest) (*Issue, error)
	Close(ctx context.Context, owner, repo string, number int) error
	UpdateBody(ctx context.Context, owner, repo string, number int, body string) error
}

// Client wraps the GitHub API for issue operations.
type Client struct {
	issues IssueService
}

// NewClient creates a Client backed by the real GitHub API.
func NewClient(token string) *Client {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(context.Background(), ts)
	gh := github.NewClient(tc)
	return &Client{issues: &realIssueService{gh: gh}}
}

// ListAgentIssues returns all issues with the "agent" label (open and closed).
func (c *Client) ListAgentIssues(ctx context.Context, owner, repo string) ([]Issue, error) {
	return c.issues.ListByLabel(ctx, owner, repo, AgentLabel)
}

// CreateAgentIssue creates an issue with "agent" and "ticktick-sync" labels.
func (c *Client) CreateAgentIssue(ctx context.Context, owner, repo, title, body string) (*Issue, error) {
	return c.issues.Create(ctx, owner, repo, CreateIssueRequest{
		Title:  title,
		Body:   body,
		Labels: []string{AgentLabel, SyncLabel},
	})
}

// CloseIssue closes a GitHub issue.
func (c *Client) CloseIssue(ctx context.Context, owner, repo string, number int) error {
	return c.issues.Close(ctx, owner, repo, number)
}

// UpdateIssueBody updates the body of a GitHub issue.
func (c *Client) UpdateIssueBody(ctx context.Context, owner, repo string, number int, body string) error {
	return c.issues.UpdateBody(ctx, owner, repo, number, body)
}

// ParseRepo splits "owner/repo" into owner and repo.
func ParseRepo(fullName string) (owner, repo string, err error) {
	parts := strings.SplitN(fullName, "/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid repo format %q, expected owner/repo", fullName)
	}
	return parts[0], parts[1], nil
}

// realIssueService implements IssueService using go-github.
type realIssueService struct {
	gh *github.Client
}

func (s *realIssueService) ListByLabel(ctx context.Context, owner, repo, label string) ([]Issue, error) {
	opts := &github.IssueListByRepoOptions{
		Labels:      []string{label},
		State:       "all",
		ListOptions: github.ListOptions{PerPage: 100},
	}

	var result []Issue
	for {
		issues, resp, err := s.gh.Issues.ListByRepo(ctx, owner, repo, opts)
		if err != nil {
			return nil, fmt.Errorf("list issues: %w", err)
		}
		for _, issue := range issues {
			if issue.IsPullRequest() {
				continue
			}
			labels := make([]string, 0, len(issue.Labels))
			for _, l := range issue.Labels {
				labels = append(labels, l.GetName())
			}
			result = append(result, Issue{
				Number: issue.GetNumber(),
				Title:  issue.GetTitle(),
				Body:   issue.GetBody(),
				State:  issue.GetState(),
				Labels: labels,
			})
		}
		if resp.NextPage == 0 {
			break
		}
		opts.ListOptions.Page = resp.NextPage
	}
	return result, nil
}

func (s *realIssueService) Create(ctx context.Context, owner, repo string, req CreateIssueRequest) (*Issue, error) {
	ghReq := &github.IssueRequest{
		Title:  github.String(req.Title),
		Body:   github.String(req.Body),
		Labels: &req.Labels,
	}
	issue, _, err := s.gh.Issues.Create(ctx, owner, repo, ghReq)
	if err != nil {
		return nil, fmt.Errorf("create issue: %w", err)
	}
	labels := make([]string, 0, len(issue.Labels))
	for _, l := range issue.Labels {
		labels = append(labels, l.GetName())
	}
	return &Issue{
		Number: issue.GetNumber(),
		Title:  issue.GetTitle(),
		Body:   issue.GetBody(),
		State:  issue.GetState(),
		Labels: labels,
	}, nil
}

func (s *realIssueService) Close(ctx context.Context, owner, repo string, number int) error {
	_, _, err := s.gh.Issues.Edit(ctx, owner, repo, number, &github.IssueRequest{
		State: github.String("closed"),
	})
	if err != nil {
		return fmt.Errorf("close issue #%d: %w", number, err)
	}
	return nil
}

func (s *realIssueService) UpdateBody(ctx context.Context, owner, repo string, number int, body string) error {
	_, _, err := s.gh.Issues.Edit(ctx, owner, repo, number, &github.IssueRequest{
		Body: github.String(body),
	})
	if err != nil {
		return fmt.Errorf("update issue #%d body: %w", number, err)
	}
	return nil
}

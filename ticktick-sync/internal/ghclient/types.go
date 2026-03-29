package ghclient

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const (
	AgentLabel = "agent"
	SyncLabel  = "ticktick-sync"

	tickTickMarkerFmt = "<!-- ticktick:%s/%s -->"
	gitHubMarkerFmt   = "<!-- github:%s#%d -->"
)

var (
	tickTickMarkerRe = regexp.MustCompile(`<!-- ticktick:([^/]+)/([^ ]+) -->`)
	gitHubMarkerRe   = regexp.MustCompile(`<!-- github:([^#]+)#(\d+) -->`)
)

type TickTickRef struct {
	ProjectID string
	TaskID    string
}

type GitHubRef struct {
	Repo   string
	Number int
}

func MakeTickTickMarker(projectID, taskID string) string {
	return fmt.Sprintf(tickTickMarkerFmt, projectID, taskID)
}

func MakeGitHubMarker(repo string, number int) string {
	return fmt.Sprintf(gitHubMarkerFmt, repo, number)
}

func ParseTickTickMarker(text string) *TickTickRef {
	m := tickTickMarkerRe.FindStringSubmatch(text)
	if m == nil {
		return nil
	}
	return &TickTickRef{ProjectID: m[1], TaskID: m[2]}
}

func ParseGitHubMarker(text string) *GitHubRef {
	m := gitHubMarkerRe.FindStringSubmatch(text)
	if m == nil {
		return nil
	}
	num, err := strconv.Atoi(m[2])
	if err != nil {
		return nil
	}
	return &GitHubRef{Repo: m[1], Number: num}
}

func AppendMarker(text, marker string) string {
	if strings.Contains(text, marker) {
		return text
	}
	if text == "" {
		return marker
	}
	return text + "\n\n" + marker
}

package auth

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
)

// Refresher exercises pi non-interactively to refresh the token bundle.
//
// We do not reimplement OAuth2 in the broker. Instead, we pin pi's behaviour
// (documented in spike A6): a no-op invocation against the configured PI dir
// causes pi to refresh stale tokens in-place. We invoke pi with a trivial
// prompt that should consume zero (or near-zero) provider tokens.
type Refresher struct {
	piBinary string
	piDir    string
}

func NewRefresher(piBinary, piDir string) *Refresher {
	return &Refresher{piBinary: piBinary, piDir: piDir}
}

func (r *Refresher) Refresh(ctx context.Context) error {
	if r.piBinary == "" || r.piDir == "" {
		return errors.New("refresh: piBinary and piDir required")
	}
	cmd := exec.CommandContext(ctx, r.piBinary,
		"--mode", "json",
		"--no-extensions", "--no-skills",
		"--no-prompt-templates", "--no-context-files",
		"-p", "Reply with the single token PONG and nothing else.")
	cmd.Env = append(cmd.Environ(), "PI_CODING_AGENT_DIR="+r.piDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("pi refresh: %w (output: %s)", err, string(out))
	}
	return nil
}

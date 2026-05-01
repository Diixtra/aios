package auth

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
)

// Validator runs a cheap pi command against the configured PI dir to confirm
// the bundle parses and pi can read it. We use --list-models because it does
// not require provider connectivity (model list is bundled in pi).
//
// CAVEAT (Spike A4): --list-models does NOT verify the configured agent model
// will actually authenticate at inference time — provider routing depends on
// the model id form (e.g. "openai-codex/gpt-5.4" vs bare "gpt-5.4"). End-to-end
// auth verification happens lazily on the first agent inference call. The
// orchestrator (B8) will transition to Expired and DM the user if a real
// inference call returns an auth error.
type Validator struct {
	piBinary string
	piDir    string
}

func NewValidator(piBinary, piDir string) *Validator {
	return &Validator{piBinary: piBinary, piDir: piDir}
}

func (v *Validator) Validate(ctx context.Context) error {
	if v.piBinary == "" || v.piDir == "" {
		return errors.New("validate: piBinary and piDir required")
	}
	cmd := exec.CommandContext(ctx, v.piBinary, "--list-models")
	cmd.Env = append(cmd.Environ(), "PI_CODING_AGENT_DIR="+v.piDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("pi validate: %w (output: %s)", err, string(out))
	}
	return nil
}

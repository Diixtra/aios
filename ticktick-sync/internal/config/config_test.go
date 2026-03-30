package config

import (
	"os"
	"testing"
	"time"
)

func setEnv(t *testing.T, vars map[string]string) {
	t.Helper()
	for k, v := range vars {
		t.Setenv(k, v)
	}
}

func validEnv() map[string]string {
	return map[string]string{
		"TICKTICK_ACCESS_TOKEN":  "tt-token",
		"TICKTICK_PROJECT_ID":    "proj-123",
		"GITHUB_TOKEN":           "gh-token",
		"GITHUB_WEBHOOK_SECRET":  "secret",
		"GITHUB_REPOS":           "Diixtra/aios",
	}
}

func TestLoadSuccess(t *testing.T) {
	setEnv(t, validEnv())

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.TickTickAccessToken != "tt-token" {
		t.Errorf("TickTickAccessToken = %q", cfg.TickTickAccessToken)
	}
	if cfg.GitHubRepos[0] != "Diixtra/aios" {
		t.Errorf("GitHubRepos = %v", cfg.GitHubRepos)
	}
	if cfg.PollInterval != 2*time.Minute {
		t.Errorf("PollInterval = %v, want 2m", cfg.PollInterval)
	}
	if cfg.Namespace != "aios" {
		t.Errorf("Namespace = %q, want aios", cfg.Namespace)
	}
}

func TestLoadCustomPollInterval(t *testing.T) {
	env := validEnv()
	env["POLL_INTERVAL"] = "30s"
	setEnv(t, env)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.PollInterval != 30*time.Second {
		t.Errorf("PollInterval = %v, want 30s", cfg.PollInterval)
	}
}

func TestLoadInvalidPollInterval(t *testing.T) {
	env := validEnv()
	env["POLL_INTERVAL"] = "not-a-duration"
	setEnv(t, env)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid POLL_INTERVAL")
	}
}

func TestLoadCustomNamespace(t *testing.T) {
	env := validEnv()
	env["AIOS_NAMESPACE"] = "custom-ns"
	setEnv(t, env)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.Namespace != "custom-ns" {
		t.Errorf("Namespace = %q, want custom-ns", cfg.Namespace)
	}
}

func TestLoadMissingRequired(t *testing.T) {
	tests := []struct {
		name   string
		remove string
	}{
		{"missing GITHUB_REPOS", "GITHUB_REPOS"},
		{"missing TICKTICK_ACCESS_TOKEN", "TICKTICK_ACCESS_TOKEN"},
		{"missing GITHUB_TOKEN", "GITHUB_TOKEN"},
		{"missing TICKTICK_PROJECT_ID", "TICKTICK_PROJECT_ID"},
		{"missing GITHUB_WEBHOOK_SECRET", "GITHUB_WEBHOOK_SECRET"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := validEnv()
			delete(env, tt.remove)
			setEnv(t, env)
			// Ensure the removed var is unset
			os.Unsetenv(tt.remove)

			_, err := Load()
			if err == nil {
				t.Fatalf("expected error when %s is missing", tt.remove)
			}
		})
	}
}

func TestLoadMultipleRepos(t *testing.T) {
	env := validEnv()
	env["GITHUB_REPOS"] = "Diixtra/aios,Diixtra/kompare-ng"
	setEnv(t, env)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if len(cfg.GitHubRepos) != 2 {
		t.Errorf("got %d repos, want 2", len(cfg.GitHubRepos))
	}
}

func TestLoadOptionalFields(t *testing.T) {
	env := validEnv()
	env["TICKTICK_CLIENT_ID"] = "client-id"
	env["TICKTICK_CLIENT_SECRET"] = "client-secret"
	env["TICKTICK_REFRESH_TOKEN"] = "refresh-token"
	setEnv(t, env)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.TickTickClientID != "client-id" {
		t.Errorf("ClientID = %q", cfg.TickTickClientID)
	}
	if cfg.TickTickRefreshToken != "refresh-token" {
		t.Errorf("RefreshToken = %q", cfg.TickTickRefreshToken)
	}
}

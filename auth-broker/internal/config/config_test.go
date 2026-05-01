package config

import (
	"testing"
	"time"
)

func TestLoad_RequiresPiBinary(t *testing.T) {
	env := map[string]string{
		// missing AUTH_BROKER_PI_BINARY
		"AUTH_BROKER_PI_DIR":           "/tmp",
		"AUTH_BROKER_SLACK_TOKEN":      "x",
		"AUTH_BROKER_SLACK_DM_USER_ID": "U",
		"AUTH_BROKER_ADMIN_TOKEN":      "a",
	}
	if _, err := loadFromMap(env); err == nil {
		t.Fatal("expected error for missing AUTH_BROKER_PI_BINARY")
	}
}

func TestLoad_DefaultsApplied(t *testing.T) {
	env := map[string]string{
		"AUTH_BROKER_PI_BINARY":        "/usr/local/bin/pi",
		"AUTH_BROKER_PI_DIR":           "/pi-state",
		"AUTH_BROKER_SLACK_TOKEN":      "x",
		"AUTH_BROKER_SLACK_DM_USER_ID": "U",
		"AUTH_BROKER_ADMIN_TOKEN":      "a",
	}
	cfg, err := loadFromMap(env)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LeaseCap != 4 {
		t.Fatalf("LeaseCap=%d, want 4", cfg.LeaseCap)
	}
	if cfg.LeaseTTL != 30*time.Minute {
		t.Fatalf("LeaseTTL=%v, want 30m", cfg.LeaseTTL)
	}
	if cfg.RefreshInterval != 168*time.Hour {
		t.Fatalf("RefreshInterval=%v, want 168h", cfg.RefreshInterval)
	}
	if cfg.ListenAddr != ":8080" {
		t.Fatalf("ListenAddr=%q", cfg.ListenAddr)
	}
}

func TestLoad_OverridesParse(t *testing.T) {
	env := map[string]string{
		"AUTH_BROKER_PI_BINARY":        "/usr/local/bin/pi",
		"AUTH_BROKER_PI_DIR":           "/pi-state",
		"AUTH_BROKER_SLACK_TOKEN":      "x",
		"AUTH_BROKER_SLACK_DM_USER_ID": "U",
		"AUTH_BROKER_ADMIN_TOKEN":      "a",
		"AUTH_BROKER_LEASE_CAP":        "8",
		"AUTH_BROKER_LEASE_TTL":        "10m",
		"AUTH_BROKER_REFRESH_INTERVAL": "24h",
		"AUTH_BROKER_LISTEN_ADDR":      ":9090",
	}
	cfg, err := loadFromMap(env)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LeaseCap != 8 {
		t.Fatalf("LeaseCap=%d", cfg.LeaseCap)
	}
	if cfg.LeaseTTL != 10*time.Minute {
		t.Fatalf("LeaseTTL=%v", cfg.LeaseTTL)
	}
	if cfg.RefreshInterval != 24*time.Hour {
		t.Fatalf("RefreshInterval=%v", cfg.RefreshInterval)
	}
	if cfg.ListenAddr != ":9090" {
		t.Fatalf("ListenAddr=%q", cfg.ListenAddr)
	}
}

func TestLoad_RejectsInvalidDuration(t *testing.T) {
	env := map[string]string{
		"AUTH_BROKER_PI_BINARY":        "/usr/local/bin/pi",
		"AUTH_BROKER_PI_DIR":           "/pi-state",
		"AUTH_BROKER_SLACK_TOKEN":      "x",
		"AUTH_BROKER_SLACK_DM_USER_ID": "U",
		"AUTH_BROKER_ADMIN_TOKEN":      "a",
		"AUTH_BROKER_LEASE_TTL":        "not-a-duration",
	}
	if _, err := loadFromMap(env); err == nil {
		t.Fatal("expected error for invalid duration")
	}
}

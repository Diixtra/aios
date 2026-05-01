package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	PiBinary        string
	PiDir           string
	LeaseCap        int
	LeaseTTL        time.Duration
	RefreshInterval time.Duration
	SlackToken      string
	SlackDMUserID   string
	AdminToken      string
	ListenAddr      string
}

// Load reads the configuration from process environment variables.
func Load() (*Config, error) {
	env := map[string]string{}
	for _, k := range []string{
		"AUTH_BROKER_PI_BINARY",
		"AUTH_BROKER_PI_DIR",
		"AUTH_BROKER_LEASE_CAP",
		"AUTH_BROKER_LEASE_TTL",
		"AUTH_BROKER_REFRESH_INTERVAL",
		"AUTH_BROKER_SLACK_TOKEN",
		"AUTH_BROKER_SLACK_DM_USER_ID",
		"AUTH_BROKER_ADMIN_TOKEN",
		"AUTH_BROKER_LISTEN_ADDR",
	} {
		if v, ok := os.LookupEnv(k); ok {
			env[k] = v
		}
	}
	return loadFromMap(env)
}

// loadFromMap is the testable core: same logic as Load but reads from a map
// instead of os.Environ. Required keys must be non-empty; optional keys get
// defaults; durations parse via time.ParseDuration.
func loadFromMap(env map[string]string) (*Config, error) {
	must := func(key string) (string, error) {
		v := env[key]
		if v == "" {
			return "", fmt.Errorf("config: %s is required", key)
		}
		return v, nil
	}
	envInt := func(key string, def int) (int, error) {
		v, ok := env[key]
		if !ok || v == "" {
			return def, nil
		}
		n, err := strconv.Atoi(v)
		if err != nil {
			return 0, fmt.Errorf("config: %s: %w", key, err)
		}
		return n, nil
	}
	envDuration := func(key string, def time.Duration) (time.Duration, error) {
		v, ok := env[key]
		if !ok || v == "" {
			return def, nil
		}
		d, err := time.ParseDuration(v)
		if err != nil {
			return 0, fmt.Errorf("config: %s: %w", key, err)
		}
		return d, nil
	}
	envString := func(key, def string) string {
		v, ok := env[key]
		if !ok || v == "" {
			return def
		}
		return v
	}

	piBin, err := must("AUTH_BROKER_PI_BINARY")
	if err != nil {
		return nil, err
	}
	piDir, err := must("AUTH_BROKER_PI_DIR")
	if err != nil {
		return nil, err
	}
	slackTok, err := must("AUTH_BROKER_SLACK_TOKEN")
	if err != nil {
		return nil, err
	}
	slackUser, err := must("AUTH_BROKER_SLACK_DM_USER_ID")
	if err != nil {
		return nil, err
	}
	adminTok, err := must("AUTH_BROKER_ADMIN_TOKEN")
	if err != nil {
		return nil, err
	}
	cap, err := envInt("AUTH_BROKER_LEASE_CAP", 4)
	if err != nil {
		return nil, err
	}
	ttl, err := envDuration("AUTH_BROKER_LEASE_TTL", 30*time.Minute)
	if err != nil {
		return nil, err
	}
	refresh, err := envDuration("AUTH_BROKER_REFRESH_INTERVAL", 168*time.Hour)
	if err != nil {
		return nil, err
	}

	return &Config{
		PiBinary:        piBin,
		PiDir:           piDir,
		LeaseCap:        cap,
		LeaseTTL:        ttl,
		RefreshInterval: refresh,
		SlackToken:      slackTok,
		SlackDMUserID:   slackUser,
		AdminToken:      adminTok,
		ListenAddr:      envString("AUTH_BROKER_LISTEN_ADDR", ":8080"),
	}, nil
}

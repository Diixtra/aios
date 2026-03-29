package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type Config struct {
	TickTickClientID     string
	TickTickClientSecret string
	TickTickAccessToken  string
	TickTickRefreshToken string
	TickTickProjectID    string
	GitHubToken          string
	GitHubRepos          []string
	PollInterval         time.Duration
	Namespace            string
}

func Load() (*Config, error) {
	c := &Config{
		TickTickClientID:     os.Getenv("TICKTICK_CLIENT_ID"),
		TickTickClientSecret: os.Getenv("TICKTICK_CLIENT_SECRET"),
		TickTickAccessToken:  os.Getenv("TICKTICK_ACCESS_TOKEN"),
		TickTickRefreshToken: os.Getenv("TICKTICK_REFRESH_TOKEN"),
		TickTickProjectID:    os.Getenv("TICKTICK_PROJECT_ID"),
		GitHubToken:          os.Getenv("GITHUB_TOKEN"),
		Namespace:            os.Getenv("AIOS_NAMESPACE"),
	}

	if c.Namespace == "" {
		c.Namespace = "aios"
	}

	repos := os.Getenv("GITHUB_REPOS")
	if repos == "" {
		return nil, fmt.Errorf("GITHUB_REPOS is required")
	}
	c.GitHubRepos = strings.Split(repos, ",")

	interval := os.Getenv("POLL_INTERVAL")
	if interval == "" {
		c.PollInterval = 2 * time.Minute
	} else {
		d, err := time.ParseDuration(interval)
		if err != nil {
			return nil, fmt.Errorf("invalid POLL_INTERVAL: %w", err)
		}
		c.PollInterval = d
	}

	if c.TickTickAccessToken == "" {
		return nil, fmt.Errorf("TICKTICK_ACCESS_TOKEN is required")
	}
	if c.GitHubToken == "" {
		return nil, fmt.Errorf("GITHUB_TOKEN is required")
	}
	if c.TickTickProjectID == "" {
		return nil, fmt.Errorf("TICKTICK_PROJECT_ID is required")
	}

	return c, nil
}

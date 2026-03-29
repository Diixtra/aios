package state

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const configMapName = "ticktick-sync-state"

// Mapping links a TickTick task to a GitHub issue.
type Mapping struct {
	TickTickProjectID string    `json:"ticktickProjectId"`
	TickTickTaskID    string    `json:"ticktickTaskId"`
	GitHubRepo        string    `json:"githubRepo"`
	GitHubIssueNumber int       `json:"githubIssueNumber"`
	LastSyncedAt      time.Time `json:"lastSyncedAt"`
}

// Store persists sync state.
type Store interface {
	AddMapping(ctx context.Context, m Mapping) error
	FindByTickTick(ctx context.Context, projectID, taskID string) *Mapping
	FindByGitHub(ctx context.Context, repo string, issueNumber int) *Mapping
	AllMappings(ctx context.Context) []Mapping
	SetLastTickTickPoll(ctx context.Context, t time.Time)
	SetLastGitHubPoll(ctx context.Context, t time.Time)
	LastTickTickPoll(ctx context.Context) time.Time
	LastGitHubPoll(ctx context.Context) time.Time
	Flush(ctx context.Context) error
}

// MemoryStore is an in-memory Store (also used as base for ConfigMapStore).
type MemoryStore struct {
	mu               sync.RWMutex
	mappings         []Mapping
	lastTickTickPoll time.Time
	lastGitHubPoll   time.Time
}

// NewMemoryStore creates a new in-memory store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{}
}

func (s *MemoryStore) AddMapping(_ context.Context, m Mapping) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, existing := range s.mappings {
		if existing.TickTickTaskID == m.TickTickTaskID && existing.TickTickProjectID == m.TickTickProjectID {
			s.mappings[i] = m
			return nil
		}
	}
	s.mappings = append(s.mappings, m)
	return nil
}

func (s *MemoryStore) FindByTickTick(_ context.Context, projectID, taskID string) *Mapping {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, m := range s.mappings {
		if m.TickTickProjectID == projectID && m.TickTickTaskID == taskID {
			return &m
		}
	}
	return nil
}

func (s *MemoryStore) FindByGitHub(_ context.Context, repo string, issueNumber int) *Mapping {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, m := range s.mappings {
		if m.GitHubRepo == repo && m.GitHubIssueNumber == issueNumber {
			return &m
		}
	}
	return nil
}

func (s *MemoryStore) AllMappings(_ context.Context) []Mapping {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]Mapping, len(s.mappings))
	copy(out, s.mappings)
	return out
}

func (s *MemoryStore) SetLastTickTickPoll(_ context.Context, t time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastTickTickPoll = t
}

func (s *MemoryStore) SetLastGitHubPoll(_ context.Context, t time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastGitHubPoll = t
}

func (s *MemoryStore) LastTickTickPoll(_ context.Context) time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastTickTickPoll
}

func (s *MemoryStore) LastGitHubPoll(_ context.Context) time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastGitHubPoll
}

func (s *MemoryStore) Flush(_ context.Context) error {
	return nil
}

// ConfigMapStore persists state to a Kubernetes ConfigMap.
type ConfigMapStore struct {
	*MemoryStore
	client    kubernetes.Interface
	namespace string
}

// NewConfigMapStore creates a store backed by a K8s ConfigMap.
func NewConfigMapStore(client kubernetes.Interface, namespace string) *ConfigMapStore {
	return &ConfigMapStore{
		MemoryStore: NewMemoryStore(),
		client:      client,
		namespace:   namespace,
	}
}

// Load reads state from the ConfigMap into memory.
func (s *ConfigMapStore) Load(ctx context.Context) error {
	cm, err := s.client.CoreV1().ConfigMaps(s.namespace).Get(ctx, configMapName, metav1.GetOptions{})
	if err != nil {
		return nil
	}

	if data, ok := cm.Data["mappings"]; ok {
		var mappings []Mapping
		if err := json.Unmarshal([]byte(data), &mappings); err != nil {
			return fmt.Errorf("unmarshal mappings: %w", err)
		}
		s.mu.Lock()
		s.mappings = mappings
		s.mu.Unlock()
	}

	if ts, ok := cm.Data["lastTickTickPoll"]; ok {
		t, _ := time.Parse(time.RFC3339, ts)
		s.mu.Lock()
		s.lastTickTickPoll = t
		s.mu.Unlock()
	}
	if ts, ok := cm.Data["lastGitHubPoll"]; ok {
		t, _ := time.Parse(time.RFC3339, ts)
		s.mu.Lock()
		s.lastGitHubPoll = t
		s.mu.Unlock()
	}

	return nil
}

// Flush writes in-memory state to the ConfigMap.
func (s *ConfigMapStore) Flush(ctx context.Context) error {
	s.mu.RLock()
	mappingsJSON, err := json.Marshal(s.mappings)
	tickTickPoll := s.lastTickTickPoll
	gitHubPoll := s.lastGitHubPoll
	s.mu.RUnlock()

	if err != nil {
		return fmt.Errorf("marshal mappings: %w", err)
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: s.namespace,
		},
		Data: map[string]string{
			"mappings":         string(mappingsJSON),
			"lastTickTickPoll": tickTickPoll.Format(time.RFC3339),
			"lastGitHubPoll":   gitHubPoll.Format(time.RFC3339),
		},
	}

	_, err = s.client.CoreV1().ConfigMaps(s.namespace).Update(ctx, cm, metav1.UpdateOptions{})
	if err != nil {
		_, err = s.client.CoreV1().ConfigMaps(s.namespace).Create(ctx, cm, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("persist state: %w", err)
		}
	}
	return nil
}

package state

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestStoreAddAndGetMapping(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	m := Mapping{
		TickTickProjectID: "p1",
		TickTickTaskID:    "t1",
		GitHubRepo:        "Diixtra/aios",
		GitHubIssueNumber: 42,
		LastSyncedAt:      time.Now(),
	}

	if err := s.AddMapping(ctx, m); err != nil {
		t.Fatalf("AddMapping error: %v", err)
	}

	got := s.FindByTickTick(ctx, "p1", "t1")
	if got == nil {
		t.Fatal("FindByTickTick returned nil")
	}
	if got.GitHubIssueNumber != 42 {
		t.Errorf("issue number = %d, want 42", got.GitHubIssueNumber)
	}

	got2 := s.FindByGitHub(ctx, "Diixtra/aios", 42)
	if got2 == nil {
		t.Fatal("FindByGitHub returned nil")
	}
	if got2.TickTickTaskID != "t1" {
		t.Errorf("task ID = %q, want %q", got2.TickTickTaskID, "t1")
	}
}

func TestStoreNotFound(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	if s.FindByTickTick(ctx, "nope", "nope") != nil {
		t.Error("expected nil for unknown TickTick ref")
	}
	if s.FindByGitHub(ctx, "nope/nope", 999) != nil {
		t.Error("expected nil for unknown GitHub ref")
	}
}

func TestStoreDuplicateMapping(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	m := Mapping{
		TickTickProjectID: "p1",
		TickTickTaskID:    "t1",
		GitHubRepo:        "Diixtra/aios",
		GitHubIssueNumber: 42,
		LastSyncedAt:      time.Now(),
	}
	s.AddMapping(ctx, m)

	m.LastSyncedAt = time.Now().Add(time.Hour)
	s.AddMapping(ctx, m)

	all := s.AllMappings(ctx)
	if len(all) != 1 {
		t.Errorf("got %d mappings, want 1", len(all))
	}
}

func TestStorePollTimestamps(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	now := time.Now().Truncate(time.Second)
	s.SetLastTickTickPoll(ctx, now)
	s.SetLastGitHubPoll(ctx, now)

	if got := s.LastTickTickPoll(ctx); !got.Equal(now) {
		t.Errorf("TickTick poll = %v, want %v", got, now)
	}
	if got := s.LastGitHubPoll(ctx); !got.Equal(now) {
		t.Errorf("GitHub poll = %v, want %v", got, now)
	}
}

// --- ConfigMapStore tests ---

func TestConfigMapStoreFlushAndLoad(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()

	store := NewConfigMapStore(client, "aios")

	// Add a mapping and flush
	store.AddMapping(ctx, Mapping{
		TickTickProjectID: "p1",
		TickTickTaskID:    "t1",
		GitHubRepo:        "Diixtra/aios",
		GitHubIssueNumber: 42,
		LastSyncedAt:      time.Date(2026, 3, 28, 12, 0, 0, 0, time.UTC),
	})
	store.SetLastTickTickPoll(ctx, time.Date(2026, 3, 28, 12, 0, 0, 0, time.UTC))
	store.SetLastGitHubPoll(ctx, time.Date(2026, 3, 28, 12, 0, 0, 0, time.UTC))

	if err := store.Flush(ctx); err != nil {
		t.Fatalf("Flush error: %v", err)
	}

	// Verify ConfigMap was created
	cm, err := client.CoreV1().ConfigMaps("aios").Get(ctx, configMapName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("ConfigMap not created: %v", err)
	}
	if cm.Data["mappings"] == "" {
		t.Fatal("mappings data is empty")
	}

	// Load into a fresh store
	store2 := NewConfigMapStore(client, "aios")
	if err := store2.Load(ctx); err != nil {
		t.Fatalf("Load error: %v", err)
	}

	m := store2.FindByTickTick(ctx, "p1", "t1")
	if m == nil {
		t.Fatal("mapping not found after Load")
	}
	if m.GitHubIssueNumber != 42 {
		t.Errorf("issue number = %d, want 42", m.GitHubIssueNumber)
	}
}

func TestConfigMapStoreLoadNotFound(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()

	store := NewConfigMapStore(client, "aios")
	// Load with no existing ConfigMap — should succeed with empty state
	if err := store.Load(ctx); err != nil {
		t.Fatalf("Load error on missing ConfigMap: %v", err)
	}

	all := store.AllMappings(ctx)
	if len(all) != 0 {
		t.Errorf("expected 0 mappings, got %d", len(all))
	}
}

func TestConfigMapStoreFlushUpdate(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()

	// Pre-create a ConfigMap
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: "aios",
		},
		Data: map[string]string{
			"mappings": "[]",
		},
	}
	client.CoreV1().ConfigMaps("aios").Create(ctx, cm, metav1.CreateOptions{})

	store := NewConfigMapStore(client, "aios")
	store.AddMapping(ctx, Mapping{
		TickTickProjectID: "p1",
		TickTickTaskID:    "t1",
		GitHubRepo:        "Diixtra/aios",
		GitHubIssueNumber: 99,
	})

	// Flush should update existing ConfigMap
	if err := store.Flush(ctx); err != nil {
		t.Fatalf("Flush error: %v", err)
	}

	updated, _ := client.CoreV1().ConfigMaps("aios").Get(ctx, configMapName, metav1.GetOptions{})
	var mappings []Mapping
	json.Unmarshal([]byte(updated.Data["mappings"]), &mappings)
	if len(mappings) != 1 || mappings[0].GitHubIssueNumber != 99 {
		t.Errorf("unexpected mappings after update: %+v", mappings)
	}
}

func TestConfigMapStoreLoadTimestamps(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()

	ts := time.Date(2026, 3, 28, 15, 30, 0, 0, time.UTC)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: "aios",
		},
		Data: map[string]string{
			"mappings":         "[]",
			"lastTickTickPoll": ts.Format(time.RFC3339),
			"lastGitHubPoll":   ts.Format(time.RFC3339),
		},
	}
	client.CoreV1().ConfigMaps("aios").Create(ctx, cm, metav1.CreateOptions{})

	store := NewConfigMapStore(client, "aios")
	if err := store.Load(ctx); err != nil {
		t.Fatalf("Load error: %v", err)
	}

	if got := store.LastTickTickPoll(ctx); !got.Equal(ts) {
		t.Errorf("TickTick poll = %v, want %v", got, ts)
	}
	if got := store.LastGitHubPoll(ctx); !got.Equal(ts) {
		t.Errorf("GitHub poll = %v, want %v", got, ts)
	}
}

package k8s

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func TestAgentTypeFromLabels(t *testing.T) {
	tests := []struct {
		name     string
		labels   []string
		expected string
	}{
		{name: "no labels", labels: nil, expected: "coding"},
		{name: "agent label only", labels: []string{"agent"}, expected: "coding"},
		{name: "research label", labels: []string{"agent", "research"}, expected: "research"},
		{name: "agent-both label", labels: []string{"agent", "agent-both"}, expected: "both"},
		{name: "first match wins agent-both", labels: []string{"agent-both", "research"}, expected: "both"},
		{name: "first match wins research", labels: []string{"research", "agent-both"}, expected: "research"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AgentTypeFromLabels(tt.labels)
			if got != tt.expected {
				t.Errorf("AgentTypeFromLabels(%v) = %q, want %q", tt.labels, got, tt.expected)
			}
		})
	}
}

func TestCreateAgentTask(t *testing.T) {
	scheme := runtime.NewScheme()
	fakeDyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		map[schema.GroupVersionResource]string{
			AgentTaskGVR: "AgentTaskList",
		},
	)

	client := NewClientFromDynamic(fakeDyn, "aios")

	params := TaskParams{
		Repo:        "Diixtra/aios",
		IssueNumber: 42,
		Title:       "Implement feature X",
		Body:        "Please implement feature X with tests.",
		Labels:      []string{"agent"},
	}

	err := client.CreateAgentTask(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the CR was created.
	created, err := fakeDyn.Resource(AgentTaskGVR).Namespace("aios").Get(
		context.Background(), "diixtra-aios-42", metav1.GetOptions{},
	)
	if err != nil {
		t.Fatalf("expected CR to exist: %v", err)
	}

	spec, ok := created.Object["spec"].(map[string]interface{})
	if !ok {
		t.Fatal("spec is not a map")
	}

	if spec["agentType"] != "coding" {
		t.Errorf("expected agentType 'coding', got %v", spec["agentType"])
	}
	if spec["agentConfig"] != "claude-sonnet" {
		t.Errorf("expected agentConfig 'claude-sonnet', got %v", spec["agentConfig"])
	}
	if spec["toolPolicy"] != "default-sre" {
		t.Errorf("expected toolPolicy 'default-sre', got %v", spec["toolPolicy"])
	}

	source, ok := spec["source"].(map[string]interface{})
	if !ok {
		t.Fatal("source is not a map")
	}
	if source["type"] != "github-issue" {
		t.Errorf("expected source type 'github-issue', got %v", source["type"])
	}
	if source["repo"] != "Diixtra/aios" {
		t.Errorf("expected repo 'Diixtra/aios', got %v", source["repo"])
	}

	expectedPrompt := "Implement feature X\n\nPlease implement feature X with tests."
	if spec["prompt"] != expectedPrompt {
		t.Errorf("expected prompt %q, got %v", expectedPrompt, spec["prompt"])
	}
}

func TestCreateAgentTask_ResearchLabel(t *testing.T) {
	scheme := runtime.NewScheme()
	fakeDyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		map[schema.GroupVersionResource]string{
			AgentTaskGVR: "AgentTaskList",
		},
	)

	client := NewClientFromDynamic(fakeDyn, "aios")

	params := TaskParams{
		Repo:        "Diixtra/aios",
		IssueNumber: 7,
		Title:       "Research topic Y",
		Body:        "Deep dive into topic Y.",
		Labels:      []string{"agent", "research"},
	}

	err := client.CreateAgentTask(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	created, err := fakeDyn.Resource(AgentTaskGVR).Namespace("aios").Get(
		context.Background(), "diixtra-aios-7", metav1.GetOptions{},
	)
	if err != nil {
		t.Fatalf("expected CR to exist: %v", err)
	}

	spec := created.Object["spec"].(map[string]interface{})
	if spec["agentType"] != "research" {
		t.Errorf("expected agentType 'research', got %v", spec["agentType"])
	}
}

// Ensure unstructured types satisfy the runtime.Object interface for the fake client.
func init() {
	_ = &unstructured.Unstructured{}
}

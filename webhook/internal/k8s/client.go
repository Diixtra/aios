// Package k8s provides a Kubernetes client for creating AgentTask custom resources.
package k8s

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

// AgentTaskGVR is the GroupVersionResource for AgentTask CRs.
var AgentTaskGVR = schema.GroupVersionResource{
	Group:    "aios.kazie.co.uk",
	Version:  "v1alpha1",
	Resource: "agenttasks",
}

// Client wraps a dynamic Kubernetes client for creating AgentTask CRs.
type Client struct {
	dyn       dynamic.Interface
	namespace string
}

// NewClient creates a new K8s client using in-cluster config.
func NewClient(namespace string) (*Client, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get in-cluster config: %w", err)
	}

	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	return &Client{dyn: dyn, namespace: namespace}, nil
}

// NewClientFromDynamic creates a Client from an existing dynamic.Interface (for testing).
func NewClientFromDynamic(dyn dynamic.Interface, namespace string) *Client {
	return &Client{dyn: dyn, namespace: namespace}
}

// TaskParams holds parameters for creating an AgentTask CR.
type TaskParams struct {
	Repo        string
	IssueNumber int
	Title       string
	Body        string
	Labels      []string
}

// AgentTypeFromLabels determines the agentType based on issue labels.
// - "research" label -> "research"
// - "agent-both" label -> "both"
// - otherwise -> "coding"
func AgentTypeFromLabels(labels []string) string {
	for _, l := range labels {
		if l == "research" {
			return "research"
		}
		if l == "agent-both" {
			return "both"
		}
	}
	return "coding"
}

// CreateAgentTask creates an AgentTask CR in the configured namespace.
func (c *Client) CreateAgentTask(ctx context.Context, params TaskParams) error {
	agentType := AgentTypeFromLabels(params.Labels)

	// Build a safe CR name from the repo and issue number.
	repoSlug := strings.ReplaceAll(params.Repo, "/", "-")
	name := fmt.Sprintf("%s-%d", strings.ToLower(repoSlug), params.IssueNumber)

	prompt := params.Title + "\n\n" + params.Body

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "aios.kazie.co.uk/v1alpha1",
			"kind":       "AgentTask",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": c.namespace,
			},
			"spec": map[string]interface{}{
				"source": map[string]interface{}{
					"type":        "github-issue",
					"repo":        params.Repo,
					"issueNumber": int64(params.IssueNumber),
				},
				"prompt":      prompt,
				"agentType":   agentType,
				"agentConfig": "claude-sonnet",
				"toolPolicy":  "default-sre",
			},
		},
	}

	_, err := c.dyn.Resource(AgentTaskGVR).Namespace(c.namespace).Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create AgentTask %s: %w", name, err)
	}

	return nil
}

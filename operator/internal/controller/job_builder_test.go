/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	aiosv1alpha1 "github.com/Diixtra/aios/operator/api/v1alpha1"
)

func newTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = aiosv1alpha1.AddToScheme(s)
	return s
}

func newTestTask() *aiosv1alpha1.AgentTask {
	return &aiosv1alpha1.AgentTask{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "aios.kazie.co.uk/v1alpha1",
			Kind:       "AgentTask",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-task",
			Namespace: "default",
			UID:       "test-uid-123",
		},
		Spec: aiosv1alpha1.AgentTaskSpec{
			Source: aiosv1alpha1.TaskSource{
				Type:        "github-issue",
				Repo:        "org/repo",
				IssueNumber: 42,
			},
			Prompt:      "Fix the bug in main.go",
			AgentType:   "coding",
			ToolPolicy:  "default-policy",
			AgentConfig: "default-config",
		},
	}
}

func newTestConfig() *aiosv1alpha1.AgentConfig {
	return &aiosv1alpha1.AgentConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default-config",
			Namespace: "default",
		},
		Spec: aiosv1alpha1.AgentConfigSpec{
			Runtime: aiosv1alpha1.RuntimeConfig{
				Image:     "ghcr.io/diixtra/aios-agent:latest",
				Model:     "claude-sonnet-4-6",
				MaxTokens: 200000,
			},
			Auth: aiosv1alpha1.AuthConfig{
				ClaudeKeySecret: "claude-api-key",
				GithubAppSecret: "github-app-secret",
			},
			Slack: aiosv1alpha1.SlackConfig{
				Channel: "#aios",
			},
		},
	}
}

func newTestPolicy() *aiosv1alpha1.ToolPolicy {
	return &aiosv1alpha1.ToolPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default-policy",
			Namespace: "default",
		},
		Spec: aiosv1alpha1.ToolPolicySpec{
			Allowed: aiosv1alpha1.AllowedActions{
				Commands: []string{"git", "npm", "go"},
			},
		},
	}
}

func TestBuildJob_CodingJob(t *testing.T) {
	builder := &JobBuilder{Scheme: newTestScheme()}
	task := newTestTask()
	config := newTestConfig()
	policy := newTestPolicy()

	job, err := builder.BuildJob(task, config, policy, "coding")
	require.NoError(t, err)
	require.NotNil(t, job)

	// Verify job name and namespace
	assert.Equal(t, "test-task-coding", job.Name)
	assert.Equal(t, "default", job.Namespace)

	// Verify labels
	assert.Equal(t, "test-task", job.Labels["aios.kazie.co.uk/task"])
	assert.Equal(t, "coding", job.Labels["aios.kazie.co.uk/job-type"])

	// Verify init container exists for git clone
	require.Len(t, job.Spec.Template.Spec.InitContainers, 1)
	initContainer := job.Spec.Template.Spec.InitContainers[0]
	assert.Equal(t, "git-clone", initContainer.Name)
	assert.Contains(t, initContainer.Command, "git")

	// Verify workspace volume exists
	volumeNames := make([]string, 0)
	for _, v := range job.Spec.Template.Spec.Volumes {
		volumeNames = append(volumeNames, v.Name)
	}
	assert.Contains(t, volumeNames, "workspace")
	assert.Contains(t, volumeNames, "tool-policy")

	// Verify workspace volume mount on main container
	mountNames := make([]string, 0)
	for _, m := range job.Spec.Template.Spec.Containers[0].VolumeMounts {
		mountNames = append(mountNames, m.Name)
	}
	assert.Contains(t, mountNames, "workspace")
}

func TestBuildJob_ResearchJob(t *testing.T) {
	builder := &JobBuilder{Scheme: newTestScheme()}
	task := newTestTask()
	config := newTestConfig()
	policy := newTestPolicy()

	job, err := builder.BuildJob(task, config, policy, "research")
	require.NoError(t, err)
	require.NotNil(t, job)

	// Verify no init containers for research
	assert.Empty(t, job.Spec.Template.Spec.InitContainers)

	// Verify research-output volume exists
	volumeNames := make([]string, 0)
	for _, v := range job.Spec.Template.Spec.Volumes {
		volumeNames = append(volumeNames, v.Name)
	}
	assert.Contains(t, volumeNames, "research-output")
	assert.NotContains(t, volumeNames, "workspace")

	// Verify research-output volume mount
	mountNames := make([]string, 0)
	for _, m := range job.Spec.Template.Spec.Containers[0].VolumeMounts {
		mountNames = append(mountNames, m.Name)
	}
	assert.Contains(t, mountNames, "research-output")
}

func TestBuildJob_OwnerReference(t *testing.T) {
	builder := &JobBuilder{Scheme: newTestScheme()}
	task := newTestTask()
	config := newTestConfig()
	policy := newTestPolicy()

	job, err := builder.BuildJob(task, config, policy, "coding")
	require.NoError(t, err)

	// Verify owner reference is set
	require.Len(t, job.OwnerReferences, 1)
	ownerRef := job.OwnerReferences[0]
	assert.Equal(t, "AgentTask", ownerRef.Kind)
	assert.Equal(t, "test-task", ownerRef.Name)
	assert.Equal(t, task.UID, ownerRef.UID)
	assert.True(t, *ownerRef.Controller)
}

func TestBuildJob_SecurityContext(t *testing.T) {
	builder := &JobBuilder{Scheme: newTestScheme()}
	task := newTestTask()
	config := newTestConfig()
	policy := newTestPolicy()

	job, err := builder.BuildJob(task, config, policy, "coding")
	require.NoError(t, err)

	// Verify security context on main container
	sc := job.Spec.Template.Spec.Containers[0].SecurityContext
	require.NotNil(t, sc)
	assert.True(t, *sc.RunAsNonRoot)
	assert.Equal(t, int64(1000), *sc.RunAsUser)
	assert.True(t, *sc.ReadOnlyRootFilesystem)
	assert.False(t, *sc.AllowPrivilegeEscalation)
}

func TestBuildJob_EnvVars(t *testing.T) {
	builder := &JobBuilder{Scheme: newTestScheme()}
	task := newTestTask()
	config := newTestConfig()
	policy := newTestPolicy()

	job, err := builder.BuildJob(task, config, policy, "coding")
	require.NoError(t, err)

	envVars := job.Spec.Template.Spec.Containers[0].Env
	envMap := make(map[string]string)
	envSecretMap := make(map[string]string)
	for _, e := range envVars {
		if e.Value != "" {
			envMap[e.Name] = e.Value
		}
		if e.ValueFrom != nil && e.ValueFrom.SecretKeyRef != nil {
			envSecretMap[e.Name] = e.ValueFrom.SecretKeyRef.Name
		}
	}

	// Verify plain env vars
	assert.Equal(t, "claude-sonnet-4-6", envMap["CLAUDE_MODEL"])
	assert.Equal(t, "test-task", envMap["AGENT_TASK_NAME"])
	assert.Equal(t, "default", envMap["AGENT_TASK_NAMESPACE"])
	assert.Equal(t, "coding", envMap["AGENT_TYPE"])
	assert.Equal(t, "Fix the bug in main.go", envMap["TASK_PROMPT"])
	assert.Equal(t, "org/repo", envMap["GITHUB_REPO"])
	assert.Equal(t, "42", envMap["GITHUB_ISSUE_NUMBER"])

	// Verify secret env vars
	assert.Equal(t, "claude-api-key", envSecretMap["ANTHROPIC_API_KEY"])
	assert.Equal(t, "github-app-secret", envSecretMap["GITHUB_TOKEN"])
}

func TestBuildJob_InvalidJobType(t *testing.T) {
	builder := &JobBuilder{Scheme: newTestScheme()}
	task := newTestTask()
	config := newTestConfig()
	policy := newTestPolicy()

	_, err := builder.BuildJob(task, config, policy, "invalid")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown job type")
}

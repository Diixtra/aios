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
	"encoding/json"
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
			Timeout:     "30m",
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
			Memory: &aiosv1alpha1.MemoryConfig{
				MCPServerUrl: "http://memory:8080",
				SearchUrl:    "http://search:8080",
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
				Network: &aiosv1alpha1.NetworkPolicy{
					AllowedHosts: []string{"api.github.com", "registry.npmjs.org"},
				},
			},
		},
	}
}

func TestBuildJob_CodingJob(t *testing.T) {
	builder := &JobBuilder{Scheme: newTestScheme()}
	task := newTestTask()
	config := newTestConfig()
	policy := newTestPolicy()

	result, err := builder.BuildJob(task, config, policy, "coding", "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Job)
	require.NotNil(t, result.ConfigMap)

	job := result.Job

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
	// I1: Verify git clone uses authenticated URL without shell
	assert.Equal(t, "git", initContainer.Command[0])
	assert.Equal(t, "clone", initContainer.Command[1])
	assert.Contains(t, initContainer.Command[2], "x-access-token:$(GITHUB_TOKEN)@github.com")
	assert.Equal(t, "/workspace", initContainer.Command[3])
	// I1: Verify GITHUB_TOKEN is available in init container
	require.Len(t, initContainer.Env, 1)
	assert.Equal(t, "GITHUB_TOKEN", initContainer.Env[0].Name)

	// Verify workspace volume exists
	volumeNames := make([]string, 0)
	for _, v := range job.Spec.Template.Spec.Volumes {
		volumeNames = append(volumeNames, v.Name)
	}
	assert.Contains(t, volumeNames, "workspace")
	assert.Contains(t, volumeNames, "tool-policy")
	// I6: Verify /tmp volume exists
	assert.Contains(t, volumeNames, "tmp")

	// Verify workspace volume mount on main container
	mountNames := make([]string, 0)
	for _, m := range job.Spec.Template.Spec.Containers[0].VolumeMounts {
		mountNames = append(mountNames, m.Name)
	}
	assert.Contains(t, mountNames, "workspace")
	// I6: Verify /tmp mount
	assert.Contains(t, mountNames, "tmp")

	// C2: Verify ConfigMap is created with correct name and data
	assert.Equal(t, "test-task-coding-tool-policy", result.ConfigMap.Name)
	assert.Contains(t, result.ConfigMap.Data, "policy.json")
	// Verify ConfigMap has owner reference
	require.Len(t, result.ConfigMap.OwnerReferences, 1)
	assert.Equal(t, "AgentTask", result.ConfigMap.OwnerReferences[0].Kind)

	// C2: Verify tool-policy volume references the correct ConfigMap name
	var toolPolicyVolume *string
	for _, v := range job.Spec.Template.Spec.Volumes {
		if v.Name == "tool-policy" && v.ConfigMap != nil {
			toolPolicyVolume = &v.ConfigMap.Name
		}
	}
	require.NotNil(t, toolPolicyVolume)
	assert.Equal(t, "test-task-coding-tool-policy", *toolPolicyVolume)

	// C2: Verify tool-policy mount path matches runtime expectation
	for _, m := range job.Spec.Template.Spec.Containers[0].VolumeMounts {
		if m.Name == "tool-policy" {
			assert.Equal(t, "/etc/aios/toolpolicy", m.MountPath)
		}
	}

	// I2: Verify ActiveDeadlineSeconds is set (30m = 1800s)
	require.NotNil(t, job.Spec.ActiveDeadlineSeconds)
	assert.Equal(t, int64(1800), *job.Spec.ActiveDeadlineSeconds)

	// I9: Verify NetworkPolicy spec is populated
	require.NotNil(t, result.NetworkPolicy)
	assert.Equal(t, "test-task-coding-netpol", result.NetworkPolicy.Name)
	assert.False(t, result.NetworkPolicy.AllowAll)
	assert.Equal(t, []string{"api.github.com", "registry.npmjs.org"}, result.NetworkPolicy.AllowedHosts)
}

func TestBuildJob_ResearchJob(t *testing.T) {
	builder := &JobBuilder{Scheme: newTestScheme()}
	task := newTestTask()
	config := newTestConfig()
	policy := newTestPolicy()

	result, err := builder.BuildJob(task, config, policy, "research", "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Job)

	job := result.Job

	// Verify no init containers for research
	assert.Empty(t, job.Spec.Template.Spec.InitContainers)

	// Verify research-output volume exists
	volumeNames := make([]string, 0)
	for _, v := range job.Spec.Template.Spec.Volumes {
		volumeNames = append(volumeNames, v.Name)
	}
	assert.Contains(t, volumeNames, "research-output")
	assert.NotContains(t, volumeNames, "workspace")
	// I6: Verify /tmp volume
	assert.Contains(t, volumeNames, "tmp")

	// Verify research-output volume mount
	mountNames := make([]string, 0)
	for _, m := range job.Spec.Template.Spec.Containers[0].VolumeMounts {
		mountNames = append(mountNames, m.Name)
	}
	assert.Contains(t, mountNames, "research-output")

	// S8: No PVC for research-only task (not "both")
	assert.Nil(t, result.PVC)
}

func TestBuildJob_ResearchJobWithBothType_CreatesPVC(t *testing.T) {
	builder := &JobBuilder{Scheme: newTestScheme()}
	task := newTestTask()
	task.Spec.AgentType = "both"
	config := newTestConfig()
	policy := newTestPolicy()

	result, err := builder.BuildJob(task, config, policy, "research", "")
	require.NoError(t, err)

	// S8: PVC should be created for "both" type
	require.NotNil(t, result.PVC)
	assert.Equal(t, "test-task-research-output", result.PVC.Name)
	// Verify PVC has owner reference
	require.Len(t, result.PVC.OwnerReferences, 1)
	assert.Equal(t, "AgentTask", result.PVC.OwnerReferences[0].Kind)

	// Verify research-pvc volume is mounted at /workspace/output
	job := result.Job
	var hasPVCVolume, hasPVCMount bool
	for _, v := range job.Spec.Template.Spec.Volumes {
		if v.Name == "research-pvc" && v.PersistentVolumeClaim != nil {
			hasPVCVolume = true
			assert.Equal(t, "test-task-research-output", v.PersistentVolumeClaim.ClaimName)
		}
	}
	for _, m := range job.Spec.Template.Spec.Containers[0].VolumeMounts {
		if m.Name == "research-pvc" {
			hasPVCMount = true
			assert.Equal(t, "/workspace/output", m.MountPath)
		}
	}
	assert.True(t, hasPVCVolume, "PVC volume should exist")
	assert.True(t, hasPVCMount, "PVC mount should exist")
}

func TestBuildJob_CodingJobWithResearchPVC(t *testing.T) {
	builder := &JobBuilder{Scheme: newTestScheme()}
	task := newTestTask()
	task.Spec.AgentType = "both"
	config := newTestConfig()
	policy := newTestPolicy()

	// S8: Pass research PVC name
	result, err := builder.BuildJob(task, config, policy, "coding", "test-task-research-output")
	require.NoError(t, err)

	job := result.Job

	// Verify research volume is mounted read-only at /research
	var hasResearchVolume, hasResearchMount bool
	for _, v := range job.Spec.Template.Spec.Volumes {
		if v.Name == "research" && v.PersistentVolumeClaim != nil {
			hasResearchVolume = true
			assert.Equal(t, "test-task-research-output", v.PersistentVolumeClaim.ClaimName)
			assert.True(t, v.PersistentVolumeClaim.ReadOnly)
		}
	}
	for _, m := range job.Spec.Template.Spec.Containers[0].VolumeMounts {
		if m.Name == "research" {
			hasResearchMount = true
			assert.Equal(t, "/research", m.MountPath)
			assert.True(t, m.ReadOnly)
		}
	}
	assert.True(t, hasResearchVolume, "Research PVC volume should exist")
	assert.True(t, hasResearchMount, "Research PVC mount should exist")

	// S8: Verify AIOS_RESEARCH_AVAILABLE env var is set
	envMap := make(map[string]string)
	for _, e := range job.Spec.Template.Spec.Containers[0].Env {
		if e.Value != "" {
			envMap[e.Name] = e.Value
		}
	}
	assert.Equal(t, "true", envMap["AIOS_RESEARCH_AVAILABLE"])
}

func TestBuildJob_OwnerReference(t *testing.T) {
	builder := &JobBuilder{Scheme: newTestScheme()}
	task := newTestTask()
	config := newTestConfig()
	policy := newTestPolicy()

	result, err := builder.BuildJob(task, config, policy, "coding", "")
	require.NoError(t, err)

	job := result.Job

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

	result, err := builder.BuildJob(task, config, policy, "coding", "")
	require.NoError(t, err)

	job := result.Job

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

	result, err := builder.BuildJob(task, config, policy, "coding", "")
	require.NoError(t, err)

	job := result.Job
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

	// C3: Verify env vars match runtime config.ts expectations
	assert.Equal(t, "test-task", envMap["AIOS_TASK_ID"])
	assert.Equal(t, "code", envMap["AIOS_TASK_TYPE"])
	assert.Equal(t, "Fix the bug in main.go", envMap["AIOS_PROMPT"])
	assert.Equal(t, "org/repo", envMap["AIOS_REPO"])
	assert.Equal(t, "42", envMap["AIOS_ISSUE_NUMBER"])
	assert.Equal(t, "aios/test-task", envMap["AIOS_BRANCH"])
	assert.Equal(t, "#aios", envMap["AIOS_SLACK_CHANNEL"])
	assert.Equal(t, "http://memory:8080", envMap["AIOS_MEMORY_URL"])
	assert.Equal(t, "http://search:8080", envMap["AIOS_SEARCH_URL"])
	assert.Equal(t, "/workspace", envMap["AIOS_WORKSPACE"])
	assert.Equal(t, "claude-sonnet-4-6", envMap["CLAUDE_MODEL"])

	// Verify secret env vars
	assert.Equal(t, "claude-api-key", envSecretMap["ANTHROPIC_API_KEY"])
	assert.Equal(t, "github-app-secret", envSecretMap["GITHUB_TOKEN"])
}

func TestBuildJob_ResearchEnvVars(t *testing.T) {
	builder := &JobBuilder{Scheme: newTestScheme()}
	task := newTestTask()
	config := newTestConfig()
	policy := newTestPolicy()

	result, err := builder.BuildJob(task, config, policy, "research", "")
	require.NoError(t, err)

	job := result.Job
	envMap := make(map[string]string)
	for _, e := range job.Spec.Template.Spec.Containers[0].Env {
		if e.Value != "" {
			envMap[e.Name] = e.Value
		}
	}

	// C3: Research job should have AIOS_TASK_TYPE=research
	assert.Equal(t, "research", envMap["AIOS_TASK_TYPE"])
}

func TestBuildJob_ToolPolicyConfigMap_FlatShape(t *testing.T) {
	builder := &JobBuilder{Scheme: newTestScheme()}
	task := newTestTask()
	config := newTestConfig()
	policy := &aiosv1alpha1.ToolPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default-policy",
			Namespace: "default",
		},
		Spec: aiosv1alpha1.ToolPolicySpec{
			Allowed: aiosv1alpha1.AllowedActions{
				Commands: []string{"git", "npm"},
				FileAccess: &aiosv1alpha1.FileAccessPolicy{
					Writable: []string{"/workspace/**"},
					Readable: []string{"/etc/aios/**"},
				},
				Network: &aiosv1alpha1.NetworkPolicy{
					AllowedHosts: []string{"api.github.com"},
				},
			},
			Denied: &aiosv1alpha1.DeniedActions{
				Commands: []string{"rm -rf"},
			},
		},
	}

	result, err := builder.BuildJob(task, config, policy, "coding", "")
	require.NoError(t, err)

	// Parse the ConfigMap policy.json and verify flat shape
	var parsed runtimeToolPolicy
	err = json.Unmarshal([]byte(result.ConfigMap.Data["policy.json"]), &parsed)
	require.NoError(t, err)

	assert.Equal(t, []string{"git", "npm"}, parsed.AllowedCommands)
	assert.Equal(t, []string{"rm -rf"}, parsed.DeniedCommands)
	assert.Equal(t, []string{"/workspace/**"}, parsed.WritablePaths)
	assert.Equal(t, []string{"/etc/aios/**"}, parsed.ReadablePaths)
	assert.Equal(t, []string{"api.github.com"}, parsed.AllowedHosts)
}

func TestBuildJob_NoMemoryConfig_OmitsMemoryEnvVars(t *testing.T) {
	builder := &JobBuilder{Scheme: newTestScheme()}
	task := newTestTask()
	config := newTestConfig()
	config.Spec.Memory = nil // No memory configured
	policy := newTestPolicy()

	result, err := builder.BuildJob(task, config, policy, "coding", "")
	require.NoError(t, err)

	job := result.Job
	envNames := make(map[string]bool)
	for _, e := range job.Spec.Template.Spec.Containers[0].Env {
		envNames[e.Name] = true
	}

	// When Memory is nil, AIOS_MEMORY_URL and AIOS_SEARCH_URL should not be set
	assert.False(t, envNames["AIOS_MEMORY_URL"], "AIOS_MEMORY_URL should not be set when Memory is nil")
	assert.False(t, envNames["AIOS_SEARCH_URL"], "AIOS_SEARCH_URL should not be set when Memory is nil")
}

func TestBuildJob_InvalidJobType(t *testing.T) {
	builder := &JobBuilder{Scheme: newTestScheme()}
	task := newTestTask()
	config := newTestConfig()
	policy := newTestPolicy()

	_, err := builder.BuildJob(task, config, policy, "invalid", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown job type")
}

func TestBuildJob_NetworkPolicyAllowAll(t *testing.T) {
	builder := &JobBuilder{Scheme: newTestScheme()}
	task := newTestTask()
	config := newTestConfig()
	policy := &aiosv1alpha1.ToolPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "research-policy",
			Namespace: "default",
		},
		Spec: aiosv1alpha1.ToolPolicySpec{
			Allowed: aiosv1alpha1.AllowedActions{
				Commands: []string{"curl", "wget"},
				Network: &aiosv1alpha1.NetworkPolicy{
					AllowedHosts: []string{"*"},
				},
			},
		},
	}

	result, err := builder.BuildJob(task, config, policy, "research", "")
	require.NoError(t, err)

	// I9: Research with "*" should have AllowAll=true
	require.NotNil(t, result.NetworkPolicy)
	assert.True(t, result.NetworkPolicy.AllowAll)
}

func TestBuildJob_NoNetworkPolicy(t *testing.T) {
	builder := &JobBuilder{Scheme: newTestScheme()}
	task := newTestTask()
	config := newTestConfig()
	policy := &aiosv1alpha1.ToolPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "no-network-policy",
			Namespace: "default",
		},
		Spec: aiosv1alpha1.ToolPolicySpec{
			Allowed: aiosv1alpha1.AllowedActions{
				Commands: []string{"git"},
			},
		},
	}

	result, err := builder.BuildJob(task, config, policy, "coding", "")
	require.NoError(t, err)

	// No network policy when not specified
	assert.Nil(t, result.NetworkPolicy)
}

func TestBuildJob_TimeoutParsing(t *testing.T) {
	builder := &JobBuilder{Scheme: newTestScheme()}
	task := newTestTask()
	task.Spec.Timeout = "1h30m"
	config := newTestConfig()
	policy := newTestPolicy()

	result, err := builder.BuildJob(task, config, policy, "coding", "")
	require.NoError(t, err)

	// I2: 1h30m = 5400 seconds
	require.NotNil(t, result.Job.Spec.ActiveDeadlineSeconds)
	assert.Equal(t, int64(5400), *result.Job.Spec.ActiveDeadlineSeconds)
}

func TestBuildJob_NoTimeout(t *testing.T) {
	builder := &JobBuilder{Scheme: newTestScheme()}
	task := newTestTask()
	task.Spec.Timeout = ""
	config := newTestConfig()
	policy := newTestPolicy()

	result, err := builder.BuildJob(task, config, policy, "coding", "")
	require.NoError(t, err)

	// No ActiveDeadlineSeconds when timeout is empty
	assert.Nil(t, result.Job.Spec.ActiveDeadlineSeconds)
}

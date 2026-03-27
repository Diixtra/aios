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
	"fmt"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"

	aiosv1alpha1 "github.com/Diixtra/aios/operator/api/v1alpha1"
)

// runtimeToolPolicy is the flat JSON shape the runtime expects for tool policies.
type runtimeToolPolicy struct {
	AllowedCommands []string `json:"allowedCommands"`
	DeniedCommands  []string `json:"deniedCommands,omitempty"`
	WritablePaths   []string `json:"writablePaths,omitempty"`
	ReadablePaths   []string `json:"readablePaths,omitempty"`
	AllowedHosts    []string `json:"allowedHosts"`
}

// JobBuilder constructs Kubernetes Jobs from AgentTask CRs.
type JobBuilder struct {
	Scheme *runtime.Scheme
}

// BuildResult contains all resources created by the job builder.
type BuildResult struct {
	Job           *batchv1.Job
	ConfigMap     *corev1.ConfigMap
	NetworkPolicy *NetworkPolicySpec
	PVC           *corev1.PersistentVolumeClaim
}

// NetworkPolicySpec holds the data needed to create a K8s NetworkPolicy.
// We use a separate struct because networking.k8s.io types need their own import.
type NetworkPolicySpec struct {
	Name         string
	Namespace    string
	Labels       map[string]string
	AllowedHosts []string
	AllowAll     bool
}

// BuildJob creates a Kubernetes Job and associated resources for the given AgentTask.
// jobType must be "coding" or "research".
func (b *JobBuilder) BuildJob(
	task *aiosv1alpha1.AgentTask,
	config *aiosv1alpha1.AgentConfig,
	policy *aiosv1alpha1.ToolPolicy,
	jobType string,
	researchPVCName string,
) (*BuildResult, error) {
	jobName := fmt.Sprintf("%s-%s", task.Name, jobType)

	// C2: Serialize ToolPolicy to flat JSON matching runtime's expected shape
	flatPolicy := runtimeToolPolicy{
		AllowedCommands: policy.Spec.Allowed.Commands,
		AllowedHosts:    []string{},
	}
	if policy.Spec.Denied != nil {
		flatPolicy.DeniedCommands = policy.Spec.Denied.Commands
	}
	if policy.Spec.Allowed.FileAccess != nil {
		flatPolicy.WritablePaths = policy.Spec.Allowed.FileAccess.Writable
		flatPolicy.ReadablePaths = policy.Spec.Allowed.FileAccess.Readable
	}
	if policy.Spec.Allowed.Network != nil {
		flatPolicy.AllowedHosts = policy.Spec.Allowed.Network.AllowedHosts
	}
	policyJSON, err := json.Marshal(flatPolicy)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tool policy: %w", err)
	}

	// C2: Create the ConfigMap for tool policy
	configMapName := fmt.Sprintf("%s-%s-tool-policy", task.Name, jobType)
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: task.Namespace,
			Labels: map[string]string{
				"aios.kazie.co.uk/task":     task.Name,
				"aios.kazie.co.uk/job-type": jobType,
				"app.kubernetes.io/part-of": "aios",
			},
		},
		Data: map[string]string{
			"policy.json": string(policyJSON),
		},
	}
	// Set owner reference on ConfigMap so it gets garbage collected
	if err := ctrl.SetControllerReference(task, configMap, b.Scheme); err != nil {
		return nil, fmt.Errorf("failed to set owner reference on ConfigMap: %w", err)
	}

	labels := map[string]string{
		"aios.kazie.co.uk/task":     task.Name,
		"aios.kazie.co.uk/job-type": jobType,
		"app.kubernetes.io/part-of": "aios",
	}

	// C3: Build env vars matching runtime config.ts expectations:
	// Required: AIOS_TASK_ID, AIOS_TASK_TYPE, AIOS_PROMPT, AIOS_REPO,
	//           AIOS_BRANCH, AIOS_SLACK_CHANNEL, AIOS_MEMORY_URL, AIOS_SEARCH_URL
	// Optional: AIOS_ISSUE_NUMBER, AIOS_SLACK_THREAD_TS, AIOS_WORKSPACE
	taskType := jobType
	if taskType == "coding" {
		taskType = "code"
	}

	envVars := []corev1.EnvVar{
		{Name: "AIOS_TASK_ID", Value: task.Name},
		{Name: "AIOS_TASK_TYPE", Value: taskType},
		{Name: "AIOS_PROMPT", Value: task.Spec.Prompt},
		{Name: "AIOS_REPO", Value: task.Spec.Source.Repo},
		{Name: "AIOS_ISSUE_NUMBER", Value: fmt.Sprintf("%d", task.Spec.Source.IssueNumber)},
		{Name: "AIOS_BRANCH", Value: fmt.Sprintf("aios/%s", task.Name)},
		{Name: "AIOS_SLACK_CHANNEL", Value: config.Spec.Slack.Channel},
		{Name: "CLAUDE_MODEL", Value: config.Spec.Runtime.Model},
		{
			Name: "ANTHROPIC_API_KEY",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: config.Spec.Auth.ClaudeKeySecret,
					},
					Key: "api-key",
				},
			},
		},
		{
			Name: "GITHUB_TOKEN",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: config.Spec.Auth.GithubAppSecret,
					},
					Key: "token",
				},
			},
		},
	}

	// C3: Only inject AIOS_MEMORY_URL and AIOS_SEARCH_URL when Memory is configured.
	// The runtime treats these as optional — omitting them is safe for tasks that don't need memory.
	if config.Spec.Memory != nil {
		envVars = append(envVars,
			corev1.EnvVar{Name: "AIOS_MEMORY_URL", Value: config.Spec.Memory.MCPServerUrl},
			corev1.EnvVar{Name: "AIOS_SEARCH_URL", Value: config.Spec.Memory.SearchUrl},
		)
	}

	// Security context: non-root, UID 1000, read-only root filesystem
	securityContext := &corev1.SecurityContext{
		RunAsNonRoot:             ptr.To(true),
		RunAsUser:                ptr.To(int64(1000)),
		ReadOnlyRootFilesystem:   ptr.To(true),
		AllowPrivilegeEscalation: ptr.To(false),
	}

	// Volumes and volume mounts
	volumes := []corev1.Volume{
		{
			Name: "tool-policy",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: configMapName,
					},
				},
			},
		},
		// I6: Add /tmp volume for tools that need it with read-only root filesystem
		{
			Name: "tmp",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	}

	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "tool-policy",
			MountPath: "/etc/aios/toolpolicy",
			ReadOnly:  true,
		},
		// I6: Mount /tmp
		{
			Name:      "tmp",
			MountPath: "/tmp",
		},
	}

	// GITHUB_TOKEN env var for init containers
	githubTokenEnv := corev1.EnvVar{
		Name: "GITHUB_TOKEN",
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: config.Spec.Auth.GithubAppSecret,
				},
				Key: "token",
			},
		},
	}

	var initContainers []corev1.Container
	result := &BuildResult{}

	switch jobType {
	case "coding":
		// Add workspace volume and git-clone init container
		volumes = append(volumes, corev1.Volume{
			Name: "workspace",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "workspace",
			MountPath: "/workspace",
		})

		// C3: Set AIOS_WORKSPACE for coding jobs
		envVars = append(envVars, corev1.EnvVar{Name: "AIOS_WORKSPACE", Value: "/workspace"})

		// I1: Use authenticated git clone URL — no shell to prevent injection via Repo field
		cloneURL := fmt.Sprintf("https://x-access-token:$(GITHUB_TOKEN)@github.com/%s.git",
			strings.ReplaceAll(strings.ReplaceAll(task.Spec.Source.Repo, "'", ""), "\"", ""))
		initContainers = append(initContainers, corev1.Container{
			Name:    "git-clone",
			Image:   "alpine/git:latest",
			Command: []string{"git", "clone", cloneURL, "/workspace"},
			VolumeMounts: []corev1.VolumeMount{
				{Name: "workspace", MountPath: "/workspace"},
				{Name: "tmp", MountPath: "/tmp"},
			},
			Env:             []corev1.EnvVar{githubTokenEnv},
			SecurityContext: securityContext,
		})

		// S8: If research PVC exists, mount it read-only and set env var
		if researchPVCName != "" {
			volumes = append(volumes, corev1.Volume{
				Name: "research",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: researchPVCName,
						ReadOnly:  true,
					},
				},
			})
			volumeMounts = append(volumeMounts, corev1.VolumeMount{
				Name:      "research",
				MountPath: "/research",
				ReadOnly:  true,
			})
			envVars = append(envVars, corev1.EnvVar{Name: "AIOS_RESEARCH_AVAILABLE", Value: "true"})
		}

	case "research":
		// Add research-output volume
		volumes = append(volumes, corev1.Volume{
			Name: "research-output",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "research-output",
			MountPath: "/research-output",
		})

		// S8: If this is a "both" task, create a PVC for sharing output with coding job
		if task.Spec.AgentType == "both" {
			pvcName := fmt.Sprintf("%s-research-output", task.Name)
			pvc := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pvcName,
					Namespace: task.Namespace,
					Labels:    labels,
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse("1Gi"),
						},
					},
				},
			}
			if err := ctrl.SetControllerReference(task, pvc, b.Scheme); err != nil {
				return nil, fmt.Errorf("failed to set owner reference on PVC: %w", err)
			}
			result.PVC = pvc

			// Mount the PVC at /workspace/output for the research job to write to
			volumes = append(volumes, corev1.Volume{
				Name: "research-pvc",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: pvcName,
					},
				},
			})
			volumeMounts = append(volumeMounts, corev1.VolumeMount{
				Name:      "research-pvc",
				MountPath: "/workspace/output",
			})
		}

	default:
		return nil, fmt.Errorf("unknown job type: %s", jobType)
	}

	// I2: Parse timeout and set ActiveDeadlineSeconds
	var activeDeadlineSeconds *int64
	if task.Spec.Timeout != "" {
		duration, err := time.ParseDuration(task.Spec.Timeout)
		if err != nil {
			return nil, fmt.Errorf("failed to parse timeout %q: %w", task.Spec.Timeout, err)
		}
		seconds := int64(duration.Seconds())
		if seconds > 0 {
			activeDeadlineSeconds = &seconds
		}
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: task.Namespace,
			Labels:    labels,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:          ptr.To(int32(2)),
			ActiveDeadlineSeconds: activeDeadlineSeconds,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					RestartPolicy:  corev1.RestartPolicyNever,
					InitContainers: initContainers,
					Containers: []corev1.Container{
						{
							Name:            "agent",
							Image:           config.Spec.Runtime.Image,
							Env:             envVars,
							VolumeMounts:    volumeMounts,
							Resources:       config.Spec.Resources,
							SecurityContext: securityContext,
						},
					},
					Volumes: volumes,
				},
			},
		},
	}

	// Set owner reference
	if err := ctrl.SetControllerReference(task, job, b.Scheme); err != nil {
		return nil, fmt.Errorf("failed to set owner reference: %w", err)
	}

	result.Job = job
	result.ConfigMap = configMap

	// I9: Build NetworkPolicy spec
	if policy.Spec.Allowed.Network != nil && len(policy.Spec.Allowed.Network.AllowedHosts) > 0 {
		allowAll := false
		for _, host := range policy.Spec.Allowed.Network.AllowedHosts {
			if host == "*" {
				allowAll = true
				break
			}
		}
		result.NetworkPolicy = &NetworkPolicySpec{
			Name:         fmt.Sprintf("%s-%s-netpol", task.Name, jobType),
			Namespace:    task.Namespace,
			Labels:       labels,
			AllowedHosts: policy.Spec.Allowed.Network.AllowedHosts,
			AllowAll:     allowAll,
		}
	}

	return result, nil
}

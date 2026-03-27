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

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"

	aiosv1alpha1 "github.com/Diixtra/aios/operator/api/v1alpha1"
)

// JobBuilder constructs Kubernetes Jobs from AgentTask CRs.
type JobBuilder struct {
	Scheme *runtime.Scheme
}

// BuildJob creates a Kubernetes Job for the given AgentTask.
// jobType must be "coding" or "research".
func (b *JobBuilder) BuildJob(
	task *aiosv1alpha1.AgentTask,
	config *aiosv1alpha1.AgentConfig,
	policy *aiosv1alpha1.ToolPolicy,
	jobType string,
) (*batchv1.Job, error) {
	jobName := fmt.Sprintf("%s-%s", task.Name, jobType)

	// Serialize ToolPolicy spec to JSON for ConfigMap-style mounting
	policyJSON, err := json.Marshal(policy.Spec)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tool policy: %w", err)
	}

	labels := map[string]string{
		"aios.kazie.co.uk/task":     task.Name,
		"aios.kazie.co.uk/job-type": jobType,
		"app.kubernetes.io/part-of": "aios",
	}

	// Build env vars
	envVars := []corev1.EnvVar{
		{Name: "CLAUDE_MODEL", Value: config.Spec.Runtime.Model},
		{Name: "AGENT_TASK_NAME", Value: task.Name},
		{Name: "AGENT_TASK_NAMESPACE", Value: task.Namespace},
		{Name: "AGENT_TYPE", Value: task.Spec.AgentType},
		{Name: "TASK_PROMPT", Value: task.Spec.Prompt},
		{Name: "GITHUB_REPO", Value: task.Spec.Source.Repo},
		{Name: "GITHUB_ISSUE_NUMBER", Value: fmt.Sprintf("%d", task.Spec.Source.IssueNumber)},
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
						Name: fmt.Sprintf("%s-tool-policy", task.Name),
					},
				},
			},
		},
	}

	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "tool-policy",
			MountPath: "/etc/aios/tool-policy",
			ReadOnly:  true,
		},
	}

	var initContainers []corev1.Container

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
		initContainers = append(initContainers, corev1.Container{
			Name:  "git-clone",
			Image: "alpine/git:latest",
			Command: []string{"git", "clone",
				fmt.Sprintf("https://github.com/%s.git", task.Spec.Source.Repo),
				"/workspace",
			},
			VolumeMounts: []corev1.VolumeMount{
				{Name: "workspace", MountPath: "/workspace"},
			},
			Env: []corev1.EnvVar{
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
			},
			SecurityContext: securityContext,
		})

	case "research":
		// Add research-output volume, no git clone
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

	default:
		return nil, fmt.Errorf("unknown job type: %s", jobType)
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: task.Namespace,
			Labels:    labels,
			Annotations: map[string]string{
				"aios.kazie.co.uk/tool-policy": string(policyJSON),
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: ptr.To(int32(2)),
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

	return job, nil
}

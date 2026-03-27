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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type AgentTaskSpec struct {
	Source TaskSource `json:"source"`
	Prompt string     `json:"prompt"`
	// +kubebuilder:validation:Enum=coding;research;both
	// +kubebuilder:default=coding
	AgentType  string `json:"agentType,omitempty"`
	ToolPolicy string `json:"toolPolicy"`
	// +optional
	ResearchToolPolicy string `json:"researchToolPolicy,omitempty"`
	AgentConfig        string `json:"agentConfig"`
	// +optional
	FabricPatterns *FabricPatterns `json:"fabricPatterns,omitempty"`
	// +kubebuilder:validation:Enum=normal;high;critical
	// +kubebuilder:default=normal
	Priority string `json:"priority,omitempty"`
	// +kubebuilder:default="30m"
	Timeout string `json:"timeout,omitempty"`
}

type TaskSource struct {
	// +kubebuilder:validation:Enum=github-issue;slack-command
	Type string `json:"type"`
	// +optional
	Repo string `json:"repo,omitempty"`
	// +optional
	IssueNumber int `json:"issueNumber,omitempty"`
}

type FabricPatterns struct {
	// +optional
	Understand []string `json:"understand,omitempty"`
	// +optional
	Verify []string `json:"verify,omitempty"`
	// +optional
	Deliver []string `json:"deliver,omitempty"`
}

type AgentTaskStatus struct {
	// +kubebuilder:validation:Enum=Pending;Running;Review;Completed;Failed
	Phase              string `json:"phase,omitempty"`
	PipelineStage      string `json:"pipelineStage,omitempty"`
	JobName            string `json:"jobName,omitempty"`
	ResearchJobName    string `json:"researchJobName,omitempty"`
	ResearchOutputPath string `json:"researchOutputPath,omitempty"`
	PRUrl              string `json:"prUrl,omitempty"`
	SlackThread        string `json:"slackThread,omitempty"`
	VerifyAttempts     int    `json:"verifyAttempts,omitempty"`
	// +optional
	StartedAt *metav1.Time `json:"startedAt,omitempty"`
	// +optional
	CompletedAt   *metav1.Time       `json:"completedAt,omitempty"`
	FailureReason string             `json:"failureReason,omitempty"`
	Conditions    []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Stage",type=string,JSONPath=`.status.pipelineStage`
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.agentType`
// +kubebuilder:printcolumn:name="Repo",type=string,JSONPath=`.spec.source.repo`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AgentTask is the Schema for the agenttasks API.
type AgentTask struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AgentTaskSpec   `json:"spec,omitempty"`
	Status AgentTaskStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AgentTaskList contains a list of AgentTask.
type AgentTaskList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AgentTask `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AgentTask{}, &AgentTaskList{})
}

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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type AgentConfigSpec struct {
	Runtime   RuntimeConfig               `json:"runtime"`
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
	Auth      AuthConfig                  `json:"auth"`
	Slack     SlackConfig                 `json:"slack"`
	// +optional
	Voice *VoiceConfig `json:"voice,omitempty"`
	// +optional
	Memory *MemoryConfig `json:"memory,omitempty"`
}

type RuntimeConfig struct {
	Image string `json:"image"`
	// +kubebuilder:default="claude-sonnet-4-6"
	Model string `json:"model,omitempty"`
	// +kubebuilder:default=200000
	MaxTokens int `json:"maxTokens,omitempty"`
}

type AuthConfig struct {
	ClaudeKeySecret string `json:"claudeKeySecret"`
	GithubAppSecret string `json:"githubAppSecret"`
}

type SlackConfig struct {
	Channel string `json:"channel"`
	// +optional
	EscalationChannel string `json:"escalationChannel,omitempty"`
}

type VoiceConfig struct {
	// +kubebuilder:default=true
	Enabled    bool   `json:"enabled,omitempty"`
	LocalAIUrl string `json:"localAiUrl"`
}

type MemoryConfig struct {
	MCPServerUrl string `json:"mcpServerUrl"`
	SearchUrl    string `json:"searchUrl"`
}

// +kubebuilder:object:root=true

// AgentConfig is the Schema for the agentconfigs API.
type AgentConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec AgentConfigSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// AgentConfigList contains a list of AgentConfig.
type AgentConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AgentConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AgentConfig{}, &AgentConfigList{})
}

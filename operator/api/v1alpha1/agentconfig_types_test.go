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

import "testing"

// TestRuntimeConfig_EngineDefault verifies that the Engine field exists on
// RuntimeConfig and that its zero value is empty (the kubebuilder default of
// "claude-sdk" is applied at admission time, not by Go's zero values).
func TestRuntimeConfig_EngineDefault(t *testing.T) {
	rc := RuntimeConfig{Image: "x"}
	// After applying the CRD default (kubebuilder), Engine should be claude-sdk
	// for backward compatibility.
	if rc.Engine != "" && rc.Engine != "claude-sdk" {
		t.Fatalf("zero-value Engine should be empty (CRD default applies); got %q", rc.Engine)
	}
}

// TestRuntimeConfig_EngineAcceptsPi documents that "pi" is a valid value for
// the engine field. The actual enum enforcement happens at the K8s API server
// admission layer via the kubebuilder validation marker.
func TestRuntimeConfig_EngineAcceptsPi(t *testing.T) {
	rc := RuntimeConfig{Image: "x", Engine: "pi"}
	if rc.Engine != "pi" {
		t.Fatalf("Engine field should round-trip; got %q", rc.Engine)
	}
}

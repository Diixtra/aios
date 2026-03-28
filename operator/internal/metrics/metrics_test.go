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

package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMetrics_RegistersAllMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)
	require.NotNil(t, m)

	// Verify all metrics are registered by gathering
	families, err := reg.Gather()
	require.NoError(t, err)

	// We need to trigger at least one observation per metric for them to appear
	// Counters and gauges only appear after first use
	m.TasksTotal.WithLabelValues("github-issue", "org/repo", "created").Inc()
	m.TaskDurationSeconds.WithLabelValues("org/repo", "coding").Observe(120)
	m.TaskRetriesTotal.WithLabelValues("org/repo", "coding").Inc()
	m.ClaudeTokensTotal.WithLabelValues("claude-sonnet-4-6", "input").Add(1000)
	m.ClaudeCostDollars.WithLabelValues("claude-sonnet-4-6", "org/repo").Add(0.05)
	m.CommandsExecutedTotal.WithLabelValues("git").Inc()
	m.CommandsDeniedTotal.WithLabelValues("rm", "policy").Inc()
	m.VoiceSessionsTotal.WithLabelValues("task-1").Inc()
	m.VoiceSessionDuration.Observe(30)
	m.VerifyPassRate.WithLabelValues("org/repo").Set(0.95)
	m.MemoryOperationsTotal.WithLabelValues("write").Inc()

	families, err = reg.Gather()
	require.NoError(t, err)

	metricNames := make([]string, 0, len(families))
	for _, f := range families {
		metricNames = append(metricNames, f.GetName())
	}

	expectedMetrics := []string{
		"aios_tasks_total",
		"aios_task_duration_seconds",
		"aios_task_retries_total",
		"aios_claude_tokens_total",
		"aios_claude_cost_dollars",
		"aios_commands_executed_total",
		"aios_commands_denied_total",
		"aios_voice_sessions_total",
		"aios_voice_session_duration_seconds",
		"aios_verify_pass_rate",
		"aios_memory_operations_total",
	}

	for _, expected := range expectedMetrics {
		assert.Contains(t, metricNames, expected, "missing metric: %s", expected)
	}
}

func TestRecordTaskCreated(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)

	m.RecordTaskCreated("github-issue", "org/repo")
	m.RecordTaskCreated("github-issue", "org/repo")
	m.RecordTaskCreated("slack-command", "org/other")

	val := testutil.ToFloat64(m.TasksTotal.WithLabelValues("github-issue", "org/repo", "created"))
	assert.Equal(t, float64(2), val)

	val = testutil.ToFloat64(m.TasksTotal.WithLabelValues("slack-command", "org/other", "created"))
	assert.Equal(t, float64(1), val)
}

func TestRecordCommandExecuted_Allowed(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)

	m.RecordCommandExecuted("git", true)
	m.RecordCommandExecuted("git", true)
	m.RecordCommandExecuted("npm", true)

	val := testutil.ToFloat64(m.CommandsExecutedTotal.WithLabelValues("git"))
	assert.Equal(t, float64(2), val)

	val = testutil.ToFloat64(m.CommandsExecutedTotal.WithLabelValues("npm"))
	assert.Equal(t, float64(1), val)
}

func TestRecordCommandExecuted_Denied(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)

	m.RecordCommandExecuted("rm", false)
	m.RecordCommandExecuted("rm", false)

	val := testutil.ToFloat64(m.CommandsDeniedTotal.WithLabelValues("rm", "policy"))
	assert.Equal(t, float64(2), val)

	// Executed counter should not be incremented
	val = testutil.ToFloat64(m.CommandsExecutedTotal.WithLabelValues("rm"))
	assert.Equal(t, float64(0), val)
}

func TestMetrics_DoublePanics(t *testing.T) {
	// Verify that creating metrics with the same registry panics (no accidental double-registration)
	reg := prometheus.NewRegistry()
	_ = NewMetrics(reg)

	assert.Panics(t, func() {
		_ = NewMetrics(reg)
	})
}

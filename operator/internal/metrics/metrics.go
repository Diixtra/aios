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
	"github.com/prometheus/client_golang/prometheus"
)

// Metrics holds all AIOS Prometheus metrics.
type Metrics struct {
	TasksTotal            *prometheus.CounterVec
	TaskDurationSeconds   *prometheus.HistogramVec
	TaskRetriesTotal      *prometheus.CounterVec
	ClaudeTokensTotal     *prometheus.CounterVec
	ClaudeCostDollars     *prometheus.CounterVec
	CommandsExecutedTotal *prometheus.CounterVec
	CommandsDeniedTotal   *prometheus.CounterVec
	VoiceSessionsTotal    *prometheus.CounterVec
	VoiceSessionDuration  prometheus.Histogram
	VerifyPassRate        *prometheus.GaugeVec
	MemoryOperationsTotal *prometheus.CounterVec
}

// NewMetrics creates and registers all AIOS metrics with the given registerer.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		TasksTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "aios_tasks_total",
				Help: "Total number of agent tasks created.",
			},
			[]string{"source", "repo", "status"},
		),
		TaskDurationSeconds: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "aios_task_duration_seconds",
				Help:    "Duration of agent task phases in seconds.",
				Buckets: prometheus.ExponentialBuckets(10, 2, 10),
			},
			[]string{"repo", "phase"},
		),
		TaskRetriesTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "aios_task_retries_total",
				Help: "Total number of task retries.",
			},
			[]string{"repo", "phase"},
		),
		ClaudeTokensTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "aios_claude_tokens_total",
				Help: "Total Claude API tokens consumed.",
			},
			[]string{"model", "direction"},
		),
		ClaudeCostDollars: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "aios_claude_cost_dollars",
				Help: "Total estimated Claude API cost in dollars.",
			},
			[]string{"model", "repo"},
		),
		CommandsExecutedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "aios_commands_executed_total",
				Help: "Total commands executed by agents.",
			},
			[]string{"command"},
		),
		CommandsDeniedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "aios_commands_denied_total",
				Help: "Total commands denied by tool policy.",
			},
			[]string{"command", "reason"},
		),
		VoiceSessionsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "aios_voice_sessions_total",
				Help: "Total voice sessions initiated.",
			},
			[]string{"task"},
		),
		VoiceSessionDuration: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Name:    "aios_voice_session_duration_seconds",
				Help:    "Duration of voice sessions in seconds.",
				Buckets: prometheus.ExponentialBuckets(5, 2, 8),
			},
		),
		VerifyPassRate: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "aios_verify_pass_rate",
				Help: "Verification pass rate per repository (0.0 to 1.0).",
			},
			[]string{"repo"},
		),
		MemoryOperationsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "aios_memory_operations_total",
				Help: "Total memory store operations.",
			},
			[]string{"operation"},
		),
	}

	reg.MustRegister(
		m.TasksTotal,
		m.TaskDurationSeconds,
		m.TaskRetriesTotal,
		m.ClaudeTokensTotal,
		m.ClaudeCostDollars,
		m.CommandsExecutedTotal,
		m.CommandsDeniedTotal,
		m.VoiceSessionsTotal,
		m.VoiceSessionDuration,
		m.VerifyPassRate,
		m.MemoryOperationsTotal,
	)

	return m
}

// RecordTaskCreated increments the tasks_total counter for a new task.
func (m *Metrics) RecordTaskCreated(source, repo string) {
	m.TasksTotal.WithLabelValues(source, repo, "created").Inc()
}

// RecordCommandExecuted increments the appropriate command counter.
// If allowed is true, increments commands_executed_total.
// If allowed is false, increments commands_denied_total with reason "policy".
func (m *Metrics) RecordCommandExecuted(command string, allowed bool) {
	if allowed {
		m.CommandsExecutedTotal.WithLabelValues(command).Inc()
	} else {
		m.CommandsDeniedTotal.WithLabelValues(command, "policy").Inc()
	}
}

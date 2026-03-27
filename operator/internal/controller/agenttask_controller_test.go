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
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	aiosv1alpha1 "github.com/Diixtra/aios/operator/api/v1alpha1"
)

func newTestReconcilerScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = aiosv1alpha1.AddToScheme(s)
	_ = clientgoscheme.AddToScheme(s)
	return s
}

func TestReconcile_PendingCreatesJobAndUpdatesToRunning(t *testing.T) {
	scheme := newTestReconcilerScheme()
	task := newTestTask()
	task.Status.Phase = "Pending"
	config := newTestConfig()
	policy := newTestPolicy()

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(task, config, policy).
		WithStatusSubresource(task).
		Build()

	reconciler := &AgentTaskReconciler{
		Client: client,
		Scheme: scheme,
	}

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      task.Name,
			Namespace: task.Namespace,
		},
	})
	require.NoError(t, err)
	assert.True(t, result.RequeueAfter > 0)

	// Verify Job was created
	var jobList batchv1.JobList
	err = client.List(context.Background(), &jobList)
	require.NoError(t, err)
	require.Len(t, jobList.Items, 1)
	assert.Equal(t, "test-task-coding", jobList.Items[0].Name)

	// Verify task status was updated to Running
	var updatedTask aiosv1alpha1.AgentTask
	err = client.Get(context.Background(), types.NamespacedName{
		Name:      task.Name,
		Namespace: task.Namespace,
	}, &updatedTask)
	require.NoError(t, err)
	assert.Equal(t, "Running", updatedTask.Status.Phase)
	assert.Equal(t, "coding", updatedTask.Status.PipelineStage)
	assert.NotNil(t, updatedTask.Status.StartedAt)
}

func TestReconcile_BothTypeCreatesResearchJobFirst(t *testing.T) {
	scheme := newTestReconcilerScheme()
	task := newTestTask()
	task.Spec.AgentType = "both"
	task.Status.Phase = "Pending"
	config := newTestConfig()
	policy := newTestPolicy()

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(task, config, policy).
		WithStatusSubresource(task).
		Build()

	reconciler := &AgentTaskReconciler{
		Client: client,
		Scheme: scheme,
	}

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      task.Name,
			Namespace: task.Namespace,
		},
	})
	require.NoError(t, err)
	assert.True(t, result.RequeueAfter > 0)

	// Verify research job was created
	var jobList batchv1.JobList
	err = client.List(context.Background(), &jobList)
	require.NoError(t, err)
	require.Len(t, jobList.Items, 1)
	assert.Equal(t, "test-task-research", jobList.Items[0].Name)

	// Verify status shows research pipeline stage
	var updatedTask aiosv1alpha1.AgentTask
	err = client.Get(context.Background(), types.NamespacedName{
		Name:      task.Name,
		Namespace: task.Namespace,
	}, &updatedTask)
	require.NoError(t, err)
	assert.Equal(t, "Running", updatedTask.Status.Phase)
	assert.Equal(t, "research", updatedTask.Status.PipelineStage)
	assert.Equal(t, "test-task-research", updatedTask.Status.ResearchJobName)
}

func TestReconcile_CompletedJobMarksTaskCompleted(t *testing.T) {
	scheme := newTestReconcilerScheme()
	task := newTestTask()
	task.Status.Phase = "Running"
	task.Status.PipelineStage = "coding"
	task.Status.JobName = "test-task-coding"
	now := metav1.Now()
	task.Status.StartedAt = &now

	config := newTestConfig()
	policy := newTestPolicy()

	// Create a completed Job
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-task-coding",
			Namespace: "default",
		},
		Status: batchv1.JobStatus{
			Conditions: []batchv1.JobCondition{
				{
					Type:   batchv1.JobComplete,
					Status: "True",
				},
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(task, config, policy, job).
		WithStatusSubresource(task).
		Build()

	reconciler := &AgentTaskReconciler{
		Client: client,
		Scheme: scheme,
	}

	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      task.Name,
			Namespace: task.Namespace,
		},
	})
	require.NoError(t, err)

	// Verify task was marked Completed
	var updatedTask aiosv1alpha1.AgentTask
	err = client.Get(context.Background(), types.NamespacedName{
		Name:      task.Name,
		Namespace: task.Namespace,
	}, &updatedTask)
	require.NoError(t, err)
	assert.Equal(t, "Completed", updatedTask.Status.Phase)
	assert.NotNil(t, updatedTask.Status.CompletedAt)
}

func TestReconcile_FailedJobMarksTaskFailed(t *testing.T) {
	scheme := newTestReconcilerScheme()
	task := newTestTask()
	task.Status.Phase = "Running"
	task.Status.PipelineStage = "coding"
	task.Status.JobName = "test-task-coding"

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-task-coding",
			Namespace: "default",
		},
		Status: batchv1.JobStatus{
			Conditions: []batchv1.JobCondition{
				{
					Type:   batchv1.JobFailed,
					Status: "True",
				},
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(task, job).
		WithStatusSubresource(task).
		Build()

	reconciler := &AgentTaskReconciler{
		Client: client,
		Scheme: scheme,
	}

	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      task.Name,
			Namespace: task.Namespace,
		},
	})
	require.NoError(t, err)

	var updatedTask aiosv1alpha1.AgentTask
	err = client.Get(context.Background(), types.NamespacedName{
		Name:      task.Name,
		Namespace: task.Namespace,
	}, &updatedTask)
	require.NoError(t, err)
	assert.Equal(t, "Failed", updatedTask.Status.Phase)
	assert.Contains(t, updatedTask.Status.FailureReason, "failed")
}

func TestReconcile_TerminalStatesAreNoOp(t *testing.T) {
	scheme := newTestReconcilerScheme()

	for _, phase := range []string{"Completed", "Failed"} {
		t.Run(phase, func(t *testing.T) {
			task := newTestTask()
			task.Status.Phase = phase

			client := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(task).
				WithStatusSubresource(task).
				Build()

			reconciler := &AgentTaskReconciler{
				Client: client,
				Scheme: scheme,
			}

			result, err := reconciler.Reconcile(context.Background(), ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      task.Name,
					Namespace: task.Namespace,
				},
			})
			require.NoError(t, err)
			assert.Zero(t, result.RequeueAfter)
			assert.False(t, result.Requeue)
		})
	}
}

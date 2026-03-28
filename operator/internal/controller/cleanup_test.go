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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestCleanupCompletedJobs_DeletesExpiredJobs(t *testing.T) {
	scheme := newTestReconcilerScheme()

	expiredTime := metav1.NewTime(time.Now().Add(-2 * time.Hour))
	task := newTestTask()
	task.Status.Phase = "Completed"
	task.Status.CompletedAt = &expiredTime
	task.Status.JobName = "test-task-coding"

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-task-coding",
			Namespace: "default",
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(task, job).
		WithStatusSubresource(task).
		Build()

	cleaned, err := CleanupCompletedJobs(context.Background(), client, "default", 1*time.Hour)
	require.NoError(t, err)
	assert.Equal(t, 1, cleaned)

	// Verify job was deleted
	var deletedJob batchv1.Job
	err = client.Get(context.Background(), types.NamespacedName{
		Name:      "test-task-coding",
		Namespace: "default",
	}, &deletedJob)
	assert.Error(t, err, "job should be deleted")
}

func TestCleanupCompletedJobs_SkipsRecentJobs(t *testing.T) {
	scheme := newTestReconcilerScheme()

	recentTime := metav1.NewTime(time.Now().Add(-10 * time.Minute))
	task := newTestTask()
	task.Status.Phase = "Completed"
	task.Status.CompletedAt = &recentTime
	task.Status.JobName = "test-task-coding"

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-task-coding",
			Namespace: "default",
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(task, job).
		WithStatusSubresource(task).
		Build()

	cleaned, err := CleanupCompletedJobs(context.Background(), client, "default", 1*time.Hour)
	require.NoError(t, err)
	assert.Equal(t, 0, cleaned)

	// Verify job still exists
	var existingJob batchv1.Job
	err = client.Get(context.Background(), types.NamespacedName{
		Name:      "test-task-coding",
		Namespace: "default",
	}, &existingJob)
	assert.NoError(t, err, "job should still exist")
}

func TestCleanupCompletedJobs_SkipsRunningTasks(t *testing.T) {
	scheme := newTestReconcilerScheme()

	task := newTestTask()
	task.Status.Phase = "Running"
	task.Status.JobName = "test-task-coding"

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-task-coding",
			Namespace: "default",
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(task, job).
		WithStatusSubresource(task).
		Build()

	cleaned, err := CleanupCompletedJobs(context.Background(), client, "default", 1*time.Hour)
	require.NoError(t, err)
	assert.Equal(t, 0, cleaned)
}

func TestCleanupCompletedJobs_CleansFailedTasks(t *testing.T) {
	scheme := newTestReconcilerScheme()

	expiredTime := metav1.NewTime(time.Now().Add(-2 * time.Hour))
	task := newTestTask()
	task.Status.Phase = "Failed"
	task.Status.CompletedAt = &expiredTime
	task.Status.JobName = "test-task-coding"
	task.Status.ResearchJobName = "test-task-research"

	codingJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-task-coding",
			Namespace: "default",
		},
	}
	researchJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-task-research",
			Namespace: "default",
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(task, codingJob, researchJob).
		WithStatusSubresource(task).
		Build()

	cleaned, err := CleanupCompletedJobs(context.Background(), client, "default", 1*time.Hour)
	require.NoError(t, err)
	assert.Equal(t, 2, cleaned)
}

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
	"time"

	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	aiosv1alpha1 "github.com/Diixtra/aios/operator/api/v1alpha1"
)

// CleanupClient is the client interface needed by CleanupCompletedJobs.
type CleanupClient = client.Client

// CleanupCompletedJobs lists all AgentTasks in a namespace and deletes
// associated Jobs for tasks that have been completed or failed for longer
// than the retention period. Returns the count of cleaned jobs.
func CleanupCompletedJobs(ctx context.Context, c client.Client, namespace string, retention time.Duration) (int, error) {
	logger := log.FromContext(ctx)

	var taskList aiosv1alpha1.AgentTaskList
	if err := c.List(ctx, &taskList, client.InNamespace(namespace)); err != nil {
		return 0, err
	}

	cleaned := 0
	now := time.Now()

	for i := range taskList.Items {
		task := &taskList.Items[i]

		// Only process completed or failed tasks
		if task.Status.Phase != "Completed" && task.Status.Phase != "Failed" {
			continue
		}

		// Check if CompletedAt is set and older than retention period
		if task.Status.CompletedAt == nil {
			continue
		}

		if now.Sub(task.Status.CompletedAt.Time) < retention {
			continue
		}

		// Delete associated jobs
		jobNames := []string{}
		if task.Status.JobName != "" {
			jobNames = append(jobNames, task.Status.JobName)
		}
		if task.Status.ResearchJobName != "" {
			jobNames = append(jobNames, task.Status.ResearchJobName)
		}

		for _, jobName := range jobNames {
			var job batchv1.Job
			if err := c.Get(ctx, types.NamespacedName{
				Name:      jobName,
				Namespace: namespace,
			}, &job); err != nil {
				// Job may already be deleted; skip
				logger.V(1).Info("job not found during cleanup", "job", jobName, "error", err)
				continue
			}

			propagation := client.PropagationPolicy("Background")
			if err := c.Delete(ctx, &job, propagation); err != nil {
				logger.Error(err, "failed to delete job during cleanup", "job", jobName)
				continue
			}

			cleaned++
			logger.Info("cleaned up job", "job", jobName, "task", task.Name)
		}
	}

	return cleaned, nil
}

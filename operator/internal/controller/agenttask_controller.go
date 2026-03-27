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
	"fmt"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	aiosv1alpha1 "github.com/Diixtra/aios/operator/api/v1alpha1"
)

// AgentTaskReconciler reconciles a AgentTask object.
type AgentTaskReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=aios.kazie.co.uk,resources=agenttasks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aios.kazie.co.uk,resources=agenttasks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aios.kazie.co.uk,resources=agenttasks/finalizers,verbs=update
// +kubebuilder:rbac:groups=aios.kazie.co.uk,resources=agentconfigs,verbs=get;list;watch
// +kubebuilder:rbac:groups=aios.kazie.co.uk,resources=toolpolicies,verbs=get;list;watch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *AgentTaskReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the AgentTask
	var task aiosv1alpha1.AgentTask
	if err := r.Get(ctx, req.NamespacedName, &task); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Initialize phase if empty
	if task.Status.Phase == "" {
		task.Status.Phase = "Pending"
		if err := r.Status().Update(ctx, &task); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	switch task.Status.Phase {
	case "Pending":
		return r.reconcilePending(ctx, &task)
	case "Running":
		return r.reconcileRunning(ctx, &task)
	case "Review":
		// Requeue periodically while waiting for human review
		logger.Info("task in review phase, requeueing", "task", task.Name)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	case "Completed", "Failed":
		// Terminal states, no-op
		return ctrl.Result{}, nil
	default:
		logger.Info("unknown phase", "phase", task.Status.Phase)
		return ctrl.Result{}, nil
	}
}

func (r *AgentTaskReconciler) reconcilePending(ctx context.Context, task *aiosv1alpha1.AgentTask) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Resolve AgentConfig
	var config aiosv1alpha1.AgentConfig
	if err := r.Get(ctx, types.NamespacedName{
		Name:      task.Spec.AgentConfig,
		Namespace: task.Namespace,
	}, &config); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get AgentConfig %s: %w", task.Spec.AgentConfig, err)
	}

	// Resolve ToolPolicy
	var policy aiosv1alpha1.ToolPolicy
	if err := r.Get(ctx, types.NamespacedName{
		Name:      task.Spec.ToolPolicy,
		Namespace: task.Namespace,
	}, &policy); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get ToolPolicy %s: %w", task.Spec.ToolPolicy, err)
	}

	builder := &JobBuilder{Scheme: r.Scheme}

	// Determine which job to create first
	var jobType string
	switch task.Spec.AgentType {
	case "both":
		// Start with research phase
		jobType = "research"
	case "research":
		jobType = "research"
	default:
		jobType = "coding"
	}

	// If agentType=both and we need a different tool policy for research, resolve it
	if jobType == "research" && task.Spec.ResearchToolPolicy != "" {
		var researchPolicy aiosv1alpha1.ToolPolicy
		if err := r.Get(ctx, types.NamespacedName{
			Name:      task.Spec.ResearchToolPolicy,
			Namespace: task.Namespace,
		}, &researchPolicy); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get research ToolPolicy %s: %w", task.Spec.ResearchToolPolicy, err)
		}
		policy = researchPolicy
	}

	job, err := builder.BuildJob(task, &config, &policy, jobType)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to build job: %w", err)
	}

	// Create the Job
	if err := r.Create(ctx, job); err != nil {
		if apierrors.IsAlreadyExists(err) {
			logger.Info("job already exists", "job", job.Name)
		} else {
			return ctrl.Result{}, fmt.Errorf("failed to create job: %w", err)
		}
	}

	// Update status
	now := metav1.Now()
	task.Status.Phase = "Running"
	task.Status.StartedAt = &now
	task.Status.PipelineStage = jobType

	if jobType == "research" {
		task.Status.ResearchJobName = job.Name
	} else {
		task.Status.JobName = job.Name
	}

	if err := r.Status().Update(ctx, task); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("created job, transitioning to Running", "job", job.Name, "jobType", jobType)
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

func (r *AgentTaskReconciler) reconcileRunning(ctx context.Context, task *aiosv1alpha1.AgentTask) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Determine which job to check
	jobName := task.Status.JobName
	if task.Status.PipelineStage == "research" {
		jobName = task.Status.ResearchJobName
	}

	if jobName == "" {
		logger.Info("no job name found in status, requeueing")
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// Get the Job
	var job batchv1.Job
	if err := r.Get(ctx, types.NamespacedName{
		Name:      jobName,
		Namespace: task.Namespace,
	}, &job); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("job not found, requeueing", "job", jobName)
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}
		return ctrl.Result{}, err
	}

	// Check job status
	if isJobComplete(&job) {
		return r.handleJobComplete(ctx, task)
	}

	if isJobFailed(&job) {
		return r.handleJobFailed(ctx, task, &job)
	}

	// Still running, requeue
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

func (r *AgentTaskReconciler) handleJobComplete(ctx context.Context, task *aiosv1alpha1.AgentTask) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// If agentType=both and research just finished, start coding phase
	if task.Spec.AgentType == "both" && task.Status.PipelineStage == "research" {
		logger.Info("research phase complete, starting coding phase")

		// Resolve AgentConfig and ToolPolicy for coding phase
		var config aiosv1alpha1.AgentConfig
		if err := r.Get(ctx, types.NamespacedName{
			Name:      task.Spec.AgentConfig,
			Namespace: task.Namespace,
		}, &config); err != nil {
			return ctrl.Result{}, err
		}

		var policy aiosv1alpha1.ToolPolicy
		if err := r.Get(ctx, types.NamespacedName{
			Name:      task.Spec.ToolPolicy,
			Namespace: task.Namespace,
		}, &policy); err != nil {
			return ctrl.Result{}, err
		}

		builder := &JobBuilder{Scheme: r.Scheme}
		job, err := builder.BuildJob(task, &config, &policy, "coding")
		if err != nil {
			return ctrl.Result{}, err
		}

		if err := r.Create(ctx, job); err != nil && !apierrors.IsAlreadyExists(err) {
			return ctrl.Result{}, err
		}

		task.Status.PipelineStage = "coding"
		task.Status.JobName = job.Name
		if err := r.Status().Update(ctx, task); err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// Task is complete
	now := metav1.Now()
	task.Status.Phase = "Completed"
	task.Status.CompletedAt = &now
	if err := r.Status().Update(ctx, task); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("task completed", "task", task.Name)
	return ctrl.Result{}, nil
}

func (r *AgentTaskReconciler) handleJobFailed(ctx context.Context, task *aiosv1alpha1.AgentTask, job *batchv1.Job) (ctrl.Result, error) {
	now := metav1.Now()
	task.Status.Phase = "Failed"
	task.Status.CompletedAt = &now
	task.Status.FailureReason = fmt.Sprintf("job %s failed", job.Name)

	if err := r.Status().Update(ctx, task); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func isJobComplete(job *batchv1.Job) bool {
	for _, c := range job.Status.Conditions {
		if c.Type == batchv1.JobComplete && c.Status == "True" {
			return true
		}
	}
	return false
}

func isJobFailed(job *batchv1.Job) bool {
	for _, c := range job.Status.Conditions {
		if c.Type == batchv1.JobFailed && c.Status == "True" {
			return true
		}
	}
	return false
}

// SetupWithManager sets up the controller with the Manager.
func (r *AgentTaskReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&aiosv1alpha1.AgentTask{}).
		Owns(&batchv1.Job{}).
		Named("agenttask").
		Complete(r)
}

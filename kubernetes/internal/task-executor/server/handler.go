// Copyright 2025 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"github.com/alibaba/OpenSandbox/sandbox-k8s/internal/task-executor/config"
	"github.com/alibaba/OpenSandbox/sandbox-k8s/internal/task-executor/manager"
	"github.com/alibaba/OpenSandbox/sandbox-k8s/internal/task-executor/runtime"
	"github.com/alibaba/OpenSandbox/sandbox-k8s/internal/task-executor/types"
	api "github.com/alibaba/OpenSandbox/sandbox-k8s/pkg/task-executor"
)

// ErrorResponse represents a standard error response
type ErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type resetState struct {
	mu        sync.Mutex
	status    api.ResetStatus
	version   string // Version of current reset (BatchSandbox UID)
	message   string
	details   *api.ResetDetails
	startTime time.Time
}

type Handler struct {
	manager  manager.TaskManager
	executor runtime.Executor
	config   *config.Config
	reset    *resetState
}

func NewHandler(mgr manager.TaskManager, exec runtime.Executor, cfg *config.Config) *Handler {
	if mgr == nil {
		klog.Warning("TaskManager is nil, handler may not work properly")
	}
	if cfg == nil {
		klog.Warning("Config is nil, handler may not work properly")
	}
	h := &Handler{
		manager:  mgr,
		executor: exec,
		config:   cfg,
		reset: &resetState{
			status: api.ResetStatusNone,
		},
	}
	return h
}

func (h *Handler) isResetting() bool {
	h.reset.mu.Lock()
	defer h.reset.mu.Unlock()
	return h.reset.status == api.ResetStatusInProgress
}

// isTerminalStatus returns true if the reset status is terminal (completed).
func isTerminalStatus(status api.ResetStatus) bool {
	return status == api.ResetStatusSuccess ||
		status == api.ResetStatusFailed ||
		status == api.ResetStatusTimeout ||
		status == api.ResetStatusNotSupported
}

func (h *Handler) CreateTask(w http.ResponseWriter, r *http.Request) {
	if h.manager == nil {
		writeError(w, http.StatusInternalServerError, "task manager not initialized")
		return
	}

	// Block task creation during reset to prevent race conditions
	if h.isResetting() {
		writeError(w, http.StatusServiceUnavailable, "task creation is blocked: reset operation in progress")
		return
	}

	var apiTask api.Task
	if err := json.NewDecoder(r.Body).Decode(&apiTask); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err))
		return
	}

	if apiTask.Name == "" {
		writeError(w, http.StatusBadRequest, "task name is required")
		return
	}

	task := h.convertAPIToInternalTask(&apiTask)
	if task == nil {
		writeError(w, http.StatusBadRequest, "failed to convert task")
		return
	}

	created, err := h.manager.Create(r.Context(), task)
	if err != nil {
		klog.ErrorS(err, "failed to create task", "name", apiTask.Name)
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to create task: %v", err))
		return
	}

	response := convertInternalToAPITask(created)

	writeJSON(w, http.StatusCreated, response)

	klog.InfoS("task created via API", "name", apiTask.Name)
}

func (h *Handler) SyncTasks(w http.ResponseWriter, r *http.Request) {
	if h.manager == nil {
		writeError(w, http.StatusInternalServerError, "task manager not initialized")
		return
	}

	// Block task sync during reset to prevent race conditions
	if h.isResetting() {
		writeError(w, http.StatusServiceUnavailable, "task sync is blocked: reset operation in progress")
		return
	}

	var apiTasks []api.Task
	if err := json.NewDecoder(r.Body).Decode(&apiTasks); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err))
		return
	}

	desired := make([]*types.Task, 0, len(apiTasks))
	for i := range apiTasks {
		if apiTasks[i].Name == "" {
			continue
		}
		task := h.convertAPIToInternalTask(&apiTasks[i])
		if task != nil {
			desired = append(desired, task)
		}
	}

	current, err := h.manager.Sync(r.Context(), desired)
	if err != nil {
		klog.ErrorS(err, "failed to sync tasks")
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to sync tasks: %v", err))
		return
	}

	response := make([]api.Task, 0, len(current))
	for _, task := range current {
		if task != nil {
			response = append(response, *convertInternalToAPITask(task))
		}
	}

	writeJSON(w, http.StatusOK, response)

	klog.V(1).InfoS("tasks synced via API", "count", len(response))
}

func (h *Handler) GetTask(w http.ResponseWriter, r *http.Request) {
	if h.manager == nil {
		writeError(w, http.StatusInternalServerError, "task manager not initialized")
		return
	}

	// Extract task ID from path
	taskID := r.PathValue("id")
	if taskID == "" {
		writeError(w, http.StatusBadRequest, "task id is required")
		return
	}

	task, err := h.manager.Get(r.Context(), taskID)
	if err != nil {
		klog.ErrorS(err, "failed to get task", "id", taskID)
		writeError(w, http.StatusNotFound, fmt.Sprintf("task not found: %v", err))
		return
	}

	response := convertInternalToAPITask(task)

	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) ListTasks(w http.ResponseWriter, r *http.Request) {
	if h.manager == nil {
		writeError(w, http.StatusInternalServerError, "task manager not initialized")
		return
	}

	tasks, err := h.manager.List(r.Context())
	if err != nil {
		klog.ErrorS(err, "failed to list tasks")
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list tasks: %v", err))
		return
	}

	response := make([]api.Task, 0, len(tasks))
	for _, task := range tasks {
		if task != nil {
			response = append(response, *convertInternalToAPITask(task))
		}
	}

	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	response := map[string]string{
		"status": "healthy",
	}
	writeJSON(w, http.StatusOK, response)
}

// Reset handles the reset operation for pod recycling.
// It returns immediately with the current status. If no reset is in progress, it starts a new one in a goroutine.
func (h *Handler) Reset(w http.ResponseWriter, r *http.Request) {
	if h.manager == nil {
		writeError(w, http.StatusInternalServerError, "task manager not initialized")
		return
	}

	var req api.ResetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err))
		return
	}

	// Step 1: Validate version is required
	if req.Version == "" {
		klog.ErrorS(nil, "Reset request missing version")
		writeJSON(w, http.StatusUnprocessableEntity, &api.ResetResponse{
			Status:  api.ResetStatusFailed,
			Message: "version is required for reset operation",
		})
		return
	}

	if !h.config.EnableSidecarMode {
		klog.ErrorS(nil, "Reset is only supported in sidecar mode")
		writeJSON(w, http.StatusOK, &api.ResetResponse{
			Status:  api.ResetStatusNotSupported,
			Message: "Reset is only supported in sidecar mode where task-executor runs as a sidecar container",
		})
		return
	}

	h.reset.mu.Lock()

	// Case A: version matches + InProgress -> return current status (idempotent retry)
	if h.reset.version == req.Version && h.reset.status == api.ResetStatusInProgress {
		klog.InfoS("Reset already in progress for same version, returning current status", "version", req.Version)
		resp := h.currentResetResponse()
		h.reset.mu.Unlock()
		writeJSON(w, http.StatusOK, resp)
		return
	}

	// Case B: version matches + terminal status -> return result (idempotent query)
	if h.reset.version == req.Version && isTerminalStatus(h.reset.status) {
		klog.InfoS("Reset already completed for same version, returning result", "version", req.Version, "status", h.reset.status)
		resp := h.currentResetResponse()
		h.reset.mu.Unlock()
		writeJSON(w, http.StatusOK, resp)
		return
	}

	// Case C: version mismatch (or first reset) -> start new reset
	if h.reset.version != "" {
		klog.InfoS("New version detected, starting new reset", "previousVersion", h.reset.version, "newVersion", req.Version)
	} else {
		klog.InfoS("Starting first reset", "version", req.Version)
	}

	h.reset.status = api.ResetStatusInProgress
	h.reset.version = req.Version
	h.reset.startTime = time.Now()
	h.reset.message = "Reset started"
	h.reset.details = &api.ResetDetails{}

	resp := h.currentResetResponse()

	klog.InfoS("Starting reset operation", "mainContainer", req.MainContainerName, "version", req.Version)

	h.reset.mu.Unlock()

	go h.executeReset(req)

	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) currentResetResponse() *api.ResetResponse {
	return &api.ResetResponse{
		Status:  h.reset.status,
		Message: h.reset.message,
		Details: h.reset.details,
	}
}

func (h *Handler) executeReset(req api.ResetRequest) {
	defer func() {
		if err := recover(); err != nil {
			h.reset.mu.Lock()
			if h.reset.status == api.ResetStatusInProgress {
				h.reset.status = api.ResetStatusFailed
				h.reset.message = fmt.Sprintf("reset goroutine panic: %v", err)
			}
			h.reset.mu.Unlock()
			klog.ErrorS(nil, "Reset goroutine panicked", "error", err)
		}
	}()

	timeout := time.Duration(req.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	klog.InfoS("Starting reset operation", "timeout", timeout)

	// Step 1: Stop and clean up all tasks
	stopped, err := h.manager.Clear(ctx)
	if err != nil {
		h.setResetFailed(fmt.Sprintf("failed to clear tasks: %v", err))
		return
	}
	h.reset.mu.Lock()
	h.reset.details.TasksStopped = stopped
	h.reset.mu.Unlock()
	klog.InfoS("Stopped tasks during reset", "count", stopped)

	// Step 2: Clean task data directory
	if err := h.cleanTaskDataDir(); err != nil {
		h.setResetFailed(fmt.Sprintf("failed to clean task data dir: %v", err))
		return
	}

	// Step 3: Clean user-specified directories
	if len(req.CleanDirectories) > 0 {
		cleaned, err := h.cleanDirectories(req.CleanDirectories)
		if err != nil {
			h.setResetFailed(fmt.Sprintf("failed to clean directories: %v", err))
			return
		}
		h.reset.mu.Lock()
		h.reset.details.DirectoriesCleaned = cleaned
		h.reset.mu.Unlock()
		klog.InfoS("Cleaned directories during reset", "directories", cleaned)
	}

	// Step 4: Restart main container
	mainContainer := req.MainContainerName
	if mainContainer == "" {
		mainContainer = h.config.MainContainerName
	}
	if mainContainer != "" && h.executor != nil {
		if err := h.executor.RestartMainContainer(ctx, mainContainer); err != nil {
			h.setResetFailed(fmt.Sprintf("failed to restart main container: %v", err))
			return
		}
		h.reset.mu.Lock()
		h.reset.details.MainContainerRestarted = true
		h.reset.mu.Unlock()
		klog.InfoS("Restarted main container during reset", "container", mainContainer)
	}

	// Check if context was cancelled (timeout)
	if ctx.Err() == context.DeadlineExceeded {
		h.setResetStatus(api.ResetStatusTimeout, "reset operation timed out")
		return
	}

	h.reset.mu.Lock()
	h.reset.status = api.ResetStatusSuccess
	h.reset.message = "Reset completed successfully"
	h.reset.mu.Unlock()

	klog.InfoS("Reset operation completed successfully")
}

func (h *Handler) setResetStatus(status api.ResetStatus, message string) {
	h.reset.mu.Lock()
	h.reset.status = status
	h.reset.message = message
	h.reset.mu.Unlock()
	if status == api.ResetStatusFailed || status == api.ResetStatusTimeout {
		klog.ErrorS(nil, "Reset operation failed", "status", status, "message", message)
	}
}

func (h *Handler) setResetFailed(message string) {
	h.setResetStatus(api.ResetStatusFailed, message)
}

func (h *Handler) cleanTaskDataDir() error {
	if h.config.DataDir == "" {
		return nil
	}

	entries, err := os.ReadDir(h.config.DataDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read data directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		taskDir := filepath.Join(h.config.DataDir, entry.Name())
		if err := os.RemoveAll(taskDir); err != nil {
			klog.ErrorS(err, "Failed to remove task directory", "path", taskDir)
		} else {
			klog.InfoS("Removed task directory", "path", taskDir)
		}
	}
	return nil
}

func (h *Handler) cleanDirectories(dirs []string) ([]string, error) {
	var cleaned []string
	for _, dir := range dirs {
		matches, err := filepath.Glob(dir)
		if err != nil {
			klog.ErrorS(err, "Invalid glob pattern", "pattern", dir)
			continue
		}

		for _, match := range matches {
			if err := os.RemoveAll(match); err != nil {
				klog.ErrorS(err, "Failed to clean directory", "path", match)
				continue
			}
			cleaned = append(cleaned, match)
			klog.InfoS("Cleaned directory", "path", match)
		}
	}
	return cleaned, nil
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (h *Handler) DeleteTask(w http.ResponseWriter, r *http.Request) {
	if h.manager == nil {
		writeError(w, http.StatusInternalServerError, "task manager not initialized")
		return
	}

	// Extract task ID from path
	taskID := r.PathValue("id")
	if taskID == "" {
		writeError(w, http.StatusBadRequest, "task id is required")
		return
	}

	err := h.manager.Delete(r.Context(), taskID)
	if err != nil {
		klog.ErrorS(err, "failed to delete task", "id", taskID)
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to delete task: %v", err))
		return
	}

	w.WriteHeader(http.StatusNoContent)
	klog.InfoS("task deleted via API", "id", taskID)
}

func writeError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(ErrorResponse{
		Code:    http.StatusText(code),
		Message: message,
	})
}

func (h *Handler) convertAPIToInternalTask(apiTask *api.Task) *types.Task {
	if apiTask == nil {
		return nil
	}
	task := &types.Task{
		Name:            apiTask.Name,
		Process:         apiTask.Process,
		PodTemplateSpec: apiTask.PodTemplateSpec,
	}
	task.Status = types.Status{
		State: types.TaskStatePending,
	}

	return task
}

func convertInternalToAPITask(task *types.Task) *api.Task {
	if task == nil {
		return nil
	}

	apiTask := &api.Task{
		Name:            task.Name,
		Process:         task.Process,
		PodTemplateSpec: task.PodTemplateSpec,
	}

	if task.Process != nil && len(task.Status.SubStatuses) > 0 {
		sub := task.Status.SubStatuses[0]
		apiStatus := &api.ProcessStatus{}

		if task.Status.State == types.TaskStateTimeout {
			term := &api.Terminated{
				ExitCode: 137,
				Reason:   sub.Reason,
				Message:  sub.Message,
			}
			if sub.StartedAt != nil {
				term.StartedAt = metav1.NewTime(*sub.StartedAt)
			}
			term.FinishedAt = metav1.Now()
			apiStatus.Terminated = term
		} else if sub.FinishedAt != nil {
			term := &api.Terminated{
				ExitCode: int32(sub.ExitCode),
				Reason:   sub.Reason,
				Message:  sub.Message,
			}
			term.FinishedAt = metav1.NewTime(*sub.FinishedAt)
			if sub.StartedAt != nil {
				term.StartedAt = metav1.NewTime(*sub.StartedAt)
			}
			apiStatus.Terminated = term
		} else if sub.StartedAt != nil {
			apiStatus.Running = &api.Running{
				StartedAt: metav1.NewTime(*sub.StartedAt),
			}
		} else {
			apiStatus.Waiting = &api.Waiting{
				Reason:  sub.Reason,
				Message: sub.Message,
			}
		}
		apiTask.ProcessStatus = apiStatus
	}

	if task.PodTemplateSpec != nil {
		podStatus := &corev1.PodStatus{
			Phase: corev1.PodUnknown,
		}

		switch task.Status.State {
		case types.TaskStatePending:
			podStatus.Phase = corev1.PodPending
		case types.TaskStateRunning:
			podStatus.Phase = corev1.PodRunning
		case types.TaskStateSucceeded:
			podStatus.Phase = corev1.PodSucceeded
		case types.TaskStateFailed:
			podStatus.Phase = corev1.PodFailed
		}

		for _, sub := range task.Status.SubStatuses {
			cs := corev1.ContainerStatus{
				Name: sub.Name,
			}
			if sub.FinishedAt != nil {
				cs.State.Terminated = &corev1.ContainerStateTerminated{
					ExitCode:   int32(sub.ExitCode),
					Reason:     sub.Reason,
					Message:    sub.Message,
					FinishedAt: metav1.NewTime(*sub.FinishedAt),
				}
				if sub.StartedAt != nil {
					cs.State.Terminated.StartedAt = metav1.NewTime(*sub.StartedAt)
				}
			} else if sub.StartedAt != nil {
				cs.State.Running = &corev1.ContainerStateRunning{
					StartedAt: metav1.NewTime(*sub.StartedAt),
				}
				cs.Ready = true
			} else {
				cs.State.Waiting = &corev1.ContainerStateWaiting{
					Reason:  sub.Reason,
					Message: sub.Message,
				}
			}
			podStatus.ContainerStatuses = append(podStatus.ContainerStatuses, cs)
		}

		allReady := len(podStatus.ContainerStatuses) > 0
		for _, cs := range podStatus.ContainerStatuses {
			if !cs.Ready {
				allReady = false
				break
			}
		}
		readyStatus := corev1.ConditionFalse
		if allReady {
			readyStatus = corev1.ConditionTrue
		}

		var latestTransition time.Time
		for _, sub := range task.Status.SubStatuses {
			if sub.StartedAt != nil && sub.StartedAt.After(latestTransition) {
				latestTransition = *sub.StartedAt
			}
			if sub.FinishedAt != nil && sub.FinishedAt.After(latestTransition) {
				latestTransition = *sub.FinishedAt
			}
		}
		ltt := metav1.NewTime(latestTransition)
		if latestTransition.IsZero() {
			ltt = metav1.Now()
		}

		podStatus.Conditions = append(podStatus.Conditions,
			corev1.PodCondition{
				Type:               corev1.PodReady,
				Status:             readyStatus,
				LastTransitionTime: ltt,
			},
			corev1.PodCondition{
				Type:               corev1.ContainersReady,
				Status:             readyStatus,
				LastTransitionTime: ltt,
			},
		)

		apiTask.PodStatus = podStatus
	}

	return apiTask
}

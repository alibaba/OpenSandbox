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

package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"k8s.io/klog/v2"

	"github.com/alibaba/OpenSandbox/sandbox-k8s/internal/task-executor/config"
	"github.com/alibaba/OpenSandbox/sandbox-k8s/internal/task-executor/manager"
	"github.com/alibaba/OpenSandbox/sandbox-k8s/internal/task-executor/runtime"
	"github.com/alibaba/OpenSandbox/sandbox-k8s/internal/task-executor/server"
	store "github.com/alibaba/OpenSandbox/sandbox-k8s/internal/task-executor/storage"
)

func main() {
	// Load configuration
	cfg := config.NewConfig()
	cfg.LoadFromEnv()
	cfg.LoadFromFlags()

	klog.InfoS("task-executor starting", "dataDir", cfg.DataDir, "listenAddr", cfg.ListenAddr, "sidecarMode", cfg.EnableSidecarMode)

	// Initialize TaskStore
	taskStore, err := store.NewFileStore(cfg.DataDir)
	if err != nil {
		klog.ErrorS(err, "failed to create task store")
		os.Exit(1)
	}
	klog.InfoS("task store initialized", "dataDir", cfg.DataDir)

	// Initialize Executor
	exec, err := runtime.NewExecutor(cfg)
	if err != nil {
		klog.ErrorS(err, "failed to create executor")
		os.Exit(1)
	}
	mode := "process"
	if cfg.EnableContainerMode {
		mode = "container"
	}
	klog.InfoS("executor initialized", "mode", mode)

	// Initialize TaskManager
	taskManager, err := manager.NewTaskManager(cfg, taskStore, exec)
	if err != nil {
		klog.ErrorS(err, "failed to create task manager")
		os.Exit(1)
	}

	// Start TaskManager
	taskManager.Start(context.Background())
	klog.InfoS("task manager started")

	// Initialize HTTP Handler and Router
	handler := server.NewHandler(taskManager, cfg)
	router := server.NewRouter(handler)

	// Create HTTP Server
	svr := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      router,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}

	// Start HTTP server in goroutine
	go func() {
		klog.InfoS("HTTP server listening", "address", cfg.ListenAddr)
		if err := svr.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			klog.ErrorS(err, "HTTP server error")
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	klog.InfoS("shutting down task-executor gracefully...")

	// Shutdown context with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// 1. Stop HTTP server first
	if err := svr.Shutdown(shutdownCtx); err != nil {
		klog.ErrorS(err, "HTTP server shutdown error")
	} else {
		klog.InfoS("HTTP server stopped")
	}

	// 2. Stop TaskManager
	taskManager.Stop()
	klog.InfoS("task manager stopped")

	klog.InfoS("task-executor stopped successfully")
}

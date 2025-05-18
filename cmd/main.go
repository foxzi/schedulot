package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/piligrim/gotask2/pkg/config"
	"github.com/piligrim/gotask2/pkg/executor"
	"github.com/piligrim/gotask2/pkg/logger"
	"github.com/piligrim/gotask2/pkg/scheduler"
)

const (
	logFilePath  = "app.log"
	tasksDirPath = "./tasks"
)

func main() {
	appLogger, err := logger.New(logFilePath)
	if err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	appLogger.Info("Main", "Application starting...")

	taskExecutor := executor.New(appLogger)
	taskScheduler := scheduler.New(taskExecutor, appLogger)

	loadedTasks, err := config.LoadTasksFromDir(tasksDirPath)
	if err != nil {
		appLogger.Error("Main", "Failed to load tasks", err)
		os.Exit(1)
	}

	if len(loadedTasks) == 0 {
		appLogger.Info("Main", "No tasks found in %s. The application will run but will not schedule any tasks.", tasksDirPath)
	} else {
		for _, t := range loadedTasks {
			if err := taskScheduler.AddTask(t); err != nil {
				appLogger.Error("Main", fmt.Sprintf("Failed to add task %s", t.ID), err)
			}
		}
	}

	taskScheduler.Start()
	appLogger.Info("Main", "Task scheduler started. Press Ctrl+C to exit.")

	// Graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	appLogger.Info("Main", "Shutdown signal received. Stopping scheduler...")
	taskScheduler.Stop()
	appLogger.Info("Main", "Application stopped.")
}

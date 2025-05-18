package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/piligrim/gotask2/pkg/config"
	"github.com/piligrim/gotask2/pkg/executor"
	"github.com/piligrim/gotask2/pkg/logger"
	"github.com/piligrim/gotask2/pkg/scheduler"
)

const (
	defaultLogFilePath  = "app.log"
	defaultTasksDirPath = "./tasks"
)

func main() {
	logFilePath := flag.String("logfile", defaultLogFilePath, "Path to the log file.")
	tasksDirPath := flag.String("tasksdir", defaultTasksDirPath, "Directory containing task YAML files.")
	validateTasks := flag.Bool("validate", false, "Validate tasks in the tasks directory and exit.")
	flag.Parse()

	absTasksDirPath, err := filepath.Abs(*tasksDirPath)
	if err != nil {
		fmt.Printf("Error getting absolute path for tasks directory: %v\n", err)
		os.Exit(1)
	}

	appLogger, err := logger.New(*logFilePath)
	if err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	if *validateTasks {
		appLogger.Info("Main", "Starting tasks validation in directory: %s", absTasksDirPath)
		tasks, err := config.LoadTasksFromDir(absTasksDirPath, appLogger) // Pass logger here
		if err != nil {
			appLogger.Error("Main", "Error loading tasks during validation", err)
			os.Exit(1)
		}
		if len(tasks) == 0 {
			appLogger.Info("Main", "No valid tasks found or directory is empty.")
		} else {
			appLogger.Info("Main", "Successfully validated %d task(s).", len(tasks))
			for fp, task := range tasks {
				appLogger.Info("Main", "  - Valid task ID: %s from file: %s", task.ID, fp)
			}
		}
		appLogger.Info("Main", "Task validation finished.")
		os.Exit(0)
	}

	appLogger.Info("Main", "Application starting...")
	appLogger.Info("Main", "Using tasks directory: %s", absTasksDirPath)
	appLogger.Info("Main", "Using log file: %s", *logFilePath)

	taskExecutor := executor.New(appLogger)
	taskScheduler, err := scheduler.New(taskExecutor, appLogger, absTasksDirPath) // Pass absTasksDirPath
	if err != nil {
		appLogger.Error("Main", "Failed to create scheduler", err)
		os.Exit(1)
	}

	// Initial task loading is now handled by scheduler.Start()
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

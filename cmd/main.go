package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/piligrim/gotask2/pkg/config"
	"github.com/piligrim/gotask2/pkg/executor"
	"github.com/piligrim/gotask2/pkg/logger"
	"github.com/piligrim/gotask2/pkg/scheduler"
)

const (
	defaultLogOutput    = "app.log"
	defaultLogMode      = "file" // or "stdout"
	defaultTasksDirPath = "./tasks"
)

func main() {
	logOutput := flag.String("logoutput", defaultLogOutput, "Log output path (if mode is 'file') or 'stdout'.")
	logModeStr := flag.String("logmode", defaultLogMode, "Log mode: 'file' or 'stdout'.")
	tasksDirPath := flag.String("tasksdir", defaultTasksDirPath, "Directory containing task YAML files.")
	validateTasks := flag.Bool("validate", false, "Validate tasks in the tasks directory and exit.")
	flag.Parse()

	var appLogger *logger.Logger
	var err error

	// Determine logger mode
	var currentLogMode logger.LogMode
	switch strings.ToLower(*logModeStr) {
	case "file":
		currentLogMode = logger.LogModeFile
	case "stdout":
		currentLogMode = logger.LogModeStdout
	default:
		fmt.Printf("Invalid log mode: %s. Using default '%s'.\n", *logModeStr, defaultLogMode)
		currentLogMode = logger.LogModeFile // Or your preferred default
	}

	appLogger, err = logger.New(*logOutput, currentLogMode)
	if err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	absTasksDirPath, err := filepath.Abs(*tasksDirPath)
	if err != nil {
		appLogger.Error("Main", "Error getting absolute path for tasks directory", err)
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
			for fp, taskInstance := range tasks { // Renamed task to taskInstance
				appLogger.Info("Main", "  - Valid task ID: %s from file: %s", taskInstance.ID, fp)
			}
		}
		appLogger.Info("Main", "Task validation finished.")
		os.Exit(0)
	}

	appLogger.Info("Main", "Application starting...")
	appLogger.Info("Main", "Using tasks directory: %s", absTasksDirPath)
	if currentLogMode == logger.LogModeFile {
		appLogger.Info("Main", "Using log file: %s", *logOutput)
	} else {
		appLogger.Info("Main", "Logging to stdout")
	}

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

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
	defaultConfigPath   = "./config.yaml"
)

var Version = "dev"

func main() {
	// Добавляем флаг для указания версии
	showVersion := flag.Bool("version", false, "Show application version and exit.")
	// Добавляем флаг для указания пути к конфигурационному файлу
	configPath := flag.String("config", defaultConfigPath, "Path to the configuration file.")
	logOutput := flag.String("logoutput", defaultLogOutput, "Log output path (if mode is 'file') or 'stdout'.")
	logModeStr := flag.String("logmode", defaultLogMode, "Log mode: 'file' or 'stdout'.")
	tasksDirPath := flag.String("tasksdir", defaultTasksDirPath, "Directory containing task YAML files.")
	validateTasks := flag.Bool("validate", false, "Validate tasks in the tasks directory and exit.")
	flag.Parse()

	if *showVersion {
		fmt.Println("schedulot version:", Version)
		os.Exit(0)
	}

	// Пытаемся загрузить конфигурацию из файла
	var appCfg *config.AppConfig
	var useConfig bool = false

	if _, err := os.Stat(*configPath); err == nil {
		// Конфигурационный файл существует, загружаем его
		appCfg, err = config.LoadConfig(*configPath)
		if err != nil {
			fmt.Printf("Error loading configuration file: %v. Using command line arguments.\n", err)
		} else {
			useConfig = true
			fmt.Printf("Configuration loaded from: %s\n", *configPath)
		}
	}

	var appLogger *logger.Logger
	var err error

	// Определяем режим логирования на основе конфигурации или аргументов
	var currentLogMode logger.LogMode
	var logOutputPath string

	if useConfig {
		// Используем настройки из конфигурационного файла
		switch strings.ToLower(appCfg.LogSettings.LogMode) {
		case "file":
			currentLogMode = logger.LogModeFile
		case "stdout":
			currentLogMode = logger.LogModeStdout
		default:
			fmt.Printf("Invalid log mode in config: %s. Using default '%s'.\n",
				appCfg.LogSettings.LogMode, defaultLogMode)
			currentLogMode = logger.LogModeFile
		}
		logOutputPath = appCfg.LogSettings.LogFilePath
	} else {
		// Используем настройки из аргументов командной строки
		switch strings.ToLower(*logModeStr) {
		case "file":
			currentLogMode = logger.LogModeFile
		case "stdout":
			currentLogMode = logger.LogModeStdout
		default:
			fmt.Printf("Invalid log mode: %s. Using default '%s'.\n", *logModeStr, defaultLogMode)
			currentLogMode = logger.LogModeFile
		}
		logOutputPath = *logOutput
	}

	// Инициализация логгера
	appLogger, err = logger.New(logOutputPath, currentLogMode)
	if err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	// Определяем путь к директории задач
	var absTasksDirPath string
	if useConfig {
		absTasksDirPath, err = filepath.Abs(appCfg.TasksSettings.TasksFolderPath)
	} else {
		absTasksDirPath, err = filepath.Abs(*tasksDirPath)
	}

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
		appLogger.Info("Main", "Using log file: %s", logOutputPath)
	} else {
		appLogger.Info("Main", "Logging to stdout")
	}

	// Если используется конфигурация, выводим информацию о плагинах уведомлений
	if useConfig && len(appCfg.NotificationConfig) > 0 {
		appLogger.Info("Main", "Loaded notification plugins configuration:")
		for name, plugin := range appCfg.NotificationConfig {
			appLogger.Info("Main", "  - Plugin: %s, Type: %s", name, plugin.Type)
		}
	}

	taskExecutor := executor.New(appLogger)

	var taskScheduler *scheduler.Scheduler
	if useConfig {
		// Создаем планировщик с конфигурацией плагинов
		taskScheduler, err = scheduler.NewWithConfig(taskExecutor, appLogger, absTasksDirPath, appCfg.NotificationConfig)
	} else {
		// Создаем планировщик без конфигурации плагинов
		taskScheduler, err = scheduler.New(taskExecutor, appLogger, absTasksDirPath)
	}

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

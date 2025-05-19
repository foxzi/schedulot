package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/piligrim/gotask2/pkg/logger"
	"github.com/piligrim/gotask2/pkg/task"
	"gopkg.in/yaml.v3"
)

// AppConfig представляет основную конфигурацию приложения
type AppConfig struct {
	LogSettings        LogSettings                    `yaml:"log_settings"`
	TasksSettings      TasksSettings                  `yaml:"tasks_settings"`
	NotificationConfig map[string]PluginConfiguration `yaml:"notification_plugins"`
}

// LogSettings содержит настройки логирования
type LogSettings struct {
	LogFilePath string `yaml:"log_file_path"` // Путь к файлу логов
	LogMode     string `yaml:"log_mode"`      // Режим логирования: "file" или "stdout"
}

// TasksSettings содержит настройки для задач
type TasksSettings struct {
	TasksFolderPath string `yaml:"tasks_folder_path"` // Путь к папке с задачами
}

// PluginConfiguration содержит настройки для плагина уведомлений
type PluginConfiguration struct {
	Type   string            `yaml:"type"`   // Тип плагина: "email", "slack", "file", "telegram"
	Params map[string]string `yaml:"params"` // Параметры для настройки плагина
}

// LoadConfig загружает конфигурацию из YAML файла
func LoadConfig(filePath string) (*AppConfig, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", filePath, err)
	}

	var config AppConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config from %s: %w", filePath, err)
	}

	// Устанавливаем дефолтные значения, если они не указаны
	if config.LogSettings.LogMode == "" {
		config.LogSettings.LogMode = "file"
	}
	if config.LogSettings.LogFilePath == "" {
		config.LogSettings.LogFilePath = "app.log"
	}
	if config.TasksSettings.TasksFolderPath == "" {
		config.TasksSettings.TasksFolderPath = "./tasks"
	}

	return &config, nil
}

// LoadTaskFromFile loads a single task definition from a YAML file.
func LoadTaskFromFile(filePath string) (task.Task, error) {
	var t task.Task
	data, err := os.ReadFile(filePath)
	if err != nil {
		return t, fmt.Errorf("failed to read task file %s: %w", filePath, err)
	}

	if err := yaml.Unmarshal(data, &t); err != nil {
		return t, fmt.Errorf("failed to unmarshal task from %s: %w", filePath, err)
	}

	if t.ID == "" {
		return t, fmt.Errorf("task ID is missing in file %s", filePath)
	}
	return t, nil
}

// LoadTasksFromDir loads all task definitions from YAML files in a given directory.
// It logs errors for individual invalid files using the provided logger and skips them.
func LoadTasksFromDir(dirPath string, l *logger.Logger) (map[string]task.Task, error) { // filePath -> Task
	absDirPath, err := filepath.Abs(dirPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for directory %s: %w", dirPath, err)
	}

	files, err := os.ReadDir(absDirPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %s: %w", absDirPath, err)
	}

	tasks := make(map[string]task.Task)
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		fileName := file.Name()
		if !(strings.HasSuffix(fileName, ".yml") || strings.HasSuffix(fileName, ".yaml")) {
			continue
		}

		absFilePath := filepath.Join(absDirPath, fileName)
		t, err := LoadTaskFromFile(absFilePath)
		if err != nil {
			if l != nil {
				l.Error("ConfigLoader", fmt.Sprintf("Failed to load task from %s", absFilePath), err)
			} else {
				fmt.Printf("Error loading task from %s: %v\n", absFilePath, err) // Fallback if logger is nil
			}
			continue
		}
		tasks[absFilePath] = t
	}
	return tasks, nil
}

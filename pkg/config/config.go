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

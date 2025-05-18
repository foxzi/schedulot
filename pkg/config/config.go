package config

import (
	"fmt"
	"os"

	"github.com/piligrim/gotask2/pkg/task"
	"gopkg.in/yaml.v3"
)

// LoadTasksFromDir loads all task definitions from YAML files in a given directory.
func LoadTasksFromDir(dirPath string) ([]task.Task, error) {
	files, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %s: %w", dirPath, err)
	}

	var tasks []task.Task
	for _, file := range files {
		if file.IsDir() || (len(file.Name()) < 5 || (file.Name()[len(file.Name())-4:] != ".yml" && file.Name()[len(file.Name())-5:] != ".yaml")) {
			continue
		}

		filePath := dirPath + "/" + file.Name()
		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read task file %s: %w", filePath, err)
		}

		var t task.Task
		if err := yaml.Unmarshal(data, &t); err != nil {
			return nil, fmt.Errorf("failed to unmarshal task from %s: %w", filePath, err)
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

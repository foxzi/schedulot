package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/piligrim/gotask2/pkg/config"
)

func TestLoadConfig_Defaults(t *testing.T) {
	tmpFile := filepath.Join(os.TempDir(), "schedulot_test_config.yaml")
	defer os.Remove(tmpFile)
	content := []byte(`{}`)
	if err := os.WriteFile(tmpFile, content, 0644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}
	cfg, err := config.LoadConfig(tmpFile)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.LogSettings.LogMode != "file" {
		t.Errorf("expected default log_mode 'file', got '%s'", cfg.LogSettings.LogMode)
	}
	if cfg.LogSettings.LogFilePath != "app.log" {
		t.Errorf("expected default log_file_path 'app.log', got '%s'", cfg.LogSettings.LogFilePath)
	}
	if cfg.TasksSettings.TasksFolderPath != "./tasks" {
		t.Errorf("expected default tasks_folder_path './tasks', got '%s'", cfg.TasksSettings.TasksFolderPath)
	}
}

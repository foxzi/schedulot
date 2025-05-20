package logger_test

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/piligrim/gotask2/pkg/logger"
)

func TestLogger_InfoAndError(t *testing.T) {
	tmpFile := "logger_test.log"
	defer os.Remove(tmpFile)
	l, err := logger.New(tmpFile, logger.LogModeFile)
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	l.Info("test-task", "info message: %d", 42)
	l.Error("test-task", "error message: %s", nil, "foo")
	l.Error("test-task", "error with err", os.ErrNotExist)
	l.LogExecutionResult("test-task", time.Now().Add(-time.Second), nil, "output")
	l.LogExecutionResult("test-task", time.Now().Add(-time.Second), os.ErrNotExist, "fail output")
	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}
	logStr := string(data)
	if !strings.Contains(logStr, "info message") || !strings.Contains(logStr, "error message") {
		t.Errorf("log file missing expected content: %s", logStr)
	}
}

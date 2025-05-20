package executor_test

import (
	"strings"
	"testing"
	"time"

	"github.com/piligrim/gotask2/pkg/executor"
	"github.com/piligrim/gotask2/pkg/logger"
	"github.com/piligrim/gotask2/pkg/task"
)

func TestExecuteTask_CommandSuccess(t *testing.T) {
	l, _ := logger.New("/dev/null", logger.LogModeStdout)
	e := executor.New(l)
	tk := task.Task{
		ID: "test-echo",
		Action: task.Action{
			Type: task.ActionTypeCommand,
			Command: &task.CommandAction{
				Path: "echo",
				Args: []string{"hello"},
			},
		},
		Timeout: 2 * time.Second,
	}
	out, err := e.ExecuteTask(tk)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(out, "hello") {
		t.Errorf("expected output to contain 'hello', got '%s'", out)
	}
}

func TestExecuteTask_CommandFail(t *testing.T) {
	l, _ := logger.New("/dev/null", logger.LogModeStdout)
	e := executor.New(l)
	tk := task.Task{
		ID: "test-fail",
		Action: task.Action{
			Type: task.ActionTypeCommand,
			Command: &task.CommandAction{
				Path: "false",
			},
		},
		Timeout: 2 * time.Second,
	}
	_, err := e.ExecuteTask(tk)
	if err == nil {
		t.Errorf("expected error for failing command")
	}
}

func TestExecuteTask_UnknownType(t *testing.T) {
	l, _ := logger.New("/dev/null", logger.LogModeStdout)
	e := executor.New(l)
	tk := task.Task{
		ID:     "test-unknown",
		Action: task.Action{Type: "unknown"},
	}
	_, err := e.ExecuteTask(tk)
	if err == nil {
		t.Errorf("expected error for unknown action type")
	}
}

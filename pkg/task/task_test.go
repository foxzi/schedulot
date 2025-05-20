package task_test

import (
	"testing"
	"time"

	"github.com/piligrim/gotask2/pkg/task"
)

func TestTaskStruct(t *testing.T) {
	tsk := task.Task{
		ID:          "t1",
		Description: "desc",
		Triggers:    []task.Trigger{{Type: task.TriggerTypeSchedule, Schedule: "* * * * *"}},
		Action:      task.Action{Type: task.ActionTypeCommand, Command: &task.CommandAction{Path: "echo"}},
		DependsOn:   []task.Dependency{{TaskID: "t0", Status: task.DependencyStatusSuccess}},
		MaxRetries:  2,
		Timeout:     2 * time.Second,
	}
	if tsk.ID != "t1" || tsk.Action.Type != task.ActionTypeCommand {
		t.Errorf("Task struct fields not set correctly")
	}
}

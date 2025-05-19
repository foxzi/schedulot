package task

import (
	"time"

	"github.com/piligrim/gotask2/pkg/notification" // Added import
)

// TriggerType defines the type of trigger for a task.
type TriggerType string

const (
	TriggerTypeSchedule   TriggerType = "schedule"
	TriggerTypeDependency TriggerType = "dependency"
)

// ActionType defines the type of action a task can perform.
type ActionType string

const (
	ActionTypeCommand ActionType = "command"
	ActionTypeHTTP    ActionType = "http"
)

// DependencyStatus defines the required status of a dependent task.
type DependencyStatus string

const (
	DependencyStatusSuccess DependencyStatus = "success"
	DependencyStatusFailure DependencyStatus = "failure"
)

// Task defines the structure for a task.
type Task struct {
	ID          string                            `yaml:"id"`
	Description string                            `yaml:"description,omitempty"`
	Triggers    []Trigger                         `yaml:"triggers"`
	Action      Action                            `yaml:"action"`
	DependsOn   []Dependency                      `yaml:"depends_on,omitempty"` // IDs of tasks this task depends on
	MaxRetries  int                               `yaml:"max_retries,omitempty"`
	Timeout     time.Duration                     `yaml:"timeout,omitempty"` // e.g., "30s", "5m"
	Notify      []notification.NotificationConfig `yaml:"notify,omitempty"`  // Added field
	// --- Runtime context for templating ---
	ParentTaskID string `yaml:"-"`
	ParentDate   string `yaml:"-"`
	ParentData   string `yaml:"-"`
}

// Trigger defines how a task is initiated.
type Trigger struct {
	Type     TriggerType `yaml:"type"`
	Schedule string      `yaml:"schedule,omitempty"` // Cron expression for schedule type
}

// Dependency defines a dependency on another task.
type Dependency struct {
	TaskID string           `yaml:"task_id"`
	Status DependencyStatus `yaml:"status"` // "success" or "failure"
}

// Action defines what the task does.
type Action struct {
	Type        ActionType         `yaml:"type"`
	Command     *CommandAction     `yaml:"command,omitempty"`
	HTTPRequest *HTTPRequestAction `yaml:"http_request,omitempty"`
}

// CommandAction defines a command to be executed.
type CommandAction struct {
	Path string   `yaml:"path"`
	Args []string `yaml:"args,omitempty"`
	Dir  string   `yaml:"dir,omitempty"` // Working directory for the command
}

// HTTPRequestAction defines an HTTP request to be made.
type HTTPRequestAction struct {
	URL     string            `yaml:"url"`
	Method  string            `yaml:"method"` // e.g., "GET", "POST"
	Headers map[string]string `yaml:"headers,omitempty"`
	Body    string            `yaml:"body,omitempty"`
}

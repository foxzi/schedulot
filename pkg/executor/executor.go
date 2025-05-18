package executor

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/piligrim/gotask2/pkg/logger"
	"github.com/piligrim/gotask2/pkg/task"
)

// Executor is responsible for executing tasks.
type Executor struct {
	logger *logger.Logger
}

// New creates a new Executor.
func New(l *logger.Logger) *Executor {
	return &Executor{logger: l}
}

// ExecuteTask executes a given task.
func (e *Executor) ExecuteTask(t task.Task) (string, error) {
	e.logger.Info(t.ID, "Starting task execution")
	startTime := time.Now()

	var output string
	var err error

	ctx := context.Background()
	var cancel context.CancelFunc
	if t.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, t.Timeout)
		defer cancel()
	}

	switch t.Action.Type {
	case task.ActionTypeCommand:
		if t.Action.Command == nil {
			err = fmt.Errorf("command action is nil")
			break
		}
		cmd := exec.CommandContext(ctx, t.Action.Command.Path, t.Action.Command.Args...)
		if t.Action.Command.Dir != "" {
			cmd.Dir = t.Action.Command.Dir
		}
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err = cmd.Run()
		output = stdout.String()
		if err != nil {
			output += "\nSTDERR: " + stderr.String()
			err = fmt.Errorf("command execution failed: %w, stderr: %s", err, stderr.String())
		} else if stderr.Len() > 0 {
			// Log stderr even if the command exits successfully
			e.logger.Info(t.ID, "Command stderr: %s", stderr.String())
		}
	case task.ActionTypeHTTP:
		if t.Action.HTTPRequest == nil {
			err = fmt.Errorf("http_request action is nil")
			break
		}
		reqAction := t.Action.HTTPRequest
		var reqBody io.Reader
		if reqAction.Body != "" {
			reqBody = strings.NewReader(reqAction.Body)
		}

		req, httpErr := http.NewRequestWithContext(ctx, strings.ToUpper(reqAction.Method), reqAction.URL, reqBody)
		if httpErr != nil {
			err = fmt.Errorf("failed to create HTTP request: %w", httpErr)
			break
		}

		for key, val := range reqAction.Headers {
			req.Header.Set(key, val)
		}

		client := &http.Client{}
		resp, httpErr := client.Do(req)
		if httpErr != nil {
			err = fmt.Errorf("failed to execute HTTP request: %w", httpErr)
			break
		}
		defer resp.Body.Close()

		bodyBytes, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			err = fmt.Errorf("failed to read HTTP response body: %w", readErr)
			break
		}
		output = string(bodyBytes)

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			err = fmt.Errorf("http request failed with status %s: %s", resp.Status, output)
		}
	default:
		err = fmt.Errorf("unknown action type: %s", t.Action.Type)
	}

	e.logger.LogExecutionResult(t.ID, startTime, err, output)
	return output, err
}

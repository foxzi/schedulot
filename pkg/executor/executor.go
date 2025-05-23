package executor

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"text/template"
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
		// --- Шаблонизация аргументов с поддержкой Parent* ---
		args := make([]string, len(t.Action.Command.Args))
		for i, arg := range t.Action.Command.Args {
			ctx := map[string]string{
				"TaskID":       t.ID,
				"Date":         time.Now().Format("2006-01-02 15:04:05"),
				"Data":         "", // Можно добавить данные, если есть
				"ParentTaskID": t.ParentTaskID,
				"ParentDate":   t.ParentDate,
				"ParentData":   t.ParentData,
			}
			tmpl, _ := template.New("arg").Parse(arg)
			var sb strings.Builder
			_ = tmpl.Execute(&sb, ctx)
			args[i] = sb.String()
		}
		cmd := exec.CommandContext(ctx, t.Action.Command.Path, args...)
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

		// --- Шаблонизация для url, headers, body ---
		tmplCtx := map[string]string{
			"TaskID":       t.ID,
			"Date":         time.Now().Format("2006-01-02 15:04:05"),
			"Data":         "", // Можно добавить данные, если есть
			"ParentTaskID": t.ParentTaskID,
			"ParentDate":   t.ParentDate,
			"ParentData":   t.ParentData,
		}
		// url
		tmpl, _ := template.New("url").Parse(reqAction.URL)
		var urlBuf strings.Builder
		_ = tmpl.Execute(&urlBuf, tmplCtx)
		url := urlBuf.String()
		// body
		var reqBody io.Reader
		if reqAction.Body != "" {
			tmpl, _ := template.New("body").Parse(reqAction.Body)
			var bodyBuf strings.Builder
			_ = tmpl.Execute(&bodyBuf, tmplCtx)
			reqBody = strings.NewReader(bodyBuf.String())
		}
		// headers
		headers := make(map[string]string)
		for k, v := range reqAction.Headers {
			tmpl, _ := template.New("header").Parse(v)
			var hBuf strings.Builder
			_ = tmpl.Execute(&hBuf, tmplCtx)
			headers[k] = hBuf.String()
		}
		req, httpErr := http.NewRequestWithContext(ctx, strings.ToUpper(reqAction.Method), url, reqBody)
		if httpErr != nil {
			err = fmt.Errorf("failed to create HTTP request: %w", httpErr)
			break
		}
		for key, val := range headers {
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

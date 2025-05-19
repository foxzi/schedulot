package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"time"
)

// LogMode type to define logging destination
type LogMode string

const (
	// LogModeFile writes logs to a file.
	LogModeFile LogMode = "file"
	// LogModeStdout writes logs to standard output.
	LogModeStdout LogMode = "stdout"
)

// Logger defines a simple logger for the application.
type Logger struct {
	infoLogger  *log.Logger
	errorLogger *log.Logger
	logMode     LogMode
}

// New creates a new Logger instance.
func New(logOutput string, mode LogMode) (*Logger, error) {
	var output io.Writer

	if mode == LogModeFile {
		file, err := os.OpenFile(logOutput, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
		if err != nil {
			return nil, fmt.Errorf("failed to open log file %s: %w", logOutput, err)
		}
		output = file
	} else if mode == LogModeStdout {
		output = os.Stdout
	} else {
		return nil, fmt.Errorf("invalid log mode: %s", mode)
	}

	infoLogger := log.New(output, "INFO: ", log.Ldate|log.Ltime|log.Lshortfile)
	errorLogger := log.New(output, "ERROR: ", log.Ldate|log.Ltime|log.Lshortfile)

	return &Logger{infoLogger: infoLogger, errorLogger: errorLogger, logMode: mode}, nil
}

// Info logs an informational message.
func (l *Logger) Info(taskID string, message string, args ...interface{}) {
	l.infoLogger.Printf("TaskID: %s - %s", taskID, fmt.Sprintf(message, args...))
}

// Error logs an error message.
func (l *Logger) Error(taskID string, message string, err error, args ...interface{}) {
	if err != nil {
		l.errorLogger.Printf("TaskID: %s - %s: %v", taskID, fmt.Sprintf(message, args...), err)
	} else {
		l.errorLogger.Printf("TaskID: %s - %s", taskID, fmt.Sprintf(message, args...))
	}
}

// LogExecutionResult logs the result of a task execution.
func (l *Logger) LogExecutionResult(taskID string, startTime time.Time, err error, output string) {
	duration := time.Since(startTime)
	status := "SUCCESS"
	if err != nil {
		status = "FAILURE"
	}

	logMessage := fmt.Sprintf(
		"Execution Result - TaskID: %s, Status: %s, Duration: %s, Output: %s",
		taskID, status, duration, output,
	)

	if err != nil {
		l.Error(taskID, logMessage, err)
	} else {
		l.Info(taskID, logMessage)
	}
}

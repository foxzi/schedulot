package file_notifier

import (
	"fmt"
	"os"

	"github.com/piligrim/gotask2/pkg/notification"
)

// FileNotifier sends notifications to a file.
type FileNotifier struct {
	Config notification.FileNotificationConfig
}

// NewFileNotifier creates a new FileNotifier.
func NewFileNotifier(config notification.FileNotificationConfig) *FileNotifier {
	return &FileNotifier{Config: config}
}

// Notify sends a notification to the configured file.
func (fn *FileNotifier) Notify(data map[string]interface{}) error {
	// Extract data from the map, using the correct keys and types
	taskID, ok := data["TaskID"].(string)
	if !ok {
		return fmt.Errorf("TaskID not found or not a string in notification data")
	}

	dateStr, ok := data["Date"].(string)
	if !ok {
		return fmt.Errorf("date not found or not a string in notification data") // Corrected error string casing
	}

	taskOutput, ok := data["Data"].(string) // Assuming "Data" is the key for task output
	if !ok {
		// Allow Data to be missing or not a string, as some tasks might not have string output
		taskOutput = ""
	}

	processedMessage, err := notification.ProcessTemplate(fn.Config.Message, notification.TemplateData{
		TaskID: taskID,
		Date:   dateStr,    // Use the date string from the input data
		Data:   taskOutput, // Use the task output from the input data
	})
	if err != nil {
		return fmt.Errorf("failed to process message template: %w", err)
	}

	flags := os.O_CREATE | os.O_WRONLY
	if fn.Config.Append {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC // Overwrite if not appending
	}

	file, err := os.OpenFile(fn.Config.FilePath, flags, 0660)
	if err != nil {
		return fmt.Errorf("failed to open notification file %s: %w", fn.Config.FilePath, err)
	}
	defer file.Close()

	_, err = fmt.Fprintln(file, processedMessage)
	if err != nil {
		return fmt.Errorf("failed to write notification to file %s: %w", fn.Config.FilePath, err)
	}

	return nil
}

// GetMessageTemplate returns the message template for the file notifier.
func (fn *FileNotifier) GetMessageTemplate() string {
	return fn.Config.Message
}

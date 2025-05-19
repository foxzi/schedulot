package slack_notifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/piligrim/gotask2/pkg/notification"
)

// SlackNotifier sends notifications to Slack.
type SlackNotifier struct {
	Config notification.SlackNotificationConfig
}

// NewSlackNotifier creates a new SlackNotifier.
func NewSlackNotifier(config notification.SlackNotificationConfig) *SlackNotifier {
	return &SlackNotifier{Config: config}
}

// SlackMessagePayload is the structure for the Slack message.
type SlackMessagePayload struct {
	Channel string `json:"channel,omitempty"`
	Text    string `json:"text"`
}

// Notify sends a notification to Slack.
func (sn *SlackNotifier) Notify(data map[string]interface{}) error {
	taskData, _ := data["data"].(string)
	taskID, _ := data["task_id"].(string)

	processedMessage, err := notification.ProcessTemplate(sn.Config.Message, notification.TemplateData{
		TaskID: taskID,
		Date:   time.Now().Format(time.RFC3339),
		Data:   taskData,
	})
	if err != nil {
		return fmt.Errorf("failed to process Slack message template: %w", err)
	}

	payload := SlackMessagePayload{
		Text: processedMessage,
	}
	if sn.Config.Channel != "" {
		payload.Channel = sn.Config.Channel
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal Slack payload: %w", err)
	}

	req, err := http.NewRequest("POST", sn.Config.WebhookURL, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("failed to create Slack request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send Slack notification: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Consider reading resp.Body for more detailed error from Slack
		return fmt.Errorf("failed to send Slack notification, status code: %d", resp.StatusCode)
	}

	return nil
}

// GetMessageTemplate returns the message template for the slack notifier.
func (sn *SlackNotifier) GetMessageTemplate() string {
	return sn.Config.Message
}

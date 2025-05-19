package notification

import (
	"strings"
	"text/template"
)

// NotificationConfig represents the notification configuration in a task.
type NotificationConfig struct {
	Type                 string                     `yaml:"type"`
	OnSuccess            bool                       `yaml:"on_success"`
	OnFailure            bool                       `yaml:"on_failure"`
	FileNotification     FileNotificationConfig     `yaml:"file_notification,omitempty"`
	EmailNotification    EmailNotificationConfig    `yaml:"email_notification,omitempty"`
	SlackNotification    SlackNotificationConfig    `yaml:"slack_notification,omitempty"`
	TelegramNotification TelegramNotificationConfig `yaml:"telegram_notification,omitempty"`
}

// FileNotificationConfig holds configuration for file notifications.
type FileNotificationConfig struct {
	FilePath string `yaml:"file_path"`
	Message  string `yaml:"message"`
	Append   bool   `yaml:"append"`
}

// EmailNotificationConfig holds configuration for email notifications.
type EmailNotificationConfig struct {
	To      []string `yaml:"to"`
	Subject string   `yaml:"subject"`
	Body    string   `yaml:"body"`
	// SMTP server details would go here
}

// SlackNotificationConfig holds configuration for Slack notifications.
type SlackNotificationConfig struct {
	WebhookURL string `yaml:"webhook_url"`
	Channel    string `yaml:"channel"`
	Message    string `yaml:"message"`
}

// TelegramNotificationConfig holds configuration for Telegram notifications.
type TelegramNotificationConfig struct {
	BotToken string `yaml:"bot_token"`
	ChatID   string `yaml:"chat_id"`
	Message  string `yaml:"message"`
}

// Notifier is an interface for sending notifications.
type Notifier interface {
	Notify(data map[string]interface{}) error
	GetMessageTemplate() string
}

// TemplateData holds the data for message templating.
type TemplateData struct {
	TaskID string
	Date   string
	Data   string
	Status string
}

// ProcessTemplate processes a message template with the given data.
func ProcessTemplate(messageTemplate string, data TemplateData) (string, error) {
	tmpl, err := template.New("notification").Parse(messageTemplate)
	if err != nil {
		return "", err
	}

	var processedMessage strings.Builder
	if err := tmpl.Execute(&processedMessage, data); err != nil {
		return "", err
	}
	return processedMessage.String(), nil
}

// Helper function to get the message string based on notification type
func GetMessage(cfg NotificationConfig) string {
	switch cfg.Type {
	case "file":
		return cfg.FileNotification.Message
	case "email":
		return cfg.EmailNotification.Body // Or a specific template field
	case "slack":
		return cfg.SlackNotification.Message
	case "telegram":
		return cfg.TelegramNotification.Message
	default:
		return ""
	}
}

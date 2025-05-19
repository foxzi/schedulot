package email_notifier

import (
	"fmt"
	"net/smtp"
	"time"

	"github.com/piligrim/gotask2/pkg/notification"
)

// EmailNotifier sends notifications via email.
type EmailNotifier struct {
	Config notification.EmailNotificationConfig
	// Add SMTP server details here, e.g., Host, Port, Username, Password
	SMTPHost     string
	SMTPPort     string
	SMTPUsername string
	SMTPPassword string
}

// NewEmailNotifier creates a new EmailNotifier.
func NewEmailNotifier(config notification.EmailNotificationConfig, smtpHost, smtpPort, smtpUsername, smtpPassword string) *EmailNotifier {
	return &EmailNotifier{
		Config:       config,
		SMTPHost:     smtpHost,
		SMTPPort:     smtpPort,
		SMTPUsername: smtpUsername,
		SMTPPassword: smtpPassword,
	}
}

// Notify sends an email notification.
func (en *EmailNotifier) Notify(data map[string]interface{}) error {
	taskData, _ := data["data"].(string)
	taskID, _ := data["task_id"].(string)

	processedBody, err := notification.ProcessTemplate(en.Config.Body, notification.TemplateData{
		TaskID: taskID,
		Date:   time.Now().Format(time.RFC3339),
		Data:   taskData,
	})
	if err != nil {
		return fmt.Errorf("failed to process email body template: %w", err)
	}

	// Construct the email message
	// Note: This is a very basic email construction. For production, consider using a library for MIME encoding, attachments etc.
	msg := []byte("To: " + en.Config.To[0] + "\r\n" + // Assuming at least one recipient
		"Subject: " + en.Config.Subject + "\r\n" +
		"\r\n" +
		processedBody + "\r\n")

	// SMTP server authentication
	auth := smtp.PlainAuth("", en.SMTPUsername, en.SMTPPassword, en.SMTPHost)

	// Sending the email
	addr := en.SMTPHost + ":" + en.SMTPPort
	err = smtp.SendMail(addr, auth, en.SMTPUsername, en.Config.To, msg)
	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	return nil
}

// GetMessageTemplate returns the message template for the email notifier.
func (en *EmailNotifier) GetMessageTemplate() string {
	return en.Config.Body
}

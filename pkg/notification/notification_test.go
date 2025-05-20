package notification_test

import (
	"testing"

	"github.com/piligrim/gotask2/pkg/notification"
)

func TestProcessTemplate(t *testing.T) {
	tmpl := "Task: {{.TaskID}}, Status: {{.Status}}"
	data := notification.TemplateData{
		TaskID: "t1",
		Status: "success",
	}
	msg, err := notification.ProcessTemplate(tmpl, data)
	if err != nil {
		t.Fatalf("ProcessTemplate error: %v", err)
	}
	if msg != "Task: t1, Status: success" {
		t.Errorf("unexpected template result: %s", msg)
	}
}

func TestGetMessage(t *testing.T) {
	cfg := notification.NotificationConfig{
		Type: "file",
		FileNotification: notification.FileNotificationConfig{
			Message: "Hello!",
		},
	}
	if notification.GetMessage(cfg) != "Hello!" {
		t.Errorf("GetMessage returned wrong value")
	}
}

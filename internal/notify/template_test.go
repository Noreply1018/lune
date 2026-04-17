package notify

import (
	"strings"
	"testing"
	"time"
)

func TestRenderNotificationUsesProvidedTemplates(t *testing.T) {
	n := Notification{
		Event:    "account_error",
		Severity: "critical",
		Title:    "Broken",
		Message:  "boom",
	}
	rendered, err := RenderNotification(n, "t={{ .Title }}", "b={{ .Message }}")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if rendered.Title != "t=Broken" || rendered.Body != "b=boom" {
		t.Fatalf("unexpected render: %+v", rendered)
	}
}

func TestRenderNotificationFallsBackToRawValuesWhenTemplatesEmpty(t *testing.T) {
	n := Notification{
		Event:    "account_error",
		Severity: "critical",
		Title:    "Broken",
		Message:  "boom",
	}
	rendered, err := RenderNotification(n, "", "")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if rendered.Title != "Broken" || rendered.Body != "boom" {
		t.Fatalf("expected raw fallback, got %+v", rendered)
	}
}

func TestRenderNotificationSupportsTriggeredAtAlias(t *testing.T) {
	ts := time.Date(2026, 4, 18, 0, 0, 0, 0, time.UTC)
	n := Notification{
		Event:     "account_error",
		Severity:  "critical",
		Title:     "Broken",
		Message:   "boom",
		Timestamp: ts,
	}
	rendered, err := RenderNotification(n, "at={{ .TriggeredAt }}", "when={{ .Timestamp }}")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.HasPrefix(rendered.Title, "at=") || !strings.HasPrefix(rendered.Body, "when=") {
		t.Fatalf("unexpected prefix in rendered: %+v", rendered)
	}
	tAt := strings.TrimPrefix(rendered.Title, "at=")
	tTs := strings.TrimPrefix(rendered.Body, "when=")
	if tAt != tTs {
		t.Fatalf("TriggeredAt and Timestamp should render identically, got at=%q ts=%q", tAt, tTs)
	}
}

func TestRenderNotificationPropagatesTemplateError(t *testing.T) {
	n := Notification{Event: "e", Severity: "s", Title: "t", Message: "m"}
	if _, err := RenderNotification(n, "{{ .Missing", "x"); err == nil {
		t.Fatalf("expected parse error")
	}
}

package notify

import (
	"bytes"
	"fmt"
	"text/template"
)

type RenderedMessage struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// RenderNotification renders the given title/body Go templates against the
// notification's fields. Callers pass the per-subscription templates loaded
// from notification_subscriptions; empty templates fall back to the raw
// Title/Message on the notification itself so internal callers never produce
// an empty message.
func RenderNotification(n Notification, titleTpl, bodyTpl string) (RenderedMessage, error) {
	if titleTpl == "" {
		titleTpl = n.Title
	}
	if bodyTpl == "" {
		bodyTpl = n.Message
	}
	title, err := renderTemplate(titleTpl, n)
	if err != nil {
		return RenderedMessage{}, fmt.Errorf("render title: %w", err)
	}
	body, err := renderTemplate(bodyTpl, n)
	if err != nil {
		return RenderedMessage{}, fmt.Errorf("render body: %w", err)
	}
	return RenderedMessage{Title: title, Body: body}, nil
}

func renderTemplate(tpl string, n Notification) (string, error) {
	data := map[string]any{
		"Event":       n.Event,
		"Severity":    n.Severity,
		"Title":       n.Title,
		"Message":     n.Message,
		"Timestamp":   n.Timestamp,
		"TriggeredAt": n.Timestamp,
		"Vars":        n.Vars,
		"Source":      n.Source,
	}
	for key, value := range n.Vars {
		data[key] = value
	}

	t, err := template.New("notify").Option("missingkey=zero").Parse(tpl)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

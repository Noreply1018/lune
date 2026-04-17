package notify

import (
	"bytes"
	"fmt"
	"text/template"

	"lune/internal/store"
)

type RenderedMessage struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

var defaultTemplates = map[string]RenderedMessage{
	"account_expiring": {
		Title: `Lune · {{ .Title }}`,
		Body:  `{{ .Message }}`,
	},
	"account_error": {
		Title: `Lune · {{ .Title }}`,
		Body:  `{{ .Message }}`,
	},
	"cpa_service_error": {
		Title: `Lune · {{ .Title }}`,
		Body:  `{{ .Message }}`,
	},
	"test": {
		Title: `Lune 测试消息`,
		Body:  `这是一条用于验证渠道可达性的真实消息，可忽略。`,
	},
}

func RenderNotification(n Notification, titleTpl, bodyTpl string) (RenderedMessage, error) {
	base := defaultTemplates[n.Event]
	if base.Title == "" {
		base = RenderedMessage{Title: n.Title, Body: n.Message}
	}
	if titleTpl == "" {
		titleTpl = base.Title
	}
	if bodyTpl == "" {
		bodyTpl = base.Body
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

func RenderChannelNotification(n Notification, channel store.NotificationChannel) (RenderedMessage, error) {
	titleTpl, bodyTpl := ResolveChannelTemplates(channel, n.Event)
	return RenderNotification(n, titleTpl, bodyTpl)
}

func ResolveChannelTemplates(channel store.NotificationChannel, event string) (string, string) {
	for _, sub := range channel.Subscriptions {
		if sub.Event == event {
			return firstNonEmpty(sub.TitleTemplate, channel.TitleTemplate), firstNonEmpty(sub.BodyTemplate, channel.BodyTemplate)
		}
	}
	for _, sub := range channel.Subscriptions {
		if sub.Event == "*" {
			return firstNonEmpty(sub.TitleTemplate, channel.TitleTemplate), firstNonEmpty(sub.BodyTemplate, channel.BodyTemplate)
		}
	}
	return channel.TitleTemplate, channel.BodyTemplate
}

func renderTemplate(tpl string, n Notification) (string, error) {
	data := map[string]any{
		"Event":     n.Event,
		"Severity":  n.Severity,
		"Title":     n.Title,
		"Message":   n.Message,
		"Timestamp": n.Timestamp,
		"Vars":      n.Vars,
		"Source":    n.Source,
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

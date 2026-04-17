package notify

import (
	"testing"

	"lune/internal/store"
)

func TestRenderChannelNotificationPrefersSubscriptionOverrideThenChannelThenDefault(t *testing.T) {
	n := Notification{
		Event:    "account_error",
		Severity: "critical",
		Title:    "Broken",
		Message:  "boom",
	}

	channel := store.NotificationChannel{
		TitleTemplate: "channel {{ .Title }}",
		BodyTemplate:  "channel {{ .Message }}",
		Subscriptions: []store.NotificationSubscription{
			{Event: "account_error", TitleTemplate: "subscription {{ .Title }}", BodyTemplate: "subscription {{ .Message }}"},
		},
	}
	rendered, err := RenderChannelNotification(n, channel)
	if err != nil {
		t.Fatalf("render with subscription override: %v", err)
	}
	if rendered.Title != "subscription Broken" || rendered.Body != "subscription boom" {
		t.Fatalf("unexpected subscription override render: %+v", rendered)
	}

	channel.Subscriptions = nil
	rendered, err = RenderChannelNotification(n, channel)
	if err != nil {
		t.Fatalf("render with channel default: %v", err)
	}
	if rendered.Title != "channel Broken" || rendered.Body != "channel boom" {
		t.Fatalf("unexpected channel render: %+v", rendered)
	}

	channel.TitleTemplate = ""
	channel.BodyTemplate = ""
	rendered, err = RenderChannelNotification(n, channel)
	if err != nil {
		t.Fatalf("render with built-in default: %v", err)
	}
	if rendered.Title != "Lune · Broken" || rendered.Body != "boom" {
		t.Fatalf("unexpected built-in render: %+v", rendered)
	}
}

func TestRenderChannelNotificationFallsBackToWildcardSubscriptionOverride(t *testing.T) {
	n := Notification{
		Event:    "account_error",
		Severity: "critical",
		Title:    "Broken",
		Message:  "boom",
	}
	channel := store.NotificationChannel{
		TitleTemplate: "channel {{ .Title }}",
		BodyTemplate:  "channel {{ .Message }}",
		Subscriptions: []store.NotificationSubscription{
			{Event: "*", TitleTemplate: "wild {{ .Title }}", BodyTemplate: "wild {{ .Message }}"},
		},
	}
	rendered, err := RenderChannelNotification(n, channel)
	if err != nil {
		t.Fatalf("render with wildcard override: %v", err)
	}
	if rendered.Title != "wild Broken" || rendered.Body != "wild boom" {
		t.Fatalf("unexpected wildcard render: %+v", rendered)
	}
}

func TestRenderChannelNotificationPrefersExactSubscriptionOverWildcard(t *testing.T) {
	n := Notification{
		Event:    "account_error",
		Severity: "critical",
		Title:    "Broken",
		Message:  "boom",
	}
	channel := store.NotificationChannel{
		TitleTemplate: "channel {{ .Title }}",
		BodyTemplate:  "channel {{ .Message }}",
		Subscriptions: []store.NotificationSubscription{
			{Event: "*", TitleTemplate: "wild {{ .Title }}", BodyTemplate: "wild {{ .Message }}"},
			{Event: "account_error", TitleTemplate: "exact {{ .Title }}", BodyTemplate: "exact {{ .Message }}"},
		},
	}

	rendered, err := RenderChannelNotification(n, channel)
	if err != nil {
		t.Fatalf("render with exact + wildcard override: %v", err)
	}
	if rendered.Title != "exact Broken" || rendered.Body != "exact boom" {
		t.Fatalf("expected exact override to win over wildcard, got %+v", rendered)
	}
}

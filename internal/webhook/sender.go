package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

type Payload struct {
	Event     string `json:"event"`
	Severity  string `json:"severity"`
	Title     string `json:"title"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
}

type Sender struct {
	client *http.Client
}

func NewSender() *Sender {
	return &Sender{
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *Sender) Send(ctx context.Context, webhookURL string, payload Payload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		slog.Error("marshal webhook payload", "err", err)
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		slog.Error("build webhook request", "url", webhookURL, "err", err)
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		slog.Error("send webhook", "url", webhookURL, "err", err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		err = fmt.Errorf("unexpected webhook status: %d", resp.StatusCode)
		slog.Error("send webhook", "url", webhookURL, "status", resp.StatusCode, "err", err)
		return err
	}

	return nil
}

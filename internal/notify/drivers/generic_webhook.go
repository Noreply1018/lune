package drivers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"lune/internal/notify"
)

type GenericWebhookDriver struct {
	client *http.Client
}

func NewGenericWebhookDriver() *GenericWebhookDriver {
	return &GenericWebhookDriver{client: &http.Client{Timeout: 10 * time.Second}}
}

func (d *GenericWebhookDriver) Type() string { return "generic_webhook" }

func (d *GenericWebhookDriver) SecretFields() []string { return nil }

func (d *GenericWebhookDriver) DocsURL() string { return "" }

func (d *GenericWebhookDriver) ValidateConfig(raw json.RawMessage) error {
	var cfg struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return err
	}
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(cfg.URL)), "http://") &&
		!strings.HasPrefix(strings.ToLower(strings.TrimSpace(cfg.URL)), "https://") {
		return fmt.Errorf("url must start with http:// or https://")
	}
	return nil
}

func (d *GenericWebhookDriver) Send(ctx context.Context, n notify.Notification, runtime notify.ChannelRuntime) (notify.Result, error) {
	var cfg struct {
		URL     string            `json:"url"`
		Headers map[string]string `json:"headers"`
	}
	if err := json.Unmarshal(runtime.Config, &cfg); err != nil {
		return notify.Result{}, err
	}
	rendered, err := notify.RenderNotification(n, runtime.TitleTpl, runtime.BodyTpl)
	if err != nil {
		return notify.Result{}, err
	}
	body, err := json.Marshal(map[string]any{
		"event":     n.Event,
		"severity":  n.Severity,
		"title":     rendered.Title,
		"message":   rendered.Body,
		"timestamp": n.Timestamp.UTC().Format(time.RFC3339),
		"vars":      n.Vars,
		"triggered": runtime.Triggered,
	})
	if err != nil {
		return notify.Result{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.URL, bytes.NewReader(body))
	if err != nil {
		return notify.Result{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range cfg.Headers {
		if strings.EqualFold(key, "Content-Type") {
			continue
		}
		req.Header.Set(key, value)
	}
	start := time.Now()
	resp, err := d.client.Do(req)
	if err != nil {
		return notify.Result{}, err
	}
	defer resp.Body.Close()
	excerptBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	result := notify.Result{
		OK:              resp.StatusCode >= 200 && resp.StatusCode < 300,
		UpstreamCode:    fmt.Sprintf("http %d", resp.StatusCode),
		UpstreamMessage: http.StatusText(resp.StatusCode),
		LatencyMS:       time.Since(start).Milliseconds(),
		ResponseExcerpt: string(excerptBytes),
	}
	if !result.OK {
		return result, fmt.Errorf("unexpected webhook status: %d", resp.StatusCode)
	}
	return result, nil
}

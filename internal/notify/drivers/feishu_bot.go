package drivers

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"lune/internal/notify"
)

type FeishuBotDriver struct {
	client *http.Client
}

func NewFeishuBotDriver() *FeishuBotDriver {
	return &FeishuBotDriver{client: &http.Client{Timeout: 10 * time.Second}}
}

func (d *FeishuBotDriver) Type() string { return "feishu_bot" }

func (d *FeishuBotDriver) SecretFields() []string { return []string{"secret"} }

func (d *FeishuBotDriver) DocsURL() string {
	return "https://open.feishu.cn/document/client-docs/bot-v3/add-custom-bot"
}

func (d *FeishuBotDriver) ValidateConfig(raw json.RawMessage) error {
	var cfg struct {
		WebhookURL string `json:"webhook_url"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return err
	}
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(cfg.WebhookURL)), "http") {
		return fmt.Errorf("webhook_url is required")
	}
	return nil
}

func (d *FeishuBotDriver) Send(ctx context.Context, n notify.Notification, runtime notify.ChannelRuntime) (notify.Result, error) {
	var cfg struct {
		WebhookURL string `json:"webhook_url"`
		Secret     string `json:"secret"`
		Format     string `json:"format"`
	}
	if err := json.Unmarshal(runtime.Config, &cfg); err != nil {
		return notify.Result{}, err
	}
	if cfg.Format == "" {
		cfg.Format = "post"
	}
	rendered, err := notify.RenderNotification(n, runtime.TitleTpl, runtime.BodyTpl)
	if err != nil {
		return notify.Result{}, err
	}
	payload := map[string]any{}
	if cfg.Format == "text" {
		payload["msg_type"] = "text"
		payload["content"] = map[string]any{
			"text": rendered.Title + "\n" + rendered.Body,
		}
	} else {
		payload["msg_type"] = "post"
		payload["content"] = map[string]any{
			"post": map[string]any{
				"zh_cn": map[string]any{
					"title": rendered.Title,
					"content": [][]map[string]string{{
						{"tag": "text", "text": rendered.Body},
					}},
				},
			},
		}
	}
	if strings.TrimSpace(cfg.Secret) != "" {
		timestamp := fmt.Sprintf("%d", time.Now().Unix())
		payload["timestamp"] = timestamp
		payload["sign"] = signFeishu(timestamp, cfg.Secret)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return notify.Result{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return notify.Result{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	start := time.Now()
	resp, err := d.client.Do(req)
	if err != nil {
		return notify.Result{}, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	result := notify.Result{
		LatencyMS:       time.Since(start).Milliseconds(),
		ResponseExcerpt: string(respBody),
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		result.UpstreamCode = fmt.Sprintf("http %d", resp.StatusCode)
		result.UpstreamMessage = http.StatusText(resp.StatusCode)
		return result, fmt.Errorf("unexpected webhook status: %d", resp.StatusCode)
	}
	var parsed struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return result, err
	}
	result.UpstreamCode = fmt.Sprintf("code=%d", parsed.Code)
	result.UpstreamMessage = parsed.Msg
	result.OK = parsed.Code == 0
	if !result.OK {
		return result, fmt.Errorf("feishu_bot failed: %s", parsed.Msg)
	}
	return result, nil
}

func signFeishu(timestamp, secret string) string {
	mac := hmac.New(sha256.New, []byte(timestamp+"\n"+secret))
	mac.Write([]byte{})
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

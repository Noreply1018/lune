package drivers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"lune/internal/notify"
)

type WeChatWorkBotDriver struct {
	client *http.Client
}

func NewWeChatWorkBotDriver() *WeChatWorkBotDriver {
	return &WeChatWorkBotDriver{client: &http.Client{Timeout: 10 * time.Second}}
}

func (d *WeChatWorkBotDriver) Type() string { return "wechat_work_bot" }

func (d *WeChatWorkBotDriver) SecretFields() []string { return nil }

func (d *WeChatWorkBotDriver) DocsURL() string {
	return "https://developer.work.weixin.qq.com/document/path/91770"
}

func (d *WeChatWorkBotDriver) ValidateConfig(raw json.RawMessage) error {
	var cfg struct {
		WebhookURL string `json:"webhook_url"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return err
	}
	normalized := strings.TrimSpace(cfg.WebhookURL)
	parsed, err := url.Parse(normalized)
	if err != nil || parsed == nil {
		return fmt.Errorf("webhook_url must be a valid WeCom bot webhook URL")
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Host))
	if parsed.Scheme != "https" || host != "qyapi.weixin.qq.com" || parsed.Path != "/cgi-bin/webhook/send" || strings.TrimSpace(parsed.Query().Get("key")) == "" {
		return fmt.Errorf("webhook_url must be a valid WeCom bot webhook URL")
	}
	return nil
}

func (d *WeChatWorkBotDriver) Send(ctx context.Context, n notify.Notification, runtime notify.ChannelRuntime) (notify.Result, error) {
	var cfg struct {
		WebhookURL        string   `json:"webhook_url"`
		MentionList       []string `json:"mention_list"`
		MentionMobileList []string `json:"mention_mobile_list"`
		Format            string   `json:"format"`
	}
	if err := json.Unmarshal(runtime.Config, &cfg); err != nil {
		return notify.Result{}, err
	}
	if cfg.Format == "" {
		cfg.Format = "markdown"
	}
	rendered := runtime.Rendered
	if rendered == nil {
		item, err := notify.RenderNotification(n, runtime.TitleTpl, runtime.BodyTpl)
		if err != nil {
			return notify.Result{}, err
		}
		rendered = &item
	}
	payload := map[string]any{}
	if cfg.Format == "text" {
		payload["msgtype"] = "text"
		payload["text"] = map[string]any{
			"content":               rendered.Title + "\n" + rendered.Body,
			"mentioned_list":        cfg.MentionList,
			"mentioned_mobile_list": cfg.MentionMobileList,
		}
	} else {
		mentions := make([]string, 0, len(cfg.MentionList)+len(cfg.MentionMobileList))
		for _, item := range cfg.MentionList {
			item = strings.TrimSpace(item)
			if item != "" {
				mentions = append(mentions, "<@"+item+">")
			}
		}
		for _, item := range cfg.MentionMobileList {
			item = strings.TrimSpace(item)
			if item != "" {
				mentions = append(mentions, "<@"+item+">")
			}
		}
		content := fmt.Sprintf("## %s\n\n%s", rendered.Title, rendered.Body)
		if len(mentions) > 0 {
			content += "\n\n" + strings.Join(mentions, " ")
		}
		payload["msgtype"] = "markdown"
		payload["markdown"] = map[string]any{
			"content": content,
		}
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
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		result.UpstreamCode = "parse_error"
		result.UpstreamMessage = err.Error()
		return result, err
	}
	result.UpstreamCode = fmt.Sprintf("errcode=%d", parsed.ErrCode)
	result.UpstreamMessage = parsed.ErrMsg
	result.OK = parsed.ErrCode == 0
	if !result.OK {
		return result, fmt.Errorf("wechat_work_bot failed: %s", parsed.ErrMsg)
	}
	return result, nil
}

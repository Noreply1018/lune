package drivers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestSignFeishuMatchesOfficialAlgorithm(t *testing.T) {
	timestamp := "1710000000"
	secret := "test-secret"
	stringToSign := timestamp + "\n" + secret
	mac := hmac.New(sha256.New, []byte(stringToSign))
	want := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	if got := signFeishu(timestamp, secret); got != want {
		t.Fatalf("unexpected feishu signature: got %q want %q", got, want)
	}
}

func TestFeishuValidateConfigRejectsMissingHost(t *testing.T) {
	driver := NewFeishuBotDriver()
	if err := driver.ValidateConfig(json.RawMessage(`{"webhook_url":"https://"}`)); err == nil {
		t.Fatalf("expected invalid Feishu webhook URL to be rejected")
	}
	if err := driver.ValidateConfig(json.RawMessage(`{"webhook_url":":"}`)); err == nil {
		t.Fatalf("expected malformed Feishu webhook URL to be rejected")
	}
	if err := driver.ValidateConfig(json.RawMessage(`{"webhook_url":"https://open.feishu.cn/open-apis/bot/v2/hook/abc"}`)); err != nil {
		t.Fatalf("expected valid Feishu webhook URL, got %v", err)
	}
	if err := driver.ValidateConfig(json.RawMessage(`{"webhook_url":"https://example.com/hook"}`)); err == nil {
		t.Fatalf("expected non-Feishu host to be rejected")
	}
}

func TestWeComValidateConfigRequiresNonEmptyKey(t *testing.T) {
	driver := NewWeChatWorkBotDriver()
	if err := driver.ValidateConfig(json.RawMessage(`{"webhook_url":"https://qyapi.weixin.qq.com/cgi-bin/webhook/send"}`)); err == nil {
		t.Fatalf("expected missing key to be rejected")
	}
	if err := driver.ValidateConfig(json.RawMessage(`{"webhook_url":":"}`)); err == nil {
		t.Fatalf("expected malformed WeCom webhook URL to be rejected")
	}
	if err := driver.ValidateConfig(json.RawMessage(`{"webhook_url":"https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=abc"}`)); err != nil {
		t.Fatalf("expected valid WeCom webhook URL, got %v", err)
	}
	if err := driver.ValidateConfig(json.RawMessage(`{"webhook_url":"https://example.com/x?key=abc"}`)); err == nil {
		t.Fatalf("expected non-WeCom host to be rejected")
	}
}

func TestEmailValidateConfigRejectsEmptyTLSMode(t *testing.T) {
	driver := NewEmailSMTPDriver()
	err := driver.ValidateConfig(json.RawMessage(`{
		"host":"smtp.example.com",
		"port":587,
		"from":"bot@example.com",
		"to":["ops@example.com"],
		"tls_mode":""
	}`))
	if err == nil {
		t.Fatalf("expected empty tls_mode to be rejected")
	}
}

func TestNormalizeTLSModeDoesNotAcceptEmpty(t *testing.T) {
	if got := normalizeTLSMode(""); got != "" {
		t.Fatalf("expected empty tls mode to stay empty, got %q", got)
	}
	if got := normalizeTLSMode("STARTTLS"); got != "starttls" {
		t.Fatalf("expected starttls normalization, got %q", got)
	}
}

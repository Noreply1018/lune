package drivers

import (
	"encoding/json"
	"testing"
)

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

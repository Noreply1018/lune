package cpa

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestParseAccountInfoFromTokensPrefersIDToken(t *testing.T) {
	idToken := testJWT(map[string]any{
		"email": "user@example.com",
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_plan_type":  "plus",
			"chatgpt_account_id": "acct_123",
		},
	})

	info, err := ParseAccountInfoFromTokens(idToken, "opaque-access-token")
	if err != nil {
		t.Fatalf("ParseAccountInfoFromTokens() error = %v", err)
	}
	if info.Email != "user@example.com" {
		t.Fatalf("Email = %q, want %q", info.Email, "user@example.com")
	}
	if info.PlanType != "plus" {
		t.Fatalf("PlanType = %q, want %q", info.PlanType, "plus")
	}
	if info.AccountID != "acct_123" {
		t.Fatalf("AccountID = %q, want %q", info.AccountID, "acct_123")
	}
}

func TestParseAccountInfoFromTokensFallsBackToAccessToken(t *testing.T) {
	accessToken := testJWT(map[string]any{
		"https://api.openai.com/profile": map[string]any{
			"email": "fallback@example.com",
		},
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_plan_type":  "pro",
			"chatgpt_account_id": "acct_456",
		},
	})

	info, err := ParseAccountInfoFromTokens("", accessToken)
	if err != nil {
		t.Fatalf("ParseAccountInfoFromTokens() error = %v", err)
	}
	if info.Email != "fallback@example.com" {
		t.Fatalf("Email = %q, want %q", info.Email, "fallback@example.com")
	}
	if info.PlanType != "pro" {
		t.Fatalf("PlanType = %q, want %q", info.PlanType, "pro")
	}
	if info.AccountID != "acct_456" {
		t.Fatalf("AccountID = %q, want %q", info.AccountID, "acct_456")
	}
}

func testJWT(claims map[string]any) string {
	headerJSON, _ := json.Marshal(map[string]any{"alg": "none", "typ": "JWT"})
	payloadJSON, _ := json.Marshal(claims)
	return base64.RawURLEncoding.EncodeToString(headerJSON) + "." +
		base64.RawURLEncoding.EncodeToString(payloadJSON) + ".sig"
}

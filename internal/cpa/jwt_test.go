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

func TestParseTokenSuccessReadsAuthorizationCodePayload(t *testing.T) {
	body := []byte(`{
		"data": {
			"authorization_code": "auth_code_123",
			"code_verifier": "verifier_456"
		}
	}`)

	resp, err := parseTokenSuccess(body)
	if err != nil {
		t.Fatalf("parseTokenSuccess() error = %v", err)
	}
	if resp.AuthorizationCode != "auth_code_123" {
		t.Fatalf("AuthorizationCode = %q, want %q", resp.AuthorizationCode, "auth_code_123")
	}
	if resp.CodeVerifier != "verifier_456" {
		t.Fatalf("CodeVerifier = %q, want %q", resp.CodeVerifier, "verifier_456")
	}
}

func TestParseTokenSuccessReadsDirectTokens(t *testing.T) {
	body := []byte(`{
		"access_token": "at_123",
		"refresh_token": "rt_456",
		"id_token": "it_789",
		"expires_in": 3600,
		"token_type": "Bearer"
	}`)

	resp, err := parseTokenSuccess(body)
	if err != nil {
		t.Fatalf("parseTokenSuccess() error = %v", err)
	}
	if resp.AccessToken != "at_123" || resp.RefreshToken != "rt_456" || resp.IDToken != "it_789" {
		t.Fatalf("unexpected token payload: %+v", resp)
	}
	if resp.ExpiresIn != 3600 {
		t.Fatalf("ExpiresIn = %d, want %d", resp.ExpiresIn, 3600)
	}
}

func testJWT(claims map[string]any) string {
	headerJSON, _ := json.Marshal(map[string]any{"alg": "none", "typ": "JWT"})
	payloadJSON, _ := json.Marshal(claims)
	return base64.RawURLEncoding.EncodeToString(headerJSON) + "." +
		base64.RawURLEncoding.EncodeToString(payloadJSON) + ".sig"
}

package cpa

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	deviceCodeURL      = "https://auth.openai.com/api/accounts/deviceauth/usercode"
	tokenURL           = "https://auth.openai.com/api/accounts/deviceauth/token"
	oauthTokenURL      = "https://auth.openai.com/oauth/token"
	clientID           = "app_EMoamEEZ73f0CkXaXp7hrann"
	scope              = "openid email profile offline_access"
	defaultVerifyURI   = "https://auth.openai.com/codex/device"
	defaultRedirectURI = "http://localhost:1455/auth/callback"
	defaultExpiresIn   = 900 // 15 minutes
	defaultPollSeconds = 5
)

var (
	ErrAuthorizationPending = errors.New("authorization_pending")
	ErrSlowDown             = errors.New("slow_down")
	ErrExpiredToken         = errors.New("expired_token")
	ErrAccessDenied         = errors.New("access_denied")
)

type DeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"-"`
	Interval        int    `json:"-"`
	ExpiresIn       int    `json:"-"`
}

type TokenResponse struct {
	AccessToken       string `json:"access_token"`
	RefreshToken      string `json:"refresh_token"`
	IDToken           string `json:"id_token"`
	ExpiresIn         int    `json:"expires_in"`
	TokenType         string `json:"token_type"`
	AuthorizationCode string `json:"authorization_code"`
	CodeVerifier      string `json:"code_verifier"`
}

func RequestDeviceCode(ctx context.Context) (*DeviceCodeResponse, error) {
	form := url.Values{
		"client_id": {clientID},
		"scope":     {scope},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, deviceCodeURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("device code request failed: HTTP %d", resp.StatusCode)
	}

	// Parse with raw fields since format varies
	var raw struct {
		DeviceCode              string          `json:"device_code"`
		DeviceAuthID            string          `json:"device_auth_id"`
		UserCode                string          `json:"user_code"`
		VerificationURI         string          `json:"verification_uri"`
		VerificationURIComplete string          `json:"verification_uri_complete"`
		Interval                json.RawMessage `json:"interval"`
		ExpiresIn               json.RawMessage `json:"expires_in"`
		ExpiresAt               string          `json:"expires_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}

	dcr := &DeviceCodeResponse{
		DeviceCode:      firstNonEmpty(raw.DeviceCode, raw.DeviceAuthID),
		UserCode:        raw.UserCode,
		VerificationURI: firstNonEmpty(raw.VerificationURIComplete, raw.VerificationURI, defaultVerifyURI),
		Interval:        defaultPollSeconds,
		ExpiresIn:       defaultExpiresIn,
	}

	// Parse interval (may be string or number)
	if len(raw.Interval) > 0 {
		if n := parseFlexInt(raw.Interval); n > 0 {
			dcr.Interval = n
		}
	}

	// Parse expiry: prefer expires_in (seconds), fallback to expires_at (ISO timestamp)
	if len(raw.ExpiresIn) > 0 {
		if n := parseFlexInt(raw.ExpiresIn); n > 0 {
			dcr.ExpiresIn = n
		}
	} else if raw.ExpiresAt != "" {
		if t, err := time.Parse(time.RFC3339, raw.ExpiresAt); err == nil {
			secs := int(time.Until(t).Seconds())
			if secs > 0 {
				dcr.ExpiresIn = secs
			}
		}
	}

	return dcr, nil
}

func PollForToken(ctx context.Context, deviceCode, userCode string) (*TokenResponse, error) {
	form := url.Values{
		"grant_type": {"urn:ietf:params:oauth:grant-type:device_code"},
		"client_id":  {clientID},
	}
	if strings.TrimSpace(deviceCode) != "" {
		form.Set("device_code", deviceCode)
		form.Set("device_auth_id", deviceCode)
	}
	if strings.TrimSpace(userCode) != "" {
		form.Set("user_code", userCode)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		tr, err := parseTokenSuccess(body)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(tr.AccessToken) != "" || strings.TrimSpace(tr.IDToken) != "" {
			return tr, nil
		}
		if strings.TrimSpace(tr.AuthorizationCode) == "" {
			return nil, fmt.Errorf("token poll succeeded but no tokens or authorization_code were returned")
		}
		if strings.TrimSpace(tr.CodeVerifier) == "" {
			return nil, fmt.Errorf("token poll succeeded but code_verifier is missing")
		}
		return ExchangeAuthorizationCode(ctx, tr.AuthorizationCode, tr.CodeVerifier)
	}

	// Parse error response — may be flat or nested
	var errResp struct {
		Error   any    `json:"error"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	json.NewDecoder(resp.Body).Decode(&errResp)

	errCode := errResp.Code
	if errCode == "" {
		switch e := errResp.Error.(type) {
		case string:
			errCode = e
		case map[string]any:
			if code, ok := e["code"].(string); ok {
				errCode = code
			} else if msg, ok := e["message"].(string); ok {
				errCode = msg
			}
		}
	}

	switch errCode {
	case "authorization_pending", "deviceauth_authorization_unknown":
		return nil, ErrAuthorizationPending
	case "slow_down":
		return nil, ErrSlowDown
	case "expired_token":
		return nil, ErrExpiredToken
	case "access_denied":
		return nil, ErrAccessDenied
	default:
		return nil, fmt.Errorf("token poll error: %s (HTTP %d)", errCode, resp.StatusCode)
	}
}

func ExchangeAuthorizationCode(ctx context.Context, authorizationCode, codeVerifier string) (*TokenResponse, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {clientID},
		"redirect_uri":  {defaultRedirectURI},
		"code":          {authorizationCode},
		"code_verifier": {codeVerifier},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, oauthTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("oauth token exchange failed: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	tr, err := parseTokenSuccess(body)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(tr.AccessToken) == "" && strings.TrimSpace(tr.IDToken) == "" {
		return nil, fmt.Errorf("oauth token exchange succeeded but token payload is empty")
	}
	return tr, nil
}

func parseTokenSuccess(body []byte) (*TokenResponse, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode token response: %w", err)
	}

	flat := flattenRawObject(raw)
	tr := &TokenResponse{
		AccessToken:       rawString(flat["access_token"]),
		RefreshToken:      rawString(flat["refresh_token"]),
		IDToken:           rawString(flat["id_token"]),
		TokenType:         rawString(flat["token_type"]),
		AuthorizationCode: firstNonEmpty(rawString(flat["authorization_code"]), rawString(flat["code"])),
		CodeVerifier:      rawString(flat["code_verifier"]),
		ExpiresIn:         rawInt(flat["expires_in"]),
	}
	return tr, nil
}

func flattenRawObject(raw map[string]json.RawMessage) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(raw))
	for k, v := range raw {
		out[k] = v
	}
	for _, key := range []string{"data", "result", "tokens"} {
		nested, ok := raw[key]
		if !ok {
			continue
		}
		var nestedMap map[string]json.RawMessage
		if json.Unmarshal(nested, &nestedMap) == nil {
			for k, v := range nestedMap {
				if _, exists := out[k]; !exists {
					out[k] = v
				}
			}
		}
	}
	return out
}

// parseFlexInt parses a JSON value that may be a number or a string-encoded number.
func parseFlexInt(raw json.RawMessage) int {
	var n int
	if json.Unmarshal(raw, &n) == nil {
		return n
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		var v int
		if _, err := fmt.Sscanf(s, "%d", &v); err == nil {
			return v
		}
	}
	return 0
}

func rawString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return ""
}

func rawInt(raw json.RawMessage) int {
	return parseFlexInt(raw)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

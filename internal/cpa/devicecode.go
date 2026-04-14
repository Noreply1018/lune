package cpa

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	deviceCodeURL      = "https://auth.openai.com/api/accounts/deviceauth/usercode"
	tokenURL           = "https://auth.openai.com/api/accounts/deviceauth/token"
	clientID           = "app_EMoamEEZ73f0CkXaXp7hrann"
	scope              = "openid email profile offline_access"
	defaultVerifyURI   = "https://auth.openai.com/codex/device"
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
	DeviceAuthID    string `json:"device_auth_id"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"-"`
	Interval        int    `json:"-"`
	ExpiresIn       int    `json:"-"`
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

func RequestDeviceCode(ctx context.Context) (*DeviceCodeResponse, error) {
	body := fmt.Sprintf(`{"client_id":"%s","scope":"%s"}`, clientID, scope)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, deviceCodeURL, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

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
		DeviceAuthID string `json:"device_auth_id"`
		UserCode     string `json:"user_code"`
		Interval     json.RawMessage `json:"interval"`
		ExpiresIn    json.RawMessage `json:"expires_in"`
		ExpiresAt    string          `json:"expires_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}

	dcr := &DeviceCodeResponse{
		DeviceAuthID:    raw.DeviceAuthID,
		UserCode:        raw.UserCode,
		VerificationURI: defaultVerifyURI,
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

func PollForToken(ctx context.Context, deviceAuthID, userCode string) (*TokenResponse, error) {
	payload := map[string]string{
		"device_auth_id": deviceAuthID,
		"user_code":      userCode,
		"grant_type":     "urn:ietf:params:oauth:grant-type:device_code",
		"client_id":      clientID,
	}
	payloadBytes, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(string(payloadBytes)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		var tr TokenResponse
		if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
			return nil, err
		}
		return &tr, nil
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

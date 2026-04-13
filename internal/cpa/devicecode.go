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
	deviceCodeURL = "https://auth.openai.com/api/accounts/deviceauth/usercode"
	tokenURL      = "https://auth.openai.com/api/accounts/deviceauth/token"
	clientID      = "app_EMoamEEZ73f0CkXaXp7hrann"
	scope         = "openid email profile offline_access"
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
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
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

	var dcr DeviceCodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&dcr); err != nil {
		return nil, err
	}
	if dcr.VerificationURI == "" {
		dcr.VerificationURI = "https://auth.openai.com/codex/device"
	}
	if dcr.Interval == 0 {
		dcr.Interval = 5
	}
	return &dcr, nil
}

func PollForToken(ctx context.Context, deviceCode string) (*TokenResponse, error) {
	body := fmt.Sprintf(`{"device_code":"%s","grant_type":"urn:ietf:params:oauth:grant-type:device_code","client_id":"%s"}`, deviceCode, clientID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(body))
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

	// parse error response
	var errResp struct {
		Error string `json:"error"`
	}
	json.NewDecoder(resp.Body).Decode(&errResp)

	switch errResp.Error {
	case "authorization_pending":
		return nil, ErrAuthorizationPending
	case "slow_down":
		return nil, ErrSlowDown
	case "expired_token":
		return nil, ErrExpiredToken
	case "access_denied":
		return nil, ErrAccessDenied
	default:
		return nil, fmt.Errorf("token poll error: %s (HTTP %d)", errResp.Error, resp.StatusCode)
	}
}

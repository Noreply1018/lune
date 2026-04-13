package cpa

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

type AccountInfo struct {
	Email     string `json:"email"`
	PlanType  string `json:"plan_type"`
	AccountID string `json:"account_id"`
}

func ParseAccountInfo(accessToken string) (*AccountInfo, error) {
	parts := strings.Split(accessToken, ".")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid JWT: expected at least 2 parts, got %d", len(parts))
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode JWT payload: %w", err)
	}

	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("parse JWT claims: %w", err)
	}

	info := &AccountInfo{}

	// extract email from https://api.openai.com/profile claim
	if profile, ok := claims["https://api.openai.com/profile"].(map[string]any); ok {
		if email, ok := profile["email"].(string); ok {
			info.Email = email
		}
	}

	// extract plan_type and account_id from https://api.openai.com/auth claim
	if auth, ok := claims["https://api.openai.com/auth"].(map[string]any); ok {
		if planType, ok := auth["chatgpt_plan_type"].(string); ok {
			info.PlanType = planType
		}
		if accountID, ok := auth["chatgpt_account_id"].(string); ok {
			info.AccountID = accountID
		}
	}

	if info.Email == "" {
		return nil, fmt.Errorf("email not found in JWT claims")
	}

	return info, nil
}

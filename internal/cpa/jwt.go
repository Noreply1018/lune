package cpa

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type AccountInfo struct {
	Email     string `json:"email"`
	PlanType  string `json:"plan_type"`
	AccountID string `json:"account_id"`
}

func ParseAccountInfo(token string) (*AccountInfo, error) {
	claims, err := parseJWTClaims(token)
	if err != nil {
		return nil, err
	}

	info := extractAccountInfo(claims)
	if info.Email == "" {
		return nil, fmt.Errorf("email not found in JWT claims")
	}

	return info, nil
}

func ParseAccountInfoFromTokens(idToken, accessToken string) (*AccountInfo, error) {
	var errs []string

	if strings.TrimSpace(idToken) != "" {
		info, err := ParseAccountInfo(idToken)
		if err == nil {
			return info, nil
		}
		errs = append(errs, fmt.Sprintf("id_token: %v", err))
	}

	if strings.TrimSpace(accessToken) != "" {
		info, err := ParseAccountInfo(accessToken)
		if err == nil {
			return info, nil
		}
		errs = append(errs, fmt.Sprintf("access_token: %v", err))
	}

	if len(errs) == 0 {
		return nil, fmt.Errorf("no token available for account info")
	}
	return nil, errors.New(strings.Join(errs, "; "))
}

func parseJWTClaims(token string) (map[string]any, error) {
	parts := strings.Split(token, ".")
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

	return claims, nil
}

func extractAccountInfo(claims map[string]any) *AccountInfo {
	info := &AccountInfo{}

	if email, ok := claims["email"].(string); ok {
		info.Email = email
	}
	if accountID, ok := claims["sub"].(string); ok {
		info.AccountID = accountID
	}
	if planType, ok := claims["chatgpt_plan_type"].(string); ok {
		info.PlanType = planType
	}
	if accountID, ok := claims["chatgpt_account_id"].(string); ok && accountID != "" {
		info.AccountID = accountID
	}

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

	return info
}

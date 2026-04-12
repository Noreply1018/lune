package backendproxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
)

var errBackendAdminCredsMissing = errors.New("backend admin credentials are not configured")

type adminSession struct {
	client *http.Client

	mu          sync.Mutex
	token       string
	tokenTarget string
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    string `json:"data"`
}

func newAdminSession(client *http.Client) *adminSession {
	return &adminSession{
		client: client,
	}
}

func (s *adminSession) Token(ctx context.Context, target string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.token != "" && s.tokenTarget == target {
		return s.token, nil
	}

	token, err := s.login(ctx, target)
	if err != nil {
		return "", err
	}
	s.token = token
	s.tokenTarget = target
	return token, nil
}

func (s *adminSession) Invalidate(target string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.tokenTarget == target {
		s.token = ""
	}
}

func (s *adminSession) login(ctx context.Context, target string) (string, error) {
	username := strings.TrimSpace(os.Getenv("LUNE_BACKEND_ADMIN_USERNAME"))
	password := os.Getenv("LUNE_BACKEND_ADMIN_PASSWORD")
	if username == "" || password == "" {
		return "", errBackendAdminCredsMissing
	}

	payload, err := json.Marshal(loginRequest{
		Username: username,
		Password: password,
	})
	if err != nil {
		return "", err
	}

	loginURL := strings.TrimRight(target, "/") + "/api/user/login"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, loginURL, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result loginResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("backend login failed: decode response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		if result.Message != "" {
			return "", fmt.Errorf("backend login failed: %s", result.Message)
		}
		return "", fmt.Errorf("backend login failed: %s", resp.Status)
	}
	if !result.Success || strings.TrimSpace(result.Data) == "" {
		if result.Message == "" {
			result.Message = "empty backend admin token"
		}
		return "", fmt.Errorf("backend login failed: %s", result.Message)
	}

	return result.Data, nil
}

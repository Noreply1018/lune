package cpa

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type ManagementClient struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

func NewManagementClient(baseURL, apiKey string) *ManagementClient {
	return &ManagementClient{
		baseURL: strings.TrimRight(baseURL, "/") + "/v0/management",
		apiKey:  apiKey,
		client:  &http.Client{Timeout: 20 * time.Second},
	}
}

type OAuthStartResponse struct {
	Status string `json:"status"`
	State  string `json:"state"`
	URL    string `json:"url"`
}

type OAuthStatusResponse struct {
	Status string `json:"status"`
	Error  string `json:"error"`
}

func (c *ManagementClient) StartCodexAuth(ctx context.Context) (*OAuthStartResponse, error) {
	var out OAuthStartResponse
	if err := c.doJSON(ctx, http.MethodGet, "/codex-auth-url?is_webui=true", nil, &out); err != nil {
		return nil, err
	}
	if strings.TrimSpace(out.State) == "" || strings.TrimSpace(out.URL) == "" {
		return nil, fmt.Errorf("CPA management returned an incomplete auth response")
	}
	return &out, nil
}

func (c *ManagementClient) GetAuthStatus(ctx context.Context, state string) (*OAuthStatusResponse, error) {
	q := url.Values{}
	q.Set("state", state)
	var out OAuthStatusResponse
	if err := c.doJSON(ctx, http.MethodGet, "/get-auth-status?"+q.Encode(), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *ManagementClient) SubmitOAuthCallback(ctx context.Context, provider, redirectURL string) error {
	body := map[string]string{
		"provider":     provider,
		"redirect_url": redirectURL,
	}
	return c.doJSON(ctx, http.MethodPost, "/oauth-callback", body, nil)
}

func (c *ManagementClient) doJSON(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		var errBody struct {
			Status string `json:"status"`
			Error  string `json:"error"`
		}
		_ = json.Unmarshal(raw, &errBody)
		if strings.TrimSpace(errBody.Error) != "" {
			return fmt.Errorf("%s", errBody.Error)
		}
		if len(raw) > 0 {
			return fmt.Errorf("%s", strings.TrimSpace(string(raw)))
		}
		return fmt.Errorf("CPA management request failed: HTTP %d", resp.StatusCode)
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("parse CPA management response: %w", err)
	}
	return nil
}

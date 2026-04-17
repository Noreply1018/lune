package cpa

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type ManagementClient struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

// AuthFile mirrors one entry from GET /v0/management/auth-files.
// Only the fields Lune consumes are declared; extra fields are ignored.
type AuthFile struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	AuthIndex  string `json:"auth_index"`
	Type       string `json:"type"`
	Provider   string `json:"provider"`
	Email      string `json:"email"`
	Status     string `json:"status"`
	IDToken    struct {
		ChatGPTAccountID string `json:"chatgpt_account_id"`
		PlanType         string `json:"plan_type"`
	} `json:"id_token"`
}

// APICallResponse is the envelope returned by POST /v0/management/api-call.
type APICallResponse struct {
	StatusCode int                 `json:"status_code"`
	Header     map[string][]string `json:"header"`
	Body       string              `json:"body"`
}

func NewManagementClient(baseURL, apiKey string) *ManagementClient {
	return &ManagementClient{
		baseURL: strings.TrimRight(baseURL, "/") + "/v0/management",
		apiKey:  apiKey,
		client:  &http.Client{Timeout: 20 * time.Second},
	}
}

// ListAuthFiles returns all auth files visible to the management key.
// CPA wraps the list in {"files": [...]}.
func (c *ManagementClient) ListAuthFiles(ctx context.Context) ([]AuthFile, error) {
	var out struct {
		Files []AuthFile `json:"files"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/auth-files", nil, &out); err != nil {
		return nil, err
	}
	return out.Files, nil
}

// APICall proxies a single HTTP call through CPA, substituting $TOKEN$ in header
// values using the auth file identified by authIndex.
//
// CPA requires the request field to be named `header` (singular); `headers`
// is silently ignored, which causes $TOKEN$ to leak through and return 401.
func (c *ManagementClient) APICall(ctx context.Context, authIndex, method, url string, header map[string]string) (*APICallResponse, error) {
	if authIndex == "" {
		return nil, fmt.Errorf("APICall: empty auth_index")
	}
	payload := map[string]any{
		"auth_index": authIndex,
		"method":     method,
		"url":        url,
	}
	if len(header) > 0 {
		payload["header"] = header
	}
	var out APICallResponse
	if err := c.doJSON(ctx, http.MethodPost, "/api-call", payload, &out); err != nil {
		return nil, err
	}
	return &out, nil
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

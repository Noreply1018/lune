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

func NewManagementClient(baseURL, apiKey string) *ManagementClient {
	return &ManagementClient{
		baseURL: strings.TrimRight(baseURL, "/") + "/v0/management",
		apiKey:  apiKey,
		client:  &http.Client{Timeout: 20 * time.Second},
	}
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

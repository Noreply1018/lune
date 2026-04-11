package account

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"lune/internal/config"
	"lune/internal/execution"
)

type OpenAIUpstreamAdapter struct {
	client *http.Client
}

func NewOpenAIUpstreamAdapter() *OpenAIUpstreamAdapter {
	return &OpenAIUpstreamAdapter{
		client: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

func (a *OpenAIUpstreamAdapter) ID() string {
	return "openai-upstream"
}

func (a *OpenAIUpstreamAdapter) Prepare(_ context.Context, req execution.Request, plan execution.Plan, platform config.Platform, account config.Account) (*execution.PreparedExecution, error) {
	payload := make(map[string]any, len(req.Payload)+1)
	for k, v := range req.Payload {
		payload[k] = v
	}
	payload["model"] = plan.TargetModel

	headers := req.Headers.Clone()

	apiKey := os.Getenv(account.CredentialEnv)
	if apiKey != "" {
		headers.Set("Authorization", "Bearer "+apiKey)
	}

	return &execution.PreparedExecution{
		Request:     req,
		Plan:        plan,
		Platform:    platform,
		Account:     account,
		TargetModel: plan.TargetModel,
		RawBody:     req.RawBody,
		Payload:     payload,
		Headers:     headers,
	}, nil
}

func (a *OpenAIUpstreamAdapter) Execute(ctx context.Context, prepared *execution.PreparedExecution) (*execution.RawResult, error) {
	upstreamURL := os.Getenv("LUNE_BACKEND_URL")
	if upstreamURL == "" {
		upstreamURL = "http://localhost:3000"
	}

	targetURL := upstreamURL + prepared.Request.Endpoint

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(prepared.RawBody))
	if err != nil {
		return nil, fmt.Errorf("build upstream request: %w", err)
	}

	httpReq.Header = prepared.Headers.Clone()
	httpReq.Header.Set("Content-Type", "application/json")

	timeout := time.Duration(prepared.Platform.TimeoutS) * time.Second
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
		httpReq = httpReq.WithContext(ctx)
	}

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("upstream request failed: %w", err)
	}

	return &execution.RawResult{
		StatusCode: resp.StatusCode,
		Header:     resp.Header,
		Body:       resp.Body,
	}, nil
}

func (a *OpenAIUpstreamAdapter) Normalize(_ context.Context, _ *execution.PreparedExecution, raw *execution.RawResult) (*execution.GatewayResponse, error) {
	return &execution.GatewayResponse{
		StatusCode: raw.StatusCode,
		Header:     raw.Header.Clone(),
		Body:       raw.Body,
	}, nil
}

func (a *OpenAIUpstreamAdapter) Classify(raw *execution.RawResult, err error) execution.Outcome {
	if err != nil {
		return execution.OutcomeFinalFailure
	}
	if raw == nil {
		return execution.OutcomeFinalFailure
	}
	switch {
	case raw.StatusCode == http.StatusTooManyRequests || raw.StatusCode >= 500:
		return execution.OutcomeRetryableFailure
	case raw.StatusCode >= 200 && raw.StatusCode < 400:
		return execution.OutcomeSuccess
	default:
		return execution.OutcomeFinalFailure
	}
}

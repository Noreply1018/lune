package account

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"lune/internal/config"
	"lune/internal/execution"
)

func TestOpenAIUpstreamAdapterID(t *testing.T) {
	adapter := NewOpenAIUpstreamAdapter()
	if adapter.ID() != "openai-upstream" {
		t.Fatalf("expected openai-upstream, got %s", adapter.ID())
	}
}

func TestOpenAIUpstreamAdapterPrepare(t *testing.T) {
	adapter := NewOpenAIUpstreamAdapter()

	t.Setenv("TEST_API_KEY", "sk-test-123")

	req := execution.Request{
		RequestID:  "1",
		Endpoint:   "/v1/chat/completions",
		Method:     http.MethodPost,
		ModelAlias: "gpt-4o",
		RawBody:    []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}]}`),
		Payload:    map[string]any{"model": "gpt-4o", "messages": []any{}},
		Headers:    http.Header{"Authorization": []string{"Bearer sk-user"}},
	}
	plan := execution.Plan{TargetModel: "gpt-4o-2024-08-06"}
	platform := config.Platform{ID: "backend"}
	account := config.Account{ID: "test-account", CredentialEnv: "TEST_API_KEY"}

	prepared, err := adapter.Prepare(context.Background(), req, plan, platform, account)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}

	if prepared.TargetModel != "gpt-4o-2024-08-06" {
		t.Fatalf("expected target model gpt-4o-2024-08-06, got %s", prepared.TargetModel)
	}
	if prepared.Payload["model"] != "gpt-4o-2024-08-06" {
		t.Fatalf("expected payload model gpt-4o-2024-08-06, got %v", prepared.Payload["model"])
	}
	if prepared.Headers.Get("Authorization") != "Bearer sk-test-123" {
		t.Fatalf("expected Authorization header from env, got %s", prepared.Headers.Get("Authorization"))
	}
}

func TestOpenAIUpstreamAdapterExecuteAndNormalize(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sk-upstream" {
			t.Errorf("expected upstream auth header, got %s", r.Header.Get("Authorization"))
		}
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("expected /v1/chat/completions, got %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "hello") {
			t.Errorf("expected body to contain hello, got %s", string(body))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"chatcmpl-1","choices":[{"message":{"content":"hi"}}]}`))
	}))
	defer upstream.Close()

	t.Setenv("LUNE_BACKEND_URL", upstream.URL)
	t.Setenv("TEST_KEY", "sk-upstream")

	adapter := NewOpenAIUpstreamAdapter()

	prepared := &execution.PreparedExecution{
		Request: execution.Request{
			Endpoint: "/v1/chat/completions",
			RawBody:  []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}]}`),
		},
		Platform: config.Platform{ID: "backend"},
		Account:  config.Account{ID: "test", CredentialEnv: "TEST_KEY"},
		RawBody:  []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}]}`),
		Headers:  http.Header{"Authorization": []string{"Bearer sk-upstream"}, "Content-Type": []string{"application/json"}},
	}

	raw, err := adapter.Execute(context.Background(), prepared)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if raw.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", raw.StatusCode)
	}

	response, err := adapter.Normalize(context.Background(), prepared, raw)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected normalized 200, got %d", response.StatusCode)
	}

	respBody, _ := io.ReadAll(response.Body)
	if !strings.Contains(string(respBody), "chatcmpl-1") {
		t.Fatalf("expected response body, got %s", string(respBody))
	}
}

func TestOpenAIUpstreamAdapterClassify(t *testing.T) {
	adapter := NewOpenAIUpstreamAdapter()

	tests := []struct {
		name     string
		raw      *execution.RawResult
		err      error
		expected execution.Outcome
	}{
		{"success", &execution.RawResult{StatusCode: 200}, nil, execution.OutcomeSuccess},
		{"rate limited", &execution.RawResult{StatusCode: 429}, nil, execution.OutcomeRetryableFailure},
		{"server error", &execution.RawResult{StatusCode: 502}, nil, execution.OutcomeRetryableFailure},
		{"client error", &execution.RawResult{StatusCode: 422}, nil, execution.OutcomeFinalFailure},
		{"nil result", nil, nil, execution.OutcomeFinalFailure},
		{"error", nil, io.ErrUnexpectedEOF, execution.OutcomeFinalFailure},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outcome := adapter.Classify(tt.raw, tt.err)
			if outcome != tt.expected {
				t.Fatalf("expected %s, got %s", tt.expected, outcome)
			}
		})
	}
}

func TestOpenAIUpstreamAdapterStreamingPassthrough(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data: {\"id\":\"1\"}\n\ndata: [DONE]\n\n"))
	}))
	defer upstream.Close()

	t.Setenv("LUNE_BACKEND_URL", upstream.URL)

	adapter := NewOpenAIUpstreamAdapter()

	prepared := &execution.PreparedExecution{
		Request:  execution.Request{Endpoint: "/v1/chat/completions", RawBody: []byte(`{"model":"gpt-4o","stream":true}`)},
		Platform: config.Platform{ID: "backend"},
		Account:  config.Account{ID: "test"},
		RawBody:  []byte(`{"model":"gpt-4o","stream":true}`),
		Headers:  http.Header{"Content-Type": []string{"application/json"}},
	}

	raw, err := adapter.Execute(context.Background(), prepared)
	if err != nil {
		t.Fatalf("execute streaming: %v", err)
	}

	response, err := adapter.Normalize(context.Background(), prepared, raw)
	if err != nil {
		t.Fatalf("normalize streaming: %v", err)
	}

	if response.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("expected text/event-stream content type, got %s", response.Header.Get("Content-Type"))
	}

	body, _ := io.ReadAll(response.Body)
	if !strings.Contains(string(body), "data: [DONE]") {
		t.Fatalf("expected SSE stream data, got %s", string(body))
	}
}

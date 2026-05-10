package gateway

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestForwardStreamRetryableStatusBuffersErrorBody(t *testing.T) {
	upstreamMsg := "mock upstream EOF before completion"
	bodyJSON := `{"error":{"message":"` + upstreamMsg + `"}}`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-upstream" {
			t.Fatalf("unexpected authorization header %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", strconv.Itoa(len(bodyJSON)))
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(bodyJSON))
	}))
	defer upstream.Close()

	reqBody := []byte(`{"model":"gpt-test","stream":true,"input":"hi"}`)
	replay := &ReplayBody{data: reqBody, size: int64(len(reqBody))}
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(""))
	w := httptest.NewRecorder()

	result := Forward(w, req, UpstreamTarget{
		BaseURL:   upstream.URL + "/v1",
		APIKey:    "sk-upstream",
		AccountID: 42,
	}, "responses", replay, true, "req-test", time.Second)

	if result.Err != nil {
		t.Fatalf("expected buffered upstream response, got err=%v", result.Err)
	}
	if result.Written {
		t.Fatalf("stream retryable status should not be written directly")
	}
	if w.Body.Len() != 0 {
		t.Fatalf("expected no downstream write before WriteResponse, got body=%q", w.Body.String())
	}
	if result.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", result.StatusCode)
	}
	if string(result.Body) != bodyJSON {
		t.Fatalf("expected buffered error body %q, got %q", bodyJSON, result.Body)
	}
	if got := result.Headers.Get("Content-Length"); got != "" {
		t.Fatalf("expected buffered stream error to drop upstream Content-Length, got %q", got)
	}
	if result.Stream == nil || !result.Stream.Failed || result.Stream.ErrorMessage != upstreamMsg {
		t.Fatalf("expected stream failure message %q, got %+v", upstreamMsg, result.Stream)
	}

	result.WriteResponse(w)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected downstream 500 after WriteResponse, got %d", w.Code)
	}
	if w.Body.String() != bodyJSON {
		t.Fatalf("expected downstream body %q, got %q", bodyJSON, w.Body.String())
	}
	if got := w.Header().Get("X-Lune-Account"); got != "42" {
		t.Fatalf("expected X-Lune-Account 42, got %q", got)
	}
}

func TestForwardStreamScannerErrorFailsResult(t *testing.T) {
	body := io.NopCloser(strings.NewReader("data: " + strings.Repeat("x", 1024*1024+1) + "\n"))
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       body,
	}
	w := &streamTestWriter{header: make(http.Header)}

	result := forwardStream(w, resp, "responses")

	if !result.Failed {
		t.Fatalf("expected scanner error to fail stream, got %+v", result)
	}
	if !strings.Contains(result.ErrorMessage, "stream read error") {
		t.Fatalf("expected stream read error message, got %+v", result)
	}
}

func TestForwardStreamDownstreamWriteErrorFailsResult(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`data: {"type":"response.output_text.delta","delta":"hello"}` + "\n")),
	}
	w := &streamTestWriter{header: make(http.Header), writeErr: errors.New("client disconnected")}

	result := forwardStream(w, resp, "responses")

	if !result.Failed {
		t.Fatalf("expected downstream write error to fail stream, got %+v", result)
	}
	if !strings.Contains(result.ErrorMessage, "downstream write error: client disconnected") {
		t.Fatalf("expected downstream write error message, got %+v", result)
	}
}

type streamTestWriter struct {
	header     http.Header
	statusCode int
	writeErr   error
}

func (w *streamTestWriter) Header() http.Header {
	return w.header
}

func (w *streamTestWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
}

func (w *streamTestWriter) Write(p []byte) (int, error) {
	if w.writeErr != nil {
		return 0, w.writeErr
	}
	return len(p), nil
}

func (w *streamTestWriter) Flush() {}

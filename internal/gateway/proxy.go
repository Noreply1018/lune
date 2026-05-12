package gateway

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

var hopByHopHeaders = map[string]bool{
	"Connection":          true,
	"Keep-Alive":          true,
	"Proxy-Authenticate":  true,
	"Proxy-Authorization": true,
	"Te":                  true,
	"Trailers":            true,
	"Transfer-Encoding":   true,
	"Upgrade":             true,
}

const maxBufferedStreamErrorBody = 1 << 20

type UpstreamTarget struct {
	BaseURL        string
	APIKey         string
	AccountID      int64
	RuntimeBinding *RuntimeBinding
}

type RuntimeBinding struct {
	AccountKey string
	AuthID     string
	AuthIndex  string
	OpenAIID   string
}

type ProxyResult struct {
	StatusCode int
	Usage      Usage
	Stream     *StreamResult
	Err        error
	Body       []byte      // non-stream: buffered body (not yet written to client)
	Headers    http.Header // non-stream: buffered response headers
	Written    bool        // true if response was already written (streaming)
}

type StreamResult struct {
	Usage            Usage
	Completed        bool
	CompletionMarker string
	Failed           bool
	ErrorMessage     string
	Err              error
}

func Forward(w http.ResponseWriter, r *http.Request, target UpstreamTarget, pathSuffix string, body *ReplayBody, isStream bool, requestID string, timeout time.Duration) *ProxyResult {
	// build upstream URL
	baseURL := strings.TrimRight(target.BaseURL, "/")
	upstreamURL := baseURL + "/" + pathSuffix

	bodyReader, err := body.Reader()
	if err != nil {
		return &ProxyResult{Err: fmt.Errorf("open replay body: %w", err)}
	}
	defer bodyReader.Close()

	// create upstream request
	upstreamReq, err := http.NewRequestWithContext(r.Context(), r.Method, upstreamURL, bodyReader)
	if err != nil {
		return &ProxyResult{Err: fmt.Errorf("create request: %w", err)}
	}
	upstreamReq.ContentLength = body.Size()

	// copy headers
	for k, vv := range r.Header {
		if hopByHopHeaders[k] {
			continue
		}
		if strings.EqualFold(k, "Authorization") {
			continue
		}
		if strings.EqualFold(k, "Host") {
			continue
		}
		for _, v := range vv {
			upstreamReq.Header.Add(k, v)
		}
	}
	upstreamReq.Header.Set("Authorization", "Bearer "+target.APIKey)
	if target.RuntimeBinding != nil {
		if target.RuntimeBinding.AccountKey != "" {
			upstreamReq.Header.Set("X-Lune-CPA-Account-Key", target.RuntimeBinding.AccountKey)
		}
		if target.RuntimeBinding.AuthID != "" {
			upstreamReq.Header.Set("X-Lune-Runtime-Auth-Id", target.RuntimeBinding.AuthID)
			upstreamReq.Header.Set("X-CLIProxyAPI-Pinned-Auth-Id", target.RuntimeBinding.AuthID)
		}
		if target.RuntimeBinding.AuthIndex != "" {
			upstreamReq.Header.Set("X-Lune-Runtime-Auth-Index", target.RuntimeBinding.AuthIndex)
			upstreamReq.Header.Set("X-CLIProxyAPI-Pinned-Auth-Index", target.RuntimeBinding.AuthIndex)
			upstreamReq.Header.Set("X-CPA-Auth-Index", target.RuntimeBinding.AuthIndex)
		}
		if target.RuntimeBinding.OpenAIID != "" {
			upstreamReq.Header.Set("ChatGPT-Account-Id", target.RuntimeBinding.OpenAIID)
		}
	}
	upstreamReq.Header.Set("Host", upstreamReq.URL.Host)

	client := &http.Client{Timeout: timeout}

	resp, err := client.Do(upstreamReq)
	if err != nil {
		return &ProxyResult{Err: classifyError(err)}
	}
	defer resp.Body.Close()

	result := &ProxyResult{StatusCode: resp.StatusCode}

	// collect response headers (excluding hop-by-hop)
	respHeaders := make(http.Header)
	for k, vv := range resp.Header {
		if hopByHopHeaders[k] {
			continue
		}
		for _, v := range vv {
			respHeaders.Add(k, v)
		}
	}
	respHeaders.Set("X-Lune-Request-Id", requestID)
	respHeaders.Set("X-Lune-Account", fmt.Sprintf("%d", target.AccountID))

	if isStream {
		if IsRetryableStatus(resp.StatusCode) {
			respBody, err := readBoundedBody(resp.Body, maxBufferedStreamErrorBody)
			if err != nil {
				respBody = []byte(`{"error":{"message":"failed to read upstream response"}}`)
			}
			result.Body = respBody
			result.Headers = respHeaders
			result.Headers.Del("Content-Length")
			result.Usage = ParseUsageFromBody(respBody)
			if msg := extractUpstreamErrorMessage(respBody); msg != "" {
				result.Stream = &StreamResult{
					Failed:       true,
					ErrorMessage: msg,
					Err:          errors.New(msg),
				}
			}
			return result
		}

		// streaming: write directly — cannot retry after this
		respHeaders.Del("Content-Length")
		for k, vv := range respHeaders {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
		result.Stream = forwardStream(w, resp, pathSuffix)
		result.Usage = result.Stream.Usage
		result.Written = true
	} else {
		// non-streaming: buffer body for potential retry
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			respBody = []byte(`{"error":{"message":"failed to read upstream response"}}`)
		}
		result.Body = respBody
		result.Headers = respHeaders
		result.Usage = ParseUsageFromBody(respBody)
	}

	return result
}

func readBoundedBody(r io.Reader, max int64) ([]byte, error) {
	if max <= 0 {
		return nil, nil
	}
	body, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return body, err
	}
	if int64(len(body)) > max {
		return body[:max], nil
	}
	return body, nil
}

// WriteResponse flushes a buffered non-stream response to the client.
func (r *ProxyResult) WriteResponse(w http.ResponseWriter) {
	if r.Written {
		return
	}
	for k, vv := range r.Headers {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(r.StatusCode)
	w.Write(r.Body)
	r.Written = true
}

func forwardStream(w http.ResponseWriter, resp *http.Response, pathSuffix string) *StreamResult {
	result := &StreamResult{}
	flusher, ok := w.(http.Flusher)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":{"message":"streaming not supported"}}`))
		result.Failed = true
		result.ErrorMessage = "streaming not supported"
		result.Err = errors.New(result.ErrorMessage)
		return result
	}

	w.WriteHeader(resp.StatusCode)
	flusher.Flush()

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	expectedMarker := expectedStreamCompletionMarker(pathSuffix)
	acceptDoneMarker := isChatCompletionsStream(pathSuffix)

	for scanner.Scan() {
		line := scanner.Bytes()
		if _, err := w.Write(line); err != nil {
			result.Failed = true
			result.ErrorMessage = truncateStreamError("downstream write error: "+err.Error(), 512)
			result.Err = err
			return result
		}
		if _, err := w.Write([]byte("\n")); err != nil {
			result.Failed = true
			result.ErrorMessage = truncateStreamError("downstream write error: "+err.Error(), 512)
			result.Err = err
			return result
		}
		flusher.Flush()

		// parse usage from SSE data lines
		if bytes.HasPrefix(line, []byte("data: ")) {
			data := line[6:]
			data = bytes.TrimSpace(data)
			if acceptDoneMarker && bytes.Equal(data, []byte("[DONE]")) {
				result.Completed = true
				result.CompletionMarker = "[DONE]"
				continue
			}
			updateStreamResultFromSSEData(result, data)
			if result.Failed {
				continue
			}
			if u := ParseUsageFromSSEChunk(data); u.InputTokens > 0 || u.OutputTokens > 0 {
				result.Usage = u
			}
		} else if msg := extractUpstreamErrorMessage(line); msg != "" && result.ErrorMessage == "" {
			result.ErrorMessage = msg
		}
	}

	if err := scanner.Err(); err != nil {
		result.Failed = true
		result.Err = err
		if isTimeoutError(err) {
			result.ErrorMessage = "stream timed out before " + expectedMarker
			return result
		}
		result.ErrorMessage = truncateStreamError("stream read error: "+err.Error(), 512)
		return result
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 && !result.Completed && !result.Failed {
		result.Failed = true
		result.ErrorMessage = "stream closed before " + expectedMarker
		result.Err = errors.New(result.ErrorMessage)
	}
	return result
}

func expectedStreamCompletionMarker(pathSuffix string) string {
	pathSuffix = strings.Trim(pathSuffix, "/")
	if isChatCompletionsStream(pathSuffix) {
		return "[DONE]"
	}
	if isResponsesStream(pathSuffix) {
		return "response.completed"
	}
	return "completion marker"
}

func isChatCompletionsStream(pathSuffix string) bool {
	return strings.HasSuffix(strings.Trim(pathSuffix, "/"), "chat/completions")
}

func isResponsesStream(pathSuffix string) bool {
	return strings.HasSuffix(strings.Trim(pathSuffix, "/"), "responses")
}

func updateStreamResultFromSSEData(result *StreamResult, data []byte) {
	var event map[string]any
	if err := json.Unmarshal(data, &event); err != nil {
		return
	}
	eventType, _ := event["type"].(string)
	switch eventType {
	case "response.completed":
		result.Completed = true
		result.CompletionMarker = eventType
	case "response.failed", "response.incomplete":
		result.Failed = true
		if msg := extractUpstreamErrorMessageFromMap(event); msg != "" {
			result.ErrorMessage = msg
		} else {
			result.ErrorMessage = eventType
		}
		result.Err = errors.New(result.ErrorMessage)
	}
}

func extractUpstreamErrorMessage(body []byte) string {
	var payload any
	if err := json.Unmarshal(bytes.TrimSpace(body), &payload); err != nil {
		return ""
	}
	if m, ok := payload.(map[string]any); ok {
		return extractUpstreamErrorMessageFromMap(m)
	}
	return ""
}

func extractUpstreamErrorMessageFromMap(m map[string]any) string {
	for _, key := range []string{"message", "detail", "error_message"} {
		if msg, ok := m[key].(string); ok && strings.TrimSpace(msg) != "" {
			return truncateStreamError(msg, 512)
		}
	}
	for _, key := range []string{"error", "response"} {
		if nested, ok := m[key].(map[string]any); ok {
			if msg := extractUpstreamErrorMessageFromMap(nested); msg != "" {
				return msg
			}
		}
	}
	if errVal, ok := m["error"].(string); ok && strings.TrimSpace(errVal) != "" {
		return truncateStreamError(errVal, 512)
	}
	return ""
}

func truncateStreamError(msg string, max int) string {
	msg = strings.TrimSpace(msg)
	if len(msg) <= max {
		return msg
	}
	if max <= 3 {
		return msg[:max]
	}
	return msg[:max-3] + "..."
}

type retryableError struct {
	err error
}

func (e *retryableError) Error() string { return e.err.Error() }
func (e *retryableError) Unwrap() error { return e.err }

func IsRetryable(err error) bool {
	_, ok := err.(*retryableError)
	return ok
}

func IsRetryableStatus(statusCode int) bool {
	return statusCode >= 500 || statusCode == 429
}

func classifyError(err error) error {
	// network errors are retryable
	if _, ok := err.(net.Error); ok {
		return &retryableError{err: err}
	}
	if strings.Contains(err.Error(), "connection refused") ||
		strings.Contains(err.Error(), "no such host") ||
		strings.Contains(err.Error(), "i/o timeout") {
		return &retryableError{err: err}
	}
	return err
}

func isTimeoutError(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	text := err.Error()
	return strings.Contains(text, "Client.Timeout") ||
		strings.Contains(text, "context deadline exceeded") ||
		strings.Contains(text, "i/o timeout")
}

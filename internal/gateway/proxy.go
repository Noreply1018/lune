package gateway

import (
	"bufio"
	"bytes"
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

type UpstreamTarget struct {
	BaseURL   string
	APIKey    string
	AccountID int64
}

type ProxyResult struct {
	StatusCode int
	Usage      Usage
	Err        error
	Body       []byte            // non-stream: buffered body (not yet written to client)
	Headers    http.Header       // non-stream: buffered response headers
	Written    bool              // true if response was already written (streaming)
}

func Forward(w http.ResponseWriter, r *http.Request, target UpstreamTarget, pathSuffix string, body []byte, isStream bool, requestID string, timeout time.Duration) *ProxyResult {
	// build upstream URL
	baseURL := strings.TrimRight(target.BaseURL, "/")
	upstreamURL := baseURL + "/" + pathSuffix

	// create upstream request
	upstreamReq, err := http.NewRequestWithContext(r.Context(), r.Method, upstreamURL, bytes.NewReader(body))
	if err != nil {
		return &ProxyResult{Err: fmt.Errorf("create request: %w", err)}
	}

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
		// streaming: write directly — cannot retry after this
		for k, vv := range respHeaders {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
		result.Usage = forwardStream(w, resp)
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

func forwardStream(w http.ResponseWriter, resp *http.Response) Usage {
	flusher, ok := w.(http.Flusher)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":{"message":"streaming not supported"}}`))
		return Usage{}
	}

	w.WriteHeader(resp.StatusCode)
	flusher.Flush()

	var usage Usage
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		w.Write(line)
		w.Write([]byte("\n"))
		flusher.Flush()

		// parse usage from SSE data lines
		if bytes.HasPrefix(line, []byte("data: ")) {
			data := line[6:]
			if !bytes.Equal(data, []byte("[DONE]")) {
				if u := ParseUsageFromSSEChunk(data); u.InputTokens > 0 || u.OutputTokens > 0 {
					usage = u
				}
			}
		}
	}

	return usage
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

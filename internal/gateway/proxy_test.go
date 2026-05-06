package gateway

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

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

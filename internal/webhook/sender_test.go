package webhook

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSenderSendPostsJSONPayload(t *testing.T) {
	t.Parallel()

	var got Payload
	var gotContentType string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	sender := NewSender()
	payload := Payload{
		Event:     "account_error",
		Severity:  "critical",
		Title:     "Test",
		Message:   "body",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	if err := sender.Send(context.Background(), server.URL, payload); err != nil {
		t.Fatalf("send webhook: %v", err)
	}

	if gotContentType != "application/json" {
		t.Fatalf("expected content-type application/json, got %q", gotContentType)
	}
	if got != payload {
		t.Fatalf("unexpected payload: %#v", got)
	}
}

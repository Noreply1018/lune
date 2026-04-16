package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"lune/internal/store"
	"lune/internal/webhook"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.New(filepath.Join(t.TempDir(), "lune-test.db"))
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}
	t.Cleanup(func() {
		_ = st.Close()
	})
	return st
}

func TestNormalizeSettingValueWebhookURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{name: "empty allowed", raw: `""`, want: ""},
		{name: "http allowed", raw: `"http://example.com/hook"`, want: "http://example.com/hook"},
		{name: "https allowed", raw: `"https://example.com/hook"`, want: "https://example.com/hook"},
		{name: "invalid scheme rejected", raw: `"ftp://example.com/hook"`, wantErr: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := normalizeSettingValue("webhook_url", json.RawMessage(tc.raw))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("normalize webhook_url: %v", err)
			}
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestTestWebhookUsesStoredURL(t *testing.T) {
	t.Parallel()

	st := newTestStore(t)
	cache := store.NewRoutingCache(st)

	called := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	if err := st.UpdateSettings(map[string]string{"webhook_url": server.URL}); err != nil {
		t.Fatalf("seed webhook_url: %v", err)
	}
	cache.Invalidate()

	handler := NewHandler(st, cache, "", "", nil, webhook.NewSender())
	req := httptest.NewRequest(http.MethodPost, "/admin/api/settings/webhook/test", http.NoBody)
	rr := httptest.NewRecorder()

	handler.testWebhook(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if called != 1 {
		t.Fatalf("expected webhook receiver to be called once, got %d", called)
	}
}

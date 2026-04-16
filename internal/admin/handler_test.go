package admin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"lune/internal/cpa"
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

func TestBatchImportCpaAccountsRollsBackWhenPoolMembershipFails(t *testing.T) {
	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	authDir := t.TempDir()

	svcID, err := st.CreateCpaService(&store.CpaService{
		Label:   "CPA",
		BaseURL: "https://example.com",
		APIKey:  "test-key",
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("create cpa service: %v", err)
	}

	if err := cpa.WriteAuthFile(authDir, &cpa.CpaAuthFile{
		AccountID: "acct_123",
		Email:     "batch@example.com",
		Type:      "openai",
		Disabled:  false,
	}, "acct-key"); err != nil {
		t.Fatalf("write auth file: %v", err)
	}

	handler := NewHandler(st, cache, authDir, "", nil, webhook.NewSender())
	body := bytes.NewBufferString(fmt.Sprintf(`{"service_id":%d,"account_keys":["acct-key"],"pool_id":999}`, svcID))
	req := httptest.NewRequest(http.MethodPost, "/admin/api/accounts/cpa/import/batch", body)
	rr := httptest.NewRecorder()

	handler.batchImportCpaAccounts(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp struct {
		Data struct {
			Imported int      `json:"imported"`
			Skipped  int      `json:"skipped"`
			Errors   []string `json:"errors"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Data.Imported != 0 {
		t.Fatalf("expected imported to stay 0, got %d", resp.Data.Imported)
	}
	if len(resp.Data.Errors) != 1 {
		t.Fatalf("expected 1 import error, got %d", len(resp.Data.Errors))
	}

	accounts, err := st.ListAccounts()
	if err != nil {
		t.Fatalf("list accounts: %v", err)
	}
	if len(accounts) != 0 {
		t.Fatalf("expected orphan account rollback, got %d accounts", len(accounts))
	}

}

func TestListPoolTokensIncludesPoolLabel(t *testing.T) {
	st := newTestStore(t)
	cache := store.NewRoutingCache(st)

	poolID, err := st.CreatePool("Primary Pool", 0, true)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	poolIDCopy := poolID
	if _, err := st.CreateToken(&store.AccessToken{
		Name:    "pool-token",
		Token:   "sk-lune-pool-token-1234",
		PoolID:  &poolIDCopy,
		Enabled: true,
	}); err != nil {
		t.Fatalf("create token: %v", err)
	}

	handler := NewHandler(st, cache, "", "", nil, webhook.NewSender())
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/admin/api/pools/%d/tokens", poolID), http.NoBody)
	req.SetPathValue("id", fmt.Sprintf("%d", poolID))
	rr := httptest.NewRecorder()

	handler.listPoolTokens(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp struct {
		Data []store.AccessToken `json:"data"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 token, got %d", len(resp.Data))
	}
	if resp.Data[0].PoolLabel != "Primary Pool" {
		t.Fatalf("expected pool label to be populated, got %q", resp.Data[0].PoolLabel)
	}
	if resp.Data[0].Token != "" {
		t.Fatalf("expected token secret to be stripped from list response")
	}
}

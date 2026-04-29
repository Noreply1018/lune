package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"lune/internal/auth"
	"lune/internal/router"
	"lune/internal/store"
)

func newHandlerTestStore(t *testing.T) (*store.Store, *store.RoutingCache, *Handler, *store.AccessToken) {
	t.Helper()
	st, err := store.New(filepath.Join(t.TempDir(), "lune.db"))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	cache := store.NewRoutingCache(st)
	handler := NewHandler(router.New(cache), cache, st, filepath.Join(t.TempDir(), "tmp"))
	poolID, err := st.CreatePool("test-pool", 1, true)
	if err != nil {
		t.Fatalf("CreatePool: %v", err)
	}
	poolIDCopy := poolID
	tokenID, err := st.CreateToken(&store.AccessToken{Name: "test-token", Token: "sk-test", PoolID: &poolIDCopy, Enabled: true})
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	token := &store.AccessToken{ID: tokenID, Name: "test-token", Token: "sk-test", PoolID: &poolIDCopy, Enabled: true}
	return st, cache, handler, token
}

func TestGatewayRouteErrorsLogHTTPStatus(t *testing.T) {
	st, cache, handler, token := newHandlerTestStore(t)
	req := authenticatedRequest(handler, token, `{"model":"missing-model"}`)
	rr := httptest.NewRecorder()

	req.ServeHTTP(rr, req.Request)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%s", rr.Code, rr.Body.String())
	}
	assertLatestLogStatus(t, st, 503)
	cache.Invalidate()
}

func TestGatewayNoHealthyAccountLogsHTTPStatus(t *testing.T) {
	st, _, handler, token := newHandlerTestStore(t)
	accountID, err := st.CreateAccount(&store.Account{
		Label:      "error-account",
		SourceKind: "openai_compat",
		BaseURL:    "http://example.invalid/v1",
		APIKey:     "sk-upstream",
		Provider:   "openai",
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	if err := st.UpdateAccountHealth(accountID, "error", "broken"); err != nil {
		t.Fatalf("UpdateAccountHealth: %v", err)
	}
	if _, err := st.AddPoolMember(*token.PoolID, accountID); err != nil {
		t.Fatalf("AddPoolMember: %v", err)
	}
	if err := st.RefreshAccountModels(accountID, []string{"gpt-test"}); err != nil {
		t.Fatalf("RefreshAccountModels: %v", err)
	}

	req := authenticatedRequest(handler, token, `{"model":"gpt-test"}`)
	rr := httptest.NewRecorder()

	req.ServeHTTP(rr, req.Request)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%s", rr.Code, rr.Body.String())
	}
	assertLatestLogStatus(t, st, 503)
}

func TestGatewaySuccessClearsCpaCredentialError(t *testing.T) {
	st, cache, handler, token := newHandlerTestStore(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/responses") {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": "ok"}}},
		})
	}))
	defer server.Close()

	serviceID, err := st.CreateCpaService(&store.CpaService{
		Label:   "CPA",
		BaseURL: server.URL,
		APIKey:  "service-key",
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("CreateCpaService: %v", err)
	}
	accountID, err := st.CreateAccount(&store.Account{
		Label:               "Codex",
		SourceKind:          "cpa",
		CpaServiceID:        &serviceID,
		CpaProvider:         "codex",
		CpaCredentialStatus: "needs_login",
		CpaCredentialReason: "refresh_failed",
		Enabled:             true,
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	if err := st.RefreshAccountModels(accountID, []string{"gpt-5-codex"}); err != nil {
		t.Fatalf("RefreshAccountModels: %v", err)
	}
	if err := st.UpdateAccountHealth(accountID, "healthy", ""); err != nil {
		t.Fatalf("UpdateAccountHealth: %v", err)
	}
	if _, err := st.AddPoolMember(*token.PoolID, accountID); err != nil {
		t.Fatalf("AddPoolMember: %v", err)
	}
	cache.Invalidate()

	req := authenticatedRequest(handler, token, `{"model":"gpt-5-codex","messages":[{"role":"user","content":"hi"}]}`)
	rr := httptest.NewRecorder()
	req.ServeHTTP(rr, req.Request)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	waitForGatewayTest(t, func() bool {
		acc, err := st.GetAccount(accountID)
		return err == nil && acc != nil && acc.CpaCredentialStatus == "ok"
	})
}

type authedGatewayRequest struct {
	http.Handler
	*http.Request
}

func authenticatedRequest(handler *Handler, token *store.AccessToken, body string) authedGatewayRequest {
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token.Token)
	return authedGatewayRequest{
		Handler: auth.GatewayAuth(handler, handler.cache),
		Request: req,
	}
}

func assertLatestLogStatus(t *testing.T, st *store.Store, expected int) {
	t.Helper()
	for range 20 {
		logs, _, err := st.ListLogs(1, 0)
		if err != nil {
			t.Fatalf("ListLogs: %v", err)
		}
		if len(logs) > 0 {
			if logs[0].StatusCode != expected {
				t.Fatalf("expected latest status %d, got %+v", expected, logs[0])
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected a request log with status %d", expected)
}

func waitForGatewayTest(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !condition() {
		t.Fatalf("condition not met before timeout")
	}
}

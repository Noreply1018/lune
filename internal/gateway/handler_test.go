package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"lune/internal/auth"
	"lune/internal/health"
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
	handler := NewHandler(router.NewWithOptions(cache, router.Options{CpaRuntimeBindingSupported: true}), cache, st, filepath.Join(t.TempDir(), "tmp"))
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

func TestGatewayProductionCpaRuntimeBindingUnsupportedReturnsExplicitError(t *testing.T) {
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
	serviceID, err := st.CreateCpaService(&store.CpaService{Label: "CPA", BaseURL: "http://cpa.example", APIKey: "service-key", Enabled: true})
	if err != nil {
		t.Fatalf("CreateCpaService: %v", err)
	}
	accountID := addCpaGatewayAccount(t, st, poolID, serviceID, "closed-cpa", "codex", "gpt-5-codex")
	cache.Invalidate()

	req := authenticatedRequest(handler, token, `{"model":"gpt-5-codex","input":"hi"}`)
	rr := httptest.NewRecorder()
	req.ServeHTTP(rr, req.Request)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "runtime_auth_binding_unavailable") || !strings.Contains(rr.Body.String(), "provider_pinning_unsupported") {
		t.Fatalf("expected explicit runtime binding error, got %s", rr.Body.String())
	}
	log := waitForLatestGatewayLog(t, st)
	if log.AccountID != 0 || log.SourceKind != "cpa" || !strings.Contains(log.ErrorMessage, "runtime_auth_binding_unavailable") {
		t.Fatalf("expected route-level CPA binding failure log without routed account %d, got %+v", accountID, log)
	}
}

func TestGatewaySkipsCpaAccountThatNeedsLogin(t *testing.T) {
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
		Label:                 "Codex",
		SourceKind:            "cpa",
		CpaServiceID:          &serviceID,
		CpaProvider:           "codex",
		CpaCredentialStatus:   "needs_login",
		CpaCredentialReason:   "refresh_failed",
		CpaSubscriptionStatus: "active",
		Enabled:               true,
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
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%s", rr.Code, rr.Body.String())
	}
	acc, err := st.GetAccount(accountID)
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
	if acc.CpaCredentialStatus != "needs_login" {
		t.Fatalf("expected credential status to remain needs_login, got %q", acc.CpaCredentialStatus)
	}
}

func TestGatewayCpaAuthFailureDoesNotOverwriteDiscoveryStatus(t *testing.T) {
	st, cache, handler, token := newHandlerTestStore(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"refresh token invalid"}}`))
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
		Label:                 "Codex",
		SourceKind:            "cpa",
		CpaServiceID:          &serviceID,
		CpaProvider:           "codex",
		CpaAccountKey:         "codex-key",
		CpaCredentialStatus:   "ok",
		CpaSubscriptionStatus: "active",
		Enabled:               true,
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
	handler.runtimeBinder = staticRuntimeBinder{}
	cache.Invalidate()

	req := authenticatedRequest(handler, token, `{"model":"gpt-5-codex","input":"hi"}`)
	req.Header.Set("X-Lune-Account-Id", strconv.FormatInt(accountID, 10))
	rr := httptest.NewRecorder()
	req.ServeHTTP(rr, req.Request)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%s", rr.Code, rr.Body.String())
	}

	waitForGatewayTest(t, func() bool {
		acc, err := st.GetAccount(accountID)
		return err == nil && acc != nil && acc.CpaCredentialStatus == "needs_login" && acc.Status == "healthy"
	})
}

func TestGatewayCpaRuntimeBindingConfirmedPinsHeadersAndLog(t *testing.T) {
	st, cache, handler, token := newHandlerTestStore(t)
	var sawPinned bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Lune-CPA-Account-Key"); got != "codex-key" {
			t.Fatalf("expected account key header, got %q", got)
		}
		if got := r.Header.Get("X-Lune-Runtime-Auth-Index"); got != "idx-1" {
			t.Fatalf("expected runtime auth index header, got %q", got)
		}
		if got := r.Header.Get("X-Lune-Runtime-Auth-Id"); got != "auth-1" {
			t.Fatalf("expected runtime auth id header, got %q", got)
		}
		sawPinned = true
		_ = json.NewEncoder(w).Encode(map[string]any{
			"usage": map[string]any{"input_tokens": 2, "output_tokens": 3},
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
		Label:                 "Codex",
		SourceKind:            "cpa",
		CpaServiceID:          &serviceID,
		CpaProvider:           "codex",
		CpaAccountKey:         "codex-key",
		CpaCredentialStatus:   "ok",
		CpaSubscriptionStatus: "active",
		Enabled:               true,
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	if err := st.RefreshAccountModels(accountID, []string{"gpt-5-codex"}); err != nil {
		t.Fatalf("RefreshAccountModels: %v", err)
	}
	if _, err := st.AddPoolMember(*token.PoolID, accountID); err != nil {
		t.Fatalf("AddPoolMember: %v", err)
	}
	handler.runtimeBinder = fixedRuntimeBinder{authID: "auth-1", authIndex: "idx-1"}
	cache.Invalidate()

	req := authenticatedRequest(handler, token, `{"model":"gpt-5-codex","input":"hi"}`)
	rr := httptest.NewRecorder()
	req.ServeHTTP(rr, req.Request)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !sawPinned {
		t.Fatalf("expected upstream to receive pinned binding headers")
	}
	log := waitForLatestGatewayLog(t, st)
	if log.AccountID != accountID || log.RuntimeBindingStatus != "confirmed" || log.RuntimeAuthIndex != "idx-1" || log.RuntimeAuthID != "auth-1" || log.RuntimeAccountKey != "codex-key" {
		t.Fatalf("expected confirmed runtime binding log for account %d, got %+v", accountID, log)
	}
}

type cpaUpstreamHit struct {
	AccountKey       string
	RuntimeAuthIndex string
	RuntimeAuthID    string
	PinnedAuthIndex  string
	PinnedAuthID     string
	LegacyAuthIndex  string
	OpenAIID         string
}

func TestGatewayCpaForcedAndAutomaticRoutesPreserveRuntimeBindingAccounting(t *testing.T) {
	st, cache, handler, token := newHandlerTestStore(t)

	var (
		mu   sync.Mutex
		hits []cpaUpstreamHit
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/responses") {
			http.NotFound(w, r)
			return
		}
		hit := cpaUpstreamHit{
			AccountKey:       r.Header.Get("X-Lune-CPA-Account-Key"),
			RuntimeAuthIndex: r.Header.Get("X-Lune-Runtime-Auth-Index"),
			RuntimeAuthID:    r.Header.Get("X-Lune-Runtime-Auth-Id"),
			PinnedAuthIndex:  r.Header.Get("X-CLIProxyAPI-Pinned-Auth-Index"),
			PinnedAuthID:     r.Header.Get("X-CLIProxyAPI-Pinned-Auth-Id"),
			LegacyAuthIndex:  r.Header.Get("X-CPA-Auth-Index"),
			OpenAIID:         r.Header.Get("ChatGPT-Account-Id"),
		}
		mu.Lock()
		hits = append(hits, hit)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"usage": map[string]any{"input_tokens": 2, "output_tokens": 3},
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
	accountA := addCpaGatewayAccount(t, st, *token.PoolID, serviceID, "cpa-a", "codex", "gpt-5-codex")
	accountB := addCpaGatewayAccount(t, st, *token.PoolID, serviceID, "cpa-b", "codex", "gpt-5-codex")
	accountC := addCpaGatewayAccount(t, st, *token.PoolID, serviceID, "cpa-c", "codex", "gpt-5-codex")
	handler.runtimeBinder = staticRuntimeBinder{}
	cache.Invalidate()

	var lastLogID int64
	for i := 0; i < 10; i++ {
		req := authenticatedRequest(handler, token, `{"model":"gpt-5-codex","input":"forced"}`)
		req.Header.Set("X-Lune-Account-Id", strconv.FormatInt(accountA, 10))
		rr := httptest.NewRecorder()
		req.ServeHTTP(rr, req.Request)
		if rr.Code != http.StatusOK {
			t.Fatalf("forced request %d expected 200, got %d body=%s", i+1, rr.Code, rr.Body.String())
		}
		log := waitForGatewayLogAfter(t, st, lastLogID)
		lastLogID = log.ID
		assertCpaRuntimeLog(t, log, accountA, "cpa-a-key", fmt.Sprintf("idx-%d", accountA), fmt.Sprintf("auth-%d", accountA))
	}
	assertLatestUpstreamHits(t, &mu, hits, 10, "cpa-a-key", fmt.Sprintf("idx-%d", accountA), fmt.Sprintf("auth-%d", accountA), "openai-cpa-a")

	if err := st.MarkAccountServingFailure(accountA, "forced cooldown", time.Now().UTC().Add(5*time.Minute)); err != nil {
		t.Fatalf("MarkAccountServingFailure: %v", err)
	}
	cache.Invalidate()

	req := authenticatedRequest(handler, token, `{"model":"gpt-5-codex","input":"automatic"}`)
	rr := httptest.NewRecorder()
	req.ServeHTTP(rr, req.Request)
	if rr.Code != http.StatusOK {
		t.Fatalf("automatic request expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	log := waitForGatewayLogAfter(t, st, lastLogID)
	assertCpaRuntimeLog(t, log, accountB, "cpa-b-key", fmt.Sprintf("idx-%d", accountB), fmt.Sprintf("auth-%d", accountB))
	assertLatestUpstreamHits(t, &mu, hits, 1, "cpa-b-key", fmt.Sprintf("idx-%d", accountB), fmt.Sprintf("auth-%d", accountB), "openai-cpa-b")

	accA, err := st.GetAccount(accountA)
	if err != nil {
		t.Fatalf("GetAccount A: %v", err)
	}
	if accA.ServingStatus != "cooldown" || accA.LastError != "forced cooldown" {
		t.Fatalf("automatic request must not clear cooldown on skipped account A, got %+v", accA)
	}
	accB, err := st.GetAccount(accountB)
	if err != nil {
		t.Fatalf("GetAccount B: %v", err)
	}
	if accB.ServingStatus != "healthy" || accB.LastSuccessAt == "" {
		t.Fatalf("automatic request should record serving success only on account B, got %+v", accB)
	}
	accC, err := st.GetAccount(accountC)
	if err != nil {
		t.Fatalf("GetAccount C: %v", err)
	}
	if accC.LastSuccessAt != "" || accC.FailureCount != 0 {
		t.Fatalf("automatic request must not touch unused account C accounting, got %+v", accC)
	}
}

func TestGatewayCpaProviderPinningUnsupportedFailsClosedAfterBinding(t *testing.T) {
	st, cache, handler, token := newHandlerTestStore(t)
	upstreamHits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHits++
		w.WriteHeader(http.StatusOK)
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
	accountID := addCpaGatewayAccount(t, st, *token.PoolID, serviceID, "unsupported-cpa", "codex", "gpt-5-codex")
	handler.runtimeBinder = unsupportedRuntimeBinder{}
	cache.Invalidate()

	req := authenticatedRequest(handler, token, `{"model":"gpt-5-codex","input":"hi"}`)
	rr := httptest.NewRecorder()
	req.ServeHTTP(rr, req.Request)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%s", rr.Code, rr.Body.String())
	}
	if upstreamHits != 0 {
		t.Fatalf("expected fail closed before CPA provider, got %d upstream hits", upstreamHits)
	}
	log := waitForLatestGatewayLog(t, st)
	if log.AccountID != accountID || log.RuntimeBindingStatus != "unsupported" || log.RuntimeBindingReason != "provider_pinning_unsupported" || log.RuntimeAuthIndex == "" {
		t.Fatalf("expected unsupported provider pinning log with resolved binding, got %+v", log)
	}
}

func TestGatewayCpaBindingUnavailableFailsClosed(t *testing.T) {
	st, cache, handler, token := newHandlerTestStore(t)
	upstreamHits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHits++
		w.WriteHeader(http.StatusOK)
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
	accountID := addCpaGatewayAccount(t, st, *token.PoolID, serviceID, "codex", "codex", "gpt-5-codex")
	handler.runtimeBinder = staticRuntimeBinder{err: fmt.Errorf("auth index missing")}
	cache.Invalidate()

	req := authenticatedRequest(handler, token, `{"model":"gpt-5-codex","input":"hi"}`)
	rr := httptest.NewRecorder()
	req.ServeHTTP(rr, req.Request)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "runtime_auth_binding_unavailable") {
		t.Fatalf("expected runtime binding error body, got %s", rr.Body.String())
	}
	if upstreamHits != 0 {
		t.Fatalf("expected fail closed before CPA provider, got %d upstream hits", upstreamHits)
	}
	log := waitForLatestGatewayLog(t, st)
	if log.AccountID != accountID || log.RuntimeBindingStatus == "" || log.RuntimeBindingReason == "" {
		t.Fatalf("expected failed binding log for account %d, got %+v", accountID, log)
	}
}

func TestGatewayCpaRetryExhaustedPreservesRuntimeBindingLog(t *testing.T) {
	st, cache, handler, token := newHandlerTestStore(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"temporary upstream failure"}}`))
	}))
	defer server.Close()

	if err := st.SetSetting("max_retry_attempts", "2"); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	serviceID, err := st.CreateCpaService(&store.CpaService{
		Label:   "CPA",
		BaseURL: server.URL,
		APIKey:  "service-key",
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("CreateCpaService: %v", err)
	}
	accountID := addCpaGatewayAccount(t, st, *token.PoolID, serviceID, "retry-cpa", "codex", "gpt-5-codex")
	handler.runtimeBinder = staticRuntimeBinder{}
	cache.Invalidate()

	req := authenticatedRequest(handler, token, `{"model":"gpt-5-codex","input":"hi"}`)
	rr := httptest.NewRecorder()
	req.ServeHTTP(rr, req.Request)
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 after retry exhaustion, got %d body=%s", rr.Code, rr.Body.String())
	}

	log := waitForLatestGatewayLog(t, st)
	if log.AccountID != accountID || log.RuntimeBindingStatus != "confirmed" || log.RuntimeAuthIndex == "" || log.RuntimeAuthID == "" {
		t.Fatalf("expected retry exhaustion log to preserve confirmed binding for account %d, got %+v", accountID, log)
	}
}

func TestGatewayCpaSuccessDoesNotClearQuotaState(t *testing.T) {
	st, cache, handler, token := newHandlerTestStore(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"usage": map[string]any{"input_tokens": 1}})
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
	accountID := addCpaGatewayAccount(t, st, *token.PoolID, serviceID, "suspect-cpa", "codex", "gpt-5-codex")
	if err := st.UpdateAccountCpaCredentialStatus(accountID, "auth_suspect", "quota_probe_failed", "quota probe failed", time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("UpdateAccountCpaCredentialStatus: %v", err)
	}
	if err := st.UpdateAccountCodexQuotaStatus(accountID, "error", "HTTP 403", time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("UpdateAccountCodexQuotaStatus: %v", err)
	}
	handler.runtimeBinder = staticRuntimeBinder{}
	cache.Invalidate()

	req := authenticatedRequest(handler, token, `{"model":"gpt-5-codex","input":"hi"}`)
	rr := httptest.NewRecorder()
	req.ServeHTTP(rr, req.Request)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	waitForGatewayTest(t, func() bool {
		acc, err := st.GetAccount(accountID)
		return err == nil && acc != nil &&
			acc.ServingStatus == "healthy" &&
			acc.CpaCredentialStatus == "ok" &&
			acc.CpaQuotaStatus == "error" &&
			acc.CpaQuotaLastError == "HTTP 403" &&
			acc.CpaSubscriptionStatus == "active"
	})
}

func TestGatewayCpaServiceAuthFailureDoesNotMarkAccountNeedsLogin(t *testing.T) {
	st, cache, handler, token := newHandlerTestStore(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid api key"}}`))
	}))
	defer server.Close()

	serviceID, err := st.CreateCpaService(&store.CpaService{
		Label:   "CPA",
		BaseURL: server.URL,
		APIKey:  "bad-service-key",
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("CreateCpaService: %v", err)
	}
	accountID := addCpaGatewayAccount(t, st, *token.PoolID, serviceID, "svc-auth-cpa", "codex", "gpt-5-codex")
	handler.runtimeBinder = staticRuntimeBinder{}
	cache.Invalidate()

	req := authenticatedRequest(handler, token, `{"model":"gpt-5-codex","input":"hi"}`)
	rr := httptest.NewRecorder()
	req.ServeHTTP(rr, req.Request)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%s", rr.Code, rr.Body.String())
	}
	acc, err := st.GetAccount(accountID)
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
	if acc.CpaCredentialStatus == "needs_login" {
		t.Fatalf("service auth failure must not mark account needs_login: %+v", acc)
	}
}

func TestChatStreamWithoutDoneLogsFailure(t *testing.T) {
	st, cache, handler, token := newHandlerTestStore(t)
	upstream := newSSETestServer(t, http.StatusOK, []string{
		`data: {"choices":[{"delta":{"content":"hello"}}]}`,
		`data: {"usage":{"prompt_tokens":11,"completion_tokens":7}}`,
	})
	addOpenAICompatGatewayAccount(t, st, cache, *token.PoolID, "chat-account", upstream.URL+"/v1", "gpt-test")

	req := authenticatedRequestPath(handler, token, "/v1/chat/completions", `{"model":"gpt-test","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	rr := httptest.NewRecorder()
	req.ServeHTTP(rr, req.Request)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected downstream 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	log := waitForLatestGatewayLog(t, st)
	if log.Success {
		t.Fatalf("expected stream log failure, got %+v", log)
	}
	if log.StatusCode != http.StatusOK {
		t.Fatalf("expected upstream status 200, got %+v", log)
	}
	if !strings.Contains(log.ErrorMessage, "stream closed before [DONE]") {
		t.Fatalf("expected missing [DONE] message, got %+v", log)
	}
	if log.InputTokens != 11 || log.OutputTokens != 7 {
		t.Fatalf("expected parsed usage to be preserved, got %+v", log)
	}
}

func TestResponsesStreamWithoutCompletedLogsFailure(t *testing.T) {
	st, cache, handler, token := newHandlerTestStore(t)
	upstream := newSSETestServer(t, http.StatusOK, []string{
		`data: {"type":"response.output_text.delta","delta":"hello"}`,
	})
	addOpenAICompatGatewayAccount(t, st, cache, *token.PoolID, "responses-account", upstream.URL+"/v1", "gpt-test")

	req := authenticatedRequest(handler, token, `{"model":"gpt-test","stream":true,"input":"hi"}`)
	rr := httptest.NewRecorder()
	req.ServeHTTP(rr, req.Request)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected downstream 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	log := waitForLatestGatewayLog(t, st)
	if log.Success {
		t.Fatalf("expected stream log failure, got %+v", log)
	}
	if !strings.Contains(log.ErrorMessage, "stream closed before response.completed") {
		t.Fatalf("expected missing response.completed message, got %+v", log)
	}
}

func TestResponsesStreamDoneWithoutCompletedLogsFailure(t *testing.T) {
	st, cache, handler, token := newHandlerTestStore(t)
	upstream := newSSETestServer(t, http.StatusOK, []string{
		`data: [DONE]`,
	})
	addOpenAICompatGatewayAccount(t, st, cache, *token.PoolID, "responses-done-account", upstream.URL+"/v1", "gpt-test")

	req := authenticatedRequest(handler, token, `{"model":"gpt-test","stream":true,"input":"hi"}`)
	rr := httptest.NewRecorder()
	req.ServeHTTP(rr, req.Request)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected downstream 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	log := waitForLatestGatewayLog(t, st)
	if log.Success {
		t.Fatalf("expected Responses [DONE] without response.completed to fail, got %+v", log)
	}
	if !strings.Contains(log.ErrorMessage, "stream closed before response.completed") {
		t.Fatalf("expected missing response.completed message, got %+v", log)
	}
}

func TestResponsesLongStreamTimeoutLogsFailure(t *testing.T) {
	st, cache, handler, token := newHandlerTestStore(t)
	if err := st.SetSetting("request_timeout", "1"); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	cache.Invalidate()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`data: {"type":"response.output_text.delta","delta":"hello"}` + "\n"))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		select {
		case <-r.Context().Done():
		case <-time.After(3 * time.Second):
		}
	}))
	defer upstream.Close()
	accountID := addOpenAICompatGatewayAccount(t, st, cache, *token.PoolID, "responses-timeout-account", upstream.URL+"/v1", "gpt-test")

	req := authenticatedRequest(handler, token, `{"model":"gpt-test","stream":true,"input":"hi"}`)
	rr := httptest.NewRecorder()
	req.ServeHTTP(rr, req.Request)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected downstream 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	log := waitForLatestGatewayLog(t, st)
	if log.Success {
		t.Fatalf("expected timeout stream log failure, got %+v", log)
	}
	if log.StatusCode != http.StatusOK {
		t.Fatalf("expected upstream status 200, got %+v", log)
	}
	if log.ErrorMessage != "stream timed out before response.completed" {
		t.Fatalf("expected normalized stream timeout message, got %+v", log)
	}
	if !log.Stream {
		t.Fatalf("expected stream log, got %+v", log)
	}
	if log.PoolID != *token.PoolID || log.AccountID != accountID || log.SourceKind != "openai_compat" {
		t.Fatalf("expected route metadata to be preserved, got %+v", log)
	}
	if log.LatencyMs <= 0 {
		t.Fatalf("expected latency to be recorded, got %+v", log)
	}
	if log.AttemptCount != 1 {
		t.Fatalf("expected one upstream attempt, got %+v", log)
	}
}

func TestResponsesStreamFailedLogsMessage(t *testing.T) {
	st, cache, handler, token := newHandlerTestStore(t)
	upstreamMsg := "Selected model is at capacity. Please try a different model"
	upstream := newSSETestServer(t, http.StatusOK, []string{
		`data: {"type":"response.failed","response":{"error":{"message":` + strconv.Quote(upstreamMsg) + `}}}`,
	})
	accountID := addOpenAICompatGatewayAccount(t, st, cache, *token.PoolID, "capacity-account", upstream.URL+"/v1", "gpt-test")

	req := authenticatedRequest(handler, token, `{"model":"gpt-test","stream":true,"input":"hi"}`)
	rr := httptest.NewRecorder()
	req.ServeHTTP(rr, req.Request)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected downstream 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	log := waitForLatestGatewayLog(t, st)
	if log.Success {
		t.Fatalf("expected stream log failure, got %+v", log)
	}
	if log.ErrorMessage != upstreamMsg {
		t.Fatalf("expected upstream message %q, got %+v", upstreamMsg, log)
	}
	waitForGatewayTest(t, func() bool {
		acc, err := st.GetAccount(accountID)
		return err == nil && acc != nil && acc.Status == "healthy" && acc.ServingStatus == "cooldown" && acc.LastError == upstreamMsg
	})
}

func TestStreamRetryableStatusSingleAttemptUpdatesHealth(t *testing.T) {
	st, cache, handler, token := newHandlerTestStore(t)
	if err := st.SetSetting("max_retry_attempts", "1"); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	cache.Invalidate()

	upstreamMsg := "transient upstream overload"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":` + strconv.Quote(upstreamMsg) + `}}`))
	}))
	defer upstream.Close()
	accountID := addOpenAICompatGatewayAccount(t, st, cache, *token.PoolID, "single-attempt-500-account", upstream.URL+"/v1", "gpt-test")

	req := authenticatedRequest(handler, token, `{"model":"gpt-test","stream":true,"input":"hi"}`)
	rr := httptest.NewRecorder()
	req.ServeHTTP(rr, req.Request)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected downstream 500, got %d body=%s", rr.Code, rr.Body.String())
	}
	log := waitForLatestGatewayLog(t, st)
	if log.Success || log.ErrorMessage != upstreamMsg {
		t.Fatalf("expected failed stream log with upstream message, got %+v", log)
	}
	waitForGatewayTest(t, func() bool {
		acc, err := st.GetAccount(accountID)
		return err == nil && acc != nil && acc.Status == "healthy" && acc.ServingStatus == "cooldown" && acc.LastError == upstreamMsg
	})
}

func TestCpaStreamRetryableStatusUpdatesHealthAndPreservesMessage(t *testing.T) {
	st, cache, handler, token := newHandlerTestStore(t)
	failedMsg := "CPA upstream EOF while contacting model"
	cpaUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/api/provider/good/") {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`data: {"type":"response.completed","usage":{"prompt_tokens":3,"completion_tokens":4}}` + "\n"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":` + strconv.Quote(failedMsg) + `}}`))
	}))
	defer cpaUpstream.Close()

	serviceID, err := st.CreateCpaService(&store.CpaService{
		Label:   "test-cpa-service",
		BaseURL: cpaUpstream.URL,
		APIKey:  "service-key",
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("CreateCpaService: %v", err)
	}
	badAccountID := addCpaGatewayAccount(t, st, *token.PoolID, serviceID, "bad-cpa", "bad", "gpt-5-codex")
	goodAccountID := addCpaGatewayAccount(t, st, *token.PoolID, serviceID, "good-cpa", "good", "gpt-5-codex")
	handler.runtimeBinder = staticRuntimeBinder{}
	cache.Invalidate()

	req := authenticatedRequest(handler, token, `{"model":"gpt-5-codex","stream":true,"input":"hi"}`)
	rr := httptest.NewRecorder()
	req.ServeHTTP(rr, req.Request)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected downstream 500, got %d body=%s", rr.Code, rr.Body.String())
	}
	log := waitForLatestGatewayLog(t, st)
	if log.Success || log.AccountID != badAccountID || log.ErrorMessage != failedMsg {
		t.Fatalf("expected failed CPA log with extracted message, got %+v", log)
	}
	firstLogID := log.ID
	waitForGatewayTest(t, func() bool {
		acc, err := st.GetAccount(badAccountID)
		if err != nil || acc == nil || acc.Status != "healthy" || acc.ServingStatus != "cooldown" || acc.LastError != failedMsg {
			return false
		}
		snap := cache.Get()
		cached := snap.Accounts[badAccountID]
		return cached != nil && cached.ServingStatus == "cooldown"
	})

	req = authenticatedRequest(handler, token, `{"model":"gpt-5-codex","stream":true,"input":"hi again"}`)
	rr = httptest.NewRecorder()
	req.ServeHTTP(rr, req.Request)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected downstream 200 on second account, got %d body=%s", rr.Code, rr.Body.String())
	}
	log = waitForGatewayLogAfter(t, st, firstLogID)
	if !log.Success || log.AccountID != goodAccountID {
		t.Fatalf("expected next request to use healthy CPA account %d, got %+v", goodAccountID, log)
	}
}

type authedGatewayRequest struct {
	http.Handler
	*http.Request
}

func authenticatedRequest(handler *Handler, token *store.AccessToken, body string) authedGatewayRequest {
	return authenticatedRequestPath(handler, token, "/v1/responses", body)
}

func authenticatedRequestPath(handler *Handler, token *store.AccessToken, path, body string) authedGatewayRequest {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token.Token)
	return authedGatewayRequest{
		Handler: auth.GatewayAuth(handler, handler.cache),
		Request: req,
	}
}

func newSSETestServer(t *testing.T, status int, lines []string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(status)
		for _, line := range lines {
			_, _ = w.Write([]byte(line + "\n"))
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func addOpenAICompatGatewayAccount(t *testing.T, st *store.Store, cache *store.RoutingCache, poolID int64, label, baseURL, model string) int64 {
	t.Helper()
	accountID, err := st.CreateAccount(&store.Account{
		Label:      label,
		SourceKind: "openai_compat",
		BaseURL:    baseURL,
		APIKey:     "sk-upstream",
		Provider:   "openai",
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	if _, err := st.AddPoolMember(poolID, accountID); err != nil {
		t.Fatalf("AddPoolMember: %v", err)
	}
	if err := st.RefreshAccountModels(accountID, []string{model}); err != nil {
		t.Fatalf("RefreshAccountModels: %v", err)
	}
	cache.Invalidate()
	return accountID
}

func addCpaGatewayAccount(t *testing.T, st *store.Store, poolID, serviceID int64, label, provider, model string) int64 {
	t.Helper()
	accountID, err := st.CreateAccount(&store.Account{
		Label:                 label,
		SourceKind:            "cpa",
		CpaServiceID:          &serviceID,
		CpaProvider:           provider,
		CpaAccountKey:         label + "-key",
		CpaOpenaiID:           "openai-" + label,
		CpaCredentialStatus:   "ok",
		CpaSubscriptionStatus: "active",
		Enabled:               true,
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	if _, err := st.AddPoolMember(poolID, accountID); err != nil {
		t.Fatalf("AddPoolMember: %v", err)
	}
	if err := st.RefreshAccountModels(accountID, []string{model}); err != nil {
		t.Fatalf("RefreshAccountModels: %v", err)
	}
	return accountID
}

type staticRuntimeBinder struct {
	err error
}

func (b staticRuntimeBinder) ProviderPinningSupported() bool {
	return true
}

func (b staticRuntimeBinder) ResolveRuntimeBinding(_ context.Context, acc store.Account, _ bool, _ bool) (*health.RuntimeBinding, error) {
	if b.err != nil {
		return nil, b.err
	}
	return &health.RuntimeBinding{
		AccountID:     acc.ID,
		AccountKey:    acc.CpaAccountKey,
		AuthID:        fmt.Sprintf("auth-%d", acc.ID),
		AuthIndex:     fmt.Sprintf("idx-%d", acc.ID),
		OpenAIID:      acc.CpaOpenaiID,
		Provider:      acc.CpaProvider,
		BindingStatus: "confirmed",
	}, nil
}

type fixedRuntimeBinder struct {
	authID    string
	authIndex string
}

type unsupportedRuntimeBinder struct{}

func (b unsupportedRuntimeBinder) ResolveRuntimeBinding(_ context.Context, acc store.Account, _ bool, _ bool) (*health.RuntimeBinding, error) {
	return &health.RuntimeBinding{
		AccountID:     acc.ID,
		AccountKey:    acc.CpaAccountKey,
		AuthID:        fmt.Sprintf("auth-%d", acc.ID),
		AuthIndex:     fmt.Sprintf("idx-%d", acc.ID),
		Provider:      acc.CpaProvider,
		BindingStatus: "confirmed",
	}, nil
}

func (b fixedRuntimeBinder) ProviderPinningSupported() bool {
	return true
}

func (b fixedRuntimeBinder) ResolveRuntimeBinding(_ context.Context, acc store.Account, _ bool, _ bool) (*health.RuntimeBinding, error) {
	return &health.RuntimeBinding{
		AccountID:     acc.ID,
		AccountKey:    acc.CpaAccountKey,
		AuthID:        b.authID,
		AuthIndex:     b.authIndex,
		OpenAIID:      acc.CpaOpenaiID,
		Provider:      acc.CpaProvider,
		BindingStatus: "confirmed",
	}, nil
}

func assertLatestLogStatus(t *testing.T, st *store.Store, expected int) {
	t.Helper()
	for range 20 {
		log := waitForLatestGatewayLog(t, st)
		if log.ID > 0 {
			if log.StatusCode != expected {
				t.Fatalf("expected latest status %d, got %+v", expected, log)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected a request log with status %d", expected)
}

func waitForLatestGatewayLog(t *testing.T, st *store.Store) store.RequestLog {
	t.Helper()
	return waitForGatewayLogAfter(t, st, 0)
}

func waitForGatewayLogAfter(t *testing.T, st *store.Store, afterID int64) store.RequestLog {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		logs, _, err := st.ListLogs(1, 0)
		if err != nil {
			t.Fatalf("ListLogs: %v", err)
		}
		if len(logs) > 0 && logs[0].ID > afterID {
			return logs[0]
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected a request log after id %d", afterID)
	return store.RequestLog{}
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

func assertCpaRuntimeLog(t *testing.T, log store.RequestLog, accountID int64, accountKey, authIndex, authID string) {
	t.Helper()
	if log.AccountID != accountID ||
		log.SourceKind != "cpa" ||
		!log.Success ||
		log.RuntimeBindingStatus != "confirmed" ||
		log.RuntimeBindingReason != "" ||
		log.RuntimeAccountKey != accountKey ||
		log.RuntimeAuthIndex != authIndex ||
		log.RuntimeAuthID != authID {
		t.Fatalf("unexpected CPA runtime accounting log for account %d: %+v", accountID, log)
	}
}

func assertLatestUpstreamHits(t *testing.T, mu *sync.Mutex, hits []cpaUpstreamHit, count int, accountKey, authIndex, authID, openAIID string) {
	t.Helper()
	mu.Lock()
	defer mu.Unlock()
	if len(hits) < count {
		t.Fatalf("expected at least %d upstream hits, got %d", count, len(hits))
	}
	for _, hit := range hits[len(hits)-count:] {
		if hit.AccountKey != accountKey ||
			hit.RuntimeAuthIndex != authIndex ||
			hit.RuntimeAuthID != authID ||
			hit.PinnedAuthIndex != authIndex ||
			hit.PinnedAuthID != authID ||
			hit.LegacyAuthIndex != authIndex ||
			hit.OpenAIID != openAIID {
			t.Fatalf("unexpected CPA runtime pinning headers: %+v", hit)
		}
	}
}

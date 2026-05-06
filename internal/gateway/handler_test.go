package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
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
		return err == nil && acc != nil && acc.Status == "error" && acc.LastError == upstreamMsg
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
		return err == nil && acc != nil && acc.Status == "error" && acc.LastError == upstreamMsg
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
		if err != nil || acc == nil || acc.Status != "error" || acc.LastError != failedMsg {
			return false
		}
		snap := cache.Get()
		cached := snap.Accounts[badAccountID]
		return cached != nil && cached.Status == "error"
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
		Label:        label,
		SourceKind:   "cpa",
		CpaServiceID: &serviceID,
		CpaProvider:  provider,
		Enabled:      true,
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

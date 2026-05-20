package admin

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"lune/internal/cpa"
	"lune/internal/health"
	"lune/internal/notify"
	"lune/internal/notify/drivers"
	"lune/internal/store"
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

func newTestNotifier(st *store.Store) *notify.Service {
	return notify.NewServiceWithRegistry(
		st,
		notify.NewRegistry(drivers.NewWeChatWorkBotDriver()),
	)
}

func requireRecentOperation(t *testing.T, st *store.Store, operationType string) store.Operation {
	t.Helper()
	ops, err := st.ListRecentOperations(20)
	if err != nil {
		t.Fatalf("list recent operations: %v", err)
	}
	for _, op := range ops {
		if op.OperationType == operationType {
			return op
		}
	}
	t.Fatalf("expected recent operation %q, got %+v", operationType, ops)
	return store.Operation{}
}

func fakeJWT(t *testing.T, claims map[string]any) string {
	t.Helper()
	encode := func(v any) string {
		t.Helper()
		data, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal jwt part: %v", err)
		}
		return base64.RawURLEncoding.EncodeToString(data)
	}
	return encode(map[string]any{"alg": "none", "typ": "JWT"}) + "." + encode(claims) + "."
}

func multipartAuthJSONBody(t *testing.T, filename string, fields map[string]string, payload string) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("write multipart field: %v", err)
		}
	}
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write([]byte(payload)); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	return body, writer.FormDataContentType()
}

func multipartAuthJSONBatchBody(t *testing.T, fields map[string]string, files map[string]string) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("write multipart field: %v", err)
		}
	}
	for filename, payload := range files {
		part, err := writer.CreateFormFile("files", filename)
		if err != nil {
			t.Fatalf("create form file: %v", err)
		}
		if _, err := part.Write([]byte(payload)); err != nil {
			t.Fatalf("write form file: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	return body, writer.FormDataContentType()
}

func codexAuthJSON(t *testing.T, email, accountID, refreshToken string) string {
	t.Helper()
	token := fakeJWT(t, map[string]any{
		"email":                             email,
		"chatgpt_account_id":                accountID,
		"chatgpt_plan_type":                 "plus",
		"chatgpt_subscription_active_until": time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339),
		"https://api.openai.com/profile":    map[string]any{"email": email},
		"https://api.openai.com/auth":       map[string]any{"chatgpt_account_id": accountID, "chatgpt_plan_type": "plus"},
	})
	payload, err := json.Marshal(map[string]any{
		"type":          "codex",
		"email":         email,
		"account_id":    accountID,
		"refresh_token": refreshToken,
		"access_token":  token,
		"id_token":      token,
		"disabled":      false,
		"last_refresh":  time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("marshal auth json: %v", err)
	}
	return string(payload)
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

	handler := NewHandler(st, cache, authDir, "", nil, newTestNotifier(st))
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

func TestBatchImportCpaAccountsIsIdempotentForDuplicateKey(t *testing.T) {
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
	poolID, err := st.CreatePool("Pool", 0, true)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	if err := cpa.WriteAuthFile(authDir, &cpa.CpaAuthFile{
		AccountID:   "acct_123",
		Email:       "batch@example.com",
		Type:        "codex",
		Disabled:    false,
		LastRefresh: "2026-05-10T12:00:00Z",
	}, "codex-batch@example.com-plus"); err != nil {
		t.Fatalf("write auth file: %v", err)
	}

	handler := NewHandler(st, cache, authDir, "", nil, newTestNotifier(st))
	body := bytes.NewBufferString(fmt.Sprintf(`{"service_id":%d,"account_keys":["codex-batch@example.com-plus","codex-batch@example.com-plus"],"pool_id":%d}`, svcID, poolID))
	req := httptest.NewRequest(http.MethodPost, "/admin/api/accounts/cpa/import/batch", body)
	rr := httptest.NewRecorder()

	handler.batchImportCpaAccounts(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
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
	if resp.Data.Imported != 1 || resp.Data.Skipped != 1 || len(resp.Data.Errors) != 0 {
		t.Fatalf("unexpected import result: %+v", resp.Data)
	}
	accounts, err := st.ListAccounts()
	if err != nil {
		t.Fatalf("list accounts: %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("expected one account after duplicate import, got %d", len(accounts))
	}
	members, err := st.ListPoolMembers(poolID)
	if err != nil {
		t.Fatalf("list pool members: %v", err)
	}
	if len(members) != 1 {
		t.Fatalf("expected one pool member after duplicate import, got %d", len(members))
	}
}

func TestImportCpaAuthJSONCreatesAccountAndDoesNotLeakTokens(t *testing.T) {
	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	authDir := t.TempDir()
	if _, err := st.CreateCpaService(&store.CpaService{
		Label:   "CPA",
		BaseURL: "https://cpa.example.com",
		APIKey:  "service-key",
		Enabled: true,
	}); err != nil {
		t.Fatalf("create cpa service: %v", err)
	}
	poolID, err := st.CreatePool("Codex", 0, true)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	handler := NewHandler(st, cache, authDir, "", nil, newTestNotifier(st))
	refreshToken := "refresh-secret-123"
	body, contentType := multipartAuthJSONBody(t, "../../x.json", map[string]string{
		"pool_id": fmt.Sprint(poolID),
		"label":   "Uploaded Codex",
	}, codexAuthJSON(t, "upload@example.com", "acct_upload", refreshToken))
	req := httptest.NewRequest(http.MethodPost, "/admin/api/accounts/cpa/import-json", body)
	req.Header.Set("Content-Type", contentType)
	rr := httptest.NewRecorder()

	handler.importCpaAuthJSON(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), refreshToken) {
		t.Fatalf("response leaked refresh token: %s", rr.Body.String())
	}
	accounts, err := st.ListAccounts()
	if err != nil {
		t.Fatalf("list accounts: %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("expected one account, got %d", len(accounts))
	}
	if accounts[0].Label != "Uploaded Codex" || accounts[0].CpaAccountKey != "codex-upload@example.com-plus" {
		t.Fatalf("unexpected imported account: %+v", accounts[0])
	}
	members, err := st.ListPoolMembers(poolID)
	if err != nil {
		t.Fatalf("list pool members: %v", err)
	}
	if len(members) != 1 || members[0].AccountID != accounts[0].ID {
		t.Fatalf("expected imported account in pool, got %+v", members)
	}
	if _, err := os.Stat(filepath.Join(authDir, "codex-upload@example.com-plus.json")); err != nil {
		t.Fatalf("expected generated auth file inside auth dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(authDir, "x.json")); !os.IsNotExist(err) {
		t.Fatalf("user supplied filename should not be used, stat err=%v", err)
	}
}

func TestImportCpaAuthJSONIsIdempotentForSameAccount(t *testing.T) {
	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	authDir := t.TempDir()
	if _, err := st.CreateCpaService(&store.CpaService{
		Label:   "CPA",
		BaseURL: "https://cpa.example.com",
		APIKey:  "service-key",
		Enabled: true,
	}); err != nil {
		t.Fatalf("create cpa service: %v", err)
	}
	poolID, err := st.CreatePool("Codex", 0, true)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	handler := NewHandler(st, cache, authDir, "", nil, newTestNotifier(st))
	for i := 0; i < 2; i++ {
		body, contentType := multipartAuthJSONBody(t, "auth.json", map[string]string{
			"pool_id": fmt.Sprint(poolID),
		}, codexAuthJSON(t, "repeat@example.com", "acct_repeat", fmt.Sprintf("refresh-%d", i)))
		req := httptest.NewRequest(http.MethodPost, "/admin/api/accounts/cpa/import-json", body)
		req.Header.Set("Content-Type", contentType)
		rr := httptest.NewRecorder()
		handler.importCpaAuthJSON(rr, req)
		if rr.Code != http.StatusCreated {
			t.Fatalf("import %d expected 201, got %d: %s", i, rr.Code, rr.Body.String())
		}
	}
	accounts, err := st.ListAccounts()
	if err != nil {
		t.Fatalf("list accounts: %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("expected idempotent account update, got %d accounts", len(accounts))
	}
	members, err := st.ListPoolMembers(poolID)
	if err != nil {
		t.Fatalf("list pool members: %v", err)
	}
	if len(members) != 1 {
		t.Fatalf("expected idempotent pool member, got %d", len(members))
	}
}

func TestPreviewCpaAuthJSONBatchDoesNotWriteAndSkipsDuplicate(t *testing.T) {
	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	authDir := t.TempDir()
	if _, err := st.CreateCpaService(&store.CpaService{
		Label:   "CPA",
		BaseURL: "https://cpa.example.com",
		Enabled: true,
	}); err != nil {
		t.Fatalf("create cpa service: %v", err)
	}
	poolID, err := st.CreatePool("Codex", 0, true)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	handler := NewHandler(st, cache, authDir, "", nil, newTestNotifier(st))
	authJSON := codexAuthJSON(t, "batch@example.com", "acct_batch", "refresh-secret")
	body, contentType := multipartAuthJSONBatchBody(t, map[string]string{
		"pool_id": fmt.Sprint(poolID),
	}, map[string]string{
		"a.json": authJSON,
		"b.json": authJSON,
	})
	req := httptest.NewRequest(http.MethodPost, "/admin/api/accounts/cpa/import-json-batch/preview", body)
	req.Header.Set("Content-Type", contentType)
	rr := httptest.NewRecorder()
	handler.previewCpaAuthJSONBatch(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "refresh-secret") {
		t.Fatalf("preview leaked token: %s", rr.Body.String())
	}
	var resp struct {
		Data cpaAuthJSONBatchResponse `json:"data"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Data.Summary.Created != 1 || resp.Data.Summary.Skipped != 1 {
		t.Fatalf("unexpected preview summary: %+v", resp.Data.Summary)
	}
	accounts, err := st.ListAccounts()
	if err != nil {
		t.Fatalf("list accounts: %v", err)
	}
	if len(accounts) != 0 {
		t.Fatalf("preview must not create accounts, got %d", len(accounts))
	}
	if entries, err := os.ReadDir(authDir); err != nil {
		t.Fatalf("read auth dir: %v", err)
	} else if len(entries) != 0 {
		t.Fatalf("preview must not write auth files, got %d entries", len(entries))
	}
}

func TestImportCpaAuthJSONBatchPartialSuccessAndDuplicate(t *testing.T) {
	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	authDir := t.TempDir()
	if _, err := st.CreateCpaService(&store.CpaService{
		Label:   "CPA",
		BaseURL: "https://cpa.example.com",
		Enabled: true,
	}); err != nil {
		t.Fatalf("create cpa service: %v", err)
	}
	poolID, err := st.CreatePool("Codex", 0, true)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	handler := NewHandler(st, cache, authDir, "", nil, newTestNotifier(st))
	first := codexAuthJSON(t, "first@example.com", "acct_first", "refresh-first")
	second := codexAuthJSON(t, "second@example.com", "acct_second", "refresh-second")
	body, contentType := multipartAuthJSONBatchBody(t, map[string]string{
		"pool_id": fmt.Sprint(poolID),
	}, map[string]string{
		"first.json":      first,
		"first-copy.json": first,
		"second.json":     second,
		"bad.json":        `{"type":"codex","email":"bad@example.com"}`,
	})
	req := httptest.NewRequest(http.MethodPost, "/admin/api/accounts/cpa/import-json-batch", body)
	req.Header.Set("Content-Type", contentType)
	rr := httptest.NewRecorder()
	handler.importCpaAuthJSONBatch(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "refresh-first") || strings.Contains(rr.Body.String(), "refresh-second") {
		t.Fatalf("response leaked token: %s", rr.Body.String())
	}
	var resp struct {
		Data cpaAuthJSONBatchResponse `json:"data"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Data.Summary.Created != 2 || resp.Data.Summary.Skipped != 1 || resp.Data.Summary.Failed != 1 {
		t.Fatalf("unexpected import summary: %+v", resp.Data.Summary)
	}
	if resp.Data.BatchID == "" {
		t.Fatalf("expected batch id in import response")
	}
	if resp.Data.Summary.PendingRuntimeSync != 2 {
		t.Fatalf("expected successful imports to remain runtime pending without checker, got %+v", resp.Data.Summary)
	}
	accounts, err := st.ListAccounts()
	if err != nil {
		t.Fatalf("list accounts: %v", err)
	}
	if len(accounts) != 2 {
		t.Fatalf("expected two imported accounts, got %d", len(accounts))
	}
	members, err := st.ListPoolMembers(poolID)
	if err != nil {
		t.Fatalf("list pool members: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("expected two pool members, got %d", len(members))
	}
	op, err := st.GetOperation(resp.Data.BatchID)
	if err != nil {
		t.Fatalf("get import operation: %v", err)
	}
	if op == nil {
		t.Fatalf("expected persisted import operation")
	}
	if op.OperationType != "cpa_import_batch" || op.Status != "partial" || op.TargetID != fmt.Sprint(poolID) {
		t.Fatalf("unexpected operation: %+v", op)
	}
	if len(op.Items) != 4 {
		t.Fatalf("expected 4 operation items, got %d", len(op.Items))
	}
	var seenDuplicate, seenInvalid bool
	for _, item := range op.Items {
		if strings.Contains(item.SafeErrorMessage, "refresh-first") || strings.Contains(item.SafeErrorMessage, "refresh-second") {
			t.Fatalf("operation item leaked refresh token: %+v", item)
		}
		if strings.Contains(item.AccountKeyHash, "first@example.com") || strings.Contains(item.AccountKeyHash, "codex-first") {
			t.Fatalf("operation item leaked account key: %+v", item)
		}
		if item.ErrorCode == "duplicate_in_batch" && item.Status == "skipped" && item.RuntimeSync == "not_applicable" {
			seenDuplicate = true
		}
		if item.ErrorCode == "invalid_auth_json" && item.Status == "failed" && item.Stage == "parse_input" {
			seenInvalid = true
		}
	}
	if !seenDuplicate || !seenInvalid {
		t.Fatalf("expected duplicate and invalid operation items, got %+v", op.Items)
	}

	reqRecent := httptest.NewRequest(http.MethodGet, "/admin/api/operations/recent", nil)
	rrRecent := httptest.NewRecorder()
	handler.listRecentOperations(rrRecent, reqRecent)
	if rrRecent.Code != http.StatusOK {
		t.Fatalf("expected recent operations 200, got %d: %s", rrRecent.Code, rrRecent.Body.String())
	}
	if !strings.Contains(rrRecent.Body.String(), resp.Data.BatchID) {
		t.Fatalf("recent operations did not include batch id: %s", rrRecent.Body.String())
	}

	reqOp := httptest.NewRequest(http.MethodGet, "/admin/api/operations/"+resp.Data.BatchID, nil)
	reqOp.SetPathValue("operation_id", resp.Data.BatchID)
	rrOp := httptest.NewRecorder()
	handler.getOperation(rrOp, reqOp)
	if rrOp.Code != http.StatusOK {
		t.Fatalf("expected operation detail 200, got %d: %s", rrOp.Code, rrOp.Body.String())
	}
	if strings.Contains(rrOp.Body.String(), "refresh-first") || strings.Contains(rrOp.Body.String(), "refresh-second") {
		t.Fatalf("operation detail leaked token: %s", rrOp.Body.String())
	}
}

func TestListAccountsIncludesDiagnosticSummary(t *testing.T) {
	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	accountID, err := st.CreateAccount(&store.Account{
		Label:      "Direct",
		SourceKind: "openai_compat",
		BaseURL:    "https://api.example.com",
		APIKey:     "sk-secret",
		Provider:   "openai",
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	handler := NewHandler(st, cache, t.TempDir(), "", nil, newTestNotifier(st))
	req := httptest.NewRequest(http.MethodGet, "/admin/api/accounts", nil)
	rr := httptest.NewRecorder()

	handler.listAccounts(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Data []store.Account `json:"data"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Data) != 1 || resp.Data[0].ID != accountID {
		t.Fatalf("unexpected accounts response: %+v", resp.Data)
	}
	account := resp.Data[0]
	if account.DiagnosticStatus != "unknown" || account.SchedulerStatus != "eligible_with_warning" || account.LastDiagnosedAt == "" {
		t.Fatalf("expected diagnostic summary fields, got %+v", account)
	}
	if account.Diagnostic == nil || account.Diagnostic.LastProbeStatus != "not_run" || account.Diagnostic.SafeSummary == "" {
		t.Fatalf("expected embedded diagnostic, got %+v", account.Diagnostic)
	}
	if strings.Contains(rr.Body.String(), "sk-secret") {
		t.Fatalf("account response leaked api key: %s", rr.Body.String())
	}
}

func TestListAccountsDoesNotExposeCpaAccountKey(t *testing.T) {
	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	serviceID, err := st.CreateCpaService(&store.CpaService{
		Label:   "CPA",
		BaseURL: "https://cpa.example.com",
		APIKey:  "service-key",
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("create cpa service: %v", err)
	}
	accountKey := "codex-secret@example.com-plus"
	if _, err := st.CreateAccount(&store.Account{
		Label:         "CPA",
		SourceKind:    "cpa",
		CpaServiceID:  &serviceID,
		CpaProvider:   "codex",
		CpaAccountKey: accountKey,
		CpaEmail:      "secret@example.com",
		CpaPlanType:   "plus",
		Enabled:       true,
	}); err != nil {
		t.Fatalf("create cpa account: %v", err)
	}
	handler := NewHandler(st, cache, t.TempDir(), "", nil, newTestNotifier(st))
	req := httptest.NewRequest(http.MethodGet, "/admin/api/accounts", nil)
	rr := httptest.NewRecorder()

	handler.listAccounts(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), accountKey) {
		t.Fatalf("account response leaked cpa account key: %s", rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "secret@example.com") {
		t.Fatalf("account response leaked cpa email: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"cpa_account_key_hash":"sha256:`) {
		t.Fatalf("expected cpa account key hash in response: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"cpa_email":"s***t@example.com"`) {
		t.Fatalf("expected masked cpa email in response: %s", rr.Body.String())
	}
}

func TestImportCpaAuthJSONRejectsUnsafeInputs(t *testing.T) {
	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	authDir := t.TempDir()
	if _, err := st.CreateCpaService(&store.CpaService{
		Label:   "CPA",
		BaseURL: "https://cpa.example.com",
		APIKey:  "service-key",
		Enabled: true,
	}); err != nil {
		t.Fatalf("create cpa service: %v", err)
	}
	poolID, err := st.CreatePool("Codex", 0, true)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	handler := NewHandler(st, cache, authDir, "", nil, newTestNotifier(st))
	cases := []struct {
		name     string
		filename string
		payload  string
		want     string
	}{
		{name: "login sessions filename", filename: ".login-sessions.json", payload: `{"sessions":[]}`, want: ".login-sessions"},
		{name: "login sessions shape", filename: "auth.json", payload: `{"sessions":[]}`, want: ".login-sessions"},
		{name: "missing refresh", filename: "auth.json", payload: `{"type":"codex","email":"x@example.com","access_token":"abc.def"}`, want: "credential field"},
		{name: "unsupported provider", filename: "auth.json", payload: `{"type":"openai","email":"x@example.com","refresh_token":"r","access_token":"abc.def"}`, want: "unsupported provider"},
		{name: "array json", filename: "auth.json", payload: `[]`, want: "object"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, contentType := multipartAuthJSONBody(t, tc.filename, map[string]string{
				"pool_id": fmt.Sprint(poolID),
			}, tc.payload)
			req := httptest.NewRequest(http.MethodPost, "/admin/api/accounts/cpa/import-json", body)
			req.Header.Set("Content-Type", contentType)
			rr := httptest.NewRecorder()
			handler.importCpaAuthJSON(rr, req)
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
			}
			if !strings.Contains(rr.Body.String(), tc.want) {
				t.Fatalf("expected response to mention %q, got %s", tc.want, rr.Body.String())
			}
		})
	}
}

func TestImportCpaAuthJSONRollsBackNewFileWhenPoolAddFails(t *testing.T) {
	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	authDir := t.TempDir()
	reloadSignal := filepath.Join(t.TempDir(), "cpa-reload.signal")
	var healthCalls int
	cpaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			t.Fatalf("unexpected CPA path %s", r.URL.Path)
		}
		healthCalls++
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer cpaServer.Close()
	if _, err := st.CreateCpaService(&store.CpaService{
		Label:   "CPA",
		BaseURL: cpaServer.URL,
		APIKey:  "service-key",
		Enabled: true,
	}); err != nil {
		t.Fatalf("create cpa service: %v", err)
	}
	checker := health.NewChecker(st, cache, authDir, "", newTestNotifier(st))
	checker.SetCpaReloadSignalPath(reloadSignal)
	handler := NewHandler(st, cache, authDir, "", checker, newTestNotifier(st))
	body, contentType := multipartAuthJSONBody(t, "auth.json", map[string]string{
		"pool_id": "999",
	}, codexAuthJSON(t, "rollback@example.com", "acct_rollback", "refresh-rollback"))
	req := httptest.NewRequest(http.MethodPost, "/admin/api/accounts/cpa/import-json", body)
	req.Header.Set("Content-Type", contentType)
	rr := httptest.NewRecorder()

	handler.importCpaAuthJSON(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
	if _, err := os.Stat(filepath.Join(authDir, "codex-rollback@example.com-plus.json")); !os.IsNotExist(err) {
		t.Fatalf("expected new auth file rollback, stat err=%v", err)
	}
	accounts, err := st.ListAccounts()
	if err != nil {
		t.Fatalf("list accounts: %v", err)
	}
	if len(accounts) != 0 {
		t.Fatalf("expected account rollback, got %+v", accounts)
	}
	if _, err := os.Stat(reloadSignal); err != nil {
		t.Fatalf("expected rollback path to request CPA runtime reload: %v", err)
	}
	if healthCalls != 1 {
		t.Fatalf("expected rollback path to request exactly one CPA runtime reload, got %d", healthCalls)
	}
}

func TestImportCpaAuthJSONBatchRequestsSingleRuntimeReload(t *testing.T) {
	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	authDir := t.TempDir()
	reloadSignal := filepath.Join(t.TempDir(), "cpa-reload.signal")
	var healthCalls int
	cpaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		healthCalls++
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer cpaServer.Close()
	if _, err := st.CreateCpaService(&store.CpaService{
		Label:   "CPA",
		BaseURL: cpaServer.URL,
		APIKey:  "service-key",
		Enabled: true,
	}); err != nil {
		t.Fatalf("create cpa service: %v", err)
	}
	poolID, err := st.CreatePool("Codex", 0, true)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	checker := health.NewChecker(st, cache, authDir, "", newTestNotifier(st))
	checker.SetCpaReloadSignalPath(reloadSignal)
	handler := NewHandler(st, cache, authDir, "", checker, newTestNotifier(st))

	body, contentType := multipartAuthJSONBatchBody(t, map[string]string{
		"pool_id": fmt.Sprint(poolID),
	}, map[string]string{
		"first.json":  codexAuthJSON(t, "batch-one@example.com", "acct_batch_one", "refresh-one"),
		"second.json": codexAuthJSON(t, "batch-two@example.com", "acct_batch_two", "refresh-two"),
	})
	req := httptest.NewRequest(http.MethodPost, "/admin/api/accounts/cpa/import-json-batch", body)
	req.Header.Set("Content-Type", contentType)
	rr := httptest.NewRecorder()

	handler.importCpaAuthJSONBatch(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if healthCalls != 1 {
		t.Fatalf("expected one batch-level CPA runtime reload, got %d", healthCalls)
	}
	var resp struct {
		Data cpaAuthJSONBatchResponse `json:"data"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Data.Summary.Created != 2 || resp.Data.Summary.PendingRuntimeSync != 2 || resp.Data.Summary.FailedRuntimeSync != 0 {
		t.Fatalf("unexpected import summary: %+v", resp.Data.Summary)
	}
}

func TestImportCpaAuthJSONBatchPersistsRuntimeReloadFailure(t *testing.T) {
	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	authDir := t.TempDir()
	reloadSignal := filepath.Join(t.TempDir(), "cpa-reload.signal")
	if _, err := st.CreateCpaService(&store.CpaService{
		Label:   "CPA",
		BaseURL: "http://127.0.0.1:1",
		APIKey:  "service-key",
		Enabled: true,
	}); err != nil {
		t.Fatalf("create cpa service: %v", err)
	}
	poolID, err := st.CreatePool("Codex", 0, true)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	checker := health.NewChecker(st, cache, authDir, "", newTestNotifier(st))
	checker.SetCpaReloadSignalPath(reloadSignal)
	handler := NewHandler(st, cache, authDir, "", checker, newTestNotifier(st))

	body, contentType := multipartAuthJSONBatchBody(t, map[string]string{
		"pool_id": fmt.Sprint(poolID),
	}, map[string]string{
		"first.json": codexAuthJSON(t, "reload-fail@example.com", "acct_reload_fail", "refresh-reload-fail"),
	})
	req := httptest.NewRequest(http.MethodPost, "/admin/api/accounts/cpa/import-json-batch", body)
	req.Header.Set("Content-Type", contentType)
	rr := httptest.NewRecorder()

	handler.importCpaAuthJSONBatch(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Data cpaAuthJSONBatchResponse `json:"data"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Data.Summary.Created != 1 || resp.Data.Summary.Failed != 0 || resp.Data.Summary.FailedRuntimeSync != 1 {
		t.Fatalf("unexpected import summary: %+v", resp.Data.Summary)
	}
	op, err := st.GetOperation(resp.Data.BatchID)
	if err != nil {
		t.Fatalf("get operation: %v", err)
	}
	if op == nil || op.Status != "partial" || len(op.Items) != 1 {
		t.Fatalf("unexpected operation: %+v", op)
	}
	item := op.Items[0]
	if item.Status != "created" || item.RuntimeSync != "failed" || item.ErrorCode != "runtime_reload_failed" || item.Stage != "request_runtime_reload" {
		t.Fatalf("expected runtime reload failure evidence, got %+v", item)
	}
	if strings.Contains(item.SafeErrorMessage, "refresh-reload-fail") || strings.Contains(item.AccountKeyHash, "reload-fail@example.com") {
		t.Fatalf("operation item leaked sensitive data: %+v", item)
	}
}

func TestImportCpaAuthJSONBatchFailsWhenAuditPersistFails(t *testing.T) {
	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	authDir := t.TempDir()
	if _, err := st.CreateCpaService(&store.CpaService{
		Label:   "CPA",
		BaseURL: "https://cpa.example.com",
		APIKey:  "service-key",
		Enabled: true,
	}); err != nil {
		t.Fatalf("create cpa service: %v", err)
	}
	poolID, err := st.CreatePool("Codex", 0, true)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	if _, err := st.DB().Exec(`DROP TABLE operation_items`); err != nil {
		t.Fatalf("drop operation_items: %v", err)
	}
	handler := NewHandler(st, cache, authDir, "", nil, newTestNotifier(st))

	body, contentType := multipartAuthJSONBatchBody(t, map[string]string{
		"pool_id": fmt.Sprint(poolID),
	}, map[string]string{
		"first.json": codexAuthJSON(t, "audit-fail@example.com", "acct_audit_fail", "refresh-audit-fail"),
	})
	req := httptest.NewRequest(http.MethodPost, "/admin/api/accounts/cpa/import-json-batch", body)
	req.Header.Set("Content-Type", contentType)
	rr := httptest.NewRecorder()

	handler.importCpaAuthJSONBatch(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 when audit persistence fails, got %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "audit_persist_failed") {
		t.Fatalf("expected audit_persist_failed response, got %s", rr.Body.String())
	}
}

func TestImportCpaAuthJSONBatchPersistsPoolMemberFailureStage(t *testing.T) {
	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	authDir := t.TempDir()
	if _, err := st.CreateCpaService(&store.CpaService{
		Label:   "CPA",
		BaseURL: "https://cpa.example.com",
		APIKey:  "service-key",
		Enabled: true,
	}); err != nil {
		t.Fatalf("create cpa service: %v", err)
	}
	poolID, err := st.CreatePool("Codex", 0, true)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	if _, err := st.DB().Exec(`CREATE TRIGGER fail_pool_member_insert BEFORE INSERT ON pool_members BEGIN SELECT RAISE(FAIL, 'forced pool member insert failure'); END`); err != nil {
		t.Fatalf("create pool member failure trigger: %v", err)
	}
	handler := NewHandler(st, cache, authDir, "", nil, newTestNotifier(st))

	body, contentType := multipartAuthJSONBatchBody(t, map[string]string{
		"pool_id": fmt.Sprint(poolID),
	}, map[string]string{
		"first.json": codexAuthJSON(t, "missing-pool@example.com", "acct_missing_pool", "refresh-missing-pool"),
	})
	req := httptest.NewRequest(http.MethodPost, "/admin/api/accounts/cpa/import-json-batch", body)
	req.Header.Set("Content-Type", contentType)
	rr := httptest.NewRecorder()

	handler.importCpaAuthJSONBatch(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Data cpaAuthJSONBatchResponse `json:"data"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Data.Summary.Failed != 1 {
		t.Fatalf("expected failed item, got %+v", resp.Data.Summary)
	}
	op, err := st.GetOperation(resp.Data.BatchID)
	if err != nil {
		t.Fatalf("get operation: %v", err)
	}
	if op == nil || op.Status != "failed" || len(op.Items) != 1 {
		t.Fatalf("unexpected operation: %+v", op)
	}
	item := op.Items[0]
	if item.Status != "failed" || item.ErrorCode == "" || item.Stage == "" || item.SafeErrorMessage == "" {
		t.Fatalf("expected structured DB failure evidence, got %+v", item)
	}
	if item.Stage != "select_max_position" && item.Stage != "select_pool_member" && item.Stage != "insert_pool_member" {
		t.Fatalf("expected pool member stage, got %+v", item)
	}
	if strings.Contains(item.SafeErrorMessage, "refresh-missing-pool") || strings.Contains(item.AccountKeyHash, "missing-pool@example.com") {
		t.Fatalf("operation item leaked sensitive data: %+v", item)
	}
}

func TestReloadFailureLogDoesNotExposeFullAccountKey(t *testing.T) {
	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	authDir := t.TempDir()
	reloadSignal := filepath.Join(t.TempDir(), "cpa-reload.signal")
	checker := health.NewChecker(st, cache, authDir, "", newTestNotifier(st))
	checker.SetCpaReloadSignalPath(reloadSignal)
	handler := NewHandler(st, cache, authDir, "", checker, newTestNotifier(st))

	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() {
		slog.SetDefault(previous)
	})

	fullKey := "codex-secret.user@example.com-plus"
	status, code, message := handler.reloadCpaRuntimeAfterAuthImport(&store.CpaService{
		Label:   "CPA",
		BaseURL: "http://127.0.0.1:1",
		Enabled: true,
	}, shortHash(fullKey), "upload")
	if status != "failed" {
		t.Fatalf("expected failed runtime sync, got %q", status)
	}
	if code != "runtime_reload_failed" || message == "" {
		t.Fatalf("expected runtime reload error detail, got code=%q message=%q", code, message)
	}
	text := logs.String()
	if strings.Contains(text, fullKey) || strings.Contains(text, "secret.user@example.com") {
		t.Fatalf("reload failure log leaked account key/email: %s", text)
	}
	if !strings.Contains(text, "target=") {
		t.Fatalf("expected sanitized target summary in log, got: %s", text)
	}
}

func TestImportCpaAuthJSONRestoresExistingAccountAndFileWhenPoolAddFails(t *testing.T) {
	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	authDir := t.TempDir()
	svcID, err := st.CreateCpaService(&store.CpaService{
		Label:   "CPA",
		BaseURL: "https://cpa.example.com",
		APIKey:  "service-key",
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("create cpa service: %v", err)
	}
	accountKey := "codex-existing@example.com-plus"
	oldAuth := &cpa.CpaAuthFile{
		Type:         "codex",
		Email:        "existing@example.com",
		AccountID:    "acct_existing",
		RefreshToken: "old-refresh",
		AccessToken:  fakeJWT(t, map[string]any{"email": "existing@example.com", "chatgpt_account_id": "acct_existing", "chatgpt_plan_type": "plus"}),
	}
	if err := cpa.WriteAuthFile(authDir, oldAuth, accountKey); err != nil {
		t.Fatalf("write old auth file: %v", err)
	}
	accountID, err := st.CreateAccount(&store.Account{
		Label:               "Existing Codex",
		SourceKind:          "cpa",
		CpaServiceID:        &svcID,
		CpaProvider:         "codex",
		CpaAccountKey:       accountKey,
		CpaEmail:            "existing@example.com",
		CpaPlanType:         "plus",
		CpaOpenaiID:         "acct_existing",
		CpaCredentialStatus: "ok",
		Enabled:             true,
		Notes:               "old notes",
	})
	if err != nil {
		t.Fatalf("create existing account: %v", err)
	}
	if err := st.UpdateAccountHealth(accountID, "error", "old failure"); err != nil {
		t.Fatalf("mark old health: %v", err)
	}

	handler := NewHandler(st, cache, authDir, "", nil, newTestNotifier(st))
	body, contentType := multipartAuthJSONBody(t, "auth.json", map[string]string{
		"pool_id": "999",
		"label":   "New Label",
		"notes":   "new notes",
	}, codexAuthJSON(t, "existing@example.com", "acct_existing", "new-refresh"))
	req := httptest.NewRequest(http.MethodPost, "/admin/api/accounts/cpa/import-json", body)
	req.Header.Set("Content-Type", contentType)
	rr := httptest.NewRecorder()

	handler.importCpaAuthJSON(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
	authAfter, err := cpa.ReadAuthFile(authDir, accountKey)
	if err != nil {
		t.Fatalf("read restored auth file: %v", err)
	}
	if authAfter.RefreshToken != "old-refresh" {
		t.Fatalf("expected old auth file to be restored, got refresh token %q", authAfter.RefreshToken)
	}
	accountAfter, err := st.GetAccount(accountID)
	if err != nil {
		t.Fatalf("get restored account: %v", err)
	}
	if accountAfter.Label != "Existing Codex" || accountAfter.Notes != "old notes" {
		t.Fatalf("expected existing account label/notes restored, got %+v", accountAfter)
	}
	if accountAfter.Status != "error" || accountAfter.LastError != "old failure" {
		t.Fatalf("expected existing account health restored, got status=%q last_error=%q", accountAfter.Status, accountAfter.LastError)
	}
	accounts, err := st.ListAccounts()
	if err != nil {
		t.Fatalf("list accounts: %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("expected no duplicate account after failed overwrite, got %+v", accounts)
	}
}

func TestUpsertImportedCpaAccountUpdatesExistingKey(t *testing.T) {
	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	handler := NewHandler(st, cache, t.TempDir(), "", nil, newTestNotifier(st))

	svcID, err := st.CreateCpaService(&store.CpaService{
		Label:   "CPA",
		BaseURL: "https://example.com",
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("create cpa service: %v", err)
	}
	svc, err := st.GetCpaServiceByID(svcID)
	if err != nil || svc == nil {
		t.Fatalf("get cpa service: %v", err)
	}

	first, err := handler.upsertImportedCpaAccount(svc, "codex-user@example.com-plus", &cpa.CpaAuthFile{
		AccountID:   "acct_old",
		Email:       "user@example.com",
		Type:        "codex",
		LastRefresh: "2026-05-10T08:00:00Z",
	}, "Original", true, "keep note")
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	second, err := handler.upsertImportedCpaAccount(svc, "codex-user@example.com-plus", &cpa.CpaAuthFile{
		AccountID:   "acct_new",
		Email:       "user@example.com",
		Type:        "codex",
		LastRefresh: "2026-05-10T12:00:00Z",
	}, "", true, "")
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("expected existing account to be updated, first=%d second=%d", first.ID, second.ID)
	}
	acc, err := st.GetAccount(first.ID)
	if err != nil {
		t.Fatalf("get account: %v", err)
	}
	if acc.CpaOpenaiID != "acct_new" || acc.CpaLastRefreshAt != "2026-05-10T12:00:00Z" {
		t.Fatalf("existing account not updated: %+v", acc)
	}
	if acc.Label != "Original" || acc.Notes != "keep note" {
		t.Fatalf("empty relogin fields should preserve label/notes, got label=%q notes=%q", acc.Label, acc.Notes)
	}
}

func TestUpsertImportedCodexAccountDerivesActiveSubscriptionStatus(t *testing.T) {
	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	handler := NewHandler(st, cache, t.TempDir(), "", nil, newTestNotifier(st))

	svcID, err := st.CreateCpaService(&store.CpaService{
		Label:   "CPA",
		BaseURL: "https://example.com",
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("create cpa service: %v", err)
	}
	svc, err := st.GetCpaServiceByID(svcID)
	if err != nil || svc == nil {
		t.Fatalf("get cpa service: %v", err)
	}

	account, err := handler.upsertImportedCpaAccount(svc, "codex-user@example.com-plus", &cpa.CpaAuthFile{
		AccountID: "acct_123",
		Email:     "user@example.com",
		Type:      "codex",
		IDToken: adminTestJWT(map[string]any{
			"email": "user@example.com",
			"https://api.openai.com/auth": map[string]any{
				"chatgpt_plan_type":                 "plus",
				"chatgpt_account_id":                "acct_123",
				"chatgpt_subscription_active_until": "2099-01-01T00:00:00+00:00",
			},
		}),
	}, "", true, "")
	if err != nil {
		t.Fatalf("upsert account: %v", err)
	}
	acc, err := st.GetAccount(account.ID)
	if err != nil {
		t.Fatalf("get account: %v", err)
	}
	if acc.CpaSubscriptionStatus != "active" || acc.CpaSubscriptionExpiresAt != "2099-01-01T00:00:00Z" || acc.CpaAccessStatus != "eligible" {
		t.Fatalf("expected active subscription status from imported auth file, got %+v", acc)
	}
}

func TestUpsertImportedCpaAccountClearsStaleHealthError(t *testing.T) {
	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	handler := NewHandler(st, cache, t.TempDir(), "", nil, newTestNotifier(st))

	svcID, err := st.CreateCpaService(&store.CpaService{
		Label:   "CPA",
		BaseURL: "https://example.com",
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("create cpa service: %v", err)
	}
	svc, err := st.GetCpaServiceByID(svcID)
	if err != nil || svc == nil {
		t.Fatalf("get cpa service: %v", err)
	}

	first, err := handler.upsertImportedCpaAccount(svc, "codex-user@example.com-plus", &cpa.CpaAuthFile{
		AccountID:   "acct_old",
		Email:       "user@example.com",
		Type:        "codex",
		LastRefresh: "2026-05-10T08:00:00Z",
	}, "Original", true, "")
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if err := st.UpdateAccountHealth(first.ID, "error", "Credential file not found"); err != nil {
		t.Fatalf("mark stale health error: %v", err)
	}

	second, err := handler.upsertImportedCpaAccount(svc, "codex-user@example.com-plus", &cpa.CpaAuthFile{
		AccountID:   "acct_new",
		Email:       "user@example.com",
		Type:        "codex",
		LastRefresh: "2026-05-10T12:00:00Z",
	}, "", true, "")
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("expected existing account to be updated, first=%d second=%d", first.ID, second.ID)
	}
	acc, err := st.GetAccount(first.ID)
	if err != nil {
		t.Fatalf("get account: %v", err)
	}
	if acc.Status != "healthy" || acc.LastError != "" {
		t.Fatalf("expected stale health error to be cleared, got status=%q last_error=%q", acc.Status, acc.LastError)
	}
	if acc.CpaCredentialStatus != "ok" || acc.CpaCredentialLastError != "" {
		t.Fatalf("expected credential state to be ok, got status=%q last_error=%q", acc.CpaCredentialStatus, acc.CpaCredentialLastError)
	}
}

func TestFinalizeLoginSameCpaKeyUpdatesExistingAccount(t *testing.T) {
	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	authDir := t.TempDir()
	handler := NewHandler(st, cache, authDir, "", nil, newTestNotifier(st))

	svcID, err := st.CreateCpaService(&store.CpaService{
		Label:   "CPA",
		BaseURL: "https://example.com",
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("create cpa service: %v", err)
	}
	svc, err := st.GetCpaServiceByID(svcID)
	if err != nil || svc == nil {
		t.Fatalf("get cpa service: %v", err)
	}
	poolID, err := st.CreatePool("Pool", 0, true)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	session := &cpa.LoginSession{
		ID:        "sess_test",
		ServiceID: svcID,
		PoolID:    poolID,
		Provider:  "codex",
	}

	handler.finalizeLogin(session, svc, &cpa.TokenResponse{
		AccessToken:  "access-1",
		RefreshToken: "refresh-1",
		IDToken: adminTestJWT(map[string]any{
			"email": "user@example.com",
			"https://api.openai.com/auth": map[string]any{
				"chatgpt_plan_type":  "plus",
				"chatgpt_account_id": "acct_old",
			},
		}),
		ExpiresIn: 3600,
	})
	handler.finalizeLogin(session, svc, &cpa.TokenResponse{
		AccessToken:  "access-2",
		RefreshToken: "refresh-2",
		IDToken: adminTestJWT(map[string]any{
			"email": "user@example.com",
			"https://api.openai.com/auth": map[string]any{
				"chatgpt_plan_type":  "plus",
				"chatgpt_account_id": "acct_new",
			},
		}),
		ExpiresIn: 3600,
	})

	accounts, err := st.ListAccounts()
	if err != nil {
		t.Fatalf("list accounts: %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("expected one account after relogin, got %d", len(accounts))
	}
	if accounts[0].CpaOpenaiID != "acct_new" {
		t.Fatalf("expected account to be updated with new token metadata, got %q", accounts[0].CpaOpenaiID)
	}
	members, err := st.ListPoolMembers(poolID)
	if err != nil {
		t.Fatalf("list pool members: %v", err)
	}
	if len(members) != 1 || members[0].AccountID != accounts[0].ID {
		t.Fatalf("expected one idempotent pool member, got %+v", members)
	}
}

func TestUpdateAccountPreservesExistingApiKeyWhenEmpty(t *testing.T) {
	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	handler := NewHandler(st, cache, t.TempDir(), "", nil, newTestNotifier(st))

	accountID, err := st.CreateAccount(&store.Account{
		Label:        "Direct",
		SourceKind:   "openai_compat",
		BaseURL:      "https://api.example.com/v1",
		APIKey:       "sk-old",
		Enabled:      true,
		Status:       "healthy",
		QuotaDisplay: "n/a",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	body := strings.NewReader(`{
		"label":"Direct",
		"source_kind":"openai_compat",
		"base_url":"https://api.example.com/v1",
		"api_key":"",
		"provider":"",
		"enabled":true,
		"notes":"",
		"quota_display":"n/a"
	}`)
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/admin/api/accounts/%d", accountID), body)
	req.SetPathValue("id", fmt.Sprintf("%d", accountID))
	rec := httptest.NewRecorder()
	handler.updateAccount(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	acc, err := st.GetAccount(accountID)
	if err != nil {
		t.Fatalf("get account: %v", err)
	}
	if acc.APIKey != "sk-old" {
		t.Fatalf("expected api key to be preserved, got %q", acc.APIKey)
	}
}

func TestUpdateTokenPreservesExistingValueWhenTokenOmitted(t *testing.T) {
	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	handler := NewHandler(st, cache, t.TempDir(), "", nil, newTestNotifier(st))
	poolID, err := st.CreatePool("Pool", 0, true)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	tokenID, err := st.CreateToken(&store.AccessToken{
		Name:    "Pool Token",
		Token:   "sk-old-token",
		PoolID:  &poolID,
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("create token: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/admin/api/tokens/%d", tokenID), strings.NewReader(`{"name":"Pool Token"}`))
	req.SetPathValue("id", fmt.Sprintf("%d", tokenID))
	rec := httptest.NewRecorder()
	handler.updateToken(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	token, err := st.GetToken(tokenID)
	if err != nil {
		t.Fatalf("get token: %v", err)
	}
	if token.Token != "sk-old-token" {
		t.Fatalf("expected token to remain unchanged, got %q", token.Token)
	}
	if token.Name != "Pool Token" {
		t.Fatalf("expected name to remain unchanged, got %q", token.Name)
	}
}

func TestUpdateTokenRejectsExplicitEmptyToken(t *testing.T) {
	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	handler := NewHandler(st, cache, t.TempDir(), "", nil, newTestNotifier(st))
	poolID, err := st.CreatePool("Pool", 0, true)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	tokenID, err := st.CreateToken(&store.AccessToken{
		Name:    "Pool Token",
		Token:   "sk-old-token",
		PoolID:  &poolID,
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("create token: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/admin/api/tokens/%d", tokenID), strings.NewReader(`{"name":"Pool Token","token":""}`))
	req.SetPathValue("id", fmt.Sprintf("%d", tokenID))
	rec := httptest.NewRecorder()
	handler.updateToken(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	token, err := st.GetToken(tokenID)
	if err != nil {
		t.Fatalf("get token: %v", err)
	}
	if token.Token != "sk-old-token" {
		t.Fatalf("expected token to remain unchanged, got %q", token.Token)
	}
}

func TestUpdateTokenReplacesExistingValueWhenNewTokenPresent(t *testing.T) {
	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	handler := NewHandler(st, cache, t.TempDir(), "", nil, newTestNotifier(st))
	poolID, err := st.CreatePool("Pool", 0, true)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	tokenID, err := st.CreateToken(&store.AccessToken{
		Name:    "Pool Token",
		Token:   "sk-old-token",
		PoolID:  &poolID,
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("create token: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/admin/api/tokens/%d", tokenID), strings.NewReader(`{"name":"Pool Token","token":"sk-new-token"}`))
	req.SetPathValue("id", fmt.Sprintf("%d", tokenID))
	rec := httptest.NewRecorder()
	handler.updateToken(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data store.AccessToken `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Data.Token != "" {
		t.Fatalf("expected token secret to be stripped from response")
	}
	if resp.Data.TokenMasked == "" {
		t.Fatalf("expected masked token in response")
	}
	token, err := st.GetToken(tokenID)
	if err != nil {
		t.Fatalf("get token: %v", err)
	}
	if token.Token != "sk-new-token" {
		t.Fatalf("expected token to be replaced, got %q", token.Token)
	}
}

func TestDeleteCpaAccountRemovesAuthFileAndSignalsReload(t *testing.T) {
	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	authDir := t.TempDir()
	reloadSignal := filepath.Join(t.TempDir(), "reload.signal")

	svcID, err := st.CreateCpaService(&store.CpaService{
		Label:   "CPA",
		BaseURL: "https://cpa.example.com",
		APIKey:  "svc-key",
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("create cpa service: %v", err)
	}
	if err := cpa.WriteAuthFile(authDir, &cpa.CpaAuthFile{
		AccountID: "acct-delete",
		Email:     "delete-test",
		Type:      "codex",
	}, "codex-delete-test-plus"); err != nil {
		t.Fatalf("write auth file: %v", err)
	}
	accID, err := st.CreateAccount(&store.Account{
		Label:                 "Delete me",
		SourceKind:            "cpa",
		CpaServiceID:          &svcID,
		CpaProvider:           "codex",
		CpaAccountKey:         "codex-delete-test-plus",
		CpaCredentialStatus:   "ok",
		CpaSubscriptionStatus: "active",
		Enabled:               true,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	checker := health.NewChecker(st, cache, authDir, "", newTestNotifier(st))
	checker.SetCpaReloadSignalPath(reloadSignal)
	handler := NewHandler(st, cache, authDir, "", checker, newTestNotifier(st))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequestWithContext(ctx, http.MethodDelete, fmt.Sprintf("/admin/api/accounts/%d", accID), http.NoBody)
	req.SetPathValue("id", fmt.Sprintf("%d", accID))
	rr := httptest.NewRecorder()
	handler.deleteAccount(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if _, err := os.Stat(filepath.Join(authDir, "codex-delete-test-plus.json")); !os.IsNotExist(err) {
		t.Fatalf("expected auth file to be removed, stat err=%v", err)
	}
	if acc, err := st.GetAccount(accID); err != nil || acc != nil {
		t.Fatalf("expected account row to be deleted, acc=%+v err=%v", acc, err)
	}
	if _, err := os.Stat(reloadSignal); err != nil {
		t.Fatalf("expected reload signal to be written: %v", err)
	}
	deleteOp := requireRecentOperation(t, st, "delete_account")
	if deleteOp.Status != "partial" || deleteOp.ErrorCode != "runtime_reload_failed" || deleteOp.TargetType != "account" || deleteOp.TargetID != fmt.Sprint(accID) {
		t.Fatalf("unexpected delete account operation: %+v", deleteOp)
	}
	if deleteOp.SafeErrorMessage != "CPA runtime reload failed" || strings.Contains(deleteOp.SafeErrorMessage, "context canceled") {
		t.Fatalf("delete account operation leaked reload internals: %+v", deleteOp)
	}
	reloadOp := requireRecentOperation(t, st, "runtime_reload")
	if reloadOp.Status != "failed" || reloadOp.ErrorCode != "runtime_reload_failed" || reloadOp.Source != "delete_account" {
		t.Fatalf("unexpected runtime reload operation: %+v", reloadOp)
	}
	if reloadOp.CorrelationID != deleteOp.OperationID {
		t.Fatalf("runtime reload operation is not correlated to delete account operation: reload=%+v delete=%+v", reloadOp, deleteOp)
	}
	if reloadOp.SafeErrorMessage != "CPA runtime reload failed" || strings.Contains(reloadOp.SafeErrorMessage, "context canceled") {
		t.Fatalf("runtime reload operation leaked reload internals: %+v", reloadOp)
	}
	detail, err := st.GetOperation(reloadOp.OperationID)
	if err != nil {
		t.Fatalf("get reload operation: %v", err)
	}
	if detail == nil || len(detail.Items) != 1 || detail.Items[0].Stage != "request_runtime_reload" {
		t.Fatalf("unexpected runtime reload operation detail: %+v", detail)
	}
	if detail.Items[0].SafeErrorMessage != "CPA runtime reload failed" || strings.Contains(detail.Items[0].SafeErrorMessage, "context canceled") {
		t.Fatalf("runtime reload item leaked reload internals: %+v", detail.Items[0])
	}
}

func TestDeleteCpaAccountRedactsAuthFileDeletionFailure(t *testing.T) {
	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	authDir := t.TempDir()
	svcID, err := st.CreateCpaService(&store.CpaService{
		Label:   "CPA",
		BaseURL: "https://cpa.example.com",
		APIKey:  "svc-key",
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("create cpa service: %v", err)
	}
	accID, err := st.CreateAccount(&store.Account{
		Label:               "Delete me",
		SourceKind:          "cpa",
		CpaServiceID:        &svcID,
		CpaProvider:         "codex",
		CpaAccountKey:       "codex-delete-redact-plus",
		CpaCredentialStatus: "ok",
		Enabled:             true,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	reloadSignal := filepath.Join(t.TempDir(), "reload.signal")
	checker := health.NewChecker(st, cache, authDir, "", newTestNotifier(st))
	checker.SetCpaReloadSignalPath(reloadSignal)
	handler := NewHandler(st, cache, authDir, "", checker, newTestNotifier(st))
	if err := os.MkdirAll(filepath.Join(authDir, "codex-delete-redact-plus.json"), 0o755); err != nil {
		t.Fatalf("create blocking auth path: %v", err)
	}
	if err := os.WriteFile(filepath.Join(authDir, "codex-delete-redact-plus.json", "keep"), []byte("x"), 0o600); err != nil {
		t.Fatalf("seed blocking auth path: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/admin/api/accounts/%d", accID), http.NoBody)
	req.SetPathValue("id", fmt.Sprintf("%d", accID))
	rr := httptest.NewRecorder()
	handler.deleteAccount(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
	op := requireRecentOperation(t, st, "delete_account")
	if !strings.Contains(op.SafeErrorMessage, "cpa auth file deletion failed") {
		t.Fatalf("expected redacted auth file delete error, got %+v", op)
	}
	if strings.Contains(op.SafeErrorMessage, "codex-delete-redact-plus") || strings.Contains(op.SafeErrorMessage, ".json") || strings.Contains(op.SafeErrorMessage, "@") {
		t.Fatalf("operation leaked sensitive file details: %+v", op)
	}
}

func TestDeletePoolWithCpaAccountReportsReloadFailure(t *testing.T) {
	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	authDir := t.TempDir()
	reloadSignal := filepath.Join(t.TempDir(), "reload.signal")
	poolID, err := st.CreatePool("CPA Pool", 0, true)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	svcID, err := st.CreateCpaService(&store.CpaService{
		Label:   "CPA",
		BaseURL: "http://127.0.0.1:1",
		APIKey:  "svc-key",
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("create cpa service: %v", err)
	}
	accID, err := st.CreateAccount(&store.Account{
		Label:               "CPA Account",
		SourceKind:          "cpa",
		CpaServiceID:        &svcID,
		CpaProvider:         "codex",
		CpaAccountKey:       "codex-delete-pool-plus",
		CpaCredentialStatus: "ok",
		Enabled:             true,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if _, err := st.AddPoolMember(poolID, accID); err != nil {
		t.Fatalf("add pool member: %v", err)
	}
	checker := health.NewChecker(st, cache, authDir, "", newTestNotifier(st))
	checker.SetCpaReloadSignalPath(reloadSignal)
	handler := NewHandler(st, cache, authDir, "", checker, newTestNotifier(st))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequestWithContext(ctx, http.MethodDelete, fmt.Sprintf("/admin/api/pools/%d", poolID), http.NoBody)
	req.SetPathValue("id", fmt.Sprintf("%d", poolID))
	rr := httptest.NewRecorder()
	handler.deletePool(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 reload failure, got %d: %s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "context canceled") {
		t.Fatalf("delete pool response leaked reload internals: %s", rr.Body.String())
	}
	if pool, err := st.GetPool(poolID); err != nil || pool != nil {
		t.Fatalf("pool should already be deleted despite reload failure, pool=%+v err=%v", pool, err)
	}
	if acc, err := st.GetAccount(accID); err != nil || acc != nil {
		t.Fatalf("account should already be deleted despite reload failure, acc=%+v err=%v", acc, err)
	}
	if _, err := os.Stat(reloadSignal); err != nil {
		t.Fatalf("expected reload signal write attempt: %v", err)
	}
	deleteOp := requireRecentOperation(t, st, "delete_pool")
	if deleteOp.Status != "partial" || deleteOp.ErrorCode != "runtime_reload_failed" || deleteOp.TargetID != fmt.Sprint(poolID) {
		t.Fatalf("unexpected delete pool operation: %+v", deleteOp)
	}
	if deleteOp.SafeErrorMessage != "CPA runtime reload failed" || strings.Contains(deleteOp.SafeErrorMessage, "context canceled") {
		t.Fatalf("delete pool operation leaked reload internals: %+v", deleteOp)
	}
	reloadOp := requireRecentOperation(t, st, "runtime_reload")
	if reloadOp.Status != "failed" || reloadOp.ErrorCode != "runtime_reload_failed" || reloadOp.Source != "delete_pool" {
		t.Fatalf("unexpected runtime reload operation: %+v", reloadOp)
	}
	if reloadOp.CorrelationID != deleteOp.OperationID {
		t.Fatalf("runtime reload operation is not correlated to delete pool operation: reload=%+v delete=%+v", reloadOp, deleteOp)
	}
	if reloadOp.SafeErrorMessage != "CPA runtime reload failed" || strings.Contains(reloadOp.SafeErrorMessage, "context canceled") {
		t.Fatalf("runtime reload operation leaked reload internals: %+v", reloadOp)
	}
	detail, err := st.GetOperation(reloadOp.OperationID)
	if err != nil {
		t.Fatalf("get reload operation: %v", err)
	}
	if detail == nil || len(detail.Items) != 1 || detail.Items[0].Stage != "request_runtime_reload" {
		t.Fatalf("unexpected runtime reload operation detail: %+v", detail)
	}
	if detail.Items[0].SafeErrorMessage != "CPA runtime reload failed" || strings.Contains(detail.Items[0].SafeErrorMessage, "context canceled") {
		t.Fatalf("runtime reload item leaked reload internals: %+v", detail.Items[0])
	}
}

func TestDiagnosticRequestBypassesServingCooldownWithoutUpdatingUsageOrHealth(t *testing.T) {
	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"diag","choices":[{"message":{"content":"ok"}}],"usage":{"prompt_tokens":2,"completion_tokens":3}}`))
	}))
	defer upstream.Close()

	poolID, err := st.CreatePool("Pool", 0, true)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	accID, err := st.CreateAccount(&store.Account{
		Label:         "Cooldown",
		SourceKind:    "openai_compat",
		BaseURL:       upstream.URL + "/v1",
		APIKey:        "fake-key",
		Provider:      "mock",
		Enabled:       true,
		ServingStatus: "healthy",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if _, err := st.AddPoolMember(poolID, accID); err != nil {
		t.Fatalf("add pool member: %v", err)
	}
	if err := st.MarkAccountServingFailure(accID, "previous failure", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("mark cooldown: %v", err)
	}
	cache.Invalidate()

	handler := NewHandler(st, cache, "", "", nil, newTestNotifier(st), t.TempDir())
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/admin/api/accounts/%d/diagnostic-request", accID), strings.NewReader(`{"model":"gpt-test","messages":[{"role":"user","content":"ping"}]}`))
	req.SetPathValue("id", fmt.Sprintf("%d", accID))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.diagnosticRequest(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected diagnostic request to bypass cooldown, got %d: %s", rr.Code, rr.Body.String())
	}
	acc, err := st.GetAccount(accID)
	if err != nil {
		t.Fatalf("get account: %v", err)
	}
	if acc.ServingStatus != "cooldown" {
		t.Fatalf("diagnostic request should not update serving health, got %q", acc.ServingStatus)
	}
	for i := 0; i < 20; i++ {
		logs, _, err := st.ListLogs(10, 0)
		if err != nil {
			t.Fatalf("list logs: %v", err)
		}
		if len(logs) > 0 {
			if !logs[0].Diagnostic {
				t.Fatalf("expected diagnostic log, got %+v", logs[0])
			}
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	stats, err := st.GetUsageSummary(store.UsageFilter{})
	if err != nil {
		t.Fatalf("usage summary: %v", err)
	}
	if stats.TotalRequests != 0 {
		t.Fatalf("diagnostic request should not count ordinary usage, got %+v", stats)
	}
}

func adminTestJWT(claims map[string]any) string {
	headerJSON, _ := json.Marshal(map[string]any{"alg": "none", "typ": "JWT"})
	payloadJSON, _ := json.Marshal(claims)
	return base64.RawURLEncoding.EncodeToString(headerJSON) + "." +
		base64.RawURLEncoding.EncodeToString(payloadJSON) + ".sig"
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

	handler := NewHandler(st, cache, "", "", nil, newTestNotifier(st))
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

func TestGetPoolDetailHandlesEmptyPool(t *testing.T) {
	st := newTestStore(t)
	cache := store.NewRoutingCache(st)

	poolID, _, err := st.CreatePoolWithDefaultToken("Empty Pool", 0, true)
	if err != nil {
		t.Fatalf("create pool with token: %v", err)
	}

	handler := NewHandler(st, cache, "", "", nil, newTestNotifier(st))
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/admin/api/pools/%d", poolID), http.NoBody)
	req.SetPathValue("id", fmt.Sprintf("%d", poolID))
	rr := httptest.NewRecorder()

	handler.getPoolDetail(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Data struct {
			Pool struct {
				ID int64 `json:"id"`
			} `json:"pool"`
			Members []store.PoolMember  `json:"members"`
			Tokens  []store.AccessToken `json:"tokens"`
			Models  []string            `json:"models"`
			Stats   store.UsageStats    `json:"stats"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Data.Pool.ID != poolID {
		t.Fatalf("expected pool id %d, got %d", poolID, resp.Data.Pool.ID)
	}
	if resp.Data.Members == nil {
		t.Fatalf("expected members to be an empty array, got nil")
	}
	if len(resp.Data.Tokens) != 1 {
		t.Fatalf("expected one pool token, got %d", len(resp.Data.Tokens))
	}
	if resp.Data.Tokens[0].Token != "" || resp.Data.Tokens[0].TokenMasked == "" {
		t.Fatalf("expected masked token only, got %+v", resp.Data.Tokens[0])
	}
	if resp.Data.Models == nil {
		t.Fatalf("expected models to be an empty array, got nil")
	}
	if resp.Data.Stats.ByAccount == nil || resp.Data.Stats.ByToken == nil {
		t.Fatalf("expected empty stats arrays, got %+v", resp.Data.Stats)
	}
}

func TestGetPoolDetailMissingPoolReturnsNotFound(t *testing.T) {
	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	handler := NewHandler(st, cache, "", "", nil, newTestNotifier(st))

	req := httptest.NewRequest(http.MethodGet, "/admin/api/pools/404", http.NoBody)
	req.SetPathValue("id", "404")
	rr := httptest.NewRecorder()

	handler.getPoolDetail(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "pool not found") {
		t.Fatalf("expected structured not found response, got %s", rr.Body.String())
	}
}

func TestImportConfigCreatesPoolsSkipsExistingTokensAndIgnoresAdminToken(t *testing.T) {
	st := newTestStore(t)
	cache := store.NewRoutingCache(st)

	existingPoolID, err := st.CreatePool("Existing Pool", 1, true)
	if err != nil {
		t.Fatalf("create existing pool: %v", err)
	}
	existingPoolIDCopy := existingPoolID
	if _, err := st.CreateToken(&store.AccessToken{
		Name:    "existing-token",
		PoolID:  &existingPoolIDCopy,
		Enabled: true,
	}); err != nil {
		t.Fatalf("create existing token: %v", err)
	}

	handler := NewHandler(st, cache, "", "", nil, newTestNotifier(st))
	body := bytes.NewBufferString(`{
		"data":{
			"pools":[
				{"label":"Existing Pool","priority":3,"enabled":false},
				{"label":"Imported Pool","priority":5,"enabled":true}
			],
			"access_tokens":[
				{"name":"existing-token","pool_id":1,"pool_label":"Existing Pool","enabled":true},
				{"name":"imported-token","pool_id":999,"pool_label":"Imported Pool","enabled":true}
			],
			"settings":{
				"request_timeout":"180",
				"data_retention_days":"14",
				"admin_token":"masked-value"
			}
		}
	}`)
	req := httptest.NewRequest(http.MethodPost, "/admin/api/import", body)
	rr := httptest.NewRecorder()

	handler.importConfig(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Data store.ConfigImportResult `json:"data"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Data.CreatedPools != 1 || resp.Data.UpdatedPools != 1 {
		t.Fatalf("unexpected pool import result: %+v", resp.Data)
	}
	if resp.Data.CreatedTokens != 1 || resp.Data.SkippedTokens != 1 {
		t.Fatalf("unexpected token import result: %+v", resp.Data)
	}
	if resp.Data.UpdatedSettings != 2 {
		t.Fatalf("expected 2 updated settings, got %+v", resp.Data)
	}

	importedPool, err := st.GetPoolByLabel("Imported Pool")
	if err != nil || importedPool == nil {
		t.Fatalf("expected imported pool, err=%v", err)
	}

	existingPool, err := st.GetPoolByLabel("Existing Pool")
	if err != nil || existingPool == nil {
		t.Fatalf("expected existing pool, err=%v", err)
	}
	if existingPool.Priority != 3 || existingPool.Enabled {
		t.Fatalf("expected existing pool to be updated, got %+v", existingPool)
	}

	importedToken, err := st.GetTokenByNameAndPool("imported-token", &importedPool.ID)
	if err != nil || importedToken == nil {
		t.Fatalf("expected imported token, err=%v", err)
	}

	settings, err := st.GetSettings()
	if err != nil {
		t.Fatalf("get settings: %v", err)
	}
	if settings["request_timeout"] != "180" || settings["data_retention_days"] != "14" {
		t.Fatalf("expected imported settings, got %#v", settings)
	}
	if settings["admin_token"] != "" {
		t.Fatalf("expected admin_token to be ignored, got %q", settings["admin_token"])
	}
}

func TestUpdateSettingsRejectsDeprecatedNotificationFlagsOnCleanStore(t *testing.T) {
	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	handler := NewHandler(st, cache, "", "", nil, newTestNotifier(st))

	req := httptest.NewRequest(
		http.MethodPut,
		"/admin/api/settings",
		strings.NewReader(`{"notification_error_enabled":true,"notification_expiring_enabled":true,"notification_expiring_days":5,"webhook_enabled":true,"webhook_url":"https://example.com/hook"}`),
	)
	rr := httptest.NewRecorder()

	handler.updateSettings(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	settings, err := st.GetSettings()
	if err != nil {
		t.Fatalf("get settings: %v", err)
	}
	if settings["notification_expiring_days"] != "5" {
		t.Fatalf("expected supported setting to persist, got %q", settings["notification_expiring_days"])
	}
	for _, deprecated := range []string{"notification_error_enabled", "notification_expiring_enabled", "webhook_enabled", "webhook_url"} {
		if v, ok := settings[deprecated]; ok && v != "" {
			t.Fatalf("expected deprecated %s to NOT be written, got %q", deprecated, v)
		}
	}
}

func TestImportConfigRejectsInvalidSettingValue(t *testing.T) {
	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	handler := NewHandler(st, cache, "", "", nil, newTestNotifier(st))

	req := httptest.NewRequest(
		http.MethodPost,
		"/admin/api/import",
		bytes.NewBufferString(`{"data":{"settings":{"request_timeout":"abc"}}}`),
	)
	rr := httptest.NewRecorder()

	handler.importConfig(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}

	settings, err := st.GetSettings()
	if err != nil {
		t.Fatalf("get settings: %v", err)
	}
	if settings["request_timeout"] != "" {
		t.Fatalf("expected invalid setting import to be rejected")
	}
}

func TestGetNotificationsReturnsSettingsAndSubscriptions(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	handler := NewHandler(st, cache, "", "", nil, newTestNotifier(st))

	req := httptest.NewRequest(http.MethodGet, "/admin/api/notifications", http.NoBody)
	rr := httptest.NewRecorder()
	handler.getNotifications(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Data struct {
			Settings      store.NotificationSettings       `json:"settings"`
			Subscriptions []store.NotificationSubscription `json:"subscriptions"`
			EventTypes    []notify.EventType               `json:"event_types"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Data.Settings.Enabled {
		t.Fatalf("expected settings to default to disabled")
	}
	if len(resp.Data.Subscriptions) != 6 {
		t.Fatalf("expected 6 subscriptions, got %d", len(resp.Data.Subscriptions))
	}
	if len(resp.Data.EventTypes) != 6 {
		t.Fatalf("expected 6 event types, got %d", len(resp.Data.EventTypes))
	}
}

func TestUpdateNotificationSettingsRejectsInvalidWebhook(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	handler := NewHandler(st, cache, "", "", nil, newTestNotifier(st))

	req := httptest.NewRequest(
		http.MethodPut,
		"/admin/api/notifications/settings",
		strings.NewReader(`{"enabled":true,"webhook_url":"ftp://bad","mention_mobile_list":[]}`),
	)
	rr := httptest.NewRecorder()
	handler.updateNotificationSettings(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestUpdateNotificationSettingsRejectsInvalidMobile(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	handler := NewHandler(st, cache, "", "", nil, newTestNotifier(st))

	req := httptest.NewRequest(
		http.MethodPut,
		"/admin/api/notifications/settings",
		strings.NewReader(`{"enabled":true,"webhook_url":"https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=abcd1234","mention_mobile_list":["12345"]}`),
	)
	rr := httptest.NewRecorder()
	handler.updateNotificationSettings(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestUpdateNotificationSettingsPersistsValidPayload(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	handler := NewHandler(st, cache, "", "", nil, newTestNotifier(st))

	req := httptest.NewRequest(
		http.MethodPut,
		"/admin/api/notifications/settings",
		strings.NewReader(`{"enabled":true,"webhook_url":"https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=abcd1234","mention_mobile_list":["13800138000","@all","13800138000"]}`),
	)
	rr := httptest.NewRecorder()
	handler.updateNotificationSettings(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	stored, err := st.GetNotificationSettings()
	if err != nil {
		t.Fatalf("load settings: %v", err)
	}
	if !stored.Enabled {
		t.Fatalf("settings did not persist: %+v", stored)
	}
	if len(stored.MentionMobileList) != 2 {
		t.Fatalf("expected dedup of mentions, got %+v", stored.MentionMobileList)
	}
}

func TestUpdateNotificationSubscriptionRejectsUnknownEvent(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	handler := NewHandler(st, cache, "", "", nil, newTestNotifier(st))

	req := httptest.NewRequest(
		http.MethodPut,
		"/admin/api/notifications/subscriptions/bogus",
		strings.NewReader(`{"subscribed":true,"body_template":"b"}`),
	)
	req.SetPathValue("event", "bogus")
	rr := httptest.NewRecorder()
	handler.updateNotificationSubscription(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestUpdateNotificationSubscriptionRequiresBody(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	handler := NewHandler(st, cache, "", "", nil, newTestNotifier(st))

	req := httptest.NewRequest(
		http.MethodPut,
		"/admin/api/notifications/subscriptions/account_error",
		strings.NewReader(`{"subscribed":true,"body_template":""}`),
	)
	req.SetPathValue("event", "account_error")
	rr := httptest.NewRecorder()
	handler.updateNotificationSubscription(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestTestNotificationReturns409WhenDisabled(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	handler := NewHandler(st, cache, "", "", nil, newTestNotifier(st))

	req := httptest.NewRequest(http.MethodPost, "/admin/api/notifications/test", http.NoBody)
	rr := httptest.NewRecorder()
	handler.testNotification(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409 when notifications are disabled, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestGetCpaServiceDoesNotExposeManagementKey(t *testing.T) {
	t.Parallel()

	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	if err := st.SetSetting("cpa_provider_pinning_supported", "1"); err != nil {
		t.Fatalf("set provider pinning capability: %v", err)
	}
	if _, err := st.CreateCpaService(&store.CpaService{
		Label:         "CPA",
		BaseURL:       "https://cpa.example.com",
		APIKey:        "api-key",
		ManagementKey: "manage-secret",
		Enabled:       true,
	}); err != nil {
		t.Fatalf("create cpa service: %v", err)
	}
	handler := NewHandler(st, cache, "", "", nil, newTestNotifier(st))

	req := httptest.NewRequest(http.MethodGet, "/admin/api/cpa/service", http.NoBody)
	rr := httptest.NewRecorder()
	handler.getCpaService(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, ok := resp.Data["management_key"]; ok {
		t.Fatalf("expected management_key to be omitted from response")
	}
	if got, ok := resp.Data["provider_pinning_supported"].(bool); !ok || !got {
		t.Fatalf("expected provider_pinning_supported=true, got %#v", resp.Data["provider_pinning_supported"])
	}
	if got, ok := resp.Data["provider_pinning_state"].(string); !ok || got != "enabled" {
		t.Fatalf("expected provider_pinning_state=enabled, got %#v", resp.Data["provider_pinning_state"])
	}
}

func TestUpsertCpaServicePreservesManagementKey(t *testing.T) {
	t.Parallel()

	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	id, err := st.CreateCpaService(&store.CpaService{
		Label:         "CPA",
		BaseURL:       "https://cpa.example.com",
		APIKey:        "api-key",
		ManagementKey: "manage-secret",
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("create cpa service: %v", err)
	}
	handler := NewHandler(st, cache, "", "", nil, newTestNotifier(st))

	req := httptest.NewRequest(http.MethodPut, "/admin/api/cpa/service", bytes.NewBufferString(`{
		"label":"Updated CPA",
		"base_url":"https://cpa.example.com/v2",
		"enabled":true
	}`))
	rr := httptest.NewRecorder()
	handler.upsertCpaService(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	svc, err := st.GetCpaServiceByID(id)
	if err != nil {
		t.Fatalf("get cpa service: %v", err)
	}
	if svc == nil {
		t.Fatalf("expected cpa service to exist")
	}
	if svc.ManagementKey != "manage-secret" {
		t.Fatalf("expected management key to be preserved, got %q", svc.ManagementKey)
	}
	var resp struct {
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	got, ok := resp.Data["provider_pinning_supported"].(bool)
	if !ok {
		t.Fatalf("expected provider_pinning_supported field in response, got %#v", resp.Data["provider_pinning_supported"])
	}
	if got {
		t.Fatalf("expected provider_pinning_supported=false when capability is unset")
	}
	if state, ok := resp.Data["provider_pinning_state"].(string); !ok || state != "unknown" {
		t.Fatalf("expected provider_pinning_state=unknown when capability is unset, got %#v", resp.Data["provider_pinning_state"])
	}
}

func TestExportDoesNotExposeManagementKey(t *testing.T) {
	t.Parallel()

	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	if _, err := st.CreateCpaService(&store.CpaService{
		Label:         "CPA",
		BaseURL:       "https://cpa.example.com",
		APIKey:        "api-key",
		ManagementKey: "manage-secret",
		Enabled:       true,
	}); err != nil {
		t.Fatalf("create cpa service: %v", err)
	}
	handler := NewHandler(st, cache, "", "", nil, newTestNotifier(st))

	req := httptest.NewRequest(http.MethodGet, "/admin/api/export", http.NoBody)
	rr := httptest.NewRecorder()
	handler.getExport(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Data struct {
			CpaServices []map[string]any `json:"cpa_services"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode export response: %v", err)
	}
	if len(resp.Data.CpaServices) != 1 {
		t.Fatalf("expected 1 cpa service, got %d", len(resp.Data.CpaServices))
	}
	if _, ok := resp.Data.CpaServices[0]["management_key"]; ok {
		t.Fatalf("expected management_key to be omitted from export")
	}
}

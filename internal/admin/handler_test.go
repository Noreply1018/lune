package admin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"lune/internal/cpa"
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
	if len(resp.Data.Subscriptions) != 5 {
		t.Fatalf("expected 5 subscriptions, got %d", len(resp.Data.Subscriptions))
	}
	if len(resp.Data.EventTypes) != 5 {
		t.Fatalf("expected 5 event types, got %d", len(resp.Data.EventTypes))
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

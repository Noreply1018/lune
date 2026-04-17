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

func createNotificationChannelForTest(t *testing.T, st *store.Store) int64 {
	t.Helper()
	id, err := st.CreateNotificationChannel(&store.NotificationChannel{
		Name:          "Ops",
		Type:          "generic_webhook",
		Enabled:       true,
		Config:        json.RawMessage(`{"schema":1,"url":"https://example.com/hook"}`),
		Subscriptions: []store.NotificationSubscription{{Event: "*"}},
	})
	if err != nil {
		t.Fatalf("create notification channel: %v", err)
	}
	return id
}

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
		notify.NewRegistry(
			drivers.NewGenericWebhookDriver(),
			drivers.NewWeChatWorkBotDriver(),
			drivers.NewFeishuBotDriver(),
			drivers.NewEmailSMTPDriver(),
		),
	)
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

	handler := NewHandler(st, cache, "", "", nil, newTestNotifier(st))
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

func TestUpdateSettingsIgnoresDeprecatedNotificationFlags(t *testing.T) {
	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	handler := NewHandler(st, cache, "", "", nil, newTestNotifier(st))

	req := httptest.NewRequest(
		http.MethodPut,
		"/admin/api/settings",
		strings.NewReader(`{"notification_error_enabled":false,"notification_expiring_enabled":false,"notification_expiring_days":5}`),
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
	if _, ok := settings["notification_error_enabled"]; ok {
		t.Fatalf("expected deprecated notification_error_enabled to be ignored")
	}
	if _, ok := settings["notification_expiring_enabled"]; ok {
		t.Fatalf("expected deprecated notification_expiring_enabled to be ignored")
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

func TestPreviewNotificationsAllowsEmptyBody(t *testing.T) {
	t.Parallel()

	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	handler := NewHandler(st, cache, "", "", nil, newTestNotifier(st))

	req := httptest.NewRequest(http.MethodPost, "/admin/api/notifications/preview", http.NoBody)
	rr := httptest.NewRecorder()

	handler.previewNotifications(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Data []notify.PreviewItem `json:"data"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode preview response: %v", err)
	}
	if len(resp.Data) != 0 {
		t.Fatalf("expected no preview items without channels, got %d", len(resp.Data))
	}
}

func TestTestNotificationChannelReturns404WhenMissing(t *testing.T) {
	t.Parallel()

	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	handler := NewHandler(st, cache, "", "", nil, newTestNotifier(st))

	req := httptest.NewRequest(http.MethodPost, "/admin/api/notifications/channels/999/test", http.NoBody)
	req.SetPathValue("id", "999")
	rr := httptest.NewRecorder()

	handler.testNotificationChannel(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestDeleteNotificationChannelReturns404WhenMissing(t *testing.T) {
	t.Parallel()

	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	handler := NewHandler(st, cache, "", "", nil, newTestNotifier(st))

	req := httptest.NewRequest(http.MethodDelete, "/admin/api/notifications/channels/999", http.NoBody)
	req.SetPathValue("id", "999")
	rr := httptest.NewRecorder()

	handler.deleteNotificationChannel(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestSetNotificationChannelEnabledOnlyUpdatesEnabledField(t *testing.T) {
	t.Parallel()

	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	channelID := createNotificationChannelForTest(t, st)
	handler := NewHandler(st, cache, "", "", nil, newTestNotifier(st))

	req := httptest.NewRequest(
		http.MethodPost,
		fmt.Sprintf("/admin/api/notifications/channels/%d/enabled", channelID),
		bytes.NewBufferString(`{"enabled":false}`),
	)
	req.SetPathValue("id", fmt.Sprintf("%d", channelID))
	rr := httptest.NewRecorder()

	handler.setNotificationChannelEnabled(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	channel, err := st.GetNotificationChannel(channelID)
	if err != nil {
		t.Fatalf("get channel: %v", err)
	}
	if channel == nil {
		t.Fatalf("expected channel to exist")
	}
	if channel.Enabled {
		t.Fatalf("expected channel to be disabled")
	}
	var cfg struct {
		Schema int    `json:"schema"`
		URL    string `json:"url"`
	}
	if err := json.Unmarshal(channel.Config, &cfg); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if cfg.URL != "https://example.com/hook" {
		t.Fatalf("expected config URL to remain unchanged, got %q", cfg.URL)
	}
}

func TestCreateNotificationChannelRejectsUnknownType(t *testing.T) {
	t.Parallel()

	st := newTestStore(t)
	cache := store.NewRoutingCache(st)
	handler := NewHandler(st, cache, "", "", nil, newTestNotifier(st))

	req := httptest.NewRequest(http.MethodPost, "/admin/api/notifications/channels", bytes.NewBufferString(`{
		"name":"Bad",
		"type":"bogus",
		"enabled":true,
		"config":{"password":"secret"}
	}`))
	rr := httptest.NewRecorder()

	handler.createNotificationChannel(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
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

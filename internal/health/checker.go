package health

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"lune/internal/cpa"
	"lune/internal/notify"
	"lune/internal/store"
	"lune/internal/syscfg"
)

const (
	degradedLatencyThreshold       = 5 * time.Second
	maxConcurrency                 = 10
	codexSubscriptionFetchInterval = 6 * time.Hour
)

type Checker struct {
	store         *store.Store
	cache         *store.RoutingCache
	client        *http.Client
	cpaAuthDir    string
	managementKey string
	notifier      *notify.Service
}

func NewChecker(st *store.Store, cache *store.RoutingCache, cpaAuthDir, managementKey string, notifier *notify.Service) *Checker {
	return &Checker{
		store:         st,
		cache:         cache,
		client:        &http.Client{Timeout: 15 * time.Second},
		cpaAuthDir:    cpaAuthDir,
		managementKey: managementKey,
		notifier:      notifier,
	}
}

func (c *Checker) Run(ctx context.Context) {
	interval := c.getInterval()
	slog.Info("health checker started", "interval", interval)

	c.checkAll(ctx)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.checkAll(ctx)
		case <-ctx.Done():
			slog.Info("health checker stopped")
			return
		}
	}
}

func (c *Checker) checkAll(ctx context.Context) {
	// check CPA service health first
	if svc := c.cache.GetCpaServiceSingle(); svc != nil && svc.Enabled {
		c.checkCpaService(ctx, svc)
	}

	accounts := c.cache.GetAccounts()
	if len(accounts) > 0 {
		// Semaphore-limited concurrency
		sem := make(chan struct{}, maxConcurrency)
		var wg sync.WaitGroup

		for _, acc := range accounts {
			if !acc.Enabled {
				continue
			}
			wg.Add(1)
			go func(a store.Account) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				c.checkOne(ctx, a)
			}(acc)
		}
		wg.Wait()
	}

	// sync CPA metadata from auth files
	c.syncCpaMetadata()
	c.fetchCodexQuotas(ctx)
	c.fetchCodexSubscriptions(ctx)
	c.pruneRequestLogs()

	c.cache.Invalidate()
	c.dispatchSystemNotifications(ctx)
}

// checkOne probes an account's health and discovers its available models.
func (c *Checker) checkOne(ctx context.Context, acc store.Account) {
	var url, apiKey string

	if acc.SourceKind == "cpa" && acc.CpaServiceID != nil {
		svc := c.cache.GetCpaService(*acc.CpaServiceID)
		if svc == nil || svc.Status == "error" || !svc.Enabled {
			c.store.UpdateAccountHealth(acc.ID, "error", "CPA service unreachable")
			return
		}
		url = fmt.Sprintf("%s/api/provider/%s/v1/models", strings.TrimRight(svc.BaseURL, "/"), acc.CpaProvider)
		apiKey = svc.APIKey
	} else {
		url = fmt.Sprintf("%s/models", strings.TrimRight(acc.BaseURL, "/"))
		apiKey = acc.APIKey
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		c.store.UpdateAccountHealth(acc.ID, "error", err.Error())
		return
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	start := time.Now()
	resp, err := c.client.Do(req)
	latency := time.Since(start)

	if err != nil {
		c.store.UpdateAccountHealth(acc.ID, "error", err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
		msg := formatHTTPStatusError(resp.StatusCode, body)
		c.store.UpdateAccountHealth(acc.ID, "error", msg)
		if acc.SourceKind == "cpa" && isCpaAuthFailure(resp.StatusCode, string(body)) {
			c.markCpaCredentialNeedsLogin(acc.ID, credentialReasonFromAuthText(string(body)), msg)
		}
		return
	}

	// Read response body for model discovery
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB limit
	if err != nil {
		c.store.UpdateAccountHealth(acc.ID, "error", "failed to read models response")
		return
	}

	// Update health status
	if latency > degradedLatencyThreshold {
		c.store.UpdateAccountHealth(acc.ID, "degraded", fmt.Sprintf("slow response: %s", latency))
	} else {
		c.store.UpdateAccountHealth(acc.ID, "healthy", "")
	}
	if acc.SourceKind == "cpa" {
		c.markCpaCredentialOK(acc.ID)
	}

	// Parse and refresh discovered models
	models := parseModelList(body)
	if len(models) > 0 {
		if err := c.store.RefreshAccountModels(acc.ID, models); err != nil {
			slog.Error("failed to refresh account models", "account_id", acc.ID, "err", err)
		}
	}
}

// DiscoverModels triggers on-demand model discovery for a specific account.
func (c *Checker) DiscoverModels(ctx context.Context, acc store.Account) ([]string, error) {
	var url, apiKey string

	if acc.SourceKind == "cpa" && acc.CpaServiceID != nil {
		svc := c.cache.GetCpaService(*acc.CpaServiceID)
		if svc == nil || !svc.Enabled {
			return nil, fmt.Errorf("CPA service unreachable")
		}
		url = fmt.Sprintf("%s/api/provider/%s/v1/models", strings.TrimRight(svc.BaseURL, "/"), acc.CpaProvider)
		apiKey = svc.APIKey
	} else {
		url = fmt.Sprintf("%s/models", strings.TrimRight(acc.BaseURL, "/"))
		apiKey = acc.APIKey
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
		msg := formatHTTPStatusError(resp.StatusCode, body)
		if acc.SourceKind == "cpa" && isCpaAuthFailure(resp.StatusCode, string(body)) {
			c.markCpaCredentialNeedsLogin(acc.ID, credentialReasonFromAuthText(string(body)), msg)
		}
		return nil, fmt.Errorf("%s", msg)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}

	models := parseModelList(body)
	if len(models) > 0 {
		if err := c.store.RefreshAccountModels(acc.ID, models); err != nil {
			return nil, fmt.Errorf("store models: %w", err)
		}
		c.cache.Invalidate()
	}
	if acc.SourceKind == "cpa" {
		c.markCpaCredentialOK(acc.ID)
	}

	return models, nil
}

func (c *Checker) RefreshCodexQuota(ctx context.Context, acc store.Account) (bool, error) {
	if acc.SourceKind != "cpa" || strings.ToLower(acc.CpaProvider) != "codex" {
		return false, nil
	}
	if !acc.Enabled || acc.CpaDisabled {
		return false, nil
	}
	if acc.CpaOpenaiID == "" {
		return false, fmt.Errorf("missing ChatGPT account id")
	}
	if acc.CpaServiceID == nil {
		return false, fmt.Errorf("missing CPA service")
	}
	svc := c.cache.GetCpaService(*acc.CpaServiceID)
	if svc == nil || !svc.Enabled {
		return false, fmt.Errorf("CPA service unreachable")
	}
	managementKey := svc.ManagementKey
	if managementKey == "" {
		managementKey = c.managementKey
	}
	if managementKey == "" {
		return false, fmt.Errorf("CPA management key is empty")
	}

	client := cpa.NewManagementClient(svc.BaseURL, managementKey)
	authIndex, err := c.findAuthIndex(ctx, client, acc.CpaAccountKey)
	if err != nil {
		if strings.Contains(err.Error(), "auth file not found") {
			c.markCpaCredentialNeedsLogin(acc.ID, "auth_index_missing", err.Error())
		}
		return false, err
	}
	if err := c.fetchOneCodexQuota(ctx, client, acc, authIndex); err != nil {
		return false, err
	}
	return true, nil
}

func (c *Checker) RefreshCodexSubscription(ctx context.Context, acc store.Account) (bool, error) {
	if acc.SourceKind != "cpa" || strings.ToLower(acc.CpaProvider) != "codex" {
		return false, nil
	}
	if !acc.Enabled || acc.CpaDisabled {
		return false, nil
	}
	if acc.CpaServiceID == nil {
		return false, fmt.Errorf("missing CPA service")
	}
	svc := c.cache.GetCpaService(*acc.CpaServiceID)
	if svc == nil || !svc.Enabled {
		return false, fmt.Errorf("CPA service unreachable")
	}
	managementKey := svc.ManagementKey
	if managementKey == "" {
		managementKey = c.managementKey
	}
	if managementKey == "" {
		return false, fmt.Errorf("CPA management key is empty")
	}

	client := cpa.NewManagementClient(svc.BaseURL, managementKey)
	authIndex, err := c.findAuthIndex(ctx, client, acc.CpaAccountKey)
	if err != nil {
		if strings.Contains(err.Error(), "auth file not found") {
			c.markCpaCredentialNeedsLogin(acc.ID, "auth_index_missing", err.Error())
		}
		return false, err
	}
	if err := c.fetchOneCodexSubscription(ctx, client, acc, authIndex); err != nil {
		return false, err
	}
	return true, nil
}

func parseModelList(body []byte) []string {
	var resp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil
	}
	models := make([]string, 0, len(resp.Data))
	for _, m := range resp.Data {
		if m.ID != "" {
			models = append(models, m.ID)
		}
	}
	return models
}

func (c *Checker) checkCpaService(ctx context.Context, svc *store.CpaService) {
	url := fmt.Sprintf("%s/healthz", strings.TrimRight(svc.BaseURL, "/"))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		c.store.UpdateCpaServiceHealth(svc.ID, "error", err.Error())
		c.cache.Invalidate()
		return
	}
	if svc.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+svc.APIKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		c.store.UpdateCpaServiceHealth(svc.ID, "error", err.Error())
		c.cache.Invalidate()
		return
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.store.UpdateCpaServiceHealth(svc.ID, "error", fmt.Sprintf("HTTP %d", resp.StatusCode))
		c.cache.Invalidate()
		return
	}

	c.store.UpdateCpaServiceHealth(svc.ID, "healthy", "")
	c.cache.Invalidate()
}

// fetchCodexQuotas pulls `wham/usage` through CPA api-call for every enabled
// Codex account whose `codex_quota_fetched_at` is older than the configured
// interval, then persists the raw JSON. Failures are logged and skipped — the
// next tick retries, so transient hiccups do not cascade into account errors.
func (c *Checker) fetchCodexQuotas(ctx context.Context) {
	svc := c.cache.GetCpaServiceSingle()
	if svc == nil || !svc.Enabled {
		return
	}
	managementKey := svc.ManagementKey
	if managementKey == "" {
		managementKey = c.managementKey
	}
	if managementKey == "" {
		return
	}

	interval := syscfg.ParsePositiveInt(
		c.cache.GetSetting("codex_quota_fetch_interval"),
		syscfg.DefaultCodexQuotaFetchInterval,
	)
	cutoff := time.Now().UTC().Add(-time.Duration(interval) * time.Second)

	var targets []store.Account
	for _, acc := range c.cache.GetAccounts() {
		if acc.SourceKind != "cpa" || acc.CpaProvider != "codex" {
			continue
		}
		if !acc.Enabled || acc.CpaDisabled {
			continue
		}
		if acc.CpaOpenaiID == "" {
			continue
		}
		if acc.CodexQuotaFetchedAt != "" {
			if t, err := time.Parse("2006-01-02 15:04:05", acc.CodexQuotaFetchedAt); err == nil && t.After(cutoff) {
				continue
			}
		}
		targets = append(targets, acc)
	}
	if len(targets) == 0 {
		return
	}

	client := cpa.NewManagementClient(svc.BaseURL, managementKey)
	keyToIndex, err := c.listAuthIndexes(ctx, client)
	if err != nil {
		slog.Warn("fetch codex quotas: list auth-files", "err", err)
		return
	}

	sem := make(chan struct{}, maxConcurrency)
	var wg sync.WaitGroup
	for _, acc := range targets {
		authIndex, ok := keyToIndex[acc.CpaAccountKey]
		if !ok || authIndex == "" {
			c.markCpaCredentialNeedsLogin(acc.ID, "auth_index_missing", "CPA auth file not found")
			continue
		}
		wg.Add(1)
		go func(a store.Account, idx string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if err := c.fetchOneCodexQuota(ctx, client, a, idx); err != nil {
				slog.Warn("fetch codex quota", "account_id", a.ID, "err", err)
			}
		}(acc, authIndex)
	}
	wg.Wait()
}

func (c *Checker) fetchCodexSubscriptions(ctx context.Context) {
	svc := c.cache.GetCpaServiceSingle()
	if svc == nil || !svc.Enabled {
		return
	}
	managementKey := svc.ManagementKey
	if managementKey == "" {
		managementKey = c.managementKey
	}
	if managementKey == "" {
		return
	}

	cutoff := time.Now().UTC().Add(-codexSubscriptionFetchInterval)
	var targets []store.Account
	for _, acc := range c.cache.GetAccounts() {
		if acc.SourceKind != "cpa" || strings.ToLower(acc.CpaProvider) != "codex" {
			continue
		}
		if !acc.Enabled || acc.CpaDisabled {
			continue
		}
		if acc.CpaSubscriptionFetchedAt != "" {
			if t, err := time.Parse("2006-01-02 15:04:05", acc.CpaSubscriptionFetchedAt); err == nil && t.After(cutoff) {
				continue
			}
		}
		targets = append(targets, acc)
	}
	if len(targets) == 0 {
		return
	}

	client := cpa.NewManagementClient(svc.BaseURL, managementKey)
	keyToIndex, err := c.listAuthIndexes(ctx, client)
	if err != nil {
		slog.Warn("fetch codex subscriptions: list auth-files", "err", err)
		return
	}

	sem := make(chan struct{}, maxConcurrency)
	var wg sync.WaitGroup
	for _, acc := range targets {
		authIndex, ok := keyToIndex[acc.CpaAccountKey]
		if !ok || authIndex == "" {
			c.markCpaCredentialNeedsLogin(acc.ID, "auth_index_missing", "CPA auth file not found")
			continue
		}
		wg.Add(1)
		go func(a store.Account, idx string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if err := c.fetchOneCodexSubscription(ctx, client, a, idx); err != nil {
				slog.Warn("fetch codex subscription", "account_id", a.ID, "err", err)
			}
		}(acc, authIndex)
	}
	wg.Wait()
}

func (c *Checker) listAuthIndexes(ctx context.Context, client *cpa.ManagementClient) (map[string]string, error) {
	files, err := client.ListAuthFiles(ctx)
	if err != nil {
		return nil, err
	}
	keyToIndex := make(map[string]string, len(files))
	for _, f := range files {
		key := strings.TrimSuffix(f.ID, ".json")
		if key == "" {
			key = strings.TrimSuffix(f.Name, ".json")
		}
		if key == "" || f.AuthIndex == "" {
			continue
		}
		keyToIndex[key] = f.AuthIndex
	}
	return keyToIndex, nil
}

func (c *Checker) findAuthIndex(ctx context.Context, client *cpa.ManagementClient, accountKey string) (string, error) {
	keyToIndex, err := c.listAuthIndexes(ctx, client)
	if err != nil {
		return "", err
	}
	authIndex := keyToIndex[accountKey]
	if authIndex == "" {
		return "", fmt.Errorf("CPA auth file not found")
	}
	return authIndex, nil
}

func (c *Checker) fetchOneCodexQuota(ctx context.Context, client *cpa.ManagementClient, acc store.Account, authIndex string) error {
	header := map[string]string{
		"Authorization":      "Bearer $TOKEN$",
		"ChatGPT-Account-Id": acc.CpaOpenaiID,
	}
	resp, err := client.APICall(ctx, authIndex, http.MethodGet, "https://chatgpt.com/backend-api/wham/usage", header)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		if isCpaAuthFailure(resp.StatusCode, resp.Body) {
			c.markCpaCredentialNeedsLogin(acc.ID, credentialReasonFromAuthText(resp.Body), fmt.Sprintf("HTTP %d", resp.StatusCode))
		}
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body := strings.TrimSpace(resp.Body)
	if body == "" {
		return fmt.Errorf("empty quota response")
	}
	var sanity map[string]any
	if err := json.Unmarshal([]byte(body), &sanity); err != nil {
		return fmt.Errorf("invalid quota JSON: %w", err)
	}

	fetchedAt := time.Now().UTC().Format("2006-01-02 15:04:05")
	if err := c.store.UpdateAccountCodexQuota(acc.ID, body, fetchedAt); err != nil {
		return fmt.Errorf("persist quota: %w", err)
	}
	c.markCpaCredentialOK(acc.ID)
	c.cache.Invalidate()
	return nil
}

func (c *Checker) fetchOneCodexSubscription(ctx context.Context, client *cpa.ManagementClient, acc store.Account, authIndex string) error {
	header := map[string]string{
		"Authorization": "Bearer $TOKEN$",
	}
	if acc.CpaOpenaiID != "" {
		header["ChatGPT-Account-Id"] = acc.CpaOpenaiID
	}
	resp, err := client.APICall(ctx, authIndex, http.MethodGet, "https://chatgpt.com/backend-api/accounts/check/v4-2023-04-27", header)
	fetchedAt := time.Now().UTC().Format("2006-01-02 15:04:05")
	if err != nil {
		_ = c.store.UpdateAccountCpaSubscription(acc.ID, acc.CpaSubscriptionExpiresAt, fetchedAt, err.Error())
		return err
	}
	if resp.StatusCode != http.StatusOK {
		msg := fmt.Sprintf("HTTP %d", resp.StatusCode)
		if isCpaAuthFailure(resp.StatusCode, resp.Body) {
			c.markCpaCredentialNeedsLogin(acc.ID, credentialReasonFromAuthText(resp.Body), msg)
		}
		_ = c.store.UpdateAccountCpaSubscription(acc.ID, acc.CpaSubscriptionExpiresAt, fetchedAt, msg)
		return fmt.Errorf("%s", msg)
	}
	body := strings.TrimSpace(resp.Body)
	if body == "" {
		msg := "empty subscription response"
		_ = c.store.UpdateAccountCpaSubscription(acc.ID, acc.CpaSubscriptionExpiresAt, fetchedAt, msg)
		return fmt.Errorf("%s", msg)
	}
	expiresAt, err := parseCodexSubscriptionExpiry([]byte(body), acc.CpaOpenaiID)
	if err != nil {
		_ = c.store.UpdateAccountCpaSubscription(acc.ID, acc.CpaSubscriptionExpiresAt, fetchedAt, err.Error())
		return err
	}
	if err := c.store.UpdateAccountCpaSubscription(acc.ID, expiresAt, fetchedAt, ""); err != nil {
		return fmt.Errorf("persist subscription: %w", err)
	}
	c.markCpaCredentialOK(acc.ID)
	c.cache.Invalidate()
	return nil
}

func (c *Checker) syncCpaMetadata() {
	if c.cpaAuthDir == "" {
		return
	}

	accounts, err := c.store.ListCpaAccountsWithKey()
	if err != nil {
		slog.Error("sync cpa metadata: list accounts", "err", err)
		return
	}

	for _, acc := range accounts {
		f, err := cpa.ReadAuthFile(c.cpaAuthDir, acc.CpaAccountKey)
		if err != nil {
			if os.IsNotExist(err) {
				c.store.UpdateAccountHealth(acc.ID, "error", "Credential file not found")
				c.markCpaCredentialNeedsLogin(acc.ID, "file_missing", "Credential file not found")
			} else {
				c.store.UpdateAccountHealth(acc.ID, "error", "Credential file corrupt")
				c.markCpaCredentialNeedsLogin(acc.ID, "file_corrupt", "Credential file corrupt")
			}
			continue
		}
		c.store.UpdateAccountCpaMetadata(acc.ID, f.Expired, f.LastRefresh, f.Disabled)
		if f.Disabled {
			c.store.UpdateAccountHealth(acc.ID, "error", "CPA credential disabled")
			c.markCpaCredentialNeedsLogin(acc.ID, "disabled", "CPA credential disabled")
		}
	}
}

func (c *Checker) markCpaCredentialOK(accountID int64) {
	checkedAt := time.Now().UTC().Format(time.RFC3339)
	if err := c.store.UpdateAccountCpaCredentialStatus(accountID, "ok", "", "", checkedAt); err != nil {
		slog.Warn("mark cpa credential ok", "account_id", accountID, "err", err)
		return
	}
	c.cache.Invalidate()
}

func (c *Checker) markCpaCredentialNeedsLogin(accountID int64, reason, lastError string) {
	if reason == "" {
		reason = "auth_failed"
	}
	if lastError == "" {
		lastError = reason
	}
	checkedAt := time.Now().UTC().Format(time.RFC3339)
	if err := c.store.UpdateAccountCpaCredentialStatus(accountID, "needs_login", reason, lastError, checkedAt); err != nil {
		slog.Warn("mark cpa credential needs login", "account_id", accountID, "reason", reason, "err", err)
		return
	}
	c.cache.Invalidate()
}

func isCpaAuthFailure(statusCode int, body string) bool {
	if statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden {
		return true
	}
	text := strings.ToLower(body)
	return strings.Contains(text, "invalid_token") ||
		strings.Contains(text, "unauthorized") ||
		strings.Contains(text, "access denied") ||
		strings.Contains(text, "refresh token") ||
		strings.Contains(text, "expired token")
}

func credentialReasonFromAuthText(text string) string {
	lower := strings.ToLower(text)
	if strings.Contains(lower, "refresh") {
		return "refresh_failed"
	}
	return "auth_failed"
}

func formatHTTPStatusError(statusCode int, body []byte) string {
	msg := fmt.Sprintf("HTTP %d", statusCode)
	bodyText := strings.TrimSpace(string(body))
	if bodyText == "" {
		return msg
	}
	if len(bodyText) > 240 {
		bodyText = bodyText[:240]
	}
	return msg + ": " + bodyText
}

func (c *Checker) getInterval() time.Duration {
	return time.Duration(syscfg.ParsePositiveInt(c.cache.GetSetting("health_check_interval"), syscfg.DefaultHealthCheckInterval)) * time.Second
}

func (c *Checker) pruneRequestLogs() {
	retentionDays := syscfg.ParseNonNegativeInt(c.cache.GetSetting("data_retention_days"), syscfg.DefaultDataRetentionDays)
	deletedLogs, err := c.store.PruneRequestLogs(retentionDays)
	if err != nil {
		slog.Error("prune request logs", "err", err)
	} else if deletedLogs > 0 {
		slog.Info("pruned request logs", "deleted", deletedLogs, "retention_days", retentionDays)
	}

	deletedDeliveries, deletedOutbox, err := c.store.PruneNotificationHistory(retentionDays)
	if err != nil {
		slog.Error("prune notification history", "err", err)
		return
	}
	if deletedDeliveries > 0 || deletedOutbox > 0 {
		slog.Info(
			"pruned notification history",
			"deleted_deliveries", deletedDeliveries,
			"deleted_outbox", deletedOutbox,
			"retention_days", retentionDays,
		)
	}

	// Record the run unconditionally so the UI can show a recent
	// "last_prune_at" timestamp even when nothing was due for deletion —
	// that's how the user knows auto-prune is alive.
	if retentionDays > 0 {
		if err := c.store.RecordPruneRun(deletedLogs, deletedDeliveries, deletedOutbox); err != nil {
			slog.Error("record prune run", "err", err)
		}
	}
}

func (c *Checker) dispatchSystemNotifications(ctx context.Context) {
	if c.notifier == nil {
		return
	}

	notifications, err := c.store.ListSystemNotifications()
	if err != nil {
		slog.Error("list system notifications", "err", err)
		return
	}
	for _, item := range notifications {
		vars := map[string]any{
			"title":   item.Title,
			"message": item.Message,
		}
		if item.Label != "" {
			// Both account_* and cpa_service_error templates key off these
			// Vars names; populating both aliases keeps templates simple
			// regardless of the source type.
			vars["account_label"] = item.Label
			vars["service_label"] = item.Label
		}
		if item.LastError != "" {
			vars["last_error"] = item.LastError
		}
		if item.ExpiresAt != "" {
			vars["expires_at"] = item.ExpiresAt
		}
		n := notify.Notification{
			Event:     item.Type,
			Severity:  item.Severity,
			Title:     item.Title,
			Message:   item.Message,
			Timestamp: time.Now().UTC(),
			Source: notify.NotificationSource{
				AccountID: item.AccountID,
				ServiceID: item.ServiceID,
			},
			Vars: vars,
		}
		if err := c.notifier.Dispatch(ctx, n); err != nil {
			slog.Error("dispatch notification", "event", n.Event, "err", err)
		}
	}
}

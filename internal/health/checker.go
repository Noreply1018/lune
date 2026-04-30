package health

import (
	"context"
	"encoding/json"
	"errors"
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
	defaultCpaHealthAttempts       = 10
	defaultCpaHealthRetryDelay     = 500 * time.Millisecond
	defaultAuthIndexAttempts       = 10
	defaultAuthIndexRetryDelay     = 400 * time.Millisecond
)

type RefreshOptions struct {
	Models        bool
	Quota         bool
	Subscription  bool
	Force         bool
	WaitAuthIndex bool
}

type RefreshResult struct {
	Models []string `json:"models"`

	ModelsRefreshed       bool `json:"models_refreshed"`
	QuotaRefreshed        bool `json:"quota_refreshed"`
	SubscriptionRefreshed bool `json:"subscription_refreshed"`

	ModelsError       string `json:"models_error"`
	QuotaError        string `json:"quota_error"`
	SubscriptionError string `json:"subscription_error"`

	CredentialStatus string `json:"credential_status,omitempty"`
	CredentialReason string `json:"credential_reason,omitempty"`
}

type cpaRuntime struct {
	account  store.Account
	service  store.CpaService
	authFile cpa.CpaAuthFile
	authMeta authFileMetadata
	client   *cpa.ManagementClient
}

type resolveOptions struct {
	NeedAuthIndex bool
	WaitAuthIndex bool
}

type resolveError struct {
	status  string
	reason  string
	message string
}

func (e *resolveError) Error() string {
	return e.message
}

type Checker struct {
	store         *store.Store
	cache         *store.RoutingCache
	client        *http.Client
	cpaAuthDir    string
	managementKey string
	notifier      *notify.Service

	cpaHealthAttempts   int
	cpaHealthRetryDelay time.Duration
}

func NewChecker(st *store.Store, cache *store.RoutingCache, cpaAuthDir, managementKey string, notifier *notify.Service) *Checker {
	return &Checker{
		store:         st,
		cache:         cache,
		client:        &http.Client{Timeout: 15 * time.Second},
		cpaAuthDir:    cpaAuthDir,
		managementKey: managementKey,
		notifier:      notifier,

		cpaHealthAttempts:   defaultCpaHealthAttempts,
		cpaHealthRetryDelay: defaultCpaHealthRetryDelay,
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

	// Parse and refresh discovered models
	models := parseModelList(body)
	if len(models) > 0 {
		if err := c.store.RefreshAccountModels(acc.ID, models); err != nil {
			slog.Error("failed to refresh account models", "account_id", acc.ID, "err", err)
		}
	}
}

func (c *Checker) discoverModelsViaService(ctx context.Context, acc store.Account, svc store.CpaService) ([]string, error) {
	url := fmt.Sprintf("%s/api/provider/%s/v1/models", strings.TrimRight(svc.BaseURL, "/"), acc.CpaProvider)
	return c.discoverModelsFromURL(ctx, acc.ID, url, svc.APIKey)
}

func (c *Checker) discoverModelsFromURL(ctx context.Context, accountID int64, url, apiKey string) ([]string, error) {
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
		return nil, fmt.Errorf("%s", msg)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}

	models := parseModelList(body)
	if len(models) > 0 {
		if err := c.store.RefreshAccountModels(accountID, models); err != nil {
			return nil, fmt.Errorf("store models: %w", err)
		}
		c.cache.Invalidate()
	}
	return models, nil
}

// DiscoverModels triggers on-demand model discovery for a specific account.
func (c *Checker) DiscoverModels(ctx context.Context, acc store.Account) ([]string, error) {
	if acc.SourceKind == "cpa" && acc.CpaServiceID != nil {
		result, err := c.RefreshAccount(ctx, acc, RefreshOptions{Models: true})
		if result != nil {
			return result.Models, err
		}
		return nil, err
	}
	url := fmt.Sprintf("%s/models", strings.TrimRight(acc.BaseURL, "/"))
	return c.discoverModelsFromURL(ctx, acc.ID, url, acc.APIKey)
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
	result, err := c.RefreshAccount(ctx, acc, RefreshOptions{Quota: true, WaitAuthIndex: true})
	if result == nil {
		return false, err
	}
	return result.QuotaRefreshed, err
}

func (c *Checker) RefreshCodexSubscription(ctx context.Context, acc store.Account) (bool, error) {
	if acc.SourceKind != "cpa" || strings.ToLower(acc.CpaProvider) != "codex" {
		return false, nil
	}
	if !acc.Enabled || acc.CpaDisabled {
		return false, nil
	}
	result, err := c.RefreshAccount(ctx, acc, RefreshOptions{Subscription: true, WaitAuthIndex: true})
	if result == nil {
		return false, err
	}
	return result.SubscriptionRefreshed, err
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
	if err := c.probeCpaService(ctx, svc); err != nil {
		c.store.UpdateCpaServiceHealth(svc.ID, "error", err.Error())
		c.cache.Invalidate()
		return
	}

	c.store.UpdateCpaServiceHealth(svc.ID, "healthy", "")
	c.cache.Invalidate()
}

func (c *Checker) probeCpaService(ctx context.Context, svc *store.CpaService) error {
	url := fmt.Sprintf("%s/healthz", strings.TrimRight(svc.BaseURL, "/"))
	var lastErr error

	attempts := c.cpaHealthAttempts
	if attempts < 1 {
		attempts = 1
	}

	for attempt := 1; attempt <= attempts; attempt++ {
		retryable, err := c.probeCpaHealthURL(ctx, url, svc.APIKey)
		if err == nil {
			return nil
		}
		lastErr = err

		if !retryable || attempt == attempts {
			break
		}
		if !sleepContext(ctx, c.cpaHealthRetryDelay) {
			return ctx.Err()
		}
	}

	return lastErr
}

func (c *Checker) probeCpaHealthURL(ctx context.Context, url, apiKey string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, err
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return true, err
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	return false, nil
}

func sleepContext(ctx context.Context, delay time.Duration) bool {
	if delay <= 0 {
		return true
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func (c *Checker) resolveCpaRuntime(ctx context.Context, acc store.Account, opts resolveOptions) (*cpaRuntime, error) {
	if acc.SourceKind != "cpa" {
		return nil, &resolveError{status: "unknown", reason: "not_cpa", message: "account is not a CPA account"}
	}
	if acc.CpaServiceID == nil {
		err := &resolveError{status: "runtime_error", reason: "service_missing", message: "missing CPA service"}
		c.markCpaCredentialState(acc.ID, err.status, err.reason, err.message)
		return nil, err
	}
	svc := c.cache.GetCpaService(*acc.CpaServiceID)
	if svc == nil || !svc.Enabled {
		err := &resolveError{status: "runtime_error", reason: "runtime_unreachable", message: "CPA service unreachable"}
		c.markCpaCredentialState(acc.ID, err.status, err.reason, err.message)
		return nil, err
	}
	if c.cpaAuthDir == "" {
		err := &resolveError{status: "runtime_error", reason: "auth_dir_missing", message: "cpa_auth_dir is not configured"}
		c.markCpaCredentialState(acc.ID, err.status, err.reason, err.message)
		return nil, err
	}

	authFile, err := cpa.ReadAuthFile(c.cpaAuthDir, acc.CpaAccountKey)
	if err != nil {
		if os.IsNotExist(err) {
			rerr := &resolveError{status: "needs_login", reason: "file_missing", message: "Credential file not found"}
			c.store.UpdateAccountHealth(acc.ID, "error", rerr.message)
			c.markCpaCredentialState(acc.ID, rerr.status, rerr.reason, rerr.message)
			return nil, rerr
		}
		rerr := &resolveError{status: "needs_login", reason: "file_corrupt", message: "Credential file corrupt"}
		c.store.UpdateAccountHealth(acc.ID, "error", rerr.message)
		c.markCpaCredentialState(acc.ID, rerr.status, rerr.reason, rerr.message)
		return nil, rerr
	}
	if err := c.store.UpdateAccountCpaMetadata(acc.ID, authFile.Expired, authFile.LastRefresh, authFile.Disabled); err != nil {
		slog.Warn("resolve cpa runtime: update metadata", "account_id", acc.ID, "err", err)
	}
	if authFile.Disabled {
		rerr := &resolveError{status: "needs_login", reason: "disabled", message: "CPA credential disabled"}
		c.store.UpdateAccountHealth(acc.ID, "error", rerr.message)
		c.markCpaCredentialState(acc.ID, rerr.status, rerr.reason, rerr.message)
		return nil, rerr
	}

	managementKey := svc.ManagementKey
	if managementKey == "" {
		managementKey = c.managementKey
	}
	client := cpa.NewManagementClient(svc.BaseURL, managementKey)
	rt := &cpaRuntime{
		account:  acc,
		service:  *svc,
		authFile: *authFile,
		client:   client,
	}

	if strings.ToLower(authFile.Type) == "codex" {
		if expiresAt := cpa.SubscriptionActiveUntilFromTokens(authFile.IDToken, authFile.AccessToken); expiresAt != "" {
			fetchedAt := time.Now().UTC().Format("2006-01-02 15:04:05")
			_ = c.store.UpdateAccountCpaSubscription(acc.ID, expiresAt, fetchedAt, "")
		}
	}
	if !opts.NeedAuthIndex {
		return rt, nil
	}
	if managementKey == "" {
		rerr := &resolveError{status: "runtime_error", reason: "management_key_missing", message: "CPA management key is empty"}
		c.markCpaCredentialState(acc.ID, rerr.status, rerr.reason, rerr.message)
		return nil, rerr
	}

	attempts := 1
	if opts.WaitAuthIndex {
		attempts = defaultAuthIndexAttempts
	}
	var lastErr error
	for i := 0; i < attempts; i++ {
		meta, err := c.findAuthMetadata(ctx, client, acc.CpaAccountKey)
		if err == nil && meta.authIndex != "" {
			rt.authMeta = meta
			c.markCpaCredentialOK(acc.ID)
			return rt, nil
		}
		lastErr = err
		if i < attempts-1 && !sleepContext(ctx, defaultAuthIndexRetryDelay) {
			return nil, ctx.Err()
		}
	}

	message := "CPA runtime has not loaded this credential yet"
	if lastErr != nil && !isAuthIndexMissingError(lastErr) {
		message = lastErr.Error()
		rerr := &resolveError{status: "runtime_error", reason: "runtime_unreachable", message: message}
		c.markCpaCredentialState(acc.ID, rerr.status, rerr.reason, rerr.message)
		return nil, rerr
	}
	rerr := &resolveError{status: "runtime_pending", reason: "auth_index_pending", message: message}
	c.markCpaCredentialState(acc.ID, rerr.status, rerr.reason, rerr.message)
	return nil, rerr
}

func isAuthIndexMissingError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "auth file not found")
}

func (c *Checker) RefreshAccount(ctx context.Context, acc store.Account, opts RefreshOptions) (*RefreshResult, error) {
	result := &RefreshResult{}
	if !opts.Models && !opts.Quota && !opts.Subscription {
		opts.Models = true
	}
	if acc.SourceKind != "cpa" {
		models, err := c.DiscoverModels(ctx, acc)
		if err != nil {
			result.ModelsError = err.Error()
			return result, err
		}
		result.Models = models
		result.ModelsRefreshed = true
		return result, nil
	}

	needAuthIndex := opts.Quota || (opts.Subscription && strings.ToLower(acc.CpaProvider) == "codex")
	rt, err := c.resolveCpaRuntime(ctx, acc, resolveOptions{NeedAuthIndex: needAuthIndex, WaitAuthIndex: opts.WaitAuthIndex})
	if err != nil {
		result.CredentialStatus, result.CredentialReason = cpaResolveState(err)
		if opts.Models {
			result.ModelsError = err.Error()
		}
		if opts.Quota {
			result.QuotaError = err.Error()
		}
		if opts.Subscription {
			result.SubscriptionError = err.Error()
		}
		return result, err
	}
	result.CredentialStatus = "ok"

	var firstErr error
	if opts.Models {
		models, err := c.discoverModelsViaService(ctx, acc, rt.service)
		if err != nil {
			result.ModelsError = err.Error()
			firstErr = err
		} else {
			result.Models = models
			result.ModelsRefreshed = true
		}
	}
	if opts.Quota && strings.ToLower(acc.CpaProvider) == "codex" {
		refreshed, err := c.refreshCodexQuotaResolved(ctx, rt)
		result.QuotaRefreshed = refreshed
		if err != nil {
			result.QuotaError = err.Error()
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	if opts.Subscription && strings.ToLower(acc.CpaProvider) == "codex" {
		refreshed, err := c.refreshCodexSubscriptionResolved(rt)
		result.SubscriptionRefreshed = refreshed
		if err != nil {
			result.SubscriptionError = err.Error()
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	c.cache.Invalidate()
	return result, firstErr
}

func cpaResolveState(err error) (status, reason string) {
	var rerr *resolveError
	if errors.As(err, &rerr) {
		return rerr.status, rerr.reason
	}
	return "runtime_error", "runtime_unreachable"
}

// fetchCodexQuotas pulls `wham/usage` through CPA api-call for every enabled
// Codex account whose `codex_quota_fetched_at` is older than the configured
// interval, then persists the raw JSON. Failures are logged and skipped — the
// next tick retries, so transient hiccups do not cascade into account errors.
func (c *Checker) fetchCodexQuotas(ctx context.Context) {
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

	sem := make(chan struct{}, maxConcurrency)
	var wg sync.WaitGroup
	for _, acc := range targets {
		wg.Add(1)
		go func(a store.Account) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if _, err := c.RefreshCodexQuota(ctx, a); err != nil {
				slog.Warn("fetch codex quota", "account_id", a.ID, "err", err)
			}
		}(acc)
	}
	wg.Wait()
}

func (c *Checker) fetchCodexSubscriptions(ctx context.Context) {
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

	for _, acc := range targets {
		if _, err := c.RefreshCodexSubscription(ctx, acc); err != nil {
			slog.Warn("fetch codex subscription", "account_id", acc.ID, "err", err)
		}
	}
}

type authFileMetadata struct {
	authIndex             string
	subscriptionExpiresAt string
}

func (c *Checker) listAuthMetadata(ctx context.Context, client *cpa.ManagementClient) (map[string]authFileMetadata, error) {
	files, err := client.ListAuthFiles(ctx)
	if err != nil {
		return nil, err
	}
	keyToMeta := make(map[string]authFileMetadata, len(files))
	for _, f := range files {
		if f.AuthIndex == "" {
			continue
		}
		meta := authFileMetadata{
			authIndex:             f.AuthIndex,
			subscriptionExpiresAt: cpa.NormalizeSubscriptionActiveUntil(f.IDToken.ChatGPTSubscriptionActiveUntil),
		}
		for _, key := range authFileCandidateKeys(f) {
			keyToMeta[key] = meta
		}
	}
	return keyToMeta, nil
}

func authFileCandidateKeys(f cpa.AuthFile) []string {
	seen := make(map[string]bool)
	var keys []string
	add := func(key string) {
		key = strings.TrimSuffix(strings.TrimSpace(key), ".json")
		if key == "" || seen[key] {
			return
		}
		seen[key] = true
		keys = append(keys, key)
	}
	add(f.ID)
	add(f.Name)
	provider := f.Provider
	if provider == "" {
		provider = f.Type
	}
	plan := f.IDToken.PlanType
	if plan == "" {
		plan = "unknown"
	}
	if provider != "" && f.Email != "" {
		add(fmt.Sprintf("%s-%s-%s", provider, f.Email, plan))
	}
	return keys
}

func (c *Checker) findAuthIndex(ctx context.Context, client *cpa.ManagementClient, accountKey string) (string, error) {
	keyToMeta, err := c.listAuthMetadata(ctx, client)
	if err != nil {
		return "", err
	}
	authMeta, ok := keyToMeta[accountKey]
	if !ok || authMeta.authIndex == "" {
		return "", fmt.Errorf("CPA auth file not found")
	}
	return authMeta.authIndex, nil
}

func (c *Checker) findAuthMetadata(ctx context.Context, client *cpa.ManagementClient, accountKey string) (authFileMetadata, error) {
	keyToMeta, err := c.listAuthMetadata(ctx, client)
	if err != nil {
		return authFileMetadata{}, err
	}
	authMeta, ok := keyToMeta[accountKey]
	if !ok || authMeta.authIndex == "" {
		return authFileMetadata{}, fmt.Errorf("CPA auth file not found")
	}
	return authMeta, nil
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

func (c *Checker) refreshCodexQuotaResolved(ctx context.Context, rt *cpaRuntime) (bool, error) {
	if rt.account.CpaOpenaiID == "" {
		return false, fmt.Errorf("missing ChatGPT account id")
	}
	if rt.authMeta.authIndex == "" {
		return false, fmt.Errorf("CPA runtime has not loaded this credential yet")
	}
	if err := c.fetchOneCodexQuota(ctx, rt.client, rt.account, rt.authMeta.authIndex); err != nil {
		return false, err
	}
	return true, nil
}

func (c *Checker) refreshCodexSubscriptionResolved(rt *cpaRuntime) (bool, error) {
	if expiresAt := cpa.SubscriptionActiveUntilFromTokens(rt.authFile.IDToken, rt.authFile.AccessToken); expiresAt != "" {
		fetchedAt := time.Now().UTC().Format("2006-01-02 15:04:05")
		if err := c.store.UpdateAccountCpaSubscription(rt.account.ID, expiresAt, fetchedAt, ""); err != nil {
			return false, fmt.Errorf("persist subscription: %w", err)
		}
		c.cache.Invalidate()
		return true, nil
	}
	if rt.authMeta.subscriptionExpiresAt != "" {
		fetchedAt := time.Now().UTC().Format("2006-01-02 15:04:05")
		if err := c.store.UpdateAccountCpaSubscription(rt.account.ID, rt.authMeta.subscriptionExpiresAt, fetchedAt, ""); err != nil {
			return false, fmt.Errorf("persist subscription: %w", err)
		}
		c.cache.Invalidate()
		return true, nil
	}
	fetchedAt := time.Now().UTC().Format("2006-01-02 15:04:05")
	msg := "subscription metadata pending"
	_ = c.store.UpdateAccountCpaSubscription(rt.account.ID, rt.account.CpaSubscriptionExpiresAt, fetchedAt, msg)
	return false, fmt.Errorf("%s", msg)
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
		if strings.ToLower(f.Type) == "codex" {
			if expiresAt := cpa.SubscriptionActiveUntilFromTokens(f.IDToken, f.AccessToken); expiresAt != "" {
				fetchedAt := time.Now().UTC().Format("2006-01-02 15:04:05")
				_ = c.store.UpdateAccountCpaSubscription(acc.ID, expiresAt, fetchedAt, "")
			}
		}
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

func (c *Checker) markCpaCredentialState(accountID int64, status, reason, lastError string) {
	if status == "" {
		status = "unknown"
	}
	if lastError == "" {
		lastError = reason
	}
	checkedAt := time.Now().UTC().Format(time.RFC3339)
	if err := c.store.UpdateAccountCpaCredentialStatus(accountID, status, reason, lastError, checkedAt); err != nil {
		slog.Warn("mark cpa credential state", "account_id", accountID, "status", status, "reason", reason, "err", err)
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

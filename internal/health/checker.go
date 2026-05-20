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
	"path/filepath"
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
	accountErrorDiscoveryGrace     = 5 * time.Minute
	maxConcurrency                 = 10
	codexSubscriptionFetchInterval = 6 * time.Hour
	codexQuotaBackoffBase          = 5 * time.Minute
	codexQuotaBackoffMax           = time.Hour
	defaultCpaHealthAttempts       = 10
	defaultCpaHealthRetryDelay     = 500 * time.Millisecond
	defaultAuthIndexAttempts       = 10
	defaultAuthIndexRetryDelay     = 400 * time.Millisecond
)

type RefreshOptions struct {
	Models        bool
	Quota         bool
	Subscription  bool
	WaitAuthIndex bool
}

type RefreshResult struct {
	Models []string `json:"models"`

	ModelsRefreshed       bool `json:"models_refreshed"`
	QuotaRefreshed        bool `json:"quota_refreshed"`
	SubscriptionRefreshed bool `json:"subscription_refreshed"`
	QuotaPending          bool `json:"quota_pending"`
	SubscriptionPending   bool `json:"subscription_pending"`

	ModelsError       string `json:"models_error"`
	QuotaError        string `json:"quota_error"`
	SubscriptionError string `json:"subscription_error"`

	CredentialStatus string `json:"credential_status,omitempty"`
	CredentialReason string `json:"credential_reason,omitempty"`
}

type RuntimeBinding struct {
	AccountID     int64
	AccountKey    string
	AuthID        string
	AuthIndex     string
	OpenAIID      string
	Provider      string
	Email         string
	PlanType      string
	BindingStatus string
	BindingReason string
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
	ReadOnly      bool
}

type resolveError struct {
	status  string
	reason  string
	message string
}

func (e *resolveError) Error() string {
	return e.message
}

func (e *resolveError) Status() string {
	if e == nil {
		return ""
	}
	return e.status
}

func (e *resolveError) Reason() string {
	if e == nil {
		return ""
	}
	return e.reason
}

type Checker struct {
	store               *store.Store
	cache               *store.RoutingCache
	client              *http.Client
	cpaAuthDir          string
	managementKey       string
	cpaReloadSignalPath string
	notifier            *notify.Service

	cpaHealthAttempts        int
	cpaHealthRetryDelay      time.Duration
	authIndexAttempts        int
	authIndexRetryDelay      time.Duration
	providerPinningSupported bool
	reloadMu                 sync.Mutex
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
		authIndexAttempts:   defaultAuthIndexAttempts,
		authIndexRetryDelay: defaultAuthIndexRetryDelay,
	}
}

func (c *Checker) SetCpaReloadSignalPath(path string) {
	c.cpaReloadSignalPath = strings.TrimSpace(path)
}

func (c *Checker) SetProviderPinningSupported(supported bool) {
	c.providerPinningSupported = supported
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

	start := time.Now()
	if _, err := c.discoverModelsFromURL(ctx, acc.ID, url, apiKey); err != nil {
		c.store.UpdateAccountHealth(acc.ID, "error", err.Error())
		return
	}
	latency := time.Since(start)
	current := acc
	if latest, err := c.store.GetAccount(acc.ID); err == nil && latest != nil {
		current = *latest
	}
	if shouldPreserveAccountErrorDuringDiscovery(current, time.Now()) {
		return
	}
	status := "healthy"
	lastError := ""
	if latency > degradedLatencyThreshold {
		status = "degraded"
		lastError = fmt.Sprintf("slow response: %s", latency)
	}
	if _, err := c.store.UpdateAccountHealthIfUnchanged(acc.ID, status, lastError, current); err != nil {
		slog.Warn("update discovery health", "account_id", acc.ID, "err", err)
	}
}

func shouldPreserveAccountErrorDuringDiscovery(acc store.Account, now time.Time) bool {
	if acc.Status != "error" || isModelDiscoveryHealthError(acc.LastError) || acc.LastCheckedAt == nil || strings.TrimSpace(*acc.LastCheckedAt) == "" {
		return false
	}
	checkedAt, ok := parseAccountCheckedAt(*acc.LastCheckedAt)
	if !ok {
		return false
	}
	if checkedAt.After(now) {
		return true
	}
	return now.Sub(checkedAt) < accountErrorDiscoveryGrace
}

func isModelDiscoveryHealthError(message string) bool {
	message = strings.TrimSpace(message)
	if message == "" {
		return false
	}
	lower := strings.ToLower(message)
	if strings.HasPrefix(message, "Get ") && strings.Contains(lower, "/models") {
		return true
	}
	if message == "CPA service unreachable" {
		return true
	}
	return false
}

func parseAccountCheckedAt(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05",
	} {
		if ts, err := time.Parse(layout, value); err == nil {
			return ts, true
		}
	}
	return time.Time{}, false
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
		if !opts.ReadOnly {
			c.markCpaCredentialState(acc.ID, err.status, err.reason, err.message)
		}
		return nil, err
	}
	svc := c.cache.GetCpaService(*acc.CpaServiceID)
	if svc == nil || !svc.Enabled {
		err := &resolveError{status: "runtime_error", reason: "runtime_unreachable", message: "CPA service unreachable"}
		if !opts.ReadOnly {
			c.markCpaCredentialState(acc.ID, err.status, err.reason, err.message)
		}
		return nil, err
	}
	if c.cpaAuthDir == "" {
		err := &resolveError{status: "runtime_error", reason: "auth_dir_missing", message: "cpa_auth_dir is not configured"}
		if !opts.ReadOnly {
			c.markCpaCredentialState(acc.ID, err.status, err.reason, err.message)
		}
		return nil, err
	}

	authFile, err := cpa.ReadAuthFile(c.cpaAuthDir, acc.CpaAccountKey)
	if err != nil {
		if os.IsNotExist(err) {
			rerr := &resolveError{status: "needs_login", reason: "file_missing", message: "Credential file not found"}
			if !opts.ReadOnly {
				c.store.UpdateAccountHealth(acc.ID, "error", rerr.message)
				c.markCpaCredentialState(acc.ID, rerr.status, rerr.reason, rerr.message)
			}
			return nil, rerr
		}
		rerr := &resolveError{status: "needs_login", reason: "file_corrupt", message: "Credential file corrupt"}
		if !opts.ReadOnly {
			c.store.UpdateAccountHealth(acc.ID, "error", rerr.message)
			c.markCpaCredentialState(acc.ID, rerr.status, rerr.reason, rerr.message)
		}
		return nil, rerr
	}
	if !opts.ReadOnly && !authFileMetadataIsOlder(acc.CpaLastRefreshAt, authFile.LastRefresh) {
		if err := c.store.UpdateAccountCpaMetadata(acc.ID, authFile.Expired, authFile.LastRefresh, authFile.Disabled); err != nil {
			slog.Warn("resolve cpa runtime: update metadata", "account_id", acc.ID, "err", err)
		}
	} else {
		slog.Warn("resolve cpa runtime: skip older auth file metadata", "account_id", acc.ID, "account_key", acc.CpaAccountKey, "db_last_refresh", acc.CpaLastRefreshAt, "file_last_refresh", authFile.LastRefresh)
	}
	if authFile.Disabled {
		rerr := &resolveError{status: "needs_login", reason: "disabled", message: "CPA credential disabled"}
		if !opts.ReadOnly {
			c.store.UpdateAccountHealth(acc.ID, "error", rerr.message)
			c.markCpaCredentialState(acc.ID, rerr.status, rerr.reason, rerr.message)
		}
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

	if !opts.ReadOnly && strings.ToLower(authFile.Type) == "codex" {
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
		if !opts.ReadOnly {
			c.markCpaCredentialState(acc.ID, rerr.status, rerr.reason, rerr.message)
		}
		return nil, rerr
	}

	meta, err := c.resolveAuthMetadata(ctx, client, *svc, acc.CpaAccountKey, *authFile, opts)
	if err == nil && meta.authIndex != "" {
		rt.authMeta = meta
		if !opts.ReadOnly {
			c.markCpaCredentialOK(acc.ID)
		}
		return rt, nil
	}

	message := "CPA credential is still syncing"
	if err != nil && !isAuthIndexMissingError(err) {
		message = err.Error()
		rerr := &resolveError{status: "runtime_error", reason: "runtime_unreachable", message: message}
		if !opts.ReadOnly {
			c.markCpaCredentialState(acc.ID, rerr.status, rerr.reason, rerr.message)
		}
		return nil, rerr
	}
	rerr := &resolveError{status: "runtime_pending", reason: "auth_index_pending", message: message}
	if !opts.ReadOnly {
		c.markCpaCredentialState(acc.ID, rerr.status, rerr.reason, rerr.message)
	}
	return nil, rerr
}

func (c *Checker) ResolveRuntimeBinding(ctx context.Context, acc store.Account, waitAuthIndex bool, readOnly bool) (*RuntimeBinding, error) {
	rt, err := c.resolveCpaRuntime(ctx, acc, resolveOptions{NeedAuthIndex: true, WaitAuthIndex: waitAuthIndex, ReadOnly: readOnly})
	if err != nil {
		return nil, err
	}
	return &RuntimeBinding{
		AccountID:     acc.ID,
		AccountKey:    acc.CpaAccountKey,
		AuthID:        rt.authMeta.id,
		AuthIndex:     rt.authMeta.authIndex,
		OpenAIID:      firstNonEmpty(rt.authMeta.openaiID, acc.CpaOpenaiID),
		Provider:      firstNonEmpty(rt.authMeta.provider, acc.CpaProvider),
		Email:         firstNonEmpty(rt.authMeta.email, acc.CpaEmail),
		PlanType:      firstNonEmpty(rt.authMeta.planType, acc.CpaPlanType),
		BindingStatus: "confirmed",
		BindingReason: "",
	}, nil
}

func (c *Checker) ProviderPinningSupported() bool {
	return c.providerPinningSupported
}

func RuntimeBindingErrorState(err error) (status, reason string) {
	type stateReason interface {
		Status() string
		Reason() string
	}
	var sr stateReason
	if errors.As(err, &sr) {
		return sr.Status(), sr.Reason()
	}
	var rerr *resolveError
	if errors.As(err, &rerr) {
		return rerr.status, rerr.reason
	}
	return "runtime_error", "runtime_auth_binding_unavailable"
}

func (c *Checker) resolveAuthMetadata(ctx context.Context, client *cpa.ManagementClient, svc store.CpaService, accountKey string, authFile cpa.CpaAuthFile, opts resolveOptions) (authFileMetadata, error) {
	attempts := 1
	if opts.WaitAuthIndex {
		attempts = c.authIndexAttempts
		if attempts < 1 {
			attempts = 1
		}
	}
	meta, err := c.waitForAuthMetadata(ctx, client, accountKey, attempts)
	if err == nil && authMetadataMatchesAuthFile(meta, authFile) {
		return meta, nil
	}
	if err == nil {
		err = fmt.Errorf("CPA auth index metadata mismatch")
	}
	if !opts.WaitAuthIndex || opts.ReadOnly || (!isAuthIndexMissingError(err) && !isAuthMetadataMismatchError(err)) || c.cpaReloadSignalPath == "" {
		return meta, err
	}

	if reloadErr := c.requestCpaRuntimeReload(ctx, &svc); reloadErr != nil {
		return authFileMetadata{}, reloadErr
	}
	meta, err = c.waitForAuthMetadata(ctx, client, accountKey, attempts)
	if err != nil {
		return meta, err
	}
	if !authMetadataMatchesAuthFile(meta, authFile) {
		return meta, fmt.Errorf("CPA auth index metadata mismatch")
	}
	return meta, nil
}

func (c *Checker) waitForAuthMetadata(ctx context.Context, client *cpa.ManagementClient, accountKey string, attempts int) (authFileMetadata, error) {
	var lastErr error
	for i := 0; i < attempts; i++ {
		meta, err := c.findAuthMetadata(ctx, client, accountKey)
		if err == nil && meta.authIndex != "" {
			return meta, nil
		}
		lastErr = err
		if i < attempts-1 && !sleepContext(ctx, c.authIndexRetryDelay) {
			return authFileMetadata{}, ctx.Err()
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("CPA auth index not ready")
	}
	return authFileMetadata{}, lastErr
}

func (c *Checker) requestCpaRuntimeReload(ctx context.Context, svc *store.CpaService) error {
	c.reloadMu.Lock()
	defer c.reloadMu.Unlock()

	if err := os.MkdirAll(filepath.Dir(c.cpaReloadSignalPath), 0755); err != nil {
		return fmt.Errorf("prepare CPA reload signal: %w", err)
	}
	payload := []byte(time.Now().UTC().Format(time.RFC3339Nano))
	if err := os.WriteFile(c.cpaReloadSignalPath, payload, 0600); err != nil {
		return fmt.Errorf("write CPA reload signal: %w", err)
	}
	slog.Warn("requested embedded CPA restart for auth index reconciliation", "signal", c.cpaReloadSignalPath)

	attempts := c.cpaHealthAttempts
	if attempts < 1 {
		attempts = 1
	}
	for i := 0; i < attempts; i++ {
		if !sleepContext(ctx, c.cpaHealthRetryDelay) {
			return ctx.Err()
		}
		if err := c.probeCpaService(ctx, svc); err == nil {
			return nil
		}
	}
	return fmt.Errorf("CPA runtime reload did not become healthy")
}

func isAuthIndexMissingError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "auth index not ready") || strings.Contains(text, "auth file not found")
}

func isAuthMetadataMismatchError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "metadata mismatch")
}

func (c *Checker) RequestCpaRuntimeReload(ctx context.Context, svc *store.CpaService) error {
	if strings.TrimSpace(c.cpaReloadSignalPath) == "" {
		return nil
	}
	return c.requestCpaRuntimeReload(ctx, svc)
}

func (c *Checker) RefreshAccount(ctx context.Context, acc store.Account, opts RefreshOptions) (*RefreshResult, error) {
	result := &RefreshResult{}
	if !opts.Models && !opts.Quota && !opts.Subscription {
		opts.Models = true
	}
	if acc.SourceKind != "cpa" {
		url := fmt.Sprintf("%s/models", strings.TrimRight(acc.BaseURL, "/"))
		models, err := c.discoverModelsFromURL(ctx, acc.ID, url, acc.APIKey)
		if err != nil {
			result.ModelsError = err.Error()
			return result, err
		}
		result.Models = models
		result.ModelsRefreshed = true
		return result, nil
	}

	rt, err := c.resolveCpaRuntime(ctx, acc, resolveOptions{})
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
	result.CredentialStatus = rt.account.CpaCredentialStatus

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
	if opts.Subscription && strings.ToLower(acc.CpaProvider) == "codex" {
		refreshed, err := c.refreshCodexSubscriptionResolved(rt)
		result.SubscriptionRefreshed = refreshed
		if err != nil {
			if err.Error() == "subscription metadata pending" {
				result.SubscriptionPending = true
			} else {
				result.SubscriptionError = err.Error()
			}
			if firstErr == nil && result.SubscriptionError != "" {
				firstErr = err
			}
		}
	}
	if opts.Quota && strings.ToLower(acc.CpaProvider) == "codex" {
		credentialBeforeQuota := acc
		if latest, getErr := c.store.GetAccount(acc.ID); getErr == nil && latest != nil {
			credentialBeforeQuota = *latest
		}
		authRuntime, err := c.resolveCpaRuntime(ctx, acc, resolveOptions{NeedAuthIndex: true, WaitAuthIndex: opts.WaitAuthIndex})
		if err != nil {
			status, reason := cpaResolveState(err)
			result.CredentialStatus = status
			result.CredentialReason = reason
			if status == "runtime_pending" && reason == "auth_index_pending" {
				result.QuotaPending = true
				if opts.Subscription && !result.SubscriptionRefreshed && result.SubscriptionError == "" {
					result.SubscriptionPending = true
				}
			} else {
				result.QuotaError = err.Error()
			}
			if firstErr == nil && result.QuotaError != "" {
				firstErr = err
			}
		} else {
			c.restoreCpaCredentialStateIfNeeded(credentialBeforeQuota)
			refreshed, err := c.refreshCodexQuotaResolved(ctx, authRuntime)
			result.QuotaRefreshed = refreshed
			if err != nil {
				result.QuotaError = err.Error()
				if firstErr == nil {
					firstErr = err
				}
			}
			if opts.Subscription && result.SubscriptionPending {
				refreshed, err := c.refreshCodexSubscriptionResolved(authRuntime)
				result.SubscriptionRefreshed = refreshed
				result.SubscriptionPending = false
				if err != nil {
					if err.Error() == "subscription metadata pending" {
						result.SubscriptionPending = true
					} else {
						result.SubscriptionError = err.Error()
					}
					if firstErr == nil && result.SubscriptionError != "" {
						firstErr = err
					}
				}
			}
		}
	} else if opts.Subscription && strings.ToLower(acc.CpaProvider) == "codex" && result.SubscriptionPending {
		authRuntime, err := c.resolveCpaRuntime(ctx, acc, resolveOptions{NeedAuthIndex: true, WaitAuthIndex: opts.WaitAuthIndex})
		if err != nil {
			status, reason := cpaResolveState(err)
			result.CredentialStatus = status
			result.CredentialReason = reason
			if status == "runtime_pending" && reason == "auth_index_pending" {
				result.SubscriptionPending = true
				result.SubscriptionError = ""
			} else {
				result.SubscriptionPending = false
				result.SubscriptionError = err.Error()
			}
			if firstErr == nil && result.SubscriptionError != "" {
				firstErr = err
			}
		} else {
			refreshed, err := c.refreshCodexSubscriptionResolved(authRuntime)
			result.SubscriptionRefreshed = refreshed
			result.SubscriptionPending = false
			if err != nil {
				if err.Error() == "subscription metadata pending" {
					result.SubscriptionPending = true
					result.SubscriptionError = ""
				} else {
					result.SubscriptionError = err.Error()
					if firstErr == nil {
						firstErr = err
					}
				}
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
		if acc.CpaQuotaBackoffUntil != "" {
			if t, err := time.Parse("2006-01-02 15:04:05", acc.CpaQuotaBackoffUntil); err == nil && t.After(time.Now().UTC()) {
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
			if _, err := c.RefreshAccount(ctx, a, RefreshOptions{Quota: true, WaitAuthIndex: true}); err != nil {
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
		if _, err := c.RefreshAccount(ctx, acc, RefreshOptions{Subscription: true, WaitAuthIndex: true}); err != nil {
			slog.Warn("fetch codex subscription", "account_id", acc.ID, "err", err)
		}
	}
}

type authFileMetadata struct {
	id                    string
	authIndex             string
	subscriptionExpiresAt string
	provider              string
	email                 string
	openaiID              string
	planType              string
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
			id:                    f.ID,
			authIndex:             f.AuthIndex,
			subscriptionExpiresAt: cpa.NormalizeSubscriptionActiveUntil(f.IDToken.ChatGPTSubscriptionActiveUntil),
			provider:              firstNonEmpty(f.Provider, f.Type),
			email:                 f.Email,
			openaiID:              f.IDToken.ChatGPTAccountID,
			planType:              f.IDToken.PlanType,
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

func (c *Checker) findAuthMetadata(ctx context.Context, client *cpa.ManagementClient, accountKey string) (authFileMetadata, error) {
	keyToMeta, err := c.listAuthMetadata(ctx, client)
	if err != nil {
		return authFileMetadata{}, err
	}
	authMeta, ok := keyToMeta[accountKey]
	if !ok || authMeta.authIndex == "" {
		return authFileMetadata{}, fmt.Errorf("CPA auth index not ready")
	}
	return authMeta, nil
}

func authMetadataMatchesAuthFile(meta authFileMetadata, authFile cpa.CpaAuthFile) bool {
	if meta.provider != "" && authFile.Type != "" && !strings.EqualFold(meta.provider, authFile.Type) {
		return false
	}
	if meta.email != "" && authFile.Email != "" && !strings.EqualFold(meta.email, authFile.Email) {
		return false
	}
	if meta.openaiID != "" && authFile.AccountID != "" && meta.openaiID != authFile.AccountID {
		return false
	}
	if meta.planType != "" {
		if info, err := cpa.ParseAccountInfoFromTokens(authFile.IDToken, authFile.AccessToken); err == nil && info.PlanType != "" && info.PlanType != meta.planType {
			return false
		}
	}
	return true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
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
	checkedAt := time.Now().UTC().Format("2006-01-02 15:04:05")
	if resp.StatusCode != http.StatusOK {
		msg := fmt.Sprintf("HTTP %d", resp.StatusCode)
		status := "error"
		if previousQuotaSnapshotBlocked(acc.CodexQuotaJSON) {
			status = "blocked"
		}
		_ = c.store.UpdateAccountCodexQuotaStatus(acc.ID, status, msg, checkedAt)
		_ = c.store.UpdateAccountCodexQuotaBackoff(acc.ID, nextCodexQuotaBackoffUntil(acc.CpaQuotaBackoffCount, time.Now().UTC()), acc.CpaQuotaBackoffCount+1)
		if isQuotaProbeAuthFailureStatus(resp.StatusCode) {
			c.recordCodexQuotaProbeDiagnostic(acc, resp.StatusCode, msg)
		}
		c.cache.Invalidate()
		return fmt.Errorf("%s", msg)
	}
	body := strings.TrimSpace(resp.Body)
	if body == "" {
		_ = c.store.UpdateAccountCodexQuotaStatus(acc.ID, "error", "empty quota response", checkedAt)
		_ = c.store.UpdateAccountCodexQuotaBackoff(acc.ID, nextCodexQuotaBackoffUntil(acc.CpaQuotaBackoffCount, time.Now().UTC()), acc.CpaQuotaBackoffCount+1)
		return fmt.Errorf("empty quota response")
	}
	var sanity map[string]any
	if err := json.Unmarshal([]byte(body), &sanity); err != nil {
		_ = c.store.UpdateAccountCodexQuotaStatus(acc.ID, "error", "invalid quota JSON", checkedAt)
		_ = c.store.UpdateAccountCodexQuotaBackoff(acc.ID, nextCodexQuotaBackoffUntil(acc.CpaQuotaBackoffCount, time.Now().UTC()), acc.CpaQuotaBackoffCount+1)
		return fmt.Errorf("invalid quota JSON: %w", err)
	}

	fetchedAt := checkedAt
	if err := c.store.UpdateAccountCodexQuota(acc.ID, body, fetchedAt); err != nil {
		return fmt.Errorf("persist quota: %w", err)
	}
	blocked := quotaSnapshotBlocked(sanity)
	if !blocked {
		if err := c.store.UpdateAccountCpaAccessStatus(acc.ID, "eligible", "wham_usage_allowed", "", time.Now().UTC().Format(time.RFC3339)); err != nil {
			return fmt.Errorf("persist access status: %w", err)
		}
	}
	if blocked {
		if err := c.store.UpdateAccountCodexQuotaStatus(acc.ID, "blocked", "quota blocked by upstream", checkedAt); err != nil {
			return fmt.Errorf("persist quota status: %w", err)
		}
	}
	c.cache.Invalidate()
	return nil
}

func (c *Checker) recordCodexQuotaProbeDiagnostic(acc store.Account, httpStatus int, msg string) {
	stableStatus := "unknown"
	probeStatus := "quota_probe_auth_failed"
	safeSummary := "quota probe authentication failed"
	preserveStable := true
	current, err := c.store.GetAccountDiagnostic(acc.ID)
	if err != nil {
		slog.Warn("load account diagnostic for quota probe", "account_id", acc.ID, "err", err)
		return
	}
	if current != nil {
		stableStatus = current.StableDiagnosticStatus
		if stableStatus == "" {
			stableStatus = "unknown"
		}
		switch stableStatus {
		case "usable", "quota_probe_auth_failed_but_usable":
			if hasFreshUsableDiagnostic(current) {
				stableStatus = "quota_probe_auth_failed_but_usable"
				safeSummary = "quota probe authentication failed but account remains usable"
				preserveStable = false
			}
		}
	}
	previousStable := "unknown"
	if current != nil && strings.TrimSpace(current.StableDiagnosticStatus) != "" {
		previousStable = current.StableDiagnosticStatus
	}
	status := httpStatus
	operationID := store.AccountKeyHash(fmt.Sprintf("account-diagnostic:%d:%s", acc.ID, time.Now().UTC().Format(time.RFC3339Nano)))
	if err := c.store.UpdateAccountDiagnosticWithEvidence(acc.ID, store.AccountDiagnosticUpdate{
		OperationID:                    operationID,
		StableDiagnosticStatus:         stableStatus,
		PreviousStableDiagnosticStatus: previousStable,
		LastProbeStatus:                probeStatus,
		SafeSummary:                    safeSummary,
		PreserveStableStatus:           preserveStable,
	}, store.AccountDiagnosticEvidenceInput{
		ProbeType:           "quota_probe",
		Stage:               "wham_usage",
		HTTPStatus:          &status,
		UpstreamErrorCode:   "quota_probe_auth_failed",
		NormalizedErrorCode: "quota_probe_auth_failed",
		SafeMessage:         "quota probe authentication failed",
		ObservedAt:          time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		slog.Warn("record quota probe diagnostic", "account_id", acc.ID, "err", err)
	}
}

func hasFreshUsableDiagnostic(diag *store.AccountDiagnostic) bool {
	if diag == nil {
		return false
	}
	if !strings.EqualFold(diag.StableDiagnosticStatus, "usable") &&
		!strings.EqualFold(diag.StableDiagnosticStatus, "quota_probe_auth_failed_but_usable") {
		return false
	}
	return strings.EqualFold(diag.LastProbeStatus, "succeeded")
}

func isQuotaProbeAuthFailureStatus(statusCode int) bool {
	return statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden
}

func nextCodexQuotaBackoffUntil(failures int, now time.Time) string {
	if failures < 0 {
		failures = 0
	}
	delay := codexQuotaBackoffBase
	for i := 0; i < failures; i++ {
		delay *= 2
		if delay >= codexQuotaBackoffMax {
			delay = codexQuotaBackoffMax
			break
		}
	}
	return now.Add(delay).UTC().Format("2006-01-02 15:04:05")
}

func quotaSnapshotBlocked(raw map[string]any) bool {
	for _, key := range []string{"allowed", "is_allowed"} {
		if v, ok := raw[key].(bool); ok && !v {
			return true
		}
	}
	for _, key := range []string{"limit_reached", "limited", "blocked"} {
		if v, ok := raw[key].(bool); ok && v {
			return true
		}
	}
	for _, key := range []string{"rate_limit", "limits"} {
		if nested, ok := raw[key].(map[string]any); ok && quotaSnapshotBlocked(nested) {
			return true
		}
	}
	return false
}

func previousQuotaSnapshotBlocked(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	var snapshot map[string]any
	if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
		return false
	}
	return quotaSnapshotBlocked(snapshot)
}

func (c *Checker) refreshCodexQuotaResolved(ctx context.Context, rt *cpaRuntime) (bool, error) {
	if rt.account.CpaOpenaiID == "" {
		return false, fmt.Errorf("missing ChatGPT account id")
	}
	if rt.authMeta.authIndex == "" {
		return false, fmt.Errorf("CPA credential is still syncing")
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
		if !authFileMetadataIsOlder(acc.CpaLastRefreshAt, f.LastRefresh) {
			c.store.UpdateAccountCpaMetadata(acc.ID, f.Expired, f.LastRefresh, f.Disabled)
		} else {
			slog.Warn("sync cpa metadata: skip older auth file metadata", "account_id", acc.ID, "account_key", acc.CpaAccountKey, "db_last_refresh", acc.CpaLastRefreshAt, "file_last_refresh", f.LastRefresh)
			continue
		}
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

func authFileMetadataIsOlder(existingLastRefresh, fileLastRefresh string) bool {
	existing, okExisting := parseFlexibleTime(existingLastRefresh)
	file, okFile := parseFlexibleTime(fileLastRefresh)
	if !okExisting || !okFile {
		return false
	}
	return file.Before(existing)
}

func parseFlexibleTime(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"} {
		t, err := time.Parse(layout, value)
		if err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func (c *Checker) markCpaCredentialOK(accountID int64) {
	checkedAt := time.Now().UTC().Format(time.RFC3339)
	if err := c.store.UpdateAccountCpaCredentialStatus(accountID, "ok", "", "", checkedAt); err != nil {
		slog.Warn("mark cpa credential ok", "account_id", accountID, "err", err)
		return
	}
	c.cache.Invalidate()
}

func (c *Checker) restoreCpaCredentialStateIfNeeded(previous store.Account) {
	status := strings.TrimSpace(previous.CpaCredentialStatus)
	if status == "" || status == "unknown" || status == "ok" || status == "runtime_pending" || status == "runtime_error" {
		return
	}
	checkedAt := previous.CpaCredentialCheckedAt
	if checkedAt == "" {
		checkedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if err := c.store.UpdateAccountCpaCredentialStatus(previous.ID, status, previous.CpaCredentialReason, previous.CpaCredentialLastError, checkedAt); err != nil {
		slog.Warn("restore cpa credential state", "account_id", previous.ID, "status", status, "err", err)
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
	result, err := c.store.PruneDataRetention(retentionDays, "health_checker")
	if err != nil {
		slog.Error("prune data retention", "err", err)
		return
	}
	if result.DeletedLogs > 0 || result.DeletedDeliveries > 0 || result.DeletedOutbox > 0 ||
		result.DeletedOperations > 0 || result.DeletedOperationItems > 0 {
		slog.Info(
			"pruned data retention",
			"deleted_logs", result.DeletedLogs,
			"deleted_deliveries", result.DeletedDeliveries,
			"deleted_outbox", result.DeletedOutbox,
			"deleted_operations", result.DeletedOperations,
			"deleted_operation_items", result.DeletedOperationItems,
			"retention_days", retentionDays,
		)
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

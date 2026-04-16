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
	"lune/internal/store"
	"lune/internal/syscfg"
	"lune/internal/webhook"
)

const (
	degradedLatencyThreshold = 5 * time.Second
	maxConcurrency           = 10
)

type Checker struct {
	store               *store.Store
	cache               *store.RoutingCache
	client              *http.Client
	cpaAuthDir          string
	webhook             *webhook.Sender
	lastNotifiedMu      sync.Mutex
	lastNotified        map[string]time.Time
	notifying           map[string]bool
	inFlightSeq         map[string]uint64
	nextNotifySeq       uint64
	notificationBackoff time.Duration
}

func NewChecker(st *store.Store, cache *store.RoutingCache, cpaAuthDir string, sender *webhook.Sender) *Checker {
	return &Checker{
		store:               st,
		cache:               cache,
		client:              &http.Client{Timeout: 15 * time.Second},
		cpaAuthDir:          cpaAuthDir,
		webhook:             sender,
		lastNotified:        make(map[string]time.Time),
		notifying:           make(map[string]bool),
		inFlightSeq:         make(map[string]uint64),
		notificationBackoff: time.Hour,
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
	c.pruneRequestLogs()

	c.cache.Invalidate()
	c.sendWebhookNotifications(ctx)
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
		c.store.UpdateAccountHealth(acc.ID, "error", fmt.Sprintf("HTTP %d", resp.StatusCode))
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
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
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
			} else {
				c.store.UpdateAccountHealth(acc.ID, "error", "Credential file corrupt")
			}
			continue
		}
		c.store.UpdateAccountCpaMetadata(acc.ID, f.Expired, f.LastRefresh, f.Disabled)
	}
}

func (c *Checker) getInterval() time.Duration {
	return time.Duration(syscfg.ParsePositiveInt(c.cache.GetSetting("health_check_interval"), syscfg.DefaultHealthCheckInterval)) * time.Second
}

func (c *Checker) pruneRequestLogs() {
	retentionDays := syscfg.ParseNonNegativeInt(c.cache.GetSetting("data_retention_days"), syscfg.DefaultDataRetentionDays)
	deleted, err := c.store.PruneRequestLogs(retentionDays)
	if err != nil {
		slog.Error("prune request logs", "err", err)
		return
	}
	if deleted > 0 {
		slog.Info("pruned request logs", "deleted", deleted, "retention_days", retentionDays)
	}
}

func (c *Checker) sendWebhookNotifications(ctx context.Context) {
	if c.webhook == nil {
		return
	}

	webhookEnabled := syscfg.ParseBool(c.cache.GetSetting("webhook_enabled"), syscfg.DefaultWebhookEnabled)
	webhookURL := strings.TrimSpace(c.cache.GetSetting("webhook_url"))
	if !webhookEnabled || webhookURL == "" {
		return
	}

	notifications, err := c.store.ListSystemNotifications()
	if err != nil {
		slog.Error("list system notifications for webhook", "err", err)
		return
	}

	now := time.Now().UTC()
	activeKeys := make(map[string]struct{}, len(notifications))

	c.lastNotifiedMu.Lock()
	for _, notification := range notifications {
		key := notificationDedupKey(notification)
		if key == "" {
			continue
		}
		activeKeys[key] = struct{}{}
	}
	for key := range c.lastNotified {
		if _, ok := activeKeys[key]; !ok {
			delete(c.lastNotified, key)
		}
	}
	for key := range c.notifying {
		if _, ok := activeKeys[key]; !ok {
			delete(c.notifying, key)
			delete(c.inFlightSeq, key)
		}
	}
	c.lastNotifiedMu.Unlock()

	for _, notification := range notifications {
		key := notificationDedupKey(notification)
		if key == "" {
			continue
		}

		c.lastNotifiedMu.Lock()
		lastSentAt, exists := c.lastNotified[key]
		inFlight := c.notifying[key]
		var seq uint64
		if !inFlight && (!exists || now.Sub(lastSentAt) >= c.notificationBackoff) {
			c.notifying[key] = true
			c.nextNotifySeq++
			seq = c.nextNotifySeq
			c.inFlightSeq[key] = seq
		}
		c.lastNotifiedMu.Unlock()
		if inFlight || (exists && now.Sub(lastSentAt) < c.notificationBackoff) {
			continue
		}

		payload := webhook.Payload{
			Event:     notification.Type,
			Severity:  notification.Severity,
			Title:     notification.Title,
			Message:   notification.Message,
			Timestamp: now.Format(time.RFC3339),
		}
		go c.deliverWebhookNotification(ctx, webhookURL, key, seq, payload)
	}
}

func (c *Checker) deliverWebhookNotification(ctx context.Context, webhookURL, key string, seq uint64, payload webhook.Payload) {
	err := c.webhook.Send(ctx, webhookURL, payload)

	c.lastNotifiedMu.Lock()
	defer c.lastNotifiedMu.Unlock()

	currentSeq, ok := c.inFlightSeq[key]
	if !ok || currentSeq != seq {
		return
	}

	delete(c.notifying, key)
	delete(c.inFlightSeq, key)
	if err != nil {
		slog.Error("deliver webhook notification", "event", payload.Event, "err", err)
		return
	}

	c.lastNotified[key] = time.Now().UTC()
}

func notificationDedupKey(notification store.SystemNotification) string {
	switch notification.Type {
	case "account_error", "account_expiring":
		if notification.AccountID == nil {
			return ""
		}
		return fmt.Sprintf("%s:%d:%s:%s", notification.Type, *notification.AccountID, notification.Severity, notification.Title)
	case "cpa_service_error":
		if notification.ServiceID == nil {
			return ""
		}
		return fmt.Sprintf("%s:%d:%s:%s", notification.Type, *notification.ServiceID, notification.Severity, notification.Title)
	default:
		return ""
	}
}

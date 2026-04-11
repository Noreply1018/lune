package platform

import (
	"context"
	"sync"
	"time"

	"lune/internal/config"
)

type Status struct {
	PlatformID          string    `json:"platform_id"`
	Enabled             bool      `json:"enabled"`
	Healthy             bool      `json:"healthy"`
	LastChecked         time.Time `json:"last_checked"`
	LastError           string    `json:"last_error"`
	Type                string    `json:"type"`
	Adapter             string    `json:"adapter"`
	AccountCount        int       `json:"account_count"`
	EnabledAccountCount int       `json:"enabled_account_count"`
	PoolCount           int       `json:"pool_count"`
	ActivePoolCount     int       `json:"active_pool_count"`
}

type Registry struct {
	getConfig func() config.Config

	mu       sync.RWMutex
	statuses map[string]Status
}

func New(getConfig func() config.Config) *Registry {
	cfg := getConfig()
	statuses := make(map[string]Status, len(cfg.Platforms))
	registry := &Registry{
		getConfig: getConfig,
		statuses:  statuses,
	}
	registry.CheckAll(context.Background())
	return registry
}

func (r *Registry) Snapshot() []Status {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]Status, 0, len(r.statuses))
	for _, status := range r.statuses {
		out = append(out, status)
	}
	return out
}

func (r *Registry) Status(platformID string) (Status, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	status, ok := r.statuses[platformID]
	return status, ok
}

func (r *Registry) Start(ctx context.Context, interval time.Duration) {
	r.CheckAll(ctx)
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.CheckAll(ctx)
			}
		}
	}()
}

func (r *Registry) CheckAll(ctx context.Context) {
	_ = ctx
	cfg := r.getConfig()
	for _, platform := range cfg.Platforms {
		r.checkOne(cfg, platform)
	}
}

func (r *Registry) checkOne(cfg config.Config, platform config.Platform) {
	status := Status{
		PlatformID:  platform.ID,
		Enabled:     platform.Enabled,
		Healthy:     platform.Enabled,
		LastChecked: time.Now().UTC(),
		Type:        platform.Type,
		Adapter:     platform.Adapter,
	}

	for _, account := range cfg.Accounts {
		if account.Platform != platform.ID {
			continue
		}
		status.AccountCount++
		if account.Enabled && isRunnableAccount(account.Status) {
			status.EnabledAccountCount++
		}
	}

	for _, pool := range cfg.AccountPools {
		if pool.Platform != platform.ID {
			continue
		}
		status.PoolCount++
		if pool.Enabled {
			status.ActivePoolCount++
		}
	}

	switch {
	case !platform.Enabled:
		status.Healthy = false
		status.LastError = "platform is disabled"
	case status.EnabledAccountCount == 0:
		status.Healthy = false
		status.LastError = "no runnable accounts"
	case status.ActivePoolCount == 0:
		status.Healthy = false
		status.LastError = "no enabled account pools"
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.statuses[status.PlatformID] = status
}

func isRunnableAccount(status string) bool {
	switch status {
	case "", "healthy", "ready", "active":
		return true
	default:
		return false
	}
}

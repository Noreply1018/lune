package health

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"lune/internal/store"
)

type Checker struct {
	store  *store.Store
	cache  *store.RoutingCache
	logger *log.Logger
	client *http.Client
}

func NewChecker(st *store.Store, cache *store.RoutingCache, logger *log.Logger) *Checker {
	return &Checker{
		store:  st,
		cache:  cache,
		logger: logger,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Checker) Run(ctx context.Context) {
	interval := c.getInterval()
	c.logger.Printf("health checker started (interval: %s)", interval)

	c.checkAll(ctx)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.checkAll(ctx)
		case <-ctx.Done():
			c.logger.Println("health checker stopped")
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
	if len(accounts) == 0 {
		return
	}

	var wg sync.WaitGroup
	for _, acc := range accounts {
		if !acc.Enabled {
			continue
		}
		wg.Add(1)
		go func(a store.Account) {
			defer wg.Done()
			c.checkOne(ctx, a)
		}(acc)
	}
	wg.Wait()
	c.cache.Invalidate()
}

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
		url = fmt.Sprintf("%s/models", acc.BaseURL)
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
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.store.UpdateAccountHealth(acc.ID, "error", fmt.Sprintf("HTTP %d", resp.StatusCode))
		return
	}

	if latency > 5*time.Second {
		c.store.UpdateAccountHealth(acc.ID, "degraded", fmt.Sprintf("slow response: %s", latency))
		return
	}

	c.store.UpdateAccountHealth(acc.ID, "healthy", "")
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

func (c *Checker) getInterval() time.Duration {
	v := c.cache.GetSetting("health_check_interval")
	if n, err := strconv.Atoi(v); err == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	return 60 * time.Second
}

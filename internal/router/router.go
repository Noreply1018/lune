package router

import (
	"errors"
	"strings"

	"lune/internal/config"
	"lune/internal/execution"
)

var (
	ErrModelNotFound      = errors.New("model alias not found")
	ErrNoAvailableAccount = errors.New("no runnable account found for model")
)

type Router struct {
	models    map[string]config.ModelRoute
	pools     map[string]config.AccountPool
	accounts  map[string]config.Account
	platforms map[string]config.Platform
}

func New(cfg config.Config) *Router {
	models := make(map[string]config.ModelRoute, len(cfg.Models))
	for _, model := range cfg.Models {
		models[model.Alias] = model
	}

	pools := make(map[string]config.AccountPool, len(cfg.AccountPools))
	for _, pool := range cfg.AccountPools {
		pools[pool.ID] = pool
	}

	accounts := make(map[string]config.Account, len(cfg.Accounts))
	for _, account := range cfg.Accounts {
		accounts[account.ID] = account
	}

	platforms := make(map[string]config.Platform, len(cfg.Platforms))
	for _, platform := range cfg.Platforms {
		platforms[platform.ID] = platform
	}

	return &Router{
		models:    models,
		pools:     pools,
		accounts:  accounts,
		platforms: platforms,
	}
}

func (r *Router) Resolve(req execution.Request) ([]execution.Plan, error) {
	model, ok := r.models[req.ModelAlias]
	if !ok {
		return nil, ErrModelNotFound
	}

	primaryPoolID := strings.TrimSpace(model.TargetID)
	if primaryPoolID == "" {
		primaryPoolID = strings.TrimSpace(model.AccountPool)
	}

	candidates := r.poolCandidates(primaryPoolID, model.TargetModel, 0)
	for _, fallback := range model.Fallbacks {
		poolID, targetModel := parseFallback(fallback, model.TargetModel)
		candidates = append(candidates, r.poolCandidates(poolID, targetModel, len(candidates))...)
	}

	if len(candidates) == 0 {
		return nil, ErrNoAvailableAccount
	}
	return candidates, nil
}

func (r *Router) poolCandidates(poolID string, targetModel string, offset int) []execution.Plan {
	pool, ok := r.pools[strings.TrimSpace(poolID)]
	if !ok || !pool.Enabled {
		return nil
	}

	candidates := make([]execution.Plan, 0, len(pool.Members))
	for _, memberID := range pool.Members {
		account, ok := r.accounts[strings.TrimSpace(memberID)]
		if !ok || !account.Enabled || !isRunnableAccount(account.Status) {
			continue
		}

		platformID := account.Platform
		if platformID == "" {
			platformID = pool.Platform
		}
		if platformID == "" {
			continue
		}
		platform := r.platforms[platformID]
		adapterID := platform.Adapter
		if strings.TrimSpace(adapterID) == "" {
			adapterID = platform.Type
		}

		candidates = append(candidates, execution.Plan{
			PoolID:       pool.ID,
			PlatformID:   platformID,
			AccountID:    account.ID,
			TargetModel:  targetModel,
			AdapterID:    strings.TrimSpace(adapterID),
			AttemptIndex: offset + len(candidates),
		})
	}
	return candidates
}

func parseFallback(raw string, defaultTargetModel string) (string, string) {
	poolID, targetModel, ok := strings.Cut(strings.TrimSpace(raw), ":")
	if !ok {
		return strings.TrimSpace(raw), defaultTargetModel
	}
	if strings.TrimSpace(targetModel) == "" {
		return strings.TrimSpace(poolID), defaultTargetModel
	}
	return strings.TrimSpace(poolID), strings.TrimSpace(targetModel)
}

func isRunnableAccount(status string) bool {
	switch status {
	case "", "healthy", "ready", "active":
		return true
	default:
		return false
	}
}

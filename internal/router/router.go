package router

import (
	"errors"
	"strings"
	"time"

	"lune/internal/store"
)

var (
	ErrNoRoute           = errors.New("no_route")
	ErrPoolDisabled      = errors.New("pool_disabled")
	ErrNoHealthyAccount  = errors.New("no_healthy_account")
	ErrModelNotOnAccount = errors.New("model_not_on_account")
)

type Router struct {
	cache *store.RoutingCache
}

func New(cache *store.RoutingCache) *Router {
	return &Router{cache: cache}
}

type ResolvedRoute struct {
	PoolID      int64
	TargetModel string
	AccountID   int64
	Account     store.Account
}

// Resolve finds the best account for the given model within the token's Pool.
// forceAccountID bypasses normal member ordering but must still belong to the
// token Pool.
func (rt *Router) Resolve(model string, tokenPoolID *int64, forceAccountID *int64) (*ResolvedRoute, error) {
	snap := rt.cache.Get()
	if tokenPoolID == nil {
		return nil, ErrNoRoute
	}

	// Force-route to a specific account (for inline testing)
	if forceAccountID != nil {
		return rt.resolveToAccount(snap, model, *tokenPoolID, *forceAccountID)
	}

	return rt.resolveInPool(snap, model, *tokenPoolID)
}

// SelectNextAccount finds the next available account for retry, excluding already-tried accounts.
func (rt *Router) SelectNextAccount(model string, tokenPoolID *int64, exclude []int64) (*ResolvedRoute, error) {
	if tokenPoolID == nil {
		return nil, ErrNoRoute
	}
	return rt.resolveInPool(rt.cache.Get(), model, *tokenPoolID, exclude...)
}

func (rt *Router) resolveToAccount(snap *store.CacheSnapshot, model string, poolID, accountID int64) (*ResolvedRoute, error) {
	pool, ok := snap.Pools[poolID]
	if !ok || !pool.Enabled {
		return nil, ErrPoolDisabled
	}
	memberOK := false
	for _, m := range snap.Members[poolID] {
		if m.AccountID == accountID && m.Enabled {
			memberOK = true
			break
		}
	}
	if !memberOK {
		return nil, ErrNoRoute
	}

	acc, ok := snap.Accounts[accountID]
	if !ok {
		return nil, ErrNoRoute
	}
	if !accountRoutable(acc) {
		return nil, ErrNoHealthyAccount
	}
	// When the account has a discovered model list, reject models that are not
	// on it so force-account probes fail fast with a clear error instead of
	// surfacing an opaque upstream "model not found". Empty list means model
	// discovery hasn't populated anything yet; pass through optimistically so
	// we don't break accounts whose upstream lacks /v1/models.
	if len(acc.Models) > 0 {
		supported := false
		for _, m := range acc.Models {
			if m == model {
				supported = true
				break
			}
		}
		if !supported {
			return nil, ErrModelNotOnAccount
		}
	}

	return &ResolvedRoute{
		PoolID:      poolID,
		TargetModel: model,
		AccountID:   accountID,
		Account:     *acc,
	}, nil
}

func (rt *Router) resolveInPool(snap *store.CacheSnapshot, model string, poolID int64, exclude ...int64) (*ResolvedRoute, error) {
	pool, ok := snap.Pools[poolID]
	if !ok || !pool.Enabled {
		return nil, ErrPoolDisabled
	}

	members, ok := snap.Members[poolID]
	if !ok || len(members) == 0 {
		return nil, ErrNoHealthyAccount
	}

	excludeSet := makeExcludeSet(exclude)

	// Try accounts that have the model, in position order
	for _, m := range members {
		if !m.Enabled || excludeSet[m.AccountID] {
			continue
		}
		acc, ok := snap.Accounts[m.AccountID]
		if !ok || !accountRoutable(acc) {
			continue
		}
		if !accountHasModel(snap, m.AccountID, model) {
			continue
		}
		return &ResolvedRoute{
			PoolID:      poolID,
			TargetModel: model,
			AccountID:   m.AccountID,
			Account:     *acc,
		}, nil
	}

	// Fallback within pool: try first healthy account regardless of model
	for _, m := range members {
		if !m.Enabled || excludeSet[m.AccountID] {
			continue
		}
		acc, ok := snap.Accounts[m.AccountID]
		if !ok || !accountRoutable(acc) {
			continue
		}
		return &ResolvedRoute{
			PoolID:      poolID,
			TargetModel: model,
			AccountID:   m.AccountID,
			Account:     *acc,
		}, nil
	}

	return nil, ErrNoHealthyAccount
}

func accountRoutable(acc *store.Account) bool {
	if acc == nil || !acc.Enabled {
		return false
	}
	if acc.Status != "healthy" && acc.Status != "degraded" {
		return false
	}
	if strings.EqualFold(acc.ServingStatus, "cooldown") {
		if cooldownUntil, ok := parseRouteTime(acc.CooldownUntil); !ok || cooldownUntil.After(time.Now().UTC()) {
			return false
		}
	}
	if acc.SourceKind == "cpa" {
		switch strings.ToLower(acc.CpaCredentialStatus) {
		case "needs_login", "refresh_failed", "runtime_pending", "runtime_error", "auth_suspect":
			return false
		}
		if strings.EqualFold(acc.CpaQuotaStatus, "blocked") {
			return false
		}
	}
	return true
}

func parseRouteTime(value string) (time.Time, bool) {
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

func accountHasModel(snap *store.CacheSnapshot, accountID int64, model string) bool {
	// Check ModelIndex: iterate all account IDs for this model
	accountIDs, ok := snap.ModelIndex[model]
	if !ok {
		return false
	}
	for _, id := range accountIDs {
		if id == accountID {
			return true
		}
	}
	return false
}

func makeExcludeSet(exclude []int64) map[int64]bool {
	if len(exclude) == 0 {
		return nil
	}
	s := make(map[int64]bool, len(exclude))
	for _, id := range exclude {
		s[id] = true
	}
	return s
}

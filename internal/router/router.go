package router

import (
	"errors"
	"sort"

	"lune/internal/store"
)

var (
	ErrNoRoute          = errors.New("no_route")
	ErrPoolDisabled     = errors.New("pool_disabled")
	ErrNoHealthyAccount = errors.New("no_healthy_account")
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

// Resolve finds the best (pool, account) for the given model.
// tokenPoolID scopes to a specific pool (Pool Token); nil means global routing.
// forceAccountID bypasses routing and sends to a specific account (inline test).
func (rt *Router) Resolve(model string, tokenPoolID *int64, forceAccountID *int64) (*ResolvedRoute, error) {
	snap := rt.cache.Get()

	// Force-route to a specific account (for inline testing)
	if forceAccountID != nil {
		return rt.resolveToAccount(snap, model, *forceAccountID)
	}

	if tokenPoolID != nil {
		// Pool Token: only route within the specified pool
		return rt.resolveInPool(snap, model, *tokenPoolID)
	}

	// Global Token: use ModelIndex to find which accounts support this model
	accountIDs, ok := snap.ModelIndex[model]
	if ok && len(accountIDs) > 0 {
		return rt.selectBestCandidate(snap, accountIDs, model, nil)
	}

	// Fallback: model not in index (discovery not yet complete)
	return rt.resolveFallback(snap, model)
}

// SelectNextAccount finds the next available account for retry, excluding already-tried accounts.
func (rt *Router) SelectNextAccount(model string, tokenPoolID *int64, exclude []int64) (*ResolvedRoute, error) {
	snap := rt.cache.Get()

	if tokenPoolID != nil {
		return rt.resolveInPool(snap, model, *tokenPoolID, exclude...)
	}

	accountIDs, ok := snap.ModelIndex[model]
	if ok && len(accountIDs) > 0 {
		return rt.selectBestCandidate(snap, accountIDs, model, exclude)
	}

	return rt.resolveFallback(snap, model, exclude...)
}

func (rt *Router) resolveToAccount(snap *store.CacheSnapshot, model string, accountID int64) (*ResolvedRoute, error) {
	acc, ok := snap.Accounts[accountID]
	if !ok {
		return nil, ErrNoRoute
	}
	if !acc.Enabled {
		return nil, ErrNoHealthyAccount
	}

	// Find which pool this account belongs to
	var poolID int64
	for pid, members := range snap.Members {
		for _, m := range members {
			if m.AccountID == accountID {
				poolID = pid
				break
			}
		}
		if poolID > 0 {
			break
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
		if !ok || !acc.Enabled {
			continue
		}
		if acc.Status != "healthy" && acc.Status != "degraded" {
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
		if !ok || !acc.Enabled {
			continue
		}
		if acc.Status != "healthy" && acc.Status != "degraded" {
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

// candidate represents a (pool, account) pair for ranking.
type candidate struct {
	poolPriority int
	position     int
	poolID       int64
	accountID    int64
}

func (rt *Router) selectBestCandidate(snap *store.CacheSnapshot, accountIDs []int64, model string, exclude []int64) (*ResolvedRoute, error) {
	excludeSet := makeExcludeSet(exclude)

	var candidates []candidate

	for _, accID := range accountIDs {
		if excludeSet[accID] {
			continue
		}
		acc, ok := snap.Accounts[accID]
		if !ok || !acc.Enabled {
			continue
		}
		if acc.Status != "healthy" && acc.Status != "degraded" {
			continue
		}

		// Find this account's pool(s) and position
		for poolID, members := range snap.Members {
			pool, ok := snap.Pools[poolID]
			if !ok || !pool.Enabled {
				continue
			}
			for _, m := range members {
				if m.AccountID == accID && m.Enabled {
					candidates = append(candidates, candidate{
						poolPriority: pool.Priority,
						position:     m.Position,
						poolID:       poolID,
						accountID:    accID,
					})
				}
			}
		}
	}

	if len(candidates) == 0 {
		return nil, ErrNoHealthyAccount
	}

	// Sort: pool priority ASC, then position ASC, then pool ID ASC
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].poolPriority != candidates[j].poolPriority {
			return candidates[i].poolPriority < candidates[j].poolPriority
		}
		if candidates[i].position != candidates[j].position {
			return candidates[i].position < candidates[j].position
		}
		return candidates[i].poolID < candidates[j].poolID
	})

	best := candidates[0]
	acc := snap.Accounts[best.accountID]
	return &ResolvedRoute{
		PoolID:      best.poolID,
		TargetModel: model,
		AccountID:   best.accountID,
		Account:     *acc,
	}, nil
}

func (rt *Router) resolveFallback(snap *store.CacheSnapshot, model string, exclude ...int64) (*ResolvedRoute, error) {
	excludeSet := makeExcludeSet(exclude)

	// Sort pools by priority (ascending)
	type poolEntry struct {
		id       int64
		priority int
	}
	var pools []poolEntry
	for _, p := range snap.Pools {
		if p.Enabled {
			pools = append(pools, poolEntry{id: p.ID, priority: p.Priority})
		}
	}
	sort.Slice(pools, func(i, j int) bool {
		if pools[i].priority != pools[j].priority {
			return pools[i].priority < pools[j].priority
		}
		return pools[i].id < pools[j].id
	})

	// Try the first healthy account from each pool in priority order
	for _, pe := range pools {
		members, ok := snap.Members[pe.id]
		if !ok {
			continue
		}
		for _, m := range members {
			if !m.Enabled || excludeSet[m.AccountID] {
				continue
			}
			acc, ok := snap.Accounts[m.AccountID]
			if !ok || !acc.Enabled {
				continue
			}
			if acc.Status != "healthy" && acc.Status != "degraded" {
				continue
			}
			return &ResolvedRoute{
				PoolID:      pe.id,
				TargetModel: model,
				AccountID:   m.AccountID,
				Account:     *acc,
			}, nil
		}
	}

	return nil, ErrNoRoute
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

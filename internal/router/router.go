package router

import (
	"errors"
	"math/rand/v2"

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
	PoolID        int64
	TargetModel   string
	Alias         string
	IsDefaultPool bool
}

func (rt *Router) Resolve(modelAlias string) (*ResolvedRoute, error) {
	route := rt.cache.FindRoute(modelAlias)
	if route != nil {
		return &ResolvedRoute{
			PoolID:      route.PoolID,
			TargetModel: route.TargetModel,
			Alias:       modelAlias,
		}, nil
	}

	defaultPoolID := rt.cache.GetDefaultPoolID()
	if defaultPoolID == 0 {
		return nil, ErrNoRoute
	}
	pool := rt.cache.GetPool(defaultPoolID)
	if pool == nil || !pool.Enabled {
		return nil, ErrNoRoute
	}
	return &ResolvedRoute{
		PoolID:        defaultPoolID,
		TargetModel:   modelAlias, // no rewrite for default pool
		Alias:         modelAlias,
		IsDefaultPool: true,
	}, nil
}

type SelectedAccount struct {
	Account store.Account
	PoolID  int64
}

func (rt *Router) SelectAccount(poolID int64, targetModel string, exclude []int64) (*SelectedAccount, error) {
	pool := rt.cache.GetPool(poolID)
	if pool == nil {
		return nil, ErrPoolDisabled
	}
	if !pool.Enabled {
		return nil, ErrPoolDisabled
	}

	snap := rt.cache.Get()

	// build account lookup
	accountMap := make(map[int64]*store.Account, len(snap.Accounts))
	for i := range snap.Accounts {
		accountMap[snap.Accounts[i].ID] = &snap.Accounts[i]
	}

	excludeSet := make(map[int64]bool, len(exclude))
	for _, id := range exclude {
		excludeSet[id] = true
	}

	// group members by priority
	type priorityGroup struct {
		priority int
		members  []store.PoolMember
	}
	groups := make(map[int]*priorityGroup)
	var priorities []int
	for _, m := range pool.Members {
		if _, exists := groups[m.Priority]; !exists {
			groups[m.Priority] = &priorityGroup{priority: m.Priority}
			priorities = append(priorities, m.Priority)
		}
		groups[m.Priority].members = append(groups[m.Priority].members, m)
	}

	// sort priorities ascending
	for i := 0; i < len(priorities)-1; i++ {
		for j := i + 1; j < len(priorities); j++ {
			if priorities[j] < priorities[i] {
				priorities[i], priorities[j] = priorities[j], priorities[i]
			}
		}
	}

	// try each priority level
	for _, p := range priorities {
		group := groups[p]
		var candidates []candidateAccount
		for _, m := range group.members {
			if excludeSet[m.AccountID] {
				continue
			}
			acc := accountMap[m.AccountID]
			if acc == nil || !acc.Enabled {
				continue
			}
			if acc.Status != "healthy" && acc.Status != "degraded" {
				continue
			}
			if !modelAllowed(acc.ModelAllowlist, targetModel) {
				continue
			}
			candidates = append(candidates, candidateAccount{
				account: *acc,
				weight:  m.Weight,
			})
		}

		if len(candidates) > 0 {
			selected := weightedRandom(candidates)
			return &SelectedAccount{
				Account: selected,
				PoolID:  poolID,
			}, nil
		}
	}

	return nil, ErrNoHealthyAccount
}

type candidateAccount struct {
	account store.Account
	weight  int
}

func weightedRandom(candidates []candidateAccount) store.Account {
	if len(candidates) == 1 {
		return candidates[0].account
	}

	totalWeight := 0
	for _, c := range candidates {
		totalWeight += c.weight
	}

	r := rand.IntN(totalWeight)
	for _, c := range candidates {
		r -= c.weight
		if r < 0 {
			return c.account
		}
	}
	return candidates[len(candidates)-1].account
}

func modelAllowed(allowlist []string, model string) bool {
	if len(allowlist) == 0 {
		return true
	}
	for _, m := range allowlist {
		if m == model {
			return true
		}
	}
	return false
}

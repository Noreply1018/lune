package router

import (
	"encoding/json"
	"errors"

	"lune/internal/store"
)

var (
	ErrNoRoute           = errors.New("no_route")
	ErrPoolDisabled      = errors.New("pool_disabled")
	ErrNoHealthyAccount  = errors.New("no_healthy_account")
	ErrModelNotOnAccount = errors.New("model_not_on_account")
	ErrRuntimeBinding    = errors.New("runtime_auth_binding_unavailable")
)

type Router struct {
	cache   *store.RoutingCache
	options Options
}

type Options struct {
	CpaRuntimeBindingSupported bool
}

type ResolveOptions struct {
	Diagnostic bool
}

func New(cache *store.RoutingCache) *Router {
	return &Router{cache: cache}
}

func NewWithOptions(cache *store.RoutingCache, options Options) *Router {
	return &Router{cache: cache, options: options}
}

type ResolvedRoute struct {
	PoolID      int64
	TargetModel string
	AccountID   int64
	Account     store.Account
}

type CandidateTrace struct {
	AccountID int64  `json:"account_id"`
	Position  int    `json:"position"`
	Outcome   string `json:"outcome"`
	Reason    string `json:"reason,omitempty"`
}

func (c CandidateTrace) MarshalJSON() ([]byte, error) {
	type candidateTrace struct {
		AccountID  int64  `json:"account_id"`
		Position   int    `json:"position"`
		Outcome    string `json:"outcome"`
		Reason     string `json:"reason,omitempty"`
		SkipReason string `json:"skip_reason,omitempty"`
	}
	return json.Marshal(candidateTrace{
		AccountID:  c.AccountID,
		Position:   c.Position,
		Outcome:    c.Outcome,
		Reason:     c.Reason,
		SkipReason: c.Reason,
	})
}

// Resolve finds the best account for the given model within the token's Pool.
// forceAccountID bypasses normal member ordering but must still belong to the
// token Pool.
func (rt *Router) Resolve(model string, tokenPoolID *int64, forceAccountID *int64) (*ResolvedRoute, error) {
	return rt.ResolveWithOptions(model, tokenPoolID, forceAccountID, ResolveOptions{})
}

func (rt *Router) ResolveWithOptions(model string, tokenPoolID *int64, forceAccountID *int64, opts ResolveOptions) (*ResolvedRoute, error) {
	snap := rt.cache.Get()
	if tokenPoolID == nil {
		return nil, ErrNoRoute
	}

	// Force-route to a specific account (for inline testing)
	if forceAccountID != nil {
		return rt.resolveToAccount(snap, model, *tokenPoolID, *forceAccountID, opts)
	}

	return rt.resolveInPool(snap, model, *tokenPoolID, opts)
}

// SelectNextAccount finds the next available account for retry, excluding already-tried accounts.
func (rt *Router) SelectNextAccount(model string, tokenPoolID *int64, exclude []int64) (*ResolvedRoute, error) {
	if tokenPoolID == nil {
		return nil, ErrNoRoute
	}
	return rt.resolveInPool(rt.cache.Get(), model, *tokenPoolID, ResolveOptions{}, exclude...)
}

func (rt *Router) ExplainCandidates(model string, poolID int64, exclude []int64, opts ResolveOptions) []CandidateTrace {
	snap := rt.cache.Get()
	pool, ok := snap.Pools[poolID]
	if !ok || !pool.Enabled {
		return nil
	}
	members := snap.Members[poolID]
	excludeSet := makeExcludeSet(exclude)
	traces := make([]CandidateTrace, 0, len(members))
	for _, m := range members {
		trace := CandidateTrace{AccountID: m.AccountID, Position: m.Position, Outcome: "skipped"}
		if !m.Enabled {
			trace.Reason = "member_disabled"
			traces = append(traces, trace)
			continue
		}
		if excludeSet[m.AccountID] {
			trace.Reason = "already_attempted"
			traces = append(traces, trace)
			continue
		}
		acc, ok := snap.Accounts[m.AccountID]
		if !ok {
			trace.Reason = "missing_account"
			traces = append(traces, trace)
			continue
		}
		decision := rt.accountDecision(acc, opts)
		if !decision.Routable {
			trace.Reason = decision.Reason
			traces = append(traces, trace)
			continue
		}
		switch accountModelMatch(snap, m.AccountID, model) {
		case modelExplicitMatch:
			trace.Outcome = "candidate"
			trace.Reason = "model_explicit_match"
		case modelUnknown:
			trace.Outcome = "candidate"
			trace.Reason = "model_unknown"
		default:
			trace.Reason = "model_explicit_mismatch"
		}
		traces = append(traces, trace)
	}
	return traces
}

func (rt *Router) resolveToAccount(snap *store.CacheSnapshot, model string, poolID, accountID int64, opts ResolveOptions) (*ResolvedRoute, error) {
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
	if !rt.accountRoutable(acc, opts) {
		if rt.accountBlockedByRuntimeBinding(acc) {
			return nil, ErrRuntimeBinding
		}
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

func (rt *Router) resolveInPool(snap *store.CacheSnapshot, model string, poolID int64, opts ResolveOptions, exclude ...int64) (*ResolvedRoute, error) {
	pool, ok := snap.Pools[poolID]
	if !ok || !pool.Enabled {
		return nil, ErrPoolDisabled
	}

	members, ok := snap.Members[poolID]
	if !ok || len(members) == 0 {
		return nil, ErrNoHealthyAccount
	}

	excludeSet := makeExcludeSet(exclude)
	runtimeBindingBlocked := false
	if store.NormalizeRoutingPolicy(pool.RoutingPolicy) == "ordered" {
		resolved, blocked := rt.pickOrdered(snap, members, model, poolID, excludeSet, opts)
		if blocked {
			runtimeBindingBlocked = true
		}
		if resolved != nil {
			return resolved, nil
		}
		if runtimeBindingBlocked {
			return nil, ErrRuntimeBinding
		}
		return nil, ErrNoHealthyAccount
	}

	for _, stage := range []struct {
		requireModel bool
		maxPenalty   int
	}{
		{requireModel: true, maxPenalty: 0},
		{requireModel: true, maxPenalty: 1},
		{requireModel: true, maxPenalty: 999},
		{requireModel: false, maxPenalty: 0},
		{requireModel: false, maxPenalty: 1},
		{requireModel: false, maxPenalty: 999},
	} {
		resolved, blocked := rt.pickFromMembers(snap, members, model, poolID, excludeSet, stage.requireModel, stage.maxPenalty, opts)
		runtimeBindingBlocked = runtimeBindingBlocked || blocked
		if resolved != nil {
			return resolved, nil
		}
	}

	if runtimeBindingBlocked {
		return nil, ErrRuntimeBinding
	}
	return nil, ErrNoHealthyAccount
}

func (rt *Router) pickOrdered(snap *store.CacheSnapshot, members []*store.PoolMember, model string, poolID int64, excludeSet map[int64]bool, opts ResolveOptions) (*ResolvedRoute, bool) {
	runtimeBindingBlocked := false
	for _, requireExplicitModel := range []bool{true, false} {
		for _, m := range members {
			if !m.Enabled || excludeSet[m.AccountID] {
				continue
			}
			acc, ok := snap.Accounts[m.AccountID]
			if !ok || !rt.accountRoutable(acc, opts) {
				if ok && rt.accountBlockedByRuntimeBinding(acc) {
					runtimeBindingBlocked = true
				}
				continue
			}
			match := accountModelMatch(snap, m.AccountID, model)
			if requireExplicitModel {
				if match != modelExplicitMatch {
					continue
				}
			} else if match != modelUnknown {
				continue
			}
			return &ResolvedRoute{
				PoolID:      poolID,
				TargetModel: model,
				AccountID:   m.AccountID,
				Account:     *acc,
			}, runtimeBindingBlocked
		}
	}
	return nil, runtimeBindingBlocked
}

func (rt *Router) pickFromMembers(snap *store.CacheSnapshot, members []*store.PoolMember, model string, poolID int64, excludeSet map[int64]bool, requireModel bool, maxPenalty int, opts ResolveOptions) (*ResolvedRoute, bool) {
	runtimeBindingBlocked := false
	for _, m := range members {
		if !m.Enabled || excludeSet[m.AccountID] {
			continue
		}
		acc, ok := snap.Accounts[m.AccountID]
		if !ok || !rt.accountRoutable(acc, opts) {
			if ok && rt.accountBlockedByRuntimeBinding(acc) {
				runtimeBindingBlocked = true
			}
			continue
		}
		if accountRoutePenalty(acc) > maxPenalty {
			continue
		}
		match := accountModelMatch(snap, m.AccountID, model)
		if requireModel && match != modelExplicitMatch {
			continue
		}
		if !requireModel && match != modelUnknown {
			continue
		}
		return &ResolvedRoute{
			PoolID:      poolID,
			TargetModel: model,
			AccountID:   m.AccountID,
			Account:     *acc,
		}, runtimeBindingBlocked
	}
	return nil, runtimeBindingBlocked
}

func (rt *Router) accountRoutable(acc *store.Account, opts ResolveOptions) bool {
	return rt.accountDecision(acc, opts).Routable
}

func (rt *Router) accountBlockedByRuntimeBinding(acc *store.Account) bool {
	return rt.accountDecision(acc, ResolveOptions{}).RuntimeBindingBlocked
}

func (rt *Router) accountDecision(acc *store.Account, opts ResolveOptions) store.RoutabilityDecision {
	return store.EvaluateAccountRoutability(acc, store.RoutabilityOptions{
		Diagnostic:                 opts.Diagnostic,
		CpaRuntimeBindingSupported: rt.options.CpaRuntimeBindingSupported,
	})
}

func accountRoutePenalty(acc *store.Account) int {
	if acc == nil {
		return 0
	}
	penalty := 0
	if acc.SourceKind == "cpa" {
		if acc.CpaCredentialStatus == "auth_suspect" {
			penalty++
		}
		switch acc.CpaQuotaStatus {
		case "error", "unknown", "pending":
			penalty++
		}
	}
	return penalty
}

type modelMatch int

const (
	modelExplicitMatch modelMatch = iota
	modelExplicitMismatch
	modelUnknown
)

func accountModelMatch(snap *store.CacheSnapshot, accountID int64, model string) modelMatch {
	if snap == nil || !accountHasAnyModel(snap, accountID) {
		return modelUnknown
	}
	for _, id := range snap.ModelIndex[model] {
		if id == accountID {
			return modelExplicitMatch
		}
	}
	return modelExplicitMismatch
}

func accountHasAnyModel(snap *store.CacheSnapshot, accountID int64) bool {
	for _, accountIDs := range snap.ModelIndex {
		for _, id := range accountIDs {
			if id == accountID {
				return true
			}
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

package store

import (
	"sort"
	"sync"
	"sync/atomic"
)

type CacheSnapshot struct {
	Pools       map[int64]*Pool
	Accounts    map[int64]*Account
	Members     map[int64][]*PoolMember // pool_id → members (sorted by position)
	ModelIndex  map[string][]int64      // model_id → account_ids
	Tokens      []AccessToken
	Settings    map[string]string
	CpaServices map[int64]*CpaService
}

type RoutingCache struct {
	mu       sync.RWMutex
	version  uint64
	dbVer    atomic.Uint64
	snapshot *CacheSnapshot
	store    *Store
}

func NewRoutingCache(store *Store) *RoutingCache {
	c := &RoutingCache{store: store}
	c.dbVer.Store(1) // force initial load
	return c
}

func (c *RoutingCache) Invalidate() {
	c.dbVer.Add(1)
}

func (c *RoutingCache) Get() *CacheSnapshot {
	dbv := c.dbVer.Load()

	c.mu.RLock()
	if c.version == dbv && c.snapshot != nil {
		snap := c.snapshot
		c.mu.RUnlock()
		return snap
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	// double-check after acquiring write lock
	dbv = c.dbVer.Load()
	if c.version == dbv && c.snapshot != nil {
		return c.snapshot
	}

	snap := c.loadFromDB()
	c.snapshot = snap
	c.version = dbv
	return snap
}

func (c *RoutingCache) loadFromDB() *CacheSnapshot {
	snap := &CacheSnapshot{
		Pools:       make(map[int64]*Pool),
		Accounts:    make(map[int64]*Account),
		Members:     make(map[int64][]*PoolMember),
		ModelIndex:  make(map[string][]int64),
		Settings:    make(map[string]string),
		CpaServices: make(map[int64]*CpaService),
	}

	// Load accounts
	if accs, err := c.store.ListAccounts(); err == nil {
		for i := range accs {
			acc := accs[i]
			snap.Accounts[acc.ID] = &acc
		}
	}

	// Load pools
	if pools, err := c.store.ListPools(); err == nil {
		for i := range pools {
			p := pools[i]
			snap.Pools[p.ID] = &p
		}
	}

	// Load pool_members with JOINed account data
	c.loadMembers(snap)

	// Load account_models → build ModelIndex
	c.loadModelIndex(snap)

	// Load tokens
	if tokens, err := c.store.ListTokens(); err == nil {
		snap.Tokens = tokens
	}

	// Load settings
	if settings, err := c.store.GetSettings(); err == nil {
		snap.Settings = settings
	}

	// Load cpa_services
	if svcs, err := c.store.ListCpaServices(); err == nil {
		for i := range svcs {
			svc := svcs[i]
			snap.CpaServices[svc.ID] = &svc
		}
	}

	return snap
}

func (c *RoutingCache) loadMembers(snap *CacheSnapshot) {
	rows, err := c.store.db.Query(
		`SELECT pm.id, pm.pool_id, pm.account_id, pm.position, pm.enabled
		 FROM pool_members pm
		 ORDER BY pm.pool_id, pm.position, pm.id`,
	)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var m PoolMember
		var enabled int
		if err := rows.Scan(&m.ID, &m.PoolID, &m.AccountID, &m.Position, &enabled); err != nil {
			continue
		}
		m.Enabled = enabled != 0

		// Attach the account reference from snap.Accounts
		if acc, ok := snap.Accounts[m.AccountID]; ok {
			m.Account = acc
		}

		member := m // copy for pointer stability
		snap.Members[m.PoolID] = append(snap.Members[m.PoolID], &member)
	}

	// Ensure sorted by position within each pool
	for _, members := range snap.Members {
		sort.Slice(members, func(i, j int) bool {
			if members[i].Position != members[j].Position {
				return members[i].Position < members[j].Position
			}
			return members[i].ID < members[j].ID
		})
	}
}

func (c *RoutingCache) loadModelIndex(snap *CacheSnapshot) {
	rows, err := c.store.db.Query(`SELECT account_id, model_id FROM account_models`)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var accountID int64
		var modelID string
		if err := rows.Scan(&accountID, &modelID); err != nil {
			continue
		}
		snap.ModelIndex[modelID] = append(snap.ModelIndex[modelID], accountID)
	}
}

func (c *RoutingCache) FindAccessToken(tokenValue string) *AccessToken {
	snap := c.Get()
	for i := range snap.Tokens {
		if snap.Tokens[i].Token == tokenValue {
			t := snap.Tokens[i]
			return &t
		}
	}
	return nil
}

func (c *RoutingCache) GetPool(id int64) *Pool {
	snap := c.Get()
	if p, ok := snap.Pools[id]; ok {
		cp := *p
		return &cp
	}
	return nil
}

func (c *RoutingCache) GetAccounts() []Account {
	snap := c.Get()
	accs := make([]Account, 0, len(snap.Accounts))
	for _, acc := range snap.Accounts {
		accs = append(accs, *acc)
	}
	return accs
}

func (c *RoutingCache) GetSetting(key string) string {
	snap := c.Get()
	return snap.Settings[key]
}

func (c *RoutingCache) GetAllModels() []string {
	snap := c.Get()
	models := make([]string, 0, len(snap.ModelIndex))
	for model := range snap.ModelIndex {
		models = append(models, model)
	}
	sort.Strings(models)
	return models
}

func (c *RoutingCache) GetCpaService(id int64) *CpaService {
	snap := c.Get()
	if svc, ok := snap.CpaServices[id]; ok {
		s := *svc
		return &s
	}
	return nil
}

func (c *RoutingCache) GetCpaServiceSingle() *CpaService {
	snap := c.Get()
	for _, svc := range snap.CpaServices {
		s := *svc
		return &s
	}
	return nil
}

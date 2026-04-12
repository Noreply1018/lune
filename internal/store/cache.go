package store

import (
	"sync"
	"sync/atomic"
)

type CacheSnapshot struct {
	Accounts []Account
	Pools    []Pool
	Routes   []ModelRoute
	Tokens   []AccessToken
	Settings map[string]string
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
		Settings: make(map[string]string),
	}

	if accs, err := c.store.ListAccounts(); err == nil {
		snap.Accounts = accs
	}
	if pools, err := c.store.ListPools(); err == nil {
		snap.Pools = pools
	}
	if routes, err := c.store.ListRoutes(); err == nil {
		snap.Routes = routes
	}
	if tokens, err := c.store.ListTokens(); err == nil {
		snap.Tokens = tokens
	}
	if settings, err := c.store.GetSettings(); err == nil {
		snap.Settings = settings
	}

	return snap
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

func (c *RoutingCache) FindRoute(modelAlias string) *ModelRoute {
	snap := c.Get()
	for i := range snap.Routes {
		if snap.Routes[i].Alias == modelAlias && snap.Routes[i].Enabled {
			r := snap.Routes[i]
			return &r
		}
	}
	return nil
}

func (c *RoutingCache) GetDefaultPoolID() int64 {
	snap := c.Get()
	v, ok := snap.Settings["default_pool_id"]
	if !ok || v == "" {
		return 0
	}
	var id int64
	for _, ch := range v {
		if ch < '0' || ch > '9' {
			return 0
		}
		id = id*10 + int64(ch-'0')
	}
	return id
}

func (c *RoutingCache) GetPool(id int64) *Pool {
	snap := c.Get()
	for i := range snap.Pools {
		if snap.Pools[i].ID == id {
			p := snap.Pools[i]
			return &p
		}
	}
	return nil
}

func (c *RoutingCache) GetAccounts() []Account {
	return c.Get().Accounts
}

func (c *RoutingCache) GetSetting(key string) string {
	snap := c.Get()
	return snap.Settings[key]
}

func (c *RoutingCache) GetEnabledModelAliases() []string {
	snap := c.Get()
	var aliases []string
	for _, r := range snap.Routes {
		if r.Enabled {
			aliases = append(aliases, r.Alias)
		}
	}
	return aliases
}

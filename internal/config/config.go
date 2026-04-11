package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

const defaultPath = "configs/config.json"

type Config struct {
	Server       ServerConfig  `json:"server"`
	Auth         AuthConfig    `json:"auth"`
	Platforms    []Platform    `json:"platforms"`
	Accounts     []Account     `json:"accounts"`
	AccountPools []AccountPool `json:"account_pools"`
	Models       []ModelRoute  `json:"models"`
}

type ServerConfig struct {
	Port                    int    `json:"port"`
	StreamSmoothing         bool   `json:"stream_smoothing"`
	StreamHeartbeat         bool   `json:"stream_heartbeat"`
	RequestTimeoutS         int    `json:"request_timeout_seconds"`
	ShutdownTimeoutS        int    `json:"shutdown_timeout_seconds"`
	DataDir                 string `json:"data_dir"`
	PlatformRefreshInterval int    `json:"platform_refresh_interval_seconds"`
	UpstreamURL             string `json:"backend_url"`
}

type AuthConfig struct {
	AdminToken   string        `json:"admin_token"`
	AccessTokens []AccessToken `json:"access_tokens"`
}

type AccessToken struct {
	Name           string `json:"name"`
	Token          string `json:"token"`
	Enabled        bool   `json:"enabled"`
	QuotaCalls     int64  `json:"quota_calls"`
	CostPerRequest int64  `json:"cost_per_request"`
}

type Platform struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Adapter  string `json:"adapter"`
	Enabled  bool   `json:"enabled"`
	Priority int    `json:"priority"`
	Weight   int    `json:"weight"`
	TimeoutS int    `json:"timeout_seconds"`
}

type Account struct {
	ID             string  `json:"id"`
	Platform       string  `json:"platform"`
	Label          string  `json:"label"`
	CredentialType string  `json:"credential_type"`
	Credential     string  `json:"credential,omitempty"`
	CredentialEnv  string  `json:"credential_env,omitempty"`
	EgressProxyEnv string  `json:"egress_proxy_env,omitempty"`
	PlanType       string  `json:"plan_type"`
	Enabled        bool    `json:"enabled"`
	Status         string  `json:"status"`
	RiskScore      float64 `json:"risk_score"`
}

type AccountPool struct {
	ID       string   `json:"id"`
	Platform string   `json:"platform"`
	Strategy string   `json:"strategy"`
	Enabled  bool     `json:"enabled"`
	Members  []string `json:"members"`
}

type ModelRoute struct {
	Alias       string   `json:"alias"`
	AccountPool string   `json:"account_pool"`
	TargetKind  string   `json:"target_kind"`
	TargetID    string   `json:"target_id"`
	TargetModel string   `json:"target_model"`
	Fallbacks   []string `json:"fallbacks"`
}

func PathFromEnv() string {
	if path := os.Getenv("LUNE_CONFIG"); path != "" {
		return path
	}
	return defaultPath
}

func Load(path string) (Config, error) {
	var cfg Config

	raw, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}

	if err := json.Unmarshal(raw, &cfg); err != nil {
		return cfg, err
	}

	return Prepare(cfg)
}

// LoadOrDefault loads config from path. If the file does not exist,
// returns a zero Config with fresh=true so the caller can bootstrap.
func LoadOrDefault(path string) (cfg Config, fresh bool, err error) {
	raw, readErr := os.ReadFile(path)
	if readErr != nil {
		if os.IsNotExist(readErr) {
			cfg.applyDefaults()
			return cfg, true, nil
		}
		return cfg, false, readErr
	}

	if err := json.Unmarshal(raw, &cfg); err != nil {
		return cfg, false, err
	}

	prepared, err := PrepareBoot(cfg)
	return prepared, false, err
}

func Prepare(cfg Config) (Config, error) {
	cfg.normalize()

	if err := cfg.Validate(); err != nil {
		return cfg, err
	}

	cfg.applyDefaults()
	return cfg, nil
}

// PrepareBoot applies lenient validation suitable for startup.
// It allows empty accounts/pools/tokens so the app can boot in setup mode.
func PrepareBoot(cfg Config) (Config, error) {
	cfg.normalize()

	if err := cfg.ValidateBoot(); err != nil {
		return cfg, err
	}

	cfg.applyDefaults()
	return cfg, nil
}

// GenerateAdminToken creates a random admin token with a "lune-" prefix.
func GenerateAdminToken() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return "lune-" + hex.EncodeToString(b)
}

func Marshal(cfg Config) ([]byte, error) {
	prepared, err := Prepare(cfg)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(prepared, "", "  ")
}

// NeedsBootstrap returns true when the config lacks essential runtime entities.
func (c Config) NeedsBootstrap() bool {
	return len(c.Accounts) == 0 || len(c.AccountPools) == 0 || len(c.Auth.AccessTokens) == 0
}

func (c *Config) normalize() {
	for i := range c.Models {
		switch {
		case strings.TrimSpace(c.Models[i].TargetKind) != "" && strings.TrimSpace(c.Models[i].TargetID) != "":
		case strings.TrimSpace(c.Models[i].AccountPool) != "":
			c.Models[i].TargetKind = "account_pool"
			c.Models[i].TargetID = strings.TrimSpace(c.Models[i].AccountPool)
		case strings.TrimSpace(c.Models[i].TargetID) != "":
			c.Models[i].TargetKind = "account_pool"
		}

		if c.Models[i].AccountPool == "" && c.Models[i].TargetKind == "account_pool" {
			c.Models[i].AccountPool = c.Models[i].TargetID
		}
	}
}

func (c *Config) applyDefaults() {
	if c.Server.Port == 0 {
		c.Server.Port = 7788
	}
	if c.Server.RequestTimeoutS == 0 {
		c.Server.RequestTimeoutS = 120
	}
	if c.Server.ShutdownTimeoutS == 0 {
		c.Server.ShutdownTimeoutS = 10
	}
	if c.Server.DataDir == "" {
		c.Server.DataDir = "data"
	}
	if c.Server.PlatformRefreshInterval == 0 {
		c.Server.PlatformRefreshInterval = 60
	}
	if c.Server.UpstreamURL == "" {
		if envURL := os.Getenv("LUNE_BACKEND_URL"); envURL != "" {
			c.Server.UpstreamURL = envURL
		} else {
			c.Server.UpstreamURL = "http://localhost:3000"
		}
	}
}

// ValidateBoot performs lenient validation for startup — checks structural
// correctness (port range, no duplicate IDs, valid cross-references) but does
// not require minimum counts for platforms/accounts/pools/tokens.
func (c Config) ValidateBoot() error {
	if c.Server.Port < 0 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server.port: %d", c.Server.Port)
	}

	platforms := make(map[string]struct{}, len(c.Platforms))
	for _, p := range c.Platforms {
		if p.ID == "" {
			return fmt.Errorf("platform id is required")
		}
		if _, exists := platforms[p.ID]; exists {
			return fmt.Errorf("duplicate platform id: %s", p.ID)
		}
		platforms[p.ID] = struct{}{}
	}

	accounts := make(map[string]struct{}, len(c.Accounts))
	for _, a := range c.Accounts {
		if a.ID == "" {
			return fmt.Errorf("account id is required")
		}
		if _, exists := accounts[a.ID]; exists {
			return fmt.Errorf("duplicate account id: %s", a.ID)
		}
		accounts[a.ID] = struct{}{}
		if a.Platform != "" {
			if _, exists := platforms[a.Platform]; !exists {
				return fmt.Errorf("account %s references unknown platform %s", a.ID, a.Platform)
			}
		}
	}

	pools := make(map[string]struct{}, len(c.AccountPools))
	for _, p := range c.AccountPools {
		if p.ID == "" {
			return fmt.Errorf("account pool id is required")
		}
		if _, exists := pools[p.ID]; exists {
			return fmt.Errorf("duplicate account pool id: %s", p.ID)
		}
		pools[p.ID] = struct{}{}
		if p.Platform != "" {
			if _, exists := platforms[p.Platform]; !exists {
				return fmt.Errorf("account pool %s references unknown platform %s", p.ID, p.Platform)
			}
		}
		for _, member := range p.Members {
			if _, exists := accounts[member]; !exists {
				return fmt.Errorf("account pool %s references unknown account %s", p.ID, member)
			}
		}
	}

	models := make(map[string]struct{}, len(c.Models))
	for _, m := range c.Models {
		if m.Alias == "" {
			return fmt.Errorf("model alias is required")
		}
		if _, exists := models[m.Alias]; exists {
			return fmt.Errorf("duplicate model alias: %s", m.Alias)
		}
		models[m.Alias] = struct{}{}
	}

	return nil
}

func (c Config) Validate() error {
	if c.Server.Port < 0 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server.port: %d", c.Server.Port)
	}
	if c.Auth.AdminToken == "" {
		return fmt.Errorf("auth.admin_token is required")
	}
	if len(c.Auth.AccessTokens) == 0 {
		return fmt.Errorf("at least one access token is required")
	}

	platforms := make(map[string]struct{}, len(c.Platforms))
	for _, platform := range c.Platforms {
		if platform.ID == "" {
			return fmt.Errorf("platform id is required")
		}
		if _, exists := platforms[platform.ID]; exists {
			return fmt.Errorf("duplicate platform id: %s", platform.ID)
		}
		platforms[platform.ID] = struct{}{}
	}
	if len(platforms) == 0 {
		return fmt.Errorf("at least one platform is required")
	}

	accounts := make(map[string]struct{}, len(c.Accounts))
	for _, account := range c.Accounts {
		if account.ID == "" {
			return fmt.Errorf("account id is required")
		}
		if _, exists := accounts[account.ID]; exists {
			return fmt.Errorf("duplicate account id: %s", account.ID)
		}
		accounts[account.ID] = struct{}{}
		if account.Platform == "" {
			return fmt.Errorf("account %s platform is required", account.ID)
		}
		if _, exists := platforms[account.Platform]; !exists {
			return fmt.Errorf("account %s references unknown platform %s", account.ID, account.Platform)
		}
	}

	accountPools := make(map[string]struct{}, len(c.AccountPools))
	for _, pool := range c.AccountPools {
		if pool.ID == "" {
			return fmt.Errorf("account pool id is required")
		}
		if _, exists := accountPools[pool.ID]; exists {
			return fmt.Errorf("duplicate account pool id: %s", pool.ID)
		}
		accountPools[pool.ID] = struct{}{}
		if pool.Platform == "" {
			return fmt.Errorf("account pool %s platform is required", pool.ID)
		}
		if _, exists := platforms[pool.Platform]; !exists {
			return fmt.Errorf("account pool %s references unknown platform %s", pool.ID, pool.Platform)
		}
		for _, member := range pool.Members {
			if _, exists := accounts[member]; !exists {
				return fmt.Errorf("account pool %s references unknown account %s", pool.ID, member)
			}
		}
	}

	models := make(map[string]struct{}, len(c.Models))
	for _, model := range c.Models {
		if model.Alias == "" {
			return fmt.Errorf("model alias is required")
		}
		if _, exists := models[model.Alias]; exists {
			return fmt.Errorf("duplicate model alias: %s", model.Alias)
		}
		models[model.Alias] = struct{}{}

		targetID := strings.TrimSpace(model.TargetID)
		if targetID == "" {
			targetID = strings.TrimSpace(model.AccountPool)
		}
		switch model.TargetKind {
		case "", "account_pool":
			if targetID == "" {
				return fmt.Errorf("model %s account pool is required", model.Alias)
			}
			if _, exists := accountPools[targetID]; !exists {
				return fmt.Errorf("model %s references unknown account pool %s", model.Alias, targetID)
			}
		default:
			return fmt.Errorf("model %s has unsupported target kind %s", model.Alias, model.TargetKind)
		}
	}

	return nil
}

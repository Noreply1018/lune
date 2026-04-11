package runtimeconfig

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"lune/internal/config"
	"lune/internal/store"
)

type Manager struct {
	path    string
	store   *store.Store
	onApply func(config.Config)

	mu  sync.RWMutex
	cfg config.Config
}

func New(path string, cfg config.Config, st *store.Store, onApply func(config.Config)) *Manager {
	return &Manager{
		path:    path,
		store:   st,
		onApply: onApply,
		cfg:     cfg,
	}
}

func (m *Manager) Current() config.Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfg
}

func (m *Manager) ValidateOnly(candidate config.Config) (config.Config, error) {
	return config.Prepare(candidate)
}

func (m *Manager) Apply(ctx context.Context, candidate config.Config) (config.Config, error) {
	// Use lenient validation during bootstrap, strict once fully configured.
	var prepared config.Config
	var err error
	if candidate.NeedsBootstrap() {
		prepared, err = config.PrepareBoot(candidate)
	} else {
		prepared, err = config.Prepare(candidate)
	}
	if err != nil {
		return config.Config{}, err
	}

	raw, err := json.MarshalIndent(prepared, "", "  ")
	if err != nil {
		return config.Config{}, err
	}

	if err := m.writeAtomic(raw); err != nil {
		return config.Config{}, err
	}

	m.mu.Lock()
	m.cfg = prepared
	m.mu.Unlock()

	if m.store != nil {
		if err := m.store.SyncAccessTokens(ctx, prepared.Auth.AccessTokens); err != nil {
			return config.Config{}, fmt.Errorf("sync access tokens: %w", err)
		}
		if err := m.store.SyncAccounts(ctx, prepared.Accounts); err != nil {
			return config.Config{}, fmt.Errorf("sync accounts: %w", err)
		}
		if err := m.store.SyncAccountPools(ctx, prepared.AccountPools); err != nil {
			return config.Config{}, fmt.Errorf("sync account pools: %w", err)
		}
	}

	if m.onApply != nil {
		m.onApply(prepared)
	}
	return prepared, nil
}

func (m *Manager) writeAtomic(raw []byte) error {
	dir := filepath.Dir(m.path)
	base := filepath.Base(m.path)

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	if existing, err := os.ReadFile(m.path); err == nil {
		backup := filepath.Join(dir, fmt.Sprintf("%s.bak-%s", base, time.Now().UTC().Format("20060102-150405")))
		if err := os.WriteFile(backup, existing, 0o644); err != nil {
			return fmt.Errorf("write backup: %w", err)
		}
	}

	tmp := filepath.Join(dir, fmt.Sprintf(".%s.tmp", base))
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return fmt.Errorf("write temp config: %w", err)
	}
	if err := os.Rename(tmp, m.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace config: %w", err)
	}
	return nil
}

package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"lune/internal/health"
	"lune/internal/httpserver"
	"lune/internal/store"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Port       int    `yaml:"port"`
	DataDir    string `yaml:"data_dir"`
	CpaAuthDir string `yaml:"cpa_auth_dir"`
	CpaBaseURL string `yaml:"cpa_base_url"`
	CpaAPIKey  string `yaml:"cpa_api_key"`
}

func LoadConfig() Config {
	cfg := Config{
		Port:    7788,
		DataDir: "./data",
	}

	// try lune.yaml
	if data, err := os.ReadFile("lune.yaml"); err == nil {
		_ = yaml.Unmarshal(data, &cfg)
	}

	// env vars override
	if v := os.Getenv("LUNE_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Port = p
		}
	}
	if v := os.Getenv("LUNE_DATA_DIR"); v != "" {
		cfg.DataDir = v
	}
	if v := os.Getenv("LUNE_CPA_AUTH_DIR"); v != "" {
		cfg.CpaAuthDir = v
	}
	if v := os.Getenv("LUNE_CPA_BASE_URL"); v != "" {
		cfg.CpaBaseURL = v
	}
	if v := os.Getenv("LUNE_CPA_API_KEY"); v != "" {
		cfg.CpaAPIKey = v
	}

	return cfg
}

type App struct {
	cfg    Config
	logger *log.Logger
	store  *store.Store
	cache  *store.RoutingCache
}

func New(cfg Config) (*App, error) {
	logger := log.New(os.Stdout, "", log.LstdFlags)

	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	dbPath := filepath.Join(cfg.DataDir, "lune.db")
	st, err := store.New(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	cache := store.NewRoutingCache(st)

	return &App{
		cfg:    cfg,
		logger: logger,
		store:  st,
		cache:  cache,
	}, nil
}

func (a *App) Run() error {
	// resolve admin token
	adminToken, tokenSource := a.resolveAdminToken()

	// auto-configure default CPA service if env vars are set
	a.ensureDefaultCpa()

	srv := httpserver.New(a.logger, a.store, a.cache, a.cfg.CpaAuthDir)

	// Prefer an explicit IPv4 listener so WSL localhost forwarding can
	// consistently expose the service to Windows browsers.
	addr := fmt.Sprintf("0.0.0.0:%d", a.cfg.Port)
	ln, err := net.Listen("tcp4", addr)
	if err != nil {
		return fmt.Errorf("port %d is already in use", a.cfg.Port)
	}

	httpServer := &http.Server{Handler: srv.Handler()}

	// print startup banner
	fmt.Printf("\nLune is running\n\n")
	fmt.Printf("  Admin UI:    http://127.0.0.1:%d/admin\n", a.cfg.Port)
	fmt.Printf("  Gateway API: http://127.0.0.1:%d/v1\n", a.cfg.Port)
	fmt.Printf("  Admin Token: %s (%s)\n", adminToken, tokenSource)
	fmt.Printf("\nPress Ctrl+C to stop\n\n")

	// graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// start health checker
	hc := health.NewChecker(a.store, a.cache, a.logger, a.cfg.CpaAuthDir)
	go hc.Run(ctx)

	errCh := make(chan error, 1)
	go func() {
		errCh <- httpServer.Serve(ln)
	}()

	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			return err
		}
	case <-ctx.Done():
		a.logger.Println("shutting down...")
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		httpServer.Shutdown(shutCtx)
	}

	a.store.Close()
	return nil
}

func (a *App) resolveAdminToken() (token, source string) {
	// 1. env var
	if v := os.Getenv("LUNE_ADMIN_TOKEN"); v != "" {
		_ = a.store.SetSetting("admin_token", v)
		a.cache.Invalidate()
		return v, "from env"
	}

	// 2. existing DB value
	if v, err := a.store.GetSetting("admin_token"); err == nil && v != "" {
		return v, "from database"
	}

	// 3. auto-generate
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	token = "lune-" + hex.EncodeToString(b)
	_ = a.store.SetSetting("admin_token", token)
	a.cache.Invalidate()
	return token, "auto-generated"
}

func (a *App) ensureDefaultCpa() {
	if a.cfg.CpaBaseURL == "" {
		return
	}
	existing, _ := a.store.GetCpaService()
	if existing != nil {
		return
	}
	svc := &store.CpaService{
		Label:   "Default CPA",
		BaseURL: a.cfg.CpaBaseURL,
		APIKey:  a.cfg.CpaAPIKey,
		Enabled: true,
	}
	if _, err := a.store.CreateCpaService(svc); err != nil {
		a.logger.Printf("auto-configure CPA: %v", err)
		return
	}
	a.cache.Invalidate()
	a.logger.Printf("CPA service auto-configured: %s", a.cfg.CpaBaseURL)
}

func Check(cfg Config) error {
	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		return fmt.Errorf("data dir not writable: %w", err)
	}

	dbPath := filepath.Join(cfg.DataDir, "lune.db")
	st, err := store.New(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer st.Close()

	schemaVer := st.SchemaVersion()
	totalAccounts, byStatus, _ := st.CountAccounts()
	totalPools, _ := st.CountPools()
	totalRoutes, _ := st.CountRoutes()
	totalTokens, _ := st.CountTokens()

	fmt.Printf("Lune check passed\n\n")
	fmt.Printf("  Database: %s (schema v%d)\n", dbPath, schemaVer)
	fmt.Printf("  Accounts: %d", totalAccounts)
	if len(byStatus) > 0 {
		fmt.Printf(" (")
		first := true
		for status, count := range byStatus {
			if !first {
				fmt.Printf(", ")
			}
			fmt.Printf("%d %s", count, status)
			first = false
		}
		fmt.Printf(")")
	}
	fmt.Println()
	fmt.Printf("  Pools:    %d\n", totalPools)
	fmt.Printf("  Routes:   %d\n", totalRoutes)
	fmt.Printf("  Tokens:   %d\n", totalTokens)

	return nil
}

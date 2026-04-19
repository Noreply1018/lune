package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"lune/internal/health"
	"lune/internal/httpserver"
	"lune/internal/notify"
	"lune/internal/notify/drivers"
	"lune/internal/store"
)

type LoggingConfig struct {
	Format string `yaml:"format"` // json | text
	Level  string `yaml:"level"`  // debug | info | warn | error
}

type Config struct {
	Port             int           `yaml:"port"`
	DataDir          string        `yaml:"data_dir"`
	CpaAuthDir       string        `yaml:"cpa_auth_dir"`
	CpaBaseURL       string        `yaml:"cpa_base_url"`
	CpaAPIKey        string        `yaml:"cpa_api_key"`
	CpaManagementKey string        `yaml:"cpa_management_key"`
	Logging          LoggingConfig `yaml:"logging"`
}

func (cfg Config) Validate() error {
	var errs []string
	if cfg.Port < 1 || cfg.Port > 65535 {
		errs = append(errs, fmt.Sprintf("invalid port: %d", cfg.Port))
	}
	if cfg.DataDir == "" {
		errs = append(errs, "data_dir is required")
	}
	if len(errs) > 0 {
		return fmt.Errorf("config validation failed:\n  %s", strings.Join(errs, "\n  "))
	}
	return nil
}

func LoadConfig() Config {
	cfg := Config{
		Port:    7788,
		DataDir: "./data",
	}

	// env vars override defaults
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
	if v := os.Getenv("LUNE_CPA_MANAGEMENT_KEY"); v != "" {
		cfg.CpaManagementKey = v
	}
	if v := os.Getenv("LUNE_LOG_LEVEL"); v != "" {
		cfg.Logging.Level = v
	}
	if v := os.Getenv("LUNE_LOG_FORMAT"); v != "" {
		cfg.Logging.Format = v
	}

	return cfg
}

func initSlog(cfg LoggingConfig) {
	var level slog.Level
	switch strings.ToLower(cfg.Level) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	if strings.ToLower(cfg.Format) == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}
	slog.SetDefault(slog.New(handler))
}

type App struct {
	cfg   Config
	store *store.Store
	cache *store.RoutingCache
}

func New(cfg Config) (*App, error) {
	initSlog(cfg.Logging)

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	if cfg.CpaAuthDir != "" {
		if err := os.MkdirAll(cfg.CpaAuthDir, 0755); err != nil {
			return nil, fmt.Errorf("create cpa auth dir: %w", err)
		}
	}

	dbPath := filepath.Join(cfg.DataDir, "lune.db")
	st, err := store.New(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	cache := store.NewRoutingCache(st)

	// ensure a default global token exists
	if tok, err := st.EnsureDefaultGlobalToken(); err != nil {
		slog.Error("ensure default global token", "err", err)
	} else if tok != nil {
		slog.Info("default global token created", "token", tok.TokenMasked)
	}

	return &App{
		cfg:   cfg,
		store: st,
		cache: cache,
	}, nil
}

func (a *App) Run() error {
	// resolve admin token
	adminToken, tokenSource := a.resolveAdminToken()

	// auto-configure default CPA service if env vars are set
	a.ensureDefaultCpa()

	registry := notify.NewRegistry(
		drivers.NewWeChatWorkBotDriver(),
	)
	notifier := notify.NewServiceWithRegistry(a.store, registry)

	// create health checker (needed by admin handler for model discovery)
	hc := health.NewChecker(a.store, a.cache, a.cfg.CpaAuthDir, a.cfg.CpaManagementKey, notifier)

	srv := httpserver.New(a.store, a.cache, a.cfg.CpaAuthDir, a.cfg.CpaManagementKey, hc, notifier)

	// Prefer an explicit IPv4 listener so WSL localhost forwarding can
	// consistently expose the service to Windows browsers.
	addr := fmt.Sprintf("0.0.0.0:%d", a.cfg.Port)
	ln, err := net.Listen("tcp4", addr)
	if err != nil {
		return fmt.Errorf("port %d is already in use", a.cfg.Port)
	}

	httpServer := &http.Server{Handler: srv.Handler()}

	// print startup banner
	maskedAdmin := maskToken(adminToken)
	fmt.Printf("\nLune is running\n\n")
	fmt.Printf("  Admin UI:    http://127.0.0.1:%d/admin\n", a.cfg.Port)
	fmt.Printf("  Gateway API: http://127.0.0.1:%d/v1\n", a.cfg.Port)
	fmt.Printf("  Admin Token: %s (%s)\n", maskedAdmin, tokenSource)
	fmt.Printf("\nPress Ctrl+C to stop\n\n")

	// graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// start health checker
	go hc.Run(ctx)
	go notifier.Run(ctx)

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
		slog.Info("shutting down...")
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
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Errorf("generate admin token: %w", err))
	}
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
		slog.Error("auto-configure CPA", "err", err)
		return
	}
	a.cache.Invalidate()
	slog.Info("CPA service auto-configured", "base_url", a.cfg.CpaBaseURL)
}

func maskToken(token string) string {
	if len(token) > 12 {
		return token[:8] + "..." + token[len(token)-4:]
	}
	return token
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
	fmt.Printf("  Tokens:   %d\n", totalTokens)

	return nil
}

package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"lune/internal/config"
	"lune/internal/httpserver"
	"lune/internal/metrics"
	"lune/internal/platform"
	"lune/internal/runtimeconfig"
	"lune/internal/store"
)

func Run(logger *log.Logger) error {
	cfgPath := config.PathFromEnv()

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if err := os.MkdirAll(cfg.Server.DataDir, 0o755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	logStore, err := store.Open(filepath.Join(cfg.Server.DataDir, "lune.db"))
	if err != nil {
		return fmt.Errorf("open sqlite store: %w", err)
	}
	defer logStore.Close()

	if err := logStore.SyncAccessTokens(context.Background(), cfg.Auth.AccessTokens); err != nil {
		return fmt.Errorf("sync access tokens: %w", err)
	}
	if err := logStore.SyncAccounts(context.Background(), cfg.Accounts); err != nil {
		return fmt.Errorf("sync accounts: %w", err)
	}
	if err := logStore.SyncAccountPools(context.Background(), cfg.AccountPools); err != nil {
		return fmt.Errorf("sync account pools: %w", err)
	}

	metricCollector := metrics.New()
	var cfgManager *runtimeconfig.Manager
	platformRegistry := platform.New(func() config.Config {
		if cfgManager == nil {
			return cfg
		}
		return cfgManager.Current()
	})
	platformRegistry.Start(context.Background(), time.Duration(cfg.Server.PlatformRefreshInterval)*time.Second)

	cfgManager = runtimeconfig.New(cfgPath, cfg, logStore, func(applied config.Config) {
		platformRegistry.CheckAll(context.Background())
	})

	server := httpserver.New(cfgManager, logger, logStore, metricCollector, platformRegistry)

	httpServer := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:           server.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)

	go func() {
		logger.Printf("listening on :%d", cfg.Server.Port)
		errCh <- httpServer.ListenAndServe()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		logger.Printf("received signal: %s", sig)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := httpServer.Shutdown(ctx); err != nil {
			return fmt.Errorf("shutdown: %w", err)
		}

		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

package httpserver

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"lune/internal/admin"
	"lune/internal/auth"
	"lune/internal/gateway"
	"lune/internal/health"
	"lune/internal/notify"
	"lune/internal/router"
	"lune/internal/site"
	"lune/internal/store"
)

type Server struct {
	mux              *http.ServeMux
	store            *store.Store
	cache            *store.RoutingCache
	cpaAuthDir       string
	cpaManagementKey string
	gatewayTmpDir    string
	healthChecker    *health.Checker
	notifier         *notify.Service
}

var accessLogDedup = struct {
	sync.Mutex
	entries map[string]accessLogEntry
}{entries: make(map[string]accessLogEntry)}

type accessLogEntry struct {
	last       time.Time
	suppressed int
}

func New(st *store.Store, cache *store.RoutingCache, cpaAuthDir, cpaManagementKey, gatewayTmpDir string, hc *health.Checker, notifier *notify.Service) *Server {
	s := &Server{
		mux:              http.NewServeMux(),
		store:            st,
		cache:            cache,
		cpaAuthDir:       cpaAuthDir,
		cpaManagementKey: cpaManagementKey,
		gatewayTmpDir:    gatewayTmpDir,
		healthChecker:    hc,
		notifier:         notifier,
	}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return s.logging(s.mux)
}

func (s *Server) routes() {
	// health endpoints
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})
	s.mux.HandleFunc("GET /readyz", s.handleReadyz)

	// admin API
	adminHandler := admin.NewHandler(s.store, s.cache, s.cpaAuthDir, s.cpaManagementKey, s.healthChecker, s.notifier, s.gatewayTmpDir)
	adminWrap := func(next http.Handler) http.Handler {
		return auth.AdminAuth(next, s.cache)
	}
	adminHandler.RegisterRoutes(s.mux, adminWrap)

	// SPA for /admin paths
	s.mux.Handle("/admin", site.Handler())
	s.mux.Handle("/admin/", site.Handler())

	// root redirect to /admin
	s.mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/admin", http.StatusFound)
	})

	// gateway
	rt := router.NewWithOptions(s.cache, router.Options{CpaRuntimeBindingSupported: s.healthChecker != nil && s.healthChecker.ProviderPinningSupported()})
	gw := gateway.NewHandler(rt, s.cache, s.store, s.gatewayTmpDir, s.healthChecker)
	gwAuth := auth.GatewayAuth(gw, s.cache)

	// GET /v1/models — no auth required
	s.mux.Handle("GET /v1/models", gw)
	s.mux.Handle("GET /openai/v1/models", gw)

	// all other /v1/* and /openai/v1/* — require gateway auth
	s.mux.Handle("/v1/", gwAuth)
	s.mux.Handle("/openai/v1/", gwAuth)
}

func (s *Server) handleReadyz(w http.ResponseWriter, _ *http.Request) {
	total, _, err := s.store.CountAccounts()
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"status":"error","message":"database error"}`))
		return
	}
	if total == 0 {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"status":"error","message":"no accounts configured"}`))
		return
	}
	cpaAccounts, err := s.enabledCpaAccounts()
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"status":"error","message":"database error"}`))
		return
	}
	if len(cpaAccounts) > 0 {
		serviceByID := s.cpaServiceMap()
		for _, acc := range cpaAccounts {
			if acc.CpaServiceID == nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				w.Write([]byte(`{"status":"error","message":"cpa runtime unavailable: service missing"}`))
				return
			}
			svc, ok := serviceByID[*acc.CpaServiceID]
			if !ok {
				w.WriteHeader(http.StatusServiceUnavailable)
				w.Write([]byte(`{"status":"error","message":"cpa runtime unavailable: service missing"}`))
				return
			}
			if !svc.Enabled {
				w.WriteHeader(http.StatusServiceUnavailable)
				w.Write([]byte(`{"status":"error","message":"cpa runtime unavailable: service disabled"}`))
				return
			}
			if strings.ToLower(strings.TrimSpace(svc.Status)) != "healthy" {
				w.WriteHeader(http.StatusServiceUnavailable)
				w.Write([]byte(`{"status":"error","message":"cpa runtime unavailable"}`))
				return
			}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

func (s *Server) enabledCpaAccounts() ([]store.Account, error) {
	accounts, err := s.store.ListAccounts()
	if err != nil {
		return nil, err
	}
	enabled := make([]store.Account, 0, len(accounts))
	for _, acc := range accounts {
		if acc.SourceKind == "cpa" && acc.Enabled {
			enabled = append(enabled, acc)
		}
	}
	return enabled, nil
}

func (s *Server) cpaServiceMap() map[int64]store.CpaService {
	services := make(map[int64]store.CpaService)
	snap := s.cache.Get()
	for id, svc := range snap.CpaServices {
		if svc != nil {
			services[id] = *svc
		}
	}
	return services
}

func (s *Server) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		logAccessRequest(r.Method, r.URL.Path, rw.status, time.Since(start))
	})
}

func logAccessRequest(method, path string, status int, duration time.Duration) {
	key := method + " " + path + " " + strconv.Itoa(status)
	now := time.Now()
	accessLogDedup.Lock()
	entry := accessLogDedup.entries[key]
	if now.Sub(entry.last) < 5*time.Second {
		entry.suppressed++
		accessLogDedup.entries[key] = entry
		accessLogDedup.Unlock()
		return
	}
	accessLogDedup.entries[key] = accessLogEntry{last: now}
	accessLogDedup.Unlock()

	attrs := []any{
		"method", method,
		"path", path,
		"status", status,
		"duration_ms", duration.Milliseconds(),
	}
	if entry.suppressed > 0 {
		attrs = append(attrs, "suppressed_repeats", entry.suppressed)
	}
	if status >= 500 {
		slog.Warn("request completed", attrs...)
		return
	}
	slog.Info("request completed", attrs...)
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

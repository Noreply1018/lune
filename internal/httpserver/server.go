package httpserver

import (
	"log/slog"
	"net/http"
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
	adminHandler := admin.NewHandler(s.store, s.cache, s.cpaAuthDir, s.cpaManagementKey, s.healthChecker, s.notifier)
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
	rt := router.New(s.cache)
	gw := gateway.NewHandler(rt, s.cache, s.store, s.gatewayTmpDir)
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
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

func (s *Server) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		slog.Info("request completed",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
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

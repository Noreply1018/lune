package httpserver

import (
	"log"
	"net/http"
	"time"

	"lune/internal/admin"
	"lune/internal/auth"
	"lune/internal/gateway"
	"lune/internal/router"
	"lune/internal/site"
	"lune/internal/store"
)

type Server struct {
	logger     *log.Logger
	mux        *http.ServeMux
	store      *store.Store
	cache      *store.RoutingCache
	cpaAuthDir string
}

func New(logger *log.Logger, st *store.Store, cache *store.RoutingCache, cpaAuthDir string) *Server {
	s := &Server{
		logger:     logger,
		mux:        http.NewServeMux(),
		store:      st,
		cache:      cache,
		cpaAuthDir: cpaAuthDir,
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
	adminHandler := admin.NewHandler(s.store, s.cache, s.cpaAuthDir)
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
	gw := gateway.NewHandler(rt, s.cache, s.store)
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
		next.ServeHTTP(w, r)
		s.logger.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}

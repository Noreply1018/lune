package httpserver

import (
	"log"
	"net/http"
	"time"

	"lune/internal/admin"
	"lune/internal/auth"
	"lune/internal/site"
	"lune/internal/store"
)

type Server struct {
	logger *log.Logger
	mux    *http.ServeMux
	store  *store.Store
	cache  *store.RoutingCache
}

func New(logger *log.Logger, st *store.Store, cache *store.RoutingCache) *Server {
	s := &Server{
		logger: logger,
		mux:    http.NewServeMux(),
		store:  st,
		cache:  cache,
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
	adminHandler := admin.NewHandler(s.store, s.cache)
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

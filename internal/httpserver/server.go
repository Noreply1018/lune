package httpserver

import (
	"log"
	"net/http"
	"time"

	accountadapter "lune/internal/adapter/account"
	"lune/internal/api/admin"
	"lune/internal/api/oneapiproxy"
	"lune/internal/api/public"
	"lune/internal/auth"
	"lune/internal/config"
	"lune/internal/metrics"
	"lune/internal/platform"
	"lune/internal/proxy"
	"lune/internal/runtimeconfig"
	"lune/internal/site"
	"lune/internal/store"
)

type Server struct {
	cfg      *runtimeconfig.Manager
	logger   *log.Logger
	mux      *http.ServeMux
	store    *store.Store
	metrics  *metrics.Collector
	registry *platform.Registry
}

func New(cfg *runtimeconfig.Manager, logger *log.Logger, st *store.Store, metricCollector *metrics.Collector, registry *platform.Registry) *Server {
	s := &Server{
		cfg:      cfg,
		logger:   logger,
		mux:      http.NewServeMux(),
		store:    st,
		metrics:  metricCollector,
		registry: registry,
	}

	s.registerRoutes()
	return s
}

func (s *Server) Handler() http.Handler {
	return s.loggingMiddleware(s.mux)
}

func (s *Server) registerRoutes() {
	adapterRegistry := accountadapter.NewRegistry(
		accountadapter.NewOpenAIUpstreamAdapter(),
	)
	proxySvc := proxy.New(s.cfg, s.registry, adapterRegistry, s.store, s.metrics)
	publicHandler := public.NewHandler(s.cfg, proxySvc)
	adminHandler := admin.NewHandler(s.cfg, s.store, s.metrics, s.registry)

	s.mux.HandleFunc("/healthz", publicHandler.Healthz)
	s.mux.HandleFunc("/readyz", publicHandler.Readyz)
	s.mux.HandleFunc("/v1/models", publicHandler.Models)

	// One-API reverse proxy (requires admin token)
	s.mux.Handle("/oneapi/", auth.RequireAdminFunc(
		func() string { return s.cfg.Current().Auth.AdminToken },
		oneapiproxy.Handler(s.cfg),
	))

	// SPA deep-link fallbacks — more specific than /admin/ so they win
	for _, p := range []string{"/admin/channels", "/admin/usage", "/admin/tokens", "/admin/login"} {
		s.mux.Handle(p, site.Handler())
	}

	s.mux.Handle("/admin", site.Handler())
	s.mux.HandleFunc("/admin/", adminHandler.Route)

	s.mux.Handle("/openai/v1/chat/completions", auth.RequireAccessTokenFunc(func() []config.AccessToken {
		return s.cfg.Current().Auth.AccessTokens
	}, http.HandlerFunc(publicHandler.ChatCompletions)))
	s.mux.Handle("/openai/v1/responses", auth.RequireAccessTokenFunc(func() []config.AccessToken {
		return s.cfg.Current().Auth.AccessTokens
	}, http.HandlerFunc(publicHandler.Responses)))
	s.mux.Handle("/openai/v1/embeddings", auth.RequireAccessTokenFunc(func() []config.AccessToken {
		return s.cfg.Current().Auth.AccessTokens
	}, http.HandlerFunc(publicHandler.Embeddings)))
	s.mux.Handle("/openai/v1/images/generations", auth.RequireAccessTokenFunc(func() []config.AccessToken {
		return s.cfg.Current().Auth.AccessTokens
	}, http.HandlerFunc(publicHandler.ImagesGenerations)))
	s.mux.Handle("/", site.Handler())
}

func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		s.logger.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).String())
	})
}

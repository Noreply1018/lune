package oneapiproxy

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"lune/internal/runtimeconfig"
)

// Handler returns an http.Handler that reverse-proxies requests to the One-API
// upstream. It strips the "/oneapi" path prefix so that
// /oneapi/api/channel/ → /api/channel/ on the upstream.
//
// The upstream URL is read from the runtime config on every request so that
// hot-reloads take effect immediately.
func Handler(cfg *runtimeconfig.Manager) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstream := cfg.Current().Server.UpstreamURL
		if upstream == "" {
			upstream = "http://localhost:3000"
		}

		target, err := url.Parse(upstream)
		if err != nil {
			http.Error(w, "bad upstream URL", http.StatusBadGateway)
			return
		}

		proxy := &httputil.ReverseProxy{
			Director: func(req *http.Request) {
				req.URL.Scheme = target.Scheme
				req.URL.Host = target.Host
				req.Host = target.Host

				// Strip the /oneapi prefix: /oneapi/api/foo → /api/foo
				req.URL.Path = strings.TrimPrefix(req.URL.Path, "/oneapi")
				if req.URL.RawPath != "" {
					req.URL.RawPath = strings.TrimPrefix(req.URL.RawPath, "/oneapi")
				}
			},
		}

		proxy.ServeHTTP(w, r)
	})
}

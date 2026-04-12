package auth

import (
	"net"
	"net/http"

	"lune/internal/store"
	"lune/internal/webutil"
)

func AdminAuth(next http.Handler, cache *store.RoutingCache) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isLocalhost(r) {
			next.ServeHTTP(w, r)
			return
		}

		token := webutil.BearerToken(r.Header.Get("Authorization"))
		adminToken := cache.GetSetting("admin_token")
		if token == "" || adminToken == "" || token != adminToken {
			webutil.WriteAdminError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isLocalhost(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return host == "127.0.0.1" || host == "::1"
}

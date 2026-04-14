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
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	// Trust loopback and private networks. When running in Docker,
	// requests arrive from the bridge network (e.g. 172.17.0.1) which
	// is a private IP. Security is enforced at the Docker layer by
	// binding the published port to 127.0.0.1 in docker-compose.yml.
	return ip.IsLoopback() || ip.IsPrivate()
}

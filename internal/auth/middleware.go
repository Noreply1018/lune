package auth

import (
	"net/http"

	"lune/internal/config"
	"lune/internal/webutil"
)

func RequireAdmin(adminToken string, next http.Handler) http.Handler {
	return RequireAdminFunc(func() string {
		return adminToken
	}, next)
}

func RequireAdminFunc(getAdminToken func() string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if webutil.BearerToken(r.Header.Get("Authorization")) != getAdminToken() {
			webutil.WriteJSON(w, http.StatusUnauthorized, map[string]any{
				"error": map[string]any{
					"message": "invalid admin token",
					"type":    "auth_error",
				},
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func RequireAccessToken(tokens []config.AccessToken, next http.Handler) http.Handler {
	return RequireAccessTokenFunc(func() []config.AccessToken {
		return tokens
	}, next)
}

func RequireAccessTokenFunc(getTokens func() []config.AccessToken, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		enabled := enabledTokens(getTokens())
		token := webutil.BearerToken(r.Header.Get("Authorization"))
		matched, ok := enabled[token]
		if !ok {
			webutil.WriteJSON(w, http.StatusUnauthorized, map[string]any{
				"error": map[string]any{
					"message": "invalid access token",
					"type":    "auth_error",
				},
			})
			return
		}
		next.ServeHTTP(w, r.WithContext(WithAccessTokenName(r.Context(), matched.Name)))
	})
}

func enabledTokens(tokens []config.AccessToken) map[string]config.AccessToken {
	enabled := make(map[string]config.AccessToken, len(tokens))
	for _, token := range tokens {
		if token.Enabled && token.Token != "" {
			enabled[token.Token] = token
		}
	}
	return enabled
}

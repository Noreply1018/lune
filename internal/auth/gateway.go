package auth

import (
	"context"
	"net/http"

	"lune/internal/store"
	"lune/internal/webutil"
)

type contextKey int

const accessTokenKey contextKey = iota

func GatewayAuth(next http.Handler, cache *store.RoutingCache) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenValue := webutil.BearerToken(r.Header.Get("Authorization"))
		if tokenValue == "" {
			webutil.WriteGatewayError(w, 401, "invalid_token", "invalid access token")
			return
		}

		accessToken := cache.FindAccessToken(tokenValue)
		if accessToken == nil {
			webutil.WriteGatewayError(w, 401, "invalid_token", "invalid access token")
			return
		}
		if !accessToken.Enabled {
			webutil.WriteGatewayError(w, 403, "token_disabled", "access token is disabled")
			return
		}
		if accessToken.QuotaTokens > 0 && accessToken.UsedTokens >= accessToken.QuotaTokens {
			webutil.WriteGatewayError(w, 429, "quota_exhausted", "token quota exhausted")
			return
		}

		ctx := context.WithValue(r.Context(), accessTokenKey, accessToken)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func AccessTokenFromContext(ctx context.Context) *store.AccessToken {
	t, _ := ctx.Value(accessTokenKey).(*store.AccessToken)
	return t
}

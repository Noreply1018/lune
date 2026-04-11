package auth

import "context"

type contextKey string

const accessTokenNameKey contextKey = "access_token_name"

func WithAccessTokenName(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, accessTokenNameKey, name)
}

func AccessTokenNameFromContext(ctx context.Context) string {
	value, _ := ctx.Value(accessTokenNameKey).(string)
	return value
}

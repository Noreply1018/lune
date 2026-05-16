package gateway

import "context"

type diagnosticContextKey struct{}

func ContextWithDiagnostic(ctx context.Context) context.Context {
	return context.WithValue(ctx, diagnosticContextKey{}, true)
}

func DiagnosticFromContext(ctx context.Context) bool {
	v, _ := ctx.Value(diagnosticContextKey{}).(bool)
	return v
}

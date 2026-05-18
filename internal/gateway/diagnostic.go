package gateway

import "context"

type diagnosticContextKey struct{}
type forceAccountContextKey struct{}
type statefulProbeContextKey struct{}

func ContextWithDiagnostic(ctx context.Context) context.Context {
	return context.WithValue(ctx, diagnosticContextKey{}, true)
}

func DiagnosticFromContext(ctx context.Context) bool {
	v, _ := ctx.Value(diagnosticContextKey{}).(bool)
	return v
}

func ContextWithForceAccount(ctx context.Context) context.Context {
	return context.WithValue(ctx, forceAccountContextKey{}, true)
}

func ForceAccountFromContext(ctx context.Context) bool {
	v, _ := ctx.Value(forceAccountContextKey{}).(bool)
	return v
}

func ContextWithStatefulProbe(ctx context.Context) context.Context {
	return context.WithValue(ctx, statefulProbeContextKey{}, true)
}

func StatefulProbeFromContext(ctx context.Context) bool {
	v, _ := ctx.Value(statefulProbeContextKey{}).(bool)
	return v
}

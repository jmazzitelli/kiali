package context

import (
	"context"
)

var contextKeyTenancyNamespace contextKey = "tenancyNamespace"

// SetTenancyNamespace attaches a namespace to the context for Prometheus tenancy
// scoping. The queryParamsRoundTripper reads this value and substitutes it into
// query parameters that use the {namespace} placeholder.
func SetTenancyNamespace(ctx context.Context, namespace string) context.Context {
	return context.WithValue(ctx, contextKeyTenancyNamespace, namespace)
}

// GetTenancyNamespace retrieves the tenancy namespace from the context, or
// returns empty string if not set.
func GetTenancyNamespace(ctx context.Context) string {
	if ns, ok := ctx.Value(contextKeyTenancyNamespace).(string); ok {
		return ns
	}
	return ""
}

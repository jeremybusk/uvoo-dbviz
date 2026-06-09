package server

import (
	"context"

	"uvoo-dbviz/internal/auth"
)

type principalKey struct{}

func withPrincipal(ctx context.Context, principal auth.Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, principal)
}

func principal(ctxCarrier interface{ Context() context.Context }) auth.Principal {
	value, _ := ctxCarrier.Context().Value(principalKey{}).(auth.Principal)
	return value
}

func statePrincipal(ctxCarrier interface{ Context() context.Context }) auth.Principal {
	user := principal(ctxCarrier)
	if activeTenant := user.Headers["ActiveTenantID"]; activeTenant != "" {
		user.TenantID = activeTenant
	}
	return user
}

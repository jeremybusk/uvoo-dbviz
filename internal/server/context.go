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

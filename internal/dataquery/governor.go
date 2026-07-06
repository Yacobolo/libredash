package dataquery

import "context"

type ResultTransformer func(*Result, error) error

type Governor interface {
	GovernDataQuery(ctx context.Context, request Query) (Query, ResultTransformer, error)
}

type governorContextKey struct{}
type governanceAppliedContextKey struct{}

func WithGovernor(ctx context.Context, governor Governor) context.Context {
	if governor == nil {
		return ctx
	}
	return context.WithValue(ctx, governorContextKey{}, governor)
}

func GovernorFromContext(ctx context.Context) (Governor, bool) {
	governor, ok := ctx.Value(governorContextKey{}).(Governor)
	return governor, ok && governor != nil
}

func WithGovernanceApplied(ctx context.Context) context.Context {
	return context.WithValue(ctx, governanceAppliedContextKey{}, true)
}

func GovernanceApplied(ctx context.Context) bool {
	applied, _ := ctx.Value(governanceAppliedContextKey{}).(bool)
	return applied
}

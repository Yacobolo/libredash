package access

import (
	"context"
	"sync"
)

type authorizationCacheContextKey struct{}

type AuthorizationCache struct {
	mu        sync.Mutex
	decisions map[authorizationCacheKey]AuthorizationDecision
}

type authorizationCacheKey struct {
	PrincipalID string
	Privilege   Privilege
	ObjectID    string
}

func WithAuthorizationCache(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Value(authorizationCacheContextKey{}).(*AuthorizationCache); ok {
		return ctx
	}
	return context.WithValue(ctx, authorizationCacheContextKey{}, &AuthorizationCache{decisions: map[authorizationCacheKey]AuthorizationDecision{}})
}

func CachedAuthorizationDecision(ctx context.Context, principalID string, privilege Privilege, object ObjectRef) (AuthorizationDecision, bool) {
	cache, ok := ctx.Value(authorizationCacheContextKey{}).(*AuthorizationCache)
	if !ok || cache == nil {
		return AuthorizationDecision{}, false
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	decision, ok := cache.decisions[authorizationCacheKey{PrincipalID: principalID, Privilege: privilege, ObjectID: object.CanonicalID()}]
	return decision, ok
}

func StoreAuthorizationDecision(ctx context.Context, principalID string, decision AuthorizationDecision) {
	cache, ok := ctx.Value(authorizationCacheContextKey{}).(*AuthorizationCache)
	if !ok || cache == nil {
		return
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	cache.decisions[authorizationCacheKey{PrincipalID: principalID, Privilege: decision.Privilege, ObjectID: decision.Object.CanonicalID()}] = decision
}

func ClearAuthorizationCache(ctx context.Context) {
	cache, ok := ctx.Value(authorizationCacheContextKey{}).(*AuthorizationCache)
	if !ok || cache == nil {
		return
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	clear(cache.decisions)
}

package rbac

import (
	"context"
	"sync"
)

type AuthCall struct {
	Actor  Subject
	Action Action
	Object Object
}

type cachedCalls struct {
	authz Authorizer
}

// Cacher returns an Authorizer that can use a cache stored on a context
// to short circuit duplicate calls to the Authorizer. This is useful when
// multiple calls are made to the Authorizer for the same subject, action, and
// object. The cache is on each `ctx` and is not shared between requests.
// If no cache is found on the context, the Authorizer is called as normal.
//
// Cacher is safe for multiple actors.
func Cacher(authz Authorizer) Authorizer {
	return &cachedCalls{authz: authz}
}

func (c *cachedCalls) Authorize(ctx context.Context, subject Subject, action Action, object Object) error {
	cache := cacheFromContext(ctx)

	resp, ok := cache.Load(subject, action, object)
	if ok {
		return resp
	}

	err := c.authz.Authorize(ctx, subject, action, object)
	cache.Save(subject, action, object, err)
	return err
}

// Prepare returns the underlying PreparedAuthorized. The cache does not apply
// to prepared authorizations. These should be using a SQL filter, and
// therefore the cache is not needed.
func (c *cachedCalls) Prepare(ctx context.Context, subject Subject, action Action, objectType string) (PreparedAuthorized, error) {
	return c.authz.Prepare(ctx, subject, action, objectType)
}

type cachedAuthCall struct {
	AuthCall
	Err error
}

type authorizeCache struct {
	sync.Mutex
	// calls is a list of all calls made to the Authorizer.
	// This list is cached per request context. The size of this list is expected
	// to be incredibly small. Often 1 or 2 calls.
	calls []cachedAuthCall
}

//nolint:error-return,revive
func (c *authorizeCache) Load(subject Subject, action Action, object Object) (error, bool) {
	if c == nil {
		return nil, false
	}
	c.Lock()
	defer c.Unlock()

	for _, call := range c.calls {
		if call.Action == action && call.Object.Equal(object) && call.Actor.Equal(subject) {
			return call.Err, true
		}
	}
	return nil, false
}

func (c *authorizeCache) Save(subject Subject, action Action, object Object, err error) {
	if c == nil {
		return
	}
	c.Lock()
	defer c.Unlock()

	c.calls = append(c.calls, cachedAuthCall{
		AuthCall: AuthCall{
			Actor:  subject,
			Action: action,
			Object: object,
		},
		Err: err,
	})
}

// cacheContextKey is a context key used to store the cache in the context.
type cacheContextKey struct{}

// cacheFromContext returns the cache from the context.
// If there is no cache, a nil value is returned.
// The nil cache can still be called as a normal cache, but will not cache or
// return any values.
func cacheFromContext(ctx context.Context) *authorizeCache {
	cache, _ := ctx.Value(cacheContextKey{}).(*authorizeCache)
	return cache
}

func WithCacheCtx(ctx context.Context) context.Context {
	return context.WithValue(ctx, cacheContextKey{}, &authorizeCache{})
}

package keypool

import "time"

// ResolvedKey represents a key returned by a KeyResolver. The
// caller uses Value() for the upstream request and calls the
// appropriate Mark method based on the response.
type ResolvedKey interface {
	// Value returns the key string for authenticating with the
	// upstream provider.
	Value() string
	// MarkTemporary signals a temporary failure (e.g. 429).
	MarkTemporary(cooldown time.Duration)
	// MarkPermanent signals a permanent failure (e.g. 401/403).
	MarkPermanent()
}

// KeyResolver provides keys for upstream requests. The
// implementation determines whether keys come from a shared
// pool (centralized) or from the user (BYOK).
type KeyResolver interface {
	// Next returns the next key to use. Returns
	// ErrAllKeysExhausted when no more keys are available.
	Next() (ResolvedKey, error)
}

// PoolResolver resolves keys from a shared key pool. Each call
// to Next advances the walker to the next available key.
type PoolResolver struct {
	walker *Walker
}

// NewPoolResolver creates a resolver backed by the given pool.
func NewPoolResolver(pool *Pool) *PoolResolver {
	return &PoolResolver{walker: pool.Walker()}
}

// Next returns the next available key from the pool. The
// returned Key implements ResolvedKey with mark methods that
// update pool state.
func (r *PoolResolver) Next() (ResolvedKey, error) {
	return r.walker.Next()
}

// BYOKResolver resolves a single user-provided key. Next always
// returns the same key because BYOK keys are not managed by a
// pool.
type BYOKResolver struct {
	key *byokResolvedKey
}

// NewBYOKResolver creates a resolver for a user-provided key.
func NewBYOKResolver(value string) *BYOKResolver {
	return &BYOKResolver{
		key: &byokResolvedKey{value: value},
	}
}

// Next always returns the same BYOK key.
func (r *BYOKResolver) Next() (ResolvedKey, error) {
	return r.key, nil
}

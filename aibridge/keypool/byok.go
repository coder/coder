package keypool

import "time"

// byokResolvedKey is a BYOK key that is not managed by a pool. Mark
// methods are no-ops because the key is owned by the user,
// not by the system.
type byokResolvedKey struct {
	value string
}

func (k *byokResolvedKey) Value() string { return k.value }

// MarkTemporary is a no-op for BYOK keys.
func (*byokResolvedKey) MarkTemporary(_ time.Duration) {}

// MarkPermanent is a no-op for BYOK keys.
func (*byokResolvedKey) MarkPermanent() {}

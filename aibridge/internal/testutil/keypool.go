package testutil

import (
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/quartz"
)

// SingleKeyPool builds a centralized key pool containing a single key, or nil
// when key is empty (no centralized credential). It panics if the pool cannot
// be built, which does not happen for a non-empty key.
func SingleKeyPool(name, key string) *keypool.Pool {
	if key == "" {
		return nil
	}
	pool, err := keypool.New(name, []string{key}, quartz.NewReal(), nil)
	if err != nil {
		panic(err)
	}
	return pool
}

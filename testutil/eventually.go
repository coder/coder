package testutil

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// Eventually is like require.Eventually except it allows passing
// a context into the condition. It is safe to use with `require.*`.
//
// If ctx times out, the test will fail, but not immediately.
// It is the caller's responsibility to exit early if required.
//
// It is the caller's responsibility to ensure that ctx has a
// deadline or timeout set. Eventually will panic if this is not
// the case in order to avoid potentially waiting forever.
//
// condition is not run in a goroutine; use the provided
// context argument for cancellation if required.
func Eventually(ctx context.Context, t testing.TB, condition func(ctx context.Context) (done bool), tick time.Duration, msgAndArgs ...interface{}) (done bool) {
	t.Helper()

	if _, ok := ctx.Deadline(); !ok {
		panic("developer error: must set deadline or timeout on ctx")
	}

	msg := "Eventually timed out"
	if len(msgAndArgs) > 0 {
		m, ok := msgAndArgs[0].(string)
		if !ok {
			panic("developer error: first argument of msgAndArgs must be a string")
		}
		msg = fmt.Sprintf(m, msgAndArgs[1:]...)
	}

	ticker := time.NewTicker(tick)
	defer ticker.Stop()
	for tick := ticker.C; ; {
		select {
		case <-t.Context().Done():
			assert.NoError(t, t.Context().Err(), msg)
			return false
		case <-ctx.Done():
			assert.NoError(t, ctx.Err(), msg)
			return false
		case <-tick:
			if !assert.NoError(t, ctx.Err(), msg) {
				return false
			}
			if condition(ctx) {
				return true
			}
		}
	}
}

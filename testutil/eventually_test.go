package testutil_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"

	"github.com/coder/coder/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestEventually(t *testing.T) {
	t.Parallel()
	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		state := 0
		condition := func(_ context.Context) bool {
			defer func() {
				state++
			}()
			return state > 2
		}
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()
		testutil.Eventually(ctx, t, condition, testutil.IntervalFast)
	})

	t.Run("Panic", func(t *testing.T) {
		t.Parallel()

		panicky := func() {
			mockT := new(testing.T)
			condition := func(_ context.Context) bool { return true }
			testutil.Eventually(context.Background(), mockT, condition, testutil.IntervalFast)
		}
		assert.Panics(t, panicky)
	})
}

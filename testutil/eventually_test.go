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

	t.Run("Timeout", func(t *testing.T) {
		t.Parallel()
		condition := func(_ context.Context) bool {
			return false
		}
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()
		mockT := new(testing.T)
		testutil.Eventually(ctx, mockT, condition, testutil.IntervalFast)
		assert.True(t, mockT.Failed())
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

	t.Run("Short", func(t *testing.T) {
		t.Parallel()
		testutil.EventuallyShort(t, func(_ context.Context) bool { return true })
	})

	t.Run("Medium", func(t *testing.T) {
		t.Parallel()
		testutil.EventuallyMedium(t, func(_ context.Context) bool { return true })
	})

	t.Run("Long", func(t *testing.T) {
		t.Parallel()
		testutil.EventuallyLong(t, func(_ context.Context) bool { return true })
	})
}

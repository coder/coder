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
		condition := func() bool {
			defer func() {
				state++
			}()
			return state > 2
		}
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()
		assert.True(t, testutil.Eventually(ctx, t, condition, testutil.IntervalFast))
	})

	t.Run("Timeout", func(t *testing.T) {
		t.Parallel()
		condition := func() bool {
			return false
		}
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()
		mockT := new(testing.T)
		assert.False(t, testutil.Eventually(ctx, mockT, condition, testutil.IntervalFast))
		assert.True(t, mockT.Failed())
	})

	t.Run("Panic", func(t *testing.T) {
		t.Parallel()

		panicky := func() {
			mockT := new(testing.T)
			condition := func() bool { return true }
			assert.False(t, testutil.Eventually(context.Background(), mockT, condition, testutil.IntervalFast))
		}
		assert.Panics(t, panicky)
	})

	t.Run("Short", func(t *testing.T) {
		t.Parallel()
		assert.True(t, testutil.EventuallyShort(t, func() bool { return true }))
	})

	t.Run("Medium", func(t *testing.T) {
		t.Parallel()
		assert.True(t, testutil.EventuallyMedium(t, func() bool { return true }))
	})

	t.Run("Long", func(t *testing.T) {
		t.Parallel()
		assert.True(t, testutil.EventuallyLong(t, func() bool { return true }))
	})
}

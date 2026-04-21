package utils_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"golang.org/x/xerrors"

	"github.com/coder/aibridge/utils"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestConcurrentGroup(t *testing.T) {
	t.Parallel()

	t.Run("no goroutines", func(t *testing.T) {
		t.Parallel()

		cg := utils.NewConcurrentGroup()
		require.NoError(t, cg.Wait())
	})

	t.Run("multiple goroutines, all ok", func(t *testing.T) {
		t.Parallel()

		cg := utils.NewConcurrentGroup()
		cg.Go(func() error {
			return nil
		})
		cg.Go(func() error {
			return nil
		})
		require.NoError(t, cg.Wait())
	})

	t.Run("multiple goroutines, one err", func(t *testing.T) {
		t.Parallel()

		cg := utils.NewConcurrentGroup()
		oops := xerrors.New("oops")
		cg.Go(func() error {
			return oops
		})
		cg.Go(func() error {
			return nil
		})
		require.ErrorIs(t, cg.Wait(), oops)
	})

	t.Run("multiple goroutines, multiple errs", func(t *testing.T) {
		t.Parallel()

		cg := utils.NewConcurrentGroup()
		oops := xerrors.New("oops")
		eek := xerrors.New("eek")
		cg.Go(func() error {
			return oops
		})
		cg.Go(func() error {
			return eek
		})

		errs := cg.Wait()
		require.ErrorIs(t, errs, oops)
		require.ErrorIs(t, errs, eek)
	})
}

func BenchmarkConcurrentGroup(b *testing.B) {
	for i := 0; i < b.N; i++ {
		cg := utils.NewConcurrentGroup()
		for j := 0; j < 10; j++ {
			cg.Go(func() error { return nil })
		}
		_ = cg.Wait()
	}
}

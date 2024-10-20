package runtimeconfig_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/runtimeconfig"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
)

func TestEntry(t *testing.T) {
	t.Parallel()

	t.Run("new", func(t *testing.T) {
		t.Parallel()

		require.Panics(t, func() {
			// No name should panic
			runtimeconfig.MustNew[*serpent.Float64]("")
		})

		require.NotPanics(t, func() {
			runtimeconfig.MustNew[*serpent.Float64]("my-field")
		})
	})

	t.Run("simple", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		mgr := runtimeconfig.NewManager()
		db, _ := dbtestutil.NewDB(t)

		override := serpent.String("dogfood@dev.coder.com")

		field := runtimeconfig.MustNew[*serpent.String]("string-field")

		// No value set yet.
		_, err := field.Resolve(ctx, mgr.Resolver(db))
		require.ErrorIs(t, err, runtimeconfig.ErrEntryNotFound)
		// Set an org-level override.
		require.NoError(t, field.SetRuntimeValue(ctx, mgr.Resolver(db), &override))
		// Value was updated
		val, err := field.Resolve(ctx, mgr.Resolver(db))
		require.NoError(t, err)
		require.Equal(t, override.String(), val.String())
	})

	t.Run("complex", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		mgr := runtimeconfig.NewManager()
		db, _ := dbtestutil.NewDB(t)

		override := serpent.Struct[map[string]string]{
			Value: map[string]string{
				"a": "b",
				"c": "d",
			},
		}

		field := runtimeconfig.MustNew[*serpent.Struct[map[string]string]]("string-field")
		// Validate that there is no runtime override right now.
		_, err := field.Resolve(ctx, mgr.Resolver(db))
		require.ErrorIs(t, err, runtimeconfig.ErrEntryNotFound)
		// Set a runtime value
		require.NoError(t, field.SetRuntimeValue(ctx, mgr.Resolver(db), &override))
		// Coalesce now returns the org-level value.
		structVal, err := field.Resolve(ctx, mgr.Resolver(db))
		require.NoError(t, err)
		require.Equal(t, override.Value, structVal.Value)
	})
}

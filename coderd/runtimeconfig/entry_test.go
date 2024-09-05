package runtimeconfig_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database/dbmem"
	"github.com/coder/coder/v2/coderd/runtimeconfig"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
)

// TestEntry demonstrates creating org-level overrides for deployment-level settings.
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

		{
			var field runtimeconfig.DeploymentEntry[*serpent.Float64]
			field.Initialize("my-field")
			// "hello" cannot be set on a *serpent.Float64 field.
			require.Error(t, field.Set("hello"))
		}
	})

	t.Run("zero", func(t *testing.T) {
		t.Parallel()

		rlv := runtimeconfig.NewNoopResolver()

		// A zero-value declaration of a runtimeconfig.Entry should behave as a zero value of the generic type.
		// NB! A name has not been set for this entry; it is "uninitialized".
		var field runtimeconfig.DeploymentEntry[*serpent.Bool]
		var zero serpent.Bool
		require.Equal(t, field.StartupValue().Value(), zero.Value())

		// Setting a value will not produce an error.
		require.NoError(t, field.SetStartupValue("true"))

		// But attempting to resolve will produce an error.
		_, err := field.Resolve(context.Background(), rlv)
		require.ErrorIs(t, err, runtimeconfig.ErrNameNotSet)
		// But attempting to set the runtime value will produce an error.
		val := serpent.BoolOf(ptr.Ref(true))
		require.ErrorIs(t, field.SetRuntimeValue(context.Background(), rlv, val), runtimeconfig.ErrNameNotSet)
	})

	t.Run("simple", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		mgr := runtimeconfig.NewStoreManager()
		db := dbmem.New()

		var (
			base     = serpent.String("system@dev.coder.com")
			override = serpent.String("dogfood@dev.coder.com")
		)

		var field runtimeconfig.DeploymentEntry[*serpent.String]
		field.Initialize("my-field")
		field.SetStartupValue(base.String())
		// Check that default has been set.
		require.Equal(t, base.String(), field.StartupValue().String())
		// Validate that it returns that value.
		require.Equal(t, base.String(), field.String())
		// Validate that there is no org-level override right now.
		_, err := field.Resolve(ctx, mgr.DeploymentResolver(db))
		require.ErrorIs(t, err, runtimeconfig.EntryNotFound)
		// Coalesce returns the deployment-wide value.
		val, err := field.Coalesce(ctx, mgr.DeploymentResolver(db))
		require.NoError(t, err)
		require.Equal(t, base.String(), val.String())
		// Set an org-level override.
		require.NoError(t, field.SetRuntimeValue(ctx, mgr.DeploymentResolver(db), &override))
		// Coalesce now returns the org-level value.
		val, err = field.Coalesce(ctx, mgr.DeploymentResolver(db))
		require.NoError(t, err)
		require.Equal(t, override.String(), val.String())
	})

	t.Run("complex", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		mgr := runtimeconfig.NewStoreManager()
		db := dbmem.New()

		var (
			base = serpent.Struct[map[string]string]{
				Value: map[string]string{"access_type": "offline"},
			}
			override = serpent.Struct[map[string]string]{
				Value: map[string]string{
					"a": "b",
					"c": "d",
				},
			}
		)

		var field runtimeconfig.DeploymentEntry[*serpent.Struct[map[string]string]]
		field.Initialize("my-field")
		field.SetStartupValue(base.String())

		// Check that default has been set.
		require.Equal(t, base.String(), field.StartupValue().String())
		// Validate that there is no org-level override right now.
		_, err := field.Resolve(ctx, mgr.DeploymentResolver(db))
		require.ErrorIs(t, err, runtimeconfig.EntryNotFound)
		// Coalesce returns the deployment-wide value.
		val, err := field.Coalesce(ctx, mgr.DeploymentResolver(db))
		require.NoError(t, err)
		require.Equal(t, base.Value, val.Value)
		// Set an org-level override.
		require.NoError(t, field.SetRuntimeValue(ctx, mgr.DeploymentResolver(db), &override))
		// Coalesce now returns the org-level value.
		structVal, err := field.Resolve(ctx, mgr.DeploymentResolver(db))
		require.NoError(t, err)
		require.Equal(t, override.Value, structVal.Value)
	})
}

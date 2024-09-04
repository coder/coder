package runtimeconfig_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/serpent"

	"github.com/coder/coder/v2/coderd/database/dbmem"
	"github.com/coder/coder/v2/coderd/runtimeconfig"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/testutil"
)

func TestUsage(t *testing.T) {
	t.Parallel()

	t.Run("deployment value without runtimeconfig", func(t *testing.T) {
		t.Parallel()

		var field serpent.StringArray
		opt := serpent.Option{
			Name:        "my deployment value",
			Description: "this mimicks an option we'd define in codersdk/deployment.go",
			Env:         "MY_DEPLOYMENT_VALUE",
			Default:     "pestle,mortar",
			Value:       &field,
		}

		set := serpent.OptionSet{opt}
		require.NoError(t, set.SetDefaults())
		require.Equal(t, []string{"pestle", "mortar"}, field.Value())
	})

	t.Run("deployment value with runtimeconfig", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		mgr := runtimeconfig.NewStoreManager(dbmem.New())

		// NOTE: this field is now wrapped
		var field runtimeconfig.Entry[*serpent.HostPort]
		opt := serpent.Option{
			Name:        "my deployment value",
			Description: "this mimicks an option we'd define in codersdk/deployment.go",
			Env:         "MY_DEPLOYMENT_VALUE",
			Default:     "localhost:1234",
			Value:       &field,
		}

		set := serpent.OptionSet{opt}
		require.NoError(t, set.SetDefaults())

		// The value has to now be retrieved from a StartupValue() call.
		require.Equal(t, "localhost:1234", field.StartupValue().String())

		// One new constraint is that we have to set the name on the runtimeconfig.Entry.
		// Attempting to perform any operation which accesses the store will enforce the need for a name.
		_, err := field.Resolve(ctx, mgr)
		require.ErrorIs(t, err, runtimeconfig.ErrNameNotSet)

		// Let's set that name; the environment var name is likely to be the most stable.
		field.Initialize(opt.Env)

		newVal := serpent.HostPort{Host: "12.34.56.78", Port: "1234"}
		// Now that we've set it, we can update the runtime value of this field, which modifies given store.
		require.NoError(t, field.SetRuntimeValue(ctx, mgr, &newVal))

		// ...and we can retrieve the value, as well.
		resolved, err := field.Resolve(ctx, mgr)
		require.NoError(t, err)
		require.Equal(t, newVal.String(), resolved.String())

		// We can also remove the runtime config.
		require.NoError(t, field.UnsetRuntimeValue(ctx, mgr))
	})
}

// TestConfig demonstrates creating org-level overrides for deployment-level settings.
func TestConfig(t *testing.T) {
	t.Parallel()

	t.Run("new", func(t *testing.T) {
		t.Parallel()

		require.Panics(t, func() {
			// "hello" cannot be set on a *serpent.Float64 field.
			runtimeconfig.MustNew[*serpent.Float64]("my-field", "hello")
		})

		require.NotPanics(t, func() {
			runtimeconfig.MustNew[*serpent.Float64]("my-field", "91.1234")
		})
	})

	t.Run("zero", func(t *testing.T) {
		t.Parallel()

		mgr := runtimeconfig.NewNoopManager()

		// A zero-value declaration of a runtimeconfig.Entry should behave as a zero value of the generic type.
		// NB! A name has not been set for this entry; it is "uninitialized".
		var field runtimeconfig.Entry[*serpent.Bool]
		var zero serpent.Bool
		require.Equal(t, field.StartupValue().Value(), zero.Value())

		// Setting a value will not produce an error.
		require.NoError(t, field.SetStartupValue("true"))

		// But attempting to resolve will produce an error.
		_, err := field.Resolve(context.Background(), mgr)
		require.ErrorIs(t, err, runtimeconfig.ErrNameNotSet)
		// But attempting to set the runtime value will produce an error.
		val := serpent.BoolOf(ptr.Ref(true))
		require.ErrorIs(t, field.SetRuntimeValue(context.Background(), mgr, val), runtimeconfig.ErrNameNotSet)
	})

	t.Run("simple", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		mgr := runtimeconfig.NewStoreManager(dbmem.New())

		var (
			base     = serpent.String("system@dev.coder.com")
			override = serpent.String("dogfood@dev.coder.com")
		)

		field := runtimeconfig.MustNew[*serpent.String]("my-field", base.String())
		// Check that default has been set.
		require.Equal(t, base.String(), field.StartupValue().String())
		// Validate that it returns that value.
		require.Equal(t, base.String(), field.String())
		// Validate that there is no org-level override right now.
		_, err := field.Resolve(ctx, mgr)
		require.ErrorIs(t, err, runtimeconfig.EntryNotFound)
		// Coalesce returns the deployment-wide value.
		val, err := field.Coalesce(ctx, mgr)
		require.NoError(t, err)
		require.Equal(t, base.String(), val.String())
		// Set an org-level override.
		require.NoError(t, field.SetRuntimeValue(ctx, mgr, &override))
		// Coalesce now returns the org-level value.
		val, err = field.Coalesce(ctx, mgr)
		require.NoError(t, err)
		require.Equal(t, override.String(), val.String())
	})

	t.Run("complex", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		mgr := runtimeconfig.NewStoreManager(dbmem.New())

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

		field := runtimeconfig.MustNew[*serpent.Struct[map[string]string]]("my-field", base.String())

		// Check that default has been set.
		require.Equal(t, base.String(), field.StartupValue().String())
		// Validate that there is no org-level override right now.
		_, err := field.Resolve(ctx, mgr)
		require.ErrorIs(t, err, runtimeconfig.EntryNotFound)
		// Coalesce returns the deployment-wide value.
		val, err := field.Coalesce(ctx, mgr)
		require.NoError(t, err)
		require.Equal(t, base.Value, val.Value)
		// Set an org-level override.
		require.NoError(t, field.SetRuntimeValue(ctx, mgr, &override))
		// Coalesce now returns the org-level value.
		structVal, err := field.Resolve(ctx, mgr)
		require.NoError(t, err)
		require.Equal(t, override.Value, structVal.Value)
	})
}

func TestScoped(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()

	ctx := testutil.Context(t, testutil.WaitShort)

	// Set up a config manager and a field which will have runtime configs.
	mgr := runtimeconfig.NewStoreManager(dbmem.New())
	field := runtimeconfig.MustNew[*serpent.HostPort]("addr", "localhost:3000")

	// No runtime value set at this point, Coalesce will return startup value.
	_, err := field.Resolve(ctx, mgr)
	require.ErrorIs(t, err, runtimeconfig.EntryNotFound)
	val, err := field.Coalesce(ctx, mgr)
	require.NoError(t, err)
	require.Equal(t, field.StartupValue().String(), val.String())

	// Set a runtime value which is NOT org-scoped.
	host, port := "localhost", "1234"
	require.NoError(t, field.SetRuntimeValue(ctx, mgr, &serpent.HostPort{Host: host, Port: port}))
	val, err = field.Resolve(ctx, mgr)
	require.NoError(t, err)
	require.Equal(t, host, val.Host)
	require.Equal(t, port, val.Port)

	orgMgr := mgr.Scoped(orgID.String())
	// Using the org scope, nothing will be returned.
	_, err = field.Resolve(ctx, orgMgr)
	require.ErrorIs(t, err, runtimeconfig.EntryNotFound)

	// Now set an org-scoped value.
	host, port = "localhost", "4321"
	require.NoError(t, field.SetRuntimeValue(ctx, orgMgr, &serpent.HostPort{Host: host, Port: port}))
	val, err = field.Resolve(ctx, orgMgr)
	require.NoError(t, err)
	require.Equal(t, host, val.Host)
	require.Equal(t, port, val.Port)

	// Ensure the two runtime configs are NOT equal to each other nor the startup value.
	global, err := field.Resolve(ctx, mgr)
	require.NoError(t, err)
	org, err := field.Resolve(ctx, orgMgr)
	require.NoError(t, err)

	require.NotEqual(t, global.String(), org.String())
	require.NotEqual(t, field.StartupValue().String(), global.String())
	require.NotEqual(t, field.StartupValue().String(), org.String())
}

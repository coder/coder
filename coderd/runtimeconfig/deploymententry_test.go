package runtimeconfig_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database/dbmem"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/runtimeconfig"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
)

func ExampleDeploymentValues() {
	ctx := context.Background()
	db := dbmem.New()
	st := runtimeconfig.NewStoreManager()

	// Define the field, this will usually live on Deployment Values.
	var stringField runtimeconfig.DeploymentEntry[*serpent.String]
	// All fields need to be initialized with their "key". This will be used
	// to uniquely identify the field in the store.
	stringField.Initialize("string-field")

	// The startup value configured by the deployment env vars
	// This acts as a default value if no runtime value is set.
	// Can be used to support migrating a value from startup to runtime.
	_ = stringField.SetStartupValue("default")

	// Runtime values take priority over startup values.
	_ = stringField.SetRuntimeValue(ctx, st.Resolver(db), serpent.StringOf(ptr.Ref("hello world")))

	// Resolve the value of the field.
	val, err := stringField.Resolve(ctx, st.Resolver(db))
	if err != nil {
		panic(err)
	}
	fmt.Println(val)
	// Output: hello world
}

// TestResolveDBError ensures a db error that is not a sql.ErrNoRows
// will bubble up using Coalesce. The error should not be ignored and replaced
// with the startup value.
func TestResolveDBError(t *testing.T) {
	t.Parallel()

	dbErr := xerrors.Errorf("some db error")
	ctrl := gomock.NewController(t)
	mDB := dbmock.NewMockStore(ctrl)
	// Error on fetch
	mDB.EXPECT().
		GetRuntimeConfig(gomock.Any(), gomock.Any()).
		Times(1).
		Return("", dbErr)

	// Error on upsert
	mDB.EXPECT().
		UpsertRuntimeConfig(gomock.Any(), gomock.Any()).
		Times(1).
		Return(dbErr)

	// Error on delete
	mDB.EXPECT().
		DeleteRuntimeConfig(gomock.Any(), gomock.Any()).
		Times(1).
		Return(dbErr)

	st := runtimeconfig.NewStoreManager()
	var stringField runtimeconfig.DeploymentEntry[*serpent.String]
	stringField.Initialize("string-field")
	stringField.SetStartupValue("default")

	ctx := testutil.Context(t, testutil.WaitMedium)
	// Resolve
	_, err := stringField.Coalesce(ctx, st.Resolver(mDB))
	require.ErrorIs(t, err, dbErr)
	// Set
	err = stringField.SetRuntimeValue(ctx, st.Resolver(mDB), serpent.StringOf(ptr.Ref("hello world")))
	require.ErrorIs(t, err, dbErr)
	// Unset
	err = stringField.UnsetRuntimeValue(ctx, st.Resolver(mDB))
	require.ErrorIs(t, err, dbErr)
}

// TestSerpentDeploymentEntry uses the package as the serpent options will use it.
// Some of the usage might feel awkward, since the serpent package values come from
// the serpent parsing (strings), not manual assignment.
func TestSerpentDeploymentEntry(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitMedium)
	db, _ := dbtestutil.NewDB(t)
	st := runtimeconfig.NewStoreManager()

	// TestEntries is how entries are defined in deployment values.
	type TestEntries struct {
		String runtimeconfig.DeploymentEntry[*serpent.String]
		Bool   runtimeconfig.DeploymentEntry[*serpent.Bool]
		// codersdk.Feature is arbitrary, just using an actual struct to test.
		Struct runtimeconfig.DeploymentEntry[*serpent.Struct[codersdk.Feature]]
	}

	var entries TestEntries
	// Init fields
	entries.String.Initialize("string-field")
	entries.Bool.Initialize("bool-field")
	entries.Struct.Initialize("struct-field")

	// Check the Type() methods are unchanged
	require.Equal(t, entries.String.Type(), (serpent.String("")).Type())
	require.Equal(t, entries.Bool.Type(), (serpent.Bool(false)).Type())
	require.Equal(t, entries.Struct.Type(), (&serpent.Struct[codersdk.Feature]{}).Type())

	// When using Coalesce, the default value is the empty value
	stringVal, err := entries.String.Coalesce(ctx, st.Resolver(db))
	require.NoError(t, err)
	require.Equal(t, "", stringVal.String())

	// Set some defaults for some
	_ = entries.String.SetStartupValue("default")
	_ = entries.Struct.SetStartupValue((&serpent.Struct[codersdk.Feature]{
		Value: codersdk.Feature{
			Entitlement: codersdk.EntitlementEntitled,
			Enabled:     false,
			Limit:       ptr.Ref(int64(100)),
			Actual:      nil,
		},
	}).String())

	// Retrieve startup values
	stringVal, err = entries.String.Coalesce(ctx, st.Resolver(db))
	require.NoError(t, err)
	require.Equal(t, "default", stringVal.String())

	structVal, err := entries.Struct.Coalesce(ctx, st.Resolver(db))
	require.NoError(t, err)
	require.Equal(t, structVal.Value.Entitlement, codersdk.EntitlementEntitled)
	require.Equal(t, structVal.Value.Limit, ptr.Ref(int64(100)))

	// Override some defaults
	err = entries.String.SetRuntimeValue(ctx, st.Resolver(db), serpent.StringOf(ptr.Ref("hello world")))
	require.NoError(t, err)

	err = entries.Struct.SetRuntimeValue(ctx, st.Resolver(db), &serpent.Struct[codersdk.Feature]{
		Value: codersdk.Feature{
			Entitlement: codersdk.EntitlementGracePeriod,
		},
	})
	require.NoError(t, err)

	// Retrieve runtime values
	stringVal, err = entries.String.Coalesce(ctx, st.Resolver(db))
	require.NoError(t, err)
	require.Equal(t, "hello world", stringVal.String())

	structVal, err = entries.Struct.Coalesce(ctx, st.Resolver(db))
	require.NoError(t, err)
	require.Equal(t, structVal.Value.Entitlement, codersdk.EntitlementGracePeriod)

	// Test unset
	err = entries.String.UnsetRuntimeValue(ctx, st.Resolver(db))
	require.NoError(t, err)
	stringVal, err = entries.String.Coalesce(ctx, st.Resolver(db))
	require.NoError(t, err)
	require.Equal(t, "default", stringVal.String())

	// Test using org scoped resolver
	orgID := uuid.New()
	orgResolver := st.OrganizationResolver(db, orgID)
	// No org runtime set
	stringVal, err = entries.String.Coalesce(ctx, orgResolver)
	require.NoError(t, err)
	require.Equal(t, "default", stringVal.String())
	// Update org runtime
	err = entries.String.SetRuntimeValue(ctx, orgResolver, serpent.StringOf(ptr.Ref("hello organizations")))
	require.NoError(t, err)
	// Verify org runtime
	stringVal, err = entries.String.Coalesce(ctx, orgResolver)
	require.NoError(t, err)
	require.Equal(t, "hello organizations", stringVal.String())
	// Unset org runtime
	err = entries.String.UnsetRuntimeValue(ctx, orgResolver)
	require.NoError(t, err)
	// Verify org runtime is back to default
	stringVal, err = entries.String.Coalesce(ctx, orgResolver)
	require.NoError(t, err)
	require.Equal(t, "default", stringVal.String())
}

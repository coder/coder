package runtimeconfig_test

import (
	"testing"

	"github.com/coder/serpent"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/runtimeconfig"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

// TestConfig demonstrates creating org-level overrides for deployment-level settings.
func TestConfig(t *testing.T) {
	t.Parallel()

	vals := coderdtest.DeploymentValues(t)
	vals.Experiments = []string{string(codersdk.ExperimentMultiOrganization)}
	adminClient, _, _, _ := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
		Options: &coderdtest.Options{DeploymentValues: vals},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureMultipleOrganizations: 1,
			},
		},
	})
	altOrg := coderdenttest.CreateOrganization(t, adminClient, coderdenttest.CreateOrganizationOptions{})

	t.Run("panics unless initialized", func(t *testing.T) {
		t.Parallel()

		field := runtimeconfig.Entry[*serpent.String]{}
		require.Panics(t, func() {
			field.StartupValue().String()
		})

		field.Init("my-field")
		require.NotPanics(t, func() {
			field.StartupValue().String()
		})
	})

	t.Run("simple", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		store := runtimeconfig.NewInMemoryStore()
		resolver := runtimeconfig.NewOrgResolver(altOrg.ID, runtimeconfig.NewStoreResolver(store))
		mutator := runtimeconfig.NewOrgMutator(altOrg.ID, runtimeconfig.NewStoreMutator(store))

		var (
			base     = serpent.String("system@dev.coder.com")
			override = serpent.String("dogfood@dev.coder.com")
		)

		field := runtimeconfig.Entry[*serpent.String]{}
		field.Init("my-field")
		// Check that no default has been set.
		require.Empty(t, field.StartupValue().String())
		// Initialize the value.
		require.NoError(t, field.Set(base.String()))
		// Validate that it returns that value.
		require.Equal(t, base.String(), field.String())
		// Validate that there is no org-level override right now.
		_, err := field.Resolve(ctx, resolver)
		require.ErrorIs(t, err, runtimeconfig.EntryNotFound)
		// Coalesce returns the deployment-wide value.
		val, err := field.Coalesce(ctx, resolver)
		require.NoError(t, err)
		require.Equal(t, base.String(), val.String())
		// Set an org-level override.
		require.NoError(t, field.Save(ctx, mutator, &override))
		// Coalesce now returns the org-level value.
		val, err = field.Coalesce(ctx, resolver)
		require.NoError(t, err)
		require.Equal(t, override.String(), val.String())
	})

	t.Run("complex", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		store := runtimeconfig.NewInMemoryStore()
		resolver := runtimeconfig.NewOrgResolver(altOrg.ID, runtimeconfig.NewStoreResolver(store))
		mutator := runtimeconfig.NewOrgMutator(altOrg.ID, runtimeconfig.NewStoreMutator(store))

		field := runtimeconfig.Entry[*serpent.Struct[map[string]string]]{}
		field.Init("my-field")
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

		// Check that no default has been set.
		require.Empty(t, field.StartupValue().Value)
		// Initialize the value.
		require.NoError(t, field.Set(base.String()))
		// Validate that there is no org-level override right now.
		_, err := field.Resolve(false, resolver)
		require.ErrorIs(t, err, runtimeconfig.EntryNotFound)
		// Coalesce returns the deployment-wide value.
		val, err := field.Coalesce(false, resolver)
		require.NoError(t, err)
		require.Equal(t, base.Value, val.Value)
		// Set an org-level override.
		require.NoError(t, field.Save(ctx, mutator, &override))
		// Coalesce now returns the org-level value.
		structVal, err := field.Resolve(false, resolver)
		require.NoError(t, err)
		require.Equal(t, override.Value, structVal.Value)
	})
}

package coderd_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

// setupOrganizationAIModelPricesTest returns an owner client, a store for
// seeding providers, the default organization, and a client for an admin of
// that organization.
func setupOrganizationAIModelPricesTest(t *testing.T) (*codersdk.Client, database.Store, uuid.UUID, *codersdk.Client) {
	t.Helper()

	ownerClient, db, owner := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
				codersdk.FeatureAIBridge:     1,
			},
		},
	})
	adminClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.ScopedRoleOrgAdmin(owner.OrganizationID))
	return ownerClient, db, owner.OrganizationID, adminClient
}

func newOrganizationAIModelPrice(providerID uuid.UUID, model string, input int64) codersdk.OrganizationAIModelPriceUpsert {
	return codersdk.OrganizationAIModelPriceUpsert{
		ProviderID: providerID,
		Model:      model,
		InputPrice: &input,
	}
}

// requireSDKError asserts err is an API error with the given status, and
// returns it for further checks.
func requireSDKError(t *testing.T, err error, status int) *codersdk.Error {
	t.Helper()
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, status, sdkErr.StatusCode())
	return sdkErr
}

func TestOrganizationAIModelPrices(t *testing.T) {
	t.Parallel()

	t.Run("OrgAdminManagesPrices", func(t *testing.T) {
		t.Parallel()

		ownerClient, db, orgID, adminClient := setupOrganizationAIModelPricesTest(t)
		anthropic := dbgen.AIProvider(t, db, database.AIProvider{Type: database.AIProviderTypeAnthropic})
		gateway := dbgen.AIProvider(t, db, database.AIProvider{Type: database.AIProviderTypeOpenaiCompat})
		ctx := testutil.Context(t, testutil.WaitLong)
		exp := codersdk.NewExperimentalClient(adminClient)

		// Given: organization prices for an anthropic provider and for an
		// openai-compat gateway.
		err := exp.UpsertOrganizationAIModelPrices(ctx, orgID, codersdk.UpsertOrganizationAIModelPricesRequest{
			Prices: []codersdk.OrganizationAIModelPriceUpsert{
				newOrganizationAIModelPrice(anthropic.ID, "org-model", 100),
				newOrganizationAIModelPrice(gateway.ID, "bedrock/claude-sonnet-4-5", 200),
			},
		})
		require.NoError(t, err)

		// Then: the organization lists both, scoped to it and its providers.
		prices, err := exp.ListOrganizationAIModelPrices(ctx, orgID, codersdk.AIModelPricesFilter{Model: "org-model"})
		require.NoError(t, err)
		require.Len(t, prices, 1)
		require.Equal(t, codersdk.AIModelPriceScopeOrganizationProvider, prices[0].Scope)
		require.Equal(t, &orgID, prices[0].OrganizationID)
		require.Equal(t, &anthropic.ID, prices[0].ProviderID)
		require.Equal(t, "anthropic", prices[0].Provider)
		require.Equal(t, new(int64(100)), prices[0].InputPrice)
		require.Equal(t, codersdk.AIModelPriceSourceCustom, prices[0].Source)

		prices, err = exp.ListOrganizationAIModelPrices(ctx, orgID, codersdk.AIModelPricesFilter{Provider: "openai-compat"})
		require.NoError(t, err)
		require.Len(t, prices, 1)
		require.Equal(t, codersdk.AIModelPriceScopeOrganizationProvider, prices[0].Scope)
		require.Equal(t, &gateway.ID, prices[0].ProviderID)
		require.Equal(t, gateway.Name, prices[0].ProviderName)
		require.Equal(t, "bedrock/claude-sonnet-4-5", prices[0].Model)
		require.Equal(t, new(int64(200)), prices[0].InputPrice)

		// And: the site-wide listing is unchanged.
		//nolint:gocritic // Reading site-wide prices is owner-only.
		sitePrices, err := codersdk.NewExperimentalClient(ownerClient).ListAIModelPrices(ctx, codersdk.AIModelPricesFilter{Model: "org-model"})
		require.NoError(t, err)
		require.Empty(t, sitePrices)

		// When: the org admin deletes both prices by their keys. The model
		// name holds a slash, which the query string carries intact.
		require.NoError(t, exp.DeleteOrganizationAIModelPrice(ctx, orgID, codersdk.OrganizationAIModelPriceKey{
			ProviderID: anthropic.ID, Model: "org-model",
		}))
		require.NoError(t, exp.DeleteOrganizationAIModelPrice(ctx, orgID, codersdk.OrganizationAIModelPriceKey{
			ProviderID: gateway.ID, Model: "bedrock/claude-sonnet-4-5",
		}))

		// Then: they are gone, and deleting again finds nothing.
		prices, err = exp.ListOrganizationAIModelPrices(ctx, orgID, codersdk.AIModelPricesFilter{Source: codersdk.AIModelPriceSourceFilterCustom})
		require.NoError(t, err)
		require.Empty(t, prices)
		err = exp.DeleteOrganizationAIModelPrice(ctx, orgID, codersdk.OrganizationAIModelPriceKey{
			ProviderID: anthropic.ID, Model: "org-model",
		})
		requireSDKError(t, err, http.StatusNotFound)
	})

	t.Run("ListsProviderPriceWithSitePrice", func(t *testing.T) {
		t.Parallel()

		ownerClient, db, orgID, adminClient := setupOrganizationAIModelPricesTest(t)
		provider := dbgen.AIProvider(t, db, database.AIProvider{Type: database.AIProviderTypeAnthropic})
		ctx := testutil.Context(t, testutil.WaitLong)
		exp := codersdk.NewExperimentalClient(adminClient)

		// Given: a site-wide custom price for the provider type, and an
		// organization price for one provider of that type.
		//nolint:gocritic // Setting site-wide prices is owner-only.
		require.NoError(t, codersdk.NewExperimentalClient(ownerClient).UpsertAIModelPrices(ctx, codersdk.UpsertAIModelPricesRequest{
			Prices: []codersdk.AIModelPriceUpsert{newAIModelPrice("anthropic", "shared-model", 1)},
		}))
		require.NoError(t, exp.UpsertOrganizationAIModelPrices(ctx, orgID, codersdk.UpsertOrganizationAIModelPricesRequest{
			Prices: []codersdk.OrganizationAIModelPriceUpsert{newOrganizationAIModelPrice(provider.ID, "shared-model", 2)},
		}))

		// Then: both are listed, since other providers of the type still use
		// the site-wide price.
		prices, err := exp.ListOrganizationAIModelPrices(ctx, orgID, codersdk.AIModelPricesFilter{Model: "shared-model"})
		require.NoError(t, err)
		require.Len(t, prices, 2)
		require.Equal(t, codersdk.AIModelPriceScopeSite, prices[0].Scope)
		require.Nil(t, prices[0].OrganizationID)
		require.Equal(t, new(int64(1)), prices[0].InputPrice)
		require.Equal(t, codersdk.AIModelPriceScopeOrganizationProvider, prices[1].Scope)
		require.Equal(t, new(int64(2)), prices[1].InputPrice)

		// And: deleting the organization price leaves the site-wide price.
		require.NoError(t, exp.DeleteOrganizationAIModelPrice(ctx, orgID, codersdk.OrganizationAIModelPriceKey{
			ProviderID: provider.ID, Model: "shared-model",
		}))
		prices, err = exp.ListOrganizationAIModelPrices(ctx, orgID, codersdk.AIModelPricesFilter{Model: "shared-model"})
		require.NoError(t, err)
		require.Len(t, prices, 1)
		require.Equal(t, codersdk.AIModelPriceScopeSite, prices[0].Scope)
	})

	t.Run("HidesPricesForDeletedProviders", func(t *testing.T) {
		t.Parallel()

		_, db, orgID, adminClient := setupOrganizationAIModelPricesTest(t)
		provider := dbgen.AIProvider(t, db, database.AIProvider{Type: database.AIProviderTypeAnthropic})
		ctx := testutil.Context(t, testutil.WaitLong)
		exp := codersdk.NewExperimentalClient(adminClient)

		require.NoError(t, exp.UpsertOrganizationAIModelPrices(ctx, orgID, codersdk.UpsertOrganizationAIModelPricesRequest{
			Prices: []codersdk.OrganizationAIModelPriceUpsert{newOrganizationAIModelPrice(provider.ID, "provider-model", 1)},
		}))
		require.NoError(t, db.DeleteAIProviderByID(dbauthz.AsSystemRestricted(ctx), provider.ID))

		prices, err := exp.ListOrganizationAIModelPrices(ctx, orgID, codersdk.AIModelPricesFilter{Model: "provider-model"})
		require.NoError(t, err)
		require.Empty(t, prices)
	})

	t.Run("Forbidden", func(t *testing.T) {
		t.Parallel()

		// Given: an organization member who is not an admin.
		ownerClient, _, orgID, _ := setupOrganizationAIModelPricesTest(t)
		memberClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, orgID)
		ctx := testutil.Context(t, testutil.WaitLong)
		exp := codersdk.NewExperimentalClient(memberClient)

		// Then: they can neither read nor change the organization's prices.
		_, err := exp.ListOrganizationAIModelPrices(ctx, orgID, codersdk.AIModelPricesFilter{})
		requireSDKError(t, err, http.StatusForbidden)
		err = exp.UpsertOrganizationAIModelPrices(ctx, orgID, codersdk.UpsertOrganizationAIModelPricesRequest{
			Prices: []codersdk.OrganizationAIModelPriceUpsert{newOrganizationAIModelPrice(uuid.New(), "my-model", 1)},
		})
		requireSDKError(t, err, http.StatusForbidden)
		err = exp.DeleteOrganizationAIModelPrice(ctx, orgID, codersdk.OrganizationAIModelPriceKey{
			ProviderID: uuid.New(), Model: "my-model",
		})
		requireSDKError(t, err, http.StatusForbidden)
	})

	t.Run("OtherOrganization", func(t *testing.T) {
		t.Parallel()

		// Given: an organization the org admin does not belong to.
		_, db, _, adminClient := setupOrganizationAIModelPricesTest(t)
		other := dbgen.Organization(t, db, database.Organization{})
		ctx := testutil.Context(t, testutil.WaitLong)
		exp := codersdk.NewExperimentalClient(adminClient)

		// Then: its prices are out of reach.
		_, err := exp.ListOrganizationAIModelPrices(ctx, other.ID, codersdk.AIModelPricesFilter{})
		requireSDKError(t, err, http.StatusNotFound)
		err = exp.UpsertOrganizationAIModelPrices(ctx, other.ID, codersdk.UpsertOrganizationAIModelPricesRequest{
			Prices: []codersdk.OrganizationAIModelPriceUpsert{newOrganizationAIModelPrice(uuid.New(), "my-model", 1)},
		})
		requireSDKError(t, err, http.StatusNotFound)
	})

	t.Run("RejectsInvalidPrices", func(t *testing.T) {
		t.Parallel()

		_, db, orgID, adminClient := setupOrganizationAIModelPricesTest(t)
		anthropic := dbgen.AIProvider(t, db, database.AIProvider{Type: database.AIProviderTypeAnthropic})
		deleted := dbgen.AIProvider(t, db, database.AIProvider{Type: database.AIProviderTypeAnthropic})
		ctx := testutil.Context(t, testutil.WaitLong)
		require.NoError(t, db.DeleteAIProviderByID(dbauthz.AsSystemRestricted(ctx), deleted.ID))
		exp := codersdk.NewExperimentalClient(adminClient)
		unknownID := uuid.New()

		tests := []struct {
			name      string
			prices    []codersdk.OrganizationAIModelPriceUpsert
			wantField string
		}{
			{
				name:      "NoProviderID",
				prices:    []codersdk.OrganizationAIModelPriceUpsert{newOrganizationAIModelPrice(uuid.Nil, "my-model", 1)},
				wantField: "prices[0].provider_id",
			},
			{
				name:      "UnknownProviderID",
				prices:    []codersdk.OrganizationAIModelPriceUpsert{newOrganizationAIModelPrice(unknownID, "my-model", 1)},
				wantField: "prices[0].provider_id",
			},
			{
				name:      "DeletedProviderID",
				prices:    []codersdk.OrganizationAIModelPriceUpsert{newOrganizationAIModelPrice(deleted.ID, "my-model", 1)},
				wantField: "prices[0].provider_id",
			},
			{
				name:      "NoModel",
				prices:    []codersdk.OrganizationAIModelPriceUpsert{newOrganizationAIModelPrice(anthropic.ID, "", 1)},
				wantField: "prices[0].model",
			},
			{
				name:      "NoPrice",
				prices:    []codersdk.OrganizationAIModelPriceUpsert{{ProviderID: anthropic.ID, Model: "my-model"}},
				wantField: "prices[0]",
			},
			{
				name:      "NegativePrice",
				prices:    []codersdk.OrganizationAIModelPriceUpsert{newOrganizationAIModelPrice(anthropic.ID, "my-model", -1)},
				wantField: "prices[0].input_price",
			},
			{
				name: "DuplicateProviderID",
				prices: []codersdk.OrganizationAIModelPriceUpsert{
					newOrganizationAIModelPrice(anthropic.ID, "my-model", 1),
					newOrganizationAIModelPrice(anthropic.ID, "my-model", 2),
				},
				wantField: "prices[1]",
			},
			{
				name:      "Empty",
				prices:    nil,
				wantField: "prices",
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				ctx := testutil.Context(t, testutil.WaitLong)
				err := exp.UpsertOrganizationAIModelPrices(ctx, orgID, codersdk.UpsertOrganizationAIModelPricesRequest{Prices: tt.prices})
				sdkErr := requireSDKError(t, err, http.StatusBadRequest)
				fields := make([]string, 0, len(sdkErr.Validations))
				for _, v := range sdkErr.Validations {
					fields = append(fields, v.Field)
				}
				require.Contains(t, fields, tt.wantField)
			})
		}
	})

	t.Run("WritesNothingWhenAnyEntryIsInvalid", func(t *testing.T) {
		t.Parallel()

		_, db, orgID, adminClient := setupOrganizationAIModelPricesTest(t)
		provider := dbgen.AIProvider(t, db, database.AIProvider{Type: database.AIProviderTypeAnthropic})
		ctx := testutil.Context(t, testutil.WaitLong)
		exp := codersdk.NewExperimentalClient(adminClient)

		// A valid entry, followed by one for a provider that does not exist.
		err := exp.UpsertOrganizationAIModelPrices(ctx, orgID, codersdk.UpsertOrganizationAIModelPricesRequest{
			Prices: []codersdk.OrganizationAIModelPriceUpsert{
				newOrganizationAIModelPrice(provider.ID, "valid-model", 1),
				newOrganizationAIModelPrice(uuid.New(), "valid-model", 2),
			},
		})
		requireSDKError(t, err, http.StatusBadRequest)

		prices, err := exp.ListOrganizationAIModelPrices(ctx, orgID, codersdk.AIModelPricesFilter{Model: "valid-model"})
		require.NoError(t, err)
		require.Empty(t, prices)
	})

	t.Run("RejectsInvalidDeleteKeys", func(t *testing.T) {
		t.Parallel()

		_, _, orgID, adminClient := setupOrganizationAIModelPricesTest(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		exp := codersdk.NewExperimentalClient(adminClient)
		providerID := uuid.New()

		for _, key := range []codersdk.OrganizationAIModelPriceKey{
			// No model.
			{ProviderID: providerID},
			// No provider ID.
			{Model: "my-model"},
		} {
			err := exp.DeleteOrganizationAIModelPrice(ctx, orgID, key)
			requireSDKError(t, err, http.StatusBadRequest)
		}
	})

	t.Run("RejectsAnUnknownListFilter", func(t *testing.T) {
		t.Parallel()

		_, _, orgID, adminClient := setupOrganizationAIModelPricesTest(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		exp := codersdk.NewExperimentalClient(adminClient)

		_, err := exp.ListOrganizationAIModelPrices(ctx, orgID, codersdk.AIModelPricesFilter{Provider: "not-a-provider"})
		requireSDKError(t, err, http.StatusBadRequest)
		_, err = exp.ListOrganizationAIModelPrices(ctx, orgID, codersdk.AIModelPricesFilter{Source: "not-a-source"})
		requireSDKError(t, err, http.StatusBadRequest)
	})
}

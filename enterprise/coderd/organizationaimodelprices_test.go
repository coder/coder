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

func newOrganizationAIModelPrice(providerID uuid.UUID, model string, input int64) codersdk.UpsertOrganizationAIModelPriceRequest {
	return codersdk.UpsertOrganizationAIModelPriceRequest{
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
		anthropicKey := codersdk.OrganizationAIModelPriceKey{ProviderID: anthropic.ID, Model: "org-model"}
		// The model name holds a slash, which the query string carries intact.
		gatewayKey := codersdk.OrganizationAIModelPriceKey{ProviderID: gateway.ID, Model: "bedrock/claude-sonnet-4-5"}

		// Given: organization prices for an anthropic provider and for an
		// openai-compat gateway.
		require.NoError(t, exp.UpsertOrganizationAIModelPrice(ctx, orgID, newOrganizationAIModelPrice(anthropicKey.ProviderID, anthropicKey.Model, 100)))
		require.NoError(t, exp.UpsertOrganizationAIModelPrice(ctx, orgID, newOrganizationAIModelPrice(gatewayKey.ProviderID, gatewayKey.Model, 200)))

		// Then: the organization reads each by its key.
		price, err := exp.GetOrganizationAIModelPrice(ctx, orgID, anthropicKey)
		require.NoError(t, err)
		require.Equal(t, orgID, price.OrganizationID)
		require.Equal(t, anthropic.ID, price.ProviderID)
		require.Equal(t, "org-model", price.Model)
		require.Equal(t, new(int64(100)), price.InputPrice)
		require.Nil(t, price.OutputPrice)

		price, err = exp.GetOrganizationAIModelPrice(ctx, orgID, gatewayKey)
		require.NoError(t, err)
		require.Equal(t, gateway.ID, price.ProviderID)
		require.Equal(t, "bedrock/claude-sonnet-4-5", price.Model)
		require.Equal(t, new(int64(200)), price.InputPrice)

		// And: the site-wide listing is unchanged.
		//nolint:gocritic // Reading site-wide prices is owner-only.
		sitePrices, err := codersdk.NewExperimentalClient(ownerClient).ListAIModelPrices(ctx, codersdk.AIModelPricesFilter{Model: "org-model"})
		require.NoError(t, err)
		require.Empty(t, sitePrices)

		// When: the org admin deletes both prices by their keys.
		require.NoError(t, exp.DeleteOrganizationAIModelPrice(ctx, orgID, anthropicKey))
		require.NoError(t, exp.DeleteOrganizationAIModelPrice(ctx, orgID, gatewayKey))

		// Then: they are gone, and deleting again finds nothing.
		_, err = exp.GetOrganizationAIModelPrice(ctx, orgID, anthropicKey)
		requireSDKError(t, err, http.StatusNotFound)
		_, err = exp.GetOrganizationAIModelPrice(ctx, orgID, gatewayKey)
		requireSDKError(t, err, http.StatusNotFound)
		err = exp.DeleteOrganizationAIModelPrice(ctx, orgID, anthropicKey)
		requireSDKError(t, err, http.StatusNotFound)
	})

	t.Run("DeleteLeavesSitePrice", func(t *testing.T) {
		t.Parallel()

		ownerClient, db, orgID, adminClient := setupOrganizationAIModelPricesTest(t)
		provider := dbgen.AIProvider(t, db, database.AIProvider{Type: database.AIProviderTypeAnthropic})
		ctx := testutil.Context(t, testutil.WaitLong)
		exp := codersdk.NewExperimentalClient(adminClient)
		//nolint:gocritic // Site-wide prices are owner-only.
		site := codersdk.NewExperimentalClient(ownerClient)
		key := codersdk.OrganizationAIModelPriceKey{ProviderID: provider.ID, Model: "shared-model"}

		// Given: a site-wide custom price for the provider type, and an
		// organization price for one provider of that type.
		require.NoError(t, site.UpsertAIModelPrices(ctx, codersdk.UpsertAIModelPricesRequest{
			Prices: []codersdk.AIModelPriceUpsert{newAIModelPrice("anthropic", "shared-model", 1)},
		}))
		require.NoError(t, exp.UpsertOrganizationAIModelPrice(ctx, orgID, newOrganizationAIModelPrice(provider.ID, "shared-model", 2)))

		// Then: the organization reads its own price, not the site-wide one.
		price, err := exp.GetOrganizationAIModelPrice(ctx, orgID, key)
		require.NoError(t, err)
		require.Equal(t, new(int64(2)), price.InputPrice)

		// And: deleting the organization price leaves the site-wide price.
		require.NoError(t, exp.DeleteOrganizationAIModelPrice(ctx, orgID, key))
		sitePrices, err := site.ListAIModelPrices(ctx, codersdk.AIModelPricesFilter{Model: "shared-model"})
		require.NoError(t, err)
		require.Len(t, sitePrices, 1)
		require.Equal(t, new(int64(1)), sitePrices[0].InputPrice)
	})

	t.Run("Forbidden", func(t *testing.T) {
		t.Parallel()

		// Given: an organization member who is not an admin.
		ownerClient, _, orgID, _ := setupOrganizationAIModelPricesTest(t)
		memberClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, orgID)
		ctx := testutil.Context(t, testutil.WaitLong)
		exp := codersdk.NewExperimentalClient(memberClient)
		key := codersdk.OrganizationAIModelPriceKey{ProviderID: uuid.New(), Model: "my-model"}

		// Then: they can neither read nor change the organization's prices.
		_, err := exp.GetOrganizationAIModelPrice(ctx, orgID, key)
		requireSDKError(t, err, http.StatusForbidden)
		err = exp.UpsertOrganizationAIModelPrice(ctx, orgID, newOrganizationAIModelPrice(key.ProviderID, key.Model, 1))
		requireSDKError(t, err, http.StatusForbidden)
		err = exp.DeleteOrganizationAIModelPrice(ctx, orgID, key)
		requireSDKError(t, err, http.StatusForbidden)
	})

	t.Run("OtherOrganization", func(t *testing.T) {
		t.Parallel()

		// Given: an organization the org admin does not belong to.
		_, db, _, adminClient := setupOrganizationAIModelPricesTest(t)
		other := dbgen.Organization(t, db, database.Organization{})
		ctx := testutil.Context(t, testutil.WaitLong)
		exp := codersdk.NewExperimentalClient(adminClient)
		key := codersdk.OrganizationAIModelPriceKey{ProviderID: uuid.New(), Model: "my-model"}

		// Then: its prices are out of reach.
		_, err := exp.GetOrganizationAIModelPrice(ctx, other.ID, key)
		requireSDKError(t, err, http.StatusNotFound)
		err = exp.UpsertOrganizationAIModelPrice(ctx, other.ID, newOrganizationAIModelPrice(key.ProviderID, key.Model, 1))
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
			price     codersdk.UpsertOrganizationAIModelPriceRequest
			wantField string
		}{
			{name: "NoProviderID", price: newOrganizationAIModelPrice(uuid.Nil, "my-model", 1), wantField: "provider_id"},
			{name: "UnknownProviderID", price: newOrganizationAIModelPrice(unknownID, "my-model", 1), wantField: "provider_id"},
			{name: "DeletedProviderID", price: newOrganizationAIModelPrice(deleted.ID, "my-model", 1), wantField: "provider_id"},
			{name: "NoModel", price: newOrganizationAIModelPrice(anthropic.ID, "", 1), wantField: "model"},
			{name: "NoPrice", price: codersdk.UpsertOrganizationAIModelPriceRequest{ProviderID: anthropic.ID, Model: "my-model"}, wantField: ""},
			{name: "NegativePrice", price: newOrganizationAIModelPrice(anthropic.ID, "my-model", -1), wantField: "input_price"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				ctx := testutil.Context(t, testutil.WaitLong)
				err := exp.UpsertOrganizationAIModelPrice(ctx, orgID, tt.price)
				sdkErr := requireSDKError(t, err, http.StatusBadRequest)
				fields := make([]string, 0, len(sdkErr.Validations))
				for _, v := range sdkErr.Validations {
					fields = append(fields, v.Field)
				}
				require.Contains(t, fields, tt.wantField)
			})
		}
	})

	t.Run("RejectsInvalidKeys", func(t *testing.T) {
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
			_, err := exp.GetOrganizationAIModelPrice(ctx, orgID, key)
			requireSDKError(t, err, http.StatusBadRequest)
			err = exp.DeleteOrganizationAIModelPrice(ctx, orgID, key)
			requireSDKError(t, err, http.StatusBadRequest)
		}
	})
}

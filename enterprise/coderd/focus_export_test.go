package coderd_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/parquet-go/parquet-go"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/aibridge/focus"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
)

// requestAIFocusExport issues a raw GET against the experimental FOCUS export
// endpoint. There is no typed SDK method for it, since the route is
// experimental (Phase One Draft, HTTP handler).
func requestAIFocusExport(ctx context.Context, t *testing.T, client *codersdk.Client, orgID uuid.UUID, params map[string]string) *http.Response {
	t.Helper()
	res, err := client.Request(ctx, http.MethodGet,
		fmt.Sprintf("/api/experimental/organizations/%s/ai/spend/export/focus", orgID),
		nil,
		func(r *http.Request) {
			q := r.URL.Query()
			for k, v := range params {
				q.Set(k, v)
			}
			r.URL.RawQuery = q.Encode()
		},
	)
	require.NoError(t, err)
	return res
}

type focusExportTestOptions struct {
	ExperimentEnabled bool
	Features          license.Features
	Database          database.Store
}

// setupAIFocusExportTest builds a deployment with FeatureAIBridge licensed
// (unless overridden) and the focus-export experiment enabled (unless
// overridden), returning an org admin client, a regular member client, and
// the organization ID.
func setupAIFocusExportTest(t *testing.T, opts focusExportTestOptions) (admin *codersdk.Client, member *codersdk.Client, orgID uuid.UUID) {
	t.Helper()

	dv := coderdtest.DeploymentValues(t)
	dv.AI.BridgeConfig.Enabled = serpent.Bool(true)
	if opts.ExperimentEnabled {
		dv.Experiments = []string{string(codersdk.ExperimentFOCUSExport)}
	}
	features := opts.Features
	if features == nil {
		features = license.Features{
			codersdk.FeatureTemplateRBAC: 1,
			codersdk.FeatureAIBridge:     1,
		}
	}
	coderdOpts := &coderdtest.Options{DeploymentValues: dv}
	if opts.Database != nil {
		coderdOpts.Database = opts.Database
	}
	ownerClient, owner := coderdenttest.New(t, &coderdenttest.Options{
		Options:        coderdOpts,
		LicenseOptions: &coderdenttest.LicenseOptions{Features: features},
	})
	adminClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleUserAdmin())
	memberClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)
	return adminClient, memberClient, owner.OrganizationID
}

func TestExportOrganizationAIFocus(t *testing.T) {
	t.Parallel()

	t.Run("DisabledByDefault", func(t *testing.T) {
		t.Parallel()
		admin, _, orgID := setupAIFocusExportTest(t, focusExportTestOptions{ExperimentEnabled: false})
		ctx := testutil.Context(t, testutil.WaitLong)

		res := requestAIFocusExport(ctx, t, admin, orgID, nil)
		defer res.Body.Close()
		require.Equal(t, http.StatusForbidden, res.StatusCode)
	})

	t.Run("RequiresLicenseFeature", func(t *testing.T) {
		t.Parallel()
		admin, _, orgID := setupAIFocusExportTest(t, focusExportTestOptions{
			ExperimentEnabled: true,
			Features:          license.Features{},
		})
		ctx := testutil.Context(t, testutil.WaitLong)

		res := requestAIFocusExport(ctx, t, admin, orgID, nil)
		defer res.Body.Close()
		require.Equal(t, http.StatusForbidden, res.StatusCode)
	})

	t.Run("RequiresOrgRead", func(t *testing.T) {
		t.Parallel()
		_, member, orgID := setupAIFocusExportTest(t, focusExportTestOptions{ExperimentEnabled: true})
		ctx := testutil.Context(t, testutil.WaitLong)

		res := requestAIFocusExport(ctx, t, member, orgID, nil)
		defer res.Body.Close()
		require.Equal(t, http.StatusForbidden, res.StatusCode)
	})

	t.Run("InvalidFormat", func(t *testing.T) {
		t.Parallel()
		admin, _, orgID := setupAIFocusExportTest(t, focusExportTestOptions{ExperimentEnabled: true})
		ctx := testutil.Context(t, testutil.WaitLong)

		res := requestAIFocusExport(ctx, t, admin, orgID, map[string]string{"format": "xml"})
		defer res.Body.Close()
		require.Equal(t, http.StatusBadRequest, res.StatusCode)
	})

	t.Run("InvalidGranularity", func(t *testing.T) {
		t.Parallel()
		admin, _, orgID := setupAIFocusExportTest(t, focusExportTestOptions{ExperimentEnabled: true})
		ctx := testutil.Context(t, testutil.WaitLong)

		res := requestAIFocusExport(ctx, t, admin, orgID, map[string]string{"granularity": "999999999"})
		defer res.Body.Close()
		require.Equal(t, http.StatusBadRequest, res.StatusCode)
	})

	t.Run("PeriodStartAndEndAreAccepted", func(t *testing.T) {
		// Regression test: aiSpendExportPeriod and this handler used to build
		// two independent query-param parsers, each rejecting the other's
		// accepted params as "excess", so period_start/period_end always
		// returned 400 alongside format/granularity.
		t.Parallel()
		admin, _, orgID := setupAIFocusExportTest(t, focusExportTestOptions{ExperimentEnabled: true})
		ctx := testutil.Context(t, testutil.WaitLong)

		now := time.Now().UTC()
		periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		periodEnd := periodStart.AddDate(0, 0, 1)

		for name, params := range map[string]map[string]string{
			"PeriodAlone": {
				"period_start": periodStart.Format(time.RFC3339),
				"period_end":   periodEnd.Format(time.RFC3339),
			},
			"PeriodWithFormat": {
				"period_start": periodStart.Format(time.RFC3339),
				"period_end":   periodEnd.Format(time.RFC3339),
				"format":       "csv",
			},
			"PeriodWithGranularity": {
				"period_start": periodStart.Format(time.RFC3339),
				"period_end":   periodEnd.Format(time.RFC3339),
				"granularity":  "0",
			},
			"PeriodWithFormatAndGranularity": {
				"period_start": periodStart.Format(time.RFC3339),
				"period_end":   periodEnd.Format(time.RFC3339),
				"format":       "parquet",
				"granularity":  "3600",
			},
		} {
			t.Run(name, func(t *testing.T) {
				res := requestAIFocusExport(ctx, t, admin, orgID, params)
				defer res.Body.Close()
				require.Equal(t, http.StatusOK, res.StatusCode)
			})
		}
	})

	t.Run("CrossOrgAdminCannotAccess", func(t *testing.T) {
		// The highest-value missing test: an admin of one organization must not
		// be able to read another organization's export by URL alone. This is
		// the actual proof of the "organization-level administrator
		// permissions" claim in the endpoint's docstring, which the
		// RequiresOrgRead/RequiresLicenseFeature/DisabledByDefault cases above
		// do not cover (they only prove a same-org member or a disabled
		// gate is rejected).
		t.Parallel()
		dv := coderdtest.DeploymentValues(t)
		dv.AI.BridgeConfig.Enabled = serpent.Bool(true)
		dv.Experiments = []string{string(codersdk.ExperimentFOCUSExport)}
		ownerClient, owner := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{DeploymentValues: dv},
			LicenseOptions: &coderdenttest.LicenseOptions{Features: license.Features{
				codersdk.FeatureTemplateRBAC:          1,
				codersdk.FeatureAIBridge:              1,
				codersdk.FeatureMultipleOrganizations: 1,
			}},
		})
		ctx := testutil.Context(t, testutil.WaitLong)

		// A second organization, with its own admin, distinct from owner's.
		//nolint:gocritic // must be an owner, only owners can create orgs
		otherOrg, err := ownerClient.CreateOrganization(ctx, codersdk.CreateOrganizationRequest{Name: "other-org"})
		require.NoError(t, err)
		otherOrgAdmin, _ := coderdtest.CreateAnotherUser(t, ownerClient, otherOrg.ID, rbac.RoleIdentifier{Name: rbac.RoleOrgAdmin(), OrganizationID: otherOrg.ID})

		// otherOrgAdmin is an admin, just not of owner's organization. Coder's
		// org-param middleware resolves "organization" through the
		// authorization-wrapped store, so an org the actor cannot read comes
		// back not-found rather than forbidden: this is the established,
		// intentional convention (it does not confirm or deny that the
		// organization exists to a non-member), matching every other
		// organization-scoped endpoint, not a gap specific to this one.
		res := requestAIFocusExport(ctx, t, otherOrgAdmin, owner.OrganizationID, nil)
		defer res.Body.Close()
		require.Equal(t, http.StatusNotFound, res.StatusCode)
	})

	t.Run("OrgAdminRoleIsSufficient", func(t *testing.T) {
		// setupAIFocusExportTest's "admin" client actually holds RoleUserAdmin,
		// a site-wide role, not the "organization-level administrator
		// permissions" the docstring documents. This proves the documented
		// role actually works, independent of RoleUserAdmin.
		t.Parallel()
		dv := coderdtest.DeploymentValues(t)
		dv.AI.BridgeConfig.Enabled = serpent.Bool(true)
		dv.Experiments = []string{string(codersdk.ExperimentFOCUSExport)}
		ownerClient, owner := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{DeploymentValues: dv},
			LicenseOptions: &coderdenttest.LicenseOptions{Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
				codersdk.FeatureAIBridge:     1,
			}},
		})
		ctx := testutil.Context(t, testutil.WaitLong)

		orgAdmin, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID,
			rbac.RoleIdentifier{Name: rbac.RoleOrgAdmin(), OrganizationID: owner.OrganizationID})

		res := requestAIFocusExport(ctx, t, orgAdmin, owner.OrganizationID, nil)
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
	})

	t.Run("HappyPathCSVAndParquet", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		admin, _, orgID := setupAIFocusExportTest(t, focusExportTestOptions{
			ExperimentEnabled: true,
			Database:          db,
		})
		ctx := testutil.Context(t, testutil.WaitLong)

		// The org's group membership signal (effective_group_id) must point at
		// a real group under orgID.
		group, err := admin.CreateGroup(ctx, orgID, codersdk.CreateGroupRequest{Name: "focus-export-group"})
		require.NoError(t, err)

		user, err := admin.User(ctx, codersdk.Me)
		require.NoError(t, err)
		_, err = admin.PatchGroup(ctx, group.ID, codersdk.PatchGroupRequest{AddUsers: []string{user.ID.String()}})
		require.NoError(t, err)

		provider := dbgen.AIProvider(t, db, database.AIProvider{Type: database.AIProviderTypeAnthropic, Name: "anthropic"})

		now := time.Now().UTC()
		periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		at := periodStart.Add(time.Hour)

		interception := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: user.ID, Provider: "anthropic", ProviderName: provider.Name, Model: "claude-haiku-4-5",
			StartedAt: at,
		}, ptrTo(at.Add(time.Second)))
		dbgen.AIBridgeTokenUsage(t, db, database.InsertAIBridgeTokenUsageParams{
			InterceptionID: interception.ID, InputTokens: 100, OutputTokens: 50,
			CreatedAt:        at.Add(time.Second),
			EffectiveGroupID: uuid.NullUUID{UUID: group.ID, Valid: true},
			InputPriceMicros: sql.NullInt64{Int64: 1_000_000, Valid: true},
		})

		t.Run("CSV", func(t *testing.T) {
			res := requestAIFocusExport(ctx, t, admin, orgID, map[string]string{"format": "csv", "granularity": "0"})
			defer res.Body.Close()
			require.Equal(t, http.StatusOK, res.StatusCode)
			require.NotEmpty(t, res.Header.Get("X-Coder-Focus-Checksum-Sha256"))
			require.Equal(t, focus.FocusVersion, res.Header.Get("X-Coder-Focus-Version"))
			require.Equal(t, focus.MappingVersion, res.Header.Get("X-Coder-Focus-Mapping-Version"))
			require.Equal(t, "0", res.Header.Get("X-Coder-Focus-Granularity-Seconds"))

			r := csv.NewReader(res.Body)
			records, err := r.ReadAll()
			require.NoError(t, err)
			require.Equal(t, focus.Header, records[0])
			require.Equal(t, "2", res.Header.Get("X-Coder-Focus-Row-Count"))
			require.Len(t, records, 3) // header + input + output rows
		})

		t.Run("Parquet", func(t *testing.T) {
			res := requestAIFocusExport(ctx, t, admin, orgID, map[string]string{"format": "parquet", "granularity": "0"})
			defer res.Body.Close()
			require.Equal(t, http.StatusOK, res.StatusCode)
			require.Equal(t, "application/vnd.apache.parquet", res.Header.Get("Content-Type"))

			var buf bytes.Buffer
			_, err := buf.ReadFrom(res.Body)
			require.NoError(t, err)
			rows, err := parquet.Read[focus.Row](bytes.NewReader(buf.Bytes()), int64(buf.Len()))
			require.NoError(t, err)
			require.Len(t, rows, 2)
		})

		t.Run("HourlyRollupFoldsToOneRow", func(t *testing.T) {
			res := requestAIFocusExport(ctx, t, admin, orgID, map[string]string{"format": "csv", "granularity": "3600"})
			defer res.Body.Close()
			require.Equal(t, http.StatusOK, res.StatusCode)

			r := csv.NewReader(res.Body)
			records, err := r.ReadAll()
			require.NoError(t, err)
			// Still fans out per non-zero token type (input + output), but
			// both from the single hourly bucket.
			require.Len(t, records, 3)
		})
	})
}

func ptrTo[T any](v T) *T { return &v }

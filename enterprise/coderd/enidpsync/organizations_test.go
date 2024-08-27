package enidpsync

import (
	"context"
	"testing"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/entitlements"
	"github.com/coder/coder/v2/coderd/idpsync"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

type ExpectedUser struct {
	SyncError     bool
	Organizations []uuid.UUID
}

type Expectations struct {
	Name   string
	Claims jwt.MapClaims
	// Parse
	ParseError     func(t *testing.T, httpErr *idpsync.HttpError)
	ExpectedParams idpsync.OrganizationParams
	// Mutate allows mutating the user before syncing
	Mutate func(t *testing.T, db database.Store, user database.User)
	Sync   ExpectedUser
}

type OrganizationSyncTestCase struct {
	Settings     idpsync.SyncSettings
	Entitlements *entitlements.Set
	Exps         []Expectations
}

func TestOrganizationSync(t *testing.T) {
	t.Parallel()

	if dbtestutil.WillUsePostgres() {
		t.Skip("Skipping test because it populates a lot of db entries, which is slow on postgres")
	}

	requireUserOrgs := func(t *testing.T, db database.Store, user database.User, expected []uuid.UUID) {
		t.Helper()

		// nolint:gocritic // in testing
		members, err := db.OrganizationMembers(dbauthz.AsSystemRestricted(context.Background()), database.OrganizationMembersParams{
			UserID: user.ID,
		})
		require.NoError(t, err)

		foundIDs := db2sdk.List(members, func(m database.OrganizationMembersRow) uuid.UUID {
			return m.OrganizationMember.OrganizationID
		})
		require.ElementsMatch(t, expected, foundIDs, "match user organizations")
	}

	entitled := entitlements.New()
	entitled.Update(func(entitlements *codersdk.Entitlements) {
		entitlements.Features[codersdk.FeatureMultipleOrganizations] = codersdk.Feature{
			Entitlement: codersdk.EntitlementEntitled,
			Enabled:     true,
			Limit:       nil,
			Actual:      nil,
		}
	})

	testCases := []struct {
		Name string
		Case func(t *testing.T, db database.Store) OrganizationSyncTestCase
	}{
		{
			Name: "SingleOrgDeployment",
			Case: func(t *testing.T, db database.Store) OrganizationSyncTestCase {
				def, _ := db.GetDefaultOrganization(context.Background())
				other := dbgen.Organization(t, db, database.Organization{})
				return OrganizationSyncTestCase{
					Entitlements: entitled,
					Settings: idpsync.SyncSettings{
						OrganizationField:         "",
						OrganizationMapping:       nil,
						OrganizationAssignDefault: true,
					},
					Exps: []Expectations{
						{
							Name:   "NoOrganizations",
							Claims: jwt.MapClaims{},
							ExpectedParams: idpsync.OrganizationParams{
								SyncEnabled:    false,
								IncludeDefault: true,
								Organizations:  []uuid.UUID{},
							},
							Sync: ExpectedUser{
								Organizations: []uuid.UUID{},
							},
						},
						{
							Name:   "AlreadyInOrgs",
							Claims: jwt.MapClaims{},
							ExpectedParams: idpsync.OrganizationParams{
								SyncEnabled:    false,
								IncludeDefault: true,
								Organizations:  []uuid.UUID{},
							},
							Mutate: func(t *testing.T, db database.Store, user database.User) {
								dbgen.OrganizationMember(t, db, database.OrganizationMember{
									UserID:         user.ID,
									OrganizationID: def.ID,
								})
								dbgen.OrganizationMember(t, db, database.OrganizationMember{
									UserID:         user.ID,
									OrganizationID: other.ID,
								})
							},
							Sync: ExpectedUser{
								Organizations: []uuid.UUID{def.ID, other.ID},
							},
						},
					},
				}
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitMedium)
			logger := slogtest.Make(t, &slogtest.Options{})

			rdb, _ := dbtestutil.NewDB(t)
			db := dbauthz.New(rdb, rbac.NewAuthorizer(prometheus.NewRegistry()), logger, coderdtest.AccessControlStorePointer())
			caseData := tc.Case(t, rdb)
			if caseData.Entitlements == nil {
				caseData.Entitlements = entitlements.New()
			}

			// Create a new sync object
			sync := NewSync(logger, caseData.Entitlements, caseData.Settings)
			for _, exp := range caseData.Exps {
				t.Run(exp.Name, func(t *testing.T) {
					params, httpErr := sync.ParseOrganizationClaims(ctx, exp.Claims)
					if exp.ParseError != nil {
						exp.ParseError(t, httpErr)
						return
					}

					require.Equal(t, exp.ExpectedParams.SyncEnabled, params.SyncEnabled, "match enabled")
					require.Equal(t, exp.ExpectedParams.IncludeDefault, params.IncludeDefault, "match include default")
					if exp.ExpectedParams.Organizations == nil {
						exp.ExpectedParams.Organizations = []uuid.UUID{}
					}
					require.ElementsMatch(t, exp.ExpectedParams.Organizations, params.Organizations, "match organizations")

					user := dbgen.User(t, db, database.User{})
					if exp.Mutate != nil {
						exp.Mutate(t, db, user)
					}

					err := sync.SyncOrganizations(ctx, db, user, params)
					if exp.Sync.SyncError {
						require.Error(t, err)
						return
					}
					requireUserOrgs(t, db, user, exp.Sync.Organizations)
				})
			}
		})
	}
}

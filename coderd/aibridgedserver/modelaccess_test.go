package aibridgedserver_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"

	"github.com/coder/coder/v2/coderd/aibridgedserver"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/entitlements"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
)

func TestResolveModelAccess(t *testing.T) {
	t.Parallel()
	queryErr := xerrors.New("database unavailable")
	for _, tc := range []struct {
		name     string
		grant    bool
		queryErr error
		allowed  bool
		reason   aibridgedserver.ModelAccessReason
	}{
		{name: "Allowed", grant: true, allowed: true, reason: aibridgedserver.ModelAccessAllowed},
		{name: "NoGrant", reason: aibridgedserver.ModelAccessDenied},
		{name: "QueryError", queryErr: queryErr, reason: aibridgedserver.ModelAccessError},
		{name: "UnverifiedGrant", grant: true, queryErr: queryErr, reason: aibridgedserver.ModelAccessError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db := dbmock.NewMockStore(gomock.NewController(t))
			userID := uuid.New()
			db.EXPECT().HasAIModelAccess(gomock.Any(), database.HasAIModelAccessParams{
				UserID: userID, ProviderName: "provider", Model: "model:version",
			}).Return(tc.grant, tc.queryErr)
			ents := entitlements.New()
			ents.Modify(func(e *codersdk.Entitlements) {
				e.Features[codersdk.FeatureMultipleOrganizations] = codersdk.Feature{Enabled: true}
			})
			srv, err := aibridgedserver.NewServer(t.Context(), aibridgedserver.Options{Store: db, Entitlements: ents})
			require.NoError(t, err)
			result, err := srv.ResolveModelAccess(t.Context(), userID, "provider", "model:version")
			if tc.queryErr != nil {
				require.ErrorIs(t, err, queryErr)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.allowed, result.Allowed)
			require.Equal(t, tc.reason, result.Reason)
		})
	}
}

func TestResolveModelAccessDatabase(t *testing.T) {
	t.Parallel()
	rawDB, _ := dbtestutil.NewDB(t)
	logger := slogtest.Make(t, nil)
	db := dbauthz.New(rawDB, rbac.NewStrictAuthorizer(prometheus.NewRegistry()), logger, coderdtest.AccessControlStorePointer())
	ents := entitlements.New()
	ents.Modify(func(e *codersdk.Entitlements) {
		e.Features[codersdk.FeatureMultipleOrganizations] = codersdk.Feature{Enabled: true}
	})
	srv, err := aibridgedserver.NewServer(t.Context(), aibridgedserver.Options{Store: db, Entitlements: ents, Logger: logger})
	require.NoError(t, err)
	user := dbgen.User(t, rawDB, database.User{})
	org := dbgen.Organization(t, rawDB, database.Organization{})
	dbgen.OrganizationMember(t, rawDB, database.OrganizationMember{UserID: user.ID, OrganizationID: org.ID})
	provider := dbgen.AIProvider(t, rawDB, database.AIProvider{})
	dbgen.ChatModelConfig(t, rawDB, database.ChatModelConfig{OrganizationID: org.ID, Model: "model", AIProviderID: uuid.NullUUID{UUID: provider.ID, Valid: true}})

	result, err := srv.ResolveModelAccess(t.Context(), user.ID, provider.Name, "model")
	require.NoError(t, err)
	require.True(t, result.Allowed)
	require.Equal(t, aibridgedserver.ModelAccessAllowed, result.Reason)
	result, err = srv.ResolveModelAccess(t.Context(), user.ID, provider.Name, "unknown")
	require.NoError(t, err)
	require.False(t, result.Allowed)

	defaultOrg, err := rawDB.GetDefaultOrganization(t.Context())
	require.NoError(t, err)
	dbgen.OrganizationMember(t, rawDB, database.OrganizationMember{UserID: user.ID, OrganizationID: defaultOrg.ID})
	result, err = srv.ResolveModelAccess(t.Context(), user.ID, provider.Name, "unknown")
	require.NoError(t, err)
	require.True(t, result.Allowed)
	require.Equal(t, aibridgedserver.ModelAccessAllowed, result.Reason)

	_, err = rawDB.UpdateOrganizationRestrictModelsToConfigured(t.Context(), database.UpdateOrganizationRestrictModelsToConfiguredParams{
		ID: defaultOrg.ID, RestrictModelsToConfigured: true, UpdatedAt: dbtime.Now(),
	})
	require.NoError(t, err)
	result, err = srv.ResolveModelAccess(t.Context(), user.ID, provider.Name, "unknown")
	require.NoError(t, err)
	require.False(t, result.Allowed)
}

func TestResolveModelAccessEntitlements(t *testing.T) {
	t.Parallel()
	db := dbmock.NewMockStore(gomock.NewController(t))
	ents := entitlements.New()
	srv, err := aibridgedserver.NewServer(t.Context(), aibridgedserver.Options{Store: db, Entitlements: ents})
	require.NoError(t, err)
	userID := uuid.New()
	for _, enabled := range []bool{false, true, false, true} {
		ents.Modify(func(e *codersdk.Entitlements) {
			e.Features[codersdk.FeatureMultipleOrganizations] = codersdk.Feature{Enabled: enabled}
			// AI Governance availability must not control model restrictions.
			e.Features[codersdk.FeatureAIBridge] = codersdk.Feature{Enabled: !enabled}
		})
		if enabled {
			db.EXPECT().HasAIModelAccess(gomock.Any(), gomock.Any()).Return(false, nil)
		}
		result, err := srv.ResolveModelAccess(t.Context(), userID, "provider", "model")
		require.NoError(t, err)
		require.Equal(t, !enabled, result.Allowed)
		if !enabled {
			require.Equal(t, aibridgedserver.ModelAccessNotEntitled, result.Reason)
		}
	}
}

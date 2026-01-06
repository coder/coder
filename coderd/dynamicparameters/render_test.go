package dynamicparameters_test

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/dynamicparameters"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/testutil"
)

func TestProvisionerVersionSupportsDynamicParameters(t *testing.T) {
	t.Parallel()

	for v, dyn := range map[string]bool{
		"":     false,
		"na":   false,
		"0.0":  false,
		"0.10": false,
		"1.4":  false,
		"1.5":  false,
		"1.6":  true,
		"1.7":  true,
		"1.8":  true,
		"2.0":  true,
		"2.17": true,
		"4.0":  true,
	} {
		t.Run(v, func(t *testing.T) {
			t.Parallel()

			does := dynamicparameters.ProvisionerVersionSupportsDynamicParameters(v)
			require.Equal(t, dyn, does)
		})
	}
}

// TestWorkspaceOwnerFallbackAuthorization verifies that the WorkspaceOwner
// function succeeds even when the caller cannot directly read the workspace
// owner's user record. This happens for shared workspace users who have
// permission to operate on a workspace but not to read other users' data.
func TestWorkspaceOwnerFallbackAuthorization(t *testing.T) {
	t.Parallel()
	db, _ := dbtestutil.NewDB(t)
	org := dbgen.Organization(t, db, database.Organization{})

	owner := dbgen.User(t, db, database.User{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		OrganizationID: org.ID,
		UserID:         owner.ID,
	})

	sharedUser := dbgen.User(t, db, database.User{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		OrganizationID: org.ID,
		UserID:         sharedUser.ID,
	})

	authz := rbac.NewAuthorizer(prometheus.NewRegistry())
	authzDB := dbauthz.New(db, authz, testutil.Logger(t), coderdtest.AccessControlStorePointer())

	// minimalSubject simulates a shared workspace user.
	minimalSubject := rbac.Subject{
		ID:    sharedUser.ID.String(),
		Roles: rbac.RoleIdentifiers{rbac.RoleMember()},
		Scope: rbac.ScopeAll,
	}
	ctx := dbauthz.As(testutil.Context(t, testutil.WaitShort), minimalSubject)

	result, err := dynamicparameters.WorkspaceOwner(ctx, authzDB, org.ID, owner.ID)
	require.NoError(t, err, "WorkspaceOwner should succeed via fallback path")
	require.NotNil(t, result)
	require.Equal(t, owner.ID.String(), result.ID)
	require.Equal(t, owner.Username, result.Name)
	require.Equal(t, owner.Email, result.Email)
}

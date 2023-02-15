package database_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/database/dbgen"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/rbac"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/database/dbtestutil"
)

func TestModelQueries(t *testing.T) {
	db, _ := dbtestutil.NewDB(t)

	var (
		ctx  = context.Background()
		az   = rbac.NewAuthorizer(prometheus.NewRegistry())
		org  = dbgen.Organization(t, db, database.Organization{})
		user = dbgen.User(t, db, database.User{
			RBACRoles: []string{rbac.RoleOwner()},
		})
		tpl = dbgen.Template(t, db, database.Template{
			OrganizationID: org.ID,
			CreatedBy:      user.ID,
		})
		tv = dbgen.TemplateVersion(t, db, database.TemplateVersion{
			OrganizationID: org.ID,
			TemplateID: uuid.NullUUID{
				UUID:  tpl.ID,
				Valid: true,
			},
			CreatedBy: user.ID,
		})
		w = dbgen.Workspace(t, db, database.Workspace{
			OrganizationID: org.ID,
			TemplateID:     tpl.ID,
			OwnerID:        user.ID,
		})
		job = dbgen.ProvisionerJob(t, db, database.ProvisionerJob{
			OrganizationID: org.ID,
			InitiatorID:    user.ID,
		})
		_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			WorkspaceID:       w.ID,
			TemplateVersionID: tv.ID,
			BuildNumber:       2,
			InitiatorID:       user.ID,
			JobID:             job.ID,
		})
	)
	roles, err := db.GetAuthorizationUserRoles(ctx, user.ID)
	require.NoError(t, err)

	subj := rbac.Subject{
		ID:     roles.ID.String(),
		Roles:  rbac.RoleNames(roles.Roles),
		Groups: roles.Groups,
		Scope:  rbac.ScopeAll,
	}

	prep, err := az.Prepare(ctx, subj, rbac.ActionRead, rbac.ResourceWorkspace.Type)
	require.NoError(t, err)

	workspaces, err := db.GetAuthorizedWorkspaces(ctx, database.GetWorkspacesParams{}, prep)
	require.NoError(t, err)
	fmt.Println(workspaces)
}

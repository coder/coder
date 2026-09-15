package chattool_test

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// Dormancy auto-delete leaves dormant_at set, which switches the row's RBAC
// object to workspace_dormant. The chatd actor must still read it so the
// deleted binding is skipped and a replacement workspace gets created.
func TestCreateWorkspace_DormantDeletedWorkspace(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	db, _ := dbtestutil.NewDB(t)

	user := dbgen.User(t, db, database.User{})
	modelCfg := seedModelConfig(t, db)
	org := dbgen.Organization(t, db, database.Organization{})
	_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})
	version := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
	})
	template := dbgen.Template(t, db, database.Template{
		OrganizationID:  org.ID,
		CreatedBy:       user.ID,
		ActiveVersionID: version.ID,
		AgentsAllowed:   true,
	})
	// Link the version back so the owner's version read authorizes through
	// the template, as in production.
	require.NoError(t, db.UpdateTemplateVersionByID(ctx, database.UpdateTemplateVersionByIDParams{
		ID:         version.ID,
		TemplateID: uuid.NullUUID{UUID: template.ID, Valid: true},
		UpdatedAt:  version.UpdatedAt,
		Name:       version.Name,
		Message:    version.Message,
	}))
	wsResp := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OwnerID:        user.ID,
		OrganizationID: org.ID,
		TemplateID:     template.ID,
		Deleted:        true,
		DormantAt:      sql.NullTime{Time: time.Now(), Valid: true},
	}).Seed(database.WorkspaceBuild{
		Transition: database.WorkspaceTransitionDelete,
	}).Do()

	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		WorkspaceID:       uuid.NullUUID{UUID: wsResp.Workspace.ID, Valid: true},
		LastModelConfigID: modelCfg.ID,
		Title:             "test-create-dormant-deleted-workspace",
	})

	authzDB := dbauthz.New(
		db,
		rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry()),
		slogtest.Make(t, nil),
		testAccessControlStorePointer(),
	)
	var createCalled atomic.Bool
	tool := chattool.CreateWorkspace(authzDB, org.ID, chat.ID, chattool.CreateWorkspaceOptions{
		OwnerID: user.ID,
		CreateFn: func(_ context.Context, _ uuid.UUID, req codersdk.CreateWorkspaceRequest) (codersdk.Workspace, error) {
			createCalled.Store(true)
			require.Equal(t, template.ID, req.TemplateID)
			return codersdk.Workspace{}, xerrors.New("creation stopped by test")
		},
		WorkspaceMu: &sync.Mutex{},
		Logger:      slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
	})

	resp, err := tool.Run(
		dbauthz.AsChatd(ctx),
		fantasy.ToolCall{
			ID:    "call-1",
			Name:  "create_workspace",
			Input: fmt.Sprintf(`{"template_id": %q}`, template.ID),
		},
	)
	require.NoError(t, err)
	require.True(t, createCalled.Load(), "the deleted binding must not block creation: %s", resp.Content)
	require.Contains(t, resp.Content, "creation stopped by test")
	require.NotContains(t, resp.Content, "forbidden")
}

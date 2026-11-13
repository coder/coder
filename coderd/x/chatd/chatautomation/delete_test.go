package chatautomation_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/x/chatd/chatautomation"
	"github.com/coder/coder/v2/testutil"
)

// fixture is one organization with a member who owns automations and chats.
type fixture struct {
	db    database.Store
	org   database.Organization
	owner database.User
	model database.ChatModelConfig
}

func newFixture(t *testing.T, db database.Store) fixture {
	t.Helper()
	org := dbgen.Organization(t, db, database.Organization{})
	owner := dbgen.User(t, db, database.User{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: owner.ID, OrganizationID: org.ID})
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{OrganizationID: org.ID})
	return fixture{db: db, org: org, owner: owner, model: model}
}

func (f fixture) chat(t *testing.T) database.Chat {
	t.Helper()
	return dbgen.Chat(t, f.db, database.Chat{OrganizationID: f.org.ID, OwnerID: f.owner.ID, LastModelConfigID: f.model.ID})
}

func (f fixture) automation(t *testing.T, target database.Chat) database.ChatAutomation {
	t.Helper()
	return dbgen.ChatAutomation(t, f.db, database.ChatAutomation{
		OrganizationID: f.org.ID,
		OwnerID:        f.owner.ID,
		TargetChatID:   uuid.NullUUID{UUID: target.ID, Valid: true},
	})
}

func (f fixture) automationExists(ctx context.Context, t *testing.T, id uuid.UUID) bool {
	t.Helper()
	_, err := f.db.GetChatAutomationByID(ctx, id)
	if xerrors.Is(err, sql.ErrNoRows) {
		return false
	}
	require.NoError(t, err)
	return true
}

func authorized(t *testing.T, db database.Store) database.Store {
	t.Helper()
	return dbauthz.New(db, rbac.NewAuthorizer(prometheus.NewRegistry()),
		slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}), coderdtest.AccessControlStorePointer())
}

// roleActor holds one custom organization role with the given actions on chat
// automations and nothing else.
func roleActor(orgID uuid.UUID, actions ...policy.Action) rbac.Subject {
	return rbac.Subject{
		Type: rbac.SubjectTypeUser,
		ID:   uuid.NewString(),
		Roles: rbac.Roles{{
			Identifier: rbac.RoleIdentifier{Name: "automation-role", OrganizationID: orgID},
			ByOrgID: map[string]rbac.OrgPermissions{orgID.String(): {
				Org: rbac.Permissions(map[string][]policy.Action{rbac.ResourceChatAutomation.Type: actions}),
			}},
		}},
		Scope: rbac.ScopeAll,
	}
}

// TestDelete deletes as a role that holds only read and delete on automations.
func TestDelete(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	db, _ := dbtestutil.NewDB(t)
	f := newFixture(t, db)
	chat := f.chat(t)
	automation, other := f.automation(t, chat), f.automation(t, chat)

	deleted, err := chatautomation.Delete(ctx, authorized(t, db),
		roleActor(f.org.ID, policy.ActionRead, policy.ActionDelete), f.org.ID, automation.ID)

	require.NoError(t, err)
	require.Equal(t, automation.ID, deleted.ID)
	require.False(t, f.automationExists(ctx, t, automation.ID))
	require.True(t, f.automationExists(ctx, t, other.ID))
}

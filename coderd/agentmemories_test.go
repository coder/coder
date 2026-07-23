package coderd_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestAgentMemoriesCRUD(t *testing.T) {
	t.Parallel()

	admin, db := coderdtest.NewWithDatabase(t, nil)
	first := coderdtest.CreateFirstUser(t, admin)
	memberRaw, member := coderdtest.CreateAnotherUser(t, admin, first.OrganizationID)
	otherRaw, _ := coderdtest.CreateAnotherUser(t, admin, first.OrganizationID, rbac.RoleAuditor())
	client := codersdk.NewExperimentalClient(memberRaw)
	other := codersdk.NewExperimentalClient(otherRaw)
	ctx := testutil.Context(t, testutil.WaitMedium)

	preferred := insertAPIAgentMemory(t, db, member, "/memory.md", "preferred")
	insertAPIAgentMemory(t, db, member, "/projects/alpha.md", "alpha")
	insertAPIAgentMemory(t, db, member, "/projects/nested/beta.md", "beta")

	root, err := client.AgentMemoryChildren(ctx, codersdk.Me, "/", 0)
	require.NoError(t, err)
	require.Len(t, root.Entries, 2)
	require.Equal(t, codersdk.AgentMemoryEntryKindDirectory, root.Entries[0].Kind)
	require.Equal(t, "/projects", root.Entries[0].Path)
	require.Equal(t, "/memory.md", root.Entries[1].Path)

	defaultMemory, err := client.DefaultAgentMemory(ctx, codersdk.Me)
	require.NoError(t, err)
	require.Equal(t, preferred.ID, defaultMemory.ID)

	got, err := client.AgentMemory(ctx, codersdk.Me, preferred.ID)
	require.NoError(t, err)
	require.Equal(t, "preferred", got.Content)

	updated, err := client.UpdateAgentMemory(ctx, codersdk.Me, preferred.ID, codersdk.UpdateAgentMemoryRequest{
		Content: "updated", ExpectedUpdatedAt: got.UpdatedAt,
	})
	require.NoError(t, err)
	require.Equal(t, "updated", updated.Content)

	_, err = client.UpdateAgentMemory(ctx, codersdk.Me, preferred.ID, codersdk.UpdateAgentMemoryRequest{
		Content: "stale", ExpectedUpdatedAt: got.UpdatedAt,
	})
	requireSDKErrorStatus(t, err, http.StatusConflict)

	_, err = client.UpdateAgentMemory(ctx, codersdk.Me, preferred.ID, codersdk.UpdateAgentMemoryRequest{
		Content: strings.Repeat("x", 65_537), ExpectedUpdatedAt: updated.UpdatedAt,
	})
	requireSDKErrorStatus(t, err, http.StatusBadRequest)

	_, err = other.AgentMemory(ctx, member.ID.String(), preferred.ID)
	requireSDKErrorStatus(t, err, http.StatusForbidden)

	require.NoError(t, client.DeleteAgentMemory(ctx, codersdk.Me, preferred.ID))
	_, err = client.AgentMemory(ctx, codersdk.Me, preferred.ID)
	requireSDKErrorStatus(t, err, http.StatusNotFound)
}

func TestAgentMemoriesEmptyAndInvalidDirectory(t *testing.T) {
	t.Parallel()

	admin := coderdtest.New(t, nil)
	first := coderdtest.CreateFirstUser(t, admin)
	memberRaw, _ := coderdtest.CreateAnotherUser(t, admin, first.OrganizationID)
	client := codersdk.NewExperimentalClient(memberRaw)
	ctx := testutil.Context(t, testutil.WaitMedium)

	_, err := client.DefaultAgentMemory(ctx, codersdk.Me)
	requireSDKErrorStatus(t, err, http.StatusNotFound)

	res, err := memberRaw.Request(ctx, http.MethodGet, "/api/experimental/users/me/agent-memories?directory=relative", nil)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusBadRequest, res.StatusCode)
}

func insertAPIAgentMemory(t *testing.T, db database.Store, user codersdk.User, path, content string) database.AgentMemory {
	t.Helper()
	memory, err := db.InsertAgentMemory(dbauthz.As(context.Background(), coderdtest.AuthzUserSubject(user)), database.InsertAgentMemoryParams{
		ID: uuid.New(), UserID: user.ID, Path: path, Content: content,
	})
	require.NoError(t, err)
	return memory
}

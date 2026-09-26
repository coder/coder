package coderd_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/httpapi/httperror"
	"github.com/coder/coder/v2/testutil"
)

// TestChatOwnerContextDeletedUser pins the chat workspace tools'
// deleted-owner contract: chats are not purged on soft-delete, so a
// tool call can still act for a deleted owner and must get a 403
// responder the tools surface as a structured response.
func TestChatOwnerContextDeletedUser(t *testing.T) {
	t.Parallel()

	client, _, api := coderdtest.NewWithAPI(t, nil)
	owner := coderdtest.CreateFirstUser(t, client)
	_, user := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
	ctx := testutil.Context(t, testutil.WaitLong)

	//nolint:gocritic // User deletion requires owner permission.
	require.NoError(t, client.DeleteUser(ctx, user.ID))

	_, _, err := coderd.ChatOwnerContext(api, ctx, user.ID)
	require.Error(t, err)
	responder, ok := httperror.IsResponder(err)
	require.True(t, ok, "deleted owner must map to a structured responder")
	status, resp := responder.Response()
	require.Equal(t, http.StatusForbidden, status)
	require.Equal(t, "Chat owner has been deleted.", resp.Message)
}

package coderd

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestManualMembershipBlockedResponse locks the singular and plural wording of
// the OIDC org-sync block message. The plural branch is what an admin sees when
// batch-adding several OIDC-synced users, so it must not regress.
func TestManualMembershipBlockedResponse(t *testing.T) {
	t.Parallel()

	t.Run("Single", func(t *testing.T) {
		t.Parallel()
		resp := manualMembershipBlockedResponse([]string{"alice"})
		require.Contains(t, resp.Message, "not allowed for this user")
		require.Contains(t, resp.Message, "Have the user re-login")
		require.Contains(t, resp.Detail, "User alice is an OIDC user")
	})

	t.Run("Multiple", func(t *testing.T) {
		t.Parallel()
		resp := manualMembershipBlockedResponse([]string{"alice", "bob"})
		require.Contains(t, resp.Message, "not allowed for these users")
		require.Contains(t, resp.Message, "Have the users re-login")
		require.Contains(t, resp.Detail, "Users alice, bob are OIDC users")
	})
}

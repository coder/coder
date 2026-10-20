package database_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/testutil"
)

// TestWorkspaceAgentLifecycleStateStartupFinished keeps the Go startup
// predicate, which decides when agents announce startup and when tools
// wait for delivery, in step with the claim query that delivers.
func TestWorkspaceAgentLifecycleStateStartupFinished(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	f := newChatFileDeliveryFixture(t)
	for _, state := range database.AllWorkspaceAgentLifecycleStateValues() {
		t.Run(string(state), func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			workspace, build := f.build(t, database.WorkspaceTable{}, 1, database.WorkspaceTransitionStart)
			f.agent(ctx, t, build, database.WorkspaceAgent{Name: "dev", LifecycleState: state})
			chat := f.chat(t, workspace.ID, uuid.Nil)
			f.linkedFile(ctx, t, chat.ID, database.ChatFilePurposeWorkspace, []byte("payload"))

			claimed := len(acquireChatFilesFor(ctx, t, f.db, chat.ID, dbtime.Now())) == 1
			require.Equal(t, state.StartupFinished(), claimed)
		})
	}
}

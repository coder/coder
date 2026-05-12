package agentfake

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/testutil"
)

// Asserts that an authentication failure during enumeration produces a fatal error, so the retry loop in
// enumerateWithRetry surfaces it immediately rather than hammering endpoints with credentials that will never work.
func Test_Manager_EnumerateExternalAgents_invalidTokenIsFatal(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)

	client, db := coderdtest.NewWithDatabase(t, nil)
	user := coderdtest.CreateFirstUser(t, client)

	r := BuildExternalAgentWorkspace(t, db, user, uuid.Nil)
	tmpl, err := client.Template(ctx, r.Workspace.TemplateID)
	require.NoError(t, err)

	// Replace the client's session token with garbage to provoke a 401 from coderd's workspace-list endpoint.
	// The Manager should surface that as a fatal error.
	client.SetSessionToken("not-a-valid-session-token")

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	m := NewManager(client, logger, ManagerOptions{Template: tmpl.Name})

	_, err = m.EnumerateExternalAgents(ctx)
	require.Error(t, err, "expected enumeration to fail with an invalid session token")
	require.True(t, isFatalEnumerationError(err),
		"expected error to be classified as fatal so the harness exits and Kubernetes can restart it; got: %v", err)
}

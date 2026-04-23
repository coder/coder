package cli_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

func TestExpAgentsE2E(t *testing.T) {
	t.Parallel()

	t.Run("EmptyStateBoot", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, _, _ := setupExpAgentsBackend(t)
		session := startExpAgentsSession(t, ctx, client)

		session.expect(ctx, "No chats yet. Press n to start a new chat.")
		session.quit()
		require.NoError(t, session.wait(ctx))
	})

	t.Run("ListAndNavigate", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, expClient, orgID := setupExpAgentsBackend(t)

		_ = seedChat(t, ctx, expClient, orgID, "alpha nav seed")
		_ = seedChat(t, ctx, expClient, orgID, "bravo nav seed")
		_ = seedChat(t, ctx, expClient, orgID, "charlie nav seed")

		session := startExpAgentsSession(t, ctx, client)

		session.expect(ctx, "charlie nav seed")
		session.expect(ctx, "enter: open")
		session.enter()
		session.expect(ctx, "esc")
		session.esc()
		session.expect(ctx, "enter: open")
		session.quit()
		require.NoError(t, session.wait(ctx))
	})

	t.Run("SearchFilter", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, expClient, orgID := setupExpAgentsBackend(t)

		_ = seedChat(t, ctx, expClient, orgID, "alpha filter seed")
		_ = seedChat(t, ctx, expClient, orgID, "zulu filter seed")

		session := startExpAgentsSession(t, ctx, client)

		session.expect(ctx, "alpha filter seed")
		session.expect(ctx, "enter: open")
		session.writeRune('/')
		session.expect(ctx, "/ ")
		for _, r := range "zzzznotamatch" {
			session.writeRune(r)
		}
		session.expect(ctx, "No matches.")
		session.ctrlC()
		require.NoError(t, session.wait(ctx))
	})

	t.Run("ExistingChatHistory", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, expClient, orgID := setupExpAgentsBackend(t)

		chat := seedChat(t, ctx, expClient, orgID, "direct open seed")
		session := startExpAgentsSession(t, ctx, client, chat.ID.String())

		// The initial render contains both the chat title/content
		// and the status bar in a single frame. Their relative
		// order in the PTY byte stream depends on async title
		// generation, so matching them with separate sequential
		// expects is racy. Instead, just confirm the seed text is
		// visible (proving we are in the chat view), then verify
		// esc navigates back to the list.
		session.expect(ctx, "direct open seed")
		session.esc()
		session.expect(ctx, "enter: open")
		session.quit()
		require.NoError(t, session.wait(ctx))
	})
}

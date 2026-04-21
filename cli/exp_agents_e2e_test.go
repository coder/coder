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

		// Match the short chat ID in the header. The mock's title
		// generation may have already replaced the seed text, so
		// the short ID is the only stable identifier.
		shortID := chat.ID.String()[:8]
		session.expect(ctx, shortID)
		// Wait for the full chat view render including the help
		// row. In the initial render the help row (containing
		// "esc back") follows the header in the byte stream, so
		// this expect succeeds without consuming text out of order.
		session.expect(ctx, "esc back")
		session.esc()
		session.expect(ctx, "enter: open")
		session.quit()
		require.NoError(t, session.wait(ctx))
	})
}

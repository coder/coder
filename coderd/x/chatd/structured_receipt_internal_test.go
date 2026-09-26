package chatd

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestStructuredReceiptMessagesSkipsDuplicates(t *testing.T) {
	t.Parallel()
	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user.ID, OrganizationID: org.ID})
	dbgen.ChatProvider(t, db, database.ChatProvider{})
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{IsDefault: true})
	chat := dbgen.Chat(t, db, database.Chat{OwnerID: user.ID, OrganizationID: org.ID, LastModelConfigID: model.ID})
	machine := chatstate.NewChatMachine(db, ps, chat.ID)
	out := codersdk.ChatStructuredOutput{RequestID: uuid.New(), Status: codersdk.ChatStructuredOutputStatusSucceeded, Value: json.RawMessage(`null`)}

	// The first terminal attempt commits one receipt under the chat lock.
	require.NoError(t, machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		messages, err := structuredReceiptMessages(ctx, store, chat.ID, 0, out)
		require.NoError(t, err)
		require.Len(t, messages, 1)
		_, err = tx.FinishError(chatstate.FinishErrorInput{TerminalMessages: messages})
		return err
	}))
	// A later attempt for the same request adds nothing; another request does.
	require.NoError(t, machine.Update(ctx, func(_ *chatstate.Tx, store database.Store) error {
		messages, err := structuredReceiptMessages(ctx, store, chat.ID, 0, out)
		require.NoError(t, err)
		require.Empty(t, messages)
		other := out
		other.RequestID = uuid.New()
		messages, err = structuredReceiptMessages(ctx, store, chat.ID, 0, other)
		require.NoError(t, err)
		require.Len(t, messages, 1)
		return nil
	}))
	history, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: chat.ID})
	require.NoError(t, err)
	require.Len(t, history, 1)
	got, err := chatstate.ReceiptRowOutcome(history[0])
	require.NoError(t, err)
	require.Equal(t, out, got)
}

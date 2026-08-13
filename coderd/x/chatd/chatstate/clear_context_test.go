package chatstate_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/testutil"
)

// The allowed-state matrix, boundary-row insertion, and error clearing
// are covered by the transition matrix tests. This file pins the
// input validation that is not matrix-driven.

func TestClearContext_RequiresBoundaryMessages(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	seeded := seedState(t, f, chatstate.StateW)
	m := chatstate.NewChatMachine(f.DB, f.Pub, seeded.chatID)

	err := m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.ClearContext(chatstate.ClearContextInput{})
		return err
	})
	require.ErrorIs(t, err, chatstate.ErrTransitionNotAllowed)
	require.Equal(t, chatstate.StateW, f.classify(ctx, t, seeded.chatID),
		"a rejected clear must not change state")
}

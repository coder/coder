package chatd

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

func TestRefreshChatWorkspaceSnapshot_NoReloadWhenWorkspacePresent(t *testing.T) {
	t.Parallel()

	workspaceID := uuid.New()
	chat := database.Chat{
		ID: uuid.New(),
		WorkspaceID: uuid.NullUUID{
			UUID:  workspaceID,
			Valid: true,
		},
	}

	calls := 0
	refreshed, err := refreshChatWorkspaceSnapshot(
		context.Background(),
		chat,
		func(context.Context, uuid.UUID) (database.Chat, error) {
			calls++
			return database.Chat{}, nil
		},
	)
	require.NoError(t, err)
	require.Equal(t, chat, refreshed)
	require.Equal(t, 0, calls)
}

func TestRefreshChatWorkspaceSnapshot_ReloadsWhenWorkspaceMissing(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	workspaceID := uuid.New()
	chat := database.Chat{ID: chatID}
	reloaded := database.Chat{
		ID: chatID,
		WorkspaceID: uuid.NullUUID{
			UUID:  workspaceID,
			Valid: true,
		},
	}

	calls := 0
	refreshed, err := refreshChatWorkspaceSnapshot(
		context.Background(),
		chat,
		func(_ context.Context, id uuid.UUID) (database.Chat, error) {
			calls++
			require.Equal(t, chatID, id)
			return reloaded, nil
		},
	)
	require.NoError(t, err)
	require.Equal(t, reloaded, refreshed)
	require.Equal(t, 1, calls)
}

func TestRefreshChatWorkspaceSnapshot_ReturnsReloadError(t *testing.T) {
	t.Parallel()

	chat := database.Chat{ID: uuid.New()}
	loadErr := xerrors.New("boom")

	refreshed, err := refreshChatWorkspaceSnapshot(
		context.Background(),
		chat,
		func(context.Context, uuid.UUID) (database.Chat, error) {
			return database.Chat{}, loadErr
		},
	)
	require.Error(t, err)
	require.ErrorContains(t, err, "reload chat workspace state")
	require.ErrorContains(t, err, loadErr.Error())
	require.Equal(t, chat, refreshed)
}

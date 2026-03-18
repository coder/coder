//nolint:testpackage // Cleanup tests need access to runner internals.
package chat

import (
	"bytes"
	"context"
	"io"
	"net/url"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/workspacebuild"
)

type workspaceBuildRunnerFunc struct {
	runFunc     func(ctx context.Context, id string, logs io.Writer) (workspacebuild.SlimWorkspace, error)
	cleanupFunc func(ctx context.Context, id string, logs io.Writer) error
}

func (f workspaceBuildRunnerFunc) RunReturningWorkspace(ctx context.Context, id string, logs io.Writer) (workspacebuild.SlimWorkspace, error) {
	if f.runFunc == nil {
		return workspacebuild.SlimWorkspace{}, nil
	}
	return f.runFunc(ctx, id, logs)
}

func (f workspaceBuildRunnerFunc) Cleanup(ctx context.Context, id string, logs io.Writer) error {
	if f.cleanupFunc == nil {
		return nil
	}
	return f.cleanupFunc(ctx, id, logs)
}

func TestRunnerCleanup(t *testing.T) {
	t.Parallel()

	workspaceID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	chatID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	serverURL, err := url.Parse("http://example.com")
	require.NoError(t, err)
	client := codersdk.New(serverURL)

	t.Run("CleansOwnedWorkspaceAfterArchivingChat", func(t *testing.T) {
		t.Parallel()

		runner := NewRunner(client, Config{})
		runner.setChatID(chatID)
		runner.setWorkspaceID(workspaceID)

		events := make([]string, 0, 2)
		runner.archiveChat = func(ctx context.Context, gotChatID uuid.UUID) error {
			require.Equal(t, chatID, gotChatID)
			events = append(events, "archive")
			return nil
		}
		runner.workspacebuildRunner = workspaceBuildRunnerFunc{
			cleanupFunc: func(ctx context.Context, id string, logs io.Writer) error {
				require.Equal(t, "runner-1", id)
				events = append(events, "workspace")
				return nil
			},
		}

		logs := bytes.NewBuffer(nil)
		err := runner.Cleanup(context.Background(), "runner-1", logs)
		require.NoError(t, err)
		require.Equal(t, []string{"archive", "workspace"}, events)
	})

	t.Run("SkipsWorkspaceDeleteForSharedWorkspaceMode", func(t *testing.T) {
		t.Parallel()

		runner := NewRunner(client, Config{})
		runner.setChatID(chatID)
		runner.setWorkspaceID(workspaceID)

		archived := false
		runner.archiveChat = func(ctx context.Context, gotChatID uuid.UUID) error {
			require.Equal(t, chatID, gotChatID)
			archived = true
			return nil
		}

		err := runner.Cleanup(context.Background(), "runner-1", bytes.NewBuffer(nil))
		require.NoError(t, err)
		require.True(t, archived)
	})

	t.Run("CleansOwnedWorkspaceEvenWithoutChat", func(t *testing.T) {
		t.Parallel()

		runner := NewRunner(client, Config{})
		runner.setWorkspaceID(workspaceID)
		runner.archiveChat = func(ctx context.Context, gotChatID uuid.UUID) error {
			t.Fatalf("archive should be skipped when no chat was created")
			return nil
		}

		deleted := false
		runner.workspacebuildRunner = workspaceBuildRunnerFunc{
			cleanupFunc: func(ctx context.Context, id string, logs io.Writer) error {
				deleted = true
				return nil
			},
		}

		err := runner.Cleanup(context.Background(), "runner-1", bytes.NewBuffer(nil))
		require.NoError(t, err)
		require.True(t, deleted)
	})
}

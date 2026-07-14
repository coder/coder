package chatd

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

func TestCoderAssistantCLIEnv(t *testing.T) {
	t.Parallel()

	chat := database.Chat{
		ID:      uuid.New(),
		OwnerID: uuid.New(),
	}

	t.Run("MintsOncePerResolver", func(t *testing.T) {
		t.Parallel()

		var (
			mu    sync.Mutex
			calls int
		)
		server := &Server{
			accessURL: "https://coder.example.com",
			mintCLITokenFn: func(_ context.Context, ownerID uuid.UUID, chatID uuid.UUID) (string, error) {
				mu.Lock()
				defer mu.Unlock()
				calls++
				assert.Equal(t, chat.OwnerID, ownerID)
				assert.Equal(t, chat.ID, chatID)
				return "minted-token", nil
			},
		}

		resolve := server.coderAssistantCLIEnv(chat)
		for range 3 {
			env, err := resolve(context.Background())
			require.NoError(t, err)
			assert.Equal(t, "minted-token", env["CODER_SESSION_TOKEN"])
			assert.Equal(t, "https://coder.example.com", env["CODER_URL"])
		}
		assert.Equal(t, 1, calls, "token should be minted once and reused")
	})

	t.Run("MintErrorNotCached", func(t *testing.T) {
		t.Parallel()

		var (
			mu    sync.Mutex
			calls int
		)
		server := &Server{
			accessURL: "https://coder.example.com",
			mintCLITokenFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID) (string, error) {
				mu.Lock()
				defer mu.Unlock()
				calls++
				if calls == 1 {
					return "", xerrors.New("mint failed")
				}
				return "minted-token", nil
			},
		}

		resolve := server.coderAssistantCLIEnv(chat)
		_, err := resolve(context.Background())
		require.ErrorContains(t, err, "mint failed")

		env, err := resolve(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "minted-token", env["CODER_SESSION_TOKEN"])
		assert.Equal(t, 2, calls, "failed mint should be retried")
	})
}

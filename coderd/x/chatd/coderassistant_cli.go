package chatd

import (
	"context"
	"sync"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

// MintCLITokenFn mints a short-lived session token for the chat
// owner so the Coder Assistant can run the Coder CLI inside a bound
// workspace with the owner's own permissions. Implementations must
// enforce the owner's RBAC subject; RBAC on every CLI call is then
// enforced server-side by the token itself.
type MintCLITokenFn func(ctx context.Context, ownerID uuid.UUID, chatID uuid.UUID) (string, error)

// coderAssistantCLIEnv returns an environment resolver for the
// execute tool that authenticates the Coder CLI as the chat owner.
// The token is minted lazily on the first command of a generation
// and reused for subsequent commands in the same generation. It is
// passed per-process via env and never written to disk.
func (server *Server) coderAssistantCLIEnv(chat database.Chat) func(context.Context) (map[string]string, error) {
	var (
		mu    sync.Mutex
		token string
	)
	return func(ctx context.Context) (map[string]string, error) {
		mu.Lock()
		defer mu.Unlock()
		if token == "" {
			minted, err := server.mintCLITokenFn(ctx, chat.OwnerID, chat.ID)
			if err != nil {
				return nil, xerrors.Errorf("mint assistant CLI token: %w", err)
			}
			token = minted
		}
		return map[string]string{
			"CODER_URL":           server.accessURL,
			"CODER_SESSION_TOKEN": token,
		}, nil
	}
}

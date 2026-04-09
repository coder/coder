package chatd

import (
	"context"

	"charm.land/fantasy"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
)

// ResolveChatModelForTest exposes resolveChatModel to external-package tests.
func ResolveChatModelForTest(
	ctx context.Context,
	server *Server,
	chat database.Chat,
) (fantasy.LanguageModel, database.ChatModelConfig, chatprovider.ProviderAPIKeys, error) {
	model, config, keys, _, err := server.resolveChatModel(ctx, chat)
	return model, config, keys, err
}

// WaitUntilIdleForTest waits for background chat work tracked by the server to
// finish without shutting the server down. Tests use this to assert final
// database state only after asynchronous chat processing has completed.
// Close waits for the same tracked work, but also stops the server.
func WaitUntilIdleForTest(server *Server) {
	server.drainInflight()
}

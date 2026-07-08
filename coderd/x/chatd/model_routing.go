package chatd

import (
	"context"
	"net/http"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
)

type modelClientRequest struct {
	Chat         database.Chat
	ModelName    string
	UserAgent    string
	ExtraHeaders map[string]string
}

type modelBuildOptions struct {
	ActiveAPIKeyID string
	RecordHTTP     bool
}

func modelBuildOptionsFromMessages(messages []database.ChatMessage) modelBuildOptions {
	apiKeyID, _ := activeTurnAPIKeyIDFromMessages(messages)
	return modelBuildOptions{ActiveAPIKeyID: apiKeyID}
}

// withActiveTurnAPIKeyID augments ctx with the active turn's delegated API
// key ID when one is known. AI Gateway routing and subagent tool callbacks
// read this value from the context to attribute requests to the correct
// turn. When no key is known, ctx is returned unchanged.
func withActiveTurnAPIKeyID(ctx context.Context, opts modelBuildOptions) context.Context {
	if opts.ActiveAPIKeyID == "" {
		return ctx
	}
	return aibridge.WithDelegatedAPIKeyID(ctx, opts.ActiveAPIKeyID)
}

func (p *Server) enabledAIProviderByID(ctx context.Context, providerID uuid.UUID) (database.AIProvider, error) {
	provider, err := p.db.GetAIProviderByID(ctx, providerID)
	if err != nil {
		return database.AIProvider{}, xerrors.Errorf("get AI provider: %w", err)
	}
	if !provider.Enabled {
		return database.AIProvider{}, xerrors.Errorf("AI provider %s is disabled", provider.ID)
	}
	return provider, nil
}

func newLanguageModel(
	providerHint string,
	modelName string,
	providerKeys chatprovider.ProviderAPIKeys,
	userAgent string,
	extraHeaders map[string]string,
	httpClient *http.Client,
) (fantasy.LanguageModel, error) {
	model, err := chatprovider.ModelFromConfig(
		providerHint,
		modelName,
		providerKeys,
		userAgent,
		extraHeaders,
		httpClient,
	)
	if err != nil {
		return nil, err
	}
	if model == nil {
		provider, resolvedModel, resolveErr := chatprovider.ResolveModelWithProviderHint(modelName, providerHint)
		if resolveErr != nil {
			return nil, resolveErr
		}
		return nil, xerrors.Errorf(
			"create model for %s/%s returned nil",
			provider,
			resolvedModel,
		)
	}
	return model, nil
}

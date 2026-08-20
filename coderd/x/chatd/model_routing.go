package chatd

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/codersdk"
)

type modelClientRequest struct {
	Chat         database.Chat
	ModelName    string
	UserAgent    string
	ExtraHeaders map[string]string
	// CallConfig is the parsed model config row's options; zero for paths
	// without a config row.
	CallConfig codersdk.ChatModelCallConfig
}

type modelBuildOptions struct {
	ActiveAPIKeyID string
	RecordHTTP     bool
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
	openAIConfig *codersdk.ChatModelOpenAIConfig,
) (chatprovider.Model, error) {
	model, err := chatprovider.ModelFromConfig(
		providerHint,
		modelName,
		providerKeys,
		userAgent,
		extraHeaders,
		httpClient,
		openAIConfig,
	)
	if err != nil {
		return chatprovider.Model{}, err
	}
	if !model.Valid() {
		provider, resolvedModel, resolveErr := chatprovider.ResolveModelWithProviderHint(modelName, providerHint)
		if resolveErr != nil {
			return chatprovider.Model{}, resolveErr
		}
		return chatprovider.Model{}, xerrors.Errorf(
			"create model for %s/%s returned nil",
			provider,
			resolvedModel,
		)
	}
	return model, nil
}

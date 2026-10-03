package chat

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/codersdk"
)

const (
	scaletestAIProviderType        = codersdk.AIProviderTypeOpenAICompat
	scaletestAIProviderName        = "coder-scaletest-mock"
	scaletestAIProviderDisplayName = "Scaletest LLM Mock"
	scaletestAIProviderAPIKey      = "coder-scaletest"
	scaletestModelName             = "scaletest-model"
	scaletestModelDisplayName      = "Scaletest Model"
	scaletestModelContextLimit     = int64(4096)
)

// DefaultProviderPropagationWait is how long to wait after creating or
// updating the mock LLM provider before starting chats. Provider config is
// cached per coderd replica with a 10 second TTL (see
// coderd/x/chatd/configcache.go), and a change is only guaranteed to be
// visible everywhere once every replica's cached entry has expired. 15
// seconds comfortably exceeds that TTL.
const DefaultProviderPropagationWait = 15 * time.Second

type scaletestAIProviderAction string

const (
	scaletestAIProviderActionCreated scaletestAIProviderAction = "created"
	scaletestAIProviderActionUpdated scaletestAIProviderAction = "updated"
	scaletestAIProviderActionReused  scaletestAIProviderAction = "reused"
)

// EnsureScaletestChatModel bootstraps the shared AI provider used by chat
// scaletests. It returns a function that creates or reuses the ChatModel for
// each organization. When the provider changes, it waits for cached provider
// configurations to expire before chats start.
func EnsureScaletestChatModel(ctx context.Context, client *codersdk.Client, logger slog.Logger, llmMockURL string, propagationWait time.Duration) (func(organizationID uuid.UUID) (uuid.UUID, error), error) {
	expClient := codersdk.NewExperimentalClient(client)

	logger.Info(ctx, "bootstrapping mock LLM provider", slog.F("llm_mock_url", llmMockURL))

	provider, providerAction, err := ensureScaletestAIProvider(ctx, expClient, llmMockURL)
	if err != nil {
		return nil, err
	}

	switch providerAction {
	case scaletestAIProviderActionCreated:
		logger.Info(ctx, "created mock LLM provider",
			slog.F("provider_name", provider.Name),
			slog.F("provider_id", provider.ID),
			slog.F("llm_mock_url", llmMockURL),
		)
	case scaletestAIProviderActionUpdated:
		logger.Info(ctx, "updated mock LLM provider",
			slog.F("provider_name", provider.Name),
			slog.F("provider_id", provider.ID),
			slog.F("llm_mock_url", llmMockURL),
		)
	case scaletestAIProviderActionReused:
		logger.Info(ctx, "reusing mock LLM provider",
			slog.F("provider_name", provider.Name),
			slog.F("provider_id", provider.ID),
		)
	}

	if providerAction != scaletestAIProviderActionReused && propagationWait > 0 {
		logger.Info(ctx, "waiting for mock LLM provider propagation",
			slog.F("provider_name", provider.Name),
			slog.F("wait", propagationWait),
		)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(propagationWait):
		}
	}

	return func(organizationID uuid.UUID) (uuid.UUID, error) {
		return ensureScaletestChatModelRecord(ctx, expClient, logger, provider, organizationID)
	}, nil
}

func ensureScaletestChatModelRecord(ctx context.Context, client chatModelClient, logger slog.Logger, provider codersdk.AIProvider, organizationID uuid.UUID) (uuid.UUID, error) {
	resp, err := client.ChatModels(ctx, organizationID)
	if err != nil {
		return uuid.Nil, xerrors.Errorf("list chat models: %w", err)
	}

	for i := range resp.Models {
		matchesProvider := resp.Models[i].AIProviderID == provider.ID
		matchesModel := resp.Models[i].Model == scaletestModelName
		if !matchesProvider || !matchesModel {
			continue
		}
		if !resp.Models[i].Enabled {
			return uuid.Nil, xerrors.Errorf("existing scaletest ChatModel %s is disabled; re-enable or delete it before running scaletests", resp.Models[i].ID)
		}
		modelID := resp.Models[i].ID
		logger.Info(ctx, "reusing scaletest ChatModel", slog.F("model_id", modelID), slog.F("organization_id", organizationID))
		return modelID, nil
	}

	enabled := true
	isDefault := false
	contextLimit := scaletestModelContextLimit
	created, err := client.CreateChatModel(ctx, organizationID, codersdk.CreateChatModelRequest{
		AIProviderID: &provider.ID,
		Model:        scaletestModelName,
		DisplayName:  scaletestModelDisplayName,
		Enabled:      &enabled,
		IsDefault:    &isDefault,
		ContextLimit: &contextLimit,
	})
	if err != nil {
		return uuid.Nil, xerrors.Errorf("create scaletest ChatModel: %w", err)
	}
	logger.Info(ctx, "created scaletest ChatModel", slog.F("model_id", created.ID), slog.F("organization_id", organizationID))
	return created.ID, nil
}

func ensureScaletestAIProvider(ctx context.Context, client *codersdk.ExperimentalClient, llmMockURL string) (codersdk.AIProvider, scaletestAIProviderAction, error) {
	provider, err := client.AIProvider(ctx, scaletestAIProviderName)
	if err != nil {
		var sdkErr *codersdk.Error
		if !xerrors.As(err, &sdkErr) || sdkErr.StatusCode() != http.StatusNotFound {
			return codersdk.AIProvider{}, "", xerrors.Errorf("look up scaletest AI provider: %w", err)
		}

		created, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:        scaletestAIProviderType,
			Name:        scaletestAIProviderName,
			DisplayName: scaletestAIProviderDisplayName,
			Enabled:     true,
			BaseURL:     llmMockURL,
			APIKeys:     []string{scaletestAIProviderAPIKey},
		})
		if err == nil {
			return created, scaletestAIProviderActionCreated, nil
		}

		sdkErr = nil
		if !xerrors.As(err, &sdkErr) || sdkErr.StatusCode() != http.StatusConflict {
			return codersdk.AIProvider{}, "", xerrors.Errorf("create scaletest AI provider: %w", err)
		}

		provider, err = client.AIProvider(ctx, scaletestAIProviderName)
		if err != nil {
			return codersdk.AIProvider{}, "", xerrors.Errorf("look up scaletest AI provider after conflict: %w", err)
		}
	}

	if provider.Type != scaletestAIProviderType {
		return codersdk.AIProvider{}, "", xerrors.Errorf("refusing to use scaletest AI provider %s with type %q", provider.ID, provider.Type)
	}
	if provider.DisplayName != scaletestAIProviderDisplayName {
		return codersdk.AIProvider{}, "", xerrors.Errorf("refusing to use scaletest AI provider %s with display name %q", provider.ID, provider.DisplayName)
	}
	if !provider.Enabled {
		return codersdk.AIProvider{}, "", xerrors.Errorf("existing scaletest AI provider %s is disabled; re-enable or delete it before running scaletests", provider.ID)
	}

	var update codersdk.UpdateAIProviderRequest
	needsUpdate := false
	if provider.BaseURL != llmMockURL {
		update.BaseURL = &llmMockURL
		needsUpdate = true
	}
	if len(provider.APIKeys) == 0 {
		apiKey := scaletestAIProviderAPIKey
		apiKeys := []codersdk.AIProviderKeyMutation{{APIKey: &apiKey}}
		update.APIKeys = &apiKeys
		needsUpdate = true
	}
	if !needsUpdate {
		return provider, scaletestAIProviderActionReused, nil
	}

	updated, err := client.UpdateAIProvider(ctx, scaletestAIProviderName, update)
	if err != nil {
		return codersdk.AIProvider{}, "", xerrors.Errorf("update scaletest AI provider: %w", err)
	}
	return updated, scaletestAIProviderActionUpdated, nil
}

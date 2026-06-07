package chat

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/codersdk"
)

const (
	scaletestProviderType        = "openai-compat"
	scaletestProviderDisplayName = "Scaletest LLM Mock"
	scaletestModelName           = "scaletest-model"
	scaletestModelDisplayName    = "Scaletest Model"
)

type scaletestProviderAction string

const (
	scaletestProviderActionCreated scaletestProviderAction = "created"
	scaletestProviderActionUpdated scaletestProviderAction = "updated"
	scaletestProviderActionReused  scaletestProviderAction = "reused"
)

// EnsureScaletestModelConfig bootstraps the shared chat provider and model
// config used by chat scaletests.
func EnsureScaletestModelConfig(ctx context.Context, client *codersdk.ExperimentalClient, logger slog.Logger, llmMockURL string) (uuid.UUID, error) {
	logger.Info(ctx, "bootstrapping mock LLM provider", slog.F("llm_mock_url", llmMockURL))

	provider, providerAction, err := ensureScaletestProvider(ctx, client, llmMockURL)
	if err != nil {
		return uuid.Nil, err
	}

	switch providerAction {
	case scaletestProviderActionCreated:
		logger.Info(ctx, "created mock LLM provider",
			slog.F("provider_type", scaletestProviderType),
			slog.F("llm_mock_url", llmMockURL),
		)
	case scaletestProviderActionUpdated:
		logger.Info(ctx, "updated mock LLM provider",
			slog.F("provider_type", scaletestProviderType),
			slog.F("provider_id", provider.ID),
			slog.F("llm_mock_url", llmMockURL),
		)
	case scaletestProviderActionReused:
		logger.Info(ctx, "reusing mock LLM provider",
			slog.F("provider_type", scaletestProviderType),
			slog.F("provider_id", provider.ID),
		)
	}

	modelConfigs, err := client.ListChatModelConfigs(ctx)
	if err != nil {
		return uuid.Nil, xerrors.Errorf("list chat model configs: %w", err)
	}

	for i := range modelConfigs {
		if modelConfigs[i].Provider != provider.Provider || modelConfigs[i].Model != scaletestModelName {
			continue
		}
		if !modelConfigs[i].Enabled {
			return uuid.Nil, xerrors.Errorf("existing scaletest chat model config %s is disabled; re-enable or delete it before running scaletests", modelConfigs[i].ID)
		}
		modelConfigID := modelConfigs[i].ID
		logger.Info(ctx, "reusing scaletest model config", slog.F("model_config_id", modelConfigID))
		return modelConfigID, nil
	}

	enabled := true
	isDefault := false
	contextLimit := int64(4096)
	created, err := client.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
		Provider:     provider.Provider,
		Model:        scaletestModelName,
		DisplayName:  scaletestModelDisplayName,
		Enabled:      &enabled,
		IsDefault:    &isDefault,
		ContextLimit: &contextLimit,
	})
	if err != nil {
		return uuid.Nil, xerrors.Errorf("create scaletest chat model config: %w", err)
	}
	logger.Info(ctx, "created scaletest model config", slog.F("model_config_id", created.ID))
	return created.ID, nil
}

func ensureScaletestProvider(ctx context.Context, client *codersdk.ExperimentalClient, llmMockURL string) (codersdk.ChatProviderConfig, scaletestProviderAction, error) {
	enabled := true
	mockProviderToken := uuid.NewString()
	created, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
		Provider:    scaletestProviderType,
		DisplayName: scaletestProviderDisplayName,
		APIKey:      mockProviderToken,
		BaseURL:     llmMockURL,
		Enabled:     &enabled,
	})
	if err == nil {
		return created, scaletestProviderActionCreated, nil
	}

	var sdkErr *codersdk.Error
	if !xerrors.As(err, &sdkErr) || sdkErr.StatusCode() != http.StatusConflict {
		return codersdk.ChatProviderConfig{}, "", xerrors.Errorf("create scaletest chat provider: %w", err)
	}

	providers, err := client.ListChatProviders(ctx)
	if err != nil {
		return codersdk.ChatProviderConfig{}, "", xerrors.Errorf("list chat providers: %w", err)
	}

	var existing *codersdk.ChatProviderConfig
	for i := range providers {
		if providers[i].Provider == scaletestProviderType {
			existing = &providers[i]
			break
		}
	}
	if existing == nil {
		return codersdk.ChatProviderConfig{}, "", xerrors.Errorf("find existing %s provider after conflict: not found", scaletestProviderType)
	}
	if existing.DisplayName != scaletestProviderDisplayName {
		return codersdk.ChatProviderConfig{}, "", xerrors.Errorf("refusing to overwrite existing %s provider %s with display name %q", scaletestProviderType, existing.ID, existing.DisplayName)
	}

	if !existing.Enabled {
		return codersdk.ChatProviderConfig{}, "", xerrors.Errorf("existing scaletest chat provider %s is disabled; re-enable or delete it before running scaletests", existing.ID)
	}
	if existing.BaseURL == llmMockURL {
		return *existing, scaletestProviderActionReused, nil
	}

	updated, err := client.UpdateChatProvider(ctx, existing.ID, codersdk.UpdateChatProviderConfigRequest{
		DisplayName: scaletestProviderDisplayName,
		APIKey:      &mockProviderToken,
		BaseURL:     &llmMockURL,
		Enabled:     &enabled,
	})
	if err != nil {
		return codersdk.ChatProviderConfig{}, "", xerrors.Errorf("update scaletest chat provider: %w", err)
	}
	return updated, scaletestProviderActionUpdated, nil
}

package chat

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/retry"
)

const (
	scaletestAIProviderType           = codersdk.AIProviderTypeOpenAICompat
	scaletestAIProviderName           = "coder-scaletest-mock"
	scaletestAIProviderDisplayName    = "Scaletest LLM Mock"
	scaletestAIProviderAPIKey         = "coder-scaletest"
	scaletestModelName                = "scaletest-model"
	scaletestModelDisplayName         = "Scaletest Model"
	scaletestModelContextLimit        = int64(4096)
	scaletestAIProviderProbeWait      = 15 * time.Second
	scaletestAIProviderProbePeriod    = 500 * time.Millisecond
	scaletestAIProviderArchiveTimeout = 5 * time.Second

	scaletestProviderReloadProbePrompt = "Reply with one short sentence."
)

type scaletestAIProviderAction string

const (
	scaletestAIProviderActionCreated scaletestAIProviderAction = "created"
	scaletestAIProviderActionUpdated scaletestAIProviderAction = "updated"
	scaletestAIProviderActionReused  scaletestAIProviderAction = "reused"
)

// EnsureScaletestModelConfig bootstraps the shared AI provider and model
// config used by chat scaletests.
func EnsureScaletestModelConfig(ctx context.Context, client *codersdk.Client, logger slog.Logger, llmMockURL string, organizationID uuid.UUID, workspaceID uuid.UUID) (uuid.UUID, error) {
	expClient := codersdk.NewExperimentalClient(client)

	logger.Info(ctx, "bootstrapping mock LLM provider", slog.F("llm_mock_url", llmMockURL))

	provider, providerAction, err := ensureScaletestAIProvider(ctx, expClient, llmMockURL)
	if err != nil {
		return uuid.Nil, err
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

	modelConfigID, err := ensureScaletestChatModelConfig(ctx, expClient, logger, provider)
	if err != nil {
		return uuid.Nil, err
	}

	if providerAction != scaletestAIProviderActionReused {
		if err := waitForScaletestProviderReload(ctx, expClient, logger, modelConfigID, organizationID, workspaceID); err != nil {
			return uuid.Nil, xerrors.Errorf("wait for mock LLM provider reload: %w", err)
		}
	}

	return modelConfigID, nil
}

func waitForScaletestProviderReload(ctx context.Context, client chatClient, logger slog.Logger, modelConfigID uuid.UUID, organizationID uuid.UUID, workspaceID uuid.UUID) error {
	logger.Info(ctx, "waiting for mock LLM provider reload", slog.F("provider_name", scaletestAIProviderName))
	waitCtx, cancel := context.WithTimeout(ctx, scaletestAIProviderProbeWait)
	defer cancel()

	var lastErr error
	for r := retry.New(scaletestAIProviderProbePeriod, scaletestAIProviderProbePeriod); r.Wait(waitCtx); {
		if err := runScaletestProviderReloadProbe(waitCtx, client, logger, modelConfigID, organizationID, workspaceID); err != nil {
			lastErr = err
			logger.Debug(ctx, "mock LLM provider probe failed", slog.Error(err))
			continue
		}
		return nil
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	if lastErr != nil {
		return xerrors.Errorf("timed out waiting for mock LLM provider reload: %w", lastErr)
	}
	return xerrors.New("timed out waiting for mock LLM provider reload")
}

func runScaletestProviderReloadProbe(ctx context.Context, client chatClient, logger slog.Logger, modelConfigID uuid.UUID, organizationID uuid.UUID, workspaceID uuid.UUID) error {
	chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: organizationID,
		WorkspaceID:    &workspaceID,
		ModelConfigID:  &modelConfigID,
		Content: []codersdk.ChatInputPart{{
			Type: codersdk.ChatInputPartTypeText,
			Text: scaletestProviderReloadProbePrompt,
		}},
	})
	if err != nil {
		return xerrors.Errorf("create probe chat: %w", err)
	}
	defer archiveScaletestProbeChat(ctx, client, logger, chat.ID)

	events, closer, err := client.StreamChat(ctx, chat.ID, nil)
	if err != nil {
		return xerrors.Errorf("stream probe chat: %w", err)
	}
	defer closer.Close()

	for event := range events {
		switch event.Type {
		case codersdk.ChatStreamEventTypeStatus:
			if event.Status != nil && event.Status.Status == codersdk.ChatStatusWaiting {
				return nil
			}
		case codersdk.ChatStreamEventTypeError:
			if event.Error != nil {
				return xerrors.Errorf("probe chat failed: %s", event.Error.Message)
			}
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return xerrors.New("probe chat stream ended before waiting status")
}

func archiveScaletestProbeChat(ctx context.Context, client chatClient, logger slog.Logger, chatID uuid.UUID) {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), scaletestAIProviderArchiveTimeout)
	defer cancel()

	archived := true
	if err := client.UpdateChat(cleanupCtx, chatID, codersdk.UpdateChatRequest{Archived: &archived}); err != nil {
		logger.Warn(ctx, "failed to archive mock LLM provider probe chat", slog.Error(err))
	}
}

func ensureScaletestChatModelConfig(ctx context.Context, client chatModelConfigClient, logger slog.Logger, provider codersdk.AIProvider) (uuid.UUID, error) {
	modelConfigs, err := client.ListChatModelConfigs(ctx)
	if err != nil {
		return uuid.Nil, xerrors.Errorf("list chat model configs: %w", err)
	}

	for i := range modelConfigs {
		matchesProvider := modelConfigs[i].AIProviderID != nil && *modelConfigs[i].AIProviderID == provider.ID
		matchesModel := modelConfigs[i].Model == scaletestModelName
		if !matchesProvider || !matchesModel {
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
	contextLimit := scaletestModelContextLimit
	created, err := client.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
		AIProviderID: &provider.ID,
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

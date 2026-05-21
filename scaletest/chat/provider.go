package chat

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

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
func EnsureScaletestModelConfig(ctx context.Context, client *codersdk.ExperimentalClient, stderr io.Writer, llmMockURL string) (*uuid.UUID, error) {
	_, _ = fmt.Fprintf(stderr, "Bootstrapping mock LLM provider at %s...\n", llmMockURL)

	provider, providerAction, err := ensureScaletestProvider(ctx, client, llmMockURL)
	if err != nil {
		return nil, err
	}

	switch providerAction {
	case scaletestProviderActionCreated:
		_, _ = fmt.Fprintf(stderr, "Created %s provider pointing at %s\n", scaletestProviderType, llmMockURL)
	case scaletestProviderActionUpdated:
		_, _ = fmt.Fprintf(stderr, "Updated %s provider %s to point at %s\n", scaletestProviderType, provider.ID, llmMockURL)
	case scaletestProviderActionReused:
		_, _ = fmt.Fprintf(stderr, "Reusing existing %s provider %s\n", scaletestProviderType, provider.ID)
	}

	modelConfigs, err := client.ListChatModelConfigs(ctx)
	if err != nil {
		return nil, xerrors.Errorf("list chat model configs: %w", err)
	}

	for i := range modelConfigs {
		if modelConfigs[i].Provider == provider.Provider && modelConfigs[i].Model == scaletestModelName {
			modelConfigID := modelConfigs[i].ID
			_, _ = fmt.Fprintf(stderr, "Reusing existing scaletest model config %s\n", modelConfigID)
			return &modelConfigID, nil
		}
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
		return nil, xerrors.Errorf("create scaletest chat model config: %w", err)
	}
	_, _ = fmt.Fprintf(stderr, "Created scaletest model config %s\n", created.ID)
	return &created.ID, nil
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

	if existing.BaseURL == llmMockURL && existing.Enabled {
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

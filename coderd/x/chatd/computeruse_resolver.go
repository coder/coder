package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
)

type computerUseTarget struct {
	provider string
	model    string
	config   database.ChatModelConfig
}

func ResolveComputerUseProvider(
	ctx context.Context,
	p *Server,
	parentChat database.Chat,
) (provider string, modelName string, err error) {
	target, err := resolveComputerUseTarget(ctx, p, parentChat)
	if err != nil {
		return "", "", err
	}
	return target.provider, target.model, nil
}

func (p *Server) isComputerUseConfigured(ctx context.Context) bool {
	deploymentKeys, err := p.computerUseDeploymentKeyChecker(ctx)
	if err != nil {
		return false
	}
	configsByProvider, err := p.enabledComputerUseModelConfigs(ctx)
	if err != nil {
		return false
	}
	for _, provider := range chattool.SupportedComputerUseProviders {
		cfg, ok := configsByProvider[provider]
		if !ok {
			continue
		}
		target, err := computerUseTargetFromConfig(cfg)
		if err != nil {
			normalizedProvider := chatprovider.NormalizeProvider(cfg.Provider)
			if normalizedProvider == "" {
				normalizedProvider = strings.TrimSpace(cfg.Provider)
			}
			p.logger.Debug(ctx, "skipping invalid computer use model config",
				slog.F("model_config_id", cfg.ID),
				slog.F("provider", normalizedProvider),
				slog.Error(err),
			)
			continue
		}
		if err := computerUseTargetEligibilityError(target, deploymentKeys); err == nil {
			return true
		}
	}
	return false
}

func resolveComputerUseTarget(
	ctx context.Context,
	p *Server,
	parentChat database.Chat,
) (computerUseTarget, error) {
	providerKeys, err := p.resolveUserProviderAPIKeys(ctx, parentChat.OwnerID)
	if err != nil {
		return computerUseTarget{}, xerrors.Errorf("resolve provider API keys: %w", err)
	}
	keyChecker := func(provider string) bool {
		return strings.TrimSpace(providerKeys.APIKey(provider)) != ""
	}

	if parentChat.LastModelConfigID != uuid.Nil {
		enabledProvider, err := p.computerUseEnabledProviderChecker(ctx)
		if err != nil {
			return computerUseTarget{}, err
		}
		parentConfig, err := p.configCache.ModelConfigByID(ctx, parentChat.LastModelConfigID)
		if err == nil {
			target, targetErr := computerUseTargetFromConfig(parentConfig)
			if targetErr == nil && computerUseTargetEligibilityError(target, keyChecker) == nil {
				// Child chats may fall back if the parent's pinned provider is no
				// longer enabled. Active computer use chats still fail fast in
				// validatePinnedComputerUseTarget.
				if enabledProvider(target.provider) {
					return target, nil
				}
			}
		} else if !xerrors.Is(err, sql.ErrNoRows) {
			return computerUseTarget{}, xerrors.Errorf("get parent computer use model config: %w", err)
		}
	}

	configsByProvider, err := p.enabledComputerUseModelConfigs(ctx)
	if err != nil {
		return computerUseTarget{}, err
	}
	for _, provider := range chattool.SupportedComputerUseProviders {
		cfg, ok := configsByProvider[provider]
		if !ok {
			continue
		}
		target, err := computerUseTargetFromConfig(cfg)
		if err != nil {
			return computerUseTarget{}, err
		}
		if err := computerUseTargetEligibilityError(target, keyChecker); err == nil {
			return target, nil
		}
	}
	return computerUseTarget{}, xerrors.New("no usable computer use provider is configured")
}

func validatePinnedComputerUseTarget(
	ctx context.Context,
	p *Server,
	chat database.Chat,
) (computerUseTarget, chatprovider.ProviderAPIKeys, error) {
	providerKeys, err := p.resolveUserProviderAPIKeys(ctx, chat.OwnerID)
	if err != nil {
		return computerUseTarget{}, chatprovider.ProviderAPIKeys{}, xerrors.Errorf("resolve provider API keys: %w", err)
	}
	target, err := validatePinnedComputerUseTargetWithKeys(ctx, p, chat, providerKeys)
	if err != nil {
		return computerUseTarget{}, chatprovider.ProviderAPIKeys{}, err
	}
	return target, providerKeys, nil
}

func validatePinnedComputerUseTargetWithKeys(
	ctx context.Context,
	p *Server,
	chat database.Chat,
	providerKeys chatprovider.ProviderAPIKeys,
) (computerUseTarget, error) {
	if chat.LastModelConfigID == uuid.Nil {
		return computerUseTarget{}, xerrors.New("computer use chat is missing a pinned model config")
	}
	cfg, err := p.configCache.ModelConfigByID(ctx, chat.LastModelConfigID)
	if err != nil {
		if xerrors.Is(err, sql.ErrNoRows) {
			return computerUseTarget{}, xerrors.New("computer use chat pinned model config is unavailable")
		}
		return computerUseTarget{}, xerrors.Errorf("get pinned computer use model config: %w", err)
	}
	target, err := computerUseTargetFromConfig(cfg)
	if err != nil {
		return computerUseTarget{}, xerrors.Errorf("resolve pinned computer use model metadata: %w", err)
	}
	enabledProvider, err := p.computerUseEnabledProviderChecker(ctx)
	if err != nil {
		return computerUseTarget{}, err
	}
	if !enabledProvider(target.provider) {
		err := xerrors.Errorf("computer use provider %q is disabled", target.provider)
		return computerUseTarget{}, chaterror.WithClassification(err, chaterror.ClassifiedError{
			Message:  err.Error(),
			Kind:     chaterror.KindConfig,
			Provider: target.provider,
		})
	}
	keyChecker := func(provider string) bool {
		return strings.TrimSpace(providerKeys.APIKey(provider)) != ""
	}
	if err := computerUseTargetEligibilityError(target, keyChecker); err != nil {
		return computerUseTarget{}, chaterror.WithClassification(err, chaterror.ClassifiedError{
			Message:  err.Error(),
			Kind:     chaterror.KindConfig,
			Provider: target.provider,
		})
	}
	return target, nil
}

func computerUseTargetFromConfig(cfg database.ChatModelConfig) (computerUseTarget, error) {
	provider, model, err := chatprovider.ResolveModelWithProviderHint(cfg.Model, cfg.Provider)
	if err != nil {
		return computerUseTarget{}, xerrors.Errorf("resolve computer use model metadata: %w", err)
	}
	provider = chatprovider.NormalizeProvider(provider)
	if !chattool.SupportsComputerUse(provider) {
		err := xerrors.Errorf("computer use provider %q is not supported", provider)
		return computerUseTarget{}, chaterror.WithClassification(err, chaterror.ClassifiedError{
			Message:  err.Error(),
			Kind:     chaterror.KindConfig,
			Provider: provider,
		})
	}
	return computerUseTarget{provider: provider, model: model, config: cfg}, nil
}

func (p *Server) enabledComputerUseModelConfigs(
	ctx context.Context,
) (map[string]database.ChatModelConfig, error) {
	//nolint:gocritic // Chatd needs scoped deployment-config read access to scan enabled model configs.
	chatdCtx := dbauthz.AsChatd(ctx)
	configs, err := p.db.GetEnabledChatModelConfigs(chatdCtx)
	if err != nil {
		return nil, xerrors.Errorf("get enabled chat model configs: %w", err)
	}
	configsByProvider := make(map[string]database.ChatModelConfig, len(chattool.SupportedComputerUseProviders))
	for _, cfg := range configs {
		target, err := computerUseTargetFromConfig(cfg)
		if err != nil {
			continue
		}
		defaultModel, ok := chattool.DefaultComputerUseModel(target.provider)
		if !ok || target.model != defaultModel {
			continue
		}
		if _, ok := configsByProvider[target.provider]; ok {
			continue
		}
		configsByProvider[target.provider] = cfg
	}
	return configsByProvider, nil
}

func (p *Server) computerUseEnabledProviderChecker(
	ctx context.Context,
) (func(string) bool, error) {
	providers, err := p.configCache.EnabledProviders(ctx)
	if err != nil {
		return nil, xerrors.Errorf("get enabled chat providers: %w", err)
	}
	enabled := make(map[string]bool, len(providers))
	for _, provider := range providers {
		normalizedProvider := chatprovider.NormalizeProvider(provider.Provider)
		if normalizedProvider == "" {
			continue
		}
		enabled[normalizedProvider] = true
	}
	return func(provider string) bool {
		return enabled[provider]
	}, nil
}

func (p *Server) computerUseDeploymentKeyChecker(
	ctx context.Context,
) (func(string) bool, error) {
	providers, err := p.configCache.EnabledProviders(ctx)
	if err != nil {
		return nil, xerrors.Errorf("get enabled chat providers: %w", err)
	}
	available := make(map[string]bool, len(providers)+len(chattool.SupportedComputerUseProviders))
	for _, provider := range chattool.SupportedComputerUseProviders {
		if strings.TrimSpace(p.providerAPIKeys.APIKey(provider)) != "" {
			available[provider] = true
		}
	}
	for _, provider := range providers {
		normalizedProvider := chatprovider.NormalizeProvider(provider.Provider)
		if normalizedProvider == "" {
			continue
		}
		if strings.TrimSpace(provider.APIKey) != "" {
			available[normalizedProvider] = true
		}
	}
	return func(provider string) bool {
		return available[provider]
	}, nil
}

func computerUseTargetEligibilityError(
	target computerUseTarget,
	hasCredentials func(string) bool,
) error {
	if !chattool.SupportsComputerUse(target.provider) {
		return xerrors.Errorf("computer use provider %q is not supported", target.provider)
	}
	defaultModel, ok := chattool.DefaultComputerUseModel(target.provider)
	if !ok {
		return xerrors.Errorf("computer use provider %q has no default model", target.provider)
	}
	if target.model != defaultModel {
		return xerrors.Errorf("computer use provider %q requires model %q", target.provider, defaultModel)
	}
	if !target.config.Enabled {
		return xerrors.Errorf("computer use model config %s is disabled", target.config.ID)
	}
	if !hasCredentials(target.provider) {
		return xerrors.Errorf("computer use provider %q credentials are unavailable", target.provider)
	}
	if target.provider == "openai" {
		enabled, err := computerUseOpenAIStoreEnabled(target.config)
		if err != nil {
			return chaterror.WithClassification(err, chaterror.ClassifiedError{
				Message:  err.Error(),
				Kind:     chaterror.KindConfig,
				Provider: target.provider,
			})
		}
		if !enabled {
			return xerrors.New("computer use OpenAI model config requires provider_options.openai.store=true")
		}
	}
	return nil
}

// computerUseOpenAIStoreEnabled treats an omitted OpenAI Store flag
// as enabled so existing configs keep working.
func computerUseOpenAIStoreEnabled(cfg database.ChatModelConfig) (bool, error) {
	callConfig := codersdk.ChatModelCallConfig{}
	if len(cfg.Options) > 0 {
		if err := json.Unmarshal(cfg.Options, &callConfig); err != nil {
			return false, xerrors.Errorf("parse computer use model call config: %w", err)
		}
	}
	if callConfig.ProviderOptions == nil || callConfig.ProviderOptions.OpenAI == nil {
		return false, nil
	}
	return callConfig.ProviderOptions.OpenAI.Store == nil || *callConfig.ProviderOptions.OpenAI.Store, nil
}

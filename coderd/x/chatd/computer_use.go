package chatd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	fantasyopenai "charm.land/fantasy/providers/openai"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	openaicomputeruse "github.com/coder/coder/v2/coderd/x/chatd/chatopenai/computeruse"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/quartz"
)

type computerUseModel struct {
	config   database.ChatModelConfig
	provider codersdk.ChatComputerUseProvider
	// reasoningEffort comes from the organization override, if any.
	reasoningEffort *string
}

func computerUseProviderForAIProviderType(providerType string) (codersdk.ChatComputerUseProvider, bool) {
	switch chatprovider.NormalizeProvider(providerType) {
	case fantasyanthropic.Name:
		return codersdk.ChatComputerUseProviderAnthropic, true
	case fantasyopenai.Name:
		return codersdk.ChatComputerUseProviderOpenAI, true
	default:
		return "", false
	}
}

func computerUseModelLabel(config database.ChatModelConfig) string {
	if config.DisplayName != "" && config.DisplayName != config.Model {
		return fmt.Sprintf("%q (%s)", config.DisplayName, config.Model)
	}
	return fmt.Sprintf("%q", config.Model)
}

// resolveComputerUseModel picks the model for a computer use subagent: the
// organization's computer_use override, else the organization's default chat
// model. A non-empty reason means computer use is unavailable to the owner.
func (p *Server) resolveComputerUseModel(
	ctx context.Context,
	ownerID uuid.UUID,
	organizationID uuid.UUID,
) (model computerUseModel, reason string, err error) {
	//nolint:gocritic // Chatd reads organization model configuration for the chat owner.
	chatdCtx := dbauthz.AsChatd(ctx)

	override, err := p.resolveModelOverride(chatdCtx, modelOverrideSpec{
		context:        string(codersdk.ChatModelOverrideContextComputerUse),
		ownerID:        ownerID,
		organizationID: organizationID,
	})
	if err != nil {
		return computerUseModel{}, "", err
	}

	overrideState := "no computer use model is configured"
	if override.Set || override.Unavailable {
		// A usable override returns below, so a reason built after this point
		// means the saved override cannot be used.
		overrideState = "the configured computer use model cannot be used"
	}

	if override.Set {
		// resolveModelOverride returns only enabled models of enabled AI
		// providers whose credentials the owner can use.
		aiProvider, err := p.db.GetAIProviderByID(chatdCtx, override.Config.AIProviderID.UUID)
		if err != nil {
			return computerUseModel{}, "", xerrors.Errorf("get AI provider for model %s: %w", override.Config.ID, err)
		}

		if provider, ok := computerUseProviderForAIProviderType(string(aiProvider.Type)); ok {
			return computerUseModel{
				config:          override.Config,
				provider:        provider,
				reasoningEffort: override.ReasoningEffort,
			}, "", nil
		}

		// The override API rejects these AI providers, but the model API can
		// still move the saved model to one. Ignore the override then, as the
		// settings page does.
		p.logger.Info(ctx, "computer use model override has an unsupported AI provider, using default model",
			slog.F("model_config_id", override.Config.ID),
			slog.F("ai_provider_type", aiProvider.Type),
		)
	}

	// Read the default under the owner's model ACLs, as chat turns do.
	modelCtx, err := p.callerModelConfigContext(chatdCtx, ownerID)
	if err != nil {
		return computerUseModel{}, "", err
	}

	model.config, err = effectiveDefaultChatModelConfig(modelCtx, p.db, organizationID)
	if errors.Is(err, sql.ErrNoRows) {
		return computerUseModel{}, fmt.Sprintf(
			`type "computer_use" is unavailable because %s and the organization has no default chat model available to the chat owner`,
			overrideState,
		), nil
	}
	if err != nil {
		return computerUseModel{}, "", xerrors.Errorf("get default chat model config: %w", err)
	}

	// The default lookup returns only enabled models of enabled AI providers.
	aiProvider, err := p.db.GetAIProviderByID(chatdCtx, model.config.AIProviderID.UUID)
	if err != nil {
		return computerUseModel{}, "", xerrors.Errorf("get AI provider for model %s: %w", model.config.ID, err)
	}

	label := computerUseModelLabel(model.config)
	provider, ok := computerUseProviderForAIProviderType(string(aiProvider.Type))
	if !ok {
		return computerUseModel{}, fmt.Sprintf(
			`type "computer_use" is unavailable because %s and model %s uses AI provider %q of type %q; computer use requires an Anthropic or OpenAI AI provider`,
			overrideState, label, aiProvider.Name, aiProvider.Type,
		), nil
	}
	model.provider = provider

	providerKeys, err := p.resolveUserProviderAPIKeys(chatdCtx, ownerID, aiProvider.ID)
	if err != nil {
		return computerUseModel{}, "", xerrors.Errorf("resolve provider API keys: %w", err)
	}

	if !userCanUseProviderKeys(providerKeys, string(provider)) {
		return computerUseModel{}, fmt.Sprintf(
			`type "computer_use" is unavailable because %s and credentials for AI provider %q, used by model %s, are not configured`,
			overrideState, aiProvider.Name, label,
		), nil
	}

	return model, "", nil
}

// anthropicComputerUseToolVersion picks the Anthropic computer use tool version
// for a model of an Anthropic AI provider. Claude Platform on AWS and Amazon
// Bedrock offer computer use only through the earlier tool versions, so a
// model that takes the toolset on the Claude API takes computer_20251124 there.
func anthropicComputerUseToolVersion(modelID string, settings codersdk.AIProviderSettings) fantasyanthropic.ComputerUseToolVersion {
	version := fantasyanthropic.ComputerUseToolVersionForModel(modelID)
	if version == fantasyanthropic.ComputerUseToolset20260801 && (settings.ClaudePlatformAWS != nil || settings.Bedrock != nil) {
		return fantasyanthropic.ComputerUse20251124
	}
	return version
}

type computerUseProviderToolOptions struct {
	provider         codersdk.ChatComputerUseProvider
	toolVersion      fantasyanthropic.ComputerUseToolVersion
	isPlanModeTurn   bool
	isComputerUse    bool
	getWorkspaceConn func(context.Context) (workspacesdk.AgentConn, error)
	storeFile        chattool.StoreFileFunc
	clock            quartz.Clock
	logger           slog.Logger
}

func appendComputerUseProviderTool(
	providerTools []chatloop.ProviderTool,
	opts computerUseProviderToolOptions,
) ([]chatloop.ProviderTool, error) {
	// This helper is called for every chat turn. Only chats created by the
	// computer_use subagent definition have ChatModeComputerUse, which filters
	// out root, general, and explore chats. Plan mode is separate from Mode, so
	// planning turns stay gated even for computer-use chats.
	if opts.isPlanModeTurn || !opts.isComputerUse {
		return providerTools, nil
	}

	desktopGeometry := chattool.DefaultComputerUseDesktopGeometry(opts.provider)
	definition, err := chattool.ComputerUseProviderTool(
		opts.provider,
		opts.toolVersion,
		desktopGeometry.DeclaredWidth,
		desktopGeometry.DeclaredHeight,
	)
	if err != nil {
		return providerTools, xerrors.Errorf(
			"build computer use provider tool for provider %q: %w",
			opts.provider,
			err,
		)
	}

	clock := opts.clock
	if clock == nil {
		clock = quartz.NewReal()
	}
	providerTool := chatloop.ProviderTool{
		Definition: definition,
		Runner: chattool.NewComputerUseTool(
			opts.provider,
			opts.toolVersion,
			desktopGeometry.DeclaredWidth,
			desktopGeometry.DeclaredHeight,
			opts.getWorkspaceConn,
			opts.storeFile,
			clock,
			opts.logger,
		),
	}
	if opts.provider == codersdk.ChatComputerUseProviderOpenAI {
		// OpenAI computer-use image results need detail metadata so the model receives
		// the screenshot at original detail when the chat loop sends the tool result.
		providerTool.ResultProviderMetadata = openaicomputeruse.ResultProviderMetadata
	}

	return append(providerTools, providerTool), nil
}

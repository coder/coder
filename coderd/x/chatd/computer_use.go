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

// computerUseModel is the model a computer use subagent runs on.
type computerUseModel struct {
	config   database.ChatModelConfig
	provider codersdk.ChatComputerUseProvider
	// reasoningEffort comes from the organization override, if any.
	reasoningEffort *string
}

// computerUseProviderForAIProviderType maps an AI provider type to the
// computer use tool flavor it supports.
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
	if override.Set {
		model = computerUseModel{config: override.Config, reasoningEffort: override.ReasoningEffort}
	} else {
		model.config, err = p.db.GetDefaultChatModelConfig(chatdCtx, organizationID)
		if errors.Is(err, sql.ErrNoRows) {
			return computerUseModel{}, `type "computer_use" is unavailable because no computer use model is configured and the organization has no default chat model`, nil
		}
		if err != nil {
			return computerUseModel{}, "", xerrors.Errorf("get default chat model config: %w", err)
		}
	}
	label := computerUseModelLabel(model.config)
	unavailable := fmt.Sprintf(
		`type "computer_use" is unavailable because model %s or its AI provider is disabled`,
		label,
	)
	if !model.config.Enabled || !model.config.AIProviderID.Valid {
		return computerUseModel{}, unavailable, nil
	}
	aiProvider, err := p.db.GetAIProviderByID(chatdCtx, model.config.AIProviderID.UUID)
	if errors.Is(err, sql.ErrNoRows) {
		return computerUseModel{}, unavailable, nil
	}
	if err != nil {
		return computerUseModel{}, "", xerrors.Errorf("get AI provider for model %s: %w", model.config.ID, err)
	}
	if !aiProvider.Enabled {
		return computerUseModel{}, unavailable, nil
	}
	provider, ok := computerUseProviderForAIProviderType(string(aiProvider.Type))
	if !ok {
		return computerUseModel{}, fmt.Sprintf(
			`type "computer_use" is unavailable because model %s uses AI provider %q of type %q; computer use requires an Anthropic or OpenAI AI provider`,
			label, aiProvider.Name, aiProvider.Type,
		), nil
	}
	model.provider = provider

	// resolveModelOverride already verified credentials for an override.
	if !override.Set {
		providerKeys, err := p.resolveUserProviderAPIKeys(chatdCtx, ownerID, aiProvider.ID)
		if err != nil {
			return computerUseModel{}, "", xerrors.Errorf("resolve provider API keys: %w", err)
		}
		if !userCanUseProviderKeys(providerKeys, string(provider)) {
			return computerUseModel{}, fmt.Sprintf(
				`type "computer_use" is unavailable because credentials for AI provider %q, used by model %s, are not configured`,
				aiProvider.Name, label,
			), nil
		}
	}
	return model, "", nil
}

type computerUseProviderToolOptions struct {
	provider         codersdk.ChatComputerUseProvider
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

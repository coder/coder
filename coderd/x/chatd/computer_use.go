package chatd

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	openaicomputeruse "github.com/coder/coder/v2/coderd/x/chatd/chatopenai/computeruse"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/quartz"
)

// computerUseConfigContext lets internal and worker callers read
// deployment-wide chat settings when they lack an HTTP-derived actor. HTTP
// handlers always carry an actor, so the AsChatd fallback never elevates user
// contexts and this function is a no-op in that path. The setting it gates is
// global and readable by any authenticated actor, not a back-door.
func computerUseConfigContext(ctx context.Context) context.Context {
	if _, ok := dbauthz.ActorFromContext(ctx); ok {
		return ctx
	}
	//nolint:gocritic // Worker contexts may lack an actor.
	return dbauthz.AsChatd(ctx)
}

// computerUseConfig is the deployment-wide computer-use configuration.
type computerUseConfig struct {
	provider      codersdk.ChatComputerUseProvider
	modelProvider string
	modelName     string
	// aiProviderID is the AI provider selected for computer use. When
	// invalid, computer use routes to the first enabled AI provider of
	// the provider type.
	aiProviderID uuid.NullUUID
}

func (p *Server) computerUseProviderAndModelFromConfig(
	ctx context.Context,
) (computerUseConfig, error) {
	selection, err := p.db.GetChatComputerUseSelection(computerUseConfigContext(ctx))
	if err != nil {
		return computerUseConfig{}, xerrors.Errorf("get computer use provider: %w", err)
	}

	provider := chattool.DefaultComputerUseProvider(
		codersdk.ChatComputerUseProvider(strings.TrimSpace(selection.Provider)),
	)

	modelProvider, modelName, ok := chattool.DefaultComputerUseModel(provider)
	if !ok {
		return computerUseConfig{}, xerrors.Errorf(
			"unknown computer-use provider %q configured in agents_computer_use_provider",
			provider,
		)
	}

	var aiProviderID uuid.NullUUID
	if trimmed := strings.TrimSpace(selection.AIProviderID); trimmed != "" {
		id, err := uuid.Parse(trimmed)
		if err != nil {
			return computerUseConfig{}, xerrors.Errorf(
				"invalid computer-use AI provider ID %q configured in agents_computer_use_ai_provider_id: %w",
				trimmed,
				err,
			)
		}
		aiProviderID = uuid.NullUUID{UUID: id, Valid: true}
	}

	return computerUseConfig{
		provider:      provider,
		modelProvider: modelProvider,
		modelName:     modelName,
		aiProviderID:  aiProviderID,
	}, nil
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

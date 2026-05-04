package chatd

import (
	"context"
	"strings"

	"charm.land/fantasy"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	openaicomputeruse "github.com/coder/coder/v2/coderd/x/chatd/chatopenai/computeruse"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
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

func (p *Server) computerUseProviderAndModelFromConfig(
	ctx context.Context,
) (provider, modelProvider, modelName string, err error) {
	rawProvider, err := p.db.GetChatComputerUseProvider(
		computerUseConfigContext(ctx),
	)
	if err != nil {
		return "", "", "", xerrors.Errorf("get computer use provider: %w", err)
	}

	provider = strings.TrimSpace(rawProvider)
	if provider == "" {
		provider = chattool.ComputerUseProviderAnthropic
	}

	modelProvider, modelName, ok := chattool.DefaultComputerUseModel(provider)
	if !ok {
		return "", "", "", xerrors.Errorf(
			"unknown computer-use provider %q configured in agents_computer_use_provider",
			provider,
		)
	}

	return provider, modelProvider, modelName, nil
}

func (p *Server) resolveComputerUseModel(
	ctx context.Context,
	chat database.Chat,
	providerKeys chatprovider.ProviderAPIKeys,
	computerUseProvider string,
	computerUseModelProvider string,
	computerUseModelName string,
) (
	model fantasy.LanguageModel,
	debugEnabled bool,
	resolvedProvider string,
	resolvedModel string,
	err error,
) {
	resolvedProvider, resolvedModel, err = chatprovider.ResolveModelWithProviderHint(
		computerUseModelName,
		computerUseModelProvider,
	)
	if err != nil {
		return nil, false, "", "", xerrors.Errorf(
			"resolve computer use model metadata for provider %q model %q: %w",
			computerUseProvider,
			computerUseModelName,
			err,
		)
	}

	model, debugEnabled, err = p.newDebugAwareModelFromConfig(
		ctx,
		chat,
		computerUseModelProvider,
		computerUseModelName,
		providerKeys,
		chatprovider.UserAgent(),
		chatprovider.CoderHeaders(chat),
	)
	if err != nil {
		return nil, false, "", "", xerrors.Errorf(
			"resolve computer use model for provider %q model %q: %w",
			computerUseProvider,
			computerUseModelName,
			err,
		)
	}

	return model, debugEnabled, resolvedProvider, resolvedModel, nil
}

type computerUseProviderToolOptions struct {
	provider         string
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
	if opts.provider == chattool.ComputerUseProviderOpenAI {
		// OpenAI computer-use image results need detail metadata so the model receives
		// the screenshot at original detail when the chat loop sends the tool result.
		providerTool.ResultProviderMetadata = openaicomputeruse.ResultProviderMetadata
	}

	return append(providerTools, providerTool), nil
}

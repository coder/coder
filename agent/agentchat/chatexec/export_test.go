package chatexec

import (
	"context"

	"charm.land/fantasy"
	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/quartz"
)

// SetBuildModel sets the model factory for testing.
func (e *Executor) SetBuildModel(fn func(string, string, chatprovider.ProviderAPIKeys, string, map[string]string) (fantasy.LanguageModel, error)) {
	e.buildModel = fn
}

// SetRunLoop sets the chatloop runner for testing.
func (e *Executor) SetRunLoop(fn func(context.Context, chatloop.RunOptions) error) {
	e.runLoop = fn
}

// SetClock sets the clock used by the publish batcher for testing.
func (e *Executor) SetClock(clock quartz.Clock) {
	e.clock = clock
}

// BuildListTemplatesToolForTest exposes buildListTemplatesTool to tests.
func BuildListTemplatesToolForTest(
	client ChatRunnerClient,
	chatID uuid.UUID,
	leaseEpoch int64,
) fantasy.AgentTool {
	return buildListTemplatesTool(client, chatID, leaseEpoch)
}

// BuildReadTemplateToolForTest exposes buildReadTemplateTool to tests.
func BuildReadTemplateToolForTest(
	client ChatRunnerClient,
	chatID uuid.UUID,
	leaseEpoch int64,
) fantasy.AgentTool {
	return buildReadTemplateTool(client, chatID, leaseEpoch)
}

// LocalToolFromDefinitionForTest exposes localToolFromDefinition to tests.
func LocalToolFromDefinitionForTest(
	def agentsdk.ChatRunnerToolDefinition,
	getLocalConn func(context.Context) (workspacesdk.AgentConn, error),
) (fantasy.AgentTool, bool, error) {
	return localToolFromDefinition(def, getLocalConn)
}

// BuildDynamicToolsForTest exposes buildDynamicTools to tests.
func BuildDynamicToolsForTest(
	logger slog.Logger,
	defs []agentsdk.ChatRunnerToolDefinition,
) []fantasy.AgentTool {
	return buildDynamicTools(logger, defs)
}

// ControlPlaneToolFromDefinitionForTest exposes
// controlPlaneToolFromDefinition to tests.
func ControlPlaneToolFromDefinitionForTest(
	def agentsdk.ChatRunnerToolDefinition,
	client ChatRunnerClient,
	chatID uuid.UUID,
	leaseEpoch int64,
) (fantasy.AgentTool, bool) {
	return controlPlaneToolFromDefinition(def, client, chatID, leaseEpoch)
}

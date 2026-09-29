package coderd

import (
	"context"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/workspaceexec"
	"github.com/coder/coder/v2/coderd/wsbuilder"
	"github.com/coder/coder/v2/codersdk"
)

func (api *API) startWorkspaceExecutionController() {
	api.workspaceExecutionController = workspaceexec.NewController(workspaceexec.ControllerOptions{
		Database: api.Database, Clock: api.Clock, Logger: api.Logger.Named("workspace-execution"),
		DialAgent: func(ctx context.Context, id uuid.UUID) (workspaceexec.ControllerAgent, func(), error) {
			return api.agentProvider.AgentConn(ctx, id)
		},
		FileCache: api.FileCache, UsageChecker: func() wsbuilder.UsageChecker { return *api.BuildUsageChecker.Load() },
		Pubsub: api.Pubsub, DeploymentValues: api.DeploymentValues, Experiments: api.Experiments,
		AllowDeletion: api.DeploymentValues.WorkspaceExecutionCleanup.Value(),
		AuthorizeCollection: func(context.Context) error {
			if api.Entitlements.Enabled(codersdk.FeatureBrowserOnly) {
				return xerrors.New("Non-browser result collection is disabled by deployment policy.")
			}
			return nil
		},
	})
	api.workspaceExecutionController.Start(api.ctx)
}

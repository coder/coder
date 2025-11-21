package terraform

import (
	"context"
	"os"
	"time"

	"github.com/spf13/afero"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
)

type planAction[T planComplete] func(ctx, killCtx context.Context, env, vars []string, logr logSink, req *proto.PlanRequest) (T, error)
type genericPlanHelpers[T planComplete] struct {
	New           func() T
	AppendTimings func(c T, timings []*proto.Timing) T
	AppendModules func(c T, mods []*proto.Module) T
	Plan          func(e *executor) planAction[T]
}

type planComplete interface {
	*proto.PlanComplete | *proto.PreApplyPlanComplete
}

func genericPlan[T planComplete](helper genericPlanHelpers[T], s *server, sess *provisionersdk.Session, request *proto.PlanRequest, canceledOrComplete <-chan struct{}) (T, error) {
	ctx, span := s.startTrace(sess.Context(), tracing.FuncName())
	defer span.End()
	ctx, cancel, killCtx, kill := s.setupContexts(ctx, canceledOrComplete)
	defer cancel()
	defer kill()

	e := s.executor(sess.Files, database.ProvisionerJobTimingStagePlan)
	if err := e.checkMinVersion(ctx); err != nil {
		return nil, err
	}
	logTerraformEnvVars(sess)

	// If we're destroying, exit early if there's no state. This is necessary to
	// avoid any cases where a workspace is "locked out" of terraform due to
	// e.g. bad template param values and cannot be deleted. This is just for
	// contingency, in the future we will try harder to prevent workspaces being
	// broken this hard.
	if request.Metadata.GetWorkspaceTransition() == proto.WorkspaceTransition_DESTROY && len(sess.Config.State) == 0 {
		sess.ProvisionLog(proto.LogLevel_INFO, "The terraform state does not exist, there is nothing to do")
		return helper.New(), nil
	}

	statefilePath := sess.Files.StateFilePath()
	if len(sess.Config.State) > 0 {
		err := os.WriteFile(statefilePath, sess.Config.State, 0o600)
		if err != nil {
			return nil, err
		}
	}

	err := CleanStaleTerraformPlugins(sess.Context(), s.cachePath, afero.NewOsFs(), time.Now(), s.logger)
	if err != nil {
		return nil, xerrors.Errorf("unable to clean stale Terraform plugins: %w", err)
	}

	s.logger.Debug(ctx, "running initialization")

	// The JSON output of `terraform init` doesn't include discrete fields for capturing timings of each plugin,
	// so we capture the whole init process.
	initTimings := newTimingAggregator(database.ProvisionerJobTimingStageInit)
	endStage := initTimings.startStage(database.ProvisionerJobTimingStageInit)

	err = e.init(ctx, killCtx, sess)
	endStage(err)
	if err != nil {
		s.logger.Debug(ctx, "init failed", slog.Error(err))

		// Special handling for "text file busy" c.f. https://github.com/coder/coder/issues/14726
		// We believe this might be due to some race condition that prevents the
		// terraform-provider-coder process from exiting.  When terraform tries to install the
		// provider during this init, it copies over the local cache. Normally this isn't an issue,
		// but if the terraform-provider-coder process is still running from a previous build, Linux
		// returns "text file busy" error when attempting to open the file.
		//
		// Capturing the stack trace from the process should help us figure out why it has not
		// exited.  We'll drop these diagnostics in a CRITICAL log so that operators are likely to
		// notice, and also because it indicates this provisioner could be permanently broken and
		// require a restart.
		var errTFB *textFileBusyError
		if xerrors.As(err, &errTFB) {
			stacktrace := tryGettingCoderProviderStacktrace(sess)
			s.logger.Critical(ctx, "init: text file busy",
				slog.Error(errTFB),
				slog.F("stderr", errTFB.stderr),
				slog.F("provider_coder_stacktrace", stacktrace),
			)
		}
		return nil, xerrors.Errorf("initialize terraform: %w", err)
	}

	modules, err := getModules(sess.Files)
	if err != nil {
		// We allow getModules to fail, as the result is used only
		// for telemetry purposes now.
		s.logger.Error(ctx, "failed to get modules from disk", slog.Error(err))
	}

	s.logger.Debug(ctx, "ran initialization")

	env, err := provisionEnv(sess.Config, request.Metadata, request.PreviousParameterValues, request.RichParameterValues, request.ExternalAuthProviders)
	if err != nil {
		return nil, xerrors.Errorf("setup env: %w", err)
	}
	env = otelEnvInject(ctx, env)

	vars, err := planVars(request)
	if err != nil {
		return nil, xerrors.Errorf("plan vars: %w", err)
	}

	resp, err := helper.Plan(e)(ctx, killCtx, env, vars, sess, request)
	if err != nil {
		return nil, xerrors.Errorf("plan: %w", err)
	}

	// Prepend init timings since they occur prior to plan timings.
	// Order is irrelevant; this is merely indicative.
	resp = helper.AppendTimings(resp, initTimings.aggregate())
	resp = helper.AppendModules(resp, modules)

	return resp, nil
}

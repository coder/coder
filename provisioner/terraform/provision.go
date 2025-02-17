package terraform

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/afero"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/terraform-provider-coder/v2/provider"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
)

const staleTerraformPluginRetention = 30 * 24 * time.Hour

func (s *server) setupContexts(parent context.Context, canceledOrComplete <-chan struct{}) (
	ctx context.Context, cancel func(), killCtx context.Context, kill func(),
) {
	// Create a context for graceful cancellation bound to the session
	// context. This ensures that we will perform graceful cancellation
	// even on connection loss.
	ctx, cancel = context.WithCancel(parent)

	// Create a separate context for forceful cancellation not tied to
	// the stream so that we can control when to terminate the process.
	killCtx, kill = context.WithCancel(context.Background())

	// Ensure processes are eventually cleaned up on graceful
	// cancellation or disconnect.
	go func() {
		<-ctx.Done()
		s.logger.Debug(ctx, "graceful context done")

		// TODO(mafredri): We should track this provision request as
		// part of graceful server shutdown procedure. Waiting on a
		// process here should delay provisioner/coder shutdown.
		t := time.NewTimer(s.exitTimeout)
		defer t.Stop()
		select {
		case <-t.C:
			s.logger.Debug(ctx, "exit timeout hit")
			kill()
		case <-killCtx.Done():
			s.logger.Debug(ctx, "kill context done")
		}
	}()

	// Process cancel
	go func() {
		<-canceledOrComplete
		s.logger.Debug(ctx, "canceledOrComplete closed")
		cancel()
	}()
	return ctx, cancel, killCtx, kill
}

func (s *server) Plan(
	sess *provisionersdk.Session, request *proto.PlanRequest, canceledOrComplete <-chan struct{},
) *proto.PlanComplete {
	ctx, span := s.startTrace(sess.Context(), tracing.FuncName())
	defer span.End()
	ctx, cancel, killCtx, kill := s.setupContexts(ctx, canceledOrComplete)
	defer cancel()
	defer kill()

	e := s.executor(sess.WorkDirectory, database.ProvisionerJobTimingStagePlan)
	if err := e.checkMinVersion(ctx); err != nil {
		return provisionersdk.PlanErrorf("%s", err.Error())
	}
	logTerraformEnvVars(sess)

	// If we're destroying, exit early if there's no state. This is necessary to
	// avoid any cases where a workspace is "locked out" of terraform due to
	// e.g. bad template param values and cannot be deleted. This is just for
	// contingency, in the future we will try harder to prevent workspaces being
	// broken this hard.
	if request.Metadata.GetWorkspaceTransition() == proto.WorkspaceTransition_DESTROY && len(sess.Config.State) == 0 {
		sess.ProvisionLog(proto.LogLevel_INFO, "The terraform state does not exist, there is nothing to do")
		return &proto.PlanComplete{}
	}

	statefilePath := getStateFilePath(sess.WorkDirectory)
	if len(sess.Config.State) > 0 {
		err := os.WriteFile(statefilePath, sess.Config.State, 0o600)
		if err != nil {
			return provisionersdk.PlanErrorf("write statefile %q: %s", statefilePath, err)
		}
	}

	err := CleanStaleTerraformPlugins(sess.Context(), s.cachePath, afero.NewOsFs(), time.Now(), s.logger)
	if err != nil {
		return provisionersdk.PlanErrorf("unable to clean stale Terraform plugins: %s", err)
	}

	s.logger.Debug(ctx, "running initialization")

	// The JSON output of `terraform init` doesn't include discrete fields for capturing timings of each plugin,
	// so we capture the whole init process.
	initTimings := newTimingAggregator(database.ProvisionerJobTimingStageInit)
	initTimings.ingest(createInitTimingsEvent(timingInitStart))

	err = e.init(ctx, killCtx, sess)
	if err != nil {
		initTimings.ingest(createInitTimingsEvent(timingInitErrored))

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
		return provisionersdk.PlanErrorf("initialize terraform: %s", err)
	}

	modules, err := getModules(sess.WorkDirectory)
	if err != nil {
		// We allow getModules to fail, as the result is used only
		// for telemetry purposes now.
		s.logger.Error(ctx, "failed to get modules from disk", slog.Error(err))
	}

	initTimings.ingest(createInitTimingsEvent(timingInitComplete))

	s.logger.Debug(ctx, "ran initialization")

	env, err := provisionEnv(sess.Config, request.Metadata, request.RichParameterValues, request.ExternalAuthProviders)
	if err != nil {
		return provisionersdk.PlanErrorf("setup env: %s", err)
	}

	vars, err := planVars(request)
	if err != nil {
		return provisionersdk.PlanErrorf("plan vars: %s", err)
	}

	resp, err := e.plan(
		ctx, killCtx, env, vars, sess,
		request.Metadata.GetWorkspaceTransition() == proto.WorkspaceTransition_DESTROY,
	)
	if err != nil {
		return provisionersdk.PlanErrorf("%s", err.Error())
	}

	// Prepend init timings since they occur prior to plan timings.
	// Order is irrelevant; this is merely indicative.
	resp.Timings = append(initTimings.aggregate(), resp.Timings...)
	resp.Modules = modules
	return resp
}

func (s *server) Apply(
	sess *provisionersdk.Session, request *proto.ApplyRequest, canceledOrComplete <-chan struct{},
) *proto.ApplyComplete {
	ctx, span := s.startTrace(sess.Context(), tracing.FuncName())
	defer span.End()
	ctx, cancel, killCtx, kill := s.setupContexts(ctx, canceledOrComplete)
	defer cancel()
	defer kill()

	e := s.executor(sess.WorkDirectory, database.ProvisionerJobTimingStageApply)
	if err := e.checkMinVersion(ctx); err != nil {
		return provisionersdk.ApplyErrorf("%s", err.Error())
	}
	logTerraformEnvVars(sess)

	// Exit early if there is no plan file. This is necessary to
	// avoid any cases where a workspace is "locked out" of terraform due to
	// e.g. bad template param values and cannot be deleted. This is just for
	// contingency, in the future we will try harder to prevent workspaces being
	// broken this hard.
	if request.Metadata.GetWorkspaceTransition() == proto.WorkspaceTransition_DESTROY && len(sess.Config.State) == 0 {
		sess.ProvisionLog(proto.LogLevel_INFO, "The terraform plan does not exist, there is nothing to do")
		return &proto.ApplyComplete{}
	}

	// Earlier in the session, Plan() will have written the state file and the plan file.
	statefilePath := getStateFilePath(sess.WorkDirectory)
	env, err := provisionEnv(sess.Config, request.Metadata, nil, nil)
	if err != nil {
		return provisionersdk.ApplyErrorf("provision env: %s", err)
	}
	resp, err := e.apply(
		ctx, killCtx, env, sess,
	)
	if err != nil {
		errorMessage := err.Error()
		// Terraform can fail and apply and still need to store it's state.
		// In this case, we return Complete with an explicit error message.
		stateData, _ := os.ReadFile(statefilePath)
		return &proto.ApplyComplete{
			State: stateData,
			Error: errorMessage,
		}
	}
	return resp
}

func planVars(plan *proto.PlanRequest) ([]string, error) {
	vars := []string{}
	for _, variable := range plan.VariableValues {
		vars = append(vars, fmt.Sprintf("%s=%s", variable.Name, variable.Value))
	}
	return vars, nil
}

func provisionEnv(
	config *proto.Config, metadata *proto.Metadata,
	richParams []*proto.RichParameterValue, externalAuth []*proto.ExternalAuthProvider,
) ([]string, error) {
	env := safeEnviron()
	ownerGroups, err := json.Marshal(metadata.GetWorkspaceOwnerGroups())
	if err != nil {
		return nil, xerrors.Errorf("marshal owner groups: %w", err)
	}

	env = append(env,
		"CODER_AGENT_URL="+metadata.GetCoderUrl(),
		"CODER_WORKSPACE_TRANSITION="+strings.ToLower(metadata.GetWorkspaceTransition().String()),
		"CODER_WORKSPACE_NAME="+metadata.GetWorkspaceName(),
		"CODER_WORKSPACE_OWNER="+metadata.GetWorkspaceOwner(),
		"CODER_WORKSPACE_OWNER_EMAIL="+metadata.GetWorkspaceOwnerEmail(),
		"CODER_WORKSPACE_OWNER_NAME="+metadata.GetWorkspaceOwnerName(),
		"CODER_WORKSPACE_OWNER_OIDC_ACCESS_TOKEN="+metadata.GetWorkspaceOwnerOidcAccessToken(),
		"CODER_WORKSPACE_OWNER_GROUPS="+string(ownerGroups),
		"CODER_WORKSPACE_OWNER_SSH_PUBLIC_KEY="+metadata.GetWorkspaceOwnerSshPublicKey(),
		"CODER_WORKSPACE_OWNER_SSH_PRIVATE_KEY="+metadata.GetWorkspaceOwnerSshPrivateKey(),
		"CODER_WORKSPACE_OWNER_LOGIN_TYPE="+metadata.GetWorkspaceOwnerLoginType(),
		"CODER_WORKSPACE_ID="+metadata.GetWorkspaceId(),
		"CODER_WORKSPACE_OWNER_ID="+metadata.GetWorkspaceOwnerId(),
		"CODER_WORKSPACE_OWNER_SESSION_TOKEN="+metadata.GetWorkspaceOwnerSessionToken(),
		"CODER_WORKSPACE_TEMPLATE_ID="+metadata.GetTemplateId(),
		"CODER_WORKSPACE_TEMPLATE_NAME="+metadata.GetTemplateName(),
		"CODER_WORKSPACE_TEMPLATE_VERSION="+metadata.GetTemplateVersion(),
		"CODER_WORKSPACE_BUILD_ID="+metadata.GetWorkspaceBuildId(),
	)
	for key, value := range provisionersdk.AgentScriptEnv() {
		env = append(env, key+"="+value)
	}
	for _, param := range richParams {
		env = append(env, provider.ParameterEnvironmentVariable(param.Name)+"="+param.Value)
	}
	for _, extAuth := range externalAuth {
		env = append(env, gitAuthAccessTokenEnvironmentVariable(extAuth.Id)+"="+extAuth.AccessToken)
		env = append(env, provider.ExternalAuthAccessTokenEnvironmentVariable(extAuth.Id)+"="+extAuth.AccessToken)
	}

	if config.ProvisionerLogLevel != "" {
		// TF_LOG=JSON enables all kind of logging: trace-debug-info-warn-error.
		// The idea behind using TF_LOG=JSON instead of TF_LOG=debug is ensuring the proper log format.
		env = append(env, "TF_LOG=JSON")
	}
	return env, nil
}

// tfEnvSafeToPrint is the set of terraform environment variables that we are quite sure won't contain secrets,
// and therefore it's ok to log their values
var tfEnvSafeToPrint = map[string]bool{
	"TF_LOG":                      true,
	"TF_LOG_PATH":                 true,
	"TF_INPUT":                    true,
	"TF_DATA_DIR":                 true,
	"TF_WORKSPACE":                true,
	"TF_IN_AUTOMATION":            true,
	"TF_REGISTRY_DISCOVERY_RETRY": true,
	"TF_REGISTRY_CLIENT_TIMEOUT":  true,
	"TF_CLI_CONFIG_FILE":          true,
	"TF_IGNORE":                   true,
}

func logTerraformEnvVars(sink logSink) {
	env := safeEnviron()
	for _, e := range env {
		if strings.HasPrefix(e, "TF_") {
			parts := strings.SplitN(e, "=", 2)
			if len(parts) != 2 {
				panic("safeEnviron() returned vars not in key=value form")
			}
			if !tfEnvSafeToPrint[parts[0]] {
				parts[1] = "<value redacted>"
			}
			sink.ProvisionLog(
				proto.LogLevel_WARN,
				fmt.Sprintf("terraform environment variable: %s=%s", parts[0], parts[1]),
			)
		}
	}
}

// tryGettingCoderProviderStacktrace attempts to dial a special pprof endpoint we added to
// terraform-provider-coder in https://github.com/coder/terraform-provider-coder/pull/295 which
// shipped in v1.0.4.  It will return the stacktraces of the provider, which will hopefully allow us
// to figure out why it hasn't exited.
func tryGettingCoderProviderStacktrace(sess *provisionersdk.Session) string {
	path := filepath.Clean(filepath.Join(sess.WorkDirectory, "../.coder/pprof"))
	sess.Logger.Info(sess.Context(), "attempting to get stack traces", slog.F("path", path))
	c := http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				d := net.Dialer{}
				return d.DialContext(ctx, "unix", path)
			},
		},
	}
	req, err := http.NewRequestWithContext(sess.Context(), http.MethodGet,
		"http://localhost/debug/pprof/goroutine?debug=2", nil)
	if err != nil {
		sess.Logger.Error(sess.Context(), "error creating GET request", slog.Error(err))
		return ""
	}
	resp, err := c.Do(req)
	if err != nil {
		// Only log at Info here, since we only added the pprof endpoint to terraform-provider-coder
		// in v1.0.4
		sess.Logger.Info(sess.Context(), "could not GET stack traces", slog.Error(err))
		return ""
	}
	defer resp.Body.Close()
	stacktraces, err := io.ReadAll(resp.Body)
	if err != nil {
		sess.Logger.Error(sess.Context(), "could not read stack traces", slog.Error(err))
	}
	return string(stacktraces)
}

// gitAuthAccessTokenEnvironmentVariable is copied from
// github.com/coder/terraform-provider-coder/provider.GitAuthAccessTokenEnvironmentVariable@v1.0.4.
// While removed in v2 of the provider, we keep this to support customers using older templates that
// depend on this environment variable. Once we are certain that no customers are still using v1 of
// the provider, we can remove this function.
func gitAuthAccessTokenEnvironmentVariable(id string) string {
	return fmt.Sprintf("CODER_GIT_AUTH_ACCESS_TOKEN_%s", id)
}

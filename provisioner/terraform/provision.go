package terraform

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/afero"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/terraform-provider-coder/provider"

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

	e := s.executor(sess.WorkDirectory)
	if err := e.checkMinVersion(ctx); err != nil {
		return provisionersdk.PlanErrorf(err.Error())
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
	err = e.init(ctx, killCtx, sess)
	if err != nil {
		s.logger.Debug(ctx, "init failed", slog.Error(err))
		return provisionersdk.PlanErrorf("initialize terraform: %s", err)
	}
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
		return provisionersdk.PlanErrorf(err.Error())
	}
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

	e := s.executor(sess.WorkDirectory)
	if err := e.checkMinVersion(ctx); err != nil {
		return provisionersdk.ApplyErrorf(err.Error())
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
		env = append(env, provider.GitAuthAccessTokenEnvironmentVariable(extAuth.Id)+"="+extAuth.AccessToken)
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

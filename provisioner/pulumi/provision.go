package pulumi

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/afero"
	"golang.org/x/xerrors"
	"gopkg.in/yaml.v3"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
)

const pulumiStackName = "coder"

type pulumiProject struct {
	Packages map[string]pulumiPackage `yaml:"packages"`
}

type pulumiPackage struct {
	Source     string   `yaml:"source"`
	Version    string   `yaml:"version"`
	Parameters []string `yaml:"parameters"`
}

func readPulumiProject(workDir string) (pulumiProject, error) {
	if strings.TrimSpace(workDir) == "" {
		return pulumiProject{}, xerrors.New("work directory must not be empty")
	}

	projectFiles := []string{"Pulumi.yaml", "Pulumi.yml"}
	for _, projectFileName := range projectFiles {
		projectFilePath := filepath.Join(workDir, projectFileName)
		projectFileContents, err := os.ReadFile(projectFilePath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return pulumiProject{}, xerrors.Errorf("read pulumi project file %q: %w", projectFilePath, err)
		}

		var project pulumiProject
		if err := yaml.Unmarshal(projectFileContents, &project); err != nil {
			return pulumiProject{}, xerrors.Errorf("parse pulumi project file %q: %w", projectFilePath, err)
		}
		return project, nil
	}

	return pulumiProject{}, xerrors.New("pulumi project file not found")
}

// Compile-time check that server implements the provisioner server interface.
var _ provisionersdk.Server = (*server)(nil)

func (s *server) Init(
	sess *provisionersdk.Session, request *provisionersdk.InitRequest, canceledOrComplete <-chan struct{},
) *proto.InitComplete {
	if s == nil {
		return provisionersdk.InitErrorf("server must not be nil")
	}
	if sess == nil {
		return provisionersdk.InitErrorf("session must not be nil")
	}
	if request == nil {
		return provisionersdk.InitErrorf("request must not be nil")
	}

	ctx, cancel, killCtx, kill := s.setupContexts(sess.Context(), canceledOrComplete)
	defer cancel()
	defer kill()

	e := s.executor(sess.Files, database.ProvisionerJobTimingStageInit)
	var stageErr error
	endStage := e.timings.startStage(database.ProvisionerJobTimingStageInit)
	defer func() {
		endStage(stageErr)
	}()

	stageErr = sess.Files.ExtractArchive(ctx, s.logger, afero.NewOsFs(), request.GetTemplateSourceArchive(), request.ModuleArchive)
	if stageErr != nil {
		return provisionersdk.InitErrorf("extract template archive: %s", stageErr)
	}

	backendDir := filepath.Join(sess.Files.WorkDirectory(), ".pulumi-backend")
	stageErr = os.MkdirAll(backendDir, 0o700)
	if stageErr != nil {
		return provisionersdk.InitErrorf("create pulumi backend directory %q: %s", backendDir, stageErr)
	}

	stageErr = e.login(ctx, killCtx)
	if stageErr != nil {
		return provisionersdk.InitErrorf("pulumi login: %s", stageErr)
	}

	stageErr = e.stackInit(ctx, killCtx, pulumiStackName)
	if stageErr != nil {
		return provisionersdk.InitErrorf("initialize pulumi stack %q: %s", pulumiStackName, stageErr)
	}

	project, err := readPulumiProject(e.files.WorkDirectory())
	if err != nil {
		stageErr = err
		return provisionersdk.InitErrorf("read pulumi project: %s", err)
	}
	packageNames := make([]string, 0, len(project.Packages))
	for packageName := range project.Packages {
		packageNames = append(packageNames, packageName)
	}
	sort.Strings(packageNames)
	for _, packageName := range packageNames {
		pkg := project.Packages[packageName]
		stageErr = e.packageAdd(ctx, killCtx, pkg.Source, pkg.Parameters)
		if stageErr != nil {
			return provisionersdk.InitErrorf("add pulumi package %q: %s", packageName, stageErr)
		}
	}

	stageErr = e.install(ctx, killCtx)
	if stageErr != nil {
		return provisionersdk.InitErrorf("install pulumi dependencies: %s", stageErr)
	}
	stageErr = nil

	return &proto.InitComplete{Timings: e.timings.aggregate()}
}

func (s *server) Parse(
	_ *provisionersdk.Session, request *proto.ParseRequest, _ <-chan struct{},
) *proto.ParseComplete {
	if s == nil {
		return provisionersdk.ParseErrorf("server must not be nil")
	}
	if request == nil {
		return provisionersdk.ParseErrorf("request must not be nil")
	}
	return &proto.ParseComplete{}
}

func (s *server) Plan(
	sess *provisionersdk.Session, request *proto.PlanRequest, canceledOrComplete <-chan struct{},
) *proto.PlanComplete {
	if s == nil {
		return provisionersdk.PlanErrorf("server must not be nil")
	}
	if sess == nil {
		return provisionersdk.PlanErrorf("session must not be nil")
	}
	if request == nil {
		return provisionersdk.PlanErrorf("request must not be nil")
	}
	metadata := request.GetMetadata()
	if metadata == nil {
		return provisionersdk.PlanErrorf("metadata must not be nil")
	}

	if metadata.GetWorkspaceTransition() == proto.WorkspaceTransition_DESTROY && len(request.GetState()) == 0 {
		sess.ProvisionLog(proto.LogLevel_INFO, "The Pulumi state does not exist, there is nothing to do")
		return &proto.PlanComplete{}
	}

	ctx, cancel, killCtx, kill := s.setupContexts(sess.Context(), canceledOrComplete)
	defer cancel()
	defer kill()

	e := s.executor(sess.Files, database.ProvisionerJobTimingStagePlan)
	var stageErr error
	endStage := e.timings.startStage(database.ProvisionerJobTimingStagePlan)
	defer func() {
		endStage(stageErr)
	}()

	stateData := request.GetState()
	if len(stateData) > 0 {
		stateFilePath := sess.Files.StateFilePath()
		stageErr = os.WriteFile(stateFilePath, stateData, 0o600)
		if stageErr != nil {
			return provisionersdk.PlanErrorf("write state file %q: %s", stateFilePath, stageErr)
		}

		stageErr = e.stackImport(ctx, killCtx, pulumiStackName, stateData)
		if stageErr != nil {
			return provisionersdk.PlanErrorf("import pulumi stack state: %s", stageErr)
		}
	}

	env, err := provisionEnv(
		sess.Config,
		metadata,
		request.GetPreviousParameterValues(),
		request.GetRichParameterValues(),
		request.GetExternalAuthProviders(),
	)
	if err != nil {
		stageErr = err
		return provisionersdk.PlanErrorf("setup env: %s", err)
	}

	destroy := metadata.GetWorkspaceTransition() == proto.WorkspaceTransition_DESTROY
	logr := logSink(func(level proto.LogLevel, line string) {
		sess.ProvisionLog(level, line)
	})
	previewBytes, err := e.preview(ctx, killCtx, pulumiStackName, destroy, env, logr)
	if err != nil {
		stageErr = err
		return provisionersdk.PlanErrorf("preview pulumi stack: %s", err)
	}

	planFilePath := sess.Files.PlanFilePath()
	stageErr = os.WriteFile(planFilePath, previewBytes, 0o600)
	if stageErr != nil {
		return provisionersdk.PlanErrorf("write plan file %q: %s", planFilePath, stageErr)
	}
	stageErr = nil

	return &proto.PlanComplete{Plan: previewBytes, Timings: e.timings.aggregate()}
}

func (s *server) Apply(
	sess *provisionersdk.Session, request *proto.ApplyRequest, canceledOrComplete <-chan struct{},
) *proto.ApplyComplete {
	if s == nil {
		return provisionersdk.ApplyErrorf("server must not be nil")
	}
	if sess == nil {
		return provisionersdk.ApplyErrorf("session must not be nil")
	}
	if request == nil {
		return provisionersdk.ApplyErrorf("request must not be nil")
	}
	metadata := request.GetMetadata()
	if metadata == nil {
		return provisionersdk.ApplyErrorf("metadata must not be nil")
	}

	stateFilePath := sess.Files.StateFilePath()
	destroy := metadata.GetWorkspaceTransition() == proto.WorkspaceTransition_DESTROY

	var importedState []byte
	if destroy {
		stateData, err := os.ReadFile(stateFilePath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				sess.ProvisionLog(proto.LogLevel_INFO, "The Pulumi state does not exist, there is nothing to do")
				return &proto.ApplyComplete{}
			}
			return provisionersdk.ApplyErrorf("read state file %q: %s", stateFilePath, err)
		}
		if len(stateData) == 0 {
			sess.ProvisionLog(proto.LogLevel_INFO, "The Pulumi state does not exist, there is nothing to do")
			return &proto.ApplyComplete{}
		}
		importedState = stateData
	} else {
		stateData, err := os.ReadFile(stateFilePath)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return provisionersdk.ApplyErrorf("read state file %q: %s", stateFilePath, err)
			}
		} else {
			importedState = stateData
		}
	}

	ctx, cancel, killCtx, kill := s.setupContexts(sess.Context(), canceledOrComplete)
	defer cancel()
	defer kill()

	e := s.executor(sess.Files, database.ProvisionerJobTimingStageApply)
	var stageErr error
	endStage := e.timings.startStage(database.ProvisionerJobTimingStageApply)
	defer func() {
		endStage(stageErr)
	}()

	if len(importedState) > 0 {
		stageErr = e.stackImport(ctx, killCtx, pulumiStackName, importedState)
		if stageErr != nil {
			return provisionersdk.ApplyErrorf("import pulumi stack state: %s", stageErr)
		}
	}

	env, err := provisionEnv(sess.Config, metadata, nil, nil, nil)
	if err != nil {
		stageErr = err
		return provisionersdk.ApplyErrorf("setup env: %s", err)
	}

	logr := logSink(func(level proto.LogLevel, line string) {
		sess.ProvisionLog(level, line)
	})

	var runErr error
	if destroy {
		runErr = e.destroy(ctx, killCtx, pulumiStackName, env, logr)
	} else {
		runErr = e.up(ctx, killCtx, pulumiStackName, env, logr)
	}

	stateData, exportErr := e.stackExport(ctx, killCtx, pulumiStackName)
	if exportErr != nil {
		if runErr == nil {
			stageErr = exportErr
			return provisionersdk.ApplyErrorf("export pulumi stack state: %s", exportErr)
		}
		stageErr = xerrors.Errorf("apply command failed: %w; export pulumi stack state: %v", runErr, exportErr)
		s.logger.Warn(ctx, "failed to export pulumi stack after apply error", slog.Error(exportErr))
		return &proto.ApplyComplete{State: stateData, Error: runErr.Error(), Timings: e.timings.aggregate()}
	}
	if runErr != nil {
		stageErr = runErr
		return &proto.ApplyComplete{State: stateData, Error: runErr.Error(), Timings: e.timings.aggregate()}
	}
	stageErr = nil

	return &proto.ApplyComplete{State: stateData, Timings: e.timings.aggregate()}
}

func (s *server) Graph(
	sess *provisionersdk.Session, request *proto.GraphRequest, canceledOrComplete <-chan struct{},
) *proto.GraphComplete {
	if s == nil {
		return provisionersdk.GraphError("server must not be nil")
	}
	if sess == nil {
		return provisionersdk.GraphError("session must not be nil")
	}
	if request == nil {
		return provisionersdk.GraphError("request must not be nil")
	}

	ctx, cancel, killCtx, kill := s.setupContexts(sess.Context(), canceledOrComplete)
	defer cancel()
	defer kill()

	e := s.executor(sess.Files, database.ProvisionerJobTimingStageGraph)
	var stageErr error
	endStage := e.timings.startStage(database.ProvisionerJobTimingStageGraph)
	defer func() {
		endStage(stageErr)
	}()

	var stateData []byte
	switch request.GetSource() {
	case proto.GraphSource_SOURCE_PLAN:
		planFilePath := sess.Files.PlanFilePath()
		_, stageErr = os.Stat(planFilePath)
		if stageErr != nil {
			return provisionersdk.GraphError("stat plan file %q: %s", planFilePath, stageErr)
		}

		stateFilePath := sess.Files.StateFilePath()
		stateData, stageErr = os.ReadFile(stateFilePath)
		if stageErr != nil {
			if os.IsNotExist(stageErr) {
				stageErr = nil
				return &proto.GraphComplete{Timings: e.timings.aggregate()}
			}
			return provisionersdk.GraphError("read state file %q: %s", stateFilePath, stageErr)
		}
	case proto.GraphSource_SOURCE_STATE:
		stateData, stageErr = e.stackExport(ctx, killCtx, pulumiStackName)
		if stageErr != nil {
			stateFilePath := sess.Files.StateFilePath()
			var readErr error
			stateData, readErr = os.ReadFile(stateFilePath)
			if readErr != nil {
				stageErr = xerrors.Errorf("export pulumi stack state: %w; read state file %q: %v", stageErr, stateFilePath, readErr)
				return provisionersdk.GraphError("%s", stageErr)
			}
			stageErr = nil
		}
	case proto.GraphSource_SOURCE_UNKNOWN:
		stageErr = xerrors.New("graph source must not be unknown")
		return provisionersdk.GraphError("%s", stageErr)
	default:
		stageErr = xerrors.Errorf("unknown graph source %q", request.GetSource().String())
		return provisionersdk.GraphError("%s", stageErr)
	}

	convertedState, err := ConvertState(ctx, stateData, s.logger)
	if err != nil {
		stageErr = err
		return provisionersdk.GraphError("convert pulumi state: %s", err)
	}
	stageErr = nil

	return &proto.GraphComplete{
		Resources:             convertedState.Resources,
		Parameters:            convertedState.Parameters,
		ExternalAuthProviders: convertedState.ExternalAuthProviders,
		Presets:               convertedState.Presets,
		HasAiTasks:            convertedState.HasAITasks,
		AiTasks:               convertedState.AITasks,
		HasExternalAgents:     convertedState.HasExternalAgents,
		Timings:               e.timings.aggregate(),
	}
}

func provisionEnv(
	config *proto.Config,
	metadata *proto.Metadata,
	previousParams, richParams []*proto.RichParameterValue,
	externalAuth []*proto.ExternalAuthProvider,
) ([]string, error) {
	if config == nil {
		return nil, xerrors.New("config must not be nil")
	}
	if metadata == nil {
		return nil, xerrors.New("metadata must not be nil")
	}

	env := safeEnviron()

	ownerGroups, err := json.Marshal(metadata.GetWorkspaceOwnerGroups())
	if err != nil {
		return nil, xerrors.Errorf("marshal owner groups: %w", err)
	}

	ownerRbacRoles, err := json.Marshal(metadata.GetWorkspaceOwnerRbacRoles())
	if err != nil {
		return nil, xerrors.Errorf("marshal owner rbac roles: %w", err)
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
		"CODER_WORKSPACE_OWNER_RBAC_ROLES="+string(ownerRbacRoles),
		"CODER_WORKSPACE_ID="+metadata.GetWorkspaceId(),
		"CODER_WORKSPACE_OWNER_ID="+metadata.GetWorkspaceOwnerId(),
		"CODER_WORKSPACE_OWNER_SESSION_TOKEN="+metadata.GetWorkspaceOwnerSessionToken(),
		"CODER_WORKSPACE_TEMPLATE_ID="+metadata.GetTemplateId(),
		"CODER_WORKSPACE_TEMPLATE_NAME="+metadata.GetTemplateName(),
		"CODER_WORKSPACE_TEMPLATE_VERSION="+metadata.GetTemplateVersion(),
		"CODER_WORKSPACE_BUILD_ID="+metadata.GetWorkspaceBuildId(),
		"CODER_TASK_ID="+metadata.GetTaskId(),
		"CODER_TASK_PROMPT="+metadata.GetTaskPrompt(),
	)
	if metadata.GetPrebuiltWorkspaceBuildStage().IsPrebuild() {
		env = append(env, "CODER_WORKSPACE_IS_PREBUILD=true")
	}
	if metadata.GetPrebuiltWorkspaceBuildStage().IsPrebuiltWorkspaceClaim() {
		env = append(env, "CODER_WORKSPACE_IS_PREBUILD_CLAIM=true")
	}

	tokens := metadata.GetRunningAgentAuthTokens()
	if len(tokens) == 1 {
		env = append(env, "CODER_RUNNING_WORKSPACE_AGENT_TOKEN="+tokens[0].Token)
	} else {
		for _, t := range tokens {
			env = append(env, "CODER_RUNNING_WORKSPACE_AGENT_TOKEN_"+t.AgentId+"="+t.Token)
		}
	}

	for key, value := range provisionersdk.AgentScriptEnv() {
		env = append(env, key+"="+value)
	}
	for _, param := range previousParams {
		env = append(env, "CODER_PARAMETER_PREVIOUS_"+param.Name+"="+param.Value)
	}
	for _, param := range richParams {
		env = append(env, "CODER_PARAMETER_"+param.Name+"="+param.Value)
	}
	for _, extAuth := range externalAuth {
		env = append(env, "CODER_GIT_AUTH_ACCESS_TOKEN_"+extAuth.Id+"="+extAuth.AccessToken)
		env = append(env, "CODER_EXTERNAL_AUTH_ACCESS_TOKEN_"+extAuth.Id+"="+extAuth.AccessToken)
	}

	return env, nil
}

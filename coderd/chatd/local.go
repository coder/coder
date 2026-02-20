package chatd

import (
	"archive/tar"
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"cdr.dev/slog/v3"
	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"tailscale.com/util/singleflight"

	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/provisionerdserver"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
)

const (
	localChatNameSuffixLength    = 8
	localChatTemplateNamePrefix  = "chat-local-tpl-"
	localChatWorkspaceNamePrefix = "chat-local-ws-"

	localChatBootstrapTimeout = 2 * time.Minute
	localChatPollInterval     = time.Second

	localChatAgentLaunchCooldown = 10 * time.Second

	localChatExternalResourceName = "main"
	localChatExternalAgentName    = "local-agent"
)

type LocalWorkspaceBinding struct {
	WorkspaceID      uuid.UUID
	WorkspaceAgentID uuid.UUID
}

type LocalServiceOptions struct {
	Logger       slog.Logger
	Database     database.Store
	AccessURL    *url.URL
	HTTPClient   *http.Client
	DeploymentID string
	AgentStarter localChatAgentStarter
}

type localChatExternalAgent struct {
	ID     uuid.UUID
	Name   string
	Status codersdk.WorkspaceAgentStatus
}

type localChatAgentStartParams struct {
	WorkspaceID uuid.UUID
	AgentID     uuid.UUID
	Credentials codersdk.ExternalAgentCredentials
	AgentName   string
}

type localChatAgentStarter func(localChatAgentStartParams) (io.Closer, error)

type LocalService struct {
	logger       slog.Logger
	db           database.Store
	accessURL    *url.URL
	httpClient   *http.Client
	deploymentID string

	localChatAgents *localChatAgentManager
	agentLaunches   *localChatAgentLaunchLimiter

	localChatWorkspaceBootstrapGroup singleflight.Group[string, LocalWorkspaceBinding]
	localChatAgentLaunchGroup        singleflight.Group[string, struct{}]
}

func NewLocalService(options LocalServiceOptions) *LocalService {
	service := &LocalService{
		logger:       options.Logger,
		db:           options.Database,
		accessURL:    options.AccessURL,
		httpClient:   options.HTTPClient,
		deploymentID: options.DeploymentID,
		agentLaunches: newLocalChatAgentLaunchLimiter(
			localChatAgentLaunchCooldown,
		),
	}

	starter := options.AgentStarter
	if starter == nil {
		starter = service.launchLocalChatAgentInProcess
	}
	service.localChatAgents = newLocalChatAgentManager(starter)

	return service
}

func (s *LocalService) Close() error {
	if s == nil || s.localChatAgents == nil {
		return nil
	}
	return s.localChatAgents.CloseAll()
}

type localChatManagedAgent struct {
	runtime io.Closer
	closed  bool
}

type localChatAgentManager struct {
	mu      sync.Mutex
	agents  map[uuid.UUID]*localChatManagedAgent
	startFn localChatAgentStarter
}

func newLocalChatAgentManager(startFn localChatAgentStarter) *localChatAgentManager {
	return &localChatAgentManager{
		agents:  make(map[uuid.UUID]*localChatManagedAgent),
		startFn: startFn,
	}
}

func (m *localChatAgentManager) Start(params localChatAgentStartParams) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if managed, ok := m.agents[params.AgentID]; ok && !managed.closed {
		return nil
	}
	if m.startFn == nil {
		return xerrors.New("local chat agent manager start function is not configured")
	}

	runtime, err := m.startFn(params)
	if err != nil {
		return err
	}
	if runtime == nil {
		return xerrors.New("local chat agent manager starter returned a nil runtime")
	}

	m.agents[params.AgentID] = &localChatManagedAgent{
		runtime: runtime,
	}
	return nil
}

func (m *localChatAgentManager) HasRunning(agentID uuid.UUID) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	managed, ok := m.agents[agentID]
	return ok && managed != nil && !managed.closed && managed.runtime != nil
}

func (m *localChatAgentManager) Close(agentID uuid.UUID) error {
	m.mu.Lock()
	managed, ok := m.agents[agentID]
	if !ok {
		m.mu.Unlock()
		return nil
	}
	delete(m.agents, agentID)
	if managed.closed || managed.runtime == nil {
		m.mu.Unlock()
		return nil
	}
	managed.closed = true
	runtime := managed.runtime
	m.mu.Unlock()

	if err := runtime.Close(); err != nil {
		return xerrors.Errorf("close local chat agent runtime %q: %w", agentID, err)
	}
	return nil
}

func (m *localChatAgentManager) CloseAll() error {
	m.mu.Lock()
	managed := m.agents
	m.agents = make(map[uuid.UUID]*localChatManagedAgent)
	m.mu.Unlock()

	var errs error
	for agentID, runtime := range managed {
		if runtime == nil || runtime.closed || runtime.runtime == nil {
			continue
		}
		runtime.closed = true
		if err := runtime.runtime.Close(); err != nil {
			errs = errors.Join(errs, xerrors.Errorf(
				"close local chat agent runtime %q: %w",
				agentID,
				err,
			))
		}
	}
	return errs
}

func (s *LocalService) EnsureWorkspaceBinding(
	ctx context.Context,
	ownerID uuid.UUID,
	sessionToken string,
) (LocalWorkspaceBinding, error) {
	if s == nil {
		return LocalWorkspaceBinding{}, xerrors.New("local chat service is not configured")
	}

	bootstrapKey := fmt.Sprintf("%s:%s", s.deploymentID, ownerID)
	binding, err, _ := s.localChatWorkspaceBootstrapGroup.Do(bootstrapKey, func() (LocalWorkspaceBinding, error) {
		return s.ensureLocalChatWorkspaceBindingSingleflight(
			ctx,
			ownerID,
			sessionToken,
		)
	})
	if err != nil {
		return LocalWorkspaceBinding{}, err
	}
	return binding, nil
}

func (s *LocalService) ensureLocalChatWorkspaceBindingSingleflight(
	ctx context.Context,
	ownerID uuid.UUID,
	sessionToken string,
) (LocalWorkspaceBinding, error) {
	bootstrapCtx, cancel := context.WithTimeout(ctx, localChatBootstrapTimeout)
	defer cancel()

	client, err := s.localChatClientFromSessionToken(sessionToken)
	if err != nil {
		return LocalWorkspaceBinding{}, err
	}

	user, err := client.User(bootstrapCtx, codersdk.Me)
	if err != nil {
		return LocalWorkspaceBinding{}, xerrors.Errorf("get current user: %w", err)
	}
	if user.ID != ownerID {
		return LocalWorkspaceBinding{}, xerrors.New(
			"request token user does not match chat owner",
		)
	}

	organizationID, err := s.localChatOrganizationID(bootstrapCtx, user)
	if err != nil {
		return LocalWorkspaceBinding{}, err
	}

	template, err := s.ensureLocalChatTemplate(
		bootstrapCtx,
		client,
		organizationID,
		ownerID,
	)
	if err != nil {
		return LocalWorkspaceBinding{}, err
	}

	workspace, err := s.ensureLocalChatWorkspace(
		bootstrapCtx,
		client,
		template,
		ownerID,
	)
	if err != nil {
		return LocalWorkspaceBinding{}, err
	}

	agent, err := s.resolveLocalChatExternalAgent(
		bootstrapCtx,
		client,
		workspace,
	)
	if err != nil {
		return LocalWorkspaceBinding{}, err
	}

	if err := s.maybeLaunchLocalChatAgent(
		bootstrapCtx,
		workspace.ID,
		agent,
	); err != nil {
		return LocalWorkspaceBinding{}, err
	}

	return LocalWorkspaceBinding{
		WorkspaceID:      workspace.ID,
		WorkspaceAgentID: agent.ID,
	}, nil
}

func (s *LocalService) localChatClientFromSessionToken(
	sessionToken string,
) (*codersdk.Client, error) {
	if s == nil || s.accessURL == nil {
		return nil, xerrors.New("deployment access URL is not configured")
	}

	token := strings.TrimSpace(sessionToken)
	if token == "" {
		return nil, xerrors.New("session token is missing")
	}

	client := codersdk.New(s.accessURL, codersdk.WithSessionToken(token))
	if s.httpClient != nil {
		client.HTTPClient = s.httpClient
	}
	return client, nil
}

func (s *LocalService) localChatOrganizationID(
	ctx context.Context,
	user codersdk.User,
) (uuid.UUID, error) {
	if len(user.OrganizationIDs) == 0 {
		return uuid.Nil, xerrors.New("user is not a member of an organization")
	}

	defaultOrg, err := s.db.GetDefaultOrganization(ctx)
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, xerrors.Errorf("resolve default organization: %w", err)
	}
	if err == nil {
		for _, orgID := range user.OrganizationIDs {
			if orgID == defaultOrg.ID {
				return orgID, nil
			}
		}
	}

	return user.OrganizationIDs[0], nil
}

func (s *LocalService) ensureLocalChatTemplate(
	ctx context.Context,
	client *codersdk.Client,
	organizationID uuid.UUID,
	ownerID uuid.UUID,
) (codersdk.Template, error) {
	templateName := localChatTemplateName(ownerID)
	template, err := client.TemplateByName(ctx, organizationID, templateName)
	if err != nil {
		if !isCodersdkStatusCode(err, http.StatusNotFound) {
			return codersdk.Template{}, xerrors.Errorf(
				"resolve local chat template %q: %w",
				templateName,
				err,
			)
		}

		version, versionErr := s.createLocalChatTemplateVersion(
			ctx,
			client,
			organizationID,
			uuid.Nil,
		)
		if versionErr != nil {
			return codersdk.Template{}, versionErr
		}

		template, err = client.CreateTemplate(ctx, organizationID, codersdk.CreateTemplateRequest{
			Name:      templateName,
			VersionID: version.ID,
		})
		if err != nil {
			if !isCodersdkStatusCode(err, http.StatusConflict) {
				return codersdk.Template{}, xerrors.Errorf(
					"create local chat template %q: %w",
					templateName,
					err,
				)
			}

			template, err = client.TemplateByName(ctx, organizationID, templateName)
			if err != nil {
				return codersdk.Template{}, xerrors.Errorf(
					"resolve local chat template after conflict %q: %w",
					templateName,
					err,
				)
			}
		}
	}

	activeVersion, err := client.TemplateVersion(ctx, template.ActiveVersionID)
	if err != nil {
		return codersdk.Template{}, xerrors.Errorf(
			"resolve local chat template active version %q: %w",
			template.ActiveVersionID,
			err,
		)
	}
	if !activeVersion.HasExternalAgent {
		return codersdk.Template{}, xerrors.Errorf(
			"local chat template %q active version %q does not expose an external agent",
			templateName,
			activeVersion.ID,
		)
	}
	resources, err := client.TemplateVersionResources(ctx, activeVersion.ID)
	if err != nil {
		return codersdk.Template{}, xerrors.Errorf(
			"resolve local chat template active version %q resources: %w",
			activeVersion.ID,
			err,
		)
	}
	if !localChatTemplateResourcesProvideAgent(resources) {
		s.logger.Warn(ctx, "local chat template active version does not expose workspace agents; creating replacement active version",
			slog.F("template_id", template.ID),
			slog.F("template_version_id", activeVersion.ID),
			slog.F("template_name", templateName),
		)
		replacementVersion, replacementErr := s.createLocalChatTemplateVersion(
			ctx,
			client,
			organizationID,
			template.ID,
		)
		if replacementErr != nil {
			return codersdk.Template{}, replacementErr
		}
		if err := client.UpdateActiveTemplateVersion(
			ctx,
			template.ID,
			codersdk.UpdateActiveTemplateVersion{ID: replacementVersion.ID},
		); err != nil {
			return codersdk.Template{}, xerrors.Errorf(
				"update local chat template %q active version to %q: %w",
				templateName,
				replacementVersion.ID,
				err,
			)
		}
		template.ActiveVersionID = replacementVersion.ID
	}

	return template, nil
}

func (s *LocalService) createLocalChatTemplateVersion(
	ctx context.Context,
	client *codersdk.Client,
	organizationID uuid.UUID,
	templateID uuid.UUID,
) (codersdk.TemplateVersion, error) {
	provisioner, err := s.resolveLocalChatTemplateProvisioner(
		ctx,
		organizationID,
	)
	if err != nil {
		return codersdk.TemplateVersion{}, err
	}
	archive, err := s.localChatTemplateArchiveForProvisioner(ctx, provisioner)
	if err != nil {
		return codersdk.TemplateVersion{}, xerrors.Errorf(
			"build local chat template archive: %w",
			err,
		)
	}

	upload, err := client.Upload(ctx, codersdk.ContentTypeTar, bytes.NewReader(archive))
	if err != nil {
		return codersdk.TemplateVersion{}, xerrors.Errorf(
			"upload local chat template archive: %w",
			err,
		)
	}

	createReq := codersdk.CreateTemplateVersionRequest{
		FileID:        upload.ID,
		StorageMethod: codersdk.ProvisionerStorageMethodFile,
		Provisioner:   provisioner,
	}
	if templateID != uuid.Nil {
		createReq.TemplateID = templateID
	}
	version, err := client.CreateTemplateVersion(ctx, organizationID, createReq)
	if err != nil {
		return codersdk.TemplateVersion{}, xerrors.Errorf(
			"create local chat template version: %w",
			err,
		)
	}
	s.logger.Info(ctx, "created local chat template version",
		slog.F("organization_id", organizationID),
		slog.F("template_version_id", version.ID),
		slog.F("provisioner_job_id", version.Job.ID),
		slog.F("provisioner", provisioner),
		slog.F("template_id", templateID),
	)

	if err := s.ensureLocalChatTemplateVersionProvisionable(
		ctx,
		organizationID,
		version.Job.ID,
		provisioner,
	); err != nil {
		if cancelErr := client.CancelTemplateVersion(ctx, version.ID); cancelErr != nil {
			s.logger.Warn(ctx, "failed to cancel local chat template version after provisioning preflight failed",
				slog.F("template_version_id", version.ID),
				slog.F("provisioner_job_id", version.Job.ID),
				slog.Error(cancelErr),
			)
		}
		return codersdk.TemplateVersion{}, err
	}

	version, err = waitForLocalChatTemplateVersionCompletion(ctx, client, version.ID)
	if err != nil {
		return codersdk.TemplateVersion{}, err
	}
	return version, nil
}

func (s *LocalService) resolveLocalChatTemplateProvisioner(
	ctx context.Context,
	organizationID uuid.UUID,
) (codersdk.ProvisionerType, error) {
	daemons, err := s.db.GetProvisionerDaemonsWithStatusByOrganization(
		dbauthz.AsSystemRestricted(ctx),
		database.GetProvisionerDaemonsWithStatusByOrganizationParams{
			OrganizationID:  organizationID,
			StaleIntervalMS: provisionerdserver.StaleInterval.Milliseconds(),
			Statuses: []database.ProvisionerDaemonStatus{
				database.ProvisionerDaemonStatusIdle,
				database.ProvisionerDaemonStatusBusy,
			},
		},
	)
	if err != nil {
		return "", xerrors.Errorf(
			"resolve online provisioner daemons for local chat in organization %q: %w",
			organizationID,
			err,
		)
	}

	hasTerraform := false
	hasEcho := false
	for _, daemon := range daemons {
		for _, daemonProvisioner := range daemon.ProvisionerDaemon.Provisioners {
			switch daemonProvisioner {
			case database.ProvisionerTypeTerraform:
				hasTerraform = true
			case database.ProvisionerTypeEcho:
				hasEcho = true
			}
		}
	}

	switch {
	case hasTerraform:
		return codersdk.ProvisionerTypeTerraform, nil
	case hasEcho:
		return codersdk.ProvisionerTypeEcho, nil
	default:
		return "", xerrors.Errorf(
			"organization %q has no online provisioner daemons supporting %q or %q",
			organizationID,
			codersdk.ProvisionerTypeTerraform,
			codersdk.ProvisionerTypeEcho,
		)
	}
}

func (s *LocalService) localChatTemplateArchiveForProvisioner(
	ctx context.Context,
	provisioner codersdk.ProvisionerType,
) ([]byte, error) {
	switch provisioner {
	case codersdk.ProvisionerTypeEcho:
		return echo.TarWithOptions(
			ctx,
			s.logger.Named("chat-local-template"),
			localChatTemplateResponses(runtime.GOOS, runtime.GOARCH),
		)
	case codersdk.ProvisionerTypeTerraform:
		return localChatTerraformTemplateArchive(localChatExternalAgentName)
	default:
		return nil, xerrors.Errorf(
			"local chat template provisioner %q is not supported",
			provisioner,
		)
	}
}

func localChatTerraformTemplateArchive(agentName string) ([]byte, error) {
	agentResourceName := sanitizeLocalChatTerraformIdentifier(agentName)
	if agentResourceName == "" {
		agentResourceName = "localagent"
	}
	mainTF := fmt.Sprintf(`terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = ">= 2.7.0"
    }
  }
}

resource "coder_agent" "%s" {
  os   = %q
  arch = %q
}

resource "coder_external_agent" "%s" {
  agent_id = coder_agent.%s.id
}
`, agentResourceName, runtime.GOOS, runtime.GOARCH, localChatExternalResourceName, agentResourceName)

	return localChatTemplateArchive(map[string]string{
		"main.tf": mainTF,
	})
}

func sanitizeLocalChatTerraformIdentifier(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(trimmed) + 1)
	for _, r := range trimmed {
		if r >= 'A' && r <= 'Z' {
			r = r - 'A' + 'a'
		}
		valid := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if !valid {
			continue
		}
		b.WriteRune(r)
	}
	if b.Len() == 0 {
		return ""
	}
	identifier := b.String()
	first := identifier[0]
	if first >= '0' && first <= '9' {
		return "a" + identifier
	}
	return b.String()
}

func localChatTemplateArchive(files map[string]string) ([]byte, error) {
	var buffer bytes.Buffer
	tw := tar.NewWriter(&buffer)

	fileNames := make([]string, 0, len(files))
	for fileName := range files {
		fileNames = append(fileNames, fileName)
	}
	sort.Strings(fileNames)

	for _, fileName := range fileNames {
		fileContent := []byte(files[fileName])
		header := &tar.Header{
			Name: fileName,
			Mode: 0o644,
			Size: int64(len(fileContent)),
		}
		if err := tw.WriteHeader(header); err != nil {
			_ = tw.Close()
			return nil, xerrors.Errorf(
				"write local chat template file header %q: %w",
				fileName,
				err,
			)
		}
		if _, err := tw.Write(fileContent); err != nil {
			_ = tw.Close()
			return nil, xerrors.Errorf(
				"write local chat template file %q: %w",
				fileName,
				err,
			)
		}
	}
	if err := tw.Close(); err != nil {
		return nil, xerrors.Errorf("close local chat template archive writer: %w", err)
	}
	return buffer.Bytes(), nil
}

func (s *LocalService) ensureLocalChatTemplateVersionProvisionable(
	ctx context.Context,
	organizationID uuid.UUID,
	jobID uuid.UUID,
	provisioner codersdk.ProvisionerType,
) error {
	if jobID == uuid.Nil {
		return xerrors.New("local chat template version job ID is empty")
	}
	eligible, err := s.db.GetEligibleProvisionerDaemonsByProvisionerJobIDs(
		dbauthz.AsSystemRestricted(ctx),
		[]uuid.UUID{jobID},
	)
	if err != nil {
		return xerrors.Errorf(
			"resolve eligible provisioner daemons for local chat template version job %q: %w",
			jobID,
			err,
		)
	}
	if len(eligible) == 0 {
		return xerrors.Errorf(
			"local chat template version job %q has no eligible provisioner daemons in organization %q; ensure at least one connected provisioner daemon supports %q and matching tags",
			jobID,
			organizationID,
			provisioner,
		)
	}

	staleCutoff := time.Now().Add(-provisionerdserver.StaleInterval)
	for _, row := range eligible {
		lastSeenAt := row.ProvisionerDaemon.LastSeenAt
		if !lastSeenAt.Valid {
			continue
		}
		if lastSeenAt.Time.Before(staleCutoff) {
			continue
		}
		return nil
	}

	return xerrors.Errorf(
		"local chat template version job %q has no online eligible provisioner daemons in organization %q (stale interval %s)",
		jobID,
		organizationID,
		provisionerdserver.StaleInterval,
	)
}

func localChatTemplateResponses(agentOS string, agentArch string) *echo.Responses {
	return &echo.Responses{
		Parse: echo.ParseComplete,
		ProvisionGraph: []*proto.Response{{
			Type: &proto.Response_Graph{
				Graph: &proto.GraphComplete{
					Resources: []*proto.Resource{{
						Type: "coder_external_agent",
						Name: localChatExternalResourceName,
						Agents: []*proto.Agent{{
							Name:            localChatExternalAgentName,
							OperatingSystem: agentOS,
							Architecture:    agentArch,
						}},
					}},
					HasExternalAgents: true,
				},
			},
		}},
		ProvisionApply: echo.ApplyComplete,
	}
}

func localChatTemplateResourcesProvideAgent(
	resources []codersdk.WorkspaceResource,
) bool {
	for _, resource := range resources {
		if len(resource.Agents) == 0 {
			continue
		}
		return true
	}
	return false
}

func waitForLocalChatTemplateVersionCompletion(
	ctx context.Context,
	client *codersdk.Client,
	versionID uuid.UUID,
) (codersdk.TemplateVersion, error) {
	ticker := time.NewTicker(localChatPollInterval)
	defer ticker.Stop()

	for {
		version, err := client.TemplateVersion(ctx, versionID)
		if err != nil {
			return codersdk.TemplateVersion{}, xerrors.Errorf(
				"get local chat template version %q: %w",
				versionID,
				err,
			)
		}

		switch version.Job.Status {
		case codersdk.ProvisionerJobPending,
			codersdk.ProvisionerJobRunning,
			codersdk.ProvisionerJobCanceling:
			select {
			case <-ctx.Done():
				return codersdk.TemplateVersion{}, xerrors.Errorf(
					"wait for local chat template version %q (job %q, last status %q): %w",
					versionID,
					version.Job.ID,
					version.Job.Status,
					ctx.Err(),
				)
			case <-ticker.C:
			}
			continue
		case codersdk.ProvisionerJobSucceeded:
			return version, nil
		default:
			return codersdk.TemplateVersion{}, xerrors.Errorf(
				"local chat template version %q finished with status %q: %s",
				version.ID,
				version.Job.Status,
				strings.TrimSpace(version.Job.Error),
			)
		}
	}
}

func (s *LocalService) ensureLocalChatWorkspace(
	ctx context.Context,
	client *codersdk.Client,
	template codersdk.Template,
	ownerID uuid.UUID,
) (codersdk.Workspace, error) {
	workspaceName := localChatWorkspaceName(ownerID)
	workspace, err := client.WorkspaceByOwnerAndName(
		ctx,
		codersdk.Me,
		workspaceName,
		codersdk.WorkspaceOptions{},
	)
	if err != nil {
		if !isCodersdkStatusCode(err, http.StatusNotFound) {
			return codersdk.Workspace{}, xerrors.Errorf(
				"resolve local chat workspace %q: %w",
				workspaceName,
				err,
			)
		}

		workspace, err = client.CreateUserWorkspace(ctx, codersdk.Me, codersdk.CreateWorkspaceRequest{
			Name:       workspaceName,
			TemplateID: template.ID,
		})
		if err != nil {
			if !isCodersdkStatusCode(err, http.StatusConflict) {
				return codersdk.Workspace{}, xerrors.Errorf(
					"create local chat workspace %q: %w",
					workspaceName,
					err,
				)
			}

			workspace, err = client.WorkspaceByOwnerAndName(
				ctx,
				codersdk.Me,
				workspaceName,
				codersdk.WorkspaceOptions{},
			)
			if err != nil {
				return codersdk.Workspace{}, xerrors.Errorf(
					"resolve local chat workspace after conflict %q: %w",
					workspaceName,
					err,
				)
			}
		}
	}
	if workspace.TemplateID != template.ID {
		return codersdk.Workspace{}, xerrors.Errorf(
			"local chat workspace %q is bound to template %q, expected %q",
			workspaceName,
			workspace.TemplateID,
			template.ID,
		)
	}

	workspace, err = s.ensureLocalChatWorkspaceReady(
		ctx,
		client,
		workspace.ID,
		template.ActiveVersionID,
	)
	if err != nil {
		return codersdk.Workspace{}, err
	}
	return workspace, nil
}

func (s *LocalService) ensureLocalChatWorkspaceReady(
	ctx context.Context,
	client *codersdk.Client,
	workspaceID uuid.UUID,
	templateVersionID uuid.UUID,
) (codersdk.Workspace, error) {
	workspace, err := waitForLocalChatWorkspaceBuildCompletion(ctx, client, workspaceID)
	if err != nil {
		return codersdk.Workspace{}, err
	}

	if workspace.LatestBuild.Job.Status == codersdk.ProvisionerJobFailed ||
		workspace.LatestBuild.Job.Status == codersdk.ProvisionerJobCanceled {
		_, err := client.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			Transition: codersdk.WorkspaceTransitionStart,
		})
		if err != nil {
			return codersdk.Workspace{}, xerrors.Errorf(
				"rebuild local chat workspace %q: %w",
				workspace.ID,
				err,
			)
		}

		workspace, err = waitForLocalChatWorkspaceBuildCompletion(
			ctx,
			client,
			workspaceID,
		)
		if err != nil {
			return codersdk.Workspace{}, err
		}
	}

	if workspace.LatestBuild.Job.Status != codersdk.ProvisionerJobSucceeded {
		return codersdk.Workspace{}, xerrors.Errorf(
			"local chat workspace %q build status is %q: %s",
			workspace.ID,
			workspace.LatestBuild.Job.Status,
			strings.TrimSpace(workspace.LatestBuild.Job.Error),
		)
	}

	hasAgents, err := s.localChatWorkspaceHasAgentsInLatestBuild(ctx, workspace.ID)
	if err != nil {
		return codersdk.Workspace{}, err
	}
	if !hasAgents {
		s.logger.Warn(ctx, "local chat workspace latest build has no agents; rebuilding with active local template version",
			slog.F("workspace_id", workspace.ID),
			slog.F("template_version_id", templateVersionID),
		)
		_, err := client.CreateWorkspaceBuild(
			ctx,
			workspace.ID,
			codersdk.CreateWorkspaceBuildRequest{
				Transition:        codersdk.WorkspaceTransitionStart,
				TemplateVersionID: templateVersionID,
			},
		)
		if err != nil {
			return codersdk.Workspace{}, xerrors.Errorf(
				"rebuild local chat workspace %q with template version %q: %w",
				workspace.ID,
				templateVersionID,
				err,
			)
		}
		workspace, err = waitForLocalChatWorkspaceBuildCompletion(
			ctx,
			client,
			workspace.ID,
		)
		if err != nil {
			return codersdk.Workspace{}, err
		}
		if workspace.LatestBuild.Job.Status != codersdk.ProvisionerJobSucceeded {
			return codersdk.Workspace{}, xerrors.Errorf(
				"local chat workspace %q rebuild with template version %q finished with status %q: %s",
				workspace.ID,
				templateVersionID,
				workspace.LatestBuild.Job.Status,
				strings.TrimSpace(workspace.LatestBuild.Job.Error),
			)
		}
	}
	return workspace, nil
}

func (s *LocalService) localChatWorkspaceHasAgentsInLatestBuild(
	ctx context.Context,
	workspaceID uuid.UUID,
) (bool, error) {
	agents, err := s.db.GetWorkspaceAgentsInLatestBuildByWorkspaceID(
		ctx,
		workspaceID,
	)
	if err != nil {
		return false, xerrors.Errorf(
			"resolve local chat workspace %q agents in latest build: %w",
			workspaceID,
			err,
		)
	}
	return len(agents) > 0, nil
}

func waitForLocalChatWorkspaceBuildCompletion(
	ctx context.Context,
	client *codersdk.Client,
	workspaceID uuid.UUID,
) (codersdk.Workspace, error) {
	ticker := time.NewTicker(localChatPollInterval)
	defer ticker.Stop()

	for {
		workspace, err := client.Workspace(ctx, workspaceID)
		if err != nil {
			return codersdk.Workspace{}, xerrors.Errorf(
				"get local chat workspace %q: %w",
				workspaceID,
				err,
			)
		}
		if !workspace.LatestBuild.Job.Status.Active() {
			return workspace, nil
		}

		select {
		case <-ctx.Done():
			return codersdk.Workspace{}, xerrors.Errorf(
				"wait for local chat workspace %q build completion: %w",
				workspaceID,
				ctx.Err(),
			)
		case <-ticker.C:
		}
	}
}

func (s *LocalService) resolveLocalChatExternalAgent(
	ctx context.Context,
	client *codersdk.Client,
	workspace codersdk.Workspace,
) (localChatExternalAgent, error) {
	agent, ok := localChatExternalAgentFromResources(workspace.LatestBuild.Resources)
	if ok {
		return agent, nil
	}
	agent, ok, err := s.localChatExternalAgentFromWorkspaceAgentsInDB(
		ctx,
		workspace.ID,
	)
	if err != nil {
		return localChatExternalAgent{}, err
	}
	if ok {
		return agent, nil
	}
	if workspace.LatestBuild.ID == uuid.Nil {
		return localChatExternalAgent{}, xerrors.Errorf(
			"local chat workspace %q has no latest build",
			workspace.ID,
		)
	}

	build, err := client.WorkspaceBuild(ctx, workspace.LatestBuild.ID)
	if err != nil {
		return localChatExternalAgent{}, xerrors.Errorf(
			"resolve local chat workspace build %q: %w",
			workspace.LatestBuild.ID,
			err,
		)
	}
	agent, ok = localChatExternalAgentFromResources(build.Resources)
	if !ok {
		agent, ok, err = s.localChatExternalAgentFromWorkspaceAgentsInDB(
			ctx,
			workspace.ID,
		)
		if err != nil {
			return localChatExternalAgent{}, err
		}
		if !ok {
			return localChatExternalAgent{}, xerrors.Errorf(
				"local chat workspace %q does not expose an external agent",
				workspace.ID,
			)
		}
	}
	return agent, nil
}

func (s *LocalService) localChatExternalAgentFromWorkspaceAgentsInDB(
	ctx context.Context,
	workspaceID uuid.UUID,
) (localChatExternalAgent, bool, error) {
	agents, err := s.db.GetWorkspaceAgentsInLatestBuildByWorkspaceID(
		ctx,
		workspaceID,
	)
	if err != nil {
		return localChatExternalAgent{}, false, xerrors.Errorf(
			"resolve local chat workspace %q agents from database: %w",
			workspaceID,
			err,
		)
	}
	for _, agent := range agents {
		agentName := strings.TrimSpace(agent.Name)
		if agentName == localChatExternalAgentName {
			return localChatExternalAgent{
				ID:   agent.ID,
				Name: agentName,
			}, true, nil
		}
	}
	for _, agent := range agents {
		agentName := strings.TrimSpace(agent.Name)
		if agentName == "" {
			continue
		}
		return localChatExternalAgent{
			ID:   agent.ID,
			Name: agentName,
		}, true, nil
	}
	return localChatExternalAgent{}, false, nil
}

func localChatExternalAgentFromResources(
	resources []codersdk.WorkspaceResource,
) (localChatExternalAgent, bool) {
	for _, resource := range resources {
		if resource.Type != "coder_external_agent" {
			continue
		}
		for _, agent := range resource.Agents {
			if agent.ID == uuid.Nil || strings.TrimSpace(agent.Name) == "" {
				continue
			}
			return localChatExternalAgent{
				ID:     agent.ID,
				Name:   strings.TrimSpace(agent.Name),
				Status: agent.Status,
			}, true
		}
	}
	return localChatExternalAgent{}, false
}

func (s *LocalService) MaybeLaunchAgentForChat(
	ctx context.Context,
	chat database.Chat,
) error {
	if s == nil {
		return xerrors.New("local chat service is not configured")
	}
	if workspaceModeFromChat(chat) != codersdk.ChatWorkspaceModeLocal {
		return nil
	}

	if !chat.WorkspaceID.Valid {
		return xerrors.Errorf("local chat %q is missing workspace_id", chat.ID)
	}
	if !chat.WorkspaceAgentID.Valid {
		return xerrors.Errorf("local chat %q is missing workspace_agent_id", chat.ID)
	}

	workspaceID := chat.WorkspaceID.UUID
	workspaceAgentID := chat.WorkspaceAgentID.UUID
	if s.localChatAgents != nil && s.localChatAgents.HasRunning(workspaceAgentID) {
		return nil
	}

	row, err := s.db.GetWorkspaceAgentAndWorkspaceByID(ctx, workspaceAgentID)
	if err != nil {
		return xerrors.Errorf(
			"get workspace agent %q from database: %w",
			workspaceAgentID,
			err,
		)
	}
	if row.WorkspaceTable.ID != workspaceID {
		return xerrors.Errorf(
			"local chat workspace agent %q does not belong to workspace %q",
			workspaceAgentID,
			workspaceID,
		)
	}

	if err := s.maybeLaunchLocalChatAgent(
		ctx,
		workspaceID,
		localChatExternalAgent{
			ID:   row.WorkspaceAgent.ID,
			Name: row.WorkspaceAgent.Name,
		},
	); err != nil {
		return xerrors.Errorf(
			"launch local chat workspace agent %q runtime: %w",
			workspaceAgentID,
			err,
		)
	}

	return nil
}

func (s *LocalService) maybeLaunchLocalChatAgent(
	ctx context.Context,
	workspaceID uuid.UUID,
	agent localChatExternalAgent,
) error {
	if s == nil {
		return xerrors.New("local chat service is not configured")
	}
	if s.localChatAgents != nil && s.localChatAgents.HasRunning(agent.ID) {
		return nil
	}
	if !s.agentLaunches.Allow(agent.ID, time.Now()) {
		return nil
	}

	_, err, _ := s.localChatAgentLaunchGroup.Do(agent.ID.String(), func() (struct{}, error) {
		if s.localChatAgents == nil {
			return struct{}{}, xerrors.New(
				"local chat agent manager is not configured",
			)
		}
		if s.localChatAgents.HasRunning(agent.ID) {
			return struct{}{}, nil
		}

		credentials, err := s.localChatExternalAgentCredentialsFromDB(
			ctx,
			workspaceID,
			agent,
		)
		if err != nil {
			return struct{}{}, err
		}
		if strings.TrimSpace(credentials.AgentToken) == "" {
			return struct{}{}, xerrors.New(
				"local chat external agent credentials token is empty",
			)
		}

		if err := s.localChatAgents.Start(localChatAgentStartParams{
			WorkspaceID: workspaceID,
			AgentID:     agent.ID,
			AgentName:   agent.Name,
			Credentials: credentials,
		}); err != nil {
			return struct{}{}, xerrors.Errorf(
				"start local chat external agent runtime: %w",
				err,
			)
		}
		return struct{}{}, nil
	})
	if err != nil {
		return err
	}
	return nil
}

func (s *LocalService) localChatExternalAgentCredentialsFromDB(
	ctx context.Context,
	workspaceID uuid.UUID,
	agent localChatExternalAgent,
) (codersdk.ExternalAgentCredentials, error) {
	row, err := s.db.GetWorkspaceAgentAndWorkspaceByID(ctx, agent.ID)
	if err != nil {
		return codersdk.ExternalAgentCredentials{}, xerrors.Errorf(
			"get local chat external agent %q from database: %w",
			agent.ID,
			err,
		)
	}
	if row.WorkspaceTable.ID != workspaceID {
		return codersdk.ExternalAgentCredentials{}, xerrors.Errorf(
			"local chat external agent %q does not belong to workspace %q",
			agent.ID,
			workspaceID,
		)
	}
	if strings.TrimSpace(row.WorkspaceAgent.Name) == "" ||
		strings.TrimSpace(row.WorkspaceAgent.Name) != strings.TrimSpace(agent.Name) {
		return codersdk.ExternalAgentCredentials{}, xerrors.Errorf(
			"local chat external agent %q name mismatch: expected %q, got %q",
			agent.ID,
			agent.Name,
			row.WorkspaceAgent.Name,
		)
	}

	return codersdk.ExternalAgentCredentials{
		AgentToken: row.WorkspaceAgent.AuthToken.String(),
	}, nil
}

func (s *LocalService) launchLocalChatAgentInProcess(
	params localChatAgentStartParams,
) (_ io.Closer, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = xerrors.Errorf(
				"initialize local chat external agent runtime: panic: %v",
				recovered,
			)
		}
	}()

	token := strings.TrimSpace(params.Credentials.AgentToken)
	if token == "" {
		return nil, xerrors.New("local chat external agent credentials token is empty")
	}

	client := agentsdk.New(s.accessURL, agentsdk.WithFixedToken(token))
	if s.httpClient != nil {
		client.SDK.HTTPClient = s.httpClient
	}

	logger := s.logger.Named("chat-local-agent").With(
		slog.F("workspace_id", params.WorkspaceID),
		slog.F("agent_id", params.AgentID),
		slog.F("agent_name", params.AgentName),
	)
	tempDir := os.TempDir()
	ag := agent.New(agent.Options{
		Client:               client,
		Logger:               logger,
		EnvironmentVariables: localChatAgentEnvironmentVariables(),
		LogDir:               tempDir,
		ScriptDataDir:        tempDir,
	})
	return ag, nil
}

func localChatAgentEnvironmentVariables() map[string]string {
	executablePath, err := os.Executable()
	if err != nil || strings.TrimSpace(executablePath) == "" {
		return nil
	}
	return map[string]string{
		"GIT_ASKPASS": executablePath,
	}
}

func localChatTemplateName(ownerID uuid.UUID) string {
	return localChatTemplateNamePrefix + localChatNameSuffix(ownerID)
}

func localChatWorkspaceName(ownerID uuid.UUID) string {
	return localChatWorkspaceNamePrefix + localChatNameSuffix(ownerID)
}

func localChatNameSuffix(ownerID uuid.UUID) string {
	compact := strings.ReplaceAll(ownerID.String(), "-", "")
	if len(compact) <= localChatNameSuffixLength {
		return compact
	}
	return compact[:localChatNameSuffixLength]
}

func isCodersdkStatusCode(err error, statusCode int) bool {
	var sdkErr *codersdk.Error
	if !xerrors.As(err, &sdkErr) {
		return false
	}
	return sdkErr.StatusCode() == statusCode
}

type localChatAgentLaunchLimiter struct {
	mu          sync.Mutex
	lastLaunch  map[uuid.UUID]time.Time
	minInterval time.Duration
}

func newLocalChatAgentLaunchLimiter(
	minInterval time.Duration,
) *localChatAgentLaunchLimiter {
	return &localChatAgentLaunchLimiter{
		lastLaunch:  make(map[uuid.UUID]time.Time),
		minInterval: minInterval,
	}
}

func (l *localChatAgentLaunchLimiter) Allow(
	agentID uuid.UUID,
	now time.Time,
) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if last, ok := l.lastLaunch[agentID]; ok {
		if now.Sub(last) < l.minInterval {
			return false
		}
	}
	l.lastLaunch[agentID] = now
	return true
}

package telemetry

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/elastic/go-sysinfo"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/buildinfo"
	clitelemetry "github.com/coder/coder/v2/cli/telemetry"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
)

const (
	// VersionHeader is sent in every telemetry request to
	// report the semantic version of Coder.
	VersionHeader = "X-Coder-Version"
)

type Options struct {
	Database database.Store
	Logger   slog.Logger
	// URL is an endpoint to direct telemetry towards!
	URL *url.URL

	BuiltinPostgres    bool
	DeploymentID       string
	GitHubOAuth        bool
	OIDCAuth           bool
	OIDCIssuerURL      string
	Wildcard           bool
	DERPServerRelayURL string
	GitAuth            []GitAuth
	Prometheus         bool
	STUN               bool
	SnapshotFrequency  time.Duration
	Tunnel             bool
	ParseLicenseJWT    func(lic *License) error
}

// New constructs a reporter for telemetry data.
// Duplicate data will be sent, it's on the server-side to index by UUID.
// Data is anonymized prior to being sent!
func New(options Options) (Reporter, error) {
	if options.SnapshotFrequency == 0 {
		// Report once every 30mins by default!
		options.SnapshotFrequency = 30 * time.Minute
	}
	snapshotURL, err := options.URL.Parse("/snapshot")
	if err != nil {
		return nil, xerrors.Errorf("parse snapshot url: %w", err)
	}
	deploymentURL, err := options.URL.Parse("/deployment")
	if err != nil {
		return nil, xerrors.Errorf("parse deployment url: %w", err)
	}

	ctx, cancelFunc := context.WithCancel(context.Background())
	reporter := &remoteReporter{
		ctx:           ctx,
		closed:        make(chan struct{}),
		closeFunc:     cancelFunc,
		options:       options,
		deploymentURL: deploymentURL,
		snapshotURL:   snapshotURL,
		startedAt:     dbtime.Now(),
	}
	go reporter.runSnapshotter()
	return reporter, nil
}

// NewNoop creates a new telemetry reporter that entirely discards all requests.
func NewNoop() Reporter {
	return &noopReporter{}
}

// Reporter sends data to the telemetry server.
type Reporter interface {
	// Report sends a snapshot to the telemetry server.
	// The contents of the snapshot can be a partial representation of the
	// database. For example, if a new user is added, a snapshot can
	// contain just that user entry.
	Report(snapshot *Snapshot)
	Close()
}

type remoteReporter struct {
	ctx        context.Context
	closed     chan struct{}
	closeMutex sync.Mutex
	closeFunc  context.CancelFunc

	options Options
	deploymentURL,
	snapshotURL *url.URL
	startedAt  time.Time
	shutdownAt *time.Time
}

func (r *remoteReporter) Report(snapshot *Snapshot) {
	go r.reportSync(snapshot)
}

func (r *remoteReporter) reportSync(snapshot *Snapshot) {
	snapshot.DeploymentID = r.options.DeploymentID
	data, err := json.Marshal(snapshot)
	if err != nil {
		r.options.Logger.Error(r.ctx, "marshal snapshot: %w", slog.Error(err))
		return
	}
	req, err := http.NewRequestWithContext(r.ctx, "POST", r.snapshotURL.String(), bytes.NewReader(data))
	if err != nil {
		r.options.Logger.Error(r.ctx, "unable to create snapshot request", slog.Error(err))
		return
	}
	req.Header.Set(VersionHeader, buildinfo.Version())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// If the request fails it's not necessarily an error.
		// In an airgapped environment, it's fine if this fails!
		r.options.Logger.Debug(r.ctx, "submit", slog.Error(err))
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		r.options.Logger.Debug(r.ctx, "bad response from telemetry server", slog.F("status", resp.StatusCode))
		return
	}
	r.options.Logger.Debug(r.ctx, "submitted snapshot")
}

func (r *remoteReporter) Close() {
	r.closeMutex.Lock()
	defer r.closeMutex.Unlock()
	if r.isClosed() {
		return
	}
	close(r.closed)
	now := dbtime.Now()
	r.shutdownAt = &now
	// Report a final collection of telemetry prior to close!
	// This could indicate final actions a user has taken, and
	// the time the deployment was shutdown.
	r.reportWithDeployment()
	r.closeFunc()
}

func (r *remoteReporter) isClosed() bool {
	select {
	case <-r.closed:
		return true
	default:
		return false
	}
}

func (r *remoteReporter) runSnapshotter() {
	first := true
	ticker := time.NewTicker(r.options.SnapshotFrequency)
	defer ticker.Stop()
	for {
		if !first {
			select {
			case <-r.closed:
				return
			case <-ticker.C:
			}
			// Skip the ticker on the first run to report instantly!
		}
		first = false
		r.closeMutex.Lock()
		if r.isClosed() {
			r.closeMutex.Unlock()
			return
		}
		r.reportWithDeployment()
		r.closeMutex.Unlock()
	}
}

func (r *remoteReporter) reportWithDeployment() {
	// Submit deployment information before creating a snapshot!
	// This is separated from the snapshot API call to reduce
	// duplicate data from being inserted. Snapshot may be called
	// numerous times simultaneously if there is lots of activity!
	err := r.deployment()
	if err != nil {
		r.options.Logger.Debug(r.ctx, "update deployment", slog.Error(err))
		return
	}
	snapshot, err := r.createSnapshot()
	if errors.Is(err, context.Canceled) {
		return
	}
	if err != nil {
		r.options.Logger.Error(r.ctx, "unable to create deployment snapshot", slog.Error(err))
		return
	}
	r.reportSync(snapshot)
}

// deployment collects host information and reports it to the telemetry server.
func (r *remoteReporter) deployment() error {
	sysInfoHost, err := sysinfo.Host()
	if err != nil {
		return xerrors.Errorf("get host info: %w", err)
	}
	mem, err := sysInfoHost.Memory()
	if err != nil {
		return xerrors.Errorf("get memory info: %w", err)
	}
	sysInfo := sysInfoHost.Info()

	containerized := false
	if sysInfo.Containerized != nil {
		containerized = *sysInfo.Containerized
	}

	// Tracks where Coder was installed from!
	installSource := os.Getenv("CODER_TELEMETRY_INSTALL_SOURCE")
	if len(installSource) > 64 {
		return xerrors.Errorf("install source must be <=64 chars: %s", installSource)
	}

	data, err := json.Marshal(&Deployment{
		ID:                 r.options.DeploymentID,
		Architecture:       sysInfo.Architecture,
		BuiltinPostgres:    r.options.BuiltinPostgres,
		Containerized:      containerized,
		Wildcard:           r.options.Wildcard,
		DERPServerRelayURL: r.options.DERPServerRelayURL,
		GitAuth:            r.options.GitAuth,
		Kubernetes:         os.Getenv("KUBERNETES_SERVICE_HOST") != "",
		GitHubOAuth:        r.options.GitHubOAuth,
		OIDCAuth:           r.options.OIDCAuth,
		OIDCIssuerURL:      r.options.OIDCIssuerURL,
		Prometheus:         r.options.Prometheus,
		InstallSource:      installSource,
		STUN:               r.options.STUN,
		Tunnel:             r.options.Tunnel,
		OSType:             sysInfo.OS.Type,
		OSFamily:           sysInfo.OS.Family,
		OSPlatform:         sysInfo.OS.Platform,
		OSName:             sysInfo.OS.Name,
		OSVersion:          sysInfo.OS.Version,
		CPUCores:           runtime.NumCPU(),
		MemoryTotal:        mem.Total,
		MachineID:          sysInfo.UniqueID,
		StartedAt:          r.startedAt,
		ShutdownAt:         r.shutdownAt,
	})
	if err != nil {
		return xerrors.Errorf("marshal deployment: %w", err)
	}
	req, err := http.NewRequestWithContext(r.ctx, "POST", r.deploymentURL.String(), bytes.NewReader(data))
	if err != nil {
		return xerrors.Errorf("create deployment request: %w", err)
	}
	req.Header.Set(VersionHeader, buildinfo.Version())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return xerrors.Errorf("perform request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		return xerrors.Errorf("update deployment: %w", err)
	}
	r.options.Logger.Debug(r.ctx, "submitted deployment info")
	return nil
}

// createSnapshot collects a full snapshot from the database.
func (r *remoteReporter) createSnapshot() (*Snapshot, error) {
	var (
		ctx = r.ctx
		// For resources that grow in size very quickly (like workspace builds),
		// we only report events that occurred within the past hour.
		createdAfter = dbtime.Now().Add(-1 * time.Hour)
		eg           errgroup.Group
		snapshot     = &Snapshot{
			DeploymentID: r.options.DeploymentID,
		}
	)

	eg.Go(func() error {
		apiKeys, err := r.options.Database.GetAPIKeysLastUsedAfter(ctx, createdAfter)
		if err != nil {
			return xerrors.Errorf("get api keys last used: %w", err)
		}
		snapshot.APIKeys = make([]APIKey, 0, len(apiKeys))
		for _, apiKey := range apiKeys {
			snapshot.APIKeys = append(snapshot.APIKeys, ConvertAPIKey(apiKey))
		}
		return nil
	})
	eg.Go(func() error {
		jobs, err := r.options.Database.GetProvisionerJobsCreatedAfter(ctx, createdAfter)
		if err != nil {
			return xerrors.Errorf("get provisioner jobs: %w", err)
		}
		snapshot.ProvisionerJobs = make([]ProvisionerJob, 0, len(jobs))
		for _, job := range jobs {
			snapshot.ProvisionerJobs = append(snapshot.ProvisionerJobs, ConvertProvisionerJob(job))
		}
		return nil
	})
	eg.Go(func() error {
		templates, err := r.options.Database.GetTemplates(ctx)
		if err != nil {
			return xerrors.Errorf("get templates: %w", err)
		}
		snapshot.Templates = make([]Template, 0, len(templates))
		for _, dbTemplate := range templates {
			snapshot.Templates = append(snapshot.Templates, ConvertTemplate(dbTemplate))
		}
		return nil
	})
	eg.Go(func() error {
		templateVersions, err := r.options.Database.GetTemplateVersionsCreatedAfter(ctx, createdAfter)
		if err != nil {
			return xerrors.Errorf("get template versions: %w", err)
		}
		snapshot.TemplateVersions = make([]TemplateVersion, 0, len(templateVersions))
		for _, version := range templateVersions {
			snapshot.TemplateVersions = append(snapshot.TemplateVersions, ConvertTemplateVersion(version))
		}
		return nil
	})
	eg.Go(func() error {
		userRows, err := r.options.Database.GetUsers(ctx, database.GetUsersParams{})
		if err != nil {
			return xerrors.Errorf("get users: %w", err)
		}
		users := database.ConvertUserRows(userRows)
		var firstUser database.User
		for _, dbUser := range users {
			if dbUser.Status != database.UserStatusActive {
				continue
			}
			if firstUser.CreatedAt.IsZero() {
				firstUser = dbUser
			}
			if dbUser.CreatedAt.Before(firstUser.CreatedAt) {
				firstUser = dbUser
			}
		}
		snapshot.Users = make([]User, 0, len(users))
		for _, dbUser := range users {
			user := ConvertUser(dbUser)
			// If it's the first user, we'll send the email!
			if firstUser.ID == dbUser.ID {
				email := dbUser.Email
				user.Email = &email
			}
			snapshot.Users = append(snapshot.Users, user)
		}
		return nil
	})
	eg.Go(func() error {
		workspaceRows, err := r.options.Database.GetWorkspaces(ctx, database.GetWorkspacesParams{})
		if err != nil {
			return xerrors.Errorf("get workspaces: %w", err)
		}
		workspaces := database.ConvertWorkspaceRows(workspaceRows)
		snapshot.Workspaces = make([]Workspace, 0, len(workspaces))
		for _, dbWorkspace := range workspaces {
			snapshot.Workspaces = append(snapshot.Workspaces, ConvertWorkspace(dbWorkspace))
		}
		return nil
	})
	eg.Go(func() error {
		workspaceApps, err := r.options.Database.GetWorkspaceAppsCreatedAfter(ctx, createdAfter)
		if err != nil {
			return xerrors.Errorf("get workspace apps: %w", err)
		}
		snapshot.WorkspaceApps = make([]WorkspaceApp, 0, len(workspaceApps))
		for _, app := range workspaceApps {
			snapshot.WorkspaceApps = append(snapshot.WorkspaceApps, ConvertWorkspaceApp(app))
		}
		return nil
	})
	eg.Go(func() error {
		workspaceAgents, err := r.options.Database.GetWorkspaceAgentsCreatedAfter(ctx, createdAfter)
		if err != nil {
			return xerrors.Errorf("get workspace agents: %w", err)
		}
		snapshot.WorkspaceAgents = make([]WorkspaceAgent, 0, len(workspaceAgents))
		for _, agent := range workspaceAgents {
			snapshot.WorkspaceAgents = append(snapshot.WorkspaceAgents, ConvertWorkspaceAgent(agent))
		}
		return nil
	})
	eg.Go(func() error {
		workspaceBuilds, err := r.options.Database.GetWorkspaceBuildsCreatedAfter(ctx, createdAfter)
		if err != nil {
			return xerrors.Errorf("get workspace builds: %w", err)
		}
		snapshot.WorkspaceBuilds = make([]WorkspaceBuild, 0, len(workspaceBuilds))
		for _, build := range workspaceBuilds {
			snapshot.WorkspaceBuilds = append(snapshot.WorkspaceBuilds, ConvertWorkspaceBuild(build))
		}
		return nil
	})
	eg.Go(func() error {
		workspaceResources, err := r.options.Database.GetWorkspaceResourcesCreatedAfter(ctx, createdAfter)
		if err != nil {
			return xerrors.Errorf("get workspace resources: %w", err)
		}
		snapshot.WorkspaceResources = make([]WorkspaceResource, 0, len(workspaceResources))
		for _, resource := range workspaceResources {
			snapshot.WorkspaceResources = append(snapshot.WorkspaceResources, ConvertWorkspaceResource(resource))
		}
		return nil
	})
	eg.Go(func() error {
		workspaceMetadata, err := r.options.Database.GetWorkspaceResourceMetadataCreatedAfter(ctx, createdAfter)
		if err != nil {
			return xerrors.Errorf("get workspace resource metadata: %w", err)
		}
		snapshot.WorkspaceResourceMetadata = make([]WorkspaceResourceMetadata, 0, len(workspaceMetadata))
		for _, metadata := range workspaceMetadata {
			snapshot.WorkspaceResourceMetadata = append(snapshot.WorkspaceResourceMetadata, ConvertWorkspaceResourceMetadata(metadata))
		}
		return nil
	})
	eg.Go(func() error {
		licenses, err := r.options.Database.GetUnexpiredLicenses(ctx)
		if err != nil {
			return xerrors.Errorf("get licenses: %w", err)
		}
		snapshot.Licenses = make([]License, 0, len(licenses))
		for _, license := range licenses {
			tl := ConvertLicense(license)
			if r.options.ParseLicenseJWT != nil {
				if err := r.options.ParseLicenseJWT(&tl); err != nil {
					r.options.Logger.Warn(ctx, "parse license JWT", slog.Error(err))
				}
			}
			snapshot.Licenses = append(snapshot.Licenses, tl)
		}
		return nil
	})
	eg.Go(func() error {
		stats, err := r.options.Database.GetWorkspaceAgentStats(ctx, createdAfter)
		if err != nil {
			return xerrors.Errorf("get workspace agent stats: %w", err)
		}
		snapshot.WorkspaceAgentStats = make([]WorkspaceAgentStat, 0, len(stats))
		for _, stat := range stats {
			snapshot.WorkspaceAgentStats = append(snapshot.WorkspaceAgentStats, ConvertWorkspaceAgentStat(stat))
		}
		return nil
	})
	eg.Go(func() error {
		proxies, err := r.options.Database.GetWorkspaceProxies(ctx)
		if err != nil {
			return xerrors.Errorf("get workspace proxies: %w", err)
		}
		snapshot.WorkspaceProxies = make([]WorkspaceProxy, 0, len(proxies))
		for _, proxy := range proxies {
			snapshot.WorkspaceProxies = append(snapshot.WorkspaceProxies, ConvertWorkspaceProxy(proxy))
		}
		return nil
	})

	err := eg.Wait()
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

// ConvertAPIKey anonymizes an API key.
func ConvertAPIKey(apiKey database.APIKey) APIKey {
	a := APIKey{
		ID:        apiKey.ID,
		UserID:    apiKey.UserID,
		CreatedAt: apiKey.CreatedAt,
		LastUsed:  apiKey.LastUsed,
		LoginType: apiKey.LoginType,
	}
	if apiKey.IPAddress.Valid {
		a.IPAddress = apiKey.IPAddress.IPNet.IP
	}
	return a
}

// ConvertWorkspace anonymizes a workspace.
func ConvertWorkspace(workspace database.Workspace) Workspace {
	return Workspace{
		ID:                workspace.ID,
		OrganizationID:    workspace.OrganizationID,
		OwnerID:           workspace.OwnerID,
		TemplateID:        workspace.TemplateID,
		CreatedAt:         workspace.CreatedAt,
		Deleted:           workspace.Deleted,
		Name:              workspace.Name,
		AutostartSchedule: workspace.AutostartSchedule.String,
		AutomaticUpdates:  string(workspace.AutomaticUpdates),
	}
}

// ConvertWorkspaceBuild anonymizes a workspace build.
func ConvertWorkspaceBuild(build database.WorkspaceBuild) WorkspaceBuild {
	return WorkspaceBuild{
		ID:                build.ID,
		CreatedAt:         build.CreatedAt,
		WorkspaceID:       build.WorkspaceID,
		JobID:             build.JobID,
		TemplateVersionID: build.TemplateVersionID,
		BuildNumber:       uint32(build.BuildNumber),
	}
}

// ConvertProvisionerJob anonymizes a provisioner job.
func ConvertProvisionerJob(job database.ProvisionerJob) ProvisionerJob {
	snapJob := ProvisionerJob{
		ID:             job.ID,
		OrganizationID: job.OrganizationID,
		InitiatorID:    job.InitiatorID,
		CreatedAt:      job.CreatedAt,
		UpdatedAt:      job.UpdatedAt,
		Error:          job.Error.String,
		Type:           job.Type,
	}
	if job.StartedAt.Valid {
		snapJob.StartedAt = &job.StartedAt.Time
	}
	if job.CanceledAt.Valid {
		snapJob.CanceledAt = &job.CanceledAt.Time
	}
	if job.CompletedAt.Valid {
		snapJob.CompletedAt = &job.CompletedAt.Time
	}
	return snapJob
}

// ConvertWorkspaceAgent anonymizes a workspace agent.
func ConvertWorkspaceAgent(agent database.WorkspaceAgent) WorkspaceAgent {
	subsystems := []string{}
	for _, subsystem := range agent.Subsystems {
		subsystems = append(subsystems, string(subsystem))
	}

	snapAgent := WorkspaceAgent{
		ID:                       agent.ID,
		CreatedAt:                agent.CreatedAt,
		ResourceID:               agent.ResourceID,
		InstanceAuth:             agent.AuthInstanceID.Valid,
		Architecture:             agent.Architecture,
		OperatingSystem:          agent.OperatingSystem,
		EnvironmentVariables:     agent.EnvironmentVariables.Valid,
		Directory:                agent.Directory != "",
		ConnectionTimeoutSeconds: agent.ConnectionTimeoutSeconds,
		Subsystems:               subsystems,
	}
	if agent.FirstConnectedAt.Valid {
		snapAgent.FirstConnectedAt = &agent.FirstConnectedAt.Time
	}
	if agent.LastConnectedAt.Valid {
		snapAgent.LastConnectedAt = &agent.LastConnectedAt.Time
	}
	if agent.DisconnectedAt.Valid {
		snapAgent.DisconnectedAt = &agent.DisconnectedAt.Time
	}
	return snapAgent
}

// ConvertWorkspaceAgentStat anonymizes a workspace agent stat.
func ConvertWorkspaceAgentStat(stat database.GetWorkspaceAgentStatsRow) WorkspaceAgentStat {
	return WorkspaceAgentStat{
		UserID:                      stat.UserID,
		TemplateID:                  stat.TemplateID,
		WorkspaceID:                 stat.WorkspaceID,
		AgentID:                     stat.AgentID,
		AggregatedFrom:              stat.AggregatedFrom,
		ConnectionLatency50:         stat.WorkspaceConnectionLatency50,
		ConnectionLatency95:         stat.WorkspaceConnectionLatency95,
		RxBytes:                     stat.WorkspaceRxBytes,
		TxBytes:                     stat.WorkspaceTxBytes,
		SessionCountVSCode:          stat.SessionCountVSCode,
		SessionCountJetBrains:       stat.SessionCountJetBrains,
		SessionCountReconnectingPTY: stat.SessionCountReconnectingPTY,
		SessionCountSSH:             stat.SessionCountSSH,
	}
}

// ConvertWorkspaceApp anonymizes a workspace app.
func ConvertWorkspaceApp(app database.WorkspaceApp) WorkspaceApp {
	return WorkspaceApp{
		ID:        app.ID,
		CreatedAt: app.CreatedAt,
		AgentID:   app.AgentID,
		Icon:      app.Icon,
		Subdomain: app.Subdomain,
	}
}

// ConvertWorkspaceResource anonymizes a workspace resource.
func ConvertWorkspaceResource(resource database.WorkspaceResource) WorkspaceResource {
	return WorkspaceResource{
		ID:           resource.ID,
		JobID:        resource.JobID,
		Transition:   resource.Transition,
		Type:         resource.Type,
		InstanceType: resource.InstanceType.String,
	}
}

// ConvertWorkspaceResourceMetadata anonymizes workspace metadata.
func ConvertWorkspaceResourceMetadata(metadata database.WorkspaceResourceMetadatum) WorkspaceResourceMetadata {
	return WorkspaceResourceMetadata{
		ResourceID: metadata.WorkspaceResourceID,
		Key:        metadata.Key,
		Sensitive:  metadata.Sensitive,
	}
}

// ConvertUser anonymizes a user.
func ConvertUser(dbUser database.User) User {
	emailHashed := ""
	atSymbol := strings.LastIndex(dbUser.Email, "@")
	if atSymbol >= 0 {
		// We hash the beginning of the user to allow for indexing users
		// by email between deployments.
		hash := sha256.Sum256([]byte(dbUser.Email[:atSymbol]))
		emailHashed = fmt.Sprintf("%x%s", hash[:], dbUser.Email[atSymbol:])
	}
	return User{
		ID:          dbUser.ID,
		EmailHashed: emailHashed,
		RBACRoles:   dbUser.RBACRoles,
		CreatedAt:   dbUser.CreatedAt,
	}
}

// ConvertTemplate anonymizes a template.
func ConvertTemplate(dbTemplate database.Template) Template {
	return Template{
		ID:              dbTemplate.ID,
		CreatedBy:       dbTemplate.CreatedBy,
		CreatedAt:       dbTemplate.CreatedAt,
		UpdatedAt:       dbTemplate.UpdatedAt,
		OrganizationID:  dbTemplate.OrganizationID,
		Deleted:         dbTemplate.Deleted,
		ActiveVersionID: dbTemplate.ActiveVersionID,
		Name:            dbTemplate.Name,
		Description:     dbTemplate.Description != "",
	}
}

// ConvertTemplateVersion anonymizes a template version.
func ConvertTemplateVersion(version database.TemplateVersion) TemplateVersion {
	snapVersion := TemplateVersion{
		ID:             version.ID,
		CreatedAt:      version.CreatedAt,
		OrganizationID: version.OrganizationID,
		JobID:          version.JobID,
	}
	if version.TemplateID.Valid {
		snapVersion.TemplateID = &version.TemplateID.UUID
	}
	return snapVersion
}

func ConvertLicense(license database.License) License {
	// License is intentionally not anonymized because it's
	// deployment-wide, and we already have an index of all issued
	// licenses.
	return License{
		JWT:        license.JWT,
		Exp:        license.Exp,
		UploadedAt: license.UploadedAt,
		UUID:       license.UUID,
	}
}

// ConvertWorkspaceProxy anonymizes a workspace proxy.
func ConvertWorkspaceProxy(proxy database.WorkspaceProxy) WorkspaceProxy {
	return WorkspaceProxy{
		ID:          proxy.ID,
		Name:        proxy.Name,
		DisplayName: proxy.DisplayName,
		DerpEnabled: proxy.DerpEnabled,
		DerpOnly:    proxy.DerpOnly,
		CreatedAt:   proxy.CreatedAt,
		UpdatedAt:   proxy.UpdatedAt,
	}
}

func ConvertExternalProvisioner(id uuid.UUID, tags map[string]string, provisioners []database.ProvisionerType) ExternalProvisioner {
	tagsCopy := make(map[string]string, len(tags))
	for k, v := range tags {
		tagsCopy[k] = v
	}
	strProvisioners := make([]string, 0, len(provisioners))
	for _, prov := range provisioners {
		strProvisioners = append(strProvisioners, string(prov))
	}
	return ExternalProvisioner{
		ID:           id.String(),
		Tags:         tagsCopy,
		Provisioners: strProvisioners,
		StartedAt:    time.Now(),
	}
}

// Snapshot represents a point-in-time anonymized database dump.
// Data is aggregated by latest on the server-side, so partial data
// can be sent without issue.
type Snapshot struct {
	DeploymentID string `json:"deployment_id"`

	APIKeys                   []APIKey                    `json:"api_keys"`
	CLIInvocations            []clitelemetry.Invocation   `json:"cli_invocations"`
	ExternalProvisioners      []ExternalProvisioner       `json:"external_provisioners"`
	Licenses                  []License                   `json:"licenses"`
	ProvisionerJobs           []ProvisionerJob            `json:"provisioner_jobs"`
	TemplateVersions          []TemplateVersion           `json:"template_versions"`
	Templates                 []Template                  `json:"templates"`
	Users                     []User                      `json:"users"`
	WorkspaceAgentStats       []WorkspaceAgentStat        `json:"workspace_agent_stats"`
	WorkspaceAgents           []WorkspaceAgent            `json:"workspace_agents"`
	WorkspaceApps             []WorkspaceApp              `json:"workspace_apps"`
	WorkspaceBuilds           []WorkspaceBuild            `json:"workspace_build"`
	WorkspaceProxies          []WorkspaceProxy            `json:"workspace_proxies"`
	WorkspaceResourceMetadata []WorkspaceResourceMetadata `json:"workspace_resource_metadata"`
	WorkspaceResources        []WorkspaceResource         `json:"workspace_resources"`
	Workspaces                []Workspace                 `json:"workspaces"`
}

// Deployment contains information about the host running Coder.
type Deployment struct {
	ID                 string     `json:"id"`
	Architecture       string     `json:"architecture"`
	BuiltinPostgres    bool       `json:"builtin_postgres"`
	Containerized      bool       `json:"containerized"`
	Kubernetes         bool       `json:"kubernetes"`
	Tunnel             bool       `json:"tunnel"`
	Wildcard           bool       `json:"wildcard"`
	DERPServerRelayURL string     `json:"derp_server_relay_url"`
	GitAuth            []GitAuth  `json:"git_auth"`
	GitHubOAuth        bool       `json:"github_oauth"`
	OIDCAuth           bool       `json:"oidc_auth"`
	OIDCIssuerURL      string     `json:"oidc_issuer_url"`
	Prometheus         bool       `json:"prometheus"`
	InstallSource      string     `json:"install_source"`
	STUN               bool       `json:"stun"`
	OSType             string     `json:"os_type"`
	OSFamily           string     `json:"os_family"`
	OSPlatform         string     `json:"os_platform"`
	OSName             string     `json:"os_name"`
	OSVersion          string     `json:"os_version"`
	CPUCores           int        `json:"cpu_cores"`
	MemoryTotal        uint64     `json:"memory_total"`
	MachineID          string     `json:"machine_id"`
	StartedAt          time.Time  `json:"started_at"`
	ShutdownAt         *time.Time `json:"shutdown_at"`
}

type GitAuth struct {
	Type string `json:"type"`
}

type APIKey struct {
	ID        string             `json:"id"`
	UserID    uuid.UUID          `json:"user_id"`
	CreatedAt time.Time          `json:"created_at"`
	LastUsed  time.Time          `json:"last_used"`
	LoginType database.LoginType `json:"login_type"`
	IPAddress net.IP             `json:"ip_address"`
}

type User struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	// Email is only filled in for the first/admin user!
	Email       *string             `json:"email"`
	EmailHashed string              `json:"email_hashed"`
	RBACRoles   []string            `json:"rbac_roles"`
	Status      database.UserStatus `json:"status"`
}

type WorkspaceResource struct {
	ID           uuid.UUID                    `json:"id"`
	JobID        uuid.UUID                    `json:"job_id"`
	Transition   database.WorkspaceTransition `json:"transition"`
	Type         string                       `json:"type"`
	InstanceType string                       `json:"instance_type"`
}

type WorkspaceResourceMetadata struct {
	ResourceID uuid.UUID `json:"resource_id"`
	Key        string    `json:"key"`
	Sensitive  bool      `json:"sensitive"`
}

type WorkspaceAgent struct {
	ID                       uuid.UUID  `json:"id"`
	CreatedAt                time.Time  `json:"created_at"`
	ResourceID               uuid.UUID  `json:"resource_id"`
	InstanceAuth             bool       `json:"instance_auth"`
	Architecture             string     `json:"architecture"`
	OperatingSystem          string     `json:"operating_system"`
	EnvironmentVariables     bool       `json:"environment_variables"`
	Directory                bool       `json:"directory"`
	FirstConnectedAt         *time.Time `json:"first_connected_at"`
	LastConnectedAt          *time.Time `json:"last_connected_at"`
	DisconnectedAt           *time.Time `json:"disconnected_at"`
	ConnectionTimeoutSeconds int32      `json:"connection_timeout_seconds"`
	Subsystems               []string   `json:"subsystems"`
}

type WorkspaceAgentStat struct {
	UserID                      uuid.UUID `json:"user_id"`
	TemplateID                  uuid.UUID `json:"template_id"`
	WorkspaceID                 uuid.UUID `json:"workspace_id"`
	AggregatedFrom              time.Time `json:"aggregated_from"`
	AgentID                     uuid.UUID `json:"agent_id"`
	RxBytes                     int64     `json:"rx_bytes"`
	TxBytes                     int64     `json:"tx_bytes"`
	ConnectionLatency50         float64   `json:"connection_latency_50"`
	ConnectionLatency95         float64   `json:"connection_latency_95"`
	SessionCountVSCode          int64     `json:"session_count_vscode"`
	SessionCountJetBrains       int64     `json:"session_count_jetbrains"`
	SessionCountReconnectingPTY int64     `json:"session_count_reconnecting_pty"`
	SessionCountSSH             int64     `json:"session_count_ssh"`
}

type WorkspaceApp struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	AgentID   uuid.UUID `json:"agent_id"`
	Icon      string    `json:"icon"`
	Subdomain bool      `json:"subdomain"`
}

type WorkspaceBuild struct {
	ID                uuid.UUID `json:"id"`
	CreatedAt         time.Time `json:"created_at"`
	WorkspaceID       uuid.UUID `json:"workspace_id"`
	TemplateVersionID uuid.UUID `json:"template_version_id"`
	JobID             uuid.UUID `json:"job_id"`
	BuildNumber       uint32    `json:"build_number"`
}

type Workspace struct {
	ID                uuid.UUID `json:"id"`
	OrganizationID    uuid.UUID `json:"organization_id"`
	OwnerID           uuid.UUID `json:"owner_id"`
	TemplateID        uuid.UUID `json:"template_id"`
	CreatedAt         time.Time `json:"created_at"`
	Deleted           bool      `json:"deleted"`
	Name              string    `json:"name"`
	AutostartSchedule string    `json:"autostart_schedule"`
	AutomaticUpdates  string    `json:"automatic_updates"`
}

type Template struct {
	ID              uuid.UUID `json:"id"`
	CreatedBy       uuid.UUID `json:"created_by"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	OrganizationID  uuid.UUID `json:"organization_id"`
	Deleted         bool      `json:"deleted"`
	ActiveVersionID uuid.UUID `json:"active_version_id"`
	Name            string    `json:"name"`
	Description     bool      `json:"description"`
}

type TemplateVersion struct {
	ID             uuid.UUID  `json:"id"`
	CreatedAt      time.Time  `json:"created_at"`
	TemplateID     *uuid.UUID `json:"template_id,omitempty"`
	OrganizationID uuid.UUID  `json:"organization_id"`
	JobID          uuid.UUID  `json:"job_id"`
}

type ProvisionerJob struct {
	ID             uuid.UUID                   `json:"id"`
	OrganizationID uuid.UUID                   `json:"organization_id"`
	InitiatorID    uuid.UUID                   `json:"initiator_id"`
	CreatedAt      time.Time                   `json:"created_at"`
	UpdatedAt      time.Time                   `json:"updated_at"`
	StartedAt      *time.Time                  `json:"started_at,omitempty"`
	CanceledAt     *time.Time                  `json:"canceled_at,omitempty"`
	CompletedAt    *time.Time                  `json:"completed_at,omitempty"`
	Error          string                      `json:"error"`
	Type           database.ProvisionerJobType `json:"type"`
}

type License struct {
	JWT        string    `json:"jwt"`
	UploadedAt time.Time `json:"uploaded_at"`
	Exp        time.Time `json:"exp"`
	UUID       uuid.UUID `json:"uuid"`
	// These two fields are set by decoding the JWT. If the signing keys aren't
	// passed in, these will always be nil.
	Email *string `json:"email"`
	Trial *bool   `json:"trial"`
}

type WorkspaceProxy struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	DisplayName string    `json:"display_name"`
	// No URLs since we don't send deployment URL.
	DerpEnabled bool `json:"derp_enabled"`
	DerpOnly    bool `json:"derp_only"`
	// No Status since it may contain sensitive information.
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ExternalProvisioner struct {
	ID           string            `json:"id"`
	Tags         map[string]string `json:"tags"`
	Provisioners []string          `json:"provisioners"`
	StartedAt    time.Time         `json:"started_at"`
	ShutdownAt   *time.Time        `json:"shutdown_at"`
}

type noopReporter struct{}

func (*noopReporter) Report(_ *Snapshot) {}
func (*noopReporter) Close()             {}

package telemetry

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/elastic/go-sysinfo"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/buildinfo"
	"github.com/coder/coder/coderd/database"
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

	BuiltinPostgres   bool
	DeploymentID      string
	GitHubOAuth       bool
	Prometheus        bool
	STUN              bool
	SnapshotFrequency time.Duration
	Tunnel            bool
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
		startedAt:     database.Now(),
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
		r.options.Logger.Error(r.ctx, "create request", slog.Error(err))
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
	now := database.Now()
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
	// numerous times simaltanously if there is lots of activity!
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
		r.options.Logger.Error(r.ctx, "create snapshot", slog.Error(err))
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
	data, err := json.Marshal(&Deployment{
		ID:              r.options.DeploymentID,
		Architecture:    sysInfo.Architecture,
		BuiltinPostgres: r.options.BuiltinPostgres,
		Containerized:   containerized,
		GitHubOAuth:     r.options.GitHubOAuth,
		Prometheus:      r.options.Prometheus,
		STUN:            r.options.STUN,
		Tunnel:          r.options.Tunnel,
		OSType:          sysInfo.OS.Type,
		OSFamily:        sysInfo.OS.Family,
		OSPlatform:      sysInfo.OS.Platform,
		OSName:          sysInfo.OS.Name,
		OSVersion:       sysInfo.OS.Version,
		CPUCores:        runtime.NumCPU(),
		MemoryTotal:     mem.Total,
		MachineID:       sysInfo.UniqueID,
		StartedAt:       r.startedAt,
		ShutdownAt:      r.shutdownAt,
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
		createdAfter = database.Now().Add(-1 * time.Hour)
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
		schemas, err := r.options.Database.GetParameterSchemasCreatedAfter(ctx, createdAfter)
		if err != nil {
			return xerrors.Errorf("get parameter schemas: %w", err)
		}
		snapshot.ParameterSchemas = make([]ParameterSchema, 0, len(schemas))
		for _, schema := range schemas {
			snapshot.ParameterSchemas = append(snapshot.ParameterSchemas, ParameterSchema{
				ID:                  schema.ID,
				JobID:               schema.JobID,
				Name:                schema.Name,
				ValidationCondition: schema.ValidationCondition,
			})
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
		users, err := r.options.Database.GetUsers(ctx, database.GetUsersParams{})
		if err != nil {
			return xerrors.Errorf("get users: %w", err)
		}
		var firstUser database.User
		for _, dbUser := range users {
			if dbUser.Status != database.UserStatusActive {
				continue
			}
			if firstUser.CreatedAt.IsZero() {
				firstUser = dbUser
			}
			if dbUser.CreatedAt.After(firstUser.CreatedAt) {
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
		workspaces, err := r.options.Database.GetWorkspaces(ctx, database.GetWorkspacesParams{})
		if err != nil {
			return xerrors.Errorf("get workspaces: %w", err)
		}
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

	err := eg.Wait()
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

// ConvertAPIKey anonymizes an API key.
func ConvertAPIKey(apiKey database.APIKey) APIKey {
	return APIKey{
		ID:        apiKey.ID,
		UserID:    apiKey.UserID,
		CreatedAt: apiKey.CreatedAt,
		LastUsed:  apiKey.LastUsed,
		LoginType: apiKey.LoginType,
	}
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
	return WorkspaceAgent{
		ID:                   agent.ID,
		CreatedAt:            agent.CreatedAt,
		ResourceID:           agent.ResourceID,
		InstanceAuth:         agent.AuthInstanceID.Valid,
		Architecture:         agent.Architecture,
		OperatingSystem:      agent.OperatingSystem,
		EnvironmentVariables: agent.EnvironmentVariables.Valid,
		StartupScript:        agent.StartupScript.Valid,
		Directory:            agent.Directory != "",
	}
}

// ConvertWorkspaceApp anonymizes a workspace app.
func ConvertWorkspaceApp(app database.WorkspaceApp) WorkspaceApp {
	return WorkspaceApp{
		ID:           app.ID,
		CreatedAt:    app.CreatedAt,
		AgentID:      app.AgentID,
		Icon:         app.Icon,
		RelativePath: app.RelativePath,
	}
}

// ConvertWorkspaceResource anonymizes a workspace resource.
func ConvertWorkspaceResource(resource database.WorkspaceResource) WorkspaceResource {
	return WorkspaceResource{
		ID:         resource.ID,
		JobID:      resource.JobID,
		Transition: resource.Transition,
		Type:       resource.Type,
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

// Snapshot represents a point-in-time anonymized database dump.
// Data is aggregated by latest on the server-side, so partial data
// can be sent without issue.
type Snapshot struct {
	DeploymentID string `json:"deployment_id"`

	APIKeys            []APIKey            `json:"api_keys"`
	ParameterSchemas   []ParameterSchema   `json:"parameter_schemas"`
	ProvisionerJobs    []ProvisionerJob    `json:"provisioner_jobs"`
	Templates          []Template          `json:"templates"`
	TemplateVersions   []TemplateVersion   `json:"template_versions"`
	Users              []User              `json:"users"`
	Workspaces         []Workspace         `json:"workspaces"`
	WorkspaceApps      []WorkspaceApp      `json:"workspace_apps"`
	WorkspaceAgents    []WorkspaceAgent    `json:"workspace_agents"`
	WorkspaceBuilds    []WorkspaceBuild    `json:"workspace_build"`
	WorkspaceResources []WorkspaceResource `json:"workspace_resources"`
}

// Deployment contains information about the host running Coder.
type Deployment struct {
	ID              string     `json:"id"`
	Architecture    string     `json:"architecture"`
	BuiltinPostgres bool       `json:"builtin_postgres"`
	Containerized   bool       `json:"containerized"`
	Tunnel          bool       `json:"tunnel"`
	GitHubOAuth     bool       `json:"github_oauth"`
	Prometheus      bool       `json:"prometheus"`
	STUN            bool       `json:"stun"`
	OSType          string     `json:"os_type"`
	OSFamily        string     `json:"os_family"`
	OSPlatform      string     `json:"os_platform"`
	OSName          string     `json:"os_name"`
	OSVersion       string     `json:"os_version"`
	CPUCores        int        `json:"cpu_cores"`
	MemoryTotal     uint64     `json:"memory_total"`
	MachineID       string     `json:"machine_id"`
	StartedAt       time.Time  `json:"started_at"`
	ShutdownAt      *time.Time `json:"shutdown_at"`
}

type APIKey struct {
	ID        string             `json:"id"`
	UserID    uuid.UUID          `json:"user_id"`
	CreatedAt time.Time          `json:"created_at"`
	LastUsed  time.Time          `json:"last_used"`
	LoginType database.LoginType `json:"login_type"`
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
	ID         uuid.UUID                    `json:"id"`
	JobID      uuid.UUID                    `json:"job_id"`
	Transition database.WorkspaceTransition `json:"transition"`
	Type       string                       `json:"type"`
}

type WorkspaceAgent struct {
	ID                   uuid.UUID `json:"id"`
	CreatedAt            time.Time `json:"created_at"`
	ResourceID           uuid.UUID `json:"resource_id"`
	InstanceAuth         bool      `json:"instance_auth"`
	Architecture         string    `json:"architecture"`
	OperatingSystem      string    `json:"operating_system"`
	EnvironmentVariables bool      `json:"environment_variables"`
	StartupScript        bool      `json:"startup_script"`
	Directory            bool      `json:"directory"`
}

type WorkspaceApp struct {
	ID           uuid.UUID `json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	AgentID      uuid.UUID `json:"agent_id"`
	Icon         string    `json:"icon"`
	RelativePath bool      `json:"relative_path"`
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

type ParameterSchema struct {
	ID                  uuid.UUID `json:"id"`
	JobID               uuid.UUID `json:"job_id"`
	Name                string    `json:"name"`
	ValidationCondition string    `json:"validation_condition"`
}

type noopReporter struct{}

func (*noopReporter) Report(_ *Snapshot) {}
func (*noopReporter) Close()             {}

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

type Options struct {
	Database database.Store
	Logger   slog.Logger
	// URL is an endpoint to direct telemetry towards!
	URL *url.URL

	DeploymentID string
	DevMode      bool
	// Disabled determines whether telemetry will be collected
	// and sent. This allows callers to still execute the API
	// without having to check whether it's enabled.
	Disabled          bool
	SnapshotFrequency time.Duration
}

// New constructs a reporter for telemetry data.
// Duplicate data will be sent, it's on the server-side to index by UUID.
// Data is anonymized prior to being sent!
func New(options Options) (*Reporter, error) {
	if options.SnapshotFrequency == 0 {
		// Report six times a day by default!
		options.SnapshotFrequency = 4 * time.Hour
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
	reporter := &Reporter{
		ctx:           ctx,
		closed:        make(chan struct{}),
		closeFunc:     cancelFunc,
		options:       options,
		deploymentURL: deploymentURL,
		snapshotURL:   snapshotURL,
		startedAt:     database.Now(),
	}
	if !options.Disabled {
		go reporter.runSnapshotter()
	}
	return reporter, nil
}

// Reporter sends data to the telemetry server.
type Reporter struct {
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

// Snapshot reports a snapshot to the telemetry server.
// The contents of the snapshot can be a partial representation of
// the database. For example, if a new user is added, a snapshot
// can contain just that user entry.
func (r *Reporter) Snapshot(ctx context.Context, snapshot *Snapshot) {
	if r.options.Disabled {
		return
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		r.options.Logger.Error(ctx, "marshal snapshot: %w", slog.Error(err))
		return
	}
	req, err := http.NewRequestWithContext(ctx, "POST", r.snapshotURL.String(), bytes.NewReader(data))
	if err != nil {
		r.options.Logger.Error(ctx, "create request", slog.Error(err))
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// If the request fails it's not necessarily an error.
		// In an airgapped environment, it's fine if this fails!
		r.options.Logger.Debug(ctx, "submit", slog.Error(err))
		return
	}
	if resp.StatusCode != http.StatusAccepted {
		r.options.Logger.Debug(ctx, "bad response from telemetry server", slog.F("status", resp.StatusCode))
		return
	}
	r.options.Logger.Debug(ctx, "submitted snapshot")
}

func (r *Reporter) Close() {
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
	r.report()
	r.closeFunc()
}

func (r *Reporter) isClosed() bool {
	select {
	case <-r.closed:
		return true
	default:
		return false
	}
}

func (r *Reporter) runSnapshotter() {
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
		r.report()
		r.closeMutex.Unlock()
	}
}

func (r *Reporter) report() {
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
	r.Snapshot(r.ctx, snapshot)
}

// deployment collects host information and reports it to the telemetry server.
func (r *Reporter) deployment() error {
	if r.options.Disabled {
		return nil
	}
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
		ID:            r.options.DeploymentID,
		Architecture:  sysInfo.Architecture,
		Containerized: containerized,
		DevMode:       r.options.DevMode,
		OSType:        sysInfo.OS.Type,
		OSFamily:      sysInfo.OS.Family,
		OSPlatform:    sysInfo.OS.Platform,
		OSName:        sysInfo.OS.Name,
		OSVersion:     sysInfo.OS.Version,
		CPUCores:      runtime.NumCPU(),
		MemoryTotal:   mem.Total,
		MachineID:     sysInfo.UniqueID,
		Version:       buildinfo.Version(),
		StartedAt:     r.startedAt,
		ShutdownAt:    r.shutdownAt,
	})
	if err != nil {
		return xerrors.Errorf("marshal deployment: %w", err)
	}
	req, err := http.NewRequestWithContext(r.ctx, "POST", r.deploymentURL.String(), bytes.NewReader(data))
	if err != nil {
		return xerrors.Errorf("create deployment request: %w", err)
	}
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
func (r *Reporter) createSnapshot() (*Snapshot, error) {
	var (
		ctx          = r.ctx
		createdAfter = database.Now().AddDate(0, 0, -1)
		eg           errgroup.Group
		snapshot     = &Snapshot{
			DeploymentID: r.options.DeploymentID,
		}
	)

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
			snapshot.ProvisionerJobs = append(snapshot.ProvisionerJobs, snapJob)
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
			snapshot.Templates = append(snapshot.Templates, Template{
				ID:              dbTemplate.ID,
				CreatedBy:       dbTemplate.CreatedBy.UUID,
				CreatedAt:       dbTemplate.CreatedAt,
				UpdatedAt:       dbTemplate.UpdatedAt,
				OrganizationID:  dbTemplate.OrganizationID,
				Deleted:         dbTemplate.Deleted,
				ActiveVersionID: dbTemplate.ActiveVersionID,
				Name:            dbTemplate.Name,
				Description:     dbTemplate.Description != "",
			})
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
			snapVersion := TemplateVersion{
				ID:             version.ID,
				CreatedAt:      version.CreatedAt,
				OrganizationID: version.OrganizationID,
				JobID:          version.JobID,
			}
			if version.TemplateID.Valid {
				snapVersion.TemplateID = &version.TemplateID.UUID
			}
			snapshot.TemplateVersions = append(snapshot.TemplateVersions, snapVersion)
		}
		return nil
	})
	eg.Go(func() error {
		users, err := r.options.Database.GetUsers(ctx, database.GetUsersParams{})
		if err != nil {
			return xerrors.Errorf("get users: %w", err)
		}
		snapshot.Users = make([]User, 0, len(users))
		for _, dbUser := range users {
			emailHashed := ""
			atSymbol := strings.LastIndex(dbUser.Email, "@")
			if atSymbol >= 0 {
				// We hash the beginning of the user to allow for indexing users
				// by email between deployments.
				hash := sha256.Sum256([]byte(dbUser.Email[:atSymbol]))
				emailHashed = fmt.Sprintf("%x%s", hash[:], dbUser.Email[atSymbol:])
			}

			snapshot.Users = append(snapshot.Users, User{
				ID:          dbUser.ID,
				EmailHashed: emailHashed,
				RBACRoles:   dbUser.RBACRoles,
				CreatedAt:   dbUser.CreatedAt,
			})
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
			snapshot.Workspaces = append(snapshot.Workspaces, Workspace{
				ID:             dbWorkspace.ID,
				OrganizationID: dbWorkspace.OrganizationID,
				OwnerID:        dbWorkspace.OwnerID,
				TemplateID:     dbWorkspace.TemplateID,
				CreatedAt:      dbWorkspace.CreatedAt,
				Deleted:        dbWorkspace.Deleted,
			})
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
			snapshot.WorkspaceApps = append(snapshot.WorkspaceApps, WorkspaceApp{
				ID:           app.ID,
				CreatedAt:    app.CreatedAt,
				AgentID:      app.AgentID,
				Icon:         app.Icon != "",
				RelativePath: app.RelativePath,
			})
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
			snapshot.WorkspaceAgents = append(snapshot.WorkspaceAgents, WorkspaceAgent{
				ID:                   agent.ID,
				CreatedAt:            agent.CreatedAt,
				ResourceID:           agent.ResourceID,
				InstanceAuth:         agent.AuthInstanceID.Valid,
				Architecture:         agent.Architecture,
				OperatingSystem:      agent.OperatingSystem,
				EnvironmentVariables: agent.EnvironmentVariables.Valid,
				StartupScript:        agent.StartupScript.Valid,
				Directory:            agent.Directory != "",
			})
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
			snapshot.WorkspaceBuilds = append(snapshot.WorkspaceBuilds, WorkspaceBuild{
				ID:                build.ID,
				CreatedAt:         build.CreatedAt,
				WorkspaceID:       build.WorkspaceID,
				JobID:             build.JobID,
				TemplateVersionID: build.TemplateVersionID,
				BuildNumber:       uint32(build.BuildNumber),
			})
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
			snapshot.WorkspaceResources = append(snapshot.WorkspaceResources, WorkspaceResource{
				ID:         resource.ID,
				JobID:      resource.JobID,
				Transition: resource.Transition,
				Type:       resource.Type,
			})
		}
		return nil
	})

	err := eg.Wait()
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

// Snapshot represents a point-in-time anonymized database dump.
// Data is aggregated by latest on the server-side, so partial data
// can be sent without issue.
type Snapshot struct {
	DeploymentID string `json:"deployment_id"`

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
	ID            string     `json:"id"`
	Architecture  string     `json:"architecture"`
	Containerized bool       `json:"containerized"`
	DevMode       bool       `json:"dev_mode"`
	OSType        string     `json:"os_type"`
	OSFamily      string     `json:"os_family"`
	OSPlatform    string     `json:"os_platform"`
	OSName        string     `json:"os_name"`
	OSVersion     string     `json:"os_version"`
	CPUCores      int        `json:"cpu_cores"`
	MemoryTotal   uint64     `json:"memory_total"`
	MachineID     string     `json:"machine_id"`
	Version       string     `json:"version"`
	StartedAt     time.Time  `json:"started_at"`
	ShutdownAt    *time.Time `json:"shutdown_at"`
}

type User struct {
	ID          uuid.UUID           `json:"uuid"`
	CreatedAt   time.Time           `json:"created_at"`
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
	Icon         bool      `json:"icon"`
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
	ID             uuid.UUID `json:"id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	OwnerID        uuid.UUID `json:"owner_id"`
	TemplateID     uuid.UUID `json:"template_id"`
	CreatedAt      time.Time `json:"created_at"`
	Deleted        bool      `json:"deleted"`
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

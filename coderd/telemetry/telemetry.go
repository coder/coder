package telemetry

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/elastic/go-sysinfo"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/buildinfo"
	clitelemetry "github.com/coder/coder/v2/cli/telemetry"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk"
	tailnetproto "github.com/coder/coder/v2/tailnet/proto"
)

const (
	// VersionHeader is sent in every telemetry request to
	// report the semantic version of Coder.
	VersionHeader = "X-Coder-Version"
)

type Options struct {
	Disabled bool
	Database database.Store
	Logger   slog.Logger
	// URL is an endpoint to direct telemetry towards!
	URL *url.URL

	DeploymentID     string
	DeploymentConfig *codersdk.DeploymentValues
	BuiltinPostgres  bool
	Tunnel           bool

	SnapshotFrequency time.Duration
	ParseLicenseJWT   func(lic *License) error
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
	Enabled() bool
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

func (r *remoteReporter) Enabled() bool {
	return !r.options.Disabled
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
	if r.Enabled() {
		// Report a final collection of telemetry prior to close!
		// This could indicate final actions a user has taken, and
		// the time the deployment was shutdown.
		r.reportWithDeployment()
	}
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

// See the corresponding test in telemetry_test.go for a truth table.
func ShouldReportTelemetryDisabled(recordedTelemetryEnabled *bool, telemetryEnabled bool) bool {
	return recordedTelemetryEnabled != nil && *recordedTelemetryEnabled && !telemetryEnabled
}

// RecordTelemetryStatus records the telemetry status in the database.
// If the status changed from enabled to disabled, returns a snapshot to
// be sent to the telemetry server.
func RecordTelemetryStatus( //nolint:revive
	ctx context.Context,
	logger slog.Logger,
	db database.Store,
	telemetryEnabled bool,
) (*Snapshot, error) {
	item, err := db.GetTelemetryItem(ctx, string(TelemetryItemKeyTelemetryEnabled))
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, xerrors.Errorf("get telemetry enabled: %w", err)
	}
	var recordedTelemetryEnabled *bool
	if !errors.Is(err, sql.ErrNoRows) {
		value, err := strconv.ParseBool(item.Value)
		if err != nil {
			logger.Debug(ctx, "parse telemetry enabled", slog.Error(err))
		}
		// If ParseBool fails, value will default to false.
		// This may happen if an admin manually edits the telemetry item
		// in the database.
		recordedTelemetryEnabled = &value
	}

	if err := db.UpsertTelemetryItem(ctx, database.UpsertTelemetryItemParams{
		Key:   string(TelemetryItemKeyTelemetryEnabled),
		Value: strconv.FormatBool(telemetryEnabled),
	}); err != nil {
		return nil, xerrors.Errorf("upsert telemetry enabled: %w", err)
	}

	shouldReport := ShouldReportTelemetryDisabled(recordedTelemetryEnabled, telemetryEnabled)
	if !shouldReport {
		return nil, nil //nolint:nilnil
	}
	// If any of the following calls fail, we will never report that telemetry changed
	// from enabled to disabled. This is okay. We only want to ping the telemetry server
	// once, and never again. If that attempt fails, so be it.
	item, err = db.GetTelemetryItem(ctx, string(TelemetryItemKeyTelemetryEnabled))
	if err != nil {
		return nil, xerrors.Errorf("get telemetry enabled after upsert: %w", err)
	}
	return &Snapshot{
		TelemetryItems: []TelemetryItem{
			ConvertTelemetryItem(item),
		},
	}, nil
}

func (r *remoteReporter) runSnapshotter() {
	telemetryDisabledSnapshot, err := RecordTelemetryStatus(r.ctx, r.options.Logger, r.options.Database, r.Enabled())
	if err != nil {
		r.options.Logger.Debug(r.ctx, "record and maybe report telemetry status", slog.Error(err))
	}
	if telemetryDisabledSnapshot != nil {
		r.reportSync(telemetryDisabledSnapshot)
	}
	r.options.Logger.Debug(r.ctx, "finished telemetry status check")
	if !r.Enabled() {
		return
	}

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

	idpOrgSync, err := checkIDPOrgSync(r.ctx, r.options.Database, r.options.DeploymentConfig)
	if err != nil {
		r.options.Logger.Debug(r.ctx, "check IDP org sync", slog.Error(err))
	}

	data, err := json.Marshal(&Deployment{
		ID:              r.options.DeploymentID,
		Architecture:    sysInfo.Architecture,
		BuiltinPostgres: r.options.BuiltinPostgres,
		Containerized:   containerized,
		Config:          r.options.DeploymentConfig,
		Kubernetes:      os.Getenv("KUBERNETES_SERVICE_HOST") != "",
		InstallSource:   installSource,
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
		IDPOrgSync:      &idpOrgSync,
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

// idpOrgSyncConfig is a subset of
// https://github.com/coder/coder/blob/5c6578d84e2940b9cfd04798c45e7c8042c3fe0e/coderd/idpsync/organization.go#L148
type idpOrgSyncConfig struct {
	Field string `json:"field"`
}

// checkIDPOrgSync inspects the server flags and the runtime config. It's based on
// the OrganizationSyncEnabled function from enterprise/coderd/enidpsync/organizations.go.
// It has one distinct difference: it doesn't check if the license entitles to the
// feature, it only checks if the feature is configured.
//
// The above function is not used because it's very hard to make it available in
// the telemetry package due to coder/coder package structure and initialization
// order of the coder server.
//
// We don't check license entitlements because it's also hard to do from the
// telemetry package, and the config check should be sufficient for telemetry purposes.
//
// While this approach duplicates code, it's simpler than the alternative.
//
// See https://github.com/coder/coder/pull/16323 for more details.
func checkIDPOrgSync(ctx context.Context, db database.Store, values *codersdk.DeploymentValues) (bool, error) {
	// key based on https://github.com/coder/coder/blob/5c6578d84e2940b9cfd04798c45e7c8042c3fe0e/coderd/idpsync/idpsync.go#L168
	syncConfigRaw, err := db.GetRuntimeConfig(ctx, "organization-sync-settings")
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// If the runtime config is not set, we check if the deployment config
			// has the organization field set.
			return values != nil && values.OIDC.OrganizationField != "", nil
		}
		return false, xerrors.Errorf("get runtime config: %w", err)
	}
	syncConfig := idpOrgSyncConfig{}
	if err := json.Unmarshal([]byte(syncConfigRaw), &syncConfig); err != nil {
		return false, xerrors.Errorf("unmarshal runtime config: %w", err)
	}
	return syncConfig.Field != "", nil
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
		groups, err := r.options.Database.GetGroups(ctx, database.GetGroupsParams{})
		if err != nil {
			return xerrors.Errorf("get groups: %w", err)
		}
		snapshot.Groups = make([]Group, 0, len(groups))
		for _, group := range groups {
			snapshot.Groups = append(snapshot.Groups, ConvertGroup(group.Group))
		}
		return nil
	})
	eg.Go(func() error {
		groupMembers, err := r.options.Database.GetGroupMembers(ctx)
		if err != nil {
			return xerrors.Errorf("get groups: %w", err)
		}
		snapshot.GroupMembers = make([]GroupMember, 0, len(groupMembers))
		for _, member := range groupMembers {
			snapshot.GroupMembers = append(snapshot.GroupMembers, ConvertGroupMember(member))
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
		workspaceModules, err := r.options.Database.GetWorkspaceModulesCreatedAfter(ctx, createdAfter)
		if err != nil {
			return xerrors.Errorf("get workspace modules: %w", err)
		}
		snapshot.WorkspaceModules = make([]WorkspaceModule, 0, len(workspaceModules))
		for _, module := range workspaceModules {
			snapshot.WorkspaceModules = append(snapshot.WorkspaceModules, ConvertWorkspaceModule(module))
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
		if r.options.DeploymentConfig != nil && slices.Contains(r.options.DeploymentConfig.Experiments, string(codersdk.ExperimentWorkspaceUsage)) {
			agentStats, err := r.options.Database.GetWorkspaceAgentUsageStats(ctx, createdAfter)
			if err != nil {
				return xerrors.Errorf("get workspace agent stats: %w", err)
			}
			snapshot.WorkspaceAgentStats = make([]WorkspaceAgentStat, 0, len(agentStats))
			for _, stat := range agentStats {
				snapshot.WorkspaceAgentStats = append(snapshot.WorkspaceAgentStats, ConvertWorkspaceAgentStat(database.GetWorkspaceAgentStatsRow(stat)))
			}
		} else {
			agentStats, err := r.options.Database.GetWorkspaceAgentStats(ctx, createdAfter)
			if err != nil {
				return xerrors.Errorf("get workspace agent stats: %w", err)
			}
			snapshot.WorkspaceAgentStats = make([]WorkspaceAgentStat, 0, len(agentStats))
			for _, stat := range agentStats {
				snapshot.WorkspaceAgentStats = append(snapshot.WorkspaceAgentStats, ConvertWorkspaceAgentStat(stat))
			}
		}
		return nil
	})
	eg.Go(func() error {
		memoryMonitors, err := r.options.Database.FetchMemoryResourceMonitorsUpdatedAfter(ctx, createdAfter)
		if err != nil {
			return xerrors.Errorf("get memory resource monitors: %w", err)
		}
		snapshot.WorkspaceAgentMemoryResourceMonitors = make([]WorkspaceAgentMemoryResourceMonitor, 0, len(memoryMonitors))
		for _, monitor := range memoryMonitors {
			snapshot.WorkspaceAgentMemoryResourceMonitors = append(snapshot.WorkspaceAgentMemoryResourceMonitors, ConvertWorkspaceAgentMemoryResourceMonitor(monitor))
		}
		return nil
	})
	eg.Go(func() error {
		volumeMonitors, err := r.options.Database.FetchVolumesResourceMonitorsUpdatedAfter(ctx, createdAfter)
		if err != nil {
			return xerrors.Errorf("get volume resource monitors: %w", err)
		}
		snapshot.WorkspaceAgentVolumeResourceMonitors = make([]WorkspaceAgentVolumeResourceMonitor, 0, len(volumeMonitors))
		for _, monitor := range volumeMonitors {
			snapshot.WorkspaceAgentVolumeResourceMonitors = append(snapshot.WorkspaceAgentVolumeResourceMonitors, ConvertWorkspaceAgentVolumeResourceMonitor(monitor))
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
	eg.Go(func() error {
		// Warning: When an organization is deleted, it's completely removed from
		// the database. It will no longer be reported, and there will be no other
		// indicator that it was deleted. This requires special handling when
		// interpreting the telemetry data later.
		orgs, err := r.options.Database.GetOrganizations(r.ctx, database.GetOrganizationsParams{})
		if err != nil {
			return xerrors.Errorf("get organizations: %w", err)
		}
		snapshot.Organizations = make([]Organization, 0, len(orgs))
		for _, org := range orgs {
			snapshot.Organizations = append(snapshot.Organizations, ConvertOrganization(org))
		}
		return nil
	})
	eg.Go(func() error {
		items, err := r.options.Database.GetTelemetryItems(ctx)
		if err != nil {
			return xerrors.Errorf("get telemetry items: %w", err)
		}
		snapshot.TelemetryItems = make([]TelemetryItem, 0, len(items))
		for _, item := range items {
			snapshot.TelemetryItems = append(snapshot.TelemetryItems, ConvertTelemetryItem(item))
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

func ConvertWorkspaceAgentMemoryResourceMonitor(monitor database.WorkspaceAgentMemoryResourceMonitor) WorkspaceAgentMemoryResourceMonitor {
	return WorkspaceAgentMemoryResourceMonitor{
		AgentID:   monitor.AgentID,
		Enabled:   monitor.Enabled,
		Threshold: monitor.Threshold,
		CreatedAt: monitor.CreatedAt,
		UpdatedAt: monitor.UpdatedAt,
	}
}

func ConvertWorkspaceAgentVolumeResourceMonitor(monitor database.WorkspaceAgentVolumeResourceMonitor) WorkspaceAgentVolumeResourceMonitor {
	return WorkspaceAgentVolumeResourceMonitor{
		AgentID:   monitor.AgentID,
		Enabled:   monitor.Enabled,
		Threshold: monitor.Threshold,
		CreatedAt: monitor.CreatedAt,
		UpdatedAt: monitor.UpdatedAt,
	}
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
	r := WorkspaceResource{
		ID:           resource.ID,
		JobID:        resource.JobID,
		CreatedAt:    resource.CreatedAt,
		Transition:   resource.Transition,
		Type:         resource.Type,
		InstanceType: resource.InstanceType.String,
	}
	if resource.ModulePath.Valid {
		r.ModulePath = &resource.ModulePath.String
	}
	return r
}

// ConvertWorkspaceResourceMetadata anonymizes workspace metadata.
func ConvertWorkspaceResourceMetadata(metadata database.WorkspaceResourceMetadatum) WorkspaceResourceMetadata {
	return WorkspaceResourceMetadata{
		ResourceID: metadata.WorkspaceResourceID,
		Key:        metadata.Key,
		Sensitive:  metadata.Sensitive,
	}
}

func shouldSendRawModuleSource(source string) bool {
	return strings.Contains(source, "registry.coder.com")
}

// ModuleSourceType is the type of source for a module.
// For reference, see https://developer.hashicorp.com/terraform/language/modules/sources
type ModuleSourceType string

const (
	ModuleSourceTypeLocal           ModuleSourceType = "local"
	ModuleSourceTypeLocalAbs        ModuleSourceType = "local_absolute"
	ModuleSourceTypePublicRegistry  ModuleSourceType = "public_registry"
	ModuleSourceTypePrivateRegistry ModuleSourceType = "private_registry"
	ModuleSourceTypeCoderRegistry   ModuleSourceType = "coder_registry"
	ModuleSourceTypeGitHub          ModuleSourceType = "github"
	ModuleSourceTypeBitbucket       ModuleSourceType = "bitbucket"
	ModuleSourceTypeGit             ModuleSourceType = "git"
	ModuleSourceTypeMercurial       ModuleSourceType = "mercurial"
	ModuleSourceTypeHTTP            ModuleSourceType = "http"
	ModuleSourceTypeS3              ModuleSourceType = "s3"
	ModuleSourceTypeGCS             ModuleSourceType = "gcs"
	ModuleSourceTypeUnknown         ModuleSourceType = "unknown"
)

// Terraform supports a variety of module source types, like:
//   - local paths (./ or ../)
//   - absolute local paths (/)
//   - git URLs (git:: or git@)
//   - http URLs
//   - s3 URLs
//
// and more!
//
// See https://developer.hashicorp.com/terraform/language/modules/sources for an overview.
//
// This function attempts to classify the source type of a module. It's imperfect,
// as checks that terraform actually does are pretty complicated.
// See e.g. https://github.com/hashicorp/go-getter/blob/842d6c379e5e70d23905b8f6b5a25a80290acb66/detect.go#L47
// if you're interested in the complexity.
func GetModuleSourceType(source string) ModuleSourceType {
	source = strings.TrimSpace(source)
	source = strings.ToLower(source)
	if strings.HasPrefix(source, "./") || strings.HasPrefix(source, "../") {
		return ModuleSourceTypeLocal
	}
	if strings.HasPrefix(source, "/") {
		return ModuleSourceTypeLocalAbs
	}
	// Match public registry modules in the format <NAMESPACE>/<NAME>/<PROVIDER>
	// Sources can have a `//...` suffix, which signifies a subdirectory.
	// The allowed characters are based on
	// https://developer.hashicorp.com/terraform/cloud-docs/api-docs/private-registry/modules#request-body-1
	// because Hashicorp's documentation about module sources doesn't mention it.
	if matched, _ := regexp.MatchString(`^[a-zA-Z0-9_-]+/[a-zA-Z0-9_-]+/[a-zA-Z0-9_-]+(//.*)?$`, source); matched {
		return ModuleSourceTypePublicRegistry
	}
	if strings.Contains(source, "github.com") {
		return ModuleSourceTypeGitHub
	}
	if strings.Contains(source, "bitbucket.org") {
		return ModuleSourceTypeBitbucket
	}
	if strings.HasPrefix(source, "git::") || strings.HasPrefix(source, "git@") {
		return ModuleSourceTypeGit
	}
	if strings.HasPrefix(source, "hg::") {
		return ModuleSourceTypeMercurial
	}
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		return ModuleSourceTypeHTTP
	}
	if strings.HasPrefix(source, "s3::") {
		return ModuleSourceTypeS3
	}
	if strings.HasPrefix(source, "gcs::") {
		return ModuleSourceTypeGCS
	}
	if strings.Contains(source, "registry.terraform.io") {
		return ModuleSourceTypePublicRegistry
	}
	if strings.Contains(source, "app.terraform.io") || strings.Contains(source, "localterraform.com") {
		return ModuleSourceTypePrivateRegistry
	}
	if strings.Contains(source, "registry.coder.com") {
		return ModuleSourceTypeCoderRegistry
	}
	return ModuleSourceTypeUnknown
}

func ConvertWorkspaceModule(module database.WorkspaceModule) WorkspaceModule {
	source := module.Source
	version := module.Version
	sourceType := GetModuleSourceType(source)
	if !shouldSendRawModuleSource(source) {
		source = fmt.Sprintf("%x", sha256.Sum256([]byte(source)))
		version = fmt.Sprintf("%x", sha256.Sum256([]byte(version)))
	}

	return WorkspaceModule{
		ID:         module.ID,
		JobID:      module.JobID,
		Transition: module.Transition,
		Source:     source,
		Version:    version,
		SourceType: sourceType,
		Key:        module.Key,
		CreatedAt:  module.CreatedAt,
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
		ID:              dbUser.ID,
		EmailHashed:     emailHashed,
		RBACRoles:       dbUser.RBACRoles,
		CreatedAt:       dbUser.CreatedAt,
		Status:          dbUser.Status,
		GithubComUserID: dbUser.GithubComUserID.Int64,
		LoginType:       string(dbUser.LoginType),
	}
}

func ConvertGroup(group database.Group) Group {
	return Group{
		ID:             group.ID,
		Name:           group.Name,
		OrganizationID: group.OrganizationID,
		AvatarURL:      group.AvatarURL,
		QuotaAllowance: group.QuotaAllowance,
		DisplayName:    group.DisplayName,
		Source:         group.Source,
	}
}

func ConvertGroupMember(member database.GroupMember) GroupMember {
	return GroupMember{
		GroupID: member.GroupID,
		UserID:  member.UserID,
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

		// Some of these fields are meant to be accessed using a specialized
		// interface (for entitlement purposes), but for telemetry purposes
		// there's minimal harm accessing them directly.
		DefaultTTLMillis:               time.Duration(dbTemplate.DefaultTTL).Milliseconds(),
		AllowUserCancelWorkspaceJobs:   dbTemplate.AllowUserCancelWorkspaceJobs,
		AllowUserAutostart:             dbTemplate.AllowUserAutostart,
		AllowUserAutostop:              dbTemplate.AllowUserAutostop,
		FailureTTLMillis:               time.Duration(dbTemplate.FailureTTL).Milliseconds(),
		TimeTilDormantMillis:           time.Duration(dbTemplate.TimeTilDormant).Milliseconds(),
		TimeTilDormantAutoDeleteMillis: time.Duration(dbTemplate.TimeTilDormantAutoDelete).Milliseconds(),
		AutostopRequirementDaysOfWeek:  codersdk.BitmapToWeekdays(uint8(dbTemplate.AutostopRequirementDaysOfWeek)),
		AutostopRequirementWeeks:       dbTemplate.AutostopRequirementWeeks,
		AutostartAllowedDays:           codersdk.BitmapToWeekdays(dbTemplate.AutostartAllowedDays()),
		RequireActiveVersion:           dbTemplate.RequireActiveVersion,
		Deprecated:                     dbTemplate.Deprecated != "",
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
	if version.SourceExampleID.Valid {
		snapVersion.SourceExampleID = &version.SourceExampleID.String
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

func ConvertOrganization(org database.Organization) Organization {
	return Organization{
		ID:        org.ID,
		CreatedAt: org.CreatedAt,
		IsDefault: org.IsDefault,
	}
}

func ConvertTelemetryItem(item database.TelemetryItem) TelemetryItem {
	return TelemetryItem{
		Key:       item.Key,
		Value:     item.Value,
		CreatedAt: item.CreatedAt,
		UpdatedAt: item.UpdatedAt,
	}
}

// Snapshot represents a point-in-time anonymized database dump.
// Data is aggregated by latest on the server-side, so partial data
// can be sent without issue.
type Snapshot struct {
	DeploymentID string `json:"deployment_id"`

	APIKeys                              []APIKey                              `json:"api_keys"`
	CLIInvocations                       []clitelemetry.Invocation             `json:"cli_invocations"`
	ExternalProvisioners                 []ExternalProvisioner                 `json:"external_provisioners"`
	Licenses                             []License                             `json:"licenses"`
	ProvisionerJobs                      []ProvisionerJob                      `json:"provisioner_jobs"`
	TemplateVersions                     []TemplateVersion                     `json:"template_versions"`
	Templates                            []Template                            `json:"templates"`
	Users                                []User                                `json:"users"`
	Groups                               []Group                               `json:"groups"`
	GroupMembers                         []GroupMember                         `json:"group_members"`
	WorkspaceAgentStats                  []WorkspaceAgentStat                  `json:"workspace_agent_stats"`
	WorkspaceAgents                      []WorkspaceAgent                      `json:"workspace_agents"`
	WorkspaceApps                        []WorkspaceApp                        `json:"workspace_apps"`
	WorkspaceBuilds                      []WorkspaceBuild                      `json:"workspace_build"`
	WorkspaceProxies                     []WorkspaceProxy                      `json:"workspace_proxies"`
	WorkspaceResourceMetadata            []WorkspaceResourceMetadata           `json:"workspace_resource_metadata"`
	WorkspaceResources                   []WorkspaceResource                   `json:"workspace_resources"`
	WorkspaceAgentMemoryResourceMonitors []WorkspaceAgentMemoryResourceMonitor `json:"workspace_agent_memory_resource_monitors"`
	WorkspaceAgentVolumeResourceMonitors []WorkspaceAgentVolumeResourceMonitor `json:"workspace_agent_volume_resource_monitors"`
	WorkspaceModules                     []WorkspaceModule                     `json:"workspace_modules"`
	Workspaces                           []Workspace                           `json:"workspaces"`
	NetworkEvents                        []NetworkEvent                        `json:"network_events"`
	Organizations                        []Organization                        `json:"organizations"`
	TelemetryItems                       []TelemetryItem                       `json:"telemetry_items"`
}

// Deployment contains information about the host running Coder.
type Deployment struct {
	ID              string                     `json:"id"`
	Architecture    string                     `json:"architecture"`
	BuiltinPostgres bool                       `json:"builtin_postgres"`
	Containerized   bool                       `json:"containerized"`
	Kubernetes      bool                       `json:"kubernetes"`
	Config          *codersdk.DeploymentValues `json:"config"`
	Tunnel          bool                       `json:"tunnel"`
	InstallSource   string                     `json:"install_source"`
	OSType          string                     `json:"os_type"`
	OSFamily        string                     `json:"os_family"`
	OSPlatform      string                     `json:"os_platform"`
	OSName          string                     `json:"os_name"`
	OSVersion       string                     `json:"os_version"`
	CPUCores        int                        `json:"cpu_cores"`
	MemoryTotal     uint64                     `json:"memory_total"`
	MachineID       string                     `json:"machine_id"`
	StartedAt       time.Time                  `json:"started_at"`
	ShutdownAt      *time.Time                 `json:"shutdown_at"`
	// While IDPOrgSync will always be set, it's nullable to make
	// the struct backwards compatible with older coder versions.
	IDPOrgSync *bool `json:"idp_org_sync"`
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
	Email           *string             `json:"email"`
	EmailHashed     string              `json:"email_hashed"`
	RBACRoles       []string            `json:"rbac_roles"`
	Status          database.UserStatus `json:"status"`
	GithubComUserID int64               `json:"github_com_user_id"`
	// Omitempty for backwards compatibility.
	LoginType string `json:"login_type,omitempty"`
}

type Group struct {
	ID             uuid.UUID            `json:"id"`
	Name           string               `json:"name"`
	OrganizationID uuid.UUID            `json:"organization_id"`
	AvatarURL      string               `json:"avatar_url"`
	QuotaAllowance int32                `json:"quota_allowance"`
	DisplayName    string               `json:"display_name"`
	Source         database.GroupSource `json:"source"`
}

type GroupMember struct {
	UserID  uuid.UUID `json:"user_id"`
	GroupID uuid.UUID `json:"group_id"`
}

type WorkspaceResource struct {
	ID           uuid.UUID                    `json:"id"`
	CreatedAt    time.Time                    `json:"created_at"`
	JobID        uuid.UUID                    `json:"job_id"`
	Transition   database.WorkspaceTransition `json:"transition"`
	Type         string                       `json:"type"`
	InstanceType string                       `json:"instance_type"`
	// ModulePath is nullable because it was added a long time after the
	// original workspace resource telemetry was added. All new resources
	// will have a module path, but deployments with older resources still
	// in the database will not.
	ModulePath *string `json:"module_path"`
}

type WorkspaceResourceMetadata struct {
	ResourceID uuid.UUID `json:"resource_id"`
	Key        string    `json:"key"`
	Sensitive  bool      `json:"sensitive"`
}

type WorkspaceModule struct {
	ID         uuid.UUID                    `json:"id"`
	CreatedAt  time.Time                    `json:"created_at"`
	JobID      uuid.UUID                    `json:"job_id"`
	Transition database.WorkspaceTransition `json:"transition"`
	Key        string                       `json:"key"`
	Version    string                       `json:"version"`
	Source     string                       `json:"source"`
	SourceType ModuleSourceType             `json:"source_type"`
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

type WorkspaceAgentMemoryResourceMonitor struct {
	AgentID   uuid.UUID `json:"agent_id"`
	Enabled   bool      `json:"enabled"`
	Threshold int32     `json:"threshold"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type WorkspaceAgentVolumeResourceMonitor struct {
	AgentID   uuid.UUID `json:"agent_id"`
	Enabled   bool      `json:"enabled"`
	Threshold int32     `json:"threshold"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
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

	DefaultTTLMillis               int64    `json:"default_ttl_ms"`
	AllowUserCancelWorkspaceJobs   bool     `json:"allow_user_cancel_workspace_jobs"`
	AllowUserAutostart             bool     `json:"allow_user_autostart"`
	AllowUserAutostop              bool     `json:"allow_user_autostop"`
	FailureTTLMillis               int64    `json:"failure_ttl_ms"`
	TimeTilDormantMillis           int64    `json:"time_til_dormant_ms"`
	TimeTilDormantAutoDeleteMillis int64    `json:"time_til_dormant_auto_delete_ms"`
	AutostopRequirementDaysOfWeek  []string `json:"autostop_requirement_days_of_week"`
	AutostopRequirementWeeks       int64    `json:"autostop_requirement_weeks"`
	AutostartAllowedDays           []string `json:"autostart_allowed_days"`
	RequireActiveVersion           bool     `json:"require_active_version"`
	Deprecated                     bool     `json:"deprecated"`
}

type TemplateVersion struct {
	ID              uuid.UUID  `json:"id"`
	CreatedAt       time.Time  `json:"created_at"`
	TemplateID      *uuid.UUID `json:"template_id,omitempty"`
	OrganizationID  uuid.UUID  `json:"organization_id"`
	JobID           uuid.UUID  `json:"job_id"`
	SourceExampleID *string    `json:"source_example_id,omitempty"`
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

type NetworkEventIPFields struct {
	Version int32  `json:"version"` // 4 or 6
	Class   string `json:"class"`   // public, private, link_local, unique_local, loopback
}

func ipFieldsFromProto(proto *tailnetproto.IPFields) NetworkEventIPFields {
	if proto == nil {
		return NetworkEventIPFields{}
	}
	return NetworkEventIPFields{
		Version: proto.Version,
		Class:   strings.ToLower(proto.Class.String()),
	}
}

type NetworkEventP2PEndpoint struct {
	Hash   string               `json:"hash"`
	Port   int                  `json:"port"`
	Fields NetworkEventIPFields `json:"fields"`
}

func p2pEndpointFromProto(proto *tailnetproto.TelemetryEvent_P2PEndpoint) NetworkEventP2PEndpoint {
	if proto == nil {
		return NetworkEventP2PEndpoint{}
	}
	return NetworkEventP2PEndpoint{
		Hash:   proto.Hash,
		Port:   int(proto.Port),
		Fields: ipFieldsFromProto(proto.Fields),
	}
}

type DERPMapHomeParams struct {
	RegionScore map[int64]float64 `json:"region_score"`
}

func derpMapHomeParamsFromProto(proto *tailnetproto.DERPMap_HomeParams) DERPMapHomeParams {
	if proto == nil {
		return DERPMapHomeParams{}
	}
	out := DERPMapHomeParams{
		RegionScore: make(map[int64]float64, len(proto.RegionScore)),
	}
	for k, v := range proto.RegionScore {
		out.RegionScore[k] = v
	}
	return out
}

type DERPRegion struct {
	RegionID      int64 `json:"region_id"`
	EmbeddedRelay bool  `json:"embedded_relay"`
	RegionCode    string
	RegionName    string
	Avoid         bool
	Nodes         []DERPNode `json:"nodes"`
}

func derpRegionFromProto(proto *tailnetproto.DERPMap_Region) DERPRegion {
	if proto == nil {
		return DERPRegion{}
	}
	nodes := make([]DERPNode, 0, len(proto.Nodes))
	for _, node := range proto.Nodes {
		nodes = append(nodes, derpNodeFromProto(node))
	}
	return DERPRegion{
		RegionID:      proto.RegionId,
		EmbeddedRelay: proto.EmbeddedRelay,
		RegionCode:    proto.RegionCode,
		RegionName:    proto.RegionName,
		Avoid:         proto.Avoid,
		Nodes:         nodes,
	}
}

type DERPNode struct {
	Name             string `json:"name"`
	RegionID         int64  `json:"region_id"`
	HostName         string `json:"host_name"`
	CertName         string `json:"cert_name"`
	IPv4             string `json:"ipv4"`
	IPv6             string `json:"ipv6"`
	STUNPort         int32  `json:"stun_port"`
	STUNOnly         bool   `json:"stun_only"`
	DERPPort         int32  `json:"derp_port"`
	InsecureForTests bool   `json:"insecure_for_tests"`
	ForceHTTP        bool   `json:"force_http"`
	STUNTestIP       string `json:"stun_test_ip"`
	CanPort80        bool   `json:"can_port_80"`
}

func derpNodeFromProto(proto *tailnetproto.DERPMap_Region_Node) DERPNode {
	if proto == nil {
		return DERPNode{}
	}
	return DERPNode{
		Name:             proto.Name,
		RegionID:         proto.RegionId,
		HostName:         proto.HostName,
		CertName:         proto.CertName,
		IPv4:             proto.Ipv4,
		IPv6:             proto.Ipv6,
		STUNPort:         proto.StunPort,
		STUNOnly:         proto.StunOnly,
		DERPPort:         proto.DerpPort,
		InsecureForTests: proto.InsecureForTests,
		ForceHTTP:        proto.ForceHttp,
		STUNTestIP:       proto.StunTestIp,
		CanPort80:        proto.CanPort_80,
	}
}

type DERPMap struct {
	HomeParams DERPMapHomeParams `json:"home_params"`
	Regions    map[int64]DERPRegion
}

func derpMapFromProto(proto *tailnetproto.DERPMap) DERPMap {
	if proto == nil {
		return DERPMap{}
	}
	regionMap := make(map[int64]DERPRegion, len(proto.Regions))
	for k, v := range proto.Regions {
		regionMap[k] = derpRegionFromProto(v)
	}
	return DERPMap{
		HomeParams: derpMapHomeParamsFromProto(proto.HomeParams),
		Regions:    regionMap,
	}
}

type NetcheckIP struct {
	Hash   string               `json:"hash"`
	Fields NetworkEventIPFields `json:"fields"`
}

func netcheckIPFromProto(proto *tailnetproto.Netcheck_NetcheckIP) NetcheckIP {
	if proto == nil {
		return NetcheckIP{}
	}
	return NetcheckIP{
		Hash:   proto.Hash,
		Fields: ipFieldsFromProto(proto.Fields),
	}
}

type Netcheck struct {
	UDP         bool `json:"udp"`
	IPv6        bool `json:"ipv6"`
	IPv4        bool `json:"ipv4"`
	IPv6CanSend bool `json:"ipv6_can_send"`
	IPv4CanSend bool `json:"ipv4_can_send"`
	ICMPv4      bool `json:"icmpv4"`

	OSHasIPv6             *bool `json:"os_has_ipv6"`
	MappingVariesByDestIP *bool `json:"mapping_varies_by_dest_ip"`
	HairPinning           *bool `json:"hair_pinning"`
	UPnP                  *bool `json:"upnp"`
	PMP                   *bool `json:"pmp"`
	PCP                   *bool `json:"pcp"`

	PreferredDERP int64 `json:"preferred_derp"`

	RegionV4Latency map[int64]time.Duration `json:"region_v4_latency"`
	RegionV6Latency map[int64]time.Duration `json:"region_v6_latency"`

	GlobalV4 NetcheckIP `json:"global_v4"`
	GlobalV6 NetcheckIP `json:"global_v6"`
}

func protoBool(b *wrapperspb.BoolValue) *bool {
	if b == nil {
		return nil
	}
	return &b.Value
}

func netcheckFromProto(proto *tailnetproto.Netcheck) Netcheck {
	if proto == nil {
		return Netcheck{}
	}

	durationMapFromProto := func(m map[int64]*durationpb.Duration) map[int64]time.Duration {
		out := make(map[int64]time.Duration, len(m))
		for k, v := range m {
			out[k] = v.AsDuration()
		}
		return out
	}

	return Netcheck{
		UDP:         proto.UDP,
		IPv6:        proto.IPv6,
		IPv4:        proto.IPv4,
		IPv6CanSend: proto.IPv6CanSend,
		IPv4CanSend: proto.IPv4CanSend,
		ICMPv4:      proto.ICMPv4,

		OSHasIPv6:             protoBool(proto.OSHasIPv6),
		MappingVariesByDestIP: protoBool(proto.MappingVariesByDestIP),
		HairPinning:           protoBool(proto.HairPinning),
		UPnP:                  protoBool(proto.UPnP),
		PMP:                   protoBool(proto.PMP),
		PCP:                   protoBool(proto.PCP),

		PreferredDERP: proto.PreferredDERP,

		RegionV4Latency: durationMapFromProto(proto.RegionV4Latency),
		RegionV6Latency: durationMapFromProto(proto.RegionV6Latency),

		GlobalV4: netcheckIPFromProto(proto.GlobalV4),
		GlobalV6: netcheckIPFromProto(proto.GlobalV6),
	}
}

// NetworkEvent and all related structs come from tailnet.proto.
type NetworkEvent struct {
	ID             uuid.UUID               `json:"id"`
	Time           time.Time               `json:"time"`
	Application    string                  `json:"application"`
	Status         string                  `json:"status"`      // connected, disconnected
	ClientType     string                  `json:"client_type"` // cli, agent, coderd, wsproxy
	ClientVersion  string                  `json:"client_version"`
	NodeIDSelf     uint64                  `json:"node_id_self"`
	NodeIDRemote   uint64                  `json:"node_id_remote"`
	P2PEndpoint    NetworkEventP2PEndpoint `json:"p2p_endpoint"`
	HomeDERP       int                     `json:"home_derp"`
	DERPMap        DERPMap                 `json:"derp_map"`
	LatestNetcheck Netcheck                `json:"latest_netcheck"`

	ConnectionAge   *time.Duration `json:"connection_age"`
	ConnectionSetup *time.Duration `json:"connection_setup"`
	P2PSetup        *time.Duration `json:"p2p_setup"`
	DERPLatency     *time.Duration `json:"derp_latency"`
	P2PLatency      *time.Duration `json:"p2p_latency"`
	ThroughputMbits *float32       `json:"throughput_mbits"`
}

func protoFloat(f *wrapperspb.FloatValue) *float32 {
	if f == nil {
		return nil
	}
	return &f.Value
}

func protoDurationNil(d *durationpb.Duration) *time.Duration {
	if d == nil {
		return nil
	}
	dur := d.AsDuration()
	return &dur
}

func NetworkEventFromProto(proto *tailnetproto.TelemetryEvent) (NetworkEvent, error) {
	if proto == nil {
		return NetworkEvent{}, xerrors.New("nil event")
	}
	id, err := uuid.FromBytes(proto.Id)
	if err != nil {
		return NetworkEvent{}, xerrors.Errorf("parse id %q: %w", proto.Id, err)
	}

	return NetworkEvent{
		ID:             id,
		Time:           proto.Time.AsTime(),
		Application:    proto.Application,
		Status:         strings.ToLower(proto.Status.String()),
		ClientType:     strings.ToLower(proto.ClientType.String()),
		ClientVersion:  proto.ClientVersion,
		NodeIDSelf:     proto.NodeIdSelf,
		NodeIDRemote:   proto.NodeIdRemote,
		P2PEndpoint:    p2pEndpointFromProto(proto.P2PEndpoint),
		HomeDERP:       int(proto.HomeDerp),
		DERPMap:        derpMapFromProto(proto.DerpMap),
		LatestNetcheck: netcheckFromProto(proto.LatestNetcheck),

		ConnectionAge:   protoDurationNil(proto.ConnectionAge),
		ConnectionSetup: protoDurationNil(proto.ConnectionSetup),
		P2PSetup:        protoDurationNil(proto.P2PSetup),
		DERPLatency:     protoDurationNil(proto.DerpLatency),
		P2PLatency:      protoDurationNil(proto.P2PLatency),
		ThroughputMbits: protoFloat(proto.ThroughputMbits),
	}, nil
}

type Organization struct {
	ID        uuid.UUID `json:"id"`
	IsDefault bool      `json:"is_default"`
	CreatedAt time.Time `json:"created_at"`
}

type telemetryItemKey string

// The comment below gets rid of the warning that the name "TelemetryItemKey" has
// the "Telemetry" prefix, and that stutters when you use it outside the package
// (telemetry.TelemetryItemKey...). "TelemetryItem" is the name of a database table,
// so it makes sense to use the "Telemetry" prefix.
//
//revive:disable:exported
const (
	TelemetryItemKeyHTMLFirstServedAt telemetryItemKey = "html_first_served_at"
	TelemetryItemKeyTelemetryEnabled  telemetryItemKey = "telemetry_enabled"
)

type TelemetryItem struct {
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type noopReporter struct{}

func (*noopReporter) Report(_ *Snapshot)            {}
func (*noopReporter) Enabled() bool                 { return false }
func (*noopReporter) Close()                        {}
func (*noopReporter) RunSnapshotter()               {}
func (*noopReporter) ReportDisabledIfNeeded() error { return nil }

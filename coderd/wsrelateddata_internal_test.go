package coderd

import (
	"database/sql"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"
	"tailscale.com/tailcfg"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/wsrelated"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/tailnettest"
	"github.com/coder/coder/v2/testutil"
)

// TestWorkspaceBuildsDataQueryGating asserts that workspaceBuildsData only
// issues the database queries implied by the selection. gomock is strict, so
// any query that is not set up here fails the test if invoked.
func TestWorkspaceBuildsDataQueryGating(t *testing.T) {
	t.Parallel()

	build := database.WorkspaceBuild{
		ID:                uuid.New(),
		JobID:             uuid.New(),
		WorkspaceID:       uuid.New(),
		TemplateVersionID: uuid.New(),
	}
	resource := database.WorkspaceResource{ID: uuid.New(), JobID: build.JobID}
	agent := database.WorkspaceAgent{ID: uuid.New(), ResourceID: resource.ID}
	app := database.WorkspaceApp{ID: uuid.New(), AgentID: agent.ID}

	expectJob := func(db *dbmock.MockStore) {
		db.EXPECT().GetProvisionerJobsByIDs(gomock.Any(), gomock.Any()).
			Return([]database.ProvisionerJob{}, nil)
		db.EXPECT().GetEligibleProvisionerDaemonsByProvisionerJobIDs(gomock.Any(), gomock.Any()).
			Return([]database.GetEligibleProvisionerDaemonsByProvisionerJobIDsRow{}, nil)
	}
	expectJobWithQueuePosition := func(db *dbmock.MockStore) {
		db.EXPECT().GetProvisionerJobsByIDsWithQueuePosition(gomock.Any(), gomock.Any()).
			Return([]database.GetProvisionerJobsByIDsWithQueuePositionRow{}, nil)
		db.EXPECT().GetEligibleProvisionerDaemonsByProvisionerJobIDs(gomock.Any(), gomock.Any()).
			Return([]database.GetEligibleProvisionerDaemonsByProvisionerJobIDsRow{}, nil)
	}
	expectTemplateVersion := func(db *dbmock.MockStore) {
		db.EXPECT().GetTemplateVersionsByIDs(gomock.Any(), gomock.Any()).
			Return([]database.TemplateVersion{}, nil)
	}
	expectResources := func(db *dbmock.MockStore) {
		db.EXPECT().GetWorkspaceResourcesByJobIDs(gomock.Any(), gomock.Any()).
			Return([]database.WorkspaceResource{resource}, nil)
	}
	expectAgents := func(db *dbmock.MockStore) {
		db.EXPECT().GetWorkspaceAgentsByResourceIDs(gomock.Any(), gomock.Any()).
			Return([]database.WorkspaceAgent{agent}, nil)
	}
	expectApps := func(db *dbmock.MockStore) {
		db.EXPECT().GetWorkspaceAppsByAgentIDs(gomock.Any(), gomock.Any()).
			Return([]database.WorkspaceApp{app}, nil)
	}

	cases := []struct {
		name  string
		cfg   wsrelated.LatestBuild
		setup func(*dbmock.MockStore)
	}{
		{
			name:  "BuildOnly",
			cfg:   wsrelated.LatestBuild{},
			setup: func(*dbmock.MockStore) {},
		},
		{
			name:  "Job",
			cfg:   wsrelated.LatestBuild{Job: &wsrelated.Job{}},
			setup: expectJob,
		},
		{
			name:  "JobWithQueuePosition",
			cfg:   wsrelated.LatestBuild{Job: &wsrelated.Job{QueuePosition: true}},
			setup: expectJobWithQueuePosition,
		},
		{
			name:  "TemplateVersion",
			cfg:   wsrelated.LatestBuild{TemplateVersion: true},
			setup: expectTemplateVersion,
		},
		{
			name:  "Resources",
			cfg:   wsrelated.LatestBuild{Resources: &wsrelated.Resources{}},
			setup: expectResources,
		},
		{
			name: "Metadata",
			cfg:  wsrelated.LatestBuild{Resources: &wsrelated.Resources{Metadata: true}},
			setup: func(db *dbmock.MockStore) {
				expectResources(db)
				db.EXPECT().GetWorkspaceResourceMetadataByResourceIDs(gomock.Any(), gomock.Any()).
					Return([]database.WorkspaceResourceMetadatum{}, nil)
			},
		},
		{
			name: "Agents",
			cfg:  wsrelated.LatestBuild{Resources: &wsrelated.Resources{Agents: &wsrelated.Agents{}}},
			setup: func(db *dbmock.MockStore) {
				expectResources(db)
				expectAgents(db)
			},
		},
		{
			name: "Apps",
			cfg:  wsrelated.LatestBuild{Resources: &wsrelated.Resources{Agents: &wsrelated.Agents{Apps: &wsrelated.Apps{}}}},
			setup: func(db *dbmock.MockStore) {
				expectResources(db)
				expectAgents(db)
				expectApps(db)
			},
		},
		{
			name: "AppStatuses",
			cfg:  wsrelated.LatestBuild{Resources: &wsrelated.Resources{Agents: &wsrelated.Agents{Apps: &wsrelated.Apps{Statuses: true}}}},
			setup: func(db *dbmock.MockStore) {
				expectResources(db)
				expectAgents(db)
				expectApps(db)
				db.EXPECT().GetWorkspaceAppStatusesByAppIDs(gomock.Any(), gomock.Any()).
					Return([]database.WorkspaceAppStatus{}, nil)
			},
		},
		{
			name: "Scripts",
			cfg:  wsrelated.LatestBuild{Resources: &wsrelated.Resources{Agents: &wsrelated.Agents{Scripts: true}}},
			setup: func(db *dbmock.MockStore) {
				expectResources(db)
				expectAgents(db)
				db.EXPECT().GetWorkspaceAgentScriptsByAgentIDs(gomock.Any(), gomock.Any()).
					Return([]database.GetWorkspaceAgentScriptsByAgentIDsRow{}, nil)
			},
		},
		{
			name: "LogSources",
			cfg:  wsrelated.LatestBuild{Resources: &wsrelated.Resources{Agents: &wsrelated.Agents{LogSources: true}}},
			setup: func(db *dbmock.MockStore) {
				expectResources(db)
				expectAgents(db)
				db.EXPECT().GetWorkspaceAgentLogSourcesByAgentIDs(gomock.Any(), gomock.Any()).
					Return([]database.WorkspaceAgentLogSource{}, nil)
			},
		},
		{
			name: "All",
			cfg:  wsrelated.AllLatestBuild(),
			setup: func(db *dbmock.MockStore) {
				expectJobWithQueuePosition(db)
				expectTemplateVersion(db)
				expectResources(db)
				expectAgents(db)
				expectApps(db)
				db.EXPECT().GetWorkspaceResourceMetadataByResourceIDs(gomock.Any(), gomock.Any()).
					Return([]database.WorkspaceResourceMetadatum{}, nil)
				db.EXPECT().GetWorkspaceAppStatusesByAppIDs(gomock.Any(), gomock.Any()).
					Return([]database.WorkspaceAppStatus{}, nil)
				db.EXPECT().GetWorkspaceAgentScriptsByAgentIDs(gomock.Any(), gomock.Any()).
					Return([]database.GetWorkspaceAgentScriptsByAgentIDsRow{}, nil)
				db.EXPECT().GetWorkspaceAgentLogSourcesByAgentIDs(gomock.Any(), gomock.Any()).
					Return([]database.WorkspaceAgentLogSource{}, nil)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)
			ctrl := gomock.NewController(t)
			db := dbmock.NewMockStore(ctrl)
			tc.setup(db)

			api := &API{Options: &Options{Database: db}}
			_, err := api.workspaceBuildsData(ctx, []database.WorkspaceBuild{build}, tc.cfg)
			require.NoError(t, err)
		})
	}
}

// TestWorkspaceBuildsDataResourcesShortCircuit verifies that when resources are
// requested but none exist, no agent-level queries are issued even though the
// deeper nodes are selected.
func TestWorkspaceBuildsDataResourcesShortCircuit(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	build := database.WorkspaceBuild{ID: uuid.New(), JobID: uuid.New(), TemplateVersionID: uuid.New()}
	// No resources returned: the agent, app, and status queries must be skipped.
	db.EXPECT().GetWorkspaceResourcesByJobIDs(gomock.Any(), gomock.Any()).
		Return([]database.WorkspaceResource{}, nil)

	cfg := wsrelated.LatestBuild{Resources: &wsrelated.Resources{Agents: &wsrelated.Agents{Apps: &wsrelated.Apps{Statuses: true}}}}
	api := &API{Options: &Options{Database: db}}
	_, err := api.workspaceBuildsData(ctx, []database.WorkspaceBuild{build}, cfg)
	require.NoError(t, err)
}

// TestWorkspaceDataQueryGating asserts that workspaceData gates its
// workspace-level queries (templates, latest builds, and latest app statuses)
// on the selection.
func TestWorkspaceDataQueryGating(t *testing.T) {
	t.Parallel()

	workspace := database.Workspace{ID: uuid.New(), TemplateID: uuid.New()}

	cases := []struct {
		name  string
		cfg   wsrelated.Config
		setup func(*dbmock.MockStore)
	}{
		{
			name:  "None",
			cfg:   wsrelated.Config{},
			setup: func(*dbmock.MockStore) {},
		},
		{
			name: "Template",
			cfg:  wsrelated.Config{Template: true},
			setup: func(db *dbmock.MockStore) {
				db.EXPECT().GetTemplatesWithFilter(gomock.Any(), gomock.Any()).
					Return([]database.Template{}, nil)
			},
		},
		{
			name: "LatestBuild",
			cfg:  wsrelated.Config{LatestBuild: &wsrelated.LatestBuild{}},
			setup: func(db *dbmock.MockStore) {
				// No builds returned, so no build-subtree queries run.
				db.EXPECT().GetLatestWorkspaceBuildsByWorkspaceIDs(gomock.Any(), gomock.Any()).
					Return([]database.WorkspaceBuild{}, nil)
			},
		},
		{
			name: "AppStatuses",
			cfg: wsrelated.Config{LatestBuild: &wsrelated.LatestBuild{
				Resources: &wsrelated.Resources{Agents: &wsrelated.Agents{Apps: &wsrelated.Apps{Statuses: true}}},
			}},
			setup: func(db *dbmock.MockStore) {
				// The workspace-level latest app status query is gated on the
				// apps.statuses node. With no builds, only the workspace-level
				// status, build, and (empty) resource queries run.
				db.EXPECT().GetLatestWorkspaceAppStatusesByWorkspaceIDs(gomock.Any(), gomock.Any()).
					Return([]database.WorkspaceAppStatus{}, nil)
				db.EXPECT().GetLatestWorkspaceBuildsByWorkspaceIDs(gomock.Any(), gomock.Any()).
					Return([]database.WorkspaceBuild{}, nil)
				db.EXPECT().GetWorkspaceResourcesByJobIDs(gomock.Any(), gomock.Any()).
					Return([]database.WorkspaceResource{}, nil)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)
			ctrl := gomock.NewController(t)
			db := dbmock.NewMockStore(ctrl)
			tc.setup(db)

			api := &API{Options: &Options{Database: db}}
			_, err := api.workspaceData(ctx, []database.Workspace{workspace}, tc.cfg)
			require.NoError(t, err)
		})
	}
}

// TestSingleWorkspaceData asserts the strict template-authorization behavior
// layered on top of workspaceData: a requested-but-absent template is reported
// as errWorkspaceTemplateUnauthorized, while an omitted template or a real
// query failure are handled distinctly.
func TestSingleWorkspaceData(t *testing.T) {
	t.Parallel()

	workspace := database.Workspace{ID: uuid.New(), TemplateID: uuid.New()}

	t.Run("TemplateReadable", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		db := dbmock.NewMockStore(gomock.NewController(t))
		db.EXPECT().GetTemplatesWithFilter(gomock.Any(), gomock.Any()).
			Return([]database.Template{{ID: workspace.TemplateID}}, nil)

		api := &API{Options: &Options{Database: db}}
		data, err := api.singleWorkspaceData(ctx, workspace, wsrelated.Config{Template: true})
		require.NoError(t, err)
		require.Len(t, data.templates, 1)
	})

	t.Run("TemplateUnauthorized", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		db := dbmock.NewMockStore(gomock.NewController(t))
		// The actor cannot read the template, so the query omits it.
		db.EXPECT().GetTemplatesWithFilter(gomock.Any(), gomock.Any()).
			Return([]database.Template{}, nil)

		api := &API{Options: &Options{Database: db}}
		_, err := api.singleWorkspaceData(ctx, workspace, wsrelated.Config{Template: true})
		require.ErrorIs(t, err, errWorkspaceTemplateUnauthorized)
	})

	t.Run("TemplateNotRequested", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		// No template query is expected, and an absent template is not an error.
		db := dbmock.NewMockStore(gomock.NewController(t))

		api := &API{Options: &Options{Database: db}}
		_, err := api.singleWorkspaceData(ctx, workspace, wsrelated.Config{})
		require.NoError(t, err)
	})

	t.Run("QueryError", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		db := dbmock.NewMockStore(gomock.NewController(t))
		db.EXPECT().GetTemplatesWithFilter(gomock.Any(), gomock.Any()).
			Return(nil, xerrors.New("boom"))

		api := &API{Options: &Options{Database: db}}
		_, err := api.singleWorkspaceData(ctx, workspace, wsrelated.Config{Template: true})
		require.Error(t, err)
		// A real query failure must not be mistaken for an authorization denial.
		require.NotErrorIs(t, err, errWorkspaceTemplateUnauthorized)
	})
}

// wsrelatedDeprecatedFields lists deprecated fields that are never populated.
// They count as covered by their parent's wsrelated tag but cannot be non-zero.
var wsrelatedDeprecatedFields = map[string]bool{
	"WorkspaceAppStatus.Icon":               true,
	"WorkspaceAppStatus.NeedsUserAttention": true,
}

// wsrelatedField is a field reachable from codersdk.Workspace along with the
// include_related value that must populate it.
type wsrelatedField struct {
	// path is the chain of field names from the root codersdk.Workspace.
	path []string
	tag  string
}

// collectWSRelatedFields walks the codersdk struct t and returns every field
// that is returned as part of a workspace. In a struct where any field has a
// wsrelated tag, untagged fields are not returned and are skipped. A struct
// with no tags is covered entirely by the tag on the field that contains it.
func collectWSRelatedFields(t reflect.Type, path []string, parentTag string) []wsrelatedField {
	tagged := false
	for i := range t.NumField() {
		if _, ok := t.Field(i).Tag.Lookup("wsrelated"); ok {
			tagged = true
			break
		}
	}

	var fields []wsrelatedField
	for i := range t.NumField() {
		f := t.Field(i)
		if !f.IsExported() || wsrelatedDeprecatedFields[t.Name()+"."+f.Name] {
			continue
		}
		tag := parentTag
		if tagged {
			var ok bool
			tag, ok = f.Tag.Lookup("wsrelated")
			if !ok {
				continue
			}
		}
		fieldPath := append(append([]string{}, path...), f.Name)
		fields = append(fields, wsrelatedField{path: fieldPath, tag: tag})

		elem := f.Type
		for elem.Kind() == reflect.Pointer || elem.Kind() == reflect.Slice || elem.Kind() == reflect.Map {
			elem = elem.Elem()
		}
		if elem.Kind() == reflect.Struct && elem.PkgPath() == t.PkgPath() {
			fields = append(fields, collectWSRelatedFields(elem, fieldPath, tag)...)
		}
	}
	return fields
}

// resolveWSRelatedField follows path from v, descending into the first element
// of slices and any value of maps. It reports false if an intermediate value
// is nil or empty.
func resolveWSRelatedField(v reflect.Value, path []string) (reflect.Value, bool) {
	for _, name := range path {
		for {
			switch v.Kind() {
			case reflect.Pointer:
				if v.IsNil() {
					return reflect.Value{}, false
				}
				v = v.Elem()
				continue
			case reflect.Slice:
				if v.Len() == 0 {
					return reflect.Value{}, false
				}
				v = v.Index(0)
				continue
			case reflect.Map:
				if v.Len() == 0 {
					return reflect.Value{}, false
				}
				v = v.MapIndex(v.MapKeys()[0])
				continue
			}
			break
		}
		v = v.FieldByName(name)
	}
	return v, true
}

// isEmptyWSRelatedValue reports whether v carries no data: it is zero, a nil
// pointer or a pointer to a zero value, or an empty slice or map.
func isEmptyWSRelatedValue(v reflect.Value) bool {
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return true
		}
		v = v.Elem()
	}
	if v.Kind() == reflect.Slice || v.Kind() == reflect.Map {
		return v.Len() == 0
	}
	return v.IsZero()
}

// wsrelatedFixture holds non-zero database rows for a workspace with every
// kind of related data.
type wsrelatedFixture struct {
	workspace database.Workspace
	template  database.Template
	build     database.WorkspaceBuild
	job       database.GetProvisionerJobsByIDsWithQueuePositionRow
	daemon    database.GetEligibleProvisionerDaemonsByProvisionerJobIDsRow
	version   database.TemplateVersion
	resource  database.WorkspaceResource
	metadata  database.WorkspaceResourceMetadatum
	agent     database.WorkspaceAgent
	app       database.WorkspaceApp
	appStatus database.WorkspaceAppStatus
	script    database.GetWorkspaceAgentScriptsByAgentIDsRow
	logSource database.WorkspaceAgentLogSource
}

// newWSRelatedFixture builds the rows. Some SDK fields are mutually exclusive
// (an agent is either healthy or has an unhealthy reason, job input either
// decodes or reports an error). This fixture populates the healthy side;
// unhealthy derives the other.
func newWSRelatedFixture() wsrelatedFixture {
	now := dbtime.Now()
	nullTime := func(t time.Time) sql.NullTime { return sql.NullTime{Time: t, Valid: true} }
	nullString := func(s string) sql.NullString { return sql.NullString{String: s, Valid: true} }

	var f wsrelatedFixture
	ownerID := database.PrebuildsSystemUserID
	actorID := uuid.New()
	f.workspace = database.Workspace{
		ID:                  uuid.New(),
		CreatedAt:           now,
		UpdatedAt:           now,
		OwnerID:             ownerID,
		OrganizationID:      uuid.New(),
		TemplateID:          uuid.New(),
		Name:                "workspace",
		AutostartSchedule:   nullString("CRON_TZ=UTC 0 9 * * 1-5"),
		Ttl:                 sql.NullInt64{Int64: int64(time.Hour), Valid: true},
		LastUsedAt:          now,
		DormantAt:           nullTime(now),
		DeletingAt:          nullTime(now),
		AutomaticUpdates:    database.AutomaticUpdatesAlways,
		Favorite:            true,
		NextStartAt:         nullTime(now),
		UserACL:             database.WorkspaceACL{actorID.String(): {Permissions: db2sdk.WorkspaceRoleActions(codersdk.WorkspaceRoleAdmin)}},
		UserACLDisplayInfo:  database.WorkspaceACLDisplayInfo{actorID.String(): {Name: "actor", AvatarURL: "https://example.com/actor.png"}},
		OwnerAvatarUrl:      "https://example.com/owner.png",
		OwnerUsername:       "owner",
		OrganizationName:    "org",
		TemplateName:        "template",
		TemplateDisplayName: "Template",
		TemplateIcon:        "/icon/template.svg",
	}
	f.template = database.Template{
		ID:                           f.workspace.TemplateID,
		ActiveVersionID:              uuid.New(),
		AllowUserCancelWorkspaceJobs: true,
		AllowUserAutostop:            true,
		RequireActiveVersion:         true,
		UseClassicParameterFlow:      true,
		AllowWorkspaceRenames:        true,
	}
	f.version = database.TemplateVersion{ID: uuid.New(), Name: "version"}

	jobID := uuid.New()
	f.build = database.WorkspaceBuild{
		ID:                      uuid.New(),
		CreatedAt:               now,
		UpdatedAt:               now,
		WorkspaceID:             f.workspace.ID,
		TemplateVersionID:       f.version.ID,
		BuildNumber:             1,
		Transition:              database.WorkspaceTransitionStart,
		InitiatorID:             uuid.New(),
		JobID:                   jobID,
		Deadline:                now,
		Reason:                  database.BuildReasonInitiator,
		DailyCost:               1,
		MaxDeadline:             now,
		TemplateVersionPresetID: uuid.NullUUID{UUID: uuid.New(), Valid: true},
		HasExternalAgent:        sql.NullBool{Bool: true, Valid: true},
		InitiatorByUsername:     "initiator",
	}

	input := json.RawMessage(`{"template_version_id":"` + uuid.NewString() + `","workspace_build_id":"` + f.build.ID.String() + `"}`)
	f.job = database.GetProvisionerJobsByIDsWithQueuePositionRow{
		ID:        jobID,
		CreatedAt: now,
		ProvisionerJob: database.ProvisionerJob{
			ID:             jobID,
			CreatedAt:      now,
			UpdatedAt:      now,
			StartedAt:      nullTime(now),
			CanceledAt:     nullTime(now),
			CompletedAt:    nullTime(now),
			Error:          nullString("error"),
			OrganizationID: f.workspace.OrganizationID,
			InitiatorID:    f.build.InitiatorID,
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			Input:          input,
			WorkerID:       uuid.NullUUID{UUID: uuid.New(), Valid: true},
			FileID:         uuid.New(),
			Tags:           database.StringMap{"owner": ""},
			ErrorCode:      nullString("REQUIRED_TEMPLATE_VARIABLES"),
			JobStatus:      database.ProvisionerJobStatusPending,
			LogsOverflowed: true,
		},
		QueuePosition: 1,
		QueueSize:     1,
	}
	f.daemon = database.GetEligibleProvisionerDaemonsByProvisionerJobIDsRow{
		JobID:             jobID,
		ProvisionerDaemon: database.ProvisionerDaemon{ID: uuid.New(), LastSeenAt: nullTime(now)},
	}

	f.resource = database.WorkspaceResource{
		ID:         uuid.New(),
		CreatedAt:  now,
		JobID:      jobID,
		Transition: database.WorkspaceTransitionStart,
		Type:       "docker_container",
		Name:       "dev",
		Hide:       true,
		Icon:       "/icon/docker.svg",
		DailyCost:  1,
	}
	f.metadata = database.WorkspaceResourceMetadatum{
		WorkspaceResourceID: f.resource.ID,
		Key:                 "key",
		Value:               nullString("value"),
		Sensitive:           true,
	}

	// The agent is connected and, to populate ParentID, a sub-agent.
	f.agent = database.WorkspaceAgent{
		ID:                       uuid.New(),
		CreatedAt:                now,
		UpdatedAt:                now,
		Name:                     "main",
		FirstConnectedAt:         nullTime(now.Add(-2 * time.Hour)),
		LastConnectedAt:          nullTime(now),
		DisconnectedAt:           nullTime(now.Add(-time.Hour)),
		ResourceID:               f.resource.ID,
		AuthInstanceID:           nullString("instance"),
		Architecture:             "amd64",
		EnvironmentVariables:     pqtype.NullRawMessage{RawMessage: json.RawMessage(`{"FOO":"bar"}`), Valid: true},
		OperatingSystem:          "linux",
		Directory:                "~",
		Version:                  "v2.0.0",
		ConnectionTimeoutSeconds: 120,
		TroubleshootingURL:       "https://example.com/troubleshoot",
		LifecycleState:           database.WorkspaceAgentLifecycleStateReady,
		ExpandedDirectory:        "/home/coder",
		LogsLength:               1,
		LogsOverflowed:           true,
		StartedAt:                nullTime(now),
		ReadyAt:                  nullTime(now),
		Subsystems:               []database.WorkspaceAgentSubsystem{database.WorkspaceAgentSubsystemEnvbox},
		DisplayApps:              []database.DisplayApp{database.DisplayAppVscode},
		APIVersion:               "1.0",
		ParentID:                 uuid.NullUUID{UUID: uuid.New(), Valid: true},
	}

	f.app = database.WorkspaceApp{
		ID:                   uuid.New(),
		CreatedAt:            now,
		AgentID:              f.agent.ID,
		DisplayName:          "App",
		Icon:                 "/icon/app.svg",
		Command:              nullString("app"),
		Url:                  nullString("http://localhost:8080"),
		HealthcheckUrl:       "http://localhost:8080/healthz",
		HealthcheckInterval:  5,
		HealthcheckThreshold: 3,
		Health:               database.WorkspaceAppHealthHealthy,
		Subdomain:            true,
		SharingLevel:         database.AppSharingLevelOwner,
		Slug:                 "app",
		External:             true,
		Hidden:               true,
		OpenIn:               database.WorkspaceAppOpenInSlimWindow,
		DisplayGroup:         nullString("group"),
		Tooltip:              "tooltip",
	}
	f.appStatus = database.WorkspaceAppStatus{
		ID:          uuid.New(),
		CreatedAt:   now,
		AgentID:     f.agent.ID,
		AppID:       f.app.ID,
		WorkspaceID: f.workspace.ID,
		State:       database.WorkspaceAppStatusStateWorking,
		Message:     "message",
		Uri:         nullString("https://example.com/pr/1"),
	}

	f.logSource = database.WorkspaceAgentLogSource{
		WorkspaceAgentID: f.agent.ID,
		ID:               uuid.New(),
		CreatedAt:        now,
		DisplayName:      "Startup",
		Icon:             "/icon/startup.svg",
	}
	f.script = database.GetWorkspaceAgentScriptsByAgentIDsRow{
		WorkspaceAgentID: f.agent.ID,
		LogSourceID:      f.logSource.ID,
		LogPath:          "/tmp/startup.log",
		CreatedAt:        now,
		Script:           "echo hello",
		Cron:             "@daily",
		StartBlocksLogin: true,
		RunOnStart:       true,
		RunOnStop:        true,
		TimeoutSeconds:   60,
		DisplayName:      "Startup",
		ID:               uuid.New(),
		ResourceAddress:  "coder_script.startup",
		Dependencies:     json.RawMessage(`[{"prerequisite_resource_address":"coder_script.clone","requirement":"success"}]`),
		ExitCode:         sql.NullInt32{Int32: 1, Valid: true},
		Status:           database.NullWorkspaceAgentScriptTimingStatus{WorkspaceAgentScriptTimingStatus: database.WorkspaceAgentScriptTimingStatusOk, Valid: true},
	}
	return f
}

// unhealthy returns a copy of f with the agent disconnected and job input
// that fails to decode. The agent is not a sub-agent, since sub-agents are
// excluded from the workspace's failing agents.
func (f wsrelatedFixture) unhealthy() wsrelatedFixture {
	f.job.ProvisionerJob.Input = json.RawMessage(`not json`)
	f.agent.ParentID = uuid.NullUUID{}
	f.agent.DisconnectedAt = sql.NullTime{Time: f.agent.LastConnectedAt.Time.Add(time.Second), Valid: true}
	return f
}

// newAPI returns an API backed by a mock store that answers every related-data
// query with the fixture rows.
func (f wsrelatedFixture) newAPI(t *testing.T) *API {
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	db.EXPECT().GetTemplatesWithFilter(gomock.Any(), gomock.Any()).
		Return([]database.Template{f.template}, nil).AnyTimes()
	db.EXPECT().GetLatestWorkspaceBuildsByWorkspaceIDs(gomock.Any(), gomock.Any()).
		Return([]database.WorkspaceBuild{f.build}, nil).AnyTimes()
	db.EXPECT().GetLatestWorkspaceAppStatusesByWorkspaceIDs(gomock.Any(), gomock.Any()).
		Return([]database.WorkspaceAppStatus{f.appStatus}, nil).AnyTimes()
	db.EXPECT().GetProvisionerJobsByIDs(gomock.Any(), gomock.Any()).
		Return([]database.ProvisionerJob{f.job.ProvisionerJob}, nil).AnyTimes()
	db.EXPECT().GetProvisionerJobsByIDsWithQueuePosition(gomock.Any(), gomock.Any()).
		Return([]database.GetProvisionerJobsByIDsWithQueuePositionRow{f.job}, nil).AnyTimes()
	db.EXPECT().GetEligibleProvisionerDaemonsByProvisionerJobIDs(gomock.Any(), gomock.Any()).
		Return([]database.GetEligibleProvisionerDaemonsByProvisionerJobIDsRow{f.daemon}, nil).AnyTimes()
	db.EXPECT().GetTemplateVersionsByIDs(gomock.Any(), gomock.Any()).
		Return([]database.TemplateVersion{f.version}, nil).AnyTimes()
	db.EXPECT().GetWorkspaceResourcesByJobIDs(gomock.Any(), gomock.Any()).
		Return([]database.WorkspaceResource{f.resource}, nil).AnyTimes()
	db.EXPECT().GetWorkspaceResourceMetadataByResourceIDs(gomock.Any(), gomock.Any()).
		Return([]database.WorkspaceResourceMetadatum{f.metadata}, nil).AnyTimes()
	db.EXPECT().GetWorkspaceAgentsByResourceIDs(gomock.Any(), gomock.Any()).
		Return([]database.WorkspaceAgent{f.agent}, nil).AnyTimes()
	db.EXPECT().GetWorkspaceAppsByAgentIDs(gomock.Any(), gomock.Any()).
		Return([]database.WorkspaceApp{f.app}, nil).AnyTimes()
	db.EXPECT().GetWorkspaceAppStatusesByAppIDs(gomock.Any(), gomock.Any()).
		Return([]database.WorkspaceAppStatus{f.appStatus}, nil).AnyTimes()
	db.EXPECT().GetWorkspaceAgentScriptsByAgentIDs(gomock.Any(), gomock.Any()).
		Return([]database.GetWorkspaceAgentScriptsByAgentIDsRow{f.script}, nil).AnyTimes()
	db.EXPECT().GetWorkspaceAgentLogSourcesByAgentIDs(gomock.Any(), gomock.Any()).
		Return([]database.WorkspaceAgentLogSource{f.logSource}, nil).AnyTimes()

	coordinator := tailnettest.NewMockCoordinator(ctrl)
	coordinator.EXPECT().Node(gomock.Any()).Return(&tailnet.Node{
		PreferredDERP: 1,
		DERPLatency:   map[string]float64{"1-derp": 0.05},
	}).AnyTimes()

	api := &API{Options: &Options{
		Database:                       db,
		Logger:                         slogtest.Make(t, nil),
		AgentInactiveDisconnectTimeout: time.Minute,
		DeploymentValues:               &codersdk.DeploymentValues{},
		BaseDERPMap: &tailcfg.DERPMap{Regions: map[int]*tailcfg.DERPRegion{
			1: {RegionID: 1, RegionName: "region"},
		}},
	}}
	var coord tailnet.Coordinator = coordinator
	api.TailnetCoordinator.Store(&coord)
	return api
}

// TestWorkspaceWSRelatedTags asserts that each field reachable from
// codersdk.Workspace is populated when include_related is set to the field's
// wsrelated tag. It loads and converts the workspace the same way the get
// workspace handlers do, against a mock store that returns non-zero rows.
func TestWorkspaceWSRelatedTags(t *testing.T) {
	t.Parallel()

	fields := collectWSRelatedFields(reflect.TypeFor[codersdk.Workspace](), nil, "")
	require.NotEmpty(t, fields)

	// This test works by checking that each field gets a non-zero value when its `wsrelated` tag is used as the
	// wsreleated.Config. However, some fields are mutually exclusive, so cannot all be non-zero from a single
	// fixture. These fields are related to healthy and unhealthy workspaces. So this test includes both a healthy
	// and an unhealthy workspace as fixtures, and considers the field a pass if the field is non-zero for either.
	healthy := newWSRelatedFixture()
	fixtures := []wsrelatedFixture{healthy, healthy.unhealthy()}
	apis := make([]*API, len(fixtures))
	for i, f := range fixtures {
		apis[i] = f.newAPI(t)
	}

	// Convert one workspace per fixture (healthy, unhealthy) for each distinct `wsrelated` tag.
	results := map[string][]codersdk.Workspace{}
	for _, field := range fields {
		if _, ok := results[field.tag]; ok {
			continue
		}
		cfg, err := wsrelated.Parse(field.tag)
		require.NoError(t, err, "parse wsrelated tag %q on %s", field.tag, strings.Join(field.path, "."))

		for i, f := range fixtures {
			ctx := testutil.Context(t, testutil.WaitShort)
			data, err := apis[i].singleWorkspaceData(ctx, f.workspace, cfg)
			require.NoError(t, err)

			var build codersdk.WorkspaceBuild
			if len(data.builds) > 0 {
				build = data.builds[0]
			}
			var template database.Template
			if len(data.templates) > 0 {
				template = data.templates[0]
			}
			var appStatus codersdk.WorkspaceAppStatus
			if len(data.appStatuses) > 0 {
				appStatus = data.appStatuses[0]
			}
			ws, err := convertWorkspace(ctx, apis[i].Logger, f.workspace.OwnerID, f.workspace, build, template, apis[i].AllowWorkspaceRenames, appStatus)
			require.NoError(t, err)
			results[field.tag] = append(results[field.tag], ws)
		}
	}

	for _, field := range fields {
		name := strings.Join(field.path, ".")
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// Some fields are mutually exclusive, so the field must be set in
			// at least one fixture.
			for _, ws := range results[field.tag] {
				if v, ok := resolveWSRelatedField(reflect.ValueOf(ws), field.path); ok && !isEmptyWSRelatedValue(v) {
					return
				}
			}
			t.Errorf("%s is zero with include_related=%q", name, field.tag)
		})
	}
}

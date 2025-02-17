package telemetry_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmem"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/idpsync"
	"github.com/coder/coder/v2/coderd/runtimeconfig"
	"github.com/coder/coder/v2/coderd/telemetry"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m, testutil.GoleakOptions...)
}

func TestTelemetry(t *testing.T) {
	t.Parallel()
	t.Run("Snapshot", func(t *testing.T) {
		t.Parallel()

		var err error

		db := dbmem.New()

		ctx := testutil.Context(t, testutil.WaitMedium)

		org, err := db.GetDefaultOrganization(ctx)
		require.NoError(t, err)

		_, _ = dbgen.APIKey(t, db, database.APIKey{})
		_ = dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			Provisioner:    database.ProvisionerTypeTerraform,
			StorageMethod:  database.ProvisionerStorageMethodFile,
			Type:           database.ProvisionerJobTypeTemplateVersionDryRun,
			OrganizationID: org.ID,
		})
		_ = dbgen.Template(t, db, database.Template{
			Provisioner:    database.ProvisionerTypeTerraform,
			OrganizationID: org.ID,
		})
		sourceExampleID := uuid.NewString()
		_ = dbgen.TemplateVersion(t, db, database.TemplateVersion{
			SourceExampleID: sql.NullString{String: sourceExampleID, Valid: true},
			OrganizationID:  org.ID,
		})
		_ = dbgen.TemplateVersion(t, db, database.TemplateVersion{
			OrganizationID: org.ID,
		})
		user := dbgen.User(t, db, database.User{})
		_ = dbgen.Workspace(t, db, database.WorkspaceTable{
			OrganizationID: org.ID,
		})
		_ = dbgen.WorkspaceApp(t, db, database.WorkspaceApp{
			SharingLevel: database.AppSharingLevelOwner,
			Health:       database.WorkspaceAppHealthDisabled,
			OpenIn:       database.WorkspaceAppOpenInSlimWindow,
		})
		_ = dbgen.TelemetryItem(t, db, database.TelemetryItem{
			Key:   string(telemetry.TelemetryItemKeyHTMLFirstServedAt),
			Value: time.Now().Format(time.RFC3339),
		})
		group := dbgen.Group(t, db, database.Group{})
		_ = dbgen.GroupMember(t, db, database.GroupMemberTable{UserID: user.ID, GroupID: group.ID})
		wsagent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{})
		// Update the workspace agent to have a valid subsystem.
		err = db.UpdateWorkspaceAgentStartupByID(ctx, database.UpdateWorkspaceAgentStartupByIDParams{
			ID:                wsagent.ID,
			Version:           wsagent.Version,
			ExpandedDirectory: wsagent.ExpandedDirectory,
			Subsystems: []database.WorkspaceAgentSubsystem{
				database.WorkspaceAgentSubsystemEnvbox,
				database.WorkspaceAgentSubsystemExectrace,
			},
		})
		require.NoError(t, err)

		_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			Transition: database.WorkspaceTransitionStart,
			Reason:     database.BuildReasonAutostart,
		})
		_ = dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
			Transition: database.WorkspaceTransitionStart,
		})
		_ = dbgen.WorkspaceAgentStat(t, db, database.WorkspaceAgentStat{})
		_, err = db.InsertLicense(ctx, database.InsertLicenseParams{
			UploadedAt: dbtime.Now(),
			JWT:        "",
			Exp:        dbtime.Now().Add(time.Hour),
			UUID:       uuid.New(),
		})
		assert.NoError(t, err)
		_, _ = dbgen.WorkspaceProxy(t, db, database.WorkspaceProxy{})

		_ = dbgen.WorkspaceModule(t, db, database.WorkspaceModule{})

		_, snapshot := collectSnapshot(t, db, nil)
		require.Len(t, snapshot.ProvisionerJobs, 1)
		require.Len(t, snapshot.Licenses, 1)
		require.Len(t, snapshot.Templates, 1)
		require.Len(t, snapshot.TemplateVersions, 2)
		require.Len(t, snapshot.Users, 1)
		require.Len(t, snapshot.Groups, 2)
		// 1 member in the everyone group + 1 member in the custom group
		require.Len(t, snapshot.GroupMembers, 2)
		require.Len(t, snapshot.Workspaces, 1)
		require.Len(t, snapshot.WorkspaceApps, 1)
		require.Len(t, snapshot.WorkspaceAgents, 1)
		require.Len(t, snapshot.WorkspaceBuilds, 1)
		require.Len(t, snapshot.WorkspaceResources, 1)
		require.Len(t, snapshot.WorkspaceAgentStats, 1)
		require.Len(t, snapshot.WorkspaceProxies, 1)
		require.Len(t, snapshot.WorkspaceModules, 1)
		require.Len(t, snapshot.Organizations, 1)
		// We create one item manually above. The other is TelemetryEnabled, created by the snapshotter.
		require.Len(t, snapshot.TelemetryItems, 2)
		wsa := snapshot.WorkspaceAgents[0]
		require.Len(t, wsa.Subsystems, 2)
		require.Equal(t, string(database.WorkspaceAgentSubsystemEnvbox), wsa.Subsystems[0])
		require.Equal(t, string(database.WorkspaceAgentSubsystemExectrace), wsa.Subsystems[1])

		tvs := snapshot.TemplateVersions
		sort.Slice(tvs, func(i, j int) bool {
			// Sort by SourceExampleID presence (non-nil comes before nil)
			if (tvs[i].SourceExampleID != nil) != (tvs[j].SourceExampleID != nil) {
				return tvs[i].SourceExampleID != nil
			}
			return false
		})
		require.Equal(t, tvs[0].SourceExampleID, &sourceExampleID)
		require.Nil(t, tvs[1].SourceExampleID)

		for _, entity := range snapshot.Workspaces {
			require.Equal(t, entity.OrganizationID, org.ID)
		}
		for _, entity := range snapshot.ProvisionerJobs {
			require.Equal(t, entity.OrganizationID, org.ID)
		}
		for _, entity := range snapshot.TemplateVersions {
			require.Equal(t, entity.OrganizationID, org.ID)
		}
		for _, entity := range snapshot.Templates {
			require.Equal(t, entity.OrganizationID, org.ID)
		}
	})
	t.Run("HashedEmail", func(t *testing.T) {
		t.Parallel()
		db := dbmem.New()
		_ = dbgen.User(t, db, database.User{
			Email: "kyle@coder.com",
		})
		_, snapshot := collectSnapshot(t, db, nil)
		require.Len(t, snapshot.Users, 1)
		require.Equal(t, snapshot.Users[0].EmailHashed, "bb44bf07cf9a2db0554bba63a03d822c927deae77df101874496df5a6a3e896d@coder.com")
	})
	t.Run("HashedModule", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		pj := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{})
		_ = dbgen.WorkspaceModule(t, db, database.WorkspaceModule{
			JobID:   pj.ID,
			Source:  "registry.coder.com/terraform/aws",
			Version: "1.0.0",
		})
		_ = dbgen.WorkspaceModule(t, db, database.WorkspaceModule{
			JobID:   pj.ID,
			Source:  "https://internal-url.com/some-module",
			Version: "1.0.0",
		})
		_, snapshot := collectSnapshot(t, db, nil)
		require.Len(t, snapshot.WorkspaceModules, 2)
		modules := snapshot.WorkspaceModules
		sort.Slice(modules, func(i, j int) bool {
			return modules[i].Source < modules[j].Source
		})
		require.Equal(t, modules[0].Source, "ed662ec0396db67e77119f14afcb9253574cc925b04a51d4374bcb1eae299f5d")
		require.Equal(t, modules[0].Version, "92521fc3cbd964bdc9f584a991b89fddaa5754ed1cc96d6d42445338669c1305")
		require.Equal(t, modules[0].SourceType, telemetry.ModuleSourceTypeHTTP)
		require.Equal(t, modules[1].Source, "registry.coder.com/terraform/aws")
		require.Equal(t, modules[1].Version, "1.0.0")
		require.Equal(t, modules[1].SourceType, telemetry.ModuleSourceTypeCoderRegistry)
	})
	t.Run("ModuleSourceType", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			source string
			want   telemetry.ModuleSourceType
		}{
			// Local relative paths
			{source: "./modules/terraform-aws-vpc", want: telemetry.ModuleSourceTypeLocal},
			{source: "../shared/modules/vpc", want: telemetry.ModuleSourceTypeLocal},
			{source: "  ./my-module  ", want: telemetry.ModuleSourceTypeLocal}, // with whitespace

			// Local absolute paths
			{source: "/opt/terraform/modules/vpc", want: telemetry.ModuleSourceTypeLocalAbs},
			{source: "/Users/dev/modules/app", want: telemetry.ModuleSourceTypeLocalAbs},
			{source: "/etc/terraform/modules/network", want: telemetry.ModuleSourceTypeLocalAbs},

			// Public registry
			{source: "hashicorp/consul/aws", want: telemetry.ModuleSourceTypePublicRegistry},
			{source: "registry.terraform.io/hashicorp/aws", want: telemetry.ModuleSourceTypePublicRegistry},
			{source: "terraform-aws-modules/vpc/aws", want: telemetry.ModuleSourceTypePublicRegistry},
			{source: "hashicorp/consul/aws//modules/consul-cluster", want: telemetry.ModuleSourceTypePublicRegistry},
			{source: "hashicorp/co-nsul/aw_s//modules/consul-cluster", want: telemetry.ModuleSourceTypePublicRegistry},

			// Private registry
			{source: "app.terraform.io/company/vpc/aws", want: telemetry.ModuleSourceTypePrivateRegistry},
			{source: "localterraform.com/org/module", want: telemetry.ModuleSourceTypePrivateRegistry},
			{source: "APP.TERRAFORM.IO/test/module", want: telemetry.ModuleSourceTypePrivateRegistry}, // case insensitive

			// Coder registry
			{source: "registry.coder.com/terraform/aws", want: telemetry.ModuleSourceTypeCoderRegistry},
			{source: "registry.coder.com/modules/base", want: telemetry.ModuleSourceTypeCoderRegistry},
			{source: "REGISTRY.CODER.COM/test/module", want: telemetry.ModuleSourceTypeCoderRegistry}, // case insensitive

			// GitHub
			{source: "github.com/hashicorp/terraform-aws-vpc", want: telemetry.ModuleSourceTypeGitHub},
			{source: "git::https://github.com/org/repo.git", want: telemetry.ModuleSourceTypeGitHub},
			{source: "git::https://github.com/org/repo//modules/vpc", want: telemetry.ModuleSourceTypeGitHub},

			// Bitbucket
			{source: "bitbucket.org/hashicorp/terraform-aws-vpc", want: telemetry.ModuleSourceTypeBitbucket},
			{source: "git::https://bitbucket.org/org/repo.git", want: telemetry.ModuleSourceTypeBitbucket},
			{source: "https://bitbucket.org/org/repo//modules/vpc", want: telemetry.ModuleSourceTypeBitbucket},

			// Generic Git
			{source: "git::ssh://git.internal.com/repo.git", want: telemetry.ModuleSourceTypeGit},
			{source: "git@gitlab.com:org/repo.git", want: telemetry.ModuleSourceTypeGit},
			{source: "git::https://git.internal.com/repo.git?ref=v1.0.0", want: telemetry.ModuleSourceTypeGit},

			// Mercurial
			{source: "hg::https://example.com/vpc.hg", want: telemetry.ModuleSourceTypeMercurial},
			{source: "hg::http://example.com/vpc.hg", want: telemetry.ModuleSourceTypeMercurial},
			{source: "hg::ssh://example.com/vpc.hg", want: telemetry.ModuleSourceTypeMercurial},

			// HTTP
			{source: "https://example.com/vpc-module.zip", want: telemetry.ModuleSourceTypeHTTP},
			{source: "http://example.com/modules/vpc", want: telemetry.ModuleSourceTypeHTTP},
			{source: "https://internal.network/terraform/modules", want: telemetry.ModuleSourceTypeHTTP},

			// S3
			{source: "s3::https://s3-eu-west-1.amazonaws.com/bucket/vpc", want: telemetry.ModuleSourceTypeS3},
			{source: "s3::https://bucket.s3.amazonaws.com/vpc", want: telemetry.ModuleSourceTypeS3},
			{source: "s3::http://bucket.s3.amazonaws.com/vpc?version=1", want: telemetry.ModuleSourceTypeS3},

			// GCS
			{source: "gcs::https://www.googleapis.com/storage/v1/bucket/vpc", want: telemetry.ModuleSourceTypeGCS},
			{source: "gcs::https://storage.googleapis.com/bucket/vpc", want: telemetry.ModuleSourceTypeGCS},
			{source: "gcs::https://bucket.storage.googleapis.com/vpc", want: telemetry.ModuleSourceTypeGCS},

			// Unknown
			{source: "custom://example.com/vpc", want: telemetry.ModuleSourceTypeUnknown},
			{source: "something-random", want: telemetry.ModuleSourceTypeUnknown},
			{source: "", want: telemetry.ModuleSourceTypeUnknown},
		}
		for _, c := range cases {
			require.Equal(t, c.want, telemetry.GetModuleSourceType(c.source))
		}
	})
	t.Run("IDPOrgSync", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		db, _ := dbtestutil.NewDB(t)

		// 1. No org sync settings
		deployment, _ := collectSnapshot(t, db, nil)
		require.False(t, *deployment.IDPOrgSync)

		// 2. Org sync settings set in server flags
		deployment, _ = collectSnapshot(t, db, func(opts telemetry.Options) telemetry.Options {
			opts.DeploymentConfig = &codersdk.DeploymentValues{
				OIDC: codersdk.OIDCConfig{
					OrganizationField: "organizations",
				},
			}
			return opts
		})
		require.True(t, *deployment.IDPOrgSync)

		// 3. Org sync settings set in runtime config
		org, err := db.GetDefaultOrganization(ctx)
		require.NoError(t, err)
		sync := idpsync.NewAGPLSync(testutil.Logger(t), runtimeconfig.NewManager(), idpsync.DeploymentSyncSettings{})
		err = sync.UpdateOrganizationSyncSettings(ctx, db, idpsync.OrganizationSyncSettings{
			Field: "organizations",
			Mapping: map[string][]uuid.UUID{
				"first": {org.ID},
			},
			AssignDefault: true,
		})
		require.NoError(t, err)
		deployment, _ = collectSnapshot(t, db, nil)
		require.True(t, *deployment.IDPOrgSync)
	})
}

// nolint:paralleltest
func TestTelemetryInstallSource(t *testing.T) {
	t.Setenv("CODER_TELEMETRY_INSTALL_SOURCE", "aws_marketplace")
	db := dbmem.New()
	deployment, _ := collectSnapshot(t, db, nil)
	require.Equal(t, "aws_marketplace", deployment.InstallSource)
}

func TestTelemetryItem(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitMedium)
	db, _ := dbtestutil.NewDB(t)
	key := testutil.GetRandomName(t)
	value := time.Now().Format(time.RFC3339)

	err := db.InsertTelemetryItemIfNotExists(ctx, database.InsertTelemetryItemIfNotExistsParams{
		Key:   key,
		Value: value,
	})
	require.NoError(t, err)

	item, err := db.GetTelemetryItem(ctx, key)
	require.NoError(t, err)
	require.Equal(t, item.Key, key)
	require.Equal(t, item.Value, value)

	// Inserting a new value should not update the existing value
	err = db.InsertTelemetryItemIfNotExists(ctx, database.InsertTelemetryItemIfNotExistsParams{
		Key:   key,
		Value: "new_value",
	})
	require.NoError(t, err)

	item, err = db.GetTelemetryItem(ctx, key)
	require.NoError(t, err)
	require.Equal(t, item.Value, value)

	// Upserting a new value should update the existing value
	err = db.UpsertTelemetryItem(ctx, database.UpsertTelemetryItemParams{
		Key:   key,
		Value: "new_value",
	})
	require.NoError(t, err)

	item, err = db.GetTelemetryItem(ctx, key)
	require.NoError(t, err)
	require.Equal(t, item.Value, "new_value")
}

func TestShouldReportTelemetryDisabled(t *testing.T) {
	t.Parallel()
	// Description                            | telemetryEnabled (db) | telemetryEnabled (is) | Report Telemetry Disabled |
	//----------------------------------------|-----------------------|-----------------------|---------------------------|
	// New deployment                         | <null>                | true                  | No                        |
	// New deployment with telemetry disabled | <null>                | false                 | No                        |
	// Telemetry was enabled, and still is    | true                  | true                  | No                        |
	// Telemetry was enabled but now disabled | true                  | false                 | Yes                       |
	// Telemetry was disabled, now is enabled | false                 | true                  | No                        |
	// Telemetry was disabled, still disabled | false                 | false                 | No                        |
	boolTrue := true
	boolFalse := false
	require.False(t, telemetry.ShouldReportTelemetryDisabled(nil, true))
	require.False(t, telemetry.ShouldReportTelemetryDisabled(nil, false))
	require.False(t, telemetry.ShouldReportTelemetryDisabled(&boolTrue, true))
	require.True(t, telemetry.ShouldReportTelemetryDisabled(&boolTrue, false))
	require.False(t, telemetry.ShouldReportTelemetryDisabled(&boolFalse, true))
	require.False(t, telemetry.ShouldReportTelemetryDisabled(&boolFalse, false))
}

func TestRecordTelemetryStatus(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		name                     string
		recordedTelemetryEnabled string
		telemetryEnabled         bool
		shouldReport             bool
	}{
		{name: "New deployment", recordedTelemetryEnabled: "nil", telemetryEnabled: true, shouldReport: false},
		{name: "Telemetry disabled", recordedTelemetryEnabled: "nil", telemetryEnabled: false, shouldReport: false},
		{name: "Telemetry was enabled and still is", recordedTelemetryEnabled: "true", telemetryEnabled: true, shouldReport: false},
		{name: "Telemetry was enabled but now disabled", recordedTelemetryEnabled: "true", telemetryEnabled: false, shouldReport: true},
		{name: "Telemetry was disabled now is enabled", recordedTelemetryEnabled: "false", telemetryEnabled: true, shouldReport: false},
		{name: "Telemetry was disabled still disabled", recordedTelemetryEnabled: "false", telemetryEnabled: false, shouldReport: false},
		{name: "Telemetry was disabled still disabled, invalid value", recordedTelemetryEnabled: "invalid", telemetryEnabled: false, shouldReport: false},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			db, _ := dbtestutil.NewDB(t)
			ctx := testutil.Context(t, testutil.WaitMedium)
			logger := testutil.Logger(t)
			if testCase.recordedTelemetryEnabled != "nil" {
				db.UpsertTelemetryItem(ctx, database.UpsertTelemetryItemParams{
					Key:   string(telemetry.TelemetryItemKeyTelemetryEnabled),
					Value: testCase.recordedTelemetryEnabled,
				})
			}
			snapshot1, err := telemetry.RecordTelemetryStatus(ctx, logger, db, testCase.telemetryEnabled)
			require.NoError(t, err)

			if testCase.shouldReport {
				require.NotNil(t, snapshot1)
				require.Equal(t, snapshot1.TelemetryItems[0].Key, string(telemetry.TelemetryItemKeyTelemetryEnabled))
				require.Equal(t, snapshot1.TelemetryItems[0].Value, "false")
			} else {
				require.Nil(t, snapshot1)
			}

			for i := 0; i < 3; i++ {
				// Whatever happens, subsequent calls should not report if telemetryEnabled didn't change
				snapshot2, err := telemetry.RecordTelemetryStatus(ctx, logger, db, testCase.telemetryEnabled)
				require.NoError(t, err)
				require.Nil(t, snapshot2)
			}
		})
	}
}

func mockTelemetryServer(t *testing.T) (*url.URL, chan *telemetry.Deployment, chan *telemetry.Snapshot) {
	t.Helper()
	deployment := make(chan *telemetry.Deployment, 64)
	snapshot := make(chan *telemetry.Snapshot, 64)
	r := chi.NewRouter()
	r.Post("/deployment", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, buildinfo.Version(), r.Header.Get(telemetry.VersionHeader))
		dd := &telemetry.Deployment{}
		err := json.NewDecoder(r.Body).Decode(dd)
		require.NoError(t, err)
		deployment <- dd
		// Ensure the header is sent only after deployment is sent
		w.WriteHeader(http.StatusAccepted)
	})
	r.Post("/snapshot", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, buildinfo.Version(), r.Header.Get(telemetry.VersionHeader))
		ss := &telemetry.Snapshot{}
		err := json.NewDecoder(r.Body).Decode(ss)
		require.NoError(t, err)
		snapshot <- ss
		// Ensure the header is sent only after snapshot is sent
		w.WriteHeader(http.StatusAccepted)
	})
	server := httptest.NewServer(r)
	t.Cleanup(server.Close)
	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)

	return serverURL, deployment, snapshot
}

func collectSnapshot(t *testing.T, db database.Store, addOptionsFn func(opts telemetry.Options) telemetry.Options) (*telemetry.Deployment, *telemetry.Snapshot) {
	t.Helper()

	serverURL, deployment, snapshot := mockTelemetryServer(t)

	options := telemetry.Options{
		Database:     db,
		Logger:       testutil.Logger(t),
		URL:          serverURL,
		DeploymentID: uuid.NewString(),
	}
	if addOptionsFn != nil {
		options = addOptionsFn(options)
	}

	reporter, err := telemetry.New(options)
	require.NoError(t, err)
	t.Cleanup(reporter.Close)
	return <-deployment, <-snapshot
}

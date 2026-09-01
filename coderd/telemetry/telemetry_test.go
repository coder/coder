package telemetry_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/coderd/boundaryusage"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/idpsync"
	"github.com/coder/coder/v2/coderd/runtimeconfig"
	"github.com/coder/coder/v2/coderd/telemetry"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m, testutil.GoleakOptions...)
}

func TestConvertTemplateAgentsAllowed(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name    string
		allowed bool
	}{
		{name: "Allowed", allowed: true},
		{name: "Disallowed", allowed: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := telemetry.ConvertTemplate(database.Template{AgentsAllowed: tt.allowed})
			require.Equal(t, tt.allowed, got.AgentsAllowed)
		})
	}
}

func TestTelemetry(t *testing.T) {
	t.Parallel()
	t.Run("Snapshot", func(t *testing.T) {
		t.Parallel()

		var err error

		db, _ := dbtestutil.NewDB(t)

		ctx := testutil.Context(t, testutil.WaitMedium)
		now := dbtime.Now()

		org, err := db.GetDefaultOrganization(ctx)
		require.NoError(t, err)

		user := dbgen.User(t, db, database.User{})
		_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
			UserID:         user.ID,
			OrganizationID: org.ID,
		})
		require.NoError(t, err)
		_, _ = dbgen.APIKey(t, db, database.APIKey{
			UserID: user.ID,
		})
		job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			Provisioner:    database.ProvisionerTypeTerraform,
			StorageMethod:  database.ProvisionerStorageMethodFile,
			Type:           database.ProvisionerJobTypeTemplateVersionDryRun,
			OrganizationID: org.ID,
		})
		tpl := dbgen.Template(t, db, database.Template{
			Provisioner:    database.ProvisionerTypeTerraform,
			OrganizationID: org.ID,
			CreatedBy:      user.ID,
		})
		sourceExampleID := uuid.NewString()
		tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{
			SourceExampleID: sql.NullString{String: sourceExampleID, Valid: true},
			OrganizationID:  org.ID,
			TemplateID:      uuid.NullUUID{UUID: tpl.ID, Valid: true},
			CreatedBy:       user.ID,
			JobID:           job.ID,
		})
		_ = dbgen.TemplateVersion(t, db, database.TemplateVersion{
			OrganizationID: org.ID,
			TemplateID:     uuid.NullUUID{UUID: tpl.ID, Valid: true},
			CreatedBy:      user.ID,
			JobID:          job.ID,
		})
		ws := dbgen.Workspace(t, db, database.WorkspaceTable{
			OwnerID:        user.ID,
			OrganizationID: org.ID,
			TemplateID:     tpl.ID,
		})
		_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			Transition:        database.WorkspaceTransitionStart,
			Reason:            database.BuildReasonAutostart,
			WorkspaceID:       ws.ID,
			TemplateVersionID: tv.ID,
			JobID:             job.ID,
		})
		wsresource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
			JobID: job.ID,
		})
		wsagent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
			ResourceID: wsresource.ID,
		})
		_ = dbgen.WorkspaceApp(t, db, database.WorkspaceApp{
			SharingLevel: database.AppSharingLevelOwner,
			Health:       database.WorkspaceAppHealthDisabled,
			OpenIn:       database.WorkspaceAppOpenInSlimWindow,
			AgentID:      wsagent.ID,
		})

		group := dbgen.Group(t, db, database.Group{
			OrganizationID: org.ID,
		})
		_ = dbgen.TelemetryItem(t, db, database.TelemetryItem{
			Key:   string(telemetry.TelemetryItemKeyHTMLFirstServedAt),
			Value: time.Now().Format(time.RFC3339),
		})
		_ = dbgen.GroupMember(t, db, database.GroupMemberTable{UserID: user.ID, GroupID: group.ID})
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

		_ = dbgen.WorkspaceAgentStat(t, db, database.WorkspaceAgentStat{
			ConnectionMedianLatencyMS: 1,
		})
		_, err = db.InsertLicense(ctx, database.InsertLicenseParams{
			UploadedAt: dbtime.Now(),
			JWT:        "",
			Exp:        dbtime.Now().Add(time.Hour),
			UUID:       uuid.New(),
		})
		assert.NoError(t, err)
		_, _ = dbgen.WorkspaceProxy(t, db, database.WorkspaceProxy{})

		_ = dbgen.WorkspaceModule(t, db, database.WorkspaceModule{
			JobID: job.ID,
		})
		_ = dbgen.WorkspaceAgentMemoryResourceMonitor(t, db, database.WorkspaceAgentMemoryResourceMonitor{
			AgentID: wsagent.ID,
		})
		_ = dbgen.WorkspaceAgentVolumeResourceMonitor(t, db, database.WorkspaceAgentVolumeResourceMonitor{
			AgentID: wsagent.ID,
		})

		previousAIBridgeInterceptionPeriod := now.Truncate(time.Hour)
		user2 := dbgen.User(t, db, database.User{})
		aiBridgeInterception1 := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: user.ID,
			Provider:    "anthropic",
			Model:       "deanseek",
			StartedAt:   previousAIBridgeInterceptionPeriod.Add(-30 * time.Minute),
		}, nil)
		_ = dbgen.AIBridgeTokenUsage(t, db, database.InsertAIBridgeTokenUsageParams{
			InterceptionID:        aiBridgeInterception1.ID,
			InputTokens:           100,
			OutputTokens:          200,
			CacheReadInputTokens:  300,
			CacheWriteInputTokens: 400,
			Metadata:              json.RawMessage(`{"cache_read_input":300,"cache_creation_input":400}`),
		})
		_ = dbgen.AIBridgeUserPrompt(t, db, database.InsertAIBridgeUserPromptParams{
			InterceptionID: aiBridgeInterception1.ID,
		})
		_ = dbgen.AIBridgeToolUsage(t, db, database.InsertAIBridgeToolUsageParams{
			InterceptionID:  aiBridgeInterception1.ID,
			Injected:        true,
			InvocationError: sql.NullString{String: "error1", Valid: true},
		})
		_, err = db.UpdateAIBridgeInterceptionEnded(ctx, database.UpdateAIBridgeInterceptionEndedParams{
			ID:      aiBridgeInterception1.ID,
			EndedAt: aiBridgeInterception1.StartedAt.Add(1 * time.Minute), // 1 minute duration
		})
		require.NoError(t, err)
		aiBridgeInterception2 := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: user2.ID,
			Provider:    aiBridgeInterception1.Provider,
			Model:       aiBridgeInterception1.Model,
			StartedAt:   aiBridgeInterception1.StartedAt,
		}, nil)
		_ = dbgen.AIBridgeTokenUsage(t, db, database.InsertAIBridgeTokenUsageParams{
			InterceptionID:        aiBridgeInterception2.ID,
			InputTokens:           100,
			OutputTokens:          200,
			CacheReadInputTokens:  300,
			CacheWriteInputTokens: 400,
			Metadata:              json.RawMessage(`{"cache_read_input":300,"cache_creation_input":400}`),
		})
		_ = dbgen.AIBridgeUserPrompt(t, db, database.InsertAIBridgeUserPromptParams{
			InterceptionID: aiBridgeInterception2.ID,
		})
		_ = dbgen.AIBridgeToolUsage(t, db, database.InsertAIBridgeToolUsageParams{
			InterceptionID: aiBridgeInterception2.ID,
			Injected:       false,
		})
		_, err = db.UpdateAIBridgeInterceptionEnded(ctx, database.UpdateAIBridgeInterceptionEndedParams{
			ID:      aiBridgeInterception2.ID,
			EndedAt: aiBridgeInterception2.StartedAt.Add(2 * time.Minute), // 2 minute duration
		})
		require.NoError(t, err)
		aiBridgeInterception3 := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: user2.ID,
			Provider:    "openai",
			Model:       "gpt-5",
			StartedAt:   aiBridgeInterception1.StartedAt,
		}, nil)
		_, err = db.UpdateAIBridgeInterceptionEnded(ctx, database.UpdateAIBridgeInterceptionEndedParams{
			ID:      aiBridgeInterception3.ID,
			EndedAt: aiBridgeInterception3.StartedAt.Add(3 * time.Minute), // 3 minute duration
		})
		require.NoError(t, err)
		_ = dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: user2.ID,
			Provider:    "openai",
			Model:       "gpt-5",
			StartedAt:   aiBridgeInterception1.StartedAt,
		}, nil)
		// not ended, so it should not affect summaries

		clock := quartz.NewMock(t)
		clock.Set(now)

		deployment, snapshot := collectSnapshot(ctx, t, db, func(opts telemetry.Options) telemetry.Options {
			opts.Clock = clock
			return opts
		})
		var agentsExperiments map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(deployment.AgentsExperiments, &agentsExperiments))
		require.Contains(t, agentsExperiments, "virtual_desktop")
		require.Contains(t, agentsExperiments, "advisor")
		require.Len(t, snapshot.ProvisionerJobs, 1)
		require.Len(t, snapshot.Licenses, 1)
		require.Len(t, snapshot.Templates, 1)
		require.Len(t, snapshot.TemplateVersions, 2)
		require.Len(t, snapshot.Users, 2)
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
		telemetryItemKeys := slice.Convert(snapshot.TelemetryItems, func(item telemetry.TelemetryItem) string {
			return item.Key
		})
		require.ElementsMatch(t, []string{
			string(telemetry.TelemetryItemKeyHTMLFirstServedAt),
			string(telemetry.TelemetryItemKeyTelemetryEnabled),
		}, telemetryItemKeys)
		require.Len(t, snapshot.WorkspaceAgentMemoryResourceMonitors, 1)
		require.Len(t, snapshot.WorkspaceAgentVolumeResourceMonitors, 1)
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

		// 2 unique provider + model + client combinations
		require.Len(t, snapshot.AIBridgeInterceptionsSummaries, 2)
		snapshot1 := snapshot.AIBridgeInterceptionsSummaries[0]
		snapshot2 := snapshot.AIBridgeInterceptionsSummaries[1]
		if snapshot1.Provider != aiBridgeInterception1.Provider {
			snapshot1, snapshot2 = snapshot2, snapshot1
		}

		require.Equal(t, snapshot1.Provider, aiBridgeInterception1.Provider)
		require.Equal(t, snapshot1.Model, aiBridgeInterception1.Model)
		require.Equal(t, snapshot1.Client, "Unknown") // no client info yet
		require.EqualValues(t, snapshot1.InterceptionCount, 2)
		require.EqualValues(t, snapshot1.InterceptionsByRoute, map[string]int64{}) // no route info yet
		require.EqualValues(t, snapshot1.InterceptionDurationMillis.P50, 90_000)
		require.EqualValues(t, snapshot1.InterceptionDurationMillis.P90, 114_000)
		require.EqualValues(t, snapshot1.InterceptionDurationMillis.P95, 117_000)
		require.EqualValues(t, snapshot1.InterceptionDurationMillis.P99, 119_400)
		require.EqualValues(t, snapshot1.UniqueInitiatorCount, 2)
		require.EqualValues(t, snapshot1.UserPromptsCount, 2)
		require.EqualValues(t, snapshot1.TokenUsagesCount, 2)
		require.EqualValues(t, snapshot1.TokenCount.Input, 200)
		require.EqualValues(t, snapshot1.TokenCount.Output, 400)
		require.EqualValues(t, snapshot1.TokenCount.CachedRead, 600)
		require.EqualValues(t, snapshot1.TokenCount.CachedWritten, 800)
		require.EqualValues(t, snapshot1.ToolCallsCount.Injected, 1)
		require.EqualValues(t, snapshot1.ToolCallsCount.NonInjected, 1)
		require.EqualValues(t, snapshot1.InjectedToolCallErrorCount, 1)

		require.Equal(t, snapshot2.Provider, aiBridgeInterception3.Provider)
		require.Equal(t, snapshot2.Model, aiBridgeInterception3.Model)
		require.Equal(t, snapshot2.Client, "Unknown") // no client info yet
		require.EqualValues(t, snapshot2.InterceptionCount, 1)
		require.EqualValues(t, snapshot2.InterceptionsByRoute, map[string]int64{}) // no route info yet
		require.EqualValues(t, snapshot2.InterceptionDurationMillis.P50, 180_000)
		require.EqualValues(t, snapshot2.InterceptionDurationMillis.P90, 180_000)
		require.EqualValues(t, snapshot2.InterceptionDurationMillis.P95, 180_000)
		require.EqualValues(t, snapshot2.InterceptionDurationMillis.P99, 180_000)
		require.EqualValues(t, snapshot2.UniqueInitiatorCount, 1)
		require.EqualValues(t, snapshot2.UserPromptsCount, 0)
		require.EqualValues(t, snapshot2.TokenUsagesCount, 0)
		require.EqualValues(t, snapshot2.TokenCount.Input, 0)
		require.EqualValues(t, snapshot2.TokenCount.Output, 0)
		require.EqualValues(t, snapshot2.TokenCount.CachedRead, 0)
		require.EqualValues(t, snapshot2.TokenCount.CachedWritten, 0)
		require.EqualValues(t, snapshot2.ToolCallsCount.Injected, 0)
		require.EqualValues(t, snapshot2.ToolCallsCount.NonInjected, 0)
	})
	t.Run("HashedEmail", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		db, _ := dbtestutil.NewDB(t)
		_ = dbgen.User(t, db, database.User{
			Email: "kyle@coder.com",
		})
		_, snapshot := collectSnapshot(ctx, t, db, nil)
		require.Len(t, snapshot.Users, 1)
		require.Equal(t, snapshot.Users[0].EmailHashed, "bb44bf07cf9a2db0554bba63a03d822c927deae77df101874496df5a6a3e896d@coder.com")
	})
	t.Run("HashedModule", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitMedium)
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
		_, snapshot := collectSnapshot(ctx, t, db, nil)
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
		deployment, _ := collectSnapshot(ctx, t, db, nil)
		require.False(t, *deployment.IDPOrgSync)

		// 2. Org sync settings set in server flags
		deployment, _ = collectSnapshot(ctx, t, db, func(opts telemetry.Options) telemetry.Options {
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
		deployment, _ = collectSnapshot(ctx, t, db, nil)
		require.True(t, *deployment.IDPOrgSync)
	})
	t.Run("SCIM", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		db, _ := dbtestutil.NewDB(t)

		// 1. Default Options: both flags false (and reported as such).
		deployment, _ := collectSnapshot(ctx, t, db, nil)
		require.NotNil(t, deployment.SCIMEnabled)
		require.False(t, *deployment.SCIMEnabled)
		require.NotNil(t, deployment.SCIMUseLegacy)
		require.False(t, *deployment.SCIMUseLegacy)

		// 2. Both Options flags true: both reported true.
		deployment, _ = collectSnapshot(ctx, t, db, func(opts telemetry.Options) telemetry.Options {
			opts.SCIMEnabled = true
			opts.SCIMUseLegacy = true
			return opts
		})
		require.True(t, *deployment.SCIMEnabled)
		require.True(t, *deployment.SCIMUseLegacy)

		// 3. Enabled only: enabled true, legacy false.
		deployment, _ = collectSnapshot(ctx, t, db, func(opts telemetry.Options) telemetry.Options {
			opts.SCIMEnabled = true
			return opts
		})
		require.True(t, *deployment.SCIMEnabled)
		require.False(t, *deployment.SCIMUseLegacy)

		// 4. The reporter never reads DeploymentConfig.SCIMAPIKey directly:
		//    even if a non-empty key sneaks through (it would not in production
		//    because of WithoutSecrets), SCIMEnabled reflects only Options.
		deployment, _ = collectSnapshot(ctx, t, db, func(opts telemetry.Options) telemetry.Options {
			opts.DeploymentConfig = &codersdk.DeploymentValues{
				SCIMAPIKey: "a-secret-bearer-token",
			}
			return opts
		})
		require.False(t, *deployment.SCIMEnabled)
	})
}

// nolint:paralleltest
func TestTelemetryInstallSource(t *testing.T) {
	t.Setenv("CODER_TELEMETRY_INSTALL_SOURCE", "aws_marketplace")
	ctx := testutil.Context(t, testutil.WaitMedium)
	db, _ := dbtestutil.NewDB(t)
	deployment, _ := collectSnapshot(ctx, t, db, nil)
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

func TestPrebuiltWorkspacesTelemetry(t *testing.T) {
	t.Parallel()
	db, _ := dbtestutil.NewDB(t)

	cases := []struct {
		name                    string
		storeFn                 func(store database.Store) database.Store
		expectedSnapshotEntries int
		expectedCreated         int
		expectedFailed          int
		expectedClaimed         int
	}{
		{
			name: "prebuilds enabled",
			storeFn: func(store database.Store) database.Store {
				return &mockDB{Store: store}
			},
			expectedSnapshotEntries: 3,
			expectedCreated:         5,
			expectedFailed:          2,
			expectedClaimed:         3,
		},
		{
			name: "prebuilds not used",
			storeFn: func(store database.Store) database.Store {
				return &emptyMockDB{Store: store}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)

			deployment, snapshot := collectSnapshot(ctx, t, db, func(opts telemetry.Options) telemetry.Options {
				opts.Database = tc.storeFn(db)
				return opts
			})

			require.NotNil(t, deployment)
			require.NotNil(t, snapshot)

			require.Len(t, snapshot.PrebuiltWorkspaces, tc.expectedSnapshotEntries)

			eventCounts := make(map[telemetry.PrebuiltWorkspaceEventType]int)
			for _, event := range snapshot.PrebuiltWorkspaces {
				eventCounts[event.EventType] = event.Count
				require.NotEqual(t, uuid.Nil, event.ID)
				require.False(t, event.CreatedAt.IsZero())
			}

			require.Equal(t, tc.expectedCreated, eventCounts[telemetry.PrebuiltWorkspaceEventTypeCreated])
			require.Equal(t, tc.expectedFailed, eventCounts[telemetry.PrebuiltWorkspaceEventTypeFailed])
			require.Equal(t, tc.expectedClaimed, eventCounts[telemetry.PrebuiltWorkspaceEventTypeClaimed])
		})
	}
}

type mockDB struct {
	database.Store
}

func (*mockDB) GetPrebuildMetrics(context.Context) ([]database.GetPrebuildMetricsRow, error) {
	return []database.GetPrebuildMetricsRow{
		{
			TemplateName:     "template1",
			PresetName:       "preset1",
			OrganizationName: "org1",
			CreatedCount:     3,
			FailedCount:      1,
			ClaimedCount:     2,
		},
		{
			TemplateName:     "template2",
			PresetName:       "preset2",
			OrganizationName: "org1",
			CreatedCount:     2,
			FailedCount:      1,
			ClaimedCount:     1,
		},
	}, nil
}

type emptyMockDB struct {
	database.Store
}

func (*emptyMockDB) GetPrebuildMetrics(context.Context) ([]database.GetPrebuildMetricsRow, error) {
	return []database.GetPrebuildMetricsRow{}, nil
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

			for range 3 {
				// Whatever happens, subsequent calls should not report if telemetryEnabled didn't change
				snapshot2, err := telemetry.RecordTelemetryStatus(ctx, logger, db, testCase.telemetryEnabled)
				require.NoError(t, err)
				require.Nil(t, snapshot2)
			}
		})
	}
}

func mockTelemetryServer(ctx context.Context, t *testing.T) (*url.URL, chan *telemetry.Deployment, chan *telemetry.Snapshot) {
	t.Helper()
	deployment := make(chan *telemetry.Deployment, 64)
	snapshot := make(chan *telemetry.Snapshot, 64)
	r := chi.NewRouter()
	r.Post("/deployment", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, buildinfo.Version(), r.Header.Get(telemetry.VersionHeader))
		dd := &telemetry.Deployment{}
		err := json.NewDecoder(r.Body).Decode(dd)
		require.NoError(t, err)
		ok := testutil.AssertSend(ctx, t, deployment, dd)
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		// Ensure the header is sent only after deployment is sent
		w.WriteHeader(http.StatusAccepted)
	})
	r.Post("/snapshot", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, buildinfo.Version(), r.Header.Get(telemetry.VersionHeader))
		ss := &telemetry.Snapshot{}
		err := json.NewDecoder(r.Body).Decode(ss)
		require.NoError(t, err)
		ok := testutil.AssertSend(ctx, t, snapshot, ss)
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		// Ensure the header is sent only after snapshot is sent
		w.WriteHeader(http.StatusAccepted)
	})
	server := httptest.NewServer(r)
	t.Cleanup(server.Close)
	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)

	return serverURL, deployment, snapshot
}

func collectSnapshot(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	addOptionsFn func(opts telemetry.Options) telemetry.Options,
) (*telemetry.Deployment, *telemetry.Snapshot) {
	t.Helper()

	serverURL, deployment, snapshot := mockTelemetryServer(ctx, t)

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

	return testutil.RequireReceive(ctx, t, deployment), testutil.RequireReceive(ctx, t, snapshot)
}

func TestTelemetry_BoundaryUsageSummary(t *testing.T) {
	t.Parallel()

	t.Run("IncludedInSnapshot", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitMedium)

		tracker := boundaryusage.NewTracker()
		workspace1, workspace2 := uuid.New(), uuid.New()
		user1, user2 := uuid.New(), uuid.New()
		replicaID := uuid.New()

		tracker.Track(workspace1, user1, 10, 2)
		tracker.Track(workspace2, user1, 5, 1)
		tracker.Track(workspace2, user2, 3, 0)

		// Flush the tracker to the database.
		err := tracker.FlushToDB(ctx, db, replicaID)
		require.NoError(t, err)

		// Collect a snapshot and verify boundary usage is included.
		clock := quartz.NewMock(t)
		clock.Set(dbtime.Now())

		_, snapshot := collectSnapshot(ctx, t, db, func(opts telemetry.Options) telemetry.Options {
			opts.Clock = clock
			return opts
		})

		require.NotNil(t, snapshot.BoundaryUsageSummary)
		require.Equal(t, int64(2), snapshot.BoundaryUsageSummary.UniqueWorkspaces)
		require.Equal(t, int64(2), snapshot.BoundaryUsageSummary.UniqueUsers)
		require.Equal(t, int64(10+5+3), snapshot.BoundaryUsageSummary.AllowedRequests)
		require.Equal(t, int64(2+1+0), snapshot.BoundaryUsageSummary.DeniedRequests)
		require.Equal(t, clock.Now().Add(-telemetry.DefaultSnapshotFrequency), snapshot.BoundaryUsageSummary.PeriodStart)
		require.Equal(t, int64(telemetry.DefaultSnapshotFrequency/time.Millisecond), snapshot.BoundaryUsageSummary.PeriodDurationMilliseconds)
	})

	t.Run("ResetAfterCollection", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitMedium)

		tracker := boundaryusage.NewTracker()
		replicaID := uuid.New()

		tracker.Track(uuid.New(), uuid.New(), 5, 1)
		err := tracker.FlushToDB(ctx, db, replicaID)
		require.NoError(t, err)

		clock := quartz.NewMock(t)
		clock.Set(dbtime.Now())

		// First snapshot should have the data.
		_, snapshot1 := collectSnapshot(ctx, t, db, func(opts telemetry.Options) telemetry.Options {
			opts.Clock = clock
			return opts
		})
		require.NotNil(t, snapshot1.BoundaryUsageSummary)
		require.Equal(t, int64(5), snapshot1.BoundaryUsageSummary.AllowedRequests)

		// Advance clock to next snapshot period to avoid lock conflict.
		clock.Advance(30 * time.Minute)

		// Second snapshot should have no data (stats were reset).
		_, snapshot2 := collectSnapshot(ctx, t, db, func(opts telemetry.Options) telemetry.Options {
			opts.Clock = clock
			return opts
		})
		// Summary should be nil or have zero values since stats were reset.
		if snapshot2.BoundaryUsageSummary != nil {
			require.Equal(t, int64(0), snapshot2.BoundaryUsageSummary.AllowedRequests)
		}
	})

	t.Run("OnlyOneReplicaCollects", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitMedium)

		// Set up boundary usage stats from two replicas.
		tracker1 := boundaryusage.NewTracker()
		tracker2 := boundaryusage.NewTracker()
		replica1ID := uuid.New()
		replica2ID := uuid.New()

		tracker1.Track(uuid.New(), uuid.New(), 10, 1)
		tracker2.Track(uuid.New(), uuid.New(), 20, 2)

		err := tracker1.FlushToDB(ctx, db, replica1ID)
		require.NoError(t, err)
		err = tracker2.FlushToDB(ctx, db, replica2ID)
		require.NoError(t, err)

		clock := quartz.NewMock(t)
		clock.Set(dbtime.Now())

		// First snapshot collects and resets.
		_, snapshot1 := collectSnapshot(ctx, t, db, func(opts telemetry.Options) telemetry.Options {
			opts.Clock = clock
			return opts
		})
		require.NotNil(t, snapshot1.BoundaryUsageSummary)
		require.Equal(t, int64(10+20), snapshot1.BoundaryUsageSummary.AllowedRequests)

		// Second snapshot in same period should skip (lock already claimed).
		_, snapshot2 := collectSnapshot(ctx, t, db, func(opts telemetry.Options) telemetry.Options {
			opts.Clock = clock
			return opts
		})
		// The second snapshot should have nil because another "replica" already
		// claimed the lock for this period.
		require.Nil(t, snapshot2.BoundaryUsageSummary)
	})
}

func TestChatsTelemetry(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitMedium)
	db, _ := dbtestutil.NewDB(t)

	user := dbgen.User(t, db, database.User{})

	// Create chat providers (required FK for model configs).
	anthropicProvider := dbgen.ChatProvider(t, db, database.ChatProvider{
		Provider:    "anthropic",
		DisplayName: "Anthropic",
	})
	openaiProvider := dbgen.ChatProvider(t, db, database.ChatProvider{
		Provider:    "openai",
		DisplayName: "OpenAI",
	})

	// Create a model config.
	modelCfg := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		AIProviderID: uuid.NullUUID{UUID: anthropicProvider.ID, Valid: true},
		Model:        "claude-sonnet-4-20250514",
		DisplayName:  "Claude Sonnet",
		IsDefault:    true,
		ContextLimit: 200000,
	})

	org2 := dbgen.Organization(t, db, database.Organization{})
	modelCfg2 := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		AIProviderID:   uuid.NullUUID{UUID: openaiProvider.ID, Valid: true},
		OrganizationID: org2.ID,
		Model:          "gpt-4o",
		DisplayName:    "GPT-4o",
	})

	// Soft-deleted model configurations must not appear in telemetry.
	deletedCfg := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		AIProviderID: uuid.NullUUID{UUID: anthropicProvider.ID, Valid: true},
		Model:        "claude-deleted",
		DisplayName:  "Deleted Model",
		ContextLimit: 100000,
	})
	_, err := db.DeleteChatModelConfigByID(ctx, deletedCfg.ID)
	require.NoError(t, err)

	// Create a root chat with a workspace.
	org, err := db.GetDefaultOrganization(ctx)
	require.NoError(t, err)
	job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		OrganizationID: org.ID,
		Type:           database.ProvisionerJobTypeTemplateVersionDryRun,
	})
	tpl := dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
	})
	tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		OrganizationID: org.ID,
		TemplateID:     uuid.NullUUID{UUID: tpl.ID, Valid: true},
		CreatedBy:      user.ID,
		JobID:          job.ID,
	})
	ws := dbgen.Workspace(t, db, database.WorkspaceTable{
		OwnerID:        user.ID,
		OrganizationID: org.ID,
		TemplateID:     tpl.ID,
	})
	_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		Transition:        database.WorkspaceTransitionStart,
		Reason:            database.BuildReasonInitiator,
		WorkspaceID:       ws.ID,
		TemplateVersionID: tv.ID,
		JobID:             job.ID,
	})

	rootChat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: modelCfg.ID,
		Title:             "Root Chat",
		Status:            database.ChatStatusRunning,
		WorkspaceID:       uuid.NullUUID{UUID: ws.ID, Valid: true},
		Mode:              database.NullChatMode{ChatMode: database.ChatModeComputerUse, Valid: true},
	})

	// Create a child chat (has parent + root).
	childChat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: modelCfg2.ID,
		Title:             "Child Chat",
		Status:            database.ChatStatusWaiting,
		ParentChatID:      uuid.NullUUID{UUID: rootChat.ID, Valid: true},
		RootChatID:        uuid.NullUUID{UUID: rootChat.ID, Valid: true},
	})

	// Associate a PR with the root chat so PullRequestState is populated.
	rootChatNow := dbtime.Now()
	_, err = db.UpsertChatDiffStatus(ctx, database.UpsertChatDiffStatusParams{
		ChatID:           rootChat.ID,
		PullRequestState: sql.NullString{String: "merged", Valid: true},
		RefreshedAt:      rootChatNow,
		StaleAt:          rootChatNow,
	})
	require.NoError(t, err)

	// Insert messages for root chat: 2 user, 2 assistant, 1 tool.
	_ = dbgen.ChatMessage(t, db, database.ChatMessage{
		ChatID:              rootChat.ID,
		CreatedBy:           uuid.NullUUID{UUID: user.ID, Valid: true},
		ModelConfigID:       uuid.NullUUID{UUID: modelCfg.ID, Valid: true},
		Role:                database.ChatMessageRoleUser,
		Content:             pqtype.NullRawMessage{RawMessage: json.RawMessage(`[{"type":"text","text":"hello"}]`), Valid: true},
		InputTokens:         sql.NullInt64{Int64: 100, Valid: true},
		TotalTokens:         sql.NullInt64{Int64: 100, Valid: true},
		CacheCreationTokens: sql.NullInt64{Int64: 50, Valid: true},
		ContextLimit:        sql.NullInt64{Int64: 200000, Valid: true},
	})
	_ = dbgen.ChatMessage(t, db, database.ChatMessage{
		ChatID:             rootChat.ID,
		ModelConfigID:      uuid.NullUUID{UUID: modelCfg.ID, Valid: true},
		Role:               database.ChatMessageRoleAssistant,
		Content:            pqtype.NullRawMessage{RawMessage: json.RawMessage(`[{"type":"text","text":"hi"}]`), Valid: true},
		InputTokens:        sql.NullInt64{Int64: 200, Valid: true},
		OutputTokens:       sql.NullInt64{Int64: 50, Valid: true},
		TotalTokens:        sql.NullInt64{Int64: 250, Valid: true},
		ReasoningTokens:    sql.NullInt64{Int64: 10, Valid: true},
		CacheReadTokens:    sql.NullInt64{Int64: 25, Valid: true},
		ContextLimit:       sql.NullInt64{Int64: 200000, Valid: true},
		RuntimeMs:          sql.NullInt64{Int64: 500, Valid: true},
		ProviderResponseID: sql.NullString{String: "resp-1", Valid: true},
	})
	_ = dbgen.ChatMessage(t, db, database.ChatMessage{
		ChatID:              rootChat.ID,
		CreatedBy:           uuid.NullUUID{UUID: user.ID, Valid: true},
		ModelConfigID:       uuid.NullUUID{UUID: modelCfg.ID, Valid: true},
		Role:                database.ChatMessageRoleUser,
		Content:             pqtype.NullRawMessage{RawMessage: json.RawMessage(`[{"type":"text","text":"help"}]`), Valid: true},
		InputTokens:         sql.NullInt64{Int64: 150, Valid: true},
		TotalTokens:         sql.NullInt64{Int64: 150, Valid: true},
		CacheCreationTokens: sql.NullInt64{Int64: 30, Valid: true},
		ContextLimit:        sql.NullInt64{Int64: 200000, Valid: true},
	})
	_ = dbgen.ChatMessage(t, db, database.ChatMessage{
		ChatID:             rootChat.ID,
		ModelConfigID:      uuid.NullUUID{UUID: modelCfg.ID, Valid: true},
		Role:               database.ChatMessageRoleAssistant,
		Content:            pqtype.NullRawMessage{RawMessage: json.RawMessage(`[{"type":"text","text":"sure"}]`), Valid: true},
		InputTokens:        sql.NullInt64{Int64: 300, Valid: true},
		OutputTokens:       sql.NullInt64{Int64: 100, Valid: true},
		TotalTokens:        sql.NullInt64{Int64: 400, Valid: true},
		ReasoningTokens:    sql.NullInt64{Int64: 20, Valid: true},
		CacheReadTokens:    sql.NullInt64{Int64: 40, Valid: true},
		ContextLimit:       sql.NullInt64{Int64: 200000, Valid: true},
		RuntimeMs:          sql.NullInt64{Int64: 800, Valid: true},
		ProviderResponseID: sql.NullString{String: "resp-2", Valid: true},
	})
	_ = dbgen.ChatMessage(t, db, database.ChatMessage{
		ChatID:        rootChat.ID,
		ModelConfigID: uuid.NullUUID{UUID: modelCfg.ID, Valid: true},
		Role:          database.ChatMessageRoleTool,
		Content:       pqtype.NullRawMessage{RawMessage: json.RawMessage(`[{"type":"text","text":"result"}]`), Valid: true},
		ContextLimit:  sql.NullInt64{Int64: 200000, Valid: true},
		RuntimeMs:     sql.NullInt64{Int64: 100, Valid: true},
	})

	// Insert messages for child chat: 1 user, 1 assistant (compressed).
	_ = dbgen.ChatMessage(t, db, database.ChatMessage{
		ChatID:              childChat.ID,
		CreatedBy:           uuid.NullUUID{UUID: user.ID, Valid: true},
		ModelConfigID:       uuid.NullUUID{UUID: modelCfg2.ID, Valid: true},
		Role:                database.ChatMessageRoleUser,
		Content:             pqtype.NullRawMessage{RawMessage: json.RawMessage(`[{"type":"text","text":"q"}]`), Valid: true},
		InputTokens:         sql.NullInt64{Int64: 500, Valid: true},
		TotalTokens:         sql.NullInt64{Int64: 500, Valid: true},
		CacheCreationTokens: sql.NullInt64{Int64: 100, Valid: true},
		ContextLimit:        sql.NullInt64{Int64: 128000, Valid: true},
	})
	_ = dbgen.ChatMessage(t, db, database.ChatMessage{
		ChatID:             childChat.ID,
		ModelConfigID:      uuid.NullUUID{UUID: modelCfg2.ID, Valid: true},
		Role:               database.ChatMessageRoleAssistant,
		Content:            pqtype.NullRawMessage{RawMessage: json.RawMessage(`[{"type":"text","text":"a"}]`), Valid: true},
		InputTokens:        sql.NullInt64{Int64: 600, Valid: true},
		OutputTokens:       sql.NullInt64{Int64: 200, Valid: true},
		TotalTokens:        sql.NullInt64{Int64: 800, Valid: true},
		ReasoningTokens:    sql.NullInt64{Int64: 50, Valid: true},
		CacheReadTokens:    sql.NullInt64{Int64: 75, Valid: true},
		ContextLimit:       sql.NullInt64{Int64: 128000, Valid: true},
		Compressed:         true,
		RuntimeMs:          sql.NullInt64{Int64: 1200, Valid: true},
		ProviderResponseID: sql.NullString{String: "resp-3", Valid: true},
	})

	// Large token values expose a missing soft-delete filter in the totals.
	poisonMsg := dbgen.ChatMessage(t, db, database.ChatMessage{
		ChatID:              rootChat.ID,
		ModelConfigID:       uuid.NullUUID{UUID: modelCfg.ID, Valid: true},
		Role:                database.ChatMessageRoleAssistant,
		Content:             pqtype.NullRawMessage{RawMessage: json.RawMessage(`[{"type":"text","text":"poison"}]`), Valid: true},
		InputTokens:         sql.NullInt64{Int64: 999999, Valid: true},
		OutputTokens:        sql.NullInt64{Int64: 999999, Valid: true},
		TotalTokens:         sql.NullInt64{Int64: 999999, Valid: true},
		ReasoningTokens:     sql.NullInt64{Int64: 999999, Valid: true},
		CacheCreationTokens: sql.NullInt64{Int64: 999999, Valid: true},
		CacheReadTokens:     sql.NullInt64{Int64: 999999, Valid: true},
		ContextLimit:        sql.NullInt64{Int64: 200000, Valid: true},
		RuntimeMs:           sql.NullInt64{Int64: 999999, Valid: true},
	})
	err = db.SoftDeleteChatMessageByID(ctx, poisonMsg.ID)
	require.NoError(t, err)

	_, snapshot := collectSnapshot(ctx, t, db, nil)

	// --- Assert Chats ---
	require.Len(t, snapshot.Chats, 2)

	// Find root and child by HasParent flag.
	var foundRoot, foundChild *telemetry.Chat
	for i := range snapshot.Chats {
		if !snapshot.Chats[i].HasParent {
			foundRoot = &snapshot.Chats[i]
		} else {
			foundChild = &snapshot.Chats[i]
		}
	}
	require.NotNil(t, foundRoot, "expected root chat")
	require.NotNil(t, foundChild, "expected child chat")

	// Root chat assertions.
	assert.Equal(t, rootChat.ID, foundRoot.ID)
	assert.Equal(t, user.ID, foundRoot.OwnerID)
	assert.Equal(t, org.ID, foundRoot.OrganizationID)
	assert.Equal(t, "running", foundRoot.Status)
	assert.False(t, foundRoot.HasParent)
	assert.Nil(t, foundRoot.RootChatID)
	require.NotNil(t, foundRoot.WorkspaceID)
	assert.Equal(t, ws.ID, *foundRoot.WorkspaceID)
	assert.Equal(t, modelCfg.ID, foundRoot.LastModelConfigID)
	require.NotNil(t, foundRoot.Mode)
	assert.Equal(t, "computer_use", *foundRoot.Mode)
	assert.False(t, foundRoot.Archived)
	assert.Equal(t, "ui", foundRoot.ClientType)
	require.NotNil(t, foundRoot.PullRequestState)
	assert.Equal(t, "merged", *foundRoot.PullRequestState)

	// Child chat assertions.

	assert.Equal(t, childChat.ID, foundChild.ID)
	assert.Equal(t, user.ID, foundChild.OwnerID)
	assert.True(t, foundChild.HasParent)
	require.NotNil(t, foundChild.RootChatID)
	assert.Equal(t, rootChat.ID, *foundChild.RootChatID)
	assert.Nil(t, foundChild.WorkspaceID)
	assert.Equal(t, "waiting", foundChild.Status)
	assert.Equal(t, modelCfg2.ID, foundChild.LastModelConfigID)
	assert.Nil(t, foundChild.Mode)
	assert.False(t, foundChild.Archived)
	assert.Equal(t, "ui", foundChild.ClientType)
	assert.Nil(t, foundChild.PullRequestState)

	// --- Assert ChatMessageSummaries ---

	require.Len(t, snapshot.ChatMessageSummaries, 2)

	summaryMap := make(map[uuid.UUID]telemetry.ChatMessageSummary)
	for _, s := range snapshot.ChatMessageSummaries {
		summaryMap[s.ChatID] = s
	}

	// Root chat summary: 2 user + 2 assistant + 1 tool = 5 messages.
	rootSummary, ok := summaryMap[rootChat.ID]
	require.True(t, ok, "expected summary for root chat")
	assert.Equal(t, int64(5), rootSummary.MessageCount)
	assert.Equal(t, int64(2), rootSummary.UserMessageCount)
	assert.Equal(t, int64(2), rootSummary.AssistantMessageCount)
	assert.Equal(t, int64(1), rootSummary.ToolMessageCount)
	assert.Equal(t, int64(0), rootSummary.SystemMessageCount)
	assert.Equal(t, int64(750), rootSummary.TotalInputTokens)        // 100+200+150+300+0
	assert.Equal(t, int64(150), rootSummary.TotalOutputTokens)       // 0+50+0+100+0
	assert.Equal(t, int64(30), rootSummary.TotalReasoningTokens)     // 0+10+0+20+0
	assert.Equal(t, int64(80), rootSummary.TotalCacheCreationTokens) // 50+0+30+0+0
	assert.Equal(t, int64(65), rootSummary.TotalCacheReadTokens)     // 0+25+0+40+0
	assert.Equal(t, int64(1400), rootSummary.TotalRuntimeMs)         // 0+500+0+800+100
	assert.Equal(t, int64(1), rootSummary.DistinctModelCount)
	assert.Equal(t, int64(0), rootSummary.CompressedMessageCount)

	// Child chat summary: 1 user + 1 assistant = 2 messages, 1 compressed.
	childSummary, ok := summaryMap[childChat.ID]
	require.True(t, ok, "expected summary for child chat")
	assert.Equal(t, int64(2), childSummary.MessageCount)
	assert.Equal(t, int64(1), childSummary.UserMessageCount)
	assert.Equal(t, int64(1), childSummary.AssistantMessageCount)
	assert.Equal(t, int64(1100), childSummary.TotalInputTokens)   // 500+600
	assert.Equal(t, int64(200), childSummary.TotalOutputTokens)   // 0+200
	assert.Equal(t, int64(50), childSummary.TotalReasoningTokens) // 0+50
	assert.Equal(t, int64(0), childSummary.ToolMessageCount)
	assert.Equal(t, int64(0), childSummary.SystemMessageCount)
	assert.Equal(t, int64(100), childSummary.TotalCacheCreationTokens) // 100+0
	assert.Equal(t, int64(75), childSummary.TotalCacheReadTokens)      // 0+75
	assert.Equal(t, int64(1200), childSummary.TotalRuntimeMs)          // 0+1200
	assert.Equal(t, int64(1), childSummary.DistinctModelCount)
	assert.Equal(t, int64(1), childSummary.CompressedMessageCount)

	// --- Assert ChatModelConfigs ---
	require.Len(t, snapshot.ChatModelConfigs, 2)

	configMap := make(map[uuid.UUID]telemetry.ChatModelConfig)
	for _, c := range snapshot.ChatModelConfigs {
		configMap[c.ID] = c
	}

	cfg1, ok := configMap[modelCfg.ID]
	require.True(t, ok)
	assert.Equal(t, org.ID, cfg1.OrganizationID)
	assert.Equal(t, "anthropic", cfg1.Provider)
	assert.Equal(t, "claude-sonnet-4-20250514", cfg1.Model)
	assert.Equal(t, int64(200000), cfg1.ContextLimit)
	assert.True(t, cfg1.Enabled)
	assert.True(t, cfg1.IsDefault)

	cfg2, ok := configMap[modelCfg2.ID]
	require.True(t, ok)
	assert.Equal(t, org2.ID, cfg2.OrganizationID)
	assert.Equal(t, "openai", cfg2.Provider)
	assert.Equal(t, "gpt-4o", cfg2.Model)
	assert.Equal(t, int64(128000), cfg2.ContextLimit)
	assert.True(t, cfg2.Enabled)
	assert.False(t, cfg2.IsDefault)
}

func TestChatDiffStatusSummaryTelemetry(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitMedium)
	db, _ := dbtestutil.NewDB(t)

	// Verify zero counts when no chat_diff_statuses exist.
	_, emptySnapshot := collectSnapshot(ctx, t, db, nil)
	require.NotNil(t, emptySnapshot.ChatDiffStatusSummary)
	assert.Equal(t, int64(0), emptySnapshot.ChatDiffStatusSummary.Total)
	assert.Equal(t, int64(0), emptySnapshot.ChatDiffStatusSummary.Open)
	assert.Equal(t, int64(0), emptySnapshot.ChatDiffStatusSummary.Merged)
	assert.Equal(t, int64(0), emptySnapshot.ChatDiffStatusSummary.Closed)

	// Set up minimal FK chain: provider -> model config -> chat.
	user := dbgen.User(t, db, database.User{})
	org, err := db.GetDefaultOrganization(ctx)
	require.NoError(t, err)

	anthropicProvider := dbgen.ChatProvider(t, db, database.ChatProvider{
		Provider:    "anthropic",
		DisplayName: "Anthropic",
	})

	modelCfg := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		AIProviderID: uuid.NullUUID{UUID: anthropicProvider.ID, Valid: true},
		Model:        "claude-sonnet-4-20250514",
		DisplayName:  "Claude Sonnet",
		IsDefault:    true,
		ContextLimit: 200000,
	})

	// Helper to create a chat and upsert its diff status.
	insertChatWithDiffStatus := func(prURL, state string) uuid.UUID {
		t.Helper()
		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    org.ID,
			OwnerID:           user.ID,
			LastModelConfigID: modelCfg.ID,
			Title:             "Chat " + state,
			Status:            database.ChatStatusWaiting,
		})
		now := dbtime.Now()
		_, chatErr := db.UpsertChatDiffStatus(ctx, database.UpsertChatDiffStatusParams{
			ChatID:           chat.ID,
			Url:              sql.NullString{String: prURL, Valid: prURL != ""},
			PullRequestState: sql.NullString{String: state, Valid: true},
			RefreshedAt:      now,
			StaleAt:          now,
		})
		require.NoError(t, chatErr)
		return chat.ID
	}

	// Insert: 1 merged, 1 open, 1 closed (each with unique URLs).
	// For pull/1, first insert an older chat with stale "open" state,
	// then a newer chat with refreshed "merged" state. The dedup
	// query orders by cds.updated_at DESC, so "merged" should win.
	insertChatWithDiffStatus("https://github.com/org/repo/pull/1", "open")
	insertChatWithDiffStatus("https://github.com/org/repo/pull/1", "merged")
	openChatID := insertChatWithDiffStatus("https://github.com/org/repo/pull/2", "open")
	insertChatWithDiffStatus("https://github.com/org/repo/pull/3", "closed")

	// Insert a chat with NULL pull_request_state (no PR yet).
	// This should be excluded from all counts.
	noPRChat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: modelCfg.ID,
		Title:             "Chat no PR",
		Status:            database.ChatStatusRunning,
	})
	now := dbtime.Now()
	_, err = db.UpsertChatDiffStatus(ctx, database.UpsertChatDiffStatusParams{
		ChatID:      noPRChat.ID,
		RefreshedAt: now,
		StaleAt:     now,
	})
	require.NoError(t, err)

	_, snapshot := collectSnapshot(ctx, t, db, nil)

	// 3 unique PRs (deduped by URL), not 4 chat_diff_statuses rows.
	require.NotNil(t, snapshot.ChatDiffStatusSummary)
	assert.Equal(t, int64(3), snapshot.ChatDiffStatusSummary.Total)
	assert.Equal(t, int64(1), snapshot.ChatDiffStatusSummary.Open)
	assert.Equal(t, int64(1), snapshot.ChatDiffStatusSummary.Merged)
	assert.Equal(t, int64(1), snapshot.ChatDiffStatusSummary.Closed)

	// Transition the "open" PR to "merged" via upsert on the same
	// chat_id. The aggregate should reflect the new state.
	now = dbtime.Now()
	_, err = db.UpsertChatDiffStatus(ctx, database.UpsertChatDiffStatusParams{
		ChatID:           openChatID,
		Url:              sql.NullString{String: "https://github.com/org/repo/pull/2", Valid: true},
		PullRequestState: sql.NullString{String: "merged", Valid: true},
		RefreshedAt:      now,
		StaleAt:          now,
	})
	require.NoError(t, err)

	_, snapshot2 := collectSnapshot(ctx, t, db, nil)

	require.NotNil(t, snapshot2.ChatDiffStatusSummary)
	assert.Equal(t, int64(3), snapshot2.ChatDiffStatusSummary.Total)
	assert.Equal(t, int64(0), snapshot2.ChatDiffStatusSummary.Open)
	assert.Equal(t, int64(2), snapshot2.ChatDiffStatusSummary.Merged)
	assert.Equal(t, int64(1), snapshot2.ChatDiffStatusSummary.Closed)
}

func TestUserSecretsTelemetry(t *testing.T) {
	t.Parallel()

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		db, _ := dbtestutil.NewDB(t)

		// Empty deployment should report a non-nil summary with zeros.
		_, snap := collectSnapshot(ctx, t, db, nil)
		require.Equal(t, &telemetry.UserSecretsSummary{}, snap.UserSecretsSummary)
	})

	t.Run("ConfigurationBreakdown", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		db, _ := dbtestutil.NewDB(t)

		userA := dbgen.User(t, db, database.User{})
		userB := dbgen.User(t, db, database.User{})

		// userA: env-only and file-only. dbgen.UserSecret defaults
		// EnvName and FilePath to non-empty, so use mutators to clear
		// them where the test wants empty values.
		_ = dbgen.UserSecret(t, db, database.UserSecret{
			UserID: userA.ID,
			Name:   "a-env",
		}, func(p *database.CreateUserSecretParams) {
			p.EnvName = "A_ENV"
			p.FilePath = ""
		})
		_ = dbgen.UserSecret(t, db, database.UserSecret{
			UserID: userA.ID,
			Name:   "a-file",
		}, func(p *database.CreateUserSecretParams) {
			p.EnvName = ""
			p.FilePath = "/home/coder/a.file"
		})
		// userB: both and neither.
		_ = dbgen.UserSecret(t, db, database.UserSecret{
			UserID: userB.ID,
			Name:   "b-both",
		}, func(p *database.CreateUserSecretParams) {
			p.EnvName = "B_BOTH"
			p.FilePath = "/home/coder/b.both"
		})
		_ = dbgen.UserSecret(t, db, database.UserSecret{
			UserID: userB.ID,
			Name:   "b-neither",
		}, func(p *database.CreateUserSecretParams) {
			p.EnvName = ""
			p.FilePath = ""
			// A target-less secret must be disabled to satisfy the
			// user_secrets_enabled_requires_target constraint. Disabled
			// secrets are still counted in the telemetry breakdown.
			p.Enabled = false
		})

		_, snap := collectSnapshot(ctx, t, db, nil)
		// Each user has exactly two secrets, so every percentile and
		// the max collapse to 2.
		require.Equal(t, &telemetry.UserSecretsSummary{
			UsersWithSecrets:  2,
			TotalSecrets:      4,
			EnvNameOnly:       1,
			FilePathOnly:      1,
			Both:              1,
			Neither:           1,
			SecretsPerUserMax: 2,
			SecretsPerUserP25: 2,
			SecretsPerUserP50: 2,
			SecretsPerUserP75: 2,
			SecretsPerUserP90: 2,
		}, snap.UserSecretsSummary)
	})

	t.Run("PercentileDistribution", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		db, _ := dbtestutil.NewDB(t)

		// Five users have secret counts 1, 2, 4, 8, 16 and five other
		// users have zero secrets. Including the zero-secret users in
		// the distribution gives a sorted vector of length 10:
		//   [0, 0, 0, 0, 0, 1, 2, 4, 8, 16]
		// percentile_disc(p) returns the value at the smallest
		// 1-indexed position i where i/n >= p, so the buckets land at:
		//   p25 -> position 3 -> 0
		//   p50 -> position 5 -> 0
		//   p75 -> position 8 -> 4
		//   p90 -> position 9 -> 8
		adopters := []int{1, 2, 4, 8, 16}
		for _, n := range adopters {
			u := dbgen.User(t, db, database.User{})
			for i := 0; i < n; i++ {
				_ = dbgen.UserSecret(t, db, database.UserSecret{
					UserID: u.ID,
					Name:   fmt.Sprintf("secret-%d", i),
				}, func(p *database.CreateUserSecretParams) {
					// Clear EnvName and FilePath so the unique
					// (user_id, env_name) and (user_id, file_path)
					// indexes don't collide across multiple secrets
					// for the same user. Target-less secrets must be
					// disabled to satisfy the
					// user_secrets_enabled_requires_target constraint.
					p.EnvName = ""
					p.FilePath = ""
					p.Enabled = false
				})
			}
		}
		for i := 0; i < 5; i++ {
			_ = dbgen.User(t, db, database.User{})
		}

		_, snap := collectSnapshot(ctx, t, db, nil)
		require.Equal(t, &telemetry.UserSecretsSummary{
			UsersWithSecrets:  5,
			TotalSecrets:      31,
			EnvNameOnly:       0,
			FilePathOnly:      0,
			Both:              0,
			Neither:           31,
			SecretsPerUserMax: 16,
			SecretsPerUserP25: 0,
			SecretsPerUserP50: 0,
			SecretsPerUserP75: 4,
			SecretsPerUserP90: 8,
		}, snap.UserSecretsSummary)
	})

	t.Run("FilterSkipsInactiveUsers", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		db, _ := dbtestutil.NewDB(t)

		// Active user with two secrets contributes the only entries
		// to UsersWithSecrets, TotalSecrets, and the percentile
		// distribution.
		active := dbgen.User(t, db, database.User{})
		_ = dbgen.UserSecret(t, db, database.UserSecret{
			UserID: active.ID,
			Name:   "active-env",
		}, func(p *database.CreateUserSecretParams) {
			p.EnvName = "ACTIVE_ENV"
			p.FilePath = ""
		})
		_ = dbgen.UserSecret(t, db, database.UserSecret{
			UserID: active.ID,
			Name:   "active-file",
		}, func(p *database.CreateUserSecretParams) {
			p.EnvName = ""
			p.FilePath = "/home/coder/active.file"
		})

		// User secret owned by a dormant user should be excluded.
		dormant := dbgen.User(t, db, database.User{Status: database.UserStatusDormant})
		_ = dbgen.UserSecret(t, db, database.UserSecret{
			UserID: dormant.ID,
			Name:   "dormant-secret",
		}, func(p *database.CreateUserSecretParams) {
			p.EnvName = "DORMANT_ENV"
			p.FilePath = ""
		})

		// User secret owned by a suspended user should be excluded.
		suspended := dbgen.User(t, db, database.User{Status: database.UserStatusSuspended})
		_ = dbgen.UserSecret(t, db, database.UserSecret{
			UserID: suspended.ID,
			Name:   "suspended-secret",
		}, func(p *database.CreateUserSecretParams) {
			p.EnvName = ""
			p.FilePath = "/home/coder/suspended.file"
		})

		// System user. Only its UUID is needed. Tying a secret to it
		// proves the is_system filter excludes it.
		_ = dbgen.UserSecret(t, db, database.UserSecret{
			UserID: database.PrebuildsSystemUserID,
			Name:   "prebuilds-secret",
		}, func(p *database.CreateUserSecretParams) {
			p.EnvName = ""
			p.FilePath = "/home/coder/prebuilds.file"
		})

		_, snap := collectSnapshot(ctx, t, db, nil)
		require.Equal(t, &telemetry.UserSecretsSummary{
			UsersWithSecrets:  1,
			TotalSecrets:      2,
			EnvNameOnly:       1,
			FilePathOnly:      1,
			Both:              0,
			Neither:           0,
			SecretsPerUserMax: 2,
			SecretsPerUserP25: 2,
			SecretsPerUserP50: 2,
			SecretsPerUserP75: 2,
			SecretsPerUserP90: 2,
		}, snap.UserSecretsSummary)
	})

	t.Run("OnlyOneReplicaCollects", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		db, _ := dbtestutil.NewDB(t)

		// Seed one user with one secret so the summary would normally
		// be populated. The user_secrets_summary aggregate has no
		// natural per-row UUID for the telemetry server to dedupe on,
		// so a telemetry lock elects a single replica per period.
		u := dbgen.User(t, db, database.User{})
		_ = dbgen.UserSecret(t, db, database.UserSecret{
			UserID: u.ID,
			Name:   "only-secret",
		}, func(p *database.CreateUserSecretParams) {
			p.EnvName = ""
			p.FilePath = ""
			// Target-less secrets must be disabled to satisfy the
			// user_secrets_enabled_requires_target constraint.
			p.Enabled = false
		})

		clock := quartz.NewMock(t)
		clock.Set(dbtime.Now())

		// First snapshot claims the lock and reports the summary.
		_, snap1 := collectSnapshot(ctx, t, db, func(opts telemetry.Options) telemetry.Options {
			opts.Clock = clock
			return opts
		})
		require.Equal(t, &telemetry.UserSecretsSummary{
			UsersWithSecrets:  1,
			TotalSecrets:      1,
			EnvNameOnly:       0,
			FilePathOnly:      0,
			Both:              0,
			Neither:           1,
			SecretsPerUserMax: 1,
			SecretsPerUserP25: 1,
			SecretsPerUserP50: 1,
			SecretsPerUserP75: 1,
			SecretsPerUserP90: 1,
		}, snap1.UserSecretsSummary)

		// A second snapshot in the same period simulates a second
		// replica racing to claim the lock; it should observe the
		// unique violation and skip reporting.
		_, snap2 := collectSnapshot(ctx, t, db, func(opts telemetry.Options) telemetry.Options {
			opts.Clock = clock
			return opts
		})
		require.Nil(t, snap2.UserSecretsSummary)
	})
}

func TestCollectAgentsVirtualDesktop(t *testing.T) {
	t.Parallel()

	collect := func(t *testing.T, opts telemetry.Options) telemetry.AgentsVirtualDesktopTelemetry {
		t.Helper()
		var payload telemetry.AgentsVirtualDesktopTelemetry
		require.NoError(t, json.Unmarshal(telemetry.CollectAgentsVirtualDesktop(context.Background(), opts), &payload))
		return payload
	}

	t.Run("Default", func(t *testing.T) {
		t.Parallel()

		db := dbmock.NewMockStore(gomock.NewController(t))
		db.EXPECT().GetChatComputerUseProvider(gomock.Any()).Return("", nil)

		payload := collect(t, telemetry.Options{Database: db, Logger: testutil.Logger(t)})
		require.False(t, payload.Enabled)
		require.EqualValues(t, codersdk.ChatComputerUseProviderAnthropic, payload.ComputerUse.Provider)
		require.Equal(t, "default", payload.ComputerUse.ProviderSource)
	})

	t.Run("Configured", func(t *testing.T) {
		t.Parallel()

		db := dbmock.NewMockStore(gomock.NewController(t))
		db.EXPECT().GetChatComputerUseProvider(gomock.Any()).Return("openai", nil)

		payload := collect(t, telemetry.Options{
			Database:    db,
			Logger:      testutil.Logger(t),
			Experiments: codersdk.Experiments{codersdk.ExperimentChatVirtualDesktop},
		})
		require.True(t, payload.Enabled)
		require.Equal(t, "openai", payload.ComputerUse.Provider)
		require.Equal(t, "configured", payload.ComputerUse.ProviderSource)
	})

	t.Run("QueryError", func(t *testing.T) {
		t.Parallel()

		db := dbmock.NewMockStore(gomock.NewController(t))
		db.EXPECT().GetChatComputerUseProvider(gomock.Any()).Return("", sql.ErrConnDone)

		payload := collect(t, telemetry.Options{Database: db, Logger: testutil.Logger(t)})
		require.Equal(t, telemetry.AgentsExperimentUnknown, payload.ComputerUse.Provider)
		require.Equal(t, telemetry.AgentsExperimentUnknown, payload.ComputerUse.ProviderSource)
	})
}

func TestCollectAgentsAdvisor(t *testing.T) {
	t.Parallel()

	collect := func(t *testing.T, opts telemetry.Options) telemetry.AgentsAdvisorTelemetry {
		t.Helper()
		var payload telemetry.AgentsAdvisorTelemetry
		require.NoError(t, json.Unmarshal(telemetry.CollectAgentsAdvisor(context.Background(), opts), &payload))
		return payload
	}
	marshalConfig := func(t *testing.T, cfg codersdk.AdvisorConfig) string {
		t.Helper()
		raw, err := json.Marshal(cfg)
		require.NoError(t, err)
		return string(raw)
	}
	expectOverrides := func(db *dbmock.MockStore, rows ...database.GetChatOrganizationModelOverridesByContextRow) {
		db.EXPECT().GetChatOrganizationModelOverridesByContext(gomock.Any(), string(codersdk.ChatModelOverrideContextAdvisor)).Return(rows, nil)
	}

	t.Run("Defaults", func(t *testing.T) {
		t.Parallel()
		db := dbmock.NewMockStore(gomock.NewController(t))
		db.EXPECT().GetChatAdvisorConfig(gomock.Any()).Return(marshalConfig(t, codersdk.AdvisorConfig{}), nil)
		expectOverrides(db)

		payload := collect(t, telemetry.Options{Database: db, Logger: testutil.Logger(t)})
		require.False(t, payload.Enabled)
		require.Zero(t, payload.MaxUsesPerRun)
		require.Zero(t, payload.MaxOutputTokens)
		require.Empty(t, payload.Overrides)
	})

	t.Run("ModelOverrides", func(t *testing.T) {
		t.Parallel()
		org := database.Organization{ID: uuid.New()}
		db := dbmock.NewMockStore(gomock.NewController(t))
		db.EXPECT().GetChatAdvisorConfig(gomock.Any()).Return(marshalConfig(t, codersdk.AdvisorConfig{MaxUsesPerRun: 7, MaxOutputTokens: 2048}), nil)
		expectOverrides(db, database.GetChatOrganizationModelOverridesByContextRow{
			OrganizationID: org.ID,
			ModelAvailable: true,
			Model:          "gpt-6-preview",
			ProviderType:   string(database.AIProviderTypeOpenai),
		})

		payload := collect(t, telemetry.Options{Database: db, Logger: testutil.Logger(t), Experiments: codersdk.Experiments{codersdk.ExperimentChatAdvisor}})
		require.True(t, payload.Enabled)
		require.Equal(t, 7, payload.MaxUsesPerRun)
		require.Equal(t, int64(2048), payload.MaxOutputTokens)
		require.Equal(t, []telemetry.AgentsAdvisorOverrideTelemetry{{
			OrganizationID: org.ID.String(), Provider: string(database.AIProviderTypeOpenai), Model: "gpt-6-preview",
		}}, payload.Overrides)
	})

	t.Run("InactiveModel", func(t *testing.T) {
		t.Parallel()
		org := database.Organization{ID: uuid.New()}
		db := dbmock.NewMockStore(gomock.NewController(t))
		db.EXPECT().GetChatAdvisorConfig(gomock.Any()).Return("{}", nil)
		expectOverrides(db, database.GetChatOrganizationModelOverridesByContextRow{
			OrganizationID: org.ID,
			ModelAvailable: false,
		})
		payload := collect(t, telemetry.Options{Database: db, Logger: testutil.Logger(t)})
		require.Equal(t, []telemetry.AgentsAdvisorOverrideTelemetry{{
			OrganizationID: org.ID.String(),
			Provider:       telemetry.AgentsExperimentAdvisorReuseChatModel,
			Model:          telemetry.AgentsExperimentAdvisorReuseChatModel,
		}}, payload.Overrides)
	})

	t.Run("NoOverride", func(t *testing.T) {
		t.Parallel()
		db := dbmock.NewMockStore(gomock.NewController(t))
		db.EXPECT().GetChatAdvisorConfig(gomock.Any()).Return("{}", nil)
		expectOverrides(db)
		payload := collect(t, telemetry.Options{Database: db, Logger: testutil.Logger(t)})
		require.Empty(t, payload.Overrides)
	})

	t.Run("MalformedConfig", func(t *testing.T) {
		t.Parallel()
		db := dbmock.NewMockStore(gomock.NewController(t))
		db.EXPECT().GetChatAdvisorConfig(gomock.Any()).Return("not-json", nil)
		expectOverrides(db)
		payload := collect(t, telemetry.Options{Database: db, Logger: testutil.Logger(t)})
		require.Zero(t, payload.MaxUsesPerRun)
		require.Zero(t, payload.MaxOutputTokens)
	})

	t.Run("ClampsNegativeLimits", func(t *testing.T) {
		t.Parallel()
		db := dbmock.NewMockStore(gomock.NewController(t))
		db.EXPECT().GetChatAdvisorConfig(gomock.Any()).Return(`{"max_uses_per_run":-3,"max_output_tokens":-99}`, nil)
		expectOverrides(db)
		payload := collect(t, telemetry.Options{Database: db, Logger: testutil.Logger(t)})
		require.Zero(t, payload.MaxUsesPerRun)
		require.Zero(t, payload.MaxOutputTokens)
	})

	t.Run("ConfigFetchError", func(t *testing.T) {
		t.Parallel()
		db := dbmock.NewMockStore(gomock.NewController(t))
		db.EXPECT().GetChatAdvisorConfig(gomock.Any()).Return("", sql.ErrConnDone)
		expectOverrides(db)
		payload := collect(t, telemetry.Options{Database: db, Logger: testutil.Logger(t)})
		require.Empty(t, payload.Overrides)
	})

	t.Run("OverridesFetchError", func(t *testing.T) {
		t.Parallel()
		db := dbmock.NewMockStore(gomock.NewController(t))
		db.EXPECT().GetChatAdvisorConfig(gomock.Any()).Return("{}", nil)
		db.EXPECT().GetChatOrganizationModelOverridesByContext(gomock.Any(), gomock.Any()).Return(nil, sql.ErrConnDone)
		payload := collect(t, telemetry.Options{Database: db, Logger: testutil.Logger(t)})
		require.Empty(t, payload.Overrides)
	})
}

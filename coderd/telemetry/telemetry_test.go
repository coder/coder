package telemetry_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"sort"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/coderd/boundaryusage"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/idpsync"
	"github.com/coder/coder/v2/coderd/runtimeconfig"
	"github.com/coder/coder/v2/coderd/telemetry"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m, testutil.GoleakOptions...)
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

		taskJob := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			Provisioner:    database.ProvisionerTypeTerraform,
			StorageMethod:  database.ProvisionerStorageMethodFile,
			Type:           database.ProvisionerJobTypeTemplateVersionDryRun,
			OrganizationID: org.ID,
		})
		taskTpl := dbgen.Template(t, db, database.Template{
			Provisioner:    database.ProvisionerTypeTerraform,
			OrganizationID: org.ID,
			CreatedBy:      user.ID,
		})
		taskTV := dbgen.TemplateVersion(t, db, database.TemplateVersion{
			OrganizationID: org.ID,
			TemplateID:     uuid.NullUUID{UUID: taskTpl.ID, Valid: true},
			CreatedBy:      user.ID,
			JobID:          taskJob.ID,
			HasAITask:      sql.NullBool{Bool: true, Valid: true},
		})
		taskWs := dbgen.Workspace(t, db, database.WorkspaceTable{
			OwnerID:        user.ID,
			OrganizationID: org.ID,
			TemplateID:     taskTpl.ID,
		})
		taskWsResource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
			JobID: taskJob.ID,
		})
		taskWsAgent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
			ResourceID: taskWsResource.ID,
		})
		taskWsApp := dbgen.WorkspaceApp(t, db, database.WorkspaceApp{
			SharingLevel: database.AppSharingLevelOwner,
			Health:       database.WorkspaceAppHealthDisabled,
			OpenIn:       database.WorkspaceAppOpenInSlimWindow,
			AgentID:      taskWsAgent.ID,
		})
		taskWB := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			Transition:        database.WorkspaceTransitionStart,
			Reason:            database.BuildReasonAutostart,
			WorkspaceID:       taskWs.ID,
			TemplateVersionID: tv.ID,
			JobID:             taskJob.ID,
			HasAITask:         sql.NullBool{Valid: true, Bool: true},
		})
		task := dbgen.Task(t, db, database.TaskTable{
			OwnerID:            user.ID,
			OrganizationID:     org.ID,
			WorkspaceID:        uuid.NullUUID{Valid: true, UUID: taskWs.ID},
			TemplateVersionID:  taskTV.ID,
			Prompt:             "example prompt",
			TemplateParameters: json.RawMessage(`{"foo": "bar"}`),
		})
		taskWA := dbgen.TaskWorkspaceApp(t, db, database.TaskWorkspaceApp{
			TaskID:               task.ID,
			WorkspaceAgentID:     uuid.NullUUID{Valid: true, UUID: taskWsAgent.ID},
			WorkspaceAppID:       uuid.NullUUID{Valid: true, UUID: taskWsApp.ID},
			WorkspaceBuildNumber: taskWB.BuildNumber,
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
			InterceptionID: aiBridgeInterception1.ID,
			InputTokens:    100,
			OutputTokens:   200,
			Metadata:       json.RawMessage(`{"cache_read_input":300,"cache_creation_input":400}`),
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
			InterceptionID: aiBridgeInterception2.ID,
			InputTokens:    100,
			OutputTokens:   200,
			Metadata:       json.RawMessage(`{"cache_read_input":300,"cache_creation_input":400}`),
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

		_, snapshot := collectSnapshot(ctx, t, db, func(opts telemetry.Options) telemetry.Options {
			opts.Clock = clock
			return opts
		})
		require.Len(t, snapshot.ProvisionerJobs, 2)
		require.Len(t, snapshot.Licenses, 1)
		require.Len(t, snapshot.Templates, 2)
		require.Len(t, snapshot.TemplateVersions, 3)
		require.Len(t, snapshot.Users, 2)
		require.Len(t, snapshot.Groups, 2)
		// 1 member in the everyone group + 1 member in the custom group
		require.Len(t, snapshot.GroupMembers, 2)
		require.Len(t, snapshot.Workspaces, 2)
		require.Len(t, snapshot.WorkspaceApps, 2)
		require.Len(t, snapshot.WorkspaceAgents, 2)
		require.Len(t, snapshot.WorkspaceBuilds, 2)
		require.Len(t, snapshot.WorkspaceResources, 2)
		require.Len(t, snapshot.WorkspaceAgentStats, 1)
		require.Len(t, snapshot.WorkspaceProxies, 1)
		require.Len(t, snapshot.WorkspaceModules, 1)
		require.Len(t, snapshot.Organizations, 1)
		// We create one item manually above. The other is TelemetryEnabled, created by the snapshotter.
		require.Len(t, snapshot.TelemetryItems, 2)
		require.Len(t, snapshot.WorkspaceAgentMemoryResourceMonitors, 1)
		require.Len(t, snapshot.WorkspaceAgentVolumeResourceMonitors, 1)
		wsa := snapshot.WorkspaceAgents[1]
		require.Len(t, wsa.Subsystems, 2)
		require.Equal(t, string(database.WorkspaceAgentSubsystemEnvbox), wsa.Subsystems[0])
		require.Equal(t, string(database.WorkspaceAgentSubsystemExectrace), wsa.Subsystems[1])
		require.Len(t, snapshot.Tasks, 1)
		for _, snapTask := range snapshot.Tasks {
			assert.Equal(t, task.ID.String(), snapTask.ID)
			assert.Equal(t, task.OrganizationID.String(), snapTask.OrganizationID)
			assert.Equal(t, task.OwnerID.String(), snapTask.OwnerID)
			assert.Equal(t, task.Name, snapTask.Name)
			if assert.True(t, task.WorkspaceID.Valid) {
				assert.Equal(t, task.WorkspaceID.UUID.String(), *snapTask.WorkspaceID)
			}
			assert.EqualValues(t, taskWA.WorkspaceBuildNumber, *snapTask.WorkspaceBuildNumber)
			assert.Equal(t, taskWA.WorkspaceAgentID.UUID.String(), *snapTask.WorkspaceAgentID)
			assert.Equal(t, taskWA.WorkspaceAppID.UUID.String(), *snapTask.WorkspaceAppID)
			assert.Equal(t, task.TemplateVersionID.String(), snapTask.TemplateVersionID)
			assert.Equal(t, "e196fe22e61cfa32d8c38749e0ce348108bb4cae29e2c36cdcce7e77faa9eb5f", snapTask.PromptHash)
			assert.Equal(t, task.CreatedAt.UTC(), snapTask.CreatedAt.UTC())
		}

		require.True(t, slices.ContainsFunc(snapshot.TemplateVersions, func(ttv telemetry.TemplateVersion) bool {
			if ttv.ID != taskTV.ID {
				return false
			}
			return assert.NotNil(t, ttv.HasAITask) && assert.True(t, *ttv.HasAITask)
		}))
		require.True(t, slices.ContainsFunc(snapshot.WorkspaceBuilds, func(twb telemetry.WorkspaceBuild) bool {
			if twb.ID != taskWB.ID {
				return false
			}
			return assert.NotNil(t, twb.HasAITask) && assert.True(t, *twb.HasAITask)
		}))

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
		require.Equal(t, snapshot1.Client, "unknown") // no client info yet
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
		require.Equal(t, snapshot2.Client, "unknown") // no client info yet
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

func TestTasksTelemetry(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitMedium)
	db, _ := dbtestutil.NewDB(t)

	// Define a fixed time for deterministic testing.
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)

	// Setup shared resources.
	org, err := db.GetDefaultOrganization(ctx)
	require.NoError(t, err)

	user := dbgen.User(t, db, database.User{})
	_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
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

	tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		OrganizationID: org.ID,
		TemplateID:     uuid.NullUUID{UUID: tpl.ID, Valid: true},
		CreatedBy:      user.ID,
		JobID:          job.ID,
		HasAITask:      sql.NullBool{Bool: true, Valid: true},
	})

	// Helper function to create a provisioner job for workspace builds.
	createJob := func() database.ProvisionerJob {
		return dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			Provisioner:    database.ProvisionerTypeTerraform,
			StorageMethod:  database.ProvisionerStorageMethodFile,
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			OrganizationID: org.ID,
		})
	}

	// Helper to create a workspace.
	createWorkspace := func() database.WorkspaceTable {
		return dbgen.Workspace(t, db, database.WorkspaceTable{
			OwnerID:        user.ID,
			OrganizationID: org.ID,
			TemplateID:     tpl.ID,
		})
	}

	// Helper to create workspace resource and agent for app status.
	createResourceAndAgent := func(jobID uuid.UUID) (database.WorkspaceResource, database.WorkspaceAgent) {
		res := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
			JobID: jobID,
		})
		agent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
			ResourceID: res.ID,
		})
		return res, agent
	}

	// Helper to create a workspace app.
	createApp := func(agentID uuid.UUID) database.WorkspaceApp {
		return dbgen.WorkspaceApp(t, db, database.WorkspaceApp{
			SharingLevel: database.AppSharingLevelOwner,
			Health:       database.WorkspaceAppHealthDisabled,
			OpenIn:       database.WorkspaceAppOpenInSlimWindow,
			AgentID:      agentID,
		})
	}

	// =====================================================
	// FIXTURE 1: pendingTask
	// No workspace, all lifecycle fields nil.
	// =====================================================
	pendingTask := dbgen.Task(t, db, database.TaskTable{
		OwnerID:           user.ID,
		OrganizationID:    org.ID,
		WorkspaceID:       uuid.NullUUID{}, // No workspace
		TemplateVersionID: tv.ID,
		Prompt:            "pending task prompt",
		CreatedAt:         now.Add(-1 * time.Hour),
	})

	// =====================================================
	// FIXTURE 2: runningTask
	// Workspace running, no pause/resume, all lifecycle fields nil.
	// =====================================================
	runningWs := createWorkspace()
	runningJob := createJob()
	runningBuild := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:       runningWs.ID,
		TemplateVersionID: tv.ID,
		JobID:             runningJob.ID,
		Transition:        database.WorkspaceTransitionStart,
		Reason:            database.BuildReasonInitiator,
		BuildNumber:       1,
		CreatedAt:         now.Add(-30 * time.Minute),
	})
	_, runningAgent := createResourceAndAgent(runningJob.ID)
	runningApp := createApp(runningAgent.ID)
	runningTask := dbgen.Task(t, db, database.TaskTable{
		OwnerID:           user.ID,
		OrganizationID:    org.ID,
		WorkspaceID:       uuid.NullUUID{UUID: runningWs.ID, Valid: true},
		TemplateVersionID: tv.ID,
		Prompt:            "running task prompt",
		CreatedAt:         now.Add(-45 * time.Minute),
	})
	_ = dbgen.TaskWorkspaceApp(t, db, database.TaskWorkspaceApp{
		TaskID:               runningTask.ID,
		WorkspaceAgentID:     uuid.NullUUID{Valid: true, UUID: runningAgent.ID},
		WorkspaceAppID:       uuid.NullUUID{Valid: true, UUID: runningApp.ID},
		WorkspaceBuildNumber: runningBuild.BuildNumber,
	})

	// =====================================================
	// FIXTURE 3: runningTaskWithStatus
	// Running with app status history → TimeToFirstStatusMS populated.
	// =====================================================
	statusWs := createWorkspace()
	statusJob := createJob()
	statusBuild := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:       statusWs.ID,
		TemplateVersionID: tv.ID,
		JobID:             statusJob.ID,
		Transition:        database.WorkspaceTransitionStart,
		Reason:            database.BuildReasonInitiator,
		BuildNumber:       1,
		CreatedAt:         now.Add(-2 * time.Hour),
	})
	_, statusAgent := createResourceAndAgent(statusJob.ID)
	statusApp := createApp(statusAgent.ID)
	taskCreatedAt := now.Add(-90 * time.Minute)
	firstStatusAt := now.Add(-85 * time.Minute) // 5 minutes after task creation
	runningTaskWithStatus := dbgen.Task(t, db, database.TaskTable{
		OwnerID:           user.ID,
		OrganizationID:    org.ID,
		WorkspaceID:       uuid.NullUUID{UUID: statusWs.ID, Valid: true},
		TemplateVersionID: tv.ID,
		Prompt:            "running task with status prompt",
		CreatedAt:         taskCreatedAt,
	})
	_ = dbgen.TaskWorkspaceApp(t, db, database.TaskWorkspaceApp{
		TaskID:               runningTaskWithStatus.ID,
		WorkspaceAgentID:     uuid.NullUUID{Valid: true, UUID: statusAgent.ID},
		WorkspaceAppID:       uuid.NullUUID{Valid: true, UUID: statusApp.ID},
		WorkspaceBuildNumber: statusBuild.BuildNumber,
	})
	// Insert first app status.
	_, err = db.InsertWorkspaceAppStatus(ctx, database.InsertWorkspaceAppStatusParams{
		ID:          uuid.New(),
		CreatedAt:   firstStatusAt,
		WorkspaceID: statusWs.ID,
		AgentID:     statusAgent.ID,
		AppID:       statusApp.ID,
		State:       database.WorkspaceAppStatusStateWorking,
		Message:     "Task started",
	})
	require.NoError(t, err)

	// =====================================================
	// FIXTURE 4: autoPausedTask
	// Stop build with task_auto_pause → LastPausedAt, PauseReason="auto".
	// =====================================================
	autoPauseWs := createWorkspace()
	autoPauseJob1 := createJob()
	_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:       autoPauseWs.ID,
		TemplateVersionID: tv.ID,
		JobID:             autoPauseJob1.ID,
		Transition:        database.WorkspaceTransitionStart,
		Reason:            database.BuildReasonInitiator,
		BuildNumber:       1,
		CreatedAt:         now.Add(-3 * time.Hour),
	})
	autoPauseJob2 := createJob()
	autoPauseBuildTime := now.Add(-20 * time.Minute)
	_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:       autoPauseWs.ID,
		TemplateVersionID: tv.ID,
		JobID:             autoPauseJob2.ID,
		Transition:        database.WorkspaceTransitionStop,
		Reason:            database.BuildReasonTaskAutoPause,
		BuildNumber:       2,
		CreatedAt:         autoPauseBuildTime,
	})
	_, autoPauseAgent := createResourceAndAgent(autoPauseJob1.ID)
	autoPauseApp := createApp(autoPauseAgent.ID)
	autoPausedTask := dbgen.Task(t, db, database.TaskTable{
		OwnerID:           user.ID,
		OrganizationID:    org.ID,
		WorkspaceID:       uuid.NullUUID{UUID: autoPauseWs.ID, Valid: true},
		TemplateVersionID: tv.ID,
		Prompt:            "auto paused task prompt",
		CreatedAt:         now.Add(-3 * time.Hour),
	})
	_ = dbgen.TaskWorkspaceApp(t, db, database.TaskWorkspaceApp{
		TaskID:               autoPausedTask.ID,
		WorkspaceAgentID:     uuid.NullUUID{Valid: true, UUID: autoPauseAgent.ID},
		WorkspaceAppID:       uuid.NullUUID{Valid: true, UUID: autoPauseApp.ID},
		WorkspaceBuildNumber: 1,
	})

	// =====================================================
	// FIXTURE 5: manuallyPausedTask
	// Stop build with task_manual_pause → LastPausedAt, PauseReason="manual".
	// =====================================================
	manualPauseWs := createWorkspace()
	manualPauseJob1 := createJob()
	_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:       manualPauseWs.ID,
		TemplateVersionID: tv.ID,
		JobID:             manualPauseJob1.ID,
		Transition:        database.WorkspaceTransitionStart,
		Reason:            database.BuildReasonInitiator,
		BuildNumber:       1,
		CreatedAt:         now.Add(-4 * time.Hour),
	})
	manualPauseJob2 := createJob()
	manualPauseBuildTime := now.Add(-15 * time.Minute)
	_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:       manualPauseWs.ID,
		TemplateVersionID: tv.ID,
		JobID:             manualPauseJob2.ID,
		Transition:        database.WorkspaceTransitionStop,
		Reason:            database.BuildReasonTaskManualPause,
		BuildNumber:       2,
		CreatedAt:         manualPauseBuildTime,
	})
	_, manualPauseAgent := createResourceAndAgent(manualPauseJob1.ID)
	manualPauseApp := createApp(manualPauseAgent.ID)
	manuallyPausedTask := dbgen.Task(t, db, database.TaskTable{
		OwnerID:           user.ID,
		OrganizationID:    org.ID,
		WorkspaceID:       uuid.NullUUID{UUID: manualPauseWs.ID, Valid: true},
		TemplateVersionID: tv.ID,
		Prompt:            "manually paused task prompt",
		CreatedAt:         now.Add(-4 * time.Hour),
	})
	_ = dbgen.TaskWorkspaceApp(t, db, database.TaskWorkspaceApp{
		TaskID:               manuallyPausedTask.ID,
		WorkspaceAgentID:     uuid.NullUUID{Valid: true, UUID: manualPauseAgent.ID},
		WorkspaceAppID:       uuid.NullUUID{Valid: true, UUID: manualPauseApp.ID},
		WorkspaceBuildNumber: 1,
	})

	// =====================================================
	// FIXTURE 6: pausedWithIdleTime
	// Auto-paused with working status before → IdleDurationMS calculated.
	// =====================================================
	idleTimeWs := createWorkspace()
	idleTimeJob1 := createJob()
	_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:       idleTimeWs.ID,
		TemplateVersionID: tv.ID,
		JobID:             idleTimeJob1.ID,
		Transition:        database.WorkspaceTransitionStart,
		Reason:            database.BuildReasonInitiator,
		BuildNumber:       1,
		CreatedAt:         now.Add(-5 * time.Hour),
	})
	_, idleTimeAgent := createResourceAndAgent(idleTimeJob1.ID)
	idleTimeApp := createApp(idleTimeAgent.ID)
	idleTimePausedTask := dbgen.Task(t, db, database.TaskTable{
		OwnerID:           user.ID,
		OrganizationID:    org.ID,
		WorkspaceID:       uuid.NullUUID{UUID: idleTimeWs.ID, Valid: true},
		TemplateVersionID: tv.ID,
		Prompt:            "paused with idle time prompt",
		CreatedAt:         now.Add(-5 * time.Hour),
	})
	_ = dbgen.TaskWorkspaceApp(t, db, database.TaskWorkspaceApp{
		TaskID:               idleTimePausedTask.ID,
		WorkspaceAgentID:     uuid.NullUUID{Valid: true, UUID: idleTimeAgent.ID},
		WorkspaceAppID:       uuid.NullUUID{Valid: true, UUID: idleTimeApp.ID},
		WorkspaceBuildNumber: 1,
	})
	// Insert working status at -40 minutes.
	lastWorkingStatusTime := now.Add(-40 * time.Minute)
	_, err = db.InsertWorkspaceAppStatus(ctx, database.InsertWorkspaceAppStatusParams{
		ID:          uuid.New(),
		CreatedAt:   lastWorkingStatusTime,
		WorkspaceID: idleTimeWs.ID,
		AgentID:     idleTimeAgent.ID,
		AppID:       idleTimeApp.ID,
		State:       database.WorkspaceAppStatusStateWorking,
		Message:     "Working on something",
	})
	require.NoError(t, err)
	// Insert idle status at -35 minutes (5 minutes after working).
	_, err = db.InsertWorkspaceAppStatus(ctx, database.InsertWorkspaceAppStatusParams{
		ID:          uuid.New(),
		CreatedAt:   now.Add(-35 * time.Minute),
		WorkspaceID: idleTimeWs.ID,
		AgentID:     idleTimeAgent.ID,
		AppID:       idleTimeApp.ID,
		State:       database.WorkspaceAppStatusStateIdle,
		Message:     "Idle now",
	})
	require.NoError(t, err)
	// Pause at -25 minutes (10 minutes after idle, 15 minutes after last working).
	idleTimePauseBuildTime := now.Add(-25 * time.Minute)
	idleTimeJob2 := createJob()
	_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:       idleTimeWs.ID,
		TemplateVersionID: tv.ID,
		JobID:             idleTimeJob2.ID,
		Transition:        database.WorkspaceTransitionStop,
		Reason:            database.BuildReasonTaskAutoPause,
		BuildNumber:       2,
		CreatedAt:         idleTimePauseBuildTime,
	})

	// =====================================================
	// FIXTURE 7: recentlyResumedTask
	// Paused then resumed < 1hr ago → LastResumedAt, PausedDurationMS.
	// =====================================================
	recentResumeWs := createWorkspace()
	recentResumeJob1 := createJob()
	_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:       recentResumeWs.ID,
		TemplateVersionID: tv.ID,
		JobID:             recentResumeJob1.ID,
		Transition:        database.WorkspaceTransitionStart,
		Reason:            database.BuildReasonInitiator,
		BuildNumber:       1,
		CreatedAt:         now.Add(-6 * time.Hour),
	})
	// Pause at -50 minutes.
	recentResumePauseTime := now.Add(-50 * time.Minute)
	recentResumeJob2 := createJob()
	_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:       recentResumeWs.ID,
		TemplateVersionID: tv.ID,
		JobID:             recentResumeJob2.ID,
		Transition:        database.WorkspaceTransitionStop,
		Reason:            database.BuildReasonTaskAutoPause,
		BuildNumber:       2,
		CreatedAt:         recentResumePauseTime,
	})
	// Resume at -10 minutes (40 minutes of paused time).
	recentResumeTime := now.Add(-10 * time.Minute)
	recentResumeJob3 := createJob()
	_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:       recentResumeWs.ID,
		TemplateVersionID: tv.ID,
		JobID:             recentResumeJob3.ID,
		Transition:        database.WorkspaceTransitionStart,
		Reason:            database.BuildReasonTaskResume,
		BuildNumber:       3,
		CreatedAt:         recentResumeTime,
	})
	_, recentResumeAgent := createResourceAndAgent(recentResumeJob1.ID)
	recentResumeApp := createApp(recentResumeAgent.ID)
	recentlyResumedTask := dbgen.Task(t, db, database.TaskTable{
		OwnerID:           user.ID,
		OrganizationID:    org.ID,
		WorkspaceID:       uuid.NullUUID{UUID: recentResumeWs.ID, Valid: true},
		TemplateVersionID: tv.ID,
		Prompt:            "recently resumed task prompt",
		CreatedAt:         now.Add(-6 * time.Hour),
	})
	_ = dbgen.TaskWorkspaceApp(t, db, database.TaskWorkspaceApp{
		TaskID:               recentlyResumedTask.ID,
		WorkspaceAgentID:     uuid.NullUUID{Valid: true, UUID: recentResumeAgent.ID},
		WorkspaceAppID:       uuid.NullUUID{Valid: true, UUID: recentResumeApp.ID},
		WorkspaceBuildNumber: 1,
	})

	// =====================================================
	// FIXTURE 8: multiplePauseResumeCycles
	// pause1 → resume1 → pause2 → captures latest of each.
	// =====================================================
	multiCycleWs := createWorkspace()
	multiCycleJob1 := createJob()
	_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:       multiCycleWs.ID,
		TemplateVersionID: tv.ID,
		JobID:             multiCycleJob1.ID,
		Transition:        database.WorkspaceTransitionStart,
		Reason:            database.BuildReasonInitiator,
		BuildNumber:       1,
		CreatedAt:         now.Add(-8 * time.Hour),
	})
	// First pause at -3 hours (auto).
	multiCycleJob2 := createJob()
	_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:       multiCycleWs.ID,
		TemplateVersionID: tv.ID,
		JobID:             multiCycleJob2.ID,
		Transition:        database.WorkspaceTransitionStop,
		Reason:            database.BuildReasonTaskAutoPause,
		BuildNumber:       2,
		CreatedAt:         now.Add(-3 * time.Hour),
	})
	// First resume at -2.5 hours.
	multiCycleJob3 := createJob()
	_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:       multiCycleWs.ID,
		TemplateVersionID: tv.ID,
		JobID:             multiCycleJob3.ID,
		Transition:        database.WorkspaceTransitionStart,
		Reason:            database.BuildReasonTaskResume,
		BuildNumber:       3,
		CreatedAt:         now.Add(-150 * time.Minute), // -2.5 hours
	})
	// Second pause at -30 minutes (manual).
	multiCycleLatestPauseTime := now.Add(-30 * time.Minute)
	multiCycleJob4 := createJob()
	_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:       multiCycleWs.ID,
		TemplateVersionID: tv.ID,
		JobID:             multiCycleJob4.ID,
		Transition:        database.WorkspaceTransitionStop,
		Reason:            database.BuildReasonTaskManualPause,
		BuildNumber:       4,
		CreatedAt:         multiCycleLatestPauseTime,
	})
	_, multiCycleAgent := createResourceAndAgent(multiCycleJob1.ID)
	multiCycleApp := createApp(multiCycleAgent.ID)
	multiplePauseResumeCyclesTask := dbgen.Task(t, db, database.TaskTable{
		OwnerID:           user.ID,
		OrganizationID:    org.ID,
		WorkspaceID:       uuid.NullUUID{UUID: multiCycleWs.ID, Valid: true},
		TemplateVersionID: tv.ID,
		Prompt:            "multiple pause resume cycles prompt",
		CreatedAt:         now.Add(-8 * time.Hour),
	})
	_ = dbgen.TaskWorkspaceApp(t, db, database.TaskWorkspaceApp{
		TaskID:               multiplePauseResumeCyclesTask.ID,
		WorkspaceAgentID:     uuid.NullUUID{Valid: true, UUID: multiCycleAgent.ID},
		WorkspaceAppID:       uuid.NullUUID{Valid: true, UUID: multiCycleApp.ID},
		WorkspaceBuildNumber: 1,
	})

	// =====================================================
	// FIXTURE 9: resumedLongAgo
	// Resumed > 1hr ago → LastResumedAt set, PausedDurationMS = nil.
	// =====================================================
	longAgoWs := createWorkspace()
	longAgoJob1 := createJob()
	_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:       longAgoWs.ID,
		TemplateVersionID: tv.ID,
		JobID:             longAgoJob1.ID,
		Transition:        database.WorkspaceTransitionStart,
		Reason:            database.BuildReasonInitiator,
		BuildNumber:       1,
		CreatedAt:         now.Add(-10 * time.Hour),
	})
	// Pause at -5 hours.
	longAgoPauseTime := now.Add(-5 * time.Hour)
	longAgoJob2 := createJob()
	_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:       longAgoWs.ID,
		TemplateVersionID: tv.ID,
		JobID:             longAgoJob2.ID,
		Transition:        database.WorkspaceTransitionStop,
		Reason:            database.BuildReasonTaskAutoPause,
		BuildNumber:       2,
		CreatedAt:         longAgoPauseTime,
	})
	// Resume at -2 hours (> 1hr ago, so PausedDurationMS should be nil).
	longAgoResumeTime := now.Add(-2 * time.Hour)
	longAgoJob3 := createJob()
	_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:       longAgoWs.ID,
		TemplateVersionID: tv.ID,
		JobID:             longAgoJob3.ID,
		Transition:        database.WorkspaceTransitionStart,
		Reason:            database.BuildReasonTaskResume,
		BuildNumber:       3,
		CreatedAt:         longAgoResumeTime,
	})
	_, longAgoAgent := createResourceAndAgent(longAgoJob1.ID)
	longAgoApp := createApp(longAgoAgent.ID)
	resumedLongAgoTask := dbgen.Task(t, db, database.TaskTable{
		OwnerID:           user.ID,
		OrganizationID:    org.ID,
		WorkspaceID:       uuid.NullUUID{UUID: longAgoWs.ID, Valid: true},
		TemplateVersionID: tv.ID,
		Prompt:            "resumed long ago task prompt",
		CreatedAt:         now.Add(-10 * time.Hour),
	})
	_ = dbgen.TaskWorkspaceApp(t, db, database.TaskWorkspaceApp{
		TaskID:               resumedLongAgoTask.ID,
		WorkspaceAgentID:     uuid.NullUUID{Valid: true, UUID: longAgoAgent.ID},
		WorkspaceAppID:       uuid.NullUUID{Valid: true, UUID: longAgoApp.ID},
		WorkspaceBuildNumber: 1,
	})

	// =====================================================
	// FIXTURE 10: taskWithAllFields
	// Full lifecycle with ALL fields populated.
	// =====================================================
	allFieldsWs := createWorkspace()
	allFieldsJob1 := createJob()
	_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:       allFieldsWs.ID,
		TemplateVersionID: tv.ID,
		JobID:             allFieldsJob1.ID,
		Transition:        database.WorkspaceTransitionStart,
		Reason:            database.BuildReasonInitiator,
		BuildNumber:       1,
		CreatedAt:         now.Add(-7 * time.Hour),
	})
	_, allFieldsAgent := createResourceAndAgent(allFieldsJob1.ID)
	allFieldsApp := createApp(allFieldsAgent.ID)
	allFieldsTaskCreatedAt := now.Add(-7 * time.Hour)
	taskWithAllFields := dbgen.Task(t, db, database.TaskTable{
		OwnerID:           user.ID,
		OrganizationID:    org.ID,
		WorkspaceID:       uuid.NullUUID{UUID: allFieldsWs.ID, Valid: true},
		TemplateVersionID: tv.ID,
		Prompt:            "task with all fields prompt",
		CreatedAt:         allFieldsTaskCreatedAt,
	})
	_ = dbgen.TaskWorkspaceApp(t, db, database.TaskWorkspaceApp{
		TaskID:               taskWithAllFields.ID,
		WorkspaceAgentID:     uuid.NullUUID{Valid: true, UUID: allFieldsAgent.ID},
		WorkspaceAppID:       uuid.NullUUID{Valid: true, UUID: allFieldsApp.ID},
		WorkspaceBuildNumber: 1,
	})
	// First status at -6.5 hours (30 minutes after creation).
	allFieldsFirstStatusTime := now.Add(-390 * time.Minute) // -6.5 hours
	_, err = db.InsertWorkspaceAppStatus(ctx, database.InsertWorkspaceAppStatusParams{
		ID:          uuid.New(),
		CreatedAt:   allFieldsFirstStatusTime,
		WorkspaceID: allFieldsWs.ID,
		AgentID:     allFieldsAgent.ID,
		AppID:       allFieldsApp.ID,
		State:       database.WorkspaceAppStatusStateWorking,
		Message:     "Started working",
	})
	require.NoError(t, err)
	// Last working status at -45 minutes.
	allFieldsLastWorkingTime := now.Add(-45 * time.Minute)
	_, err = db.InsertWorkspaceAppStatus(ctx, database.InsertWorkspaceAppStatusParams{
		ID:          uuid.New(),
		CreatedAt:   allFieldsLastWorkingTime,
		WorkspaceID: allFieldsWs.ID,
		AgentID:     allFieldsAgent.ID,
		AppID:       allFieldsApp.ID,
		State:       database.WorkspaceAppStatusStateWorking,
		Message:     "Still working",
	})
	require.NoError(t, err)
	// Pause at -35 minutes (10 minutes idle duration).
	allFieldsPauseTime := now.Add(-35 * time.Minute)
	allFieldsJob2 := createJob()
	_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:       allFieldsWs.ID,
		TemplateVersionID: tv.ID,
		JobID:             allFieldsJob2.ID,
		Transition:        database.WorkspaceTransitionStop,
		Reason:            database.BuildReasonTaskAutoPause,
		BuildNumber:       2,
		CreatedAt:         allFieldsPauseTime,
	})
	// Resume at -5 minutes (30 minutes paused duration).
	allFieldsResumeTime := now.Add(-5 * time.Minute)
	allFieldsJob3 := createJob()
	_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:       allFieldsWs.ID,
		TemplateVersionID: tv.ID,
		JobID:             allFieldsJob3.ID,
		Transition:        database.WorkspaceTransitionStart,
		Reason:            database.BuildReasonTaskResume,
		BuildNumber:       3,
		CreatedAt:         allFieldsResumeTime,
	})

	// =====================================================
	// FIXTURE 11: deletedTask
	// Soft-deleted task → NOT in results.
	// =====================================================
	deletedTaskWs := createWorkspace()
	deletedTaskJob := createJob()
	_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:       deletedTaskWs.ID,
		TemplateVersionID: tv.ID,
		JobID:             deletedTaskJob.ID,
		Transition:        database.WorkspaceTransitionStart,
		Reason:            database.BuildReasonInitiator,
		BuildNumber:       1,
		CreatedAt:         now.Add(-1 * time.Hour),
	})
	_, deletedTaskAgent := createResourceAndAgent(deletedTaskJob.ID)
	deletedTaskApp := createApp(deletedTaskAgent.ID)
	deletedTask := dbgen.Task(t, db, database.TaskTable{
		OwnerID:           user.ID,
		OrganizationID:    org.ID,
		WorkspaceID:       uuid.NullUUID{UUID: deletedTaskWs.ID, Valid: true},
		TemplateVersionID: tv.ID,
		Prompt:            "deleted task prompt",
		CreatedAt:         now.Add(-1 * time.Hour),
	})
	_ = dbgen.TaskWorkspaceApp(t, db, database.TaskWorkspaceApp{
		TaskID:               deletedTask.ID,
		WorkspaceAgentID:     uuid.NullUUID{Valid: true, UUID: deletedTaskAgent.ID},
		WorkspaceAppID:       uuid.NullUUID{Valid: true, UUID: deletedTaskApp.ID},
		WorkspaceBuildNumber: 1,
	})
	// Soft delete the task.
	_, err = db.DeleteTask(ctx, database.DeleteTaskParams{
		DeletedAt: now,
		ID:        deletedTask.ID,
	})
	require.NoError(t, err)

	// =====================================================
	// Re-fetch tasks to get computed status from the view.
	// The status is computed from workspace build state, so we need fresh data.
	// =====================================================
	pendingTask, err = db.GetTaskByID(ctx, pendingTask.ID)
	require.NoError(t, err)
	runningTask, err = db.GetTaskByID(ctx, runningTask.ID)
	require.NoError(t, err)
	runningTaskWithStatus, err = db.GetTaskByID(ctx, runningTaskWithStatus.ID)
	require.NoError(t, err)
	autoPausedTask, err = db.GetTaskByID(ctx, autoPausedTask.ID)
	require.NoError(t, err)
	manuallyPausedTask, err = db.GetTaskByID(ctx, manuallyPausedTask.ID)
	require.NoError(t, err)
	idleTimePausedTask, err = db.GetTaskByID(ctx, idleTimePausedTask.ID)
	require.NoError(t, err)
	recentlyResumedTask, err = db.GetTaskByID(ctx, recentlyResumedTask.ID)
	require.NoError(t, err)
	multiplePauseResumeCyclesTask, err = db.GetTaskByID(ctx, multiplePauseResumeCyclesTask.ID)
	require.NoError(t, err)
	resumedLongAgoTask, err = db.GetTaskByID(ctx, resumedLongAgoTask.ID)
	require.NoError(t, err)
	taskWithAllFields, err = db.GetTaskByID(ctx, taskWithAllFields.ID)
	require.NoError(t, err)

	// =====================================================
	// Build expected results.
	// =====================================================
	expected := []telemetry.Task{
		// Fixture 1: pendingTask - no workspace, all lifecycle fields nil.
		{
			ID:                   pendingTask.ID.String(),
			OrganizationID:       org.ID.String(),
			OwnerID:              user.ID.String(),
			Name:                 pendingTask.Name,
			WorkspaceID:          nil,
			WorkspaceBuildNumber: nil,
			WorkspaceAgentID:     nil,
			WorkspaceAppID:       nil,
			TemplateVersionID:    tv.ID.String(),
			PromptHash:           telemetry.HashContent(pendingTask.Prompt),
			CreatedAt:            pendingTask.CreatedAt,
			Status:               string(pendingTask.Status),
			LastPausedAt:         nil,
			LastResumedAt:        nil,
			PauseReason:          nil,
			IdleDurationMS:       nil,
			PausedDurationMS:     nil,
			TimeToFirstStatusMS:  nil,
		},
		// Fixture 2: runningTask - workspace running, all lifecycle fields nil.
		{
			ID:                   runningTask.ID.String(),
			OrganizationID:       org.ID.String(),
			OwnerID:              user.ID.String(),
			Name:                 runningTask.Name,
			WorkspaceID:          ptr.Ref(runningWs.ID.String()),
			WorkspaceBuildNumber: ptr.Ref(int64(runningBuild.BuildNumber)),
			WorkspaceAgentID:     ptr.Ref(runningAgent.ID.String()),
			WorkspaceAppID:       ptr.Ref(runningApp.ID.String()),
			TemplateVersionID:    tv.ID.String(),
			PromptHash:           telemetry.HashContent(runningTask.Prompt),
			CreatedAt:            runningTask.CreatedAt,
			Status:               string(runningTask.Status),
			LastPausedAt:         nil,
			LastResumedAt:        nil,
			PauseReason:          nil,
			IdleDurationMS:       nil,
			PausedDurationMS:     nil,
			TimeToFirstStatusMS:  nil,
		},
		// Fixture 3: runningTaskWithStatus - TimeToFirstStatusMS populated.
		{
			ID:                   runningTaskWithStatus.ID.String(),
			OrganizationID:       org.ID.String(),
			OwnerID:              user.ID.String(),
			Name:                 runningTaskWithStatus.Name,
			WorkspaceID:          ptr.Ref(statusWs.ID.String()),
			WorkspaceBuildNumber: ptr.Ref(int64(statusBuild.BuildNumber)),
			WorkspaceAgentID:     ptr.Ref(statusAgent.ID.String()),
			WorkspaceAppID:       ptr.Ref(statusApp.ID.String()),
			TemplateVersionID:    tv.ID.String(),
			PromptHash:           telemetry.HashContent(runningTaskWithStatus.Prompt),
			CreatedAt:            runningTaskWithStatus.CreatedAt,
			Status:               string(runningTaskWithStatus.Status),
			LastPausedAt:         nil,
			LastResumedAt:        nil,
			PauseReason:          nil,
			IdleDurationMS:       nil,
			PausedDurationMS:     nil,
			TimeToFirstStatusMS:  ptr.Ref(int64(5 * 60 * 1000)), // 5 minutes in ms
		},
		// Fixture 4: autoPausedTask - LastPausedAt, PauseReason="auto".
		{
			ID:                   autoPausedTask.ID.String(),
			OrganizationID:       org.ID.String(),
			OwnerID:              user.ID.String(),
			Name:                 autoPausedTask.Name,
			WorkspaceID:          ptr.Ref(autoPauseWs.ID.String()),
			WorkspaceBuildNumber: ptr.Ref(int64(1)),
			WorkspaceAgentID:     ptr.Ref(autoPauseAgent.ID.String()),
			WorkspaceAppID:       ptr.Ref(autoPauseApp.ID.String()),
			TemplateVersionID:    tv.ID.String(),
			PromptHash:           telemetry.HashContent(autoPausedTask.Prompt),
			CreatedAt:            autoPausedTask.CreatedAt,
			Status:               string(autoPausedTask.Status),
			LastPausedAt:         &autoPauseBuildTime,
			LastResumedAt:        nil,
			PauseReason:          ptr.Ref("auto"),
			IdleDurationMS:       nil,
			PausedDurationMS:     nil,
			TimeToFirstStatusMS:  nil,
		},
		// Fixture 5: manuallyPausedTask - LastPausedAt, PauseReason="manual".
		{
			ID:                   manuallyPausedTask.ID.String(),
			OrganizationID:       org.ID.String(),
			OwnerID:              user.ID.String(),
			Name:                 manuallyPausedTask.Name,
			WorkspaceID:          ptr.Ref(manualPauseWs.ID.String()),
			WorkspaceBuildNumber: ptr.Ref(int64(1)),
			WorkspaceAgentID:     ptr.Ref(manualPauseAgent.ID.String()),
			WorkspaceAppID:       ptr.Ref(manualPauseApp.ID.String()),
			TemplateVersionID:    tv.ID.String(),
			PromptHash:           telemetry.HashContent(manuallyPausedTask.Prompt),
			CreatedAt:            manuallyPausedTask.CreatedAt,
			Status:               string(manuallyPausedTask.Status),
			LastPausedAt:         &manualPauseBuildTime,
			LastResumedAt:        nil,
			PauseReason:          ptr.Ref("manual"),
			IdleDurationMS:       nil,
			PausedDurationMS:     nil,
			TimeToFirstStatusMS:  nil,
		},
		// Fixture 6: pausedWithIdleTime - IdleDurationMS calculated.
		// Idle duration = pause time - last working status time = 15 minutes.
		{
			ID:                   idleTimePausedTask.ID.String(),
			OrganizationID:       org.ID.String(),
			OwnerID:              user.ID.String(),
			Name:                 idleTimePausedTask.Name,
			WorkspaceID:          ptr.Ref(idleTimeWs.ID.String()),
			WorkspaceBuildNumber: ptr.Ref(int64(1)),
			WorkspaceAgentID:     ptr.Ref(idleTimeAgent.ID.String()),
			WorkspaceAppID:       ptr.Ref(idleTimeApp.ID.String()),
			TemplateVersionID:    tv.ID.String(),
			PromptHash:           telemetry.HashContent(idleTimePausedTask.Prompt),
			CreatedAt:            idleTimePausedTask.CreatedAt,
			Status:               string(idleTimePausedTask.Status),
			LastPausedAt:         &idleTimePauseBuildTime,
			LastResumedAt:        nil,
			PauseReason:          ptr.Ref("auto"),
			IdleDurationMS:       ptr.Ref(int64(15 * 60 * 1000)), // 15 minutes in ms
			PausedDurationMS:     nil,
			TimeToFirstStatusMS:  ptr.Ref(int64(260 * 60 * 1000)), // -5hr to -40min = 260 min
		},
		// Fixture 7: recentlyResumedTask - LastResumedAt, PausedDurationMS.
		// Paused duration = resume time - pause time = 40 minutes.
		{
			ID:                   recentlyResumedTask.ID.String(),
			OrganizationID:       org.ID.String(),
			OwnerID:              user.ID.String(),
			Name:                 recentlyResumedTask.Name,
			WorkspaceID:          ptr.Ref(recentResumeWs.ID.String()),
			WorkspaceBuildNumber: ptr.Ref(int64(1)),
			WorkspaceAgentID:     ptr.Ref(recentResumeAgent.ID.String()),
			WorkspaceAppID:       ptr.Ref(recentResumeApp.ID.String()),
			TemplateVersionID:    tv.ID.String(),
			PromptHash:           telemetry.HashContent(recentlyResumedTask.Prompt),
			CreatedAt:            recentlyResumedTask.CreatedAt,
			Status:               string(recentlyResumedTask.Status),
			LastPausedAt:         &recentResumePauseTime,
			LastResumedAt:        &recentResumeTime,
			PauseReason:          ptr.Ref("auto"),
			IdleDurationMS:       nil,
			PausedDurationMS:     ptr.Ref(int64(40 * 60 * 1000)), // 40 minutes in ms
			TimeToFirstStatusMS:  nil,
		},
		// Fixture 8: multiplePauseResumeCycles - captures latest pause/resume.
		// Latest pause is manual at -30 minutes, latest resume is at -2.5 hours.
		{
			ID:                   multiplePauseResumeCyclesTask.ID.String(),
			OrganizationID:       org.ID.String(),
			OwnerID:              user.ID.String(),
			Name:                 multiplePauseResumeCyclesTask.Name,
			WorkspaceID:          ptr.Ref(multiCycleWs.ID.String()),
			WorkspaceBuildNumber: ptr.Ref(int64(1)),
			WorkspaceAgentID:     ptr.Ref(multiCycleAgent.ID.String()),
			WorkspaceAppID:       ptr.Ref(multiCycleApp.ID.String()),
			TemplateVersionID:    tv.ID.String(),
			PromptHash:           telemetry.HashContent(multiplePauseResumeCyclesTask.Prompt),
			CreatedAt:            multiplePauseResumeCyclesTask.CreatedAt,
			Status:               string(multiplePauseResumeCyclesTask.Status),
			LastPausedAt:         &multiCycleLatestPauseTime,
			LastResumedAt:        ptr.Ref(now.Add(-150 * time.Minute)), // -2.5 hours
			PauseReason:          ptr.Ref("manual"),                    // Latest pause reason
			IdleDurationMS:       nil,
			PausedDurationMS:     nil, // Resume was > 1hr ago
			TimeToFirstStatusMS:  nil,
		},
		// Fixture 9: resumedLongAgo - LastResumedAt set, PausedDurationMS = nil.
		{
			ID:                   resumedLongAgoTask.ID.String(),
			OrganizationID:       org.ID.String(),
			OwnerID:              user.ID.String(),
			Name:                 resumedLongAgoTask.Name,
			WorkspaceID:          ptr.Ref(longAgoWs.ID.String()),
			WorkspaceBuildNumber: ptr.Ref(int64(1)),
			WorkspaceAgentID:     ptr.Ref(longAgoAgent.ID.String()),
			WorkspaceAppID:       ptr.Ref(longAgoApp.ID.String()),
			TemplateVersionID:    tv.ID.String(),
			PromptHash:           telemetry.HashContent(resumedLongAgoTask.Prompt),
			CreatedAt:            resumedLongAgoTask.CreatedAt,
			Status:               string(resumedLongAgoTask.Status),
			LastPausedAt:         &longAgoPauseTime,
			LastResumedAt:        &longAgoResumeTime,
			PauseReason:          ptr.Ref("auto"),
			IdleDurationMS:       nil,
			PausedDurationMS:     nil, // Resume was > 1hr ago
			TimeToFirstStatusMS:  nil,
		},
		// Fixture 10: taskWithAllFields - all fields populated.
		{
			ID:                   taskWithAllFields.ID.String(),
			OrganizationID:       org.ID.String(),
			OwnerID:              user.ID.String(),
			Name:                 taskWithAllFields.Name,
			WorkspaceID:          ptr.Ref(allFieldsWs.ID.String()),
			WorkspaceBuildNumber: ptr.Ref(int64(1)),
			WorkspaceAgentID:     ptr.Ref(allFieldsAgent.ID.String()),
			WorkspaceAppID:       ptr.Ref(allFieldsApp.ID.String()),
			TemplateVersionID:    tv.ID.String(),
			PromptHash:           telemetry.HashContent(taskWithAllFields.Prompt),
			CreatedAt:            taskWithAllFields.CreatedAt,
			Status:               string(taskWithAllFields.Status),
			LastPausedAt:         &allFieldsPauseTime,
			LastResumedAt:        &allFieldsResumeTime,
			PauseReason:          ptr.Ref("auto"),
			IdleDurationMS:       ptr.Ref(int64(10 * 60 * 1000)), // 10 minutes in ms
			PausedDurationMS:     ptr.Ref(int64(30 * 60 * 1000)), // 30 minutes in ms
			TimeToFirstStatusMS:  ptr.Ref(int64(30 * 60 * 1000)), // 30 minutes in ms
		},
		// Note: deletedTask (Fixture 11) should NOT appear in results.
	}

	// Sort expected by ID for deterministic comparison.
	slices.SortFunc(expected, func(a, b telemetry.Task) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})

	// Call CollectTasks.
	createdAfter := now.Add(-1 * time.Hour)
	actual, err := telemetry.CollectTasks(ctx, db, createdAfter)
	require.NoError(t, err, "unexpected error collecting tasks telemetry")

	// Sort actual by ID for deterministic comparison.
	slices.SortFunc(actual, func(a, b telemetry.Task) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})

	if diff := cmp.Diff(expected, actual); diff != "" {
		t.Fatalf("unexpected diff (-want +got):\n%s", diff)
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

			for i := 0; i < 3; i++ {
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

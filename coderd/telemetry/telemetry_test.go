package telemetry_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
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
	"github.com/coder/coder/v2/coderd/database/dbfake"
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
		require.Empty(t, snapshot.TaskEvents)
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

// taskTelemetryHelper is a grab bag of stuff useful in task telemetry test cases
type taskTelemetryHelper struct {
	t    *testing.T
	ctx  context.Context
	db   database.Store
	org  database.Organization
	user database.User
}

// nolint: dupl // Test code is better WET than DRY.
func TestTasksTelemetry(t *testing.T) {
	t.Parallel()

	// Define a fixed reference time for deterministic testing.
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)

	createAppStatus := func(ctx context.Context, db database.Store, wsID uuid.UUID, agentID, appID uuid.UUID, state database.WorkspaceAppStatusState, message string, createdAt time.Time) {
		_, err := db.InsertWorkspaceAppStatus(ctx, database.InsertWorkspaceAppStatusParams{
			ID:          uuid.New(),
			CreatedAt:   createdAt,
			WorkspaceID: wsID,
			AgentID:     agentID,
			AppID:       appID,
			State:       state,
			Message:     message,
		})
		require.NoError(t, err)
	}

	getApp := func(ctx context.Context, db database.Store, agentID uuid.UUID) database.WorkspaceApp {
		apps, err := db.GetWorkspaceAppsByAgentID(ctx, agentID)
		require.NoError(t, err)
		require.NotEmpty(t, apps, "expected at least one app")
		return apps[0]
	}

	tests := []struct {
		name  string
		setup func(t *testing.T, h *taskTelemetryHelper, now time.Time) (telemetry.Task, *telemetry.TaskEvent)
	}{
		{
			name: "no workspace - all lifecycle fields nil",
			setup: func(t *testing.T, h *taskTelemetryHelper, now time.Time) (telemetry.Task, *telemetry.TaskEvent) {
				tv := dbgen.TemplateVersion(t, h.db, database.TemplateVersion{
					OrganizationID: h.org.ID,
					CreatedBy:      h.user.ID,
					HasAITask:      sql.NullBool{Bool: true, Valid: true},
				})
				task := dbgen.Task(h.t, h.db, database.TaskTable{
					OwnerID:           h.user.ID,
					OrganizationID:    h.org.ID,
					WorkspaceID:       uuid.NullUUID{},
					TemplateVersionID: tv.ID,
					Prompt:            "pending task prompt",
					CreatedAt:         now.Add(-1 * time.Hour),
				})

				expected := telemetry.Task{
					ID:                   task.ID.String(),
					OrganizationID:       h.org.ID.String(),
					OwnerID:              h.user.ID.String(),
					Name:                 task.Name,
					WorkspaceID:          nil,
					WorkspaceBuildNumber: nil,
					WorkspaceAgentID:     nil,
					WorkspaceAppID:       nil,
					TemplateVersionID:    tv.ID.String(),
					PromptHash:           telemetry.HashContent(task.Prompt),
					CreatedAt:            task.CreatedAt,
					TimeToFirstStatusMS:  nil,
				}
				return expected, nil
			},
		},
		{
			name: "running workspace - no pause/resume events",
			setup: func(t *testing.T, h *taskTelemetryHelper, now time.Time) (telemetry.Task, *telemetry.TaskEvent) {
				resp := dbfake.WorkspaceBuild(h.t, h.db, database.WorkspaceTable{
					OrganizationID: h.org.ID,
					OwnerID:        h.user.ID,
				}).WithTask(database.TaskTable{
					Prompt:    "running task prompt",
					CreatedAt: now.Add(-45 * time.Minute),
				}, nil).Seed(database.WorkspaceBuild{
					Transition:  database.WorkspaceTransitionStart,
					Reason:      database.BuildReasonInitiator,
					BuildNumber: 1,
					CreatedAt:   now.Add(-30 * time.Minute),
				}).Succeeded().Do()

				app := getApp(h.ctx, h.db, resp.Agents[0].ID)

				expected := telemetry.Task{
					ID:                   resp.Task.ID.String(),
					OrganizationID:       h.org.ID.String(),
					OwnerID:              h.user.ID.String(),
					Name:                 resp.Task.Name,
					WorkspaceID:          ptr.Ref(resp.Workspace.ID.String()),
					WorkspaceBuildNumber: ptr.Ref(int64(1)),
					WorkspaceAgentID:     ptr.Ref(resp.Agents[0].ID.String()),
					WorkspaceAppID:       ptr.Ref(app.ID.String()),
					TemplateVersionID:    resp.TemplateVersion.ID.String(),
					PromptHash:           telemetry.HashContent(resp.Task.Prompt),
					CreatedAt:            resp.Task.CreatedAt,
					TimeToFirstStatusMS:  nil,
				}
				return expected, nil
			},
		},
		{
			name: "with app status - TimeToFirstStatusMS populated",
			setup: func(t *testing.T, h *taskTelemetryHelper, now time.Time) (telemetry.Task, *telemetry.TaskEvent) {
				taskCreatedAt := now.Add(-90 * time.Minute)
				firstStatusAt := now.Add(-85 * time.Minute)

				resp := dbfake.WorkspaceBuild(h.t, h.db, database.WorkspaceTable{
					OrganizationID: h.org.ID,
					OwnerID:        h.user.ID,
				}).WithTask(database.TaskTable{
					Prompt:    "running task with status prompt",
					CreatedAt: taskCreatedAt,
				}, nil).Seed(database.WorkspaceBuild{
					Transition:  database.WorkspaceTransitionStart,
					Reason:      database.BuildReasonInitiator,
					BuildNumber: 1,
					CreatedAt:   now.Add(-2 * time.Hour),
				}).Succeeded().Do()

				app := getApp(h.ctx, h.db, resp.Agents[0].ID)
				createAppStatus(h.ctx, h.db, resp.Workspace.ID, resp.Agents[0].ID, app.ID, database.WorkspaceAppStatusStateWorking, "Task started", firstStatusAt)

				expected := telemetry.Task{
					ID:                   resp.Task.ID.String(),
					OrganizationID:       h.org.ID.String(),
					OwnerID:              h.user.ID.String(),
					Name:                 resp.Task.Name,
					WorkspaceID:          ptr.Ref(resp.Workspace.ID.String()),
					WorkspaceBuildNumber: ptr.Ref(int64(1)),
					WorkspaceAgentID:     ptr.Ref(resp.Agents[0].ID.String()),
					WorkspaceAppID:       ptr.Ref(app.ID.String()),
					TemplateVersionID:    resp.TemplateVersion.ID.String(),
					PromptHash:           telemetry.HashContent(resp.Task.Prompt),
					CreatedAt:            resp.Task.CreatedAt,
					TimeToFirstStatusMS:  ptr.Ref(firstStatusAt.UnixMilli() - taskCreatedAt.UnixMilli()),
				}
				return expected, nil
			},
		},
		{
			name: "auto paused - LastPausedAt and PauseReason=auto",
			setup: func(t *testing.T, h *taskTelemetryHelper, now time.Time) (telemetry.Task, *telemetry.TaskEvent) {
				pauseTime := now.Add(-20 * time.Minute)

				resp := dbfake.WorkspaceBuild(h.t, h.db, database.WorkspaceTable{
					OrganizationID: h.org.ID,
					OwnerID:        h.user.ID,
				}).WithTask(database.TaskTable{
					Prompt:    "auto paused task prompt",
					CreatedAt: now.Add(-3 * time.Hour),
				}, nil).Seed(database.WorkspaceBuild{
					Transition:  database.WorkspaceTransitionStart,
					Reason:      database.BuildReasonInitiator,
					BuildNumber: 1,
					CreatedAt:   now.Add(-3 * time.Hour),
				}).Succeeded().Do()

				app := getApp(h.ctx, h.db, resp.Agents[0].ID)

				// Create second build (pause) using dbgen to avoid updating the task linkage.
				job2 := dbgen.ProvisionerJob(h.t, h.db, nil, database.ProvisionerJob{
					Provisioner:    database.ProvisionerTypeTerraform,
					StorageMethod:  database.ProvisionerStorageMethodFile,
					Type:           database.ProvisionerJobTypeWorkspaceBuild,
					OrganizationID: h.org.ID,
				})
				_ = dbgen.WorkspaceBuild(h.t, h.db, database.WorkspaceBuild{
					WorkspaceID:       resp.Workspace.ID,
					TemplateVersionID: resp.TemplateVersion.ID,
					JobID:             job2.ID,
					Transition:        database.WorkspaceTransitionStop,
					Reason:            database.BuildReasonTaskAutoPause,
					BuildNumber:       2,
					CreatedAt:         pauseTime,
					HasAITask: sql.NullBool{
						Bool:  true,
						Valid: true,
					},
				})

				expected := telemetry.Task{
					ID:                   resp.Task.ID.String(),
					OrganizationID:       h.org.ID.String(),
					OwnerID:              h.user.ID.String(),
					Name:                 resp.Task.Name,
					WorkspaceID:          ptr.Ref(resp.Workspace.ID.String()),
					WorkspaceBuildNumber: ptr.Ref(int64(1)),
					WorkspaceAgentID:     ptr.Ref(resp.Agents[0].ID.String()),
					WorkspaceAppID:       ptr.Ref(app.ID.String()),
					TemplateVersionID:    resp.TemplateVersion.ID.String(),
					PromptHash:           telemetry.HashContent(resp.Task.Prompt),
					CreatedAt:            resp.Task.CreatedAt,
					TimeToFirstStatusMS:  nil,
				}
				return expected, &telemetry.TaskEvent{
						TaskID:       resp.Task.ID.String(),
						LastPausedAt: &pauseTime,
						PauseReason:  ptr.Ref("auto"),
					}
			},
		},
		{
			name: "manual paused - LastPausedAt and PauseReason=manual",
			setup: func(t *testing.T, h *taskTelemetryHelper, now time.Time) (telemetry.Task, *telemetry.TaskEvent) {
				pauseTime := now.Add(-15 * time.Minute)

				resp := dbfake.WorkspaceBuild(h.t, h.db, database.WorkspaceTable{
					OrganizationID: h.org.ID,
					OwnerID:        h.user.ID,
				}).WithTask(database.TaskTable{
					Prompt:    "manually paused task prompt",
					CreatedAt: now.Add(-4 * time.Hour),
				}, nil).Seed(database.WorkspaceBuild{
					Transition:  database.WorkspaceTransitionStart,
					Reason:      database.BuildReasonInitiator,
					BuildNumber: 1,
					CreatedAt:   now.Add(-4 * time.Hour),
				}).Succeeded().Do()

				app := getApp(h.ctx, h.db, resp.Agents[0].ID)

				job2 := dbgen.ProvisionerJob(h.t, h.db, nil, database.ProvisionerJob{
					Provisioner:    database.ProvisionerTypeTerraform,
					StorageMethod:  database.ProvisionerStorageMethodFile,
					Type:           database.ProvisionerJobTypeWorkspaceBuild,
					OrganizationID: h.org.ID,
				})
				_ = dbgen.WorkspaceBuild(h.t, h.db, database.WorkspaceBuild{
					WorkspaceID:       resp.Workspace.ID,
					TemplateVersionID: resp.TemplateVersion.ID,
					JobID:             job2.ID,
					Transition:        database.WorkspaceTransitionStop,
					Reason:            database.BuildReasonTaskManualPause,
					BuildNumber:       2,
					CreatedAt:         pauseTime,
					HasAITask: sql.NullBool{
						Bool:  true,
						Valid: true,
					},
				})

				expected := telemetry.Task{
					ID:                   resp.Task.ID.String(),
					OrganizationID:       h.org.ID.String(),
					OwnerID:              h.user.ID.String(),
					Name:                 resp.Task.Name,
					WorkspaceID:          ptr.Ref(resp.Workspace.ID.String()),
					WorkspaceBuildNumber: ptr.Ref(int64(1)),
					WorkspaceAgentID:     ptr.Ref(resp.Agents[0].ID.String()),
					WorkspaceAppID:       ptr.Ref(app.ID.String()),
					TemplateVersionID:    resp.TemplateVersion.ID.String(),
					PromptHash:           telemetry.HashContent(resp.Task.Prompt),
					CreatedAt:            resp.Task.CreatedAt,
					TimeToFirstStatusMS:  nil,
				}
				return expected, &telemetry.TaskEvent{
						TaskID:       resp.Task.ID.String(),
						LastPausedAt: &pauseTime,
						PauseReason:  ptr.Ref("manual"),
					}
			},
		},
		{
			name: "paused with idle time - IdleDurationMS calculated",
			setup: func(t *testing.T, h *taskTelemetryHelper, now time.Time) (telemetry.Task, *telemetry.TaskEvent) {
				pauseTime := now.Add(-25 * time.Minute)

				resp := dbfake.WorkspaceBuild(h.t, h.db, database.WorkspaceTable{
					OrganizationID: h.org.ID,
					OwnerID:        h.user.ID,
				}).WithTask(database.TaskTable{
					Prompt:    "paused with idle time prompt",
					CreatedAt: now.Add(-5 * time.Hour),
				}, nil).Seed(database.WorkspaceBuild{
					Transition:  database.WorkspaceTransitionStart,
					Reason:      database.BuildReasonInitiator,
					BuildNumber: 1,
					CreatedAt:   now.Add(-5 * time.Hour),
				}).Succeeded().Do()

				app := getApp(h.ctx, h.db, resp.Agents[0].ID)

				// Working status at -40 minutes.
				createAppStatus(h.ctx, h.db, resp.Workspace.ID, resp.Agents[0].ID, app.ID, database.WorkspaceAppStatusStateWorking, "Working on something", now.Add(-40*time.Minute))
				// Idle status at -35 minutes (5 minutes after working).
				createAppStatus(h.ctx, h.db, resp.Workspace.ID, resp.Agents[0].ID, app.ID, database.WorkspaceAppStatusStateIdle, "Idle now", now.Add(-35*time.Minute))

				// Pause at -25 minutes (10 minutes after idle, 15 minutes after last working).
				job2 := dbgen.ProvisionerJob(h.t, h.db, nil, database.ProvisionerJob{
					Provisioner:    database.ProvisionerTypeTerraform,
					StorageMethod:  database.ProvisionerStorageMethodFile,
					Type:           database.ProvisionerJobTypeWorkspaceBuild,
					OrganizationID: h.org.ID,
				})
				_ = dbgen.WorkspaceBuild(h.t, h.db, database.WorkspaceBuild{
					WorkspaceID:       resp.Workspace.ID,
					TemplateVersionID: resp.TemplateVersion.ID,
					JobID:             job2.ID,
					Transition:        database.WorkspaceTransitionStop,
					Reason:            database.BuildReasonTaskAutoPause,
					BuildNumber:       2,
					CreatedAt:         pauseTime,
					HasAITask: sql.NullBool{
						Bool:  true,
						Valid: true,
					},
				})

				expected := telemetry.Task{
					ID:                   resp.Task.ID.String(),
					OrganizationID:       h.org.ID.String(),
					OwnerID:              h.user.ID.String(),
					Name:                 resp.Task.Name,
					WorkspaceID:          ptr.Ref(resp.Workspace.ID.String()),
					WorkspaceBuildNumber: ptr.Ref(int64(1)),
					WorkspaceAgentID:     ptr.Ref(resp.Agents[0].ID.String()),
					WorkspaceAppID:       ptr.Ref(app.ID.String()),
					TemplateVersionID:    resp.TemplateVersion.ID.String(),
					PromptHash:           telemetry.HashContent(resp.Task.Prompt),
					CreatedAt:            resp.Task.CreatedAt,
					TimeToFirstStatusMS:  ptr.Ref(260 * time.Minute.Milliseconds()), // -5hr to -40min = 260 min
				}
				return expected, &telemetry.TaskEvent{
						TaskID:         resp.Task.ID.String(),
						LastPausedAt:   &pauseTime,
						PauseReason:    ptr.Ref("auto"),
						IdleDurationMS: ptr.Ref(15 * time.Minute.Milliseconds()),
					}
			},
		},
		{
			name: "recently resumed - PausedDurationMS calculated",
			setup: func(t *testing.T, h *taskTelemetryHelper, now time.Time) (telemetry.Task, *telemetry.TaskEvent) {
				pauseTime := now.Add(-50 * time.Minute)
				resumeTime := now.Add(-10 * time.Minute)

				resp := dbfake.WorkspaceBuild(h.t, h.db, database.WorkspaceTable{
					OrganizationID: h.org.ID,
					OwnerID:        h.user.ID,
				}).WithTask(database.TaskTable{
					Prompt:    "recently resumed task prompt",
					CreatedAt: now.Add(-6 * time.Hour),
				}, nil).Seed(database.WorkspaceBuild{
					Transition:  database.WorkspaceTransitionStart,
					Reason:      database.BuildReasonInitiator,
					BuildNumber: 1,
					CreatedAt:   now.Add(-6 * time.Hour),
				}).Succeeded().Do()

				app := getApp(h.ctx, h.db, resp.Agents[0].ID)

				// Pause at -50 minutes.
				job2 := dbgen.ProvisionerJob(h.t, h.db, nil, database.ProvisionerJob{
					Provisioner:    database.ProvisionerTypeTerraform,
					StorageMethod:  database.ProvisionerStorageMethodFile,
					Type:           database.ProvisionerJobTypeWorkspaceBuild,
					OrganizationID: h.org.ID,
				})
				_ = dbgen.WorkspaceBuild(h.t, h.db, database.WorkspaceBuild{
					WorkspaceID:       resp.Workspace.ID,
					TemplateVersionID: resp.TemplateVersion.ID,
					JobID:             job2.ID,
					Transition:        database.WorkspaceTransitionStop,
					Reason:            database.BuildReasonTaskAutoPause,
					BuildNumber:       2,
					CreatedAt:         pauseTime,
					HasAITask: sql.NullBool{
						Bool:  true,
						Valid: true,
					},
				})

				// Resume at -10 minutes (40 minutes of paused time).
				job3 := dbgen.ProvisionerJob(h.t, h.db, nil, database.ProvisionerJob{
					Provisioner:    database.ProvisionerTypeTerraform,
					StorageMethod:  database.ProvisionerStorageMethodFile,
					Type:           database.ProvisionerJobTypeWorkspaceBuild,
					OrganizationID: h.org.ID,
				})
				_ = dbgen.WorkspaceBuild(h.t, h.db, database.WorkspaceBuild{
					WorkspaceID:       resp.Workspace.ID,
					TemplateVersionID: resp.TemplateVersion.ID,
					JobID:             job3.ID,
					Transition:        database.WorkspaceTransitionStart,
					Reason:            database.BuildReasonTaskResume,
					BuildNumber:       3,
					CreatedAt:         resumeTime,
					HasAITask: sql.NullBool{
						Bool:  true,
						Valid: true,
					},
				})

				expected := telemetry.Task{
					ID:                   resp.Task.ID.String(),
					OrganizationID:       h.org.ID.String(),
					OwnerID:              h.user.ID.String(),
					Name:                 resp.Task.Name,
					WorkspaceID:          ptr.Ref(resp.Workspace.ID.String()),
					WorkspaceBuildNumber: ptr.Ref(int64(1)),
					WorkspaceAgentID:     ptr.Ref(resp.Agents[0].ID.String()),
					WorkspaceAppID:       ptr.Ref(app.ID.String()),
					TemplateVersionID:    resp.TemplateVersion.ID.String(),
					PromptHash:           telemetry.HashContent(resp.Task.Prompt),
					CreatedAt:            resp.Task.CreatedAt,
					TimeToFirstStatusMS:  nil,
				}
				return expected, &telemetry.TaskEvent{
						TaskID:           resp.Task.ID.String(),
						LastPausedAt:     &pauseTime,
						LastResumedAt:    &resumeTime,
						PauseReason:      ptr.Ref("auto"),
						PausedDurationMS: ptr.Ref(40 * time.Minute.Milliseconds()),
					}
			},
		},
		{
			name: "resumed long ago - PausedDurationMS nil",
			setup: func(t *testing.T, h *taskTelemetryHelper, now time.Time) (telemetry.Task, *telemetry.TaskEvent) {
				pauseTime := now.Add(-5 * time.Hour)
				resumeTime := now.Add(-2 * time.Hour)

				resp := dbfake.WorkspaceBuild(h.t, h.db, database.WorkspaceTable{
					OrganizationID: h.org.ID,
					OwnerID:        h.user.ID,
				}).WithTask(database.TaskTable{
					Prompt:    "resumed long ago task prompt",
					CreatedAt: now.Add(-10 * time.Hour),
				}, nil).Seed(database.WorkspaceBuild{
					Transition:  database.WorkspaceTransitionStart,
					Reason:      database.BuildReasonInitiator,
					BuildNumber: 1,
					CreatedAt:   now.Add(-10 * time.Hour),
				}).Succeeded().Do()

				app := getApp(h.ctx, h.db, resp.Agents[0].ID)

				// Pause at -5 hours.
				job2 := dbgen.ProvisionerJob(h.t, h.db, nil, database.ProvisionerJob{
					Provisioner:    database.ProvisionerTypeTerraform,
					StorageMethod:  database.ProvisionerStorageMethodFile,
					Type:           database.ProvisionerJobTypeWorkspaceBuild,
					OrganizationID: h.org.ID,
				})
				_ = dbgen.WorkspaceBuild(h.t, h.db, database.WorkspaceBuild{
					WorkspaceID:       resp.Workspace.ID,
					TemplateVersionID: resp.TemplateVersion.ID,
					JobID:             job2.ID,
					Transition:        database.WorkspaceTransitionStop,
					Reason:            database.BuildReasonTaskAutoPause,
					BuildNumber:       2,
					CreatedAt:         pauseTime,
					HasAITask: sql.NullBool{
						Bool:  true,
						Valid: true,
					},
				})

				// Resume at -2 hours (> 1hr ago, so PausedDurationMS should be nil).
				job3 := dbgen.ProvisionerJob(h.t, h.db, nil, database.ProvisionerJob{
					Provisioner:    database.ProvisionerTypeTerraform,
					StorageMethod:  database.ProvisionerStorageMethodFile,
					Type:           database.ProvisionerJobTypeWorkspaceBuild,
					OrganizationID: h.org.ID,
				})
				_ = dbgen.WorkspaceBuild(h.t, h.db, database.WorkspaceBuild{
					WorkspaceID:       resp.Workspace.ID,
					TemplateVersionID: resp.TemplateVersion.ID,
					JobID:             job3.ID,
					Transition:        database.WorkspaceTransitionStart,
					Reason:            database.BuildReasonTaskResume,
					BuildNumber:       3,
					CreatedAt:         resumeTime,
					HasAITask: sql.NullBool{
						Bool:  true,
						Valid: true,
					},
				})

				expected := telemetry.Task{
					ID:                   resp.Task.ID.String(),
					OrganizationID:       h.org.ID.String(),
					OwnerID:              h.user.ID.String(),
					Name:                 resp.Task.Name,
					WorkspaceID:          ptr.Ref(resp.Workspace.ID.String()),
					WorkspaceBuildNumber: ptr.Ref(int64(1)),
					WorkspaceAgentID:     ptr.Ref(resp.Agents[0].ID.String()),
					WorkspaceAppID:       ptr.Ref(app.ID.String()),
					TemplateVersionID:    resp.TemplateVersion.ID.String(),
					PromptHash:           telemetry.HashContent(resp.Task.Prompt),
					CreatedAt:            resp.Task.CreatedAt,
					TimeToFirstStatusMS:  nil,
				}
				return expected, nil
			},
		},
		{
			name: "multiple cycles - captures latest pause/resume",
			setup: func(t *testing.T, h *taskTelemetryHelper, now time.Time) (telemetry.Task, *telemetry.TaskEvent) {
				firstResumeTime := now.Add(-150 * time.Minute) // -2.5 hours
				latestPauseTime := now.Add(-30 * time.Minute)

				resp := dbfake.WorkspaceBuild(h.t, h.db, database.WorkspaceTable{
					OrganizationID: h.org.ID,
					OwnerID:        h.user.ID,
				}).WithTask(database.TaskTable{
					Prompt:    "multiple pause resume cycles prompt",
					CreatedAt: now.Add(-8 * time.Hour),
				}, nil).Seed(database.WorkspaceBuild{
					Transition:  database.WorkspaceTransitionStart,
					Reason:      database.BuildReasonInitiator,
					BuildNumber: 1,
					CreatedAt:   now.Add(-8 * time.Hour),
				}).Succeeded().Do()

				app := getApp(h.ctx, h.db, resp.Agents[0].ID)

				// First pause at -3 hours (auto).
				job2 := dbgen.ProvisionerJob(h.t, h.db, nil, database.ProvisionerJob{
					Provisioner:    database.ProvisionerTypeTerraform,
					StorageMethod:  database.ProvisionerStorageMethodFile,
					Type:           database.ProvisionerJobTypeWorkspaceBuild,
					OrganizationID: h.org.ID,
				})
				_ = dbgen.WorkspaceBuild(h.t, h.db, database.WorkspaceBuild{
					WorkspaceID:       resp.Workspace.ID,
					TemplateVersionID: resp.TemplateVersion.ID,
					JobID:             job2.ID,
					Transition:        database.WorkspaceTransitionStop,
					Reason:            database.BuildReasonTaskAutoPause,
					BuildNumber:       2,
					CreatedAt:         now.Add(-3 * time.Hour),
					HasAITask: sql.NullBool{
						Bool:  true,
						Valid: true,
					},
				})

				// First resume at -2.5 hours.
				job3 := dbgen.ProvisionerJob(h.t, h.db, nil, database.ProvisionerJob{
					Provisioner:    database.ProvisionerTypeTerraform,
					StorageMethod:  database.ProvisionerStorageMethodFile,
					Type:           database.ProvisionerJobTypeWorkspaceBuild,
					OrganizationID: h.org.ID,
				})
				_ = dbgen.WorkspaceBuild(h.t, h.db, database.WorkspaceBuild{
					WorkspaceID:       resp.Workspace.ID,
					TemplateVersionID: resp.TemplateVersion.ID,
					JobID:             job3.ID,
					Transition:        database.WorkspaceTransitionStart,
					Reason:            database.BuildReasonTaskResume,
					BuildNumber:       3,
					CreatedAt:         firstResumeTime,
					HasAITask: sql.NullBool{
						Bool:  true,
						Valid: true,
					},
				})

				// Second pause at -30 minutes (manual).
				job4 := dbgen.ProvisionerJob(h.t, h.db, nil, database.ProvisionerJob{
					Provisioner:    database.ProvisionerTypeTerraform,
					StorageMethod:  database.ProvisionerStorageMethodFile,
					Type:           database.ProvisionerJobTypeWorkspaceBuild,
					OrganizationID: h.org.ID,
				})
				_ = dbgen.WorkspaceBuild(h.t, h.db, database.WorkspaceBuild{
					WorkspaceID:       resp.Workspace.ID,
					TemplateVersionID: resp.TemplateVersion.ID,
					JobID:             job4.ID,
					Transition:        database.WorkspaceTransitionStop,
					Reason:            database.BuildReasonTaskManualPause,
					BuildNumber:       4,
					CreatedAt:         latestPauseTime,
					HasAITask: sql.NullBool{
						Bool:  true,
						Valid: true,
					},
				})

				expected := telemetry.Task{
					ID:                   resp.Task.ID.String(),
					OrganizationID:       h.org.ID.String(),
					OwnerID:              h.user.ID.String(),
					Name:                 resp.Task.Name,
					WorkspaceID:          ptr.Ref(resp.Workspace.ID.String()),
					WorkspaceBuildNumber: ptr.Ref(int64(1)),
					WorkspaceAgentID:     ptr.Ref(resp.Agents[0].ID.String()),
					WorkspaceAppID:       ptr.Ref(app.ID.String()),
					TemplateVersionID:    resp.TemplateVersion.ID.String(),
					PromptHash:           telemetry.HashContent(resp.Task.Prompt),
					CreatedAt:            resp.Task.CreatedAt,
					TimeToFirstStatusMS:  nil,
				}
				return expected, &telemetry.TaskEvent{
						TaskID:        resp.Task.ID.String(),
						LastPausedAt:  &latestPauseTime,
						LastResumedAt: &firstResumeTime,
						PauseReason:   ptr.Ref("manual"),
					}
			},
		},
		{
			name: "currently paused after recent resume - PausedDurationMS nil",
			setup: func(t *testing.T, h *taskTelemetryHelper, now time.Time) (telemetry.Task, *telemetry.TaskEvent) {
				firstPauseTime := now.Add(-50 * time.Minute)
				resumeTime := now.Add(-30 * time.Minute)
				secondPauseTime := now.Add(-10 * time.Minute)

				resp := dbfake.WorkspaceBuild(h.t, h.db, database.WorkspaceTable{
					OrganizationID: h.org.ID,
					OwnerID:        h.user.ID,
				}).WithTask(database.TaskTable{
					Prompt:    "currently paused after resume prompt",
					CreatedAt: now.Add(-6 * time.Hour),
				}, nil).Seed(database.WorkspaceBuild{
					Transition:  database.WorkspaceTransitionStart,
					Reason:      database.BuildReasonInitiator,
					BuildNumber: 1,
					CreatedAt:   now.Add(-6 * time.Hour),
				}).Succeeded().Do()

				app := getApp(h.ctx, h.db, resp.Agents[0].ID)

				// First pause at -50 minutes.
				job2 := dbgen.ProvisionerJob(h.t, h.db, nil, database.ProvisionerJob{
					Provisioner:    database.ProvisionerTypeTerraform,
					StorageMethod:  database.ProvisionerStorageMethodFile,
					Type:           database.ProvisionerJobTypeWorkspaceBuild,
					OrganizationID: h.org.ID,
				})
				_ = dbgen.WorkspaceBuild(h.t, h.db, database.WorkspaceBuild{
					WorkspaceID:       resp.Workspace.ID,
					TemplateVersionID: resp.TemplateVersion.ID,
					JobID:             job2.ID,
					Transition:        database.WorkspaceTransitionStop,
					Reason:            database.BuildReasonTaskAutoPause,
					BuildNumber:       2,
					CreatedAt:         firstPauseTime,
					HasAITask: sql.NullBool{
						Bool:  true,
						Valid: true,
					},
				})

				// Resume at -30 minutes.
				job3 := dbgen.ProvisionerJob(h.t, h.db, nil, database.ProvisionerJob{
					Provisioner:    database.ProvisionerTypeTerraform,
					StorageMethod:  database.ProvisionerStorageMethodFile,
					Type:           database.ProvisionerJobTypeWorkspaceBuild,
					OrganizationID: h.org.ID,
				})
				_ = dbgen.WorkspaceBuild(h.t, h.db, database.WorkspaceBuild{
					WorkspaceID:       resp.Workspace.ID,
					TemplateVersionID: resp.TemplateVersion.ID,
					JobID:             job3.ID,
					Transition:        database.WorkspaceTransitionStart,
					Reason:            database.BuildReasonTaskResume,
					BuildNumber:       3,
					CreatedAt:         resumeTime,
					HasAITask: sql.NullBool{
						Bool:  true,
						Valid: true,
					},
				})

				// Second pause at -10 minutes (currently paused).
				job4 := dbgen.ProvisionerJob(h.t, h.db, nil, database.ProvisionerJob{
					Provisioner:    database.ProvisionerTypeTerraform,
					StorageMethod:  database.ProvisionerStorageMethodFile,
					Type:           database.ProvisionerJobTypeWorkspaceBuild,
					OrganizationID: h.org.ID,
				})
				_ = dbgen.WorkspaceBuild(h.t, h.db, database.WorkspaceBuild{
					WorkspaceID:       resp.Workspace.ID,
					TemplateVersionID: resp.TemplateVersion.ID,
					JobID:             job4.ID,
					Transition:        database.WorkspaceTransitionStop,
					Reason:            database.BuildReasonTaskManualPause,
					BuildNumber:       4,
					CreatedAt:         secondPauseTime,
					HasAITask: sql.NullBool{
						Bool:  true,
						Valid: true,
					},
				})

				expected := telemetry.Task{
					ID:                   resp.Task.ID.String(),
					OrganizationID:       h.org.ID.String(),
					OwnerID:              h.user.ID.String(),
					Name:                 resp.Task.Name,
					WorkspaceID:          ptr.Ref(resp.Workspace.ID.String()),
					WorkspaceBuildNumber: ptr.Ref(int64(1)),
					WorkspaceAgentID:     ptr.Ref(resp.Agents[0].ID.String()),
					WorkspaceAppID:       ptr.Ref(app.ID.String()),
					TemplateVersionID:    resp.TemplateVersion.ID.String(),
					PromptHash:           telemetry.HashContent(resp.Task.Prompt),
					CreatedAt:            resp.Task.CreatedAt,
					TimeToFirstStatusMS:  nil,
				}
				return expected, &telemetry.TaskEvent{
						TaskID:           resp.Task.ID.String(),
						LastPausedAt:     &secondPauseTime,
						LastResumedAt:    &resumeTime,
						PauseReason:      ptr.Ref("manual"),
						PausedDurationMS: ptr.Ref(20 * time.Minute.Milliseconds()),
					}
			},
		},
		{
			name: "multiple cycles with recent resume - pairs with preceding pause",
			setup: func(t *testing.T, h *taskTelemetryHelper, now time.Time) (telemetry.Task, *telemetry.TaskEvent) {
				firstPauseTime := now.Add(-50 * time.Minute)
				resumeTime := now.Add(-30 * time.Minute)

				resp := dbfake.WorkspaceBuild(h.t, h.db, database.WorkspaceTable{
					OrganizationID: h.org.ID,
					OwnerID:        h.user.ID,
				}).WithTask(database.TaskTable{
					Prompt:    "multi cycle recent resume prompt",
					CreatedAt: now.Add(-6 * time.Hour),
				}, nil).Seed(database.WorkspaceBuild{
					Transition:  database.WorkspaceTransitionStart,
					Reason:      database.BuildReasonInitiator,
					BuildNumber: 1,
					CreatedAt:   now.Add(-6 * time.Hour),
				}).Succeeded().Do()

				app := getApp(h.ctx, h.db, resp.Agents[0].ID)

				// Pause at -50 minutes.
				job2 := dbgen.ProvisionerJob(h.t, h.db, nil, database.ProvisionerJob{
					Provisioner:    database.ProvisionerTypeTerraform,
					StorageMethod:  database.ProvisionerStorageMethodFile,
					Type:           database.ProvisionerJobTypeWorkspaceBuild,
					OrganizationID: h.org.ID,
				})
				_ = dbgen.WorkspaceBuild(h.t, h.db, database.WorkspaceBuild{
					WorkspaceID:       resp.Workspace.ID,
					TemplateVersionID: resp.TemplateVersion.ID,
					JobID:             job2.ID,
					Transition:        database.WorkspaceTransitionStop,
					Reason:            database.BuildReasonTaskAutoPause,
					BuildNumber:       2,
					CreatedAt:         firstPauseTime,
					HasAITask: sql.NullBool{
						Bool:  true,
						Valid: true,
					},
				})

				// Resume at -30 minutes (20 min paused).
				job3 := dbgen.ProvisionerJob(h.t, h.db, nil, database.ProvisionerJob{
					Provisioner:    database.ProvisionerTypeTerraform,
					StorageMethod:  database.ProvisionerStorageMethodFile,
					Type:           database.ProvisionerJobTypeWorkspaceBuild,
					OrganizationID: h.org.ID,
				})
				_ = dbgen.WorkspaceBuild(h.t, h.db, database.WorkspaceBuild{
					WorkspaceID:       resp.Workspace.ID,
					TemplateVersionID: resp.TemplateVersion.ID,
					JobID:             job3.ID,
					Transition:        database.WorkspaceTransitionStart,
					Reason:            database.BuildReasonTaskResume,
					BuildNumber:       3,
					CreatedAt:         resumeTime,
					HasAITask: sql.NullBool{
						Bool:  true,
						Valid: true,
					},
				})

				expected := telemetry.Task{
					ID:                   resp.Task.ID.String(),
					OrganizationID:       h.org.ID.String(),
					OwnerID:              h.user.ID.String(),
					Name:                 resp.Task.Name,
					WorkspaceID:          ptr.Ref(resp.Workspace.ID.String()),
					WorkspaceBuildNumber: ptr.Ref(int64(1)),
					WorkspaceAgentID:     ptr.Ref(resp.Agents[0].ID.String()),
					WorkspaceAppID:       ptr.Ref(app.ID.String()),
					TemplateVersionID:    resp.TemplateVersion.ID.String(),
					PromptHash:           telemetry.HashContent(resp.Task.Prompt),
					CreatedAt:            resp.Task.CreatedAt,
					TimeToFirstStatusMS:  nil,
				}
				return expected, &telemetry.TaskEvent{
						TaskID:           resp.Task.ID.String(),
						LastPausedAt:     &firstPauseTime,
						LastResumedAt:    &resumeTime,
						PauseReason:      ptr.Ref("auto"),
						PausedDurationMS: ptr.Ref(20 * time.Minute.Milliseconds()),
					}
			},
		},
		{
			name: "all fields populated - full lifecycle",
			setup: func(t *testing.T, h *taskTelemetryHelper, now time.Time) (telemetry.Task, *telemetry.TaskEvent) {
				taskCreatedAt := now.Add(-7 * time.Hour)
				pauseTime := now.Add(-35 * time.Minute)
				resumeTime := now.Add(-5 * time.Minute)

				resp := dbfake.WorkspaceBuild(h.t, h.db, database.WorkspaceTable{
					OrganizationID: h.org.ID,
					OwnerID:        h.user.ID,
				}).WithTask(database.TaskTable{
					Prompt:    "task with all fields prompt",
					CreatedAt: taskCreatedAt,
				}, nil).Seed(database.WorkspaceBuild{
					Transition:  database.WorkspaceTransitionStart,
					Reason:      database.BuildReasonInitiator,
					BuildNumber: 1,
					CreatedAt:   now.Add(-7 * time.Hour),
				}).Succeeded().Do()

				app := getApp(h.ctx, h.db, resp.Agents[0].ID)

				// First status at -6.5 hours (30 minutes after creation).
				createAppStatus(h.ctx, h.db, resp.Workspace.ID, resp.Agents[0].ID, app.ID, database.WorkspaceAppStatusStateWorking, "Started working", now.Add(-390*time.Minute))
				// Last working status at -45 minutes.
				createAppStatus(h.ctx, h.db, resp.Workspace.ID, resp.Agents[0].ID, app.ID, database.WorkspaceAppStatusStateWorking, "Still working", now.Add(-45*time.Minute))

				// Pause at -35 minutes (10 minutes idle duration).
				job2 := dbgen.ProvisionerJob(h.t, h.db, nil, database.ProvisionerJob{
					Provisioner:    database.ProvisionerTypeTerraform,
					StorageMethod:  database.ProvisionerStorageMethodFile,
					Type:           database.ProvisionerJobTypeWorkspaceBuild,
					OrganizationID: h.org.ID,
				})
				_ = dbgen.WorkspaceBuild(h.t, h.db, database.WorkspaceBuild{
					WorkspaceID:       resp.Workspace.ID,
					TemplateVersionID: resp.TemplateVersion.ID,
					JobID:             job2.ID,
					Transition:        database.WorkspaceTransitionStop,
					Reason:            database.BuildReasonTaskAutoPause,
					BuildNumber:       2,
					CreatedAt:         pauseTime,
					HasAITask: sql.NullBool{
						Bool:  true,
						Valid: true,
					},
				})

				// Resume at -5 minutes (30 minutes paused duration).
				job3 := dbgen.ProvisionerJob(h.t, h.db, nil, database.ProvisionerJob{
					Provisioner:    database.ProvisionerTypeTerraform,
					StorageMethod:  database.ProvisionerStorageMethodFile,
					Type:           database.ProvisionerJobTypeWorkspaceBuild,
					OrganizationID: h.org.ID,
				})
				_ = dbgen.WorkspaceBuild(h.t, h.db, database.WorkspaceBuild{
					WorkspaceID:       resp.Workspace.ID,
					TemplateVersionID: resp.TemplateVersion.ID,
					JobID:             job3.ID,
					Transition:        database.WorkspaceTransitionStart,
					Reason:            database.BuildReasonTaskResume,
					BuildNumber:       3,
					CreatedAt:         resumeTime,
					HasAITask: sql.NullBool{
						Bool:  true,
						Valid: true,
					},
				})

				expected := telemetry.Task{
					ID:                   resp.Task.ID.String(),
					OrganizationID:       h.org.ID.String(),
					OwnerID:              h.user.ID.String(),
					Name:                 resp.Task.Name,
					WorkspaceID:          ptr.Ref(resp.Workspace.ID.String()),
					WorkspaceBuildNumber: ptr.Ref(int64(1)),
					WorkspaceAgentID:     ptr.Ref(resp.Agents[0].ID.String()),
					WorkspaceAppID:       ptr.Ref(app.ID.String()),
					TemplateVersionID:    resp.TemplateVersion.ID.String(),
					PromptHash:           telemetry.HashContent(resp.Task.Prompt),
					CreatedAt:            resp.Task.CreatedAt,
					TimeToFirstStatusMS:  ptr.Ref(30 * time.Minute.Milliseconds()),
				}
				return expected, &telemetry.TaskEvent{
						TaskID:           resp.Task.ID.String(),
						LastPausedAt:     &pauseTime,
						LastResumedAt:    &resumeTime,
						PauseReason:      ptr.Ref("auto"),
						IdleDurationMS:   ptr.Ref(10 * time.Minute.Milliseconds()),
						PausedDurationMS: ptr.Ref(30 * time.Minute.Milliseconds()),
					}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitMedium)
			db, _ := dbtestutil.NewDB(t)
			org, err := db.GetDefaultOrganization(ctx)
			require.NoError(t, err)
			user := dbgen.User(t, db, database.User{})
			_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
				UserID:         user.ID,
				OrganizationID: org.ID,
			})
			h := &taskTelemetryHelper{
				t:    t,
				ctx:  ctx,
				db:   db,
				org:  org,
				user: user,
			}

			// Create a deleted task. This is a test antagonist that should never show up in results.
			deletedTaskResp := dbfake.WorkspaceBuild(h.t, h.db, database.WorkspaceTable{
				OrganizationID: h.org.ID,
				OwnerID:        h.user.ID,
			}).WithTask(database.TaskTable{
				Prompt:    fmt.Sprintf("deleted-task-%s", t.Name()),
				CreatedAt: now.Add(-100 * time.Hour),
			}, nil).Seed(database.WorkspaceBuild{
				Transition:  database.WorkspaceTransitionStart,
				Reason:      database.BuildReasonInitiator,
				BuildNumber: 1,
				CreatedAt:   now.Add(-100 * time.Hour),
			}).Succeeded().Do()
			_, err = db.DeleteTask(h.ctx, database.DeleteTaskParams{
				DeletedAt: now.Add(-99 * time.Hour),
				ID:        deletedTaskResp.Task.ID,
			})
			require.NoError(h.t, err, "creating deleted task antagonist")

			expectedTask, expectedEvent := tt.setup(t, h, now)

			actualTasks, err := telemetry.CollectTasks(h.ctx, h.db)
			require.NoError(t, err, "unexpected error collecting tasks telemetry")
			// Invariant: deleted tasks should NEVER appear in results.
			require.Len(t, actualTasks, 1, "expected exactly one task")

			if diff := cmp.Diff(expectedTask, actualTasks[0]); diff != "" {
				t.Fatalf("task diff (-want +got):\n%s", diff)
			}

			actualEvents, err := telemetry.CollectTaskEvents(h.ctx, h.db, now.Add(-1*time.Hour))
			require.NoError(t, err)
			if expectedEvent == nil {
				require.Empty(t, actualEvents)
			} else {
				require.Len(t, actualEvents, 1)
				if diff := cmp.Diff(*expectedEvent, actualEvents[0]); diff != "" {
					t.Fatalf("event diff (-want +got):\n%s", diff)
				}
			}
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

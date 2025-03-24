package agentapi_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/types/known/durationpb"
	"tailscale.com/tailcfg"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/agentapi"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/externalauth"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/tailnet"
)

func TestGetManifest(t *testing.T) {
	t.Parallel()

	someTime, err := time.Parse(time.RFC3339, "2023-01-01T00:00:00Z")
	require.NoError(t, err)
	someTime = dbtime.Time(someTime)

	expectedEnvVars := map[string]string{
		"FOO":      "bar",
		"COOL_ENV": "dean was here",
	}
	expectedEnvVarsJSON, err := json.Marshal(expectedEnvVars)
	require.NoError(t, err)

	var (
		owner = database.User{
			ID:       uuid.New(),
			Username: "cool-user",
		}
		workspace = database.Workspace{
			ID:      uuid.New(),
			OwnerID: owner.ID,
			Name:    "cool-workspace",
		}
		agent = database.WorkspaceAgent{
			ID:   uuid.New(),
			Name: "cool-agent",
			EnvironmentVariables: pqtype.NullRawMessage{
				RawMessage: expectedEnvVarsJSON,
				Valid:      true,
			},
			Directory: "/cool/dir",
			MOTDFile:  "/cool/motd",
		}
		apps = []database.WorkspaceApp{
			{
				ID:                   uuid.New(),
				Url:                  sql.NullString{String: "http://localhost:1234", Valid: true},
				External:             false,
				Slug:                 "cool-app-1",
				DisplayName:          "app 1",
				Command:              sql.NullString{String: "cool command", Valid: true},
				Icon:                 "/icon.png",
				Subdomain:            true,
				SharingLevel:         database.AppSharingLevelAuthenticated,
				Health:               database.WorkspaceAppHealthHealthy,
				HealthcheckUrl:       "http://localhost:1234/health",
				HealthcheckInterval:  10,
				HealthcheckThreshold: 3,
			},
			{
				ID:           uuid.New(),
				Url:          sql.NullString{String: "http://google.com", Valid: true},
				External:     true,
				Slug:         "google",
				DisplayName:  "Literally Google",
				Command:      sql.NullString{Valid: false},
				Icon:         "/google.png",
				Subdomain:    false,
				SharingLevel: database.AppSharingLevelPublic,
				Health:       database.WorkspaceAppHealthDisabled,
				Hidden:       false,
			},
			{
				ID:                   uuid.New(),
				Url:                  sql.NullString{String: "http://localhost:4321", Valid: true},
				External:             true,
				Slug:                 "cool-app-2",
				DisplayName:          "another COOL app",
				Command:              sql.NullString{Valid: false},
				Icon:                 "",
				Subdomain:            false,
				SharingLevel:         database.AppSharingLevelOwner,
				Health:               database.WorkspaceAppHealthUnhealthy,
				HealthcheckUrl:       "http://localhost:4321/health",
				HealthcheckInterval:  20,
				HealthcheckThreshold: 5,
				Hidden:               true,
			},
		}
		scripts = []database.WorkspaceAgentScript{
			{
				ID:               uuid.New(),
				WorkspaceAgentID: agent.ID,
				LogSourceID:      uuid.New(),
				LogPath:          "/cool/log/path/1",
				Script:           "cool script 1",
				Cron:             "30 2 * * *",
				StartBlocksLogin: true,
				RunOnStart:       true,
				RunOnStop:        false,
				TimeoutSeconds:   60,
			},
			{
				ID:               uuid.New(),
				WorkspaceAgentID: agent.ID,
				LogSourceID:      uuid.New(),
				LogPath:          "/cool/log/path/2",
				Script:           "cool script 2",
				Cron:             "",
				StartBlocksLogin: false,
				RunOnStart:       false,
				RunOnStop:        true,
				TimeoutSeconds:   30,
			},
		}
		metadata = []database.WorkspaceAgentMetadatum{
			{
				WorkspaceAgentID: agent.ID,
				DisplayName:      "cool metadata 1",
				Key:              "cool-key-1",
				Script:           "cool script 1",
				Value:            "cool value 1",
				Error:            "",
				Timeout:          int64(time.Minute),
				Interval:         int64(time.Minute),
				CollectedAt:      someTime,
			},
			{
				WorkspaceAgentID: agent.ID,
				DisplayName:      "cool metadata 2",
				Key:              "cool-key-2",
				Script:           "cool script 2",
				Value:            "cool value 2",
				Error:            "some uncool error",
				Timeout:          int64(5 * time.Second),
				Interval:         int64(20 * time.Minute),
				CollectedAt:      someTime.Add(time.Hour),
			},
		}
		devcontainers = []database.WorkspaceAgentDevcontainer{
			{
				ID:               uuid.New(),
				WorkspaceAgentID: agent.ID,
				WorkspaceFolder:  "/cool/folder",
			},
			{
				ID:               uuid.New(),
				WorkspaceAgentID: agent.ID,
				WorkspaceFolder:  "/another/cool/folder",
				ConfigPath:       "/another/cool/folder/.devcontainer/devcontainer.json",
			},
		}
		derpMapFn = func() *tailcfg.DERPMap {
			return &tailcfg.DERPMap{
				Regions: map[int]*tailcfg.DERPRegion{
					1: {RegionName: "cool region"},
				},
			}
		}
	)

	// These are done manually to ensure the conversion logic matches what a
	// human expects.
	var (
		protoApps = []*agentproto.WorkspaceApp{
			{
				Id:            apps[0].ID[:],
				Url:           apps[0].Url.String,
				External:      apps[0].External,
				Slug:          apps[0].Slug,
				DisplayName:   apps[0].DisplayName,
				Command:       apps[0].Command.String,
				Icon:          apps[0].Icon,
				Subdomain:     apps[0].Subdomain,
				SubdomainName: fmt.Sprintf("%s--%s--%s--%s", apps[0].Slug, agent.Name, workspace.Name, owner.Username),
				SharingLevel:  agentproto.WorkspaceApp_AUTHENTICATED,
				Healthcheck: &agentproto.WorkspaceApp_Healthcheck{
					Url:       apps[0].HealthcheckUrl,
					Interval:  durationpb.New(time.Duration(apps[0].HealthcheckInterval) * time.Second),
					Threshold: apps[0].HealthcheckThreshold,
				},
				Health: agentproto.WorkspaceApp_HEALTHY,
				Hidden: false,
			},
			{
				Id:            apps[1].ID[:],
				Url:           apps[1].Url.String,
				External:      apps[1].External,
				Slug:          apps[1].Slug,
				DisplayName:   apps[1].DisplayName,
				Command:       apps[1].Command.String,
				Icon:          apps[1].Icon,
				Subdomain:     false,
				SubdomainName: "",
				SharingLevel:  agentproto.WorkspaceApp_PUBLIC,
				Healthcheck: &agentproto.WorkspaceApp_Healthcheck{
					Url:       "",
					Interval:  durationpb.New(0),
					Threshold: 0,
				},
				Health: agentproto.WorkspaceApp_DISABLED,
				Hidden: false,
			},
			{
				Id:            apps[2].ID[:],
				Url:           apps[2].Url.String,
				External:      apps[2].External,
				Slug:          apps[2].Slug,
				DisplayName:   apps[2].DisplayName,
				Command:       apps[2].Command.String,
				Icon:          apps[2].Icon,
				Subdomain:     false,
				SubdomainName: "",
				SharingLevel:  agentproto.WorkspaceApp_OWNER,
				Healthcheck: &agentproto.WorkspaceApp_Healthcheck{
					Url:       apps[2].HealthcheckUrl,
					Interval:  durationpb.New(time.Duration(apps[2].HealthcheckInterval) * time.Second),
					Threshold: apps[2].HealthcheckThreshold,
				},
				Health: agentproto.WorkspaceApp_UNHEALTHY,
				Hidden: true,
			},
		}
		protoScripts = []*agentproto.WorkspaceAgentScript{
			{
				Id:               scripts[0].ID[:],
				LogSourceId:      scripts[0].LogSourceID[:],
				LogPath:          scripts[0].LogPath,
				Script:           scripts[0].Script,
				Cron:             scripts[0].Cron,
				RunOnStart:       scripts[0].RunOnStart,
				RunOnStop:        scripts[0].RunOnStop,
				StartBlocksLogin: scripts[0].StartBlocksLogin,
				Timeout:          durationpb.New(time.Duration(scripts[0].TimeoutSeconds) * time.Second),
			},
			{
				Id:               scripts[1].ID[:],
				LogSourceId:      scripts[1].LogSourceID[:],
				LogPath:          scripts[1].LogPath,
				Script:           scripts[1].Script,
				Cron:             scripts[1].Cron,
				RunOnStart:       scripts[1].RunOnStart,
				RunOnStop:        scripts[1].RunOnStop,
				StartBlocksLogin: scripts[1].StartBlocksLogin,
				Timeout:          durationpb.New(time.Duration(scripts[1].TimeoutSeconds) * time.Second),
			},
		}
		protoMetadata = []*agentproto.WorkspaceAgentMetadata_Description{
			{
				DisplayName: metadata[0].DisplayName,
				Key:         metadata[0].Key,
				Script:      metadata[0].Script,
				Interval:    durationpb.New(time.Duration(metadata[0].Interval)),
				Timeout:     durationpb.New(time.Duration(metadata[0].Timeout)),
			},
			{
				DisplayName: metadata[1].DisplayName,
				Key:         metadata[1].Key,
				Script:      metadata[1].Script,
				Interval:    durationpb.New(time.Duration(metadata[1].Interval)),
				Timeout:     durationpb.New(time.Duration(metadata[1].Timeout)),
			},
		}
		protoDevcontainers = []*agentproto.WorkspaceAgentDevcontainer{
			{
				Id:              devcontainers[0].ID[:],
				WorkspaceFolder: devcontainers[0].WorkspaceFolder,
			},
			{
				Id:              devcontainers[1].ID[:],
				WorkspaceFolder: devcontainers[1].WorkspaceFolder,
				ConfigPath:      devcontainers[1].ConfigPath,
			},
		}
	)

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		mDB := dbmock.NewMockStore(gomock.NewController(t))

		api := &agentapi.ManifestAPI{
			AccessURL:   &url.URL{Scheme: "https", Host: "example.com"},
			AppHostname: "*--apps.example.com",
			ExternalAuthConfigs: []*externalauth.Config{
				{Type: string(codersdk.EnhancedExternalAuthProviderGitHub)},
				{Type: "some-provider"},
				{Type: string(codersdk.EnhancedExternalAuthProviderGitLab)},
			},
			DisableDirectConnections: true,
			DerpForceWebSockets:      true,

			AgentFn: func(ctx context.Context) (database.WorkspaceAgent, error) {
				return agent, nil
			},
			WorkspaceID: workspace.ID,
			Database:    mDB,
			DerpMapFn:   derpMapFn,
		}

		mDB.EXPECT().GetWorkspaceAppsByAgentID(gomock.Any(), agent.ID).Return(apps, nil)
		mDB.EXPECT().GetWorkspaceAgentScriptsByAgentIDs(gomock.Any(), []uuid.UUID{agent.ID}).Return(scripts, nil)
		mDB.EXPECT().GetWorkspaceAgentMetadata(gomock.Any(), database.GetWorkspaceAgentMetadataParams{
			WorkspaceAgentID: agent.ID,
			Keys:             nil, // all
		}).Return(metadata, nil)
		mDB.EXPECT().GetWorkspaceAgentDevcontainersByAgentID(gomock.Any(), agent.ID).Return(devcontainers, nil)
		mDB.EXPECT().GetWorkspaceByID(gomock.Any(), workspace.ID).Return(workspace, nil)
		mDB.EXPECT().GetUserByID(gomock.Any(), workspace.OwnerID).Return(owner, nil)

		got, err := api.GetManifest(context.Background(), &agentproto.GetManifestRequest{})
		require.NoError(t, err)

		expected := &agentproto.Manifest{
			AgentId:                  agent.ID[:],
			AgentName:                agent.Name,
			OwnerUsername:            owner.Username,
			WorkspaceId:              workspace.ID[:],
			WorkspaceName:            workspace.Name,
			GitAuthConfigs:           2, // two "enhanced" external auth configs
			EnvironmentVariables:     expectedEnvVars,
			Directory:                agent.Directory,
			VsCodePortProxyUri:       fmt.Sprintf("https://{{port}}--%s--%s--%s--apps.example.com", agent.Name, workspace.Name, owner.Username),
			MotdPath:                 agent.MOTDFile,
			DisableDirectConnections: true,
			DerpForceWebsockets:      true,
			// tailnet.DERPMapToProto() is extensively tested elsewhere, so it's
			// not necessary to manually recreate a big DERP map here like we
			// did for apps and metadata.
			DerpMap:       tailnet.DERPMapToProto(derpMapFn()),
			Scripts:       protoScripts,
			Apps:          protoApps,
			Metadata:      protoMetadata,
			Devcontainers: protoDevcontainers,
		}

		// Log got and expected with spew.
		// t.Log("got:\n" + spew.Sdump(got))
		// t.Log("expected:\n" + spew.Sdump(expected))

		require.Equal(t, expected, got)
	})

	t.Run("NoAppHostname", func(t *testing.T) {
		t.Parallel()

		mDB := dbmock.NewMockStore(gomock.NewController(t))

		api := &agentapi.ManifestAPI{
			AccessURL:   &url.URL{Scheme: "https", Host: "example.com"},
			AppHostname: "",
			ExternalAuthConfigs: []*externalauth.Config{
				{Type: string(codersdk.EnhancedExternalAuthProviderGitHub)},
				{Type: "some-provider"},
				{Type: string(codersdk.EnhancedExternalAuthProviderGitLab)},
			},
			DisableDirectConnections: true,
			DerpForceWebSockets:      true,

			AgentFn: func(ctx context.Context) (database.WorkspaceAgent, error) {
				return agent, nil
			},
			WorkspaceID: workspace.ID,
			Database:    mDB,
			DerpMapFn:   derpMapFn,
		}

		mDB.EXPECT().GetWorkspaceAppsByAgentID(gomock.Any(), agent.ID).Return(apps, nil)
		mDB.EXPECT().GetWorkspaceAgentScriptsByAgentIDs(gomock.Any(), []uuid.UUID{agent.ID}).Return(scripts, nil)
		mDB.EXPECT().GetWorkspaceAgentMetadata(gomock.Any(), database.GetWorkspaceAgentMetadataParams{
			WorkspaceAgentID: agent.ID,
			Keys:             nil, // all
		}).Return(metadata, nil)
		mDB.EXPECT().GetWorkspaceAgentDevcontainersByAgentID(gomock.Any(), agent.ID).Return(devcontainers, nil)
		mDB.EXPECT().GetWorkspaceByID(gomock.Any(), workspace.ID).Return(workspace, nil)
		mDB.EXPECT().GetUserByID(gomock.Any(), workspace.OwnerID).Return(owner, nil)

		got, err := api.GetManifest(context.Background(), &agentproto.GetManifestRequest{})
		require.NoError(t, err)

		expected := &agentproto.Manifest{
			AgentId:                  agent.ID[:],
			AgentName:                agent.Name,
			OwnerUsername:            owner.Username,
			WorkspaceId:              workspace.ID[:],
			WorkspaceName:            workspace.Name,
			GitAuthConfigs:           2, // two "enhanced" external auth configs
			EnvironmentVariables:     expectedEnvVars,
			Directory:                agent.Directory,
			VsCodePortProxyUri:       "", // empty with no AppHost
			MotdPath:                 agent.MOTDFile,
			DisableDirectConnections: true,
			DerpForceWebsockets:      true,
			// tailnet.DERPMapToProto() is extensively tested elsewhere, so it's
			// not necessary to manually recreate a big DERP map here like we
			// did for apps and metadata.
			DerpMap:       tailnet.DERPMapToProto(derpMapFn()),
			Scripts:       protoScripts,
			Apps:          protoApps,
			Metadata:      protoMetadata,
			Devcontainers: protoDevcontainers,
		}

		// Log got and expected with spew.
		// t.Log("got:\n" + spew.Sdump(got))
		// t.Log("expected:\n" + spew.Sdump(expected))

		require.Equal(t, expected, got)
	})
}

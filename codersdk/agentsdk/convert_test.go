package agentsdk_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
	"tailscale.com/tailcfg"

	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/tailnet"
)

func TestManifest(t *testing.T) {
	t.Parallel()
	manifest := agentsdk.Manifest{
		ParentID:           uuid.New(),
		AgentID:            uuid.New(),
		AgentName:          "test-agent",
		OwnerName:          "test-owner",
		WorkspaceID:        uuid.New(),
		WorkspaceName:      "test-workspace",
		GitAuthConfigs:     3,
		VSCodePortProxyURI: "http://proxy.example.com/stuff",
		Apps: []codersdk.WorkspaceApp{
			{
				ID:            uuid.New(),
				URL:           "http://app1.example.com",
				External:      true,
				Slug:          "app1",
				DisplayName:   "App 1",
				Command:       "app1 -d",
				Icon:          "app1.png",
				Subdomain:     true,
				SubdomainName: "app1.example.com",
				SharingLevel:  codersdk.WorkspaceAppSharingLevelAuthenticated,
				Healthcheck: codersdk.Healthcheck{
					URL:       "http://localhost:3030/healthz",
					Interval:  55555666,
					Threshold: 55555666,
				},
				Health: codersdk.WorkspaceAppHealthHealthy,
				Hidden: false,
			},
			{
				ID:            uuid.New(),
				URL:           "http://app2.example.com",
				External:      false,
				Slug:          "app2",
				DisplayName:   "App 2",
				Command:       "app2 -d",
				Icon:          "app2.png",
				Subdomain:     false,
				SubdomainName: "app2.example.com",
				SharingLevel:  codersdk.WorkspaceAppSharingLevelPublic,
				Healthcheck: codersdk.Healthcheck{
					URL:       "http://localhost:3032/healthz",
					Interval:  22555666,
					Threshold: 22555666,
				},
				Health: codersdk.WorkspaceAppHealthInitializing,
				Hidden: true,
			},
		},
		DERPMap: &tailcfg.DERPMap{
			HomeParams: &tailcfg.DERPHomeParams{RegionScore: map[int]float64{999: 0.025}},
			Regions: map[int]*tailcfg.DERPRegion{
				999: {
					EmbeddedRelay: true,
					RegionID:      999,
					RegionCode:    "default",
					RegionName:    "HOME",
					Avoid:         false,
					Nodes: []*tailcfg.DERPNode{
						{
							Name: "Home1",
						},
					},
				},
			},
		},
		DERPForceWebSockets:      true,
		EnvironmentVariables:     map[string]string{"FOO": "bar"},
		Directory:                "/home/coder",
		MOTDFile:                 "/etc/motd",
		DisableDirectConnections: true,
		Metadata: []codersdk.WorkspaceAgentMetadataDescription{
			{
				DisplayName: "CPU",
				Key:         "cpu",
				Script:      "getcpu",
				Interval:    44444422,
				Timeout:     44444411,
			},
			{
				DisplayName: "MEM",
				Key:         "mem",
				Script:      "getmem",
				Interval:    54444422,
				Timeout:     54444411,
			},
		},
		Scripts: []codersdk.WorkspaceAgentScript{
			{
				ID:               uuid.New(),
				LogSourceID:      uuid.New(),
				LogPath:          "/var/log/script.log",
				Script:           "script",
				Cron:             "somecron",
				RunOnStart:       true,
				RunOnStop:        true,
				StartBlocksLogin: true,
				Timeout:          time.Second,
				DisplayName:      "foo",
			},
			{
				ID:               uuid.New(),
				LogSourceID:      uuid.New(),
				LogPath:          "/var/log/script2.log",
				Script:           "script2",
				Cron:             "somecron2",
				RunOnStart:       false,
				RunOnStop:        true,
				StartBlocksLogin: true,
				Timeout:          time.Second * 4,
				DisplayName:      "bar",
			},
		},
		Devcontainers: []codersdk.WorkspaceAgentDevcontainer{
			{
				ID:              uuid.New(),
				WorkspaceFolder: "/home/coder/coder",
				ConfigPath:      "/home/coder/coder/.devcontainer/devcontainer.json",
				SubagentID:      uuid.NullUUID{Valid: true, UUID: uuid.New()},
			},
		},
	}
	p, err := agentsdk.ProtoFromManifest(manifest)
	require.NoError(t, err)
	back, err := agentsdk.ManifestFromProto(p)
	require.NoError(t, err)
	require.Equal(t, manifest.ParentID, back.ParentID)
	require.Equal(t, manifest.AgentID, back.AgentID)
	require.Equal(t, manifest.AgentName, back.AgentName)
	require.Equal(t, manifest.OwnerName, back.OwnerName)
	require.Equal(t, manifest.WorkspaceID, back.WorkspaceID)
	require.Equal(t, manifest.WorkspaceName, back.WorkspaceName)
	require.Equal(t, manifest.GitAuthConfigs, back.GitAuthConfigs)
	require.Equal(t, manifest.VSCodePortProxyURI, back.VSCodePortProxyURI)
	require.Equal(t, manifest.Apps, back.Apps)
	require.NotNil(t, back.DERPMap)
	require.True(t, tailnet.CompareDERPMaps(manifest.DERPMap, back.DERPMap))
	require.Equal(t, manifest.DERPForceWebSockets, back.DERPForceWebSockets)
	require.Equal(t, manifest.EnvironmentVariables, back.EnvironmentVariables)
	require.Equal(t, manifest.Directory, back.Directory)
	require.Equal(t, manifest.MOTDFile, back.MOTDFile)
	require.Equal(t, manifest.DisableDirectConnections, back.DisableDirectConnections)
	require.Equal(t, manifest.Metadata, back.Metadata)
	require.Equal(t, manifest.Scripts, back.Scripts)
	require.Equal(t, manifest.Devcontainers, back.Devcontainers)
}

func TestSubsystems(t *testing.T) {
	t.Parallel()
	ss := []codersdk.AgentSubsystem{
		codersdk.AgentSubsystemEnvbox,
		codersdk.AgentSubsystemEnvbuilder,
		codersdk.AgentSubsystemExectrace,
	}
	ps, err := agentsdk.ProtoFromSubsystems(ss)
	require.NoError(t, err)
	require.Equal(t, ps, []proto.Startup_Subsystem{
		proto.Startup_ENVBOX,
		proto.Startup_ENVBUILDER,
		proto.Startup_EXECTRACE,
	})
}

func TestProtoFromLifecycle(t *testing.T) {
	t.Parallel()
	now := dbtime.Now()
	for _, s := range codersdk.WorkspaceAgentLifecycleOrder {
		sr := agentsdk.PostLifecycleRequest{State: s, ChangedAt: now}
		pr, err := agentsdk.ProtoFromLifecycle(sr)
		require.NoError(t, err)
		require.Equal(t, now, pr.ChangedAt.AsTime())
		state, err := agentsdk.LifecycleStateFromProto(pr.State)
		require.NoError(t, err)
		require.Equal(t, s, state)
	}
}

func TestProtoFromMetadataResult(t *testing.T) {
	t.Parallel()
	now := dbtime.Now()
	result := codersdk.WorkspaceAgentMetadataResult{
		CollectedAt: now,
		Age:         4,
		Value:       "lemons",
		Error:       "rats",
	}
	pr := agentsdk.ProtoFromMetadataResult(result)
	require.NotNil(t, pr)
	require.Equal(t, now, pr.CollectedAt.AsTime())
	require.EqualValues(t, 4, pr.Age)
	require.Equal(t, "lemons", pr.Value)
	require.Equal(t, "rats", pr.Error)
	result2 := agentsdk.MetadataResultFromProto(pr)
	require.Equal(t, result, result2)
}

func TestMetadataFromProto(t *testing.T) {
	t.Parallel()
	now := dbtime.Now()
	pmd := &proto.Metadata{
		Key: "a flat",
		Result: &proto.WorkspaceAgentMetadata_Result{
			CollectedAt: timestamppb.New(now),
			Age:         88,
			Value:       "lemons",
			Error:       "rats",
		},
	}
	smd := agentsdk.MetadataFromProto(pmd)
	require.Equal(t, "a flat", smd.Key)
	require.Equal(t, now, smd.CollectedAt)
	require.EqualValues(t, 88, smd.Age)
	require.Equal(t, "lemons", smd.Value)
	require.Equal(t, "rats", smd.Error)
}

func TestSecretsRoundTrip(t *testing.T) {
	t.Parallel()
	secrets := []agentsdk.WorkspaceSecret{
		{
			EnvName:  "GITHUB_TOKEN",
			FilePath: "",
			Value:    []byte("ghp_xxxx"),
		},
		{
			EnvName:  "",
			FilePath: "~/.aws/credentials",
			Value:    []byte("[default]\naws_access_key_id=AKIA..."),
		},
		{
			EnvName:  "BOTH_ENV",
			FilePath: "/etc/both",
			Value:    []byte("both-value"),
		},
	}

	protoSecrets := agentsdk.ProtoFromSecrets(secrets)
	require.Len(t, protoSecrets, 3)
	require.Equal(t, "GITHUB_TOKEN", protoSecrets[0].EnvName)
	require.Equal(t, "", protoSecrets[0].FilePath)
	require.Equal(t, []byte("ghp_xxxx"), protoSecrets[0].Value)
	require.Equal(t, "", protoSecrets[1].EnvName)
	require.Equal(t, "~/.aws/credentials", protoSecrets[1].FilePath)
	require.Equal(t, []byte("[default]\naws_access_key_id=AKIA..."), protoSecrets[1].Value)
	require.Equal(t, "BOTH_ENV", protoSecrets[2].EnvName)
	require.Equal(t, "/etc/both", protoSecrets[2].FilePath)
	require.Equal(t, []byte("both-value"), protoSecrets[2].Value)

	roundTripped := agentsdk.SecretsFromProto(protoSecrets)
	require.Equal(t, secrets, roundTripped)
}

func TestSubagentExecutionsRoundTrip(t *testing.T) {
	t.Parallel()
	executions := []agentsdk.SubagentExecution{
		{
			ExecutionID:     uuid.New(),
			Generation:      uuid.New(),
			Name:            "sandbox",
			Driver:          "bubblewrap",
			DriverProtocol:  1,
			SharedHostPath:  "/home/coder/project",
			SharedChildPath: "/workspace/project",
			StartupTimeout:  90 * time.Second,
			RestartPolicy:   "on_failure",
		},
	}
	manifest := agentsdk.Manifest{
		AgentID:            uuid.New(),
		WorkspaceID:        uuid.New(),
		SubagentExecutions: executions,
	}

	p, err := agentsdk.ProtoFromManifest(manifest)
	require.NoError(t, err)
	require.Len(t, p.SubagentExecutions, 1)
	pse := p.SubagentExecutions[0]
	require.Equal(t, executions[0].ExecutionID[:], pse.ExecutionId)
	require.Equal(t, executions[0].Generation[:], pse.Generation)
	require.Equal(t, "sandbox", pse.Name)
	require.Equal(t, "bubblewrap", pse.Driver)
	require.EqualValues(t, 1, pse.DriverProtocol)
	require.Equal(t, "/home/coder/project", pse.SharedHostPath)
	require.Equal(t, "/workspace/project", pse.SharedChildPath)
	require.EqualValues(t, 90, pse.StartupTimeoutSeconds)
	require.Equal(t, "on_failure", pse.RestartPolicy)

	back, err := agentsdk.ManifestFromProto(p)
	require.NoError(t, err)
	require.Equal(t, executions, back.SubagentExecutions)
}

func TestSubagentExecutionsPreserveOrder(t *testing.T) {
	t.Parallel()
	executions := []agentsdk.SubagentExecution{
		{ExecutionID: uuid.New(), Generation: uuid.New(), Name: "first"},
		{ExecutionID: uuid.New(), Generation: uuid.New(), Name: "second"},
		{ExecutionID: uuid.New(), Generation: uuid.New(), Name: "third"},
	}

	protoExecutions := agentsdk.ProtoFromSubagentExecutions(executions)
	require.Len(t, protoExecutions, 3)
	require.Equal(t, []string{"first", "second", "third"}, []string{
		protoExecutions[0].Name, protoExecutions[1].Name, protoExecutions[2].Name,
	})

	back, err := agentsdk.SubagentExecutionsFromProto(protoExecutions)
	require.NoError(t, err)
	require.Equal(t, executions, back)
}

func TestSubagentExecutionsMalformedIDs(t *testing.T) {
	t.Parallel()
	validID := uuid.New()

	for _, tc := range []struct {
		name        string
		execution   *proto.SubagentExecution
		errContains string
	}{
		{
			name: "ExecutionID",
			execution: &proto.SubagentExecution{
				ExecutionId: []byte("too-short"),
				Generation:  validID[:],
			},
			errContains: "parse execution id",
		},
		{
			name: "Generation",
			execution: &proto.SubagentExecution{
				ExecutionId: validID[:],
				Generation:  []byte("too-short"),
			},
			errContains: "parse generation",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := agentsdk.SubagentExecutionsFromProto([]*proto.SubagentExecution{tc.execution})
			require.ErrorContains(t, err, tc.errContains)
			require.ErrorContains(t, err, "parse subagent execution 0")

			agentID := uuid.New()
			workspaceID := uuid.New()
			_, err = agentsdk.ManifestFromProto(&proto.Manifest{
				AgentId:            agentID[:],
				WorkspaceId:        workspaceID[:],
				SubagentExecutions: []*proto.SubagentExecution{tc.execution},
			})
			require.ErrorContains(t, err, "error converting workspace agent subagent executions")
		})
	}
}

// TestSubagentExecutionOmitsCredentials pins the declaration-only
// shape of the SDK type: child agent IDs, auth tokens, and acquisition
// versions are fetched via AcquireSubagentExecution instead, so they
// must never appear on a manifest declaration.
func TestSubagentExecutionOmitsCredentials(t *testing.T) {
	t.Parallel()
	typ := reflect.TypeOf(agentsdk.SubagentExecution{})
	fields := make([]string, 0, typ.NumField())
	for i := range typ.NumField() {
		fields = append(fields, typ.Field(i).Name)
	}
	require.Equal(t, []string{
		"ExecutionID",
		"Generation",
		"Name",
		"Driver",
		"DriverProtocol",
		"SharedHostPath",
		"SharedChildPath",
		"StartupTimeout",
		"RestartPolicy",
	}, fields)
}

// TestManifestExecutionIsolationRoundTrip asserts that the marker telling an
// agent it runs inside an execution boundary survives both conversion
// directions. The agent withholds its own credential from spawned commands
// based on it, so losing it silently restores the leak.
func TestManifestExecutionIsolationRoundTrip(t *testing.T) {
	t.Parallel()

	for _, isolated := range []bool{true, false} {
		manifest := agentsdk.Manifest{
			AgentID:            uuid.New(),
			WorkspaceID:        uuid.New(),
			ExecutionIsolation: isolated,
		}

		p, err := agentsdk.ProtoFromManifest(manifest)
		require.NoError(t, err)
		require.Equal(t, isolated, p.ExecutionIsolation)

		back, err := agentsdk.ManifestFromProto(p)
		require.NoError(t, err)
		require.Equal(t, isolated, back.ExecutionIsolation)
	}
}

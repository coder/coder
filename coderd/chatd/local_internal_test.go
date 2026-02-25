package chatd

import (
	"archive/tar"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisionersdk"
)

func localModeTestDB(t *testing.T) database.Store {
	t.Helper()

	db, _ := dbtestutil.NewDB(t)
	return db
}

func seedWorkspaceWithLatestBuildAgents(
	t *testing.T,
	db database.Store,
	agentNames ...string,
) (uuid.UUID, map[string]uuid.UUID) {
	t.Helper()

	org := dbgen.Organization(t, db, database.Organization{})
	user := dbgen.User(t, db, database.User{})
	_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})
	templateVersion := dbfake.TemplateVersion(t, db).
		Seed(database.TemplateVersion{
			OrganizationID: org.ID,
			CreatedBy:      user.ID,
		}).
		Do()
	workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		TemplateID:     templateVersion.Template.ID,
	})
	job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		OrganizationID: workspace.OrganizationID,
		InitiatorID:    user.ID,
		Provisioner:    database.ProvisionerTypeTerraform,
		Tags: database.StringMap{
			provisionersdk.TagScope: provisionersdk.ScopeOrganization,
			provisionersdk.TagOwner: "",
		},
	})
	_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:       workspace.ID,
		TemplateVersionID: templateVersion.TemplateVersion.ID,
		InitiatorID:       user.ID,
		JobID:             job.ID,
	})
	resource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
		JobID: job.ID,
		Type:  "coder_external_agent",
		Name:  localChatExternalResourceName,
	})

	agentIDByName := make(map[string]uuid.UUID, len(agentNames))
	for _, name := range agentNames {
		agent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
			ResourceID: resource.ID,
			Name:       name,
		})
		agentIDByName[name] = agent.ID
	}
	return workspace.ID, agentIDByName
}

func seedWorkspaceWithLocalChatAgent(
	t *testing.T,
	db database.Store,
	agentName string,
	agentToken uuid.UUID,
) (uuid.UUID, uuid.UUID) {
	t.Helper()

	org := dbgen.Organization(t, db, database.Organization{})
	user := dbgen.User(t, db, database.User{})
	_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})
	templateVersion := dbfake.TemplateVersion(t, db).
		Seed(database.TemplateVersion{
			OrganizationID: org.ID,
			CreatedBy:      user.ID,
		}).
		Do()
	workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		TemplateID:     templateVersion.Template.ID,
	})
	job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		OrganizationID: workspace.OrganizationID,
		InitiatorID:    user.ID,
		Provisioner:    database.ProvisionerTypeTerraform,
		Tags: database.StringMap{
			provisionersdk.TagScope: provisionersdk.ScopeOrganization,
			provisionersdk.TagOwner: "",
		},
	})
	_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:       workspace.ID,
		TemplateVersionID: templateVersion.TemplateVersion.ID,
		InitiatorID:       user.ID,
		JobID:             job.ID,
	})
	resource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
		JobID: job.ID,
		Type:  "coder_external_agent",
		Name:  localChatExternalResourceName,
	})
	agent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
		ResourceID: resource.ID,
		Name:       agentName,
		AuthToken:  agentToken,
	})
	return workspace.ID, agent.ID
}

func TestLocalChatTemplateArchiveForProvisionerTerraform(t *testing.T) {
	t.Parallel()

	service := newLocalMode(localModeOptions{})
	archiveBytes, err := service.localChatTemplateArchiveForProvisioner(
		context.Background(),
		codersdk.ProvisionerTypeTerraform,
	)
	require.NoError(t, err)

	reader := tar.NewReader(bytes.NewReader(archiveBytes))
	foundMainTF := false
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		if header.Name != "main.tf" {
			continue
		}
		foundMainTF = true
		content, err := io.ReadAll(reader)
		require.NoError(t, err)
		mainTF := string(content)
		require.Contains(t, mainTF, `resource "coder_agent" "localagent"`)
		require.Contains(t, mainTF, `resource "coder_external_agent" "main"`)
		require.Contains(t, mainTF, `agent_id = coder_agent.localagent.id`)
	}
	require.True(t, foundMainTF)
}

func TestResolveLocalChatExternalAgentFromWorkspaceResources(t *testing.T) {
	t.Parallel()

	agentID := uuid.New()
	resources := []codersdk.WorkspaceResource{
		{
			Type: "compute",
			Agents: []codersdk.WorkspaceAgent{{
				ID:   uuid.New(),
				Name: "ignored",
			}},
		},
		{
			Type: "coder_external_agent",
			Agents: []codersdk.WorkspaceAgent{{
				ID:     agentID,
				Name:   "external-agent",
				Status: codersdk.WorkspaceAgentDisconnected,
			}},
		},
	}

	service := newLocalMode(localModeOptions{})
	agent, err := service.resolveLocalChatExternalAgent(
		context.Background(),
		nil,
		codersdk.Workspace{
			ID: uuid.New(),
			LatestBuild: codersdk.WorkspaceBuild{
				Resources: resources,
			},
		},
	)
	require.NoError(t, err)
	require.Equal(t, agentID, agent.ID)
	require.Equal(t, "external-agent", agent.Name)
	require.Equal(t, codersdk.WorkspaceAgentDisconnected, agent.Status)
}

func TestResolveLocalChatExternalAgentWithoutLatestBuild(t *testing.T) {
	t.Parallel()

	db := localModeTestDB(t)
	workspaceID := uuid.New()

	service := newLocalMode(localModeOptions{database: db})
	_, err := service.resolveLocalChatExternalAgent(
		context.Background(),
		nil,
		codersdk.Workspace{
			ID: workspaceID,
			LatestBuild: codersdk.WorkspaceBuild{
				Resources: []codersdk.WorkspaceResource{{
					Type: "coder_external_agent",
				}},
			},
		},
	)
	require.ErrorContains(t, err, "has no latest build")
}

func TestResolveLocalChatExternalAgentFromWorkspaceAgentsInDB(t *testing.T) {
	t.Parallel()

	t.Run("PrefersNamedLocalAgent", func(t *testing.T) {
		t.Parallel()

		db := localModeTestDB(t)
		workspaceID, agents := seedWorkspaceWithLatestBuildAgents(
			t,
			db,
			"something-else",
			localChatExternalAgentName,
		)
		expectedID := agents[localChatExternalAgentName]

		service := newLocalMode(localModeOptions{database: db})
		agent, err := service.resolveLocalChatExternalAgent(
			context.Background(),
			nil,
			codersdk.Workspace{
				ID: workspaceID,
			},
		)
		require.NoError(t, err)
		require.Equal(t, expectedID, agent.ID)
		require.Equal(t, localChatExternalAgentName, agent.Name)
	})

	t.Run("FallsBackToFirstNamedAgent", func(t *testing.T) {
		t.Parallel()

		db := localModeTestDB(t)
		workspaceID, agents := seedWorkspaceWithLatestBuildAgents(
			t,
			db,
			"   ",
			"agent-a",
		)
		expectedID := agents["agent-a"]

		service := newLocalMode(localModeOptions{database: db})
		agent, err := service.resolveLocalChatExternalAgent(
			context.Background(),
			nil,
			codersdk.Workspace{
				ID: workspaceID,
			},
		)
		require.NoError(t, err)
		require.Equal(t, expectedID, agent.ID)
		require.Equal(t, "agent-a", agent.Name)
	})
}

func TestLocalChatAgentLaunchLimiter(t *testing.T) {
	t.Parallel()

	limiter := newLocalChatAgentLaunchLimiter(5 * time.Second)
	agentID := uuid.New()
	base := time.Unix(0, 0)

	require.True(t, limiter.Allow(agentID, base))
	require.False(t, limiter.Allow(agentID, base.Add(2*time.Second)))
	require.True(t, limiter.Allow(agentID, base.Add(6*time.Second)))
}

type localChatAgentRuntimeStub struct {
	closeCalls atomic.Int32
}

func (r *localChatAgentRuntimeStub) Close() error {
	r.closeCalls.Add(1)
	return nil
}

func TestLocalModeStartLocalChatAgentOnce(t *testing.T) {
	t.Parallel()

	var starts atomic.Int32
	service := newLocalMode(localModeOptions{
		startAgentFn: func(localChatAgentStartParams) (io.Closer, error) {
			starts.Add(1)
			return &localChatAgentRuntimeStub{}, nil
		},
	})

	params := localChatAgentStartParams{
		WorkspaceID: uuid.New(),
		AgentID:     uuid.New(),
		Credentials: codersdk.ExternalAgentCredentials{
			AgentToken: "test-token",
		},
	}

	const callers = 16
	var wg sync.WaitGroup
	wg.Add(callers)
	errCh := make(chan error, callers)
	for i := 0; i < callers; i++ {
		go func() {
			defer wg.Done()
			errCh <- service.startLocalChatAgent(params)
		}()
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		require.NoError(t, err)
	}
	require.Equal(t, int32(1), starts.Load())
}

func TestLocalModeRestartLocalChatAgentAfterClose(t *testing.T) {
	t.Parallel()

	var (
		starts   atomic.Int32
		runtimeM sync.Mutex
		runtimes []*localChatAgentRuntimeStub
	)
	service := newLocalMode(localModeOptions{
		startAgentFn: func(localChatAgentStartParams) (io.Closer, error) {
			starts.Add(1)
			runtime := &localChatAgentRuntimeStub{}
			runtimeM.Lock()
			runtimes = append(runtimes, runtime)
			runtimeM.Unlock()
			return runtime, nil
		},
	})

	params := localChatAgentStartParams{
		WorkspaceID: uuid.New(),
		AgentID:     uuid.New(),
		Credentials: codersdk.ExternalAgentCredentials{
			AgentToken: "test-token",
		},
	}

	require.NoError(t, service.startLocalChatAgent(params))
	require.Equal(t, int32(1), starts.Load())
	require.NoError(t, service.closeLocalChatAgent(params.AgentID))

	require.NoError(t, service.startLocalChatAgent(params))
	require.Equal(t, int32(2), starts.Load())

	runtimeM.Lock()
	require.Len(t, runtimes, 2)
	require.Equal(t, int32(1), runtimes[0].closeCalls.Load())
	require.Equal(t, int32(0), runtimes[1].closeCalls.Load())
	runtimeM.Unlock()
}

func TestLocalModeCloseAllLocalChatAgents(t *testing.T) {
	t.Parallel()

	var (
		runtimeM sync.Mutex
		runtimes []*localChatAgentRuntimeStub
	)
	service := newLocalMode(localModeOptions{
		startAgentFn: func(localChatAgentStartParams) (io.Closer, error) {
			runtime := &localChatAgentRuntimeStub{}
			runtimeM.Lock()
			runtimes = append(runtimes, runtime)
			runtimeM.Unlock()
			return runtime, nil
		},
	})

	first := localChatAgentStartParams{
		WorkspaceID: uuid.New(),
		AgentID:     uuid.New(),
		Credentials: codersdk.ExternalAgentCredentials{
			AgentToken: "test-token-1",
		},
	}
	second := localChatAgentStartParams{
		WorkspaceID: uuid.New(),
		AgentID:     uuid.New(),
		Credentials: codersdk.ExternalAgentCredentials{
			AgentToken: "test-token-2",
		},
	}

	require.NoError(t, service.startLocalChatAgent(first))
	require.NoError(t, service.startLocalChatAgent(second))
	require.NoError(t, service.closeAllLocalChatAgents())
	require.NoError(t, service.closeAllLocalChatAgents())

	runtimeM.Lock()
	require.Len(t, runtimes, 2)
	require.Equal(t, int32(1), runtimes[0].closeCalls.Load())
	require.Equal(t, int32(1), runtimes[1].closeCalls.Load())
	runtimeM.Unlock()
}

func TestEnsureLocalChatTemplateVersionProvisionable(t *testing.T) {
	t.Parallel()

	t.Run("NoEligibleDaemons", func(t *testing.T) {
		t.Parallel()

		db := localModeTestDB(t)
		org := dbgen.Organization(t, db, database.Organization{})
		jobID := uuid.New()
		dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			ID:             jobID,
			OrganizationID: org.ID,
			Provisioner:    database.ProvisionerTypeTerraform,
			Tags: database.StringMap{
				provisionersdk.TagScope: provisionersdk.ScopeOrganization,
				provisionersdk.TagOwner: "",
			},
		})

		service := newLocalMode(localModeOptions{database: db})
		err := service.ensureLocalChatTemplateVersionProvisionable(
			context.Background(),
			org.ID,
			jobID,
			codersdk.ProvisionerTypeTerraform,
		)
		require.Error(t, err)
		require.Contains(t, err.Error(), "no eligible provisioner daemons")
	})

	t.Run("OnlyOfflineEligibleDaemons", func(t *testing.T) {
		t.Parallel()

		db := localModeTestDB(t)
		org := dbgen.Organization(t, db, database.Organization{})
		jobID := uuid.New()
		dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			ID:             jobID,
			OrganizationID: org.ID,
			Provisioner:    database.ProvisionerTypeTerraform,
			Tags: database.StringMap{
				provisionersdk.TagScope: provisionersdk.ScopeOrganization,
				provisionersdk.TagOwner: "",
			},
		})
		dbgen.ProvisionerDaemon(t, db, database.ProvisionerDaemon{
			OrganizationID: org.ID,
			Provisioners:   []database.ProvisionerType{database.ProvisionerTypeTerraform},
			Tags: database.StringMap{
				provisionersdk.TagScope: provisionersdk.ScopeOrganization,
				provisionersdk.TagOwner: "",
			},
			LastSeenAt: sql.NullTime{
				Valid: true,
				Time:  time.Now().Add(-2 * time.Hour),
			},
		})

		service := newLocalMode(localModeOptions{database: db})
		err := service.ensureLocalChatTemplateVersionProvisionable(
			context.Background(),
			org.ID,
			jobID,
			codersdk.ProvisionerTypeTerraform,
		)
		require.Error(t, err)
		require.Contains(t, err.Error(), "no online eligible provisioner daemons")
	})

	t.Run("HasOnlineEligibleDaemon", func(t *testing.T) {
		t.Parallel()

		db := localModeTestDB(t)
		org := dbgen.Organization(t, db, database.Organization{})
		jobID := uuid.New()
		dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			ID:             jobID,
			OrganizationID: org.ID,
			Provisioner:    database.ProvisionerTypeTerraform,
			Tags: database.StringMap{
				provisionersdk.TagScope: provisionersdk.ScopeOrganization,
				provisionersdk.TagOwner: "",
			},
		})
		dbgen.ProvisionerDaemon(t, db, database.ProvisionerDaemon{
			OrganizationID: org.ID,
			Provisioners:   []database.ProvisionerType{database.ProvisionerTypeTerraform},
			Tags: database.StringMap{
				provisionersdk.TagScope: provisionersdk.ScopeOrganization,
				provisionersdk.TagOwner: "",
			},
		})

		service := newLocalMode(localModeOptions{database: db})
		err := service.ensureLocalChatTemplateVersionProvisionable(
			context.Background(),
			org.ID,
			jobID,
			codersdk.ProvisionerTypeTerraform,
		)
		require.NoError(t, err)
	})
}

func TestResolveLocalChatTemplateProvisioner(t *testing.T) {
	t.Parallel()

	t.Run("PreferTerraformWhenAvailable", func(t *testing.T) {
		t.Parallel()

		db := localModeTestDB(t)
		org := dbgen.Organization(t, db, database.Organization{})
		dbgen.ProvisionerDaemon(t, db, database.ProvisionerDaemon{
			OrganizationID: org.ID,
			Provisioners:   []database.ProvisionerType{database.ProvisionerTypeEcho},
		})
		dbgen.ProvisionerDaemon(t, db, database.ProvisionerDaemon{
			OrganizationID: org.ID,
			Provisioners:   []database.ProvisionerType{database.ProvisionerTypeTerraform},
		})

		service := newLocalMode(localModeOptions{database: db})
		provisioner, err := service.resolveLocalChatTemplateProvisioner(
			context.Background(),
			org.ID,
		)
		require.NoError(t, err)
		require.Equal(t, codersdk.ProvisionerTypeTerraform, provisioner)
	})

	t.Run("FallbackToEcho", func(t *testing.T) {
		t.Parallel()

		db := localModeTestDB(t)
		org := dbgen.Organization(t, db, database.Organization{})
		dbgen.ProvisionerDaemon(t, db, database.ProvisionerDaemon{
			OrganizationID: org.ID,
			Provisioners:   []database.ProvisionerType{database.ProvisionerTypeEcho},
		})

		service := newLocalMode(localModeOptions{database: db})
		provisioner, err := service.resolveLocalChatTemplateProvisioner(
			context.Background(),
			org.ID,
		)
		require.NoError(t, err)
		require.Equal(t, codersdk.ProvisionerTypeEcho, provisioner)
	})

	t.Run("NoSupportedProvisioner", func(t *testing.T) {
		t.Parallel()

		db := localModeTestDB(t)
		org := dbgen.Organization(t, db, database.Organization{})

		service := newLocalMode(localModeOptions{database: db})
		_, err := service.resolveLocalChatTemplateProvisioner(
			context.Background(),
			org.ID,
		)
		require.Error(t, err)
		require.Contains(t, err.Error(), "has no online provisioner daemons")
	})
}

func TestLocalChatMaybeLaunchAgentConnectedWithoutRuntimeStarts(t *testing.T) {
	t.Parallel()
	db := localModeTestDB(t)
	agentName := "local-agent"
	agentToken := uuid.New()
	workspaceID, agentID := seedWorkspaceWithLocalChatAgent(t, db, agentName, agentToken)

	var starts atomic.Int32
	service := newLocalMode(localModeOptions{
		database: db,
		startAgentFn: func(localChatAgentStartParams) (io.Closer, error) {
			starts.Add(1)
			return &localChatAgentRuntimeStub{}, nil
		},
	})
	err := service.maybeLaunchLocalChatAgent(
		context.Background(),
		workspaceID,
		localChatExternalAgent{
			ID:     agentID,
			Name:   agentName,
			Status: codersdk.WorkspaceAgentConnected,
		},
	)
	require.NoError(t, err)
	require.Equal(t, int32(1), starts.Load())
	require.True(t, service.hasRunningLocalChatAgent(agentID))
}

func TestLocalChatMaybeLaunchAgentSkipsWhenRuntimeAlreadyRunning(t *testing.T) {
	t.Parallel()
	workspaceID := uuid.New()
	agentID := uuid.New()
	agentName := "local-agent"
	db := localModeTestDB(t)

	var starts atomic.Int32
	service := newLocalMode(localModeOptions{
		database: db,
		startAgentFn: func(localChatAgentStartParams) (io.Closer, error) {
			starts.Add(1)
			return &localChatAgentRuntimeStub{}, nil
		},
	})

	require.NoError(t, service.startLocalChatAgent(localChatAgentStartParams{
		WorkspaceID: workspaceID,
		AgentID:     agentID,
		AgentName:   agentName,
		Credentials: codersdk.ExternalAgentCredentials{
			AgentToken: "existing-runtime-token",
		},
	}))
	require.True(t, service.hasRunningLocalChatAgent(agentID))
	require.Equal(t, int32(1), starts.Load())

	err := service.maybeLaunchLocalChatAgent(
		context.Background(),
		workspaceID,
		localChatExternalAgent{
			ID:     agentID,
			Name:   agentName,
			Status: codersdk.WorkspaceAgentDisconnected,
		},
	)
	require.NoError(t, err)
	require.Equal(t, int32(1), starts.Load())
}

func TestMaybeLaunchLocalChatAgentForChatLocalRunningRuntimeNoop(t *testing.T) {
	t.Parallel()
	workspaceID := uuid.New()
	agentID := uuid.New()
	agentName := "local-agent"

	db := localModeTestDB(t)

	var starts atomic.Int32
	service := newLocalMode(localModeOptions{
		database: db,
		startAgentFn: func(localChatAgentStartParams) (io.Closer, error) {
			starts.Add(1)
			return &localChatAgentRuntimeStub{}, nil
		},
	})

	require.NoError(t, service.startLocalChatAgent(localChatAgentStartParams{
		WorkspaceID: workspaceID,
		AgentID:     agentID,
		AgentName:   agentName,
		Credentials: codersdk.ExternalAgentCredentials{
			AgentToken: "existing-runtime-token",
		},
	}))
	require.Equal(t, int32(1), starts.Load())

	err := service.MaybeLaunchAgentForChat(context.Background(), database.Chat{
		ID:          uuid.New(),
		ModelConfig: json.RawMessage(`{"workspace_mode":"local"}`),
		WorkspaceID: uuid.NullUUID{
			UUID:  workspaceID,
			Valid: true,
		},
		WorkspaceAgentID: uuid.NullUUID{
			UUID:  agentID,
			Valid: true,
		},
	})
	require.NoError(t, err)
	require.Equal(t, int32(1), starts.Load())
}

func TestMaybeLaunchLocalChatAgentForChatLocalStartsRuntime(t *testing.T) {
	t.Parallel()
	db := localModeTestDB(t)
	agentName := "local-agent"
	agentToken := uuid.New()
	workspaceID, agentID := seedWorkspaceWithLocalChatAgent(t, db, agentName, agentToken)

	var starts atomic.Int32
	service := newLocalMode(localModeOptions{
		database: db,
		startAgentFn: func(localChatAgentStartParams) (io.Closer, error) {
			starts.Add(1)
			return &localChatAgentRuntimeStub{}, nil
		},
	})

	err := service.MaybeLaunchAgentForChat(context.Background(), database.Chat{
		ID:          uuid.New(),
		ModelConfig: json.RawMessage(`{"workspace_mode":"local"}`),
		WorkspaceID: uuid.NullUUID{
			UUID:  workspaceID,
			Valid: true,
		},
		WorkspaceAgentID: uuid.NullUUID{
			UUID:  agentID,
			Valid: true,
		},
	})
	require.NoError(t, err)
	require.Equal(t, int32(1), starts.Load())
	require.True(t, service.hasRunningLocalChatAgent(agentID))
}

func TestMaybeLaunchLocalChatAgentForChatNonLocalNoop(t *testing.T) {
	t.Parallel()
	db := localModeTestDB(t)

	var starts atomic.Int32
	service := newLocalMode(localModeOptions{
		database: db,
		startAgentFn: func(localChatAgentStartParams) (io.Closer, error) {
			starts.Add(1)
			return &localChatAgentRuntimeStub{}, nil
		},
	})
	err := service.MaybeLaunchAgentForChat(context.Background(), database.Chat{
		ID:          uuid.New(),
		ModelConfig: json.RawMessage(`{}`),
	})
	require.NoError(t, err)
	require.Equal(t, int32(0), starts.Load())
}

func TestProcessorEnsureLocalWorkspaceBindingRequiresLocalMode(t *testing.T) {
	t.Parallel()

	processor := &Processor{}
	_, err := processor.EnsureLocalWorkspaceBinding(
		context.Background(),
		uuid.New(),
		"session-token",
	)
	require.EqualError(t, err, "local chat mode is not configured")
}

func TestProcessorEnsureLocalAgentRuntimeForChatRequiresLocalMode(t *testing.T) {
	t.Parallel()

	processor := &Processor{}
	err := processor.EnsureLocalAgentRuntimeForChat(context.Background(), database.Chat{
		ModelConfig: json.RawMessage(`{"workspace_mode":"local"}`),
	})
	require.EqualError(t, err, "local chat mode is not configured")
}

func TestProcessorEnsureLocalAgentRuntimeForChatNonLocalNoop(t *testing.T) {
	t.Parallel()

	processor := &Processor{}
	err := processor.EnsureLocalAgentRuntimeForChat(context.Background(), database.Chat{
		ModelConfig: json.RawMessage(`{}`),
	})
	require.NoError(t, err)
}

func TestProcessorCloseClosesLocalMode(t *testing.T) {
	t.Parallel()

	runtime := &localChatAgentRuntimeStub{}
	service := newLocalMode(localModeOptions{
		startAgentFn: func(localChatAgentStartParams) (io.Closer, error) {
			return runtime, nil
		},
	})

	agentID := uuid.New()
	require.NoError(t, service.startLocalChatAgent(localChatAgentStartParams{
		WorkspaceID: uuid.New(),
		AgentID:     agentID,
		Credentials: codersdk.ExternalAgentCredentials{
			AgentToken: "test-token",
		},
		AgentName: localChatExternalAgentName,
	}))
	require.True(t, service.hasRunningLocalChatAgent(agentID))

	_, cancel := context.WithCancel(context.Background())
	processor := &Processor{
		cancel:    cancel,
		closed:    make(chan struct{}),
		localMode: service,
	}
	close(processor.closed)

	require.NoError(t, processor.Close())
	require.Equal(t, int32(1), runtime.closeCalls.Load())
}

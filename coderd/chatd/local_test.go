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
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/codersdk"
)

func TestLocalChatNameFunctions(t *testing.T) {
	t.Parallel()

	ownerID := uuid.MustParse("12345678-90ab-cdef-1234-567890abcdef")

	require.Equal(t, "12345678", localChatNameSuffix(ownerID))
	require.Equal(t, "chat-local-tpl-12345678", localChatTemplateName(ownerID))
	require.Equal(t, "chat-local-ws-12345678", localChatWorkspaceName(ownerID))
}

func TestLocalChatTerraformTemplateArchive(t *testing.T) {
	t.Parallel()

	archiveBytes, err := localChatTerraformTemplateArchive("local-agent")
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

func TestSanitizeLocalChatTerraformIdentifier(t *testing.T) {
	t.Parallel()

	require.Equal(t, "localagent", sanitizeLocalChatTerraformIdentifier("local-agent"))
	require.Equal(t, "a123abc", sanitizeLocalChatTerraformIdentifier("123abc"))
	require.Equal(t, "agentname", sanitizeLocalChatTerraformIdentifier("  agent-name  "))
	require.Equal(t, "mixedcase", sanitizeLocalChatTerraformIdentifier("MixedCase"))
	require.Equal(t, "", sanitizeLocalChatTerraformIdentifier("!!!"))
}

func TestLocalChatExternalAgentFromResources(t *testing.T) {
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

	agent, ok := localChatExternalAgentFromResources(resources)
	require.True(t, ok)
	require.Equal(t, agentID, agent.ID)
	require.Equal(t, "external-agent", agent.Name)
	require.Equal(t, codersdk.WorkspaceAgentDisconnected, agent.Status)

	_, ok = localChatExternalAgentFromResources([]codersdk.WorkspaceResource{{
		Type: "coder_external_agent",
	}})
	require.False(t, ok)
}

func TestLocalChatExternalAgentFromWorkspaceAgentsInDB(t *testing.T) {
	t.Parallel()

	t.Run("PrefersNamedLocalAgent", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		workspaceID := uuid.New()
		expectedID := uuid.New()
		db.EXPECT().GetWorkspaceAgentsInLatestBuildByWorkspaceID(
			gomock.Any(),
			workspaceID,
		).Return(
			[]database.WorkspaceAgent{
				{ID: uuid.New(), Name: "something-else"},
				{ID: expectedID, Name: localChatExternalAgentName},
			},
			nil,
		)

		service := NewLocalService(LocalServiceOptions{Database: db})
		agent, ok, err := service.localChatExternalAgentFromWorkspaceAgentsInDB(
			context.Background(),
			workspaceID,
		)
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, expectedID, agent.ID)
		require.Equal(t, localChatExternalAgentName, agent.Name)
	})

	t.Run("FallsBackToFirstNamedAgent", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		workspaceID := uuid.New()
		expectedID := uuid.New()
		db.EXPECT().GetWorkspaceAgentsInLatestBuildByWorkspaceID(
			gomock.Any(),
			workspaceID,
		).Return(
			[]database.WorkspaceAgent{
				{ID: uuid.New(), Name: "   "},
				{ID: expectedID, Name: "agent-a"},
			},
			nil,
		)

		service := NewLocalService(LocalServiceOptions{Database: db})
		agent, ok, err := service.localChatExternalAgentFromWorkspaceAgentsInDB(
			context.Background(),
			workspaceID,
		)
		require.NoError(t, err)
		require.True(t, ok)
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

func TestLocalChatAgentManagerStartOnce(t *testing.T) {
	t.Parallel()

	var starts atomic.Int32
	manager := newLocalChatAgentManager(func(localChatAgentStartParams) (io.Closer, error) {
		starts.Add(1)
		return &localChatAgentRuntimeStub{}, nil
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
			errCh <- manager.Start(params)
		}()
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		require.NoError(t, err)
	}
	require.Equal(t, int32(1), starts.Load())
}

func TestLocalChatAgentManagerRestartAfterClose(t *testing.T) {
	t.Parallel()

	var (
		starts   atomic.Int32
		runtimeM sync.Mutex
		runtimes []*localChatAgentRuntimeStub
	)
	manager := newLocalChatAgentManager(func(localChatAgentStartParams) (io.Closer, error) {
		starts.Add(1)
		runtime := &localChatAgentRuntimeStub{}
		runtimeM.Lock()
		runtimes = append(runtimes, runtime)
		runtimeM.Unlock()
		return runtime, nil
	})

	params := localChatAgentStartParams{
		WorkspaceID: uuid.New(),
		AgentID:     uuid.New(),
		Credentials: codersdk.ExternalAgentCredentials{
			AgentToken: "test-token",
		},
	}

	require.NoError(t, manager.Start(params))
	require.Equal(t, int32(1), starts.Load())
	require.NoError(t, manager.Close(params.AgentID))

	require.NoError(t, manager.Start(params))
	require.Equal(t, int32(2), starts.Load())

	runtimeM.Lock()
	require.Len(t, runtimes, 2)
	require.Equal(t, int32(1), runtimes[0].closeCalls.Load())
	require.Equal(t, int32(0), runtimes[1].closeCalls.Load())
	runtimeM.Unlock()
}

func TestLocalChatAgentManagerCloseAll(t *testing.T) {
	t.Parallel()

	var (
		runtimeM sync.Mutex
		runtimes []*localChatAgentRuntimeStub
	)
	manager := newLocalChatAgentManager(func(localChatAgentStartParams) (io.Closer, error) {
		runtime := &localChatAgentRuntimeStub{}
		runtimeM.Lock()
		runtimes = append(runtimes, runtime)
		runtimeM.Unlock()
		return runtime, nil
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

	require.NoError(t, manager.Start(first))
	require.NoError(t, manager.Start(second))
	require.NoError(t, manager.CloseAll())
	require.NoError(t, manager.CloseAll())

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

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		orgID := uuid.New()
		jobID := uuid.New()
		db.EXPECT().GetEligibleProvisionerDaemonsByProvisionerJobIDs(
			gomock.Any(),
			[]uuid.UUID{jobID},
		).Return(nil, nil)

		service := NewLocalService(LocalServiceOptions{Database: db})
		err := service.ensureLocalChatTemplateVersionProvisionable(
			context.Background(),
			orgID,
			jobID,
			codersdk.ProvisionerTypeTerraform,
		)
		require.Error(t, err)
		require.Contains(t, err.Error(), "no eligible provisioner daemons")
	})

	t.Run("OnlyOfflineEligibleDaemons", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		orgID := uuid.New()
		jobID := uuid.New()
		db.EXPECT().GetEligibleProvisionerDaemonsByProvisionerJobIDs(
			gomock.Any(),
			[]uuid.UUID{jobID},
		).Return(
			[]database.GetEligibleProvisionerDaemonsByProvisionerJobIDsRow{{
				JobID: jobID,
				ProvisionerDaemon: database.ProvisionerDaemon{
					ID: uuid.New(),
					LastSeenAt: sql.NullTime{
						Valid: true,
						Time:  time.Now().Add(-2 * time.Hour),
					},
				},
			}},
			nil,
		)

		service := NewLocalService(LocalServiceOptions{Database: db})
		err := service.ensureLocalChatTemplateVersionProvisionable(
			context.Background(),
			orgID,
			jobID,
			codersdk.ProvisionerTypeTerraform,
		)
		require.Error(t, err)
		require.Contains(t, err.Error(), "no online eligible provisioner daemons")
	})

	t.Run("HasOnlineEligibleDaemon", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		orgID := uuid.New()
		jobID := uuid.New()
		db.EXPECT().GetEligibleProvisionerDaemonsByProvisionerJobIDs(
			gomock.Any(),
			[]uuid.UUID{jobID},
		).Return(
			[]database.GetEligibleProvisionerDaemonsByProvisionerJobIDsRow{{
				JobID: jobID,
				ProvisionerDaemon: database.ProvisionerDaemon{
					ID: uuid.New(),
					LastSeenAt: sql.NullTime{
						Valid: true,
						Time:  time.Now(),
					},
				},
			}},
			nil,
		)

		service := NewLocalService(LocalServiceOptions{Database: db})
		err := service.ensureLocalChatTemplateVersionProvisionable(
			context.Background(),
			orgID,
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

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		orgID := uuid.New()
		db.EXPECT().GetProvisionerDaemonsWithStatusByOrganization(
			gomock.Any(),
			gomock.Any(),
		).Return(
			[]database.GetProvisionerDaemonsWithStatusByOrganizationRow{
				{
					ProvisionerDaemon: database.ProvisionerDaemon{
						Provisioners: []database.ProvisionerType{
							database.ProvisionerTypeEcho,
						},
					},
				},
				{
					ProvisionerDaemon: database.ProvisionerDaemon{
						Provisioners: []database.ProvisionerType{
							database.ProvisionerTypeTerraform,
						},
					},
				},
			},
			nil,
		)

		service := NewLocalService(LocalServiceOptions{Database: db})
		provisioner, err := service.resolveLocalChatTemplateProvisioner(
			context.Background(),
			orgID,
		)
		require.NoError(t, err)
		require.Equal(t, codersdk.ProvisionerTypeTerraform, provisioner)
	})

	t.Run("FallbackToEcho", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		orgID := uuid.New()
		db.EXPECT().GetProvisionerDaemonsWithStatusByOrganization(
			gomock.Any(),
			gomock.Any(),
		).Return(
			[]database.GetProvisionerDaemonsWithStatusByOrganizationRow{
				{
					ProvisionerDaemon: database.ProvisionerDaemon{
						Provisioners: []database.ProvisionerType{
							database.ProvisionerTypeEcho,
						},
					},
				},
			},
			nil,
		)

		service := NewLocalService(LocalServiceOptions{Database: db})
		provisioner, err := service.resolveLocalChatTemplateProvisioner(
			context.Background(),
			orgID,
		)
		require.NoError(t, err)
		require.Equal(t, codersdk.ProvisionerTypeEcho, provisioner)
	})

	t.Run("NoSupportedProvisioner", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		orgID := uuid.New()
		db.EXPECT().GetProvisionerDaemonsWithStatusByOrganization(
			gomock.Any(),
			gomock.Any(),
		).Return(nil, nil)

		service := NewLocalService(LocalServiceOptions{Database: db})
		_, err := service.resolveLocalChatTemplateProvisioner(
			context.Background(),
			orgID,
		)
		require.Error(t, err)
		require.Contains(t, err.Error(), "has no online provisioner daemons")
	})
}

func TestLocalChatMaybeLaunchAgentConnectedWithoutRuntimeStarts(t *testing.T) {
	workspaceID := uuid.New()
	agentID := uuid.New()
	agentName := "local-agent"
	agentToken := uuid.New()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	db.EXPECT().GetWorkspaceAgentAndWorkspaceByID(gomock.Any(), agentID).Return(
		database.GetWorkspaceAgentAndWorkspaceByIDRow{
			WorkspaceAgent: database.WorkspaceAgent{
				ID:        agentID,
				Name:      agentName,
				AuthToken: agentToken,
			},
			WorkspaceTable: database.WorkspaceTable{ID: workspaceID},
		},
		nil,
	)

	var starts atomic.Int32
	service := NewLocalService(LocalServiceOptions{
		Database: db,
		AgentStarter: func(localChatAgentStartParams) (io.Closer, error) {
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
	require.True(t, service.localChatAgents.HasRunning(agentID))
}

func TestLocalChatMaybeLaunchAgentSkipsWhenRuntimeAlreadyRunning(t *testing.T) {
	workspaceID := uuid.New()
	agentID := uuid.New()
	agentName := "local-agent"
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	var starts atomic.Int32
	service := NewLocalService(LocalServiceOptions{
		Database: db,
		AgentStarter: func(localChatAgentStartParams) (io.Closer, error) {
			starts.Add(1)
			return &localChatAgentRuntimeStub{}, nil
		},
	})

	require.NoError(t, service.localChatAgents.Start(localChatAgentStartParams{
		WorkspaceID: workspaceID,
		AgentID:     agentID,
		AgentName:   agentName,
		Credentials: codersdk.ExternalAgentCredentials{
			AgentToken: "existing-runtime-token",
		},
	}))
	require.True(t, service.localChatAgents.HasRunning(agentID))
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
	workspaceID := uuid.New()
	agentID := uuid.New()
	agentName := "local-agent"

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	var starts atomic.Int32
	service := NewLocalService(LocalServiceOptions{
		Database: db,
		AgentStarter: func(localChatAgentStartParams) (io.Closer, error) {
			starts.Add(1)
			return &localChatAgentRuntimeStub{}, nil
		},
	})

	require.NoError(t, service.localChatAgents.Start(localChatAgentStartParams{
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
	workspaceID := uuid.New()
	agentID := uuid.New()
	agentName := "local-agent"
	agentToken := uuid.New()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	row := database.GetWorkspaceAgentAndWorkspaceByIDRow{
		WorkspaceAgent: database.WorkspaceAgent{
			ID:        agentID,
			Name:      agentName,
			AuthToken: agentToken,
		},
		WorkspaceTable: database.WorkspaceTable{ID: workspaceID},
	}
	db.EXPECT().GetWorkspaceAgentAndWorkspaceByID(gomock.Any(), agentID).Return(row, nil).Times(2)

	var starts atomic.Int32
	service := NewLocalService(LocalServiceOptions{
		Database: db,
		AgentStarter: func(localChatAgentStartParams) (io.Closer, error) {
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
	require.True(t, service.localChatAgents.HasRunning(agentID))
}

func TestMaybeLaunchLocalChatAgentForChatNonLocalNoop(t *testing.T) {
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	var starts atomic.Int32
	service := NewLocalService(LocalServiceOptions{
		Database: db,
		AgentStarter: func(localChatAgentStartParams) (io.Closer, error) {
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

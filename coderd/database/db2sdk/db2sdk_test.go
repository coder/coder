package db2sdk_test

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gomock "go.uber.org/mock/gomock"
	"tailscale.com/tailcfg"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/tailnet"
	tailnetproto "github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/coder/v2/tailnet/tailnettest"
)

func TestProvisionerJobStatus(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		job    database.ProvisionerJob
		status codersdk.ProvisionerJobStatus
	}{
		{
			name: "canceling",
			job: database.ProvisionerJob{
				CanceledAt: sql.NullTime{
					Time:  dbtime.Now().Add(-time.Minute),
					Valid: true,
				},
			},
			status: codersdk.ProvisionerJobCanceling,
		},
		{
			name: "canceled",
			job: database.ProvisionerJob{
				CanceledAt: sql.NullTime{
					Time:  dbtime.Now().Add(-time.Minute),
					Valid: true,
				},
				CompletedAt: sql.NullTime{
					Time:  dbtime.Now().Add(-30 * time.Second),
					Valid: true,
				},
			},
			status: codersdk.ProvisionerJobCanceled,
		},
		{
			name: "canceled_failed",
			job: database.ProvisionerJob{
				CanceledAt: sql.NullTime{
					Time:  dbtime.Now().Add(-time.Minute),
					Valid: true,
				},
				CompletedAt: sql.NullTime{
					Time:  dbtime.Now().Add(-30 * time.Second),
					Valid: true,
				},
				Error: sql.NullString{String: "badness", Valid: true},
			},
			status: codersdk.ProvisionerJobFailed,
		},
		{
			name:   "pending",
			job:    database.ProvisionerJob{},
			status: codersdk.ProvisionerJobPending,
		},
		{
			name: "succeeded",
			job: database.ProvisionerJob{
				StartedAt: sql.NullTime{
					Time:  dbtime.Now().Add(-time.Minute),
					Valid: true,
				},
				CompletedAt: sql.NullTime{
					Time:  dbtime.Now().Add(-30 * time.Second),
					Valid: true,
				},
			},
			status: codersdk.ProvisionerJobSucceeded,
		},
		{
			name: "completed_failed",
			job: database.ProvisionerJob{
				StartedAt: sql.NullTime{
					Time:  dbtime.Now().Add(-time.Minute),
					Valid: true,
				},
				CompletedAt: sql.NullTime{
					Time:  dbtime.Now().Add(-30 * time.Second),
					Valid: true,
				},
				Error: sql.NullString{String: "badness", Valid: true},
			},
			status: codersdk.ProvisionerJobFailed,
		},
		{
			name: "updated",
			job: database.ProvisionerJob{
				StartedAt: sql.NullTime{
					Time:  dbtime.Now().Add(-time.Minute),
					Valid: true,
				},
				UpdatedAt: dbtime.Now(),
			},
			status: codersdk.ProvisionerJobRunning,
		},
	}

	// Share db for all job inserts.
	db, _ := dbtestutil.NewDB(t)
	org := dbgen.Organization(t, db, database.Organization{})

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Populate standard fields
			now := dbtime.Now().Round(time.Minute)
			tc.job.ID = uuid.New()
			tc.job.CreatedAt = now
			tc.job.UpdatedAt = now
			tc.job.InitiatorID = org.ID
			tc.job.OrganizationID = org.ID
			tc.job.Input = []byte("{}")
			tc.job.Provisioner = database.ProvisionerTypeEcho
			// Unique tags for each job.
			tc.job.Tags = map[string]string{fmt.Sprintf("%d", i): "true"}

			inserted := dbgen.ProvisionerJob(t, db, nil, tc.job)
			// Make sure the inserted job has the right values.
			require.Equal(t, tc.job.StartedAt.Time.UTC(), inserted.StartedAt.Time.UTC(), "started at")
			require.Equal(t, tc.job.CompletedAt.Time.UTC(), inserted.CompletedAt.Time.UTC(), "completed at")
			require.Equal(t, tc.job.CanceledAt.Time.UTC(), inserted.CanceledAt.Time.UTC(), "canceled at")
			require.Equal(t, tc.job.Error, inserted.Error, "error")
			require.Equal(t, tc.job.ErrorCode, inserted.ErrorCode, "error code")

			actual := codersdk.ProvisionerJobStatus(inserted.JobStatus)
			require.Equal(t, tc.status, actual)
		})
	}
}

func TestTemplateVersionParameter_OK(t *testing.T) {
	t.Parallel()
	req := require.New(t)

	// In this test we're just going to cover the fields that have to get parsed.
	options := []*proto.RichParameterOption{
		{
			Name:        "foo",
			Description: "bar",
			Value:       "baz",
			Icon:        "David Bowie",
		},
	}
	ob, err := json.Marshal(&options)
	req.NoError(err)

	db := database.TemplateVersionParameter{
		Options:     json.RawMessage(ob),
		Description: "_The Rise and Fall of **Ziggy Stardust** and the Spiders from Mars_",
	}
	sdk, err := db2sdk.TemplateVersionParameter(db)
	req.NoError(err)
	req.Len(sdk.Options, 1)
	req.Equal("foo", sdk.Options[0].Name)
	req.Equal("bar", sdk.Options[0].Description)
	req.Equal("baz", sdk.Options[0].Value)
	req.Equal("David Bowie", sdk.Options[0].Icon)
	req.Equal("The Rise and Fall of Ziggy Stardust and the Spiders from Mars", sdk.DescriptionPlaintext)
}

func TestTemplateVersionParameter_BadOptions(t *testing.T) {
	t.Parallel()
	req := require.New(t)

	db := database.TemplateVersionParameter{
		Options:     json.RawMessage("not really JSON!"),
		Description: "_The Rise and Fall of **Ziggy Stardust** and the Spiders from Mars_",
	}
	_, err := db2sdk.TemplateVersionParameter(db)
	req.Error(err)
}

func TestTemplateVersionParameter_BadDescription(t *testing.T) {
	t.Parallel()
	req := require.New(t)
	desc := make([]byte, 300)
	_, err := rand.Read(desc)
	req.NoError(err)

	db := database.TemplateVersionParameter{
		Options:     json.RawMessage("[]"),
		Description: string(desc),
	}
	sdk, err := db2sdk.TemplateVersionParameter(db)
	// Although the markdown parser can return an error, the way we use it should not, even
	// if we feed it garbage data.
	req.NoError(err)
	req.NotEmpty(sdk.DescriptionPlaintext, "broke the markdown parser with %v", desc)
}

func TestWorkspaceAgent_TunnelPeers(t *testing.T) {
	t.Parallel()

	t.Run("MultiplePeers", func(t *testing.T) {
		t.Parallel()

		agentID := uuid.New()
		now := dbtime.Now()
		peerID1 := uuid.New()
		peerID2 := uuid.New()

		ctrl := gomock.NewController(t)
		mockCoord := tailnettest.NewMockCoordinator(ctrl)
		mockCoord.EXPECT().Node(agentID).Return(nil)
		mockCoord.EXPECT().TunnelPeers(agentID).Return([]*tailnet.TunnelPeerInfo{
			{
				ID:   peerID1,
				Name: "active-user",
				Node: &tailnetproto.Node{
					Addresses:     []string{"fd60:627a:a42b:0102:0304:0506:0708:090a/128"},
					PreferredDerp: 1,
				},
				Status: tailnetproto.CoordinateResponse_PeerUpdate_NODE,
				Start:  now,
			},
			{
				ID:   peerID2,
				Name: "lost-user",
				Node: &tailnetproto.Node{
					Addresses:     []string{"fd60:627a:a42b:aaaa:bbbb:cccc:dddd:eeee/128"},
					PreferredDerp: 2,
				},
				Status: tailnetproto.CoordinateResponse_PeerUpdate_LOST,
				Start:  now.Add(-time.Hour),
			},
		})

		dbAgent := database.WorkspaceAgent{
			ID:        agentID,
			CreatedAt: now,
			Name:      "test-agent",
		}

		agent, err := db2sdk.WorkspaceAgent(
			&tailcfg.DERPMap{},
			mockCoord,
			dbAgent,
			nil, nil, nil,
			time.Minute,
			"",
		)
		require.NoError(t, err)
		require.Len(t, agent.Connections, 2)

		// Find connections by status since order is not guaranteed.
		var ongoing, lost *codersdk.WorkspaceConnection
		for i := range agent.Connections {
			switch agent.Connections[i].Status {
			case codersdk.ConnectionStatusOngoing:
				ongoing = &agent.Connections[i]
			case codersdk.ConnectionStatusControlLost:
				lost = &agent.Connections[i]
			}
		}

		require.NotNil(t, ongoing, "expected an ongoing connection")
		require.NotNil(t, ongoing.IP, "expected ongoing IP to be set")
			assert.Equal(t, "fd60:627a:a42b:102:304:506:708:90a", ongoing.IP.String())
		assert.Equal(t, now, ongoing.CreatedAt)
		require.NotNil(t, ongoing.ConnectedAt)
		assert.Equal(t, now, *ongoing.ConnectedAt)
		assert.Nil(t, ongoing.EndedAt)

		require.NotNil(t, lost, "expected a control_lost connection")
		require.NotNil(t, lost.IP, "expected lost IP to be set")
			assert.Equal(t, "fd60:627a:a42b:aaaa:bbbb:cccc:dddd:eeee", lost.IP.String())
		assert.Equal(t, now.Add(-time.Hour), lost.CreatedAt)
		require.NotNil(t, lost.ConnectedAt)
		assert.Equal(t, now.Add(-time.Hour), *lost.ConnectedAt)
		assert.Nil(t, lost.EndedAt)
	})

	t.Run("NilNode", func(t *testing.T) {
		t.Parallel()

		agentID := uuid.New()
		now := dbtime.Now()

		ctrl := gomock.NewController(t)
		mockCoord := tailnettest.NewMockCoordinator(ctrl)
		mockCoord.EXPECT().Node(agentID).Return(nil)
		mockCoord.EXPECT().TunnelPeers(agentID).Return([]*tailnet.TunnelPeerInfo{
			{
				ID:     uuid.New(),
				Name:   "no-node-user",
				Node:   nil,
				Status: tailnetproto.CoordinateResponse_PeerUpdate_NODE,
				Start:  now,
			},
		})

		dbAgent := database.WorkspaceAgent{
			ID:        agentID,
			CreatedAt: now,
			Name:      "test-agent",
		}

		agent, err := db2sdk.WorkspaceAgent(
			&tailcfg.DERPMap{},
			mockCoord,
			dbAgent,
			nil, nil, nil,
			time.Minute,
			"",
		)
		require.NoError(t, err)
		require.Len(t, agent.Connections, 1)
		assert.Nil(t, agent.Connections[0].IP, "IP should be nil when Node is nil")
		assert.Equal(t, codersdk.ConnectionStatusOngoing, agent.Connections[0].Status)
	})

	t.Run("NoPeers", func(t *testing.T) {
		t.Parallel()

		agentID := uuid.New()
		now := dbtime.Now()

		ctrl := gomock.NewController(t)
		mockCoord := tailnettest.NewMockCoordinator(ctrl)
		mockCoord.EXPECT().Node(agentID).Return(nil)
		mockCoord.EXPECT().TunnelPeers(agentID).Return(nil)

		dbAgent := database.WorkspaceAgent{
			ID:        agentID,
			CreatedAt: now,
			Name:      "test-agent",
		}

		agent, err := db2sdk.WorkspaceAgent(
			&tailcfg.DERPMap{},
			mockCoord,
			dbAgent,
			nil, nil, nil,
			time.Minute,
			"",
		)
		require.NoError(t, err)
		assert.Empty(t, agent.Connections)
	})
}

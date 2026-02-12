package coderd

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/wspubsub"
	tailnetproto "github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/coder/v2/testutil"
)

func TestHandleIdentifiedTelemetry(t *testing.T) {
	t.Parallel()

	t.Run("PublishesWorkspaceUpdate", func(t *testing.T) {
		t.Parallel()

		api, dbM, ps := newIdentifiedTelemetryTestAPI(t)
		ownerID := uuid.New()
		workspaceID := uuid.New()
		agentID := uuid.New()
		peerID := uuid.New()

		dbM.EXPECT().GetWorkspaceByAgentID(gomock.Any(), agentID).Return(database.Workspace{
			ID:      workspaceID,
			OwnerID: ownerID,
		}, nil)

		events, errs := subscribeWorkspaceEvents(t, ps, ownerID)

		api.handleIdentifiedTelemetry(agentID, peerID, []*tailnetproto.TelemetryEvent{{
			Status: tailnetproto.TelemetryEvent_CONNECTED,
		}})

		select {
		case err := <-errs:
			require.NoError(t, err)
		default:
		}

		select {
		case event := <-events:
			require.Equal(t, wspubsub.WorkspaceEventKindConnectionLogUpdate, event.Kind)
			require.Equal(t, workspaceID, event.WorkspaceID)
			require.NotNil(t, event.AgentID)
			require.Equal(t, agentID, *event.AgentID)
		case err := <-errs:
			require.NoError(t, err)
		case <-time.After(testutil.IntervalMedium):
			t.Fatal("timed out waiting for workspace event")
		}

		require.NotNil(t, api.PeerNetworkTelemetryStore.Get(agentID, peerID))
	})

	t.Run("EmptyBatchNoPublish", func(t *testing.T) {
		t.Parallel()

		api, _, ps := newIdentifiedTelemetryTestAPI(t)
		events, errs := subscribeWorkspaceEvents(t, ps, uuid.Nil)

		agentID := uuid.New()
		peerID := uuid.New()
		api.handleIdentifiedTelemetry(agentID, peerID, []*tailnetproto.TelemetryEvent{})

		select {
		case event := <-events:
			t.Fatalf("unexpected workspace event: %+v", event)
		case err := <-errs:
			t.Fatalf("unexpected pubsub error: %v", err)
		case <-time.After(testutil.IntervalFast):
		}

		require.Nil(t, api.PeerNetworkTelemetryStore.Get(agentID, peerID))
	})

	t.Run("LookupFailureNoPublish", func(t *testing.T) {
		t.Parallel()

		api, dbM, ps := newIdentifiedTelemetryTestAPI(t)
		agentID := uuid.New()
		peerID := uuid.New()
		dbM.EXPECT().GetWorkspaceByAgentID(gomock.Any(), agentID).Return(database.Workspace{}, xerrors.New("lookup failed"))

		events, errs := subscribeWorkspaceEvents(t, ps, uuid.Nil)

		api.handleIdentifiedTelemetry(agentID, peerID, []*tailnetproto.TelemetryEvent{{
			Status: tailnetproto.TelemetryEvent_CONNECTED,
		}})

		select {
		case event := <-events:
			t.Fatalf("unexpected workspace event: %+v", event)
		case err := <-errs:
			t.Fatalf("unexpected pubsub error: %v", err)
		case <-time.After(testutil.IntervalFast):
		}

		require.NotNil(t, api.PeerNetworkTelemetryStore.Get(agentID, peerID))
	})
}

func newIdentifiedTelemetryTestAPI(t *testing.T) (*API, *dbmock.MockStore, pubsub.Pubsub) {
	t.Helper()

	dbM := dbmock.NewMockStore(gomock.NewController(t))
	ps := pubsub.NewInMemory()

	api := &API{
		Options: &Options{
			Database: dbM,
			Pubsub:   ps,
			Logger:   slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
		},
		PeerNetworkTelemetryStore: NewPeerNetworkTelemetryStore(),
	}

	return api, dbM, ps
}

func subscribeWorkspaceEvents(t *testing.T, ps pubsub.Pubsub, ownerID uuid.UUID) (<-chan wspubsub.WorkspaceEvent, <-chan error) {
	t.Helper()

	events := make(chan wspubsub.WorkspaceEvent, 1)
	errs := make(chan error, 1)
	cancel, err := ps.SubscribeWithErr(wspubsub.WorkspaceEventChannel(ownerID), wspubsub.HandleWorkspaceEvent(
		func(_ context.Context, event wspubsub.WorkspaceEvent, err error) {
			if err != nil {
				select {
				case errs <- err:
				default:
				}
				return
			}
			select {
			case events <- event:
			default:
			}
		},
	))
	require.NoError(t, err)
	t.Cleanup(cancel)

	return events, errs
}

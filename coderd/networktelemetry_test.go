package coderd_test

import (
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/coder/coder/v2/coderd"
	tailnetproto "github.com/coder/coder/v2/tailnet/proto"
)

func TestPeerNetworkTelemetryStore(t *testing.T) {
	t.Parallel()

	t.Run("UpdateAndGet", func(t *testing.T) {
		t.Parallel()

		store := coderd.NewPeerNetworkTelemetryStore()
		agentID := uuid.New()
		peerID := uuid.New()
		store.Update(agentID, peerID, &tailnetproto.TelemetryEvent{
			Status:     tailnetproto.TelemetryEvent_CONNECTED,
			P2PLatency: durationpb.New(50 * time.Millisecond),
			HomeDerp:   1,
		})

		got := store.Get(agentID, peerID)
		require.NotNil(t, got)
		require.NotNil(t, got.P2P)
		require.True(t, *got.P2P)
		require.NotNil(t, got.P2PLatency)
		require.Equal(t, 50*time.Millisecond, *got.P2PLatency)
		require.Equal(t, 1, got.HomeDERP)
	})

	t.Run("GetMissing", func(t *testing.T) {
		t.Parallel()

		store := coderd.NewPeerNetworkTelemetryStore()
		require.Nil(t, store.Get(uuid.New(), uuid.New()))
	})

	t.Run("LatestWins", func(t *testing.T) {
		t.Parallel()

		store := coderd.NewPeerNetworkTelemetryStore()
		agentID := uuid.New()
		peerID := uuid.New()

		store.Update(agentID, peerID, &tailnetproto.TelemetryEvent{
			Status:     tailnetproto.TelemetryEvent_CONNECTED,
			P2PLatency: durationpb.New(10 * time.Millisecond),
			HomeDerp:   1,
		})
		store.Update(agentID, peerID, &tailnetproto.TelemetryEvent{
			Status:      tailnetproto.TelemetryEvent_CONNECTED,
			DerpLatency: durationpb.New(75 * time.Millisecond),
			HomeDerp:    2,
		})

		got := store.Get(agentID, peerID)
		require.NotNil(t, got)
		require.NotNil(t, got.P2P)
		require.False(t, *got.P2P)
		require.NotNil(t, got.DERPLatency)
		require.Equal(t, 75*time.Millisecond, *got.DERPLatency)
		require.Nil(t, got.P2PLatency)
		require.Equal(t, 2, got.HomeDERP)
	})

	t.Run("ConnectedWithoutLatencyPreservesExistingModeAndLatency", func(t *testing.T) {
		t.Parallel()

		store := coderd.NewPeerNetworkTelemetryStore()
		agentID := uuid.New()
		peerID := uuid.New()
		store.Update(agentID, peerID, &tailnetproto.TelemetryEvent{
			Status:     tailnetproto.TelemetryEvent_CONNECTED,
			P2PLatency: durationpb.New(50 * time.Millisecond),
			HomeDerp:   1,
		})
		store.Update(agentID, peerID, &tailnetproto.TelemetryEvent{
			Status: tailnetproto.TelemetryEvent_CONNECTED,
		})

		got := store.Get(agentID, peerID)
		require.NotNil(t, got)
		require.NotNil(t, got.P2P)
		require.True(t, *got.P2P)
		require.NotNil(t, got.P2PLatency)
		require.Equal(t, 50*time.Millisecond, *got.P2PLatency)
		require.Equal(t, 1, got.HomeDERP)
	})

	t.Run("ConnectedWithHomeDerpZeroPreservesPreviousHomeDerp", func(t *testing.T) {
		t.Parallel()

		store := coderd.NewPeerNetworkTelemetryStore()
		agentID := uuid.New()
		peerID := uuid.New()
		store.Update(agentID, peerID, &tailnetproto.TelemetryEvent{
			Status:   tailnetproto.TelemetryEvent_CONNECTED,
			HomeDerp: 3,
		})
		store.Update(agentID, peerID, &tailnetproto.TelemetryEvent{
			Status:      tailnetproto.TelemetryEvent_CONNECTED,
			DerpLatency: durationpb.New(20 * time.Millisecond),
		})

		got := store.Get(agentID, peerID)
		require.NotNil(t, got)
		require.Equal(t, 3, got.HomeDERP)
		require.NotNil(t, got.P2P)
		require.False(t, *got.P2P)
		require.NotNil(t, got.DERPLatency)
		require.Equal(t, 20*time.Millisecond, *got.DERPLatency)
	})

	t.Run("ConnectedWithExplicitLatencyOverridesPreviousValues", func(t *testing.T) {
		t.Parallel()

		store := coderd.NewPeerNetworkTelemetryStore()
		agentID := uuid.New()
		peerID := uuid.New()
		store.Update(agentID, peerID, &tailnetproto.TelemetryEvent{
			Status:     tailnetproto.TelemetryEvent_CONNECTED,
			P2PLatency: durationpb.New(50 * time.Millisecond),
		})
		store.Update(agentID, peerID, &tailnetproto.TelemetryEvent{
			Status:      tailnetproto.TelemetryEvent_CONNECTED,
			DerpLatency: durationpb.New(30 * time.Millisecond),
		})

		got := store.Get(agentID, peerID)
		require.NotNil(t, got)
		require.NotNil(t, got.P2P)
		require.False(t, *got.P2P)
		require.NotNil(t, got.DERPLatency)
		require.Equal(t, 30*time.Millisecond, *got.DERPLatency)
		require.Nil(t, got.P2PLatency)
	})

	t.Run("ConnectedWithoutLatencyLeavesModeUnknown", func(t *testing.T) {
		t.Parallel()

		store := coderd.NewPeerNetworkTelemetryStore()
		agentID := uuid.New()
		peerID := uuid.New()
		store.Update(agentID, peerID, &tailnetproto.TelemetryEvent{
			Status:   tailnetproto.TelemetryEvent_CONNECTED,
			HomeDerp: 1,
		})

		got := store.Get(agentID, peerID)
		require.NotNil(t, got)
		require.Nil(t, got.P2P)
		require.Nil(t, got.DERPLatency)
		require.Nil(t, got.P2PLatency)
	})

	t.Run("TwoPeersIndependentState", func(t *testing.T) {
		t.Parallel()

		store := coderd.NewPeerNetworkTelemetryStore()
		agentID := uuid.New()
		peerA := uuid.New()
		peerB := uuid.New()

		store.Update(agentID, peerA, &tailnetproto.TelemetryEvent{
			Status:     tailnetproto.TelemetryEvent_CONNECTED,
			P2PLatency: durationpb.New(10 * time.Millisecond),
			HomeDerp:   1,
		})
		store.Update(agentID, peerB, &tailnetproto.TelemetryEvent{
			Status:      tailnetproto.TelemetryEvent_CONNECTED,
			DerpLatency: durationpb.New(80 * time.Millisecond),
			HomeDerp:    2,
		})

		gotA := store.Get(agentID, peerA)
		require.NotNil(t, gotA)
		require.NotNil(t, gotA.P2P)
		require.True(t, *gotA.P2P)
		require.NotNil(t, gotA.P2PLatency)
		require.Equal(t, 10*time.Millisecond, *gotA.P2PLatency)
		require.Nil(t, gotA.DERPLatency)
		require.Equal(t, 1, gotA.HomeDERP)

		gotB := store.Get(agentID, peerB)
		require.NotNil(t, gotB)
		require.NotNil(t, gotB.P2P)
		require.False(t, *gotB.P2P)
		require.NotNil(t, gotB.DERPLatency)
		require.Equal(t, 80*time.Millisecond, *gotB.DERPLatency)
		require.Nil(t, gotB.P2PLatency)
		require.Equal(t, 2, gotB.HomeDERP)

		all := store.GetAll(agentID)
		require.Len(t, all, 2)
		require.Same(t, gotA, all[peerA])
		require.Same(t, gotB, all[peerB])
	})

	t.Run("PeerDisconnectDoesNotWipeOther", func(t *testing.T) {
		t.Parallel()

		store := coderd.NewPeerNetworkTelemetryStore()
		agentID := uuid.New()
		peerA := uuid.New()
		peerB := uuid.New()

		store.Update(agentID, peerA, &tailnetproto.TelemetryEvent{
			Status:     tailnetproto.TelemetryEvent_CONNECTED,
			P2PLatency: durationpb.New(15 * time.Millisecond),
			HomeDerp:   5,
		})
		store.Update(agentID, peerB, &tailnetproto.TelemetryEvent{
			Status:      tailnetproto.TelemetryEvent_CONNECTED,
			DerpLatency: durationpb.New(70 * time.Millisecond),
			HomeDerp:    6,
		})

		store.Update(agentID, peerA, &tailnetproto.TelemetryEvent{Status: tailnetproto.TelemetryEvent_DISCONNECTED})

		require.Nil(t, store.Get(agentID, peerA))
		gotB := store.Get(agentID, peerB)
		require.NotNil(t, gotB)
		require.NotNil(t, gotB.P2P)
		require.False(t, *gotB.P2P)
		require.Equal(t, 6, gotB.HomeDERP)
		require.NotNil(t, gotB.DERPLatency)
		require.Equal(t, 70*time.Millisecond, *gotB.DERPLatency)

		all := store.GetAll(agentID)
		require.Len(t, all, 1)
		require.Contains(t, all, peerB)
	})

	t.Run("DisconnectedDeletes", func(t *testing.T) {
		t.Parallel()

		store := coderd.NewPeerNetworkTelemetryStore()
		agentID := uuid.New()
		peerID := uuid.New()
		store.Update(agentID, peerID, &tailnetproto.TelemetryEvent{
			Status:     tailnetproto.TelemetryEvent_CONNECTED,
			P2PLatency: durationpb.New(15 * time.Millisecond),
		})
		store.Update(agentID, peerID, &tailnetproto.TelemetryEvent{Status: tailnetproto.TelemetryEvent_DISCONNECTED})

		require.Nil(t, store.Get(agentID, peerID))
	})

	t.Run("StaleEntryEvicted", func(t *testing.T) {
		t.Parallel()

		store := coderd.NewPeerNetworkTelemetryStore()
		agentID := uuid.New()
		peerID := uuid.New()
		store.Update(agentID, peerID, &tailnetproto.TelemetryEvent{
			Status:     tailnetproto.TelemetryEvent_CONNECTED,
			P2PLatency: durationpb.New(20 * time.Millisecond),
		})

		entry := store.Get(agentID, peerID)
		require.NotNil(t, entry)
		entry.LastUpdatedAt = time.Now().Add(-3 * time.Minute)

		require.Nil(t, store.Get(agentID, peerID))
		require.Nil(t, store.Get(agentID, peerID))
	})

	t.Run("Delete", func(t *testing.T) {
		t.Parallel()

		store := coderd.NewPeerNetworkTelemetryStore()
		agentID := uuid.New()
		peerID := uuid.New()
		store.Update(agentID, peerID, &tailnetproto.TelemetryEvent{
			Status:     tailnetproto.TelemetryEvent_CONNECTED,
			P2PLatency: durationpb.New(15 * time.Millisecond),
		})

		store.Delete(agentID, peerID)
		require.Nil(t, store.Get(agentID, peerID))
	})

	t.Run("ConcurrentAccess", func(t *testing.T) {
		t.Parallel()

		store := coderd.NewPeerNetworkTelemetryStore()
		agentIDs := make([]uuid.UUID, 8)
		for i := range agentIDs {
			agentIDs[i] = uuid.New()
		}
		peerIDs := make([]uuid.UUID, 16)
		for i := range peerIDs {
			peerIDs[i] = uuid.New()
		}

		const (
			goroutines = 8
			iterations = 100
		)

		var wg sync.WaitGroup
		wg.Add(goroutines)
		for g := 0; g < goroutines; g++ {
			go func(worker int) {
				defer wg.Done()
				for i := 0; i < iterations; i++ {
					agentID := agentIDs[(worker+i)%len(agentIDs)]
					peerID := peerIDs[(worker*iterations+i)%len(peerIDs)]
					store.Update(agentID, peerID, &tailnetproto.TelemetryEvent{
						Status:     tailnetproto.TelemetryEvent_CONNECTED,
						P2PLatency: durationpb.New(time.Duration(i+1) * time.Millisecond),
						HomeDerp:   int32(worker + 1), //nolint:gosec // test data, worker is small
					})
					_ = store.Get(agentID, peerID)
					_ = store.GetAll(agentID)
					if i%10 == 0 {
						store.Delete(agentID, peerID)
					}
				}
			}(g)
		}
		wg.Wait()
	})
}

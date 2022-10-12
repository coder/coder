package replica_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbtestutil"
	"github.com/coder/coder/enterprise/replica"
	"github.com/coder/coder/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestReplica(t *testing.T) {
	t.Parallel()
	t.Run("CreateOnNew", func(t *testing.T) {
		// This ensures that a new replica is created on New.
		t.Parallel()
		db, pubsub := dbtestutil.NewDB(t)
		id := uuid.New()
		cancel, err := pubsub.Subscribe(replica.PubsubEvent, func(ctx context.Context, message []byte) {
			assert.Equal(t, []byte(id.String()), message)
		})
		require.NoError(t, err)
		defer cancel()
		server, err := replica.New(context.Background(), slogtest.Make(t, nil), db, pubsub, replica.Options{
			ID: id,
		})
		require.NoError(t, err)
		_ = server.Close()
		require.NoError(t, err)
	})
	t.Run("UpdatesOnNew", func(t *testing.T) {
		// This ensures that a replica is updated when it initially connects
		// and immediately publishes it's existence!
		t.Parallel()
		db, pubsub := dbtestutil.NewDB(t)
		id := uuid.New()
		_, err := db.InsertReplica(context.Background(), database.InsertReplicaParams{
			ID: id,
		})
		require.NoError(t, err)
		cancel, err := pubsub.Subscribe(replica.PubsubEvent, func(ctx context.Context, message []byte) {
			assert.Equal(t, []byte(id.String()), message)
		})
		require.NoError(t, err)
		defer cancel()
		server, err := replica.New(context.Background(), slogtest.Make(t, nil), db, pubsub, replica.Options{
			ID: id,
		})
		require.NoError(t, err)
		_ = server.Close()
		require.NoError(t, err)
	})
	t.Run("ConnectsToPeerReplica", func(t *testing.T) {
		// Ensures that the replica reports a successful status for
		// accessing all of its peers.
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()
		db, pubsub := dbtestutil.NewDB(t)
		peer, err := db.InsertReplica(context.Background(), database.InsertReplicaParams{
			ID:           uuid.New(),
			CreatedAt:    database.Now(),
			StartedAt:    database.Now(),
			UpdatedAt:    database.Now(),
			Hostname:     "something",
			RelayAddress: srv.URL,
		})
		require.NoError(t, err)
		server, err := replica.New(context.Background(), slogtest.Make(t, nil), db, pubsub, replica.Options{
			ID: uuid.New(),
		})
		require.NoError(t, err)
		require.Len(t, server.Regional(), 1)
		require.Equal(t, peer.ID, server.Regional()[0].ID)
		require.False(t, server.Self().Error.Valid)
		_ = server.Close()
	})
	t.Run("ConnectsToFakePeerWithError", func(t *testing.T) {
		t.Parallel()
		db, pubsub := dbtestutil.NewDB(t)
		var count atomic.Int32
		cancel, err := pubsub.Subscribe(replica.PubsubEvent, func(ctx context.Context, message []byte) {
			count.Add(1)
		})
		require.NoError(t, err)
		defer cancel()
		peer, err := db.InsertReplica(context.Background(), database.InsertReplicaParams{
			ID:        uuid.New(),
			CreatedAt: database.Now(),
			StartedAt: database.Now(),
			UpdatedAt: database.Now(),
			Hostname:  "something",
			// Fake address to hit!
			RelayAddress: "http://169.254.169.254",
		})
		require.NoError(t, err)
		server, err := replica.New(context.Background(), slogtest.Make(t, nil), db, pubsub, replica.Options{
			ID:          uuid.New(),
			PeerTimeout: 1 * time.Millisecond,
		})
		require.NoError(t, err)
		require.Len(t, server.Regional(), 1)
		require.Equal(t, peer.ID, server.Regional()[0].ID)
		require.True(t, server.Self().Error.Valid)
		require.Contains(t, server.Self().Error.String, "Failed to dial peers")
		// Once for the initial creation of a replica, and another time for the error.
		require.Equal(t, int32(2), count.Load())
		_ = server.Close()
	})
	t.Run("RefreshOnPublish", func(t *testing.T) {
		// Refresh when a new replica appears!
		t.Parallel()
		db, pubsub := dbtestutil.NewDB(t)
		id := uuid.New()
		server, err := replica.New(context.Background(), slogtest.Make(t, nil), db, pubsub, replica.Options{
			ID: id,
		})
		require.NoError(t, err)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()
		peer, err := db.InsertReplica(context.Background(), database.InsertReplicaParams{
			ID:           uuid.New(),
			RelayAddress: srv.URL,
			UpdatedAt:    database.Now(),
		})
		require.NoError(t, err)
		// Publish multiple times to ensure it can handle that case.
		err = pubsub.Publish(replica.PubsubEvent, []byte(peer.ID.String()))
		require.NoError(t, err)
		err = pubsub.Publish(replica.PubsubEvent, []byte(peer.ID.String()))
		require.NoError(t, err)
		require.Eventually(t, func() bool {
			return len(server.Regional()) == 1
		}, testutil.WaitShort, testutil.IntervalFast)
		_ = server.Close()
	})
	t.Run("TwentyConcurrent", func(t *testing.T) {
		// Ensures that twenty concurrent replicas can spawn and all
		// discover each other in parallel!
		t.Parallel()
		db, pubsub := dbtestutil.NewDB(t)
		logger := slogtest.Make(t, nil)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()
		var wg sync.WaitGroup
		count := 20
		wg.Add(count)
		for i := 0; i < count; i++ {
			server, err := replica.New(context.Background(), logger, db, pubsub, replica.Options{
				ID:           uuid.New(),
				RelayAddress: srv.URL,
			})
			require.NoError(t, err)
			t.Cleanup(func() {
				_ = server.Close()
			})
			done := false
			server.SetCallback(func() {
				if len(server.All()) != count {
					return
				}
				if done {
					return
				}
				done = true
				wg.Done()
			})
		}
		wg.Wait()
	})
}

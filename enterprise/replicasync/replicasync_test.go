package replicasync_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/enterprise/replicasync"
	"github.com/coder/coder/v2/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m, testutil.GoleakOptions...)
}

func TestReplica(t *testing.T) {
	t.Parallel()
	t.Run("CreateOnNew", func(t *testing.T) {
		// This ensures that a new replica is created on New.
		t.Parallel()
		db, pubsub := dbtestutil.NewDB(t)
		closeChan := make(chan struct{}, 1)
		cancel, err := pubsub.Subscribe(replicasync.PubsubEvent, func(ctx context.Context, message []byte) {
			select {
			case closeChan <- struct{}{}:
			default:
			}
		})
		require.NoError(t, err)
		defer cancel()
		ctx, cancelCtx := context.WithCancel(context.Background())
		defer cancelCtx()
		server, err := replicasync.New(ctx, testutil.Logger(t), db, pubsub, nil)
		require.NoError(t, err)
		<-closeChan
		_ = server.Close()
		require.NoError(t, err)
	})
	t.Run("ConnectsToPeerReplica", func(t *testing.T) {
		// Ensures that the replica reports a successful status for
		// accessing all of its peers.
		t.Parallel()
		dh := &derpyHandler{}
		defer dh.requireOnlyDERPPaths(t)
		srv := httptest.NewServer(dh)
		defer srv.Close()
		db, pubsub := dbtestutil.NewDB(t)
		peer, err := db.InsertReplica(context.Background(), database.InsertReplicaParams{
			ID:           uuid.New(),
			CreatedAt:    dbtime.Now(),
			StartedAt:    dbtime.Now(),
			UpdatedAt:    dbtime.Now(),
			Hostname:     "something",
			RelayAddress: srv.URL,
			Primary:      true,
		})
		require.NoError(t, err)
		ctx, cancelCtx := context.WithCancel(context.Background())
		defer cancelCtx()
		server, err := replicasync.New(ctx, testutil.Logger(t), db, pubsub, &replicasync.Options{
			RelayAddress: "http://169.254.169.254",
		})
		require.NoError(t, err)
		defer server.Close()

		require.Len(t, server.Regional(), 1)
		require.Equal(t, peer.ID, server.Regional()[0].ID)
		require.Empty(t, server.Self().Error)
		_ = server.Close()
	})
	t.Run("ConnectsToPeerReplicaTLS", func(t *testing.T) {
		// Ensures that the replica reports a successful status for
		// accessing all of its peers.
		t.Parallel()
		rawCert := testutil.GenerateTLSCertificate(t, "hello.org")
		certificate, err := x509.ParseCertificate(rawCert.Certificate[0])
		require.NoError(t, err)
		pool := x509.NewCertPool()
		pool.AddCert(certificate)
		// nolint:gosec
		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{rawCert},
			ServerName:   "hello.org",
			RootCAs:      pool,
		}
		dh := &derpyHandler{}
		defer dh.requireOnlyDERPPaths(t)
		srv := httptest.NewUnstartedServer(dh)
		srv.TLS = tlsConfig
		srv.StartTLS()
		defer srv.Close()
		db, pubsub := dbtestutil.NewDB(t)
		peer, err := db.InsertReplica(context.Background(), database.InsertReplicaParams{
			ID:           uuid.New(),
			CreatedAt:    dbtime.Now(),
			StartedAt:    dbtime.Now(),
			UpdatedAt:    dbtime.Now(),
			Hostname:     "something",
			RelayAddress: srv.URL,
			Primary:      true,
		})
		require.NoError(t, err)
		ctx, cancelCtx := context.WithCancel(context.Background())
		defer cancelCtx()
		server, err := replicasync.New(ctx, testutil.Logger(t), db, pubsub, &replicasync.Options{
			RelayAddress: "http://169.254.169.254",
			TLSConfig:    tlsConfig,
		})
		require.NoError(t, err)
		defer server.Close()

		require.Len(t, server.Regional(), 1)
		require.Equal(t, peer.ID, server.Regional()[0].ID)
		require.Empty(t, server.Self().Error)
		_ = server.Close()
	})
	t.Run("ConnectsToFakePeerWithError", func(t *testing.T) {
		t.Parallel()
		db, pubsub := dbtestutil.NewDB(t)
		peer, err := db.InsertReplica(context.Background(), database.InsertReplicaParams{
			ID:        uuid.New(),
			CreatedAt: dbtime.Now().Add(time.Minute),
			StartedAt: dbtime.Now().Add(time.Minute),
			UpdatedAt: dbtime.Now().Add(time.Minute),
			Hostname:  "something",
			// Fake address to dial!
			RelayAddress: "http://127.0.0.1:1",
			Primary:      true,
		})
		require.NoError(t, err)
		ctx, cancelCtx := context.WithCancel(context.Background())
		defer cancelCtx()
		server, err := replicasync.New(ctx, testutil.Logger(t), db, pubsub, &replicasync.Options{
			PeerTimeout:  1 * time.Millisecond,
			RelayAddress: "http://127.0.0.1:1",
		})
		require.NoError(t, err)
		defer server.Close()

		require.Len(t, server.Regional(), 1)
		require.Equal(t, peer.ID, server.Regional()[0].ID)
		require.NotEmpty(t, server.Self().Error)
		require.Contains(t, server.Self().Error, "Failed to dial peers")
		_ = server.Close()
	})
	t.Run("RefreshOnPublish", func(t *testing.T) {
		// Refresh when a new replica appears!
		t.Parallel()
		db, pubsub := dbtestutil.NewDB(t)
		ctx, cancelCtx := context.WithCancel(context.Background())
		defer cancelCtx()
		server, err := replicasync.New(ctx, testutil.Logger(t), db, pubsub, nil)
		require.NoError(t, err)
		defer server.Close()
		dh := &derpyHandler{}
		defer dh.requireOnlyDERPPaths(t)
		srv := httptest.NewServer(dh)
		defer srv.Close()
		peer, err := db.InsertReplica(ctx, database.InsertReplicaParams{
			ID:           uuid.New(),
			RelayAddress: srv.URL,
			UpdatedAt:    dbtime.Now(),
			Primary:      true,
		})
		require.NoError(t, err)
		// Publish multiple times to ensure it can handle that case.
		err = pubsub.Publish(replicasync.PubsubEvent, []byte(peer.ID.String()))
		require.NoError(t, err)
		err = pubsub.Publish(replicasync.PubsubEvent, []byte(peer.ID.String()))
		require.NoError(t, err)
		require.Eventually(t, func() bool {
			return len(server.Regional()) == 1
		}, testutil.WaitShort, testutil.IntervalFast)
		_ = server.Close()
	})
	t.Run("DeletesOld", func(t *testing.T) {
		t.Parallel()
		db, pubsub := dbtestutil.NewDB(t)
		_, err := db.InsertReplica(context.Background(), database.InsertReplicaParams{
			ID:        uuid.New(),
			UpdatedAt: dbtime.Now().Add(-time.Hour),
			Primary:   true,
		})
		require.NoError(t, err)
		ctx, cancelCtx := context.WithCancel(context.Background())
		defer cancelCtx()
		server, err := replicasync.New(ctx, testutil.Logger(t), db, pubsub, &replicasync.Options{
			RelayAddress:    "google.com",
			CleanupInterval: time.Millisecond,
		})
		require.NoError(t, err)
		defer server.Close()
		require.Eventually(t, func() bool {
			return len(server.Regional()) == 0
		}, testutil.WaitShort, testutil.IntervalFast)
	})
	t.Run("TwentyConcurrent", func(t *testing.T) {
		// Ensures that twenty concurrent replicas can spawn and all
		// discover each other in parallel!
		t.Parallel()
		ctx, cancelCtx := context.WithCancel(context.Background())
		defer cancelCtx()
		db, pubsub := dbtestutil.NewDB(t)
		logger := testutil.Logger(t)
		dh := &derpyHandler{}
		defer dh.requireOnlyDERPPaths(t)
		srv := httptest.NewServer(dh)
		defer srv.Close()
		var wg sync.WaitGroup
		count := 20
		wg.Add(count)
		for i := 0; i < count; i++ {
			server, err := replicasync.New(ctx, logger, db, pubsub, &replicasync.Options{
				RelayAddress: srv.URL,
			})
			require.NoError(t, err)
			t.Cleanup(func() {
				_ = server.Close()
			})
			done := false

			var m sync.Mutex
			_ = server.AddCallback(func() {
				m.Lock()
				defer m.Unlock()
				if len(server.AllPrimary()) != count {
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
	t.Run("UpsertAfterDelete", func(t *testing.T) {
		t.Parallel()
		db, pubsub := dbtestutil.NewDB(t)
		ctx, cancelCtx := context.WithCancel(context.Background())
		defer cancelCtx()
		server, err := replicasync.New(ctx, testutil.Logger(t), db, pubsub, &replicasync.Options{
			RelayAddress:    "google.com",
			CleanupInterval: time.Millisecond,
			UpdateInterval:  time.Millisecond,
		})
		require.NoError(t, err)
		defer server.Close()
		err = db.DeleteReplicasUpdatedBefore(ctx, dbtime.Now())
		require.NoError(t, err)
		deleteTime := dbtime.Now()
		require.Eventually(t, func() bool {
			return server.Self().UpdatedAt.After(deleteTime)
		}, testutil.WaitShort, testutil.IntervalFast)
	})
}

func TestAddCallback(t *testing.T) {
	t.Parallel()

	t.Run("FiresOnceOnAdd", func(t *testing.T) {
		t.Parallel()
		db, pubsub := dbtestutil.NewDB(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		// Disable the periodic sync so we only observe the on-add fire.
		server, err := replicasync.New(ctx, testutil.Logger(t), db, pubsub, &replicasync.Options{
			UpdateInterval: time.Hour,
		})
		require.NoError(t, err)
		defer server.Close()

		fired := make(chan struct{}, 8)
		remove := server.AddCallback(func() {
			fired <- struct{}{}
		})
		defer remove()

		// Should fire exactly once on registration.
		select {
		case <-fired:
		case <-time.After(testutil.WaitShort):
			t.Fatal("AddCallback did not fire on registration")
		}
		select {
		case <-fired:
			t.Fatal("AddCallback fired more than once without a sync")
		case <-time.After(testutil.IntervalFast):
		}
	})

	t.Run("MultipleCallbacksAllFireOnSync", func(t *testing.T) {
		t.Parallel()
		db, pubsub := dbtestutil.NewDB(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		server, err := replicasync.New(ctx, testutil.Logger(t), db, pubsub, &replicasync.Options{
			UpdateInterval: time.Hour,
		})
		require.NoError(t, err)
		defer server.Close()

		var aCount, bCount atomic.Uint32
		removeA := server.AddCallback(func() { aCount.Add(1) })
		defer removeA()
		removeB := server.AddCallback(func() { bCount.Add(1) })
		defer removeB()

		// Fire both via an explicit sync. With two AddCallback fires
		// plus one sync, each callback should run at least twice.
		require.NoError(t, server.UpdateNow(ctx))
		require.Eventually(t, func() bool {
			return aCount.Load() >= 2 && bCount.Load() >= 2
		}, testutil.WaitShort, testutil.IntervalFast)
	})

	t.Run("RemoveDetachesFromSync", func(t *testing.T) {
		t.Parallel()
		db, pubsub := dbtestutil.NewDB(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		server, err := replicasync.New(ctx, testutil.Logger(t), db, pubsub, &replicasync.Options{
			UpdateInterval: time.Hour,
		})
		require.NoError(t, err)
		defer server.Close()

		var count atomic.Uint32
		remove := server.AddCallback(func() { count.Add(1) })
		// Wait for the on-add fire to land.
		require.Eventually(t, func() bool {
			return count.Load() >= 1
		}, testutil.WaitShort, testutil.IntervalFast)

		remove()
		before := count.Load()
		// A subsequent sync must not invoke the removed callback.
		require.NoError(t, server.UpdateNow(ctx))
		// Give any spurious dispatch time to land.
		time.Sleep(testutil.IntervalFast)
		require.Equal(t, before, count.Load())
	})

	t.Run("RemoveIsIdempotent", func(t *testing.T) {
		t.Parallel()
		db, pubsub := dbtestutil.NewDB(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		server, err := replicasync.New(ctx, testutil.Logger(t), db, pubsub, &replicasync.Options{
			UpdateInterval: time.Hour,
		})
		require.NoError(t, err)
		defer server.Close()

		remove := server.AddCallback(func() {})
		// Calling remove repeatedly must not panic.
		require.NotPanics(t, func() {
			remove()
			remove()
			remove()
		})
	})
}

type derpyHandler struct {
	atomic.Uint32
}

func (d *derpyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/derp/latency-check" {
		w.WriteHeader(http.StatusNotFound)
		d.Add(1)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (d *derpyHandler) requireOnlyDERPPaths(t *testing.T) {
	require.Equal(t, uint32(0), d.Load())
}

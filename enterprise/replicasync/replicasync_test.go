package replicasync_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/enterprise/replicasync"
	"github.com/coder/coder/v2/testutil"
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
		server, err := replicasync.New(ctx, slogtest.Make(t, nil), db, pubsub, nil)
		require.NoError(t, err)
		<-closeChan
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
		server, err := replicasync.New(ctx, slogtest.Make(t, nil), db, pubsub, &replicasync.Options{
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
		srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
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
		server, err := replicasync.New(ctx, slogtest.Make(t, nil), db, pubsub, &replicasync.Options{
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
		server, err := replicasync.New(ctx, slogtest.Make(t, nil), db, pubsub, &replicasync.Options{
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
		server, err := replicasync.New(ctx, slogtest.Make(t, nil), db, pubsub, nil)
		require.NoError(t, err)
		defer server.Close()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
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
		server, err := replicasync.New(ctx, slogtest.Make(t, nil), db, pubsub, &replicasync.Options{
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
		// This doesn't use the database fake because creating
		// this many PostgreSQL connections takes some
		// configuration tweaking.
		db := dbfake.New()
		pubsub := pubsub.NewInMemory()
		logger := slogtest.Make(t, nil)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
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
			server.SetCallback(func() {
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
}

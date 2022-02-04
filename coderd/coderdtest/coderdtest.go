package coderdtest

import (
	"context"
	"database/sql"
	"io"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/coderd"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/cryptorand"
	"github.com/coder/coder/database"
	"github.com/coder/coder/database/databasefake"
	"github.com/coder/coder/database/postgres"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionerd"
	"github.com/coder/coder/provisionersdk"
	"github.com/coder/coder/provisionersdk/proto"
)

// Server represents a test instance of coderd.
// The database is intentionally omitted from
// this struct to promote data being exposed via
// the API.
type Server struct {
	Client *codersdk.Client
	URL    *url.URL
}

// RandomInitialUser generates a random initial user and authenticates
// it with the client on the Server struct.
func (s *Server) RandomInitialUser(t *testing.T) coderd.CreateInitialUserRequest {
	username, err := cryptorand.String(12)
	require.NoError(t, err)
	password, err := cryptorand.String(12)
	require.NoError(t, err)
	organization, err := cryptorand.String(12)
	require.NoError(t, err)

	req := coderd.CreateInitialUserRequest{
		Email:        "testuser@coder.com",
		Username:     username,
		Password:     password,
		Organization: organization,
	}
	_, err = s.Client.CreateInitialUser(context.Background(), req)
	require.NoError(t, err)

	login, err := s.Client.LoginWithPassword(context.Background(), coderd.LoginWithPasswordRequest{
		Email:    "testuser@coder.com",
		Password: password,
	})
	require.NoError(t, err)
	err = s.Client.SetSessionToken(login.SessionToken)
	require.NoError(t, err)
	return req
}

// AddProvisionerd launches a new provisionerd instance with the
// test provisioner registered.
func (s *Server) AddProvisionerd(t *testing.T) io.Closer {
	echoClient, echoServer := provisionersdk.TransportPipe()
	ctx, cancelFunc := context.WithCancel(context.Background())
	t.Cleanup(func() {
		_ = echoClient.Close()
		_ = echoServer.Close()
		cancelFunc()
	})
	go func() {
		err := echo.Serve(ctx, &provisionersdk.ServeOptions{
			Listener: echoServer,
		})
		require.NoError(t, err)
	}()

	closer := provisionerd.New(s.Client.ProvisionerDaemonClient, &provisionerd.Options{
		Logger:         slogtest.Make(t, nil).Named("provisionerd").Leveled(slog.LevelDebug),
		PollInterval:   50 * time.Millisecond,
		UpdateInterval: 50 * time.Millisecond,
		Provisioners: provisionerd.Provisioners{
			string(database.ProvisionerTypeEcho): proto.NewDRPCProvisionerClient(provisionersdk.Conn(echoClient)),
		},
		WorkDirectory: t.TempDir(),
	})
	t.Cleanup(func() {
		_ = closer.Close()
	})
	return closer
}

// New constructs a new coderd test instance. This returned Server
// should contain no side-effects.
func New(t *testing.T) Server {
	// This can be hotswapped for a live database instance.
	db := databasefake.New()
	pubsub := database.NewPubsubInMemory()
	if os.Getenv("DB") != "" {
		connectionURL, close, err := postgres.Open()
		require.NoError(t, err)
		t.Cleanup(close)
		sqlDB, err := sql.Open("postgres", connectionURL)
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = sqlDB.Close()
		})
		err = database.Migrate(sqlDB)
		require.NoError(t, err)
		db = database.New(sqlDB)

		pubsub, err = database.NewPubsub(context.Background(), sqlDB, connectionURL)
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = pubsub.Close()
		})
	}

	handler := coderd.New(&coderd.Options{
		Logger:   slogtest.Make(t, nil).Leveled(slog.LevelDebug),
		Database: db,
		Pubsub:   pubsub,
	})
	srv := httptest.NewServer(handler)
	serverURL, err := url.Parse(srv.URL)
	require.NoError(t, err)
	t.Cleanup(srv.Close)

	return Server{
		Client: codersdk.New(serverURL),
		URL:    serverURL,
	}
}

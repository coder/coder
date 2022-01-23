package coderdtest

import (
	"context"
	"database/sql"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/coderd"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/cryptorand"
	"github.com/coder/coder/database"
	"github.com/coder/coder/database/databasefake"
	"github.com/coder/coder/database/postgres"
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

// New constructs a new coderd test instance. This returned Server
// should contain no side-effects.
func New(t *testing.T) Server {
	// This can be hotswapped for a live database instance.
	db := databasefake.New()
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
	}

	handler := coderd.New(&coderd.Options{
		Logger:   slogtest.Make(t, nil),
		Database: db,
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

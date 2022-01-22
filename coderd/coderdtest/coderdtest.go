package coderdtest

import (
	"context"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/coderd"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/database/databasefake"
)

// Server represents a test instance of coderd.
// The database is intentionally omitted from
// this struct to promote data being exposed via
// the API.
type Server struct {
	Client *codersdk.Client
	URL    *url.URL
}

// New constructs a new coderd test instance.
func New(t *testing.T) Server {
	// This can be hotswapped for a live database instance.
	db := databasefake.New()
	handler := coderd.New(&coderd.Options{
		Logger:   slogtest.Make(t, nil),
		Database: db,
	})
	srv := httptest.NewServer(handler)
	serverURL, err := url.Parse(srv.URL)
	require.NoError(t, err)
	t.Cleanup(srv.Close)

	client := codersdk.New(serverURL)
	_, err = client.CreateInitialUser(context.Background(), coderd.CreateUserRequest{
		Email:    "testuser@coder.com",
		Username: "testuser",
		Password: "testpassword",
	})
	require.NoError(t, err)

	login, err := client.LoginWithPassword(context.Background(), coderd.LoginWithPasswordRequest{
		Email:    "testuser@coder.com",
		Password: "testpassword",
	})
	require.NoError(t, err)
	err = client.SetSessionToken(login.SessionToken)
	require.NoError(t, err)

	return Server{
		Client: client,
		URL:    serverURL,
	}
}

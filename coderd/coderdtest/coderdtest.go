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
	"github.com/coder/coder/database"
	"github.com/coder/coder/database/databasefake"
)

type Server struct {
	Client   *codersdk.Client
	Database database.Store
	URL      *url.URL
}

func New(t *testing.T) Server {
	// This can be hotswapped for a live database instance.
	db := databasefake.New()
	handler := coderd.New(&coderd.Options{
		Logger:   slogtest.Make(t, nil),
		Database: db,
	})
	srv := httptest.NewServer(handler)
	u, err := url.Parse(srv.URL)
	require.NoError(t, err)
	t.Cleanup(srv.Close)

	client := codersdk.New(u, &codersdk.Options{})
	_, err = client.CreateInitialUser(context.Background(), coderd.CreateUserRequest{
		Email:    "testuser@coder.com",
		Username: "testuser",
		Password: "testpassword",
	})
	require.NoError(t, err)

	return Server{
		Client:   client,
		Database: db,
		URL:      u,
	}
}

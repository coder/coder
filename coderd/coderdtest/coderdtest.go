package coderdtest

import (
	"net/http/httptest"
	"testing"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/coderd"
	"github.com/coder/coder/database"
	"github.com/coder/coder/database/databasefake"
)

type Server struct {
	Database database.Store
	URL      string
}

func New(t *testing.T) Server {
	// This can be hotswapped for a live database instance.
	db := databasefake.New()
	handler := coderd.New(&coderd.Options{
		Logger:   slogtest.Make(t, nil),
		Database: db,
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return Server{
		Database: db,
		URL:      srv.URL,
	}
}

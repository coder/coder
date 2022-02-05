package coderdtest

import (
	"context"
	"database/sql"
	"io"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/pkg/namesgenerator"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/coderd"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/database"
	"github.com/coder/coder/database/databasefake"
	"github.com/coder/coder/database/postgres"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionerd"
	"github.com/coder/coder/provisionersdk"
	"github.com/coder/coder/provisionersdk/proto"
)

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

// Server represents a test instance of coderd.
// The database is intentionally omitted from
// this struct to promote data being exposed via
// the API.
type Server struct {
	Client *codersdk.Client
	URL    *url.URL
}

// NewInitialUser creates a user with preset credentials and authenticates
// with the passed in codersdk client.
func NewInitialUser(t *testing.T, client *codersdk.Client) coderd.CreateInitialUserRequest {
	req := coderd.CreateInitialUserRequest{
		Email:        "testuser@coder.com",
		Username:     "testuser",
		Password:     "testpass",
		Organization: "testorg",
	}
	_, err := client.CreateInitialUser(context.Background(), req)
	require.NoError(t, err)

	login, err := client.LoginWithPassword(context.Background(), coderd.LoginWithPasswordRequest{
		Email:    req.Email,
		Password: req.Password,
	})
	require.NoError(t, err)
	err = client.SetSessionToken(login.SessionToken)
	require.NoError(t, err)
	return req
}

// NewProvisionerDaemon launches a provisionerd instance configured to work
// well with coderd testing. It registers the "echo" provisioner for
// quick testing.
func NewProvisionerDaemon(t *testing.T, client *codersdk.Client) io.Closer {
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

	closer := provisionerd.New(client.ProvisionerDaemonClient, &provisionerd.Options{
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

// NewProject creates a project with the "echo" provisioner for
// compatibility with testing. The name assigned is randomly generated.
func NewProject(t *testing.T, client *codersdk.Client, organization string) coderd.Project {
	project, err := client.CreateProject(context.Background(), organization, coderd.CreateProjectRequest{
		Name:        randomUsername(),
		Provisioner: database.ProvisionerTypeEcho,
	})
	require.NoError(t, err)
	return project
}

// NewProjectVersion creates a project version for the "echo" provisioner
// for compatibility with testing.
func NewProjectVersion(t *testing.T, client *codersdk.Client, organization, project string, responses *echo.Responses) coderd.ProjectVersion {
	data, err := echo.Tar(responses)
	require.NoError(t, err)
	version, err := client.CreateProjectVersion(context.Background(), organization, project, coderd.CreateProjectVersionRequest{
		StorageMethod: database.ProjectStorageMethodInlineArchive,
		StorageSource: data,
	})
	require.NoError(t, err)
	return version
}

// AwaitProjectVersionImported awaits for the project import job to reach completed status.
func AwaitProjectVersionImported(t *testing.T, client *codersdk.Client, organization, project, version string) coderd.ProjectVersion {
	var projectVersion coderd.ProjectVersion
	require.Eventually(t, func() bool {
		var err error
		projectVersion, err = client.ProjectVersion(context.Background(), organization, project, version)
		require.NoError(t, err)
		return projectVersion.Import.Status.Completed()
	}, 3*time.Second, 50*time.Millisecond)
	return projectVersion
}

func randomUsername() string {
	return strings.ReplaceAll(namesgenerator.GetRandomName(0), "_", "-")
}

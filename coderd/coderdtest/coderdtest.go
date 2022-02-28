package coderdtest

import (
	"context"
	"database/sql"
	"io"
	"net"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/moby/moby/pkg/namesgenerator"
	"github.com/stretchr/testify/require"
	"go.opencensus.io/stats/view"
	"google.golang.org/api/idtoken"
	"google.golang.org/api/option"

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

type Options struct {
	GoogleTokenValidator *idtoken.Validator
}

// New constructs an in-memory coderd instance and returns
// the connected client.
func New(t *testing.T, options *Options) *codersdk.Client {
	// Stops the opencensus.io worker from leaking a goroutine.
	// The worker isn't used anyways, and is an indirect dependency
	// of the Google Cloud SDK.
	t.Cleanup(func() {
		view.Stop()
	})

	if options == nil {
		options = &Options{}
	}
	if options.GoogleTokenValidator == nil {
		ctx, cancelFunc := context.WithCancel(context.Background())
		t.Cleanup(cancelFunc)
		var err error
		options.GoogleTokenValidator, err = idtoken.NewValidator(ctx, option.WithoutAuthentication())
		require.NoError(t, err)
	}

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

	srv := httptest.NewUnstartedServer(nil)
	srv.Config.BaseContext = func(_ net.Listener) context.Context {
		ctx, cancelFunc := context.WithCancel(context.Background())
		t.Cleanup(cancelFunc)
		return ctx
	}
	srv.Start()
	serverURL, err := url.Parse(srv.URL)
	require.NoError(t, err)
	var closeWait func()
	// We set the handler after server creation for the access URL.
	srv.Config.Handler, closeWait = coderd.New(&coderd.Options{
		AccessURL: serverURL,
		Logger:    slogtest.Make(t, nil).Leveled(slog.LevelDebug),
		Database:  db,
		Pubsub:    pubsub,

		GoogleTokenValidator: options.GoogleTokenValidator,
	})
	t.Cleanup(func() {
		srv.Close()
		closeWait()
	})

	return codersdk.New(serverURL)
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

// CreateInitialUser creates a user with preset credentials and authenticates
// with the passed in codersdk client.
func CreateInitialUser(t *testing.T, client *codersdk.Client) coderd.CreateInitialUserRequest {
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
	client.SessionToken = login.SessionToken
	return req
}

// CreateProjectImportJob creates a project import provisioner job
// with the responses provided. It uses the "echo" provisioner for compatibility
// with testing.
func CreateProjectImportJob(t *testing.T, client *codersdk.Client, organization string, res *echo.Responses) coderd.ProvisionerJob {
	data, err := echo.Tar(res)
	require.NoError(t, err)
	file, err := client.UploadFile(context.Background(), codersdk.ContentTypeTar, data)
	require.NoError(t, err)
	job, err := client.CreateProjectImportJob(context.Background(), organization, coderd.CreateProjectImportJobRequest{
		StorageSource: file.Hash,
		StorageMethod: database.ProvisionerStorageMethodFile,
		Provisioner:   database.ProvisionerTypeEcho,
	})
	require.NoError(t, err)
	return job
}

// CreateProject creates a project with the "echo" provisioner for
// compatibility with testing. The name assigned is randomly generated.
func CreateProject(t *testing.T, client *codersdk.Client, organization string, job uuid.UUID) coderd.Project {
	project, err := client.CreateProject(context.Background(), organization, coderd.CreateProjectRequest{
		Name:               randomUsername(),
		VersionImportJobID: job,
	})
	require.NoError(t, err)
	return project
}

// AwaitProjectImportJob awaits for an import job to reach completed status.
func AwaitProjectImportJob(t *testing.T, client *codersdk.Client, organization string, job uuid.UUID) coderd.ProvisionerJob {
	var provisionerJob coderd.ProvisionerJob
	require.Eventually(t, func() bool {
		var err error
		provisionerJob, err = client.ProjectImportJob(context.Background(), organization, job)
		require.NoError(t, err)
		return provisionerJob.Status.Completed()
	}, 5*time.Second, 25*time.Millisecond)
	return provisionerJob
}

// AwaitWorkspaceProvisionJob awaits for a workspace provision job to reach completed status.
func AwaitWorkspaceProvisionJob(t *testing.T, client *codersdk.Client, organization string, job uuid.UUID) coderd.ProvisionerJob {
	var provisionerJob coderd.ProvisionerJob
	require.Eventually(t, func() bool {
		var err error
		provisionerJob, err = client.WorkspaceProvisionJob(context.Background(), organization, job)
		require.NoError(t, err)
		return provisionerJob.Status.Completed()
	}, 5*time.Second, 25*time.Millisecond)
	return provisionerJob
}

// CreateWorkspace creates a workspace for the user and project provided.
// A random name is generated for it.
func CreateWorkspace(t *testing.T, client *codersdk.Client, user string, projectID uuid.UUID) coderd.Workspace {
	workspace, err := client.CreateWorkspace(context.Background(), user, coderd.CreateWorkspaceRequest{
		ProjectID: projectID,
		Name:      randomUsername(),
	})
	require.NoError(t, err)
	return workspace
}

func randomUsername() string {
	return strings.ReplaceAll(namesgenerator.GetRandomName(0), "_", "-")
}

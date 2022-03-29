package coderdtest

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/compute/metadata"
	"github.com/golang-jwt/jwt"
	"github.com/google/uuid"
	"github.com/moby/moby/pkg/namesgenerator"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/idtoken"
	"google.golang.org/api/option"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/awsidentity"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/databasefake"
	"github.com/coder/coder/coderd/database/postgres"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/cryptorand"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionerd"
	"github.com/coder/coder/provisionersdk"
	"github.com/coder/coder/provisionersdk/proto"
)

type Options struct {
	AWSInstanceIdentity    awsidentity.Certificates
	GoogleInstanceIdentity *idtoken.Validator
}

// New constructs an in-memory coderd instance and returns
// the connected client.
func New(t *testing.T, options *Options) *codersdk.Client {
	if options == nil {
		options = &Options{}
	}
	if options.GoogleInstanceIdentity == nil {
		ctx, cancelFunc := context.WithCancel(context.Background())
		t.Cleanup(cancelFunc)
		var err error
		options.GoogleInstanceIdentity, err = idtoken.NewValidator(ctx, option.WithoutAuthentication())
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
		err = database.MigrateUp(sqlDB)
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
		AgentConnectionUpdateFrequency: 25 * time.Millisecond,
		AccessURL:                      serverURL,
		Logger:                         slogtest.Make(t, nil).Leveled(slog.LevelDebug),
		Database:                       db,
		Pubsub:                         pubsub,

		AWSCertificates:      options.AWSInstanceIdentity,
		GoogleTokenValidator: options.GoogleInstanceIdentity,
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

	closer := provisionerd.New(client.ListenProvisionerDaemon, &provisionerd.Options{
		Logger:              slogtest.Make(t, nil).Named("provisionerd").Leveled(slog.LevelDebug),
		PollInterval:        50 * time.Millisecond,
		UpdateInterval:      250 * time.Millisecond,
		ForceCancelInterval: 250 * time.Millisecond,
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

// CreateFirstUser creates a user with preset credentials and authenticates
// with the passed in codersdk client.
func CreateFirstUser(t *testing.T, client *codersdk.Client) codersdk.CreateFirstUserResponse {
	req := codersdk.CreateFirstUserRequest{
		Email:        "testuser@coder.com",
		Username:     "testuser",
		Password:     "testpass",
		Organization: "testorg",
	}
	resp, err := client.CreateFirstUser(context.Background(), req)
	require.NoError(t, err)

	login, err := client.LoginWithPassword(context.Background(), codersdk.LoginWithPasswordRequest{
		Email:    req.Email,
		Password: req.Password,
	})
	require.NoError(t, err)
	client.SessionToken = login.SessionToken
	return resp
}

// CreateAnotherUser creates and authenticates a new user.
func CreateAnotherUser(t *testing.T, client *codersdk.Client, organization string) *codersdk.Client {
	req := codersdk.CreateUserRequest{
		Email:          namesgenerator.GetRandomName(1) + "@coder.com",
		Username:       randomUsername(),
		Password:       "testpass",
		OrganizationID: organization,
	}
	_, err := client.CreateUser(context.Background(), req)
	require.NoError(t, err)

	login, err := client.LoginWithPassword(context.Background(), codersdk.LoginWithPasswordRequest{
		Email:    req.Email,
		Password: req.Password,
	})
	require.NoError(t, err)

	other := codersdk.New(client.URL)
	other.SessionToken = login.SessionToken
	return other
}

// CreateProjectVersion creates a project import provisioner job
// with the responses provided. It uses the "echo" provisioner for compatibility
// with testing.
func CreateProjectVersion(t *testing.T, client *codersdk.Client, organization string, res *echo.Responses) codersdk.ProjectVersion {
	data, err := echo.Tar(res)
	require.NoError(t, err)
	file, err := client.Upload(context.Background(), codersdk.ContentTypeTar, data)
	require.NoError(t, err)
	projectVersion, err := client.CreateProjectVersion(context.Background(), organization, codersdk.CreateProjectVersionRequest{
		StorageSource: file.Hash,
		StorageMethod: database.ProvisionerStorageMethodFile,
		Provisioner:   database.ProvisionerTypeEcho,
	})
	require.NoError(t, err)
	return projectVersion
}

// CreateProject creates a project with the "echo" provisioner for
// compatibility with testing. The name assigned is randomly generated.
func CreateProject(t *testing.T, client *codersdk.Client, organization string, version uuid.UUID) codersdk.Project {
	project, err := client.CreateProject(context.Background(), organization, codersdk.CreateProjectRequest{
		Name:      randomUsername(),
		VersionID: version,
	})
	require.NoError(t, err)
	return project
}

// AwaitProjectImportJob awaits for an import job to reach completed status.
func AwaitProjectVersionJob(t *testing.T, client *codersdk.Client, version uuid.UUID) codersdk.ProjectVersion {
	var projectVersion codersdk.ProjectVersion
	require.Eventually(t, func() bool {
		var err error
		projectVersion, err = client.ProjectVersion(context.Background(), version)
		require.NoError(t, err)
		return projectVersion.Job.CompletedAt != nil
	}, 5*time.Second, 25*time.Millisecond)
	return projectVersion
}

// AwaitWorkspaceBuildJob waits for a workspace provision job to reach completed status.
func AwaitWorkspaceBuildJob(t *testing.T, client *codersdk.Client, build uuid.UUID) codersdk.WorkspaceBuild {
	var workspaceBuild codersdk.WorkspaceBuild
	require.Eventually(t, func() bool {
		var err error
		workspaceBuild, err = client.WorkspaceBuild(context.Background(), build)
		require.NoError(t, err)
		return workspaceBuild.Job.CompletedAt != nil
	}, 5*time.Second, 25*time.Millisecond)
	return workspaceBuild
}

// AwaitWorkspaceAgents waits for all resources with agents to be connected.
func AwaitWorkspaceAgents(t *testing.T, client *codersdk.Client, build uuid.UUID) []codersdk.WorkspaceResource {
	var resources []codersdk.WorkspaceResource
	require.Eventually(t, func() bool {
		var err error
		resources, err = client.WorkspaceResourcesByBuild(context.Background(), build)
		require.NoError(t, err)
		for _, resource := range resources {
			if resource.Agent == nil {
				continue
			}
			if resource.Agent.FirstConnectedAt == nil {
				return false
			}
		}
		return true
	}, 5*time.Second, 25*time.Millisecond)
	return resources
}

// CreateWorkspace creates a workspace for the user and project provided.
// A random name is generated for it.
func CreateWorkspace(t *testing.T, client *codersdk.Client, user string, projectID uuid.UUID) codersdk.Workspace {
	workspace, err := client.CreateWorkspace(context.Background(), user, codersdk.CreateWorkspaceRequest{
		ProjectID: projectID,
		Name:      randomUsername(),
	})
	require.NoError(t, err)
	return workspace
}

// NewGoogleInstanceIdentity returns a metadata client and ID token validator for faking
// instance authentication for Google Cloud.
// nolint:revive
func NewGoogleInstanceIdentity(t *testing.T, instanceID string, expired bool) (*idtoken.Validator, *metadata.Client) {
	keyID, err := cryptorand.String(12)
	require.NoError(t, err)
	claims := jwt.MapClaims{
		"google": map[string]interface{}{
			"compute_engine": map[string]string{
				"instance_id": instanceID,
			},
		},
	}
	if !expired {
		claims["exp"] = time.Now().AddDate(1, 0, 0).Unix()
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = keyID
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	signedKey, err := token.SignedString(privateKey)
	require.NoError(t, err)

	// Taken from: https://github.com/googleapis/google-api-go-client/blob/4bb729045d611fa77bdbeb971f6a1204ba23161d/idtoken/validate.go#L57-L75
	type jwk struct {
		Kid string `json:"kid"`
		N   string `json:"n"`
		E   string `json:"e"`
	}
	type certResponse struct {
		Keys []jwk `json:"keys"`
	}

	validator, err := idtoken.NewValidator(context.Background(), option.WithHTTPClient(&http.Client{
		Transport: roundTripper(func(r *http.Request) (*http.Response, error) {
			data, err := json.Marshal(certResponse{
				Keys: []jwk{{
					Kid: keyID,
					N:   base64.RawURLEncoding.EncodeToString(privateKey.N.Bytes()),
					E:   base64.RawURLEncoding.EncodeToString(new(big.Int).SetInt64(int64(privateKey.E)).Bytes()),
				}},
			})
			require.NoError(t, err)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(data)),
				Header:     make(http.Header),
			}, nil
		}),
	}))
	require.NoError(t, err)

	return validator, metadata.NewClient(&http.Client{
		Transport: roundTripper(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader([]byte(signedKey))),
				Header:     make(http.Header),
			}, nil
		}),
	})
}

// NewAWSInstanceIdentity returns a metadata client and ID token validator for faking
// instance authentication for AWS.
func NewAWSInstanceIdentity(t *testing.T, instanceID string) (awsidentity.Certificates, *http.Client) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	document := []byte(`{"instanceId":"` + instanceID + `"}`)
	hashedDocument := sha256.Sum256(document)

	signatureRaw, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, hashedDocument[:])
	require.NoError(t, err)
	signature := make([]byte, base64.StdEncoding.EncodedLen(len(signatureRaw)))
	base64.StdEncoding.Encode(signature, signatureRaw)

	certificate, err := x509.CreateCertificate(rand.Reader, &x509.Certificate{
		SerialNumber: big.NewInt(2022),
	}, &x509.Certificate{}, &privateKey.PublicKey, privateKey)
	require.NoError(t, err)

	certificatePEM := bytes.Buffer{}
	err = pem.Encode(&certificatePEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certificate,
	})
	require.NoError(t, err)

	return awsidentity.Certificates{
			awsidentity.Other: certificatePEM.String(),
		}, &http.Client{
			Transport: roundTripper(func(r *http.Request) (*http.Response, error) {
				// Only handle metadata server requests.
				if r.URL.Host != "169.254.169.254" {
					return http.DefaultTransport.RoundTrip(r)
				}
				switch r.URL.Path {
				case "/latest/api/token":
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(bytes.NewReader([]byte("faketoken"))),
						Header:     make(http.Header),
					}, nil
				case "/latest/dynamic/instance-identity/signature":
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(bytes.NewReader(signature)),
						Header:     make(http.Header),
					}, nil
				case "/latest/dynamic/instance-identity/document":
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(bytes.NewReader(document)),
						Header:     make(http.Header),
					}, nil
				default:
					panic("unhandled route: " + r.URL.Path)
				}
			}),
		}
}

func randomUsername() string {
	return strings.ReplaceAll(namesgenerator.GetRandomName(0), "_", "-")
}

// Used to easily create an HTTP transport!
type roundTripper func(req *http.Request) (*http.Response, error)

func (r roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return r(req)
}

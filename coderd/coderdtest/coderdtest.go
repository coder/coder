package coderdtest

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/compute/metadata"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/fullsailor/pkcs7"
	"github.com/golang-jwt/jwt"
	"github.com/google/uuid"
	"github.com/moby/moby/pkg/namesgenerator"
	"github.com/spf13/afero"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"golang.org/x/xerrors"
	"google.golang.org/api/idtoken"
	"google.golang.org/api/option"
	"tailscale.com/derp"
	"tailscale.com/net/stun/stuntest"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"
	"tailscale.com/types/nettype"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/cli/config"
	"github.com/coder/coder/cli/deployment"
	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/audit"
	"github.com/coder/coder/coderd/autobuild/executor"
	"github.com/coder/coder/coderd/awsidentity"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbtestutil"
	"github.com/coder/coder/coderd/gitauth"
	"github.com/coder/coder/coderd/gitsshkey"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/coderd/telemetry"
	"github.com/coder/coder/coderd/updatecheck"
	"github.com/coder/coder/coderd/util/ptr"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/codersdk/agentsdk"
	"github.com/coder/coder/cryptorand"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionerd"
	provisionerdproto "github.com/coder/coder/provisionerd/proto"
	"github.com/coder/coder/provisionersdk"
	sdkproto "github.com/coder/coder/provisionersdk/proto"
	"github.com/coder/coder/tailnet"
	"github.com/coder/coder/testutil"
)

// TestAppSigningKey is used as the signing key in tests to avoid generating a
// new keypair for each test. This is exported so `coder server` tests can also
// use it.
//
// Generated with:
//
//	openssl genrsa 4096 | openssl pkcs8 -topk8 -nocrypt
const TestAppSigningKey = `
-----BEGIN PRIVATE KEY-----
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
fU+3mLRVC00Fs6X0PiWYE5OlXJ7K
-----END PRIVATE KEY-----
`

type Options struct {
	// AccessURL denotes a custom access URL. By default we use the httptest
	// server's URL. Setting this may result in unexpected behavior (especially
	// with running agents).
	AccessURL            *url.URL
	AppHostname          string
	AWSCertificates      awsidentity.Certificates
	Authorizer           rbac.Authorizer
	AzureCertificates    x509.VerifyOptions
	GithubOAuth2Config   *coderd.GithubOAuth2Config
	RealIPConfig         *httpmw.RealIPConfig
	OIDCConfig           *coderd.OIDCConfig
	GoogleTokenValidator *idtoken.Validator
	SSHKeygenAlgorithm   gitsshkey.Algorithm
	AutobuildTicker      <-chan time.Time
	AutobuildStats       chan<- executor.Stats
	Auditor              audit.Auditor
	TLSCertificates      []tls.Certificate
	GitAuthConfigs       []*gitauth.Config
	TrialGenerator       func(context.Context, string) error
	AppSigningKey        *rsa.PrivateKey

	// All rate limits default to -1 (unlimited) in tests if not set.
	APIRateLimit   int
	LoginRateLimit int
	FilesRateLimit int

	// IncludeProvisionerDaemon when true means to start an in-memory provisionerD
	IncludeProvisionerDaemon    bool
	MetricsCacheRefreshInterval time.Duration
	AgentStatsRefreshInterval   time.Duration
	DeploymentConfig            *codersdk.DeploymentConfig

	// Set update check options to enable update check.
	UpdateCheckOptions *updatecheck.Options

	// Overriding the database is heavily discouraged.
	// It should only be used in cases where multiple Coder
	// test instances are running against the same database.
	Database database.Store
	Pubsub   database.Pubsub

	SwaggerEndpoint bool
}

// New constructs a codersdk client connected to an in-memory API instance.
func New(t *testing.T, options *Options) *codersdk.Client {
	client, _ := newWithCloser(t, options)
	return client
}

// NewWithProvisionerCloser returns a client as well as a handle to close
// the provisioner. This is a temporary function while work is done to
// standardize how provisioners are registered with coderd. The option
// to include a provisioner is set to true for convenience.
func NewWithProvisionerCloser(t *testing.T, options *Options) (*codersdk.Client, io.Closer) {
	if options == nil {
		options = &Options{}
	}
	options.IncludeProvisionerDaemon = true
	client, closer := newWithCloser(t, options)
	return client, closer
}

// newWithCloser constructs a codersdk client connected to an in-memory API instance.
// The returned closer closes a provisioner if it was provided
// The API is intentionally not returned here because coderd tests should not
// require a handle to the API. Do not expose the API or wrath shall descend
// upon thee. Even the io.Closer that is exposed here shouldn't be exposed
// and is a temporary measure while the API to register provisioners is ironed
// out.
func newWithCloser(t *testing.T, options *Options) (*codersdk.Client, io.Closer) {
	client, closer, _ := NewWithAPI(t, options)
	return client, closer
}

func NewOptions(t *testing.T, options *Options) (func(http.Handler), context.CancelFunc, *url.URL, *coderd.Options) {
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
	if options.AutobuildTicker == nil {
		ticker := make(chan time.Time)
		options.AutobuildTicker = ticker
		t.Cleanup(func() { close(ticker) })
	}
	if options.AutobuildStats != nil {
		t.Cleanup(func() {
			close(options.AutobuildStats)
		})
	}
	if options.Database == nil {
		options.Database, options.Pubsub = dbtestutil.NewDB(t)
	}
	// TODO: remove this once we're ready to enable authz querier by default.
	if strings.Contains(os.Getenv("CODER_EXPERIMENTS_TEST"), "authz_querier") {
		panic("Coming soon!")
		// if options.Authorizer != nil {
		// 	options.Authorizer = &RecordingAuthorizer{}
		// }
		// options.Database = authzquery.NewAuthzQuerier(options.Database, options.Authorizer)
	}
	if options.DeploymentConfig == nil {
		options.DeploymentConfig = DeploymentConfig(t)
	}

	// If no ratelimits are set, disable all rate limiting for tests.
	if options.APIRateLimit == 0 {
		options.APIRateLimit = -1
	}
	if options.LoginRateLimit == 0 {
		options.LoginRateLimit = -1
	}
	if options.FilesRateLimit == 0 {
		options.FilesRateLimit = -1
	}

	ctx, cancelFunc := context.WithCancel(context.Background())
	lifecycleExecutor := executor.New(
		ctx,
		options.Database,
		slogtest.Make(t, nil).Named("autobuild.executor").Leveled(slog.LevelDebug),
		options.AutobuildTicker,
	).WithStatsChannel(options.AutobuildStats)
	lifecycleExecutor.Run()

	var mutex sync.RWMutex
	var handler http.Handler
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mutex.RLock()
		defer mutex.RUnlock()
		if handler != nil {
			handler.ServeHTTP(w, r)
		}
	}))
	srv.Config.BaseContext = func(_ net.Listener) context.Context {
		return ctx
	}
	if options.TLSCertificates != nil {
		srv.TLS = &tls.Config{
			Certificates: options.TLSCertificates,
			MinVersion:   tls.VersionTLS12,
		}
		srv.StartTLS()
	} else {
		srv.Start()
	}
	t.Cleanup(srv.Close)

	tcpAddr, ok := srv.Listener.Addr().(*net.TCPAddr)
	require.True(t, ok)

	serverURL, err := url.Parse(srv.URL)
	require.NoError(t, err)
	serverURL.Host = fmt.Sprintf("localhost:%d", tcpAddr.Port)

	derpPort, err := strconv.Atoi(serverURL.Port())
	require.NoError(t, err)

	accessURL := options.AccessURL
	if accessURL == nil {
		accessURL = serverURL
	}

	stunAddr, stunCleanup := stuntest.ServeWithPacketListener(t, nettype.Std{})
	t.Cleanup(stunCleanup)

	derpServer := derp.NewServer(key.NewNode(), tailnet.Logger(slogtest.Make(t, nil).Named("derp")))
	derpServer.SetMeshKey("test-key")

	// match default with cli default
	if options.SSHKeygenAlgorithm == "" {
		options.SSHKeygenAlgorithm = gitsshkey.AlgorithmEd25519
	}

	var appHostnameRegex *regexp.Regexp
	if options.AppHostname != "" {
		var err error
		appHostnameRegex, err = httpapi.CompileHostnamePattern(options.AppHostname)
		require.NoError(t, err)
	}

	if options.AppSigningKey == nil {
		appSigningKeyBlock, _ := pem.Decode([]byte(TestAppSigningKey))
		require.NotNil(t, appSigningKeyBlock)
		appSigningKeyInterface, err := x509.ParsePKCS8PrivateKey(appSigningKeyBlock.Bytes)
		require.NoError(t, err)
		appSigningKey, ok := appSigningKeyInterface.(*rsa.PrivateKey)
		require.True(t, ok)
		options.AppSigningKey = appSigningKey
	}

	return func(h http.Handler) {
			mutex.Lock()
			defer mutex.Unlock()
			handler = h
		}, cancelFunc, serverURL, &coderd.Options{
			AgentConnectionUpdateFrequency: 150 * time.Millisecond,
			// Force a long disconnection timeout to ensure
			// agents are not marked as disconnected during slow tests.
			AgentInactiveDisconnectTimeout: testutil.WaitShort,
			AccessURL:                      accessURL,
			AppHostname:                    options.AppHostname,
			AppHostnameRegex:               appHostnameRegex,
			Logger:                         slogtest.Make(t, nil).Leveled(slog.LevelDebug),
			CacheDir:                       t.TempDir(),
			Database:                       options.Database,
			Pubsub:                         options.Pubsub,
			GitAuthConfigs:                 options.GitAuthConfigs,

			Auditor:              options.Auditor,
			AWSCertificates:      options.AWSCertificates,
			AzureCertificates:    options.AzureCertificates,
			GithubOAuth2Config:   options.GithubOAuth2Config,
			RealIPConfig:         options.RealIPConfig,
			OIDCConfig:           options.OIDCConfig,
			GoogleTokenValidator: options.GoogleTokenValidator,
			SSHKeygenAlgorithm:   options.SSHKeygenAlgorithm,
			DERPServer:           derpServer,
			APIRateLimit:         options.APIRateLimit,
			LoginRateLimit:       options.LoginRateLimit,
			FilesRateLimit:       options.FilesRateLimit,
			Authorizer:           options.Authorizer,
			Telemetry:            telemetry.NewNoop(),
			TLSCertificates:      options.TLSCertificates,
			TrialGenerator:       options.TrialGenerator,
			DERPMap: &tailcfg.DERPMap{
				Regions: map[int]*tailcfg.DERPRegion{
					1: {
						EmbeddedRelay: true,
						RegionID:      1,
						RegionCode:    "coder",
						RegionName:    "Coder",
						Nodes: []*tailcfg.DERPNode{{
							Name:             "1a",
							RegionID:         1,
							IPv4:             "127.0.0.1",
							DERPPort:         derpPort,
							STUNPort:         stunAddr.Port,
							InsecureForTests: true,
							ForceHTTP:        options.TLSCertificates == nil,
						}},
					},
				},
			},
			MetricsCacheRefreshInterval: options.MetricsCacheRefreshInterval,
			AgentStatsRefreshInterval:   options.AgentStatsRefreshInterval,
			DeploymentConfig:            options.DeploymentConfig,
			UpdateCheckOptions:          options.UpdateCheckOptions,
			SwaggerEndpoint:             options.SwaggerEndpoint,
			AppSigningKey:               options.AppSigningKey,
		}
}

// NewWithAPI constructs an in-memory API instance and returns a client to talk to it.
// Most tests never need a reference to the API, but AuthorizationTest in this module uses it.
// Do not expose the API or wrath shall descend upon thee.
func NewWithAPI(t *testing.T, options *Options) (*codersdk.Client, io.Closer, *coderd.API) {
	if options == nil {
		options = &Options{}
	}
	setHandler, cancelFunc, serverURL, newOptions := NewOptions(t, options)
	// We set the handler after server creation for the access URL.
	coderAPI := coderd.New(newOptions)
	setHandler(coderAPI.RootHandler)
	var provisionerCloser io.Closer = nopcloser{}
	if options.IncludeProvisionerDaemon {
		provisionerCloser = NewProvisionerDaemon(t, coderAPI)
	}
	client := codersdk.New(serverURL)
	t.Cleanup(func() {
		cancelFunc()
		_ = provisionerCloser.Close()
		_ = coderAPI.Close()
		client.HTTPClient.CloseIdleConnections()
	})
	return client, provisionerCloser, coderAPI
}

// NewProvisionerDaemon launches a provisionerd instance configured to work
// well with coderd testing. It registers the "echo" provisioner for
// quick testing.
func NewProvisionerDaemon(t *testing.T, coderAPI *coderd.API) io.Closer {
	echoClient, echoServer := provisionersdk.MemTransportPipe()
	ctx, cancelFunc := context.WithCancel(context.Background())
	t.Cleanup(func() {
		_ = echoClient.Close()
		_ = echoServer.Close()
		cancelFunc()
	})
	fs := afero.NewMemMapFs()
	go func() {
		err := echo.Serve(ctx, fs, &provisionersdk.ServeOptions{
			Listener: echoServer,
		})
		assert.NoError(t, err)
	}()

	closer := provisionerd.New(func(ctx context.Context) (provisionerdproto.DRPCProvisionerDaemonClient, error) {
		return coderAPI.CreateInMemoryProvisionerDaemon(ctx, 0)
	}, &provisionerd.Options{
		Filesystem:          fs,
		Logger:              slogtest.Make(t, nil).Named("provisionerd").Leveled(slog.LevelDebug),
		JobPollInterval:     50 * time.Millisecond,
		UpdateInterval:      250 * time.Millisecond,
		ForceCancelInterval: time.Second,
		Provisioners: provisionerd.Provisioners{
			string(database.ProvisionerTypeEcho): sdkproto.NewDRPCProvisionerClient(echoClient),
		},
		WorkDirectory: t.TempDir(),
	})
	t.Cleanup(func() {
		_ = closer.Close()
	})
	return closer
}

func NewExternalProvisionerDaemon(t *testing.T, client *codersdk.Client, org uuid.UUID, tags map[string]string) io.Closer {
	echoClient, echoServer := provisionersdk.MemTransportPipe()
	ctx, cancelFunc := context.WithCancel(context.Background())
	t.Cleanup(func() {
		_ = echoClient.Close()
		_ = echoServer.Close()
		cancelFunc()
	})
	fs := afero.NewMemMapFs()
	go func() {
		err := echo.Serve(ctx, fs, &provisionersdk.ServeOptions{
			Listener: echoServer,
		})
		assert.NoError(t, err)
	}()

	closer := provisionerd.New(func(ctx context.Context) (provisionerdproto.DRPCProvisionerDaemonClient, error) {
		return client.ServeProvisionerDaemon(ctx, org, []codersdk.ProvisionerType{codersdk.ProvisionerTypeEcho}, tags)
	}, &provisionerd.Options{
		Filesystem:          fs,
		Logger:              slogtest.Make(t, nil).Named("provisionerd").Leveled(slog.LevelDebug),
		JobPollInterval:     50 * time.Millisecond,
		UpdateInterval:      250 * time.Millisecond,
		ForceCancelInterval: time.Second,
		Provisioners: provisionerd.Provisioners{
			string(database.ProvisionerTypeEcho): sdkproto.NewDRPCProvisionerClient(echoClient),
		},
		WorkDirectory: t.TempDir(),
	})
	t.Cleanup(func() {
		_ = closer.Close()
	})
	return closer
}

var FirstUserParams = codersdk.CreateFirstUserRequest{
	Email:    "testuser@coder.com",
	Username: "testuser",
	Password: "SomeSecurePassword!",
}

// CreateFirstUser creates a user with preset credentials and authenticates
// with the passed in codersdk client.
func CreateFirstUser(t *testing.T, client *codersdk.Client) codersdk.CreateFirstUserResponse {
	resp, err := client.CreateFirstUser(context.Background(), FirstUserParams)
	require.NoError(t, err)

	login, err := client.LoginWithPassword(context.Background(), codersdk.LoginWithPasswordRequest{
		Email:    FirstUserParams.Email,
		Password: FirstUserParams.Password,
	})
	require.NoError(t, err)
	client.SetSessionToken(login.SessionToken)
	return resp
}

// CreateAnotherUser creates and authenticates a new user.
func CreateAnotherUser(t *testing.T, client *codersdk.Client, organizationID uuid.UUID, roles ...string) (*codersdk.Client, codersdk.User) {
	return createAnotherUserRetry(t, client, organizationID, 5, roles...)
}

func createAnotherUserRetry(t *testing.T, client *codersdk.Client, organizationID uuid.UUID, retries int, roles ...string) (*codersdk.Client, codersdk.User) {
	req := codersdk.CreateUserRequest{
		Email:          namesgenerator.GetRandomName(10) + "@coder.com",
		Username:       randomUsername(),
		Password:       "SomeSecurePassword!",
		OrganizationID: organizationID,
	}

	user, err := client.CreateUser(context.Background(), req)
	var apiError *codersdk.Error
	// If the user already exists by username or email conflict, try again up to "retries" times.
	if err != nil && retries >= 0 && xerrors.As(err, &apiError) {
		if apiError.StatusCode() == http.StatusConflict {
			retries--
			return createAnotherUserRetry(t, client, organizationID, retries, roles...)
		}
	}
	require.NoError(t, err)

	login, err := client.LoginWithPassword(context.Background(), codersdk.LoginWithPasswordRequest{
		Email:    req.Email,
		Password: req.Password,
	})
	require.NoError(t, err)

	other := codersdk.New(client.URL)
	other.SetSessionToken(login.SessionToken)

	if len(roles) > 0 {
		// Find the roles for the org vs the site wide roles
		orgRoles := make(map[string][]string)
		var siteRoles []string

		for _, roleName := range roles {
			roleName := roleName
			orgID, ok := rbac.IsOrgRole(roleName)
			if ok {
				orgRoles[orgID] = append(orgRoles[orgID], roleName)
			} else {
				siteRoles = append(siteRoles, roleName)
			}
		}
		// Update the roles
		for _, r := range user.Roles {
			siteRoles = append(siteRoles, r.Name)
		}

		_, err := client.UpdateUserRoles(context.Background(), user.ID.String(), codersdk.UpdateRoles{Roles: siteRoles})
		require.NoError(t, err, "update site roles")

		// Update org roles
		for orgID, roles := range orgRoles {
			organizationID, err := uuid.Parse(orgID)
			require.NoError(t, err, fmt.Sprintf("parse org id %q", orgID))
			_, err = client.UpdateOrganizationMemberRoles(context.Background(), organizationID, user.ID.String(),
				codersdk.UpdateRoles{Roles: roles})
			require.NoError(t, err, "update org membership roles")
		}
	}
	return other, user
}

// CreateTemplateVersion creates a template import provisioner job
// with the responses provided. It uses the "echo" provisioner for compatibility
// with testing.
func CreateTemplateVersion(t *testing.T, client *codersdk.Client, organizationID uuid.UUID, res *echo.Responses) codersdk.TemplateVersion {
	t.Helper()
	data, err := echo.Tar(res)
	require.NoError(t, err)
	file, err := client.Upload(context.Background(), codersdk.ContentTypeTar, bytes.NewReader(data))
	require.NoError(t, err)
	templateVersion, err := client.CreateTemplateVersion(context.Background(), organizationID, codersdk.CreateTemplateVersionRequest{
		FileID:        file.ID,
		StorageMethod: codersdk.ProvisionerStorageMethodFile,
		Provisioner:   codersdk.ProvisionerTypeEcho,
	})
	require.NoError(t, err)
	return templateVersion
}

// CreateWorkspaceBuild creates a workspace build for the given workspace and transition.
func CreateWorkspaceBuild(
	t *testing.T,
	client *codersdk.Client,
	workspace codersdk.Workspace,
	transition database.WorkspaceTransition,
) codersdk.WorkspaceBuild {
	req := codersdk.CreateWorkspaceBuildRequest{
		Transition: codersdk.WorkspaceTransition(transition),
	}
	build, err := client.CreateWorkspaceBuild(context.Background(), workspace.ID, req)
	require.NoError(t, err)
	return build
}

// CreateTemplate creates a template with the "echo" provisioner for
// compatibility with testing. The name assigned is randomly generated.
func CreateTemplate(t *testing.T, client *codersdk.Client, organization uuid.UUID, version uuid.UUID, mutators ...func(*codersdk.CreateTemplateRequest)) codersdk.Template {
	req := codersdk.CreateTemplateRequest{
		Name:        randomUsername(),
		Description: randomUsername(),
		VersionID:   version,
	}
	for _, mut := range mutators {
		mut(&req)
	}
	template, err := client.CreateTemplate(context.Background(), organization, req)
	require.NoError(t, err)
	return template
}

// UpdateTemplateVersion creates a new template version with the "echo" provisioner
// and associates it with the given templateID.
func UpdateTemplateVersion(t *testing.T, client *codersdk.Client, organizationID uuid.UUID, res *echo.Responses, templateID uuid.UUID) codersdk.TemplateVersion {
	data, err := echo.Tar(res)
	require.NoError(t, err)
	file, err := client.Upload(context.Background(), codersdk.ContentTypeTar, bytes.NewReader(data))
	require.NoError(t, err)
	templateVersion, err := client.CreateTemplateVersion(context.Background(), organizationID, codersdk.CreateTemplateVersionRequest{
		TemplateID:    templateID,
		FileID:        file.ID,
		StorageMethod: codersdk.ProvisionerStorageMethodFile,
		Provisioner:   codersdk.ProvisionerTypeEcho,
	})
	require.NoError(t, err)
	return templateVersion
}

// AwaitTemplateImportJob awaits for an import job to reach completed status.
func AwaitTemplateVersionJob(t *testing.T, client *codersdk.Client, version uuid.UUID) codersdk.TemplateVersion {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
	defer cancel()

	t.Logf("waiting for template version job %s", version)
	var templateVersion codersdk.TemplateVersion
	require.Eventually(t, func() bool {
		var err error
		templateVersion, err = client.TemplateVersion(ctx, version)
		return assert.NoError(t, err) && templateVersion.Job.CompletedAt != nil
	}, testutil.WaitMedium, testutil.IntervalFast)
	t.Logf("got template version job %s", version)
	return templateVersion
}

// AwaitWorkspaceBuildJob waits for a workspace provision job to reach completed status.
func AwaitWorkspaceBuildJob(t *testing.T, client *codersdk.Client, build uuid.UUID) codersdk.WorkspaceBuild {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	t.Logf("waiting for workspace build job %s", build)
	var workspaceBuild codersdk.WorkspaceBuild
	require.Eventually(t, func() bool {
		var err error
		workspaceBuild, err = client.WorkspaceBuild(ctx, build)
		return assert.NoError(t, err) && workspaceBuild.Job.CompletedAt != nil
	}, testutil.WaitShort, testutil.IntervalFast)
	t.Logf("got workspace build job %s", build)
	return workspaceBuild
}

// AwaitWorkspaceAgents waits for all resources with agents to be connected.
func AwaitWorkspaceAgents(t *testing.T, client *codersdk.Client, workspaceID uuid.UUID) []codersdk.WorkspaceResource {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	t.Logf("waiting for workspace agents (workspace %s)", workspaceID)
	var resources []codersdk.WorkspaceResource
	require.Eventually(t, func() bool {
		var err error
		workspace, err := client.Workspace(ctx, workspaceID)
		if !assert.NoError(t, err) {
			return false
		}
		if workspace.LatestBuild.Job.CompletedAt.IsZero() {
			return false
		}

		for _, resource := range workspace.LatestBuild.Resources {
			for _, agent := range resource.Agents {
				if agent.Status != codersdk.WorkspaceAgentConnected {
					t.Logf("agent %s not connected yet", agent.Name)
					return false
				}
			}
		}
		resources = workspace.LatestBuild.Resources

		return true
	}, testutil.WaitLong, testutil.IntervalFast)
	t.Logf("got workspace agents (workspace %s)", workspaceID)
	return resources
}

// CreateWorkspace creates a workspace for the user and template provided.
// A random name is generated for it.
// To customize the defaults, pass a mutator func.
func CreateWorkspace(t *testing.T, client *codersdk.Client, organization uuid.UUID, templateID uuid.UUID, mutators ...func(*codersdk.CreateWorkspaceRequest)) codersdk.Workspace {
	t.Helper()
	req := codersdk.CreateWorkspaceRequest{
		TemplateID:        templateID,
		Name:              randomUsername(),
		AutostartSchedule: ptr.Ref("CRON_TZ=US/Central 30 9 * * 1-5"),
		TTLMillis:         ptr.Ref((8 * time.Hour).Milliseconds()),
	}
	for _, mutator := range mutators {
		mutator(&req)
	}
	workspace, err := client.CreateWorkspace(context.Background(), organization, codersdk.Me, req)
	require.NoError(t, err)
	return workspace
}

// TransitionWorkspace is a convenience method for transitioning a workspace from one state to another.
func MustTransitionWorkspace(t *testing.T, client *codersdk.Client, workspaceID uuid.UUID, from, to database.WorkspaceTransition) codersdk.Workspace {
	t.Helper()
	ctx := context.Background()
	workspace, err := client.Workspace(ctx, workspaceID)
	require.NoError(t, err, "unexpected error fetching workspace")
	require.Equal(t, workspace.LatestBuild.Transition, codersdk.WorkspaceTransition(from), "expected workspace state: %s got: %s", from, workspace.LatestBuild.Transition)

	template, err := client.Template(ctx, workspace.TemplateID)
	require.NoError(t, err, "fetch workspace template")

	build, err := client.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
		TemplateVersionID: template.ActiveVersionID,
		Transition:        codersdk.WorkspaceTransition(to),
	})
	require.NoError(t, err, "unexpected error transitioning workspace to %s", to)

	_ = AwaitWorkspaceBuildJob(t, client, build.ID)

	updated := MustWorkspace(t, client, workspace.ID)
	require.Equal(t, codersdk.WorkspaceTransition(to), updated.LatestBuild.Transition, "expected workspace to be in state %s but got %s", to, updated.LatestBuild.Transition)
	return updated
}

// MustWorkspace is a convenience method for fetching a workspace that should exist.
func MustWorkspace(t *testing.T, client *codersdk.Client, workspaceID uuid.UUID) codersdk.Workspace {
	t.Helper()
	ctx := context.Background()
	ws, err := client.Workspace(ctx, workspaceID)
	if err != nil && strings.Contains(err.Error(), "status code 410") {
		ws, err = client.DeletedWorkspace(ctx, workspaceID)
	}
	require.NoError(t, err, "no workspace found with id %s", workspaceID)
	return ws
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

type OIDCConfig struct {
	key    *rsa.PrivateKey
	issuer string
}

func NewOIDCConfig(t *testing.T, issuer string) *OIDCConfig {
	t.Helper()

	block, _ := pem.Decode([]byte(testRSAPrivateKey))
	pkey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	require.NoError(t, err)

	if issuer == "" {
		issuer = "https://coder.com"
	}

	return &OIDCConfig{
		key:    pkey,
		issuer: issuer,
	}
}

func (*OIDCConfig) AuthCodeURL(state string, _ ...oauth2.AuthCodeOption) string {
	return "/?state=" + url.QueryEscape(state)
}

func (*OIDCConfig) TokenSource(context.Context, *oauth2.Token) oauth2.TokenSource {
	return nil
}

func (*OIDCConfig) Exchange(_ context.Context, code string, _ ...oauth2.AuthCodeOption) (*oauth2.Token, error) {
	token, err := base64.StdEncoding.DecodeString(code)
	if err != nil {
		return nil, xerrors.Errorf("decode code: %w", err)
	}
	return (&oauth2.Token{
		AccessToken: "token",
	}).WithExtra(map[string]interface{}{
		"id_token": string(token),
	}), nil
}

func (o *OIDCConfig) EncodeClaims(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()

	if _, ok := claims["exp"]; !ok {
		claims["exp"] = time.Now().Add(time.Hour).UnixMilli()
	}

	if _, ok := claims["iss"]; !ok {
		claims["iss"] = o.issuer
	}

	if _, ok := claims["sub"]; !ok {
		claims["sub"] = "testme"
	}

	signed, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(o.key)
	require.NoError(t, err)

	return base64.StdEncoding.EncodeToString([]byte(signed))
}

func (o *OIDCConfig) OIDCConfig(t *testing.T, userInfoClaims jwt.MapClaims) *coderd.OIDCConfig {
	// By default, the provider can be empty.
	// This means it won't support any endpoints!
	provider := &oidc.Provider{}
	if userInfoClaims != nil {
		resp, err := json.Marshal(userInfoClaims)
		require.NoError(t, err)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(resp)
		}))
		t.Cleanup(srv.Close)
		cfg := &oidc.ProviderConfig{
			UserInfoURL: srv.URL,
		}
		provider = cfg.NewProvider(context.Background())
	}
	return &coderd.OIDCConfig{
		OAuth2Config: o,
		Verifier: oidc.NewVerifier(o.issuer, &oidc.StaticKeySet{
			PublicKeys: []crypto.PublicKey{o.key.Public()},
		}, &oidc.Config{
			SkipClientIDCheck: true,
		}),
		Provider:      provider,
		UsernameField: "preferred_username",
	}
}

// NewAzureInstanceIdentity returns a metadata client and ID token validator for faking
// instance authentication for Azure.
func NewAzureInstanceIdentity(t *testing.T, instanceID string) (x509.VerifyOptions, *http.Client) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	rawCertificate, err := x509.CreateCertificate(rand.Reader, &x509.Certificate{
		SerialNumber: big.NewInt(2022),
		NotAfter:     time.Now().AddDate(1, 0, 0),
		Subject: pkix.Name{
			CommonName: "metadata.azure.com",
		},
	}, &x509.Certificate{}, &privateKey.PublicKey, privateKey)
	require.NoError(t, err)

	certificate, err := x509.ParseCertificate(rawCertificate)
	require.NoError(t, err)

	signed, err := pkcs7.NewSignedData([]byte(`{"vmId":"` + instanceID + `"}`))
	require.NoError(t, err)
	err = signed.AddSigner(certificate, privateKey, pkcs7.SignerInfoConfig{})
	require.NoError(t, err)
	signatureRaw, err := signed.Finish()
	require.NoError(t, err)
	signature := make([]byte, base64.StdEncoding.EncodedLen(len(signatureRaw)))
	base64.StdEncoding.Encode(signature, signatureRaw)

	payload, err := json.Marshal(agentsdk.AzureInstanceIdentityToken{
		Signature: string(signature),
		Encoding:  "pkcs7",
	})
	require.NoError(t, err)

	certPool := x509.NewCertPool()
	certPool.AddCert(certificate)

	return x509.VerifyOptions{
			Intermediates: certPool,
			Roots:         certPool,
		}, &http.Client{
			Transport: roundTripper(func(r *http.Request) (*http.Response, error) {
				// Only handle metadata server requests.
				if r.URL.Host != "169.254.169.254" {
					return http.DefaultTransport.RoundTrip(r)
				}
				switch r.URL.Path {
				case "/metadata/attested/document":
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(bytes.NewReader(payload)),
						Header:     make(http.Header),
					}, nil
				default:
					panic("unhandled route: " + r.URL.Path)
				}
			}),
		}
}

func randomUsername() string {
	return strings.ReplaceAll(namesgenerator.GetRandomName(10), "_", "-")
}

// Used to easily create an HTTP transport!
type roundTripper func(req *http.Request) (*http.Response, error)

func (r roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return r(req)
}

type nopcloser struct{}

func (nopcloser) Close() error { return nil }

// SDKError coerces err into an SDK error.
func SDKError(t *testing.T, err error) *codersdk.Error {
	var cerr *codersdk.Error
	require.True(t, errors.As(err, &cerr))
	return cerr
}

const testRSAPrivateKey = `-----BEGIN RSA PRIVATE KEY-----
MIICXQIBAAKBgQDLets8+7M+iAQAqN/5BVyCIjhTQ4cmXulL+gm3v0oGMWzLupUS
v8KPA+Tp7dgC/DZPfMLaNH1obBBhJ9DhS6RdS3AS3kzeFrdu8zFHLWF53DUBhS92
5dCAEuJpDnNizdEhxTfoHrhuCmz8l2nt1pe5eUK2XWgd08Uc93h5ij098wIDAQAB
AoGAHLaZeWGLSaen6O/rqxg2laZ+jEFbMO7zvOTruiIkL/uJfrY1kw+8RLIn+1q0
wLcWcuEIHgKKL9IP/aXAtAoYh1FBvRPLkovF1NZB0Je/+CSGka6wvc3TGdvppZJe
rKNcUvuOYLxkmLy4g9zuY5qrxFyhtIn2qZzXEtLaVOHzPQECQQDvN0mSajpU7dTB
w4jwx7IRXGSSx65c+AsHSc1Rj++9qtPC6WsFgAfFN2CEmqhMbEUVGPv/aPjdyWk9
pyLE9xR/AkEA2cGwyIunijE5v2rlZAD7C4vRgdcMyCf3uuPcgzFtsR6ZhyQSgLZ8
YRPuvwm4cdPJMmO3YwBfxT6XGuSc2k8MjQJBAI0+b8prvpV2+DCQa8L/pjxp+VhR
Xrq2GozrHrgR7NRokTB88hwFRJFF6U9iogy9wOx8HA7qxEbwLZuhm/4AhbECQC2a
d8h4Ht09E+f3nhTEc87mODkl7WJZpHL6V2sORfeq/eIkds+H6CJ4hy5w/bSw8tjf
sz9Di8sGIaUbLZI2rd0CQQCzlVwEtRtoNCyMJTTrkgUuNufLP19RZ5FpyXxBO5/u
QastnN77KfUwdj3SJt44U/uh1jAIv4oSLBr8HYUkbnI8
-----END RSA PRIVATE KEY-----`

func DeploymentConfig(t *testing.T) *codersdk.DeploymentConfig {
	vip := deployment.NewViper()
	fs := pflag.NewFlagSet(randomUsername(), pflag.ContinueOnError)
	fs.String(config.FlagName, randomUsername(), randomUsername())
	cfg, err := deployment.Config(fs, vip)
	require.NoError(t, err)

	return cfg
}

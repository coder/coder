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
	"database/sql"
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
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cloud.google.com/go/compute/metadata"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/fullsailor/pkcs7"
	"github.com/golang-jwt/jwt"
	"github.com/google/uuid"
	"github.com/moby/moby/pkg/namesgenerator"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/afero"
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
	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/audit"
	"github.com/coder/coder/coderd/autobuild"
	"github.com/coder/coder/coderd/awsidentity"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbauthz"
	"github.com/coder/coder/coderd/database/dbtestutil"
	"github.com/coder/coder/coderd/database/pubsub"
	"github.com/coder/coder/coderd/gitauth"
	"github.com/coder/coder/coderd/gitsshkey"
	"github.com/coder/coder/coderd/healthcheck"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/coderd/schedule"
	"github.com/coder/coder/coderd/telemetry"
	"github.com/coder/coder/coderd/updatecheck"
	"github.com/coder/coder/coderd/util/ptr"
	"github.com/coder/coder/coderd/workspaceapps"
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

// AppSecurityKey is a 96-byte key used to sign JWTs and encrypt JWEs for
// workspace app tokens in tests.
var AppSecurityKey = must(workspaceapps.KeyFromString("6465616e207761732068657265206465616e207761732068657265206465616e207761732068657265206465616e207761732068657265206465616e207761732068657265206465616e207761732068657265206465616e2077617320686572"))

type Options struct {
	// AccessURL denotes a custom access URL. By default we use the httptest
	// server's URL. Setting this may result in unexpected behavior (especially
	// with running agents).
	AccessURL             *url.URL
	AppHostname           string
	AWSCertificates       awsidentity.Certificates
	Authorizer            rbac.Authorizer
	AzureCertificates     x509.VerifyOptions
	GithubOAuth2Config    *coderd.GithubOAuth2Config
	RealIPConfig          *httpmw.RealIPConfig
	OIDCConfig            *coderd.OIDCConfig
	GoogleTokenValidator  *idtoken.Validator
	SSHKeygenAlgorithm    gitsshkey.Algorithm
	AutobuildTicker       <-chan time.Time
	AutobuildStats        chan<- autobuild.Stats
	Auditor               audit.Auditor
	TLSCertificates       []tls.Certificate
	GitAuthConfigs        []*gitauth.Config
	TrialGenerator        func(context.Context, string) error
	TemplateScheduleStore schedule.TemplateScheduleStore

	HealthcheckFunc    func(ctx context.Context, apiKey string) *healthcheck.Report
	HealthcheckTimeout time.Duration
	HealthcheckRefresh time.Duration

	// All rate limits default to -1 (unlimited) in tests if not set.
	APIRateLimit   int
	LoginRateLimit int
	FilesRateLimit int

	// IncludeProvisionerDaemon when true means to start an in-memory provisionerD
	IncludeProvisionerDaemon    bool
	MetricsCacheRefreshInterval time.Duration
	AgentStatsRefreshInterval   time.Duration
	DeploymentValues            *codersdk.DeploymentValues

	// Set update check options to enable update check.
	UpdateCheckOptions *updatecheck.Options

	// Overriding the database is heavily discouraged.
	// It should only be used in cases where multiple Coder
	// test instances are running against the same database.
	Database database.Store
	Pubsub   pubsub.Pubsub

	ConfigSSH codersdk.SSHConfigResponse

	SwaggerEndpoint bool
	// Logger should only be overridden if you expect errors
	// as part of your test.
	Logger *slog.Logger
}

// New constructs a codersdk client connected to an in-memory API instance.
func New(t testing.TB, options *Options) *codersdk.Client {
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
func newWithCloser(t testing.TB, options *Options) (*codersdk.Client, io.Closer) {
	client, closer, _ := NewWithAPI(t, options)
	return client, closer
}

func NewOptions(t testing.TB, options *Options) (func(http.Handler), context.CancelFunc, *url.URL, *coderd.Options) {
	t.Helper()

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

	if options.Authorizer == nil {
		defAuth := rbac.NewCachingAuthorizer(prometheus.NewRegistry())
		if _, ok := t.(*testing.T); ok {
			options.Authorizer = &RecordingAuthorizer{
				Wrapped: defAuth,
			}
		} else {
			// In benchmarks, the recording authorizer greatly skews results.
			options.Authorizer = defAuth
		}
	}

	if options.Database == nil {
		options.Database, options.Pubsub = dbtestutil.NewDB(t)
		options.Database = dbauthz.New(options.Database, options.Authorizer, slogtest.Make(t, nil).Leveled(slog.LevelDebug))
	}

	// Some routes expect a deployment ID, so just make sure one exists.
	// Check first incase the caller already set up this database.
	// nolint:gocritic // Setting up unit test data inside test helper
	depID, err := options.Database.GetDeploymentID(dbauthz.AsSystemRestricted(context.Background()))
	if xerrors.Is(err, sql.ErrNoRows) || depID == "" {
		// nolint:gocritic // Setting up unit test data inside test helper
		err := options.Database.InsertDeploymentID(dbauthz.AsSystemRestricted(context.Background()), uuid.NewString())
		require.NoError(t, err, "insert a deployment id")
	}

	if options.DeploymentValues == nil {
		options.DeploymentValues = DeploymentValues(t)
	}
	// This value is not safe to run in parallel. Force it to be false.
	options.DeploymentValues.DisableOwnerWorkspaceExec = false

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

	var templateScheduleStore atomic.Pointer[schedule.TemplateScheduleStore]
	if options.TemplateScheduleStore == nil {
		options.TemplateScheduleStore = schedule.NewAGPLTemplateScheduleStore()
	}
	templateScheduleStore.Store(&options.TemplateScheduleStore)

	ctx, cancelFunc := context.WithCancel(context.Background())
	lifecycleExecutor := autobuild.NewExecutor(
		ctx,
		options.Database,
		&templateScheduleStore,
		slogtest.Make(t, nil).Named("autobuild.executor").Leveled(slog.LevelDebug),
		options.AutobuildTicker,
	).WithStatsChannel(options.AutobuildStats)
	lifecycleExecutor.Run()

	var mutex sync.RWMutex
	var handler http.Handler
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mutex.RLock()
		handler := handler
		mutex.RUnlock()
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
	stunAddr.IP = net.ParseIP("127.0.0.1")
	t.Cleanup(stunCleanup)

	derpServer := derp.NewServer(key.NewNode(), tailnet.Logger(slogtest.Make(t, nil).Named("derp").Leveled(slog.LevelDebug)))
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

	if options.Logger == nil {
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		options.Logger = &logger
	}
	region := &tailcfg.DERPRegion{
		EmbeddedRelay: true,
		RegionID:      int(options.DeploymentValues.DERP.Server.RegionID.Value()),
		RegionCode:    options.DeploymentValues.DERP.Server.RegionCode.String(),
		RegionName:    options.DeploymentValues.DERP.Server.RegionName.String(),
		Nodes: []*tailcfg.DERPNode{{
			Name:     fmt.Sprintf("%db", options.DeploymentValues.DERP.Server.RegionID),
			RegionID: int(options.DeploymentValues.DERP.Server.RegionID.Value()),
			IPv4:     "127.0.0.1",
			DERPPort: derpPort,
			// STUN port is added as a separate node by tailnet.NewDERPMap() if
			// direct connections are enabled.
			STUNPort:         -1,
			InsecureForTests: true,
			ForceHTTP:        options.TLSCertificates == nil,
		}},
	}
	if !options.DeploymentValues.DERP.Server.Enable.Value() {
		region = nil
	}
	derpMap, err := tailnet.NewDERPMap(ctx, region, []string{stunAddr.String()}, "", "", options.DeploymentValues.DERP.Config.BlockDirect.Value())
	require.NoError(t, err)

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
			Logger:                         *options.Logger,
			CacheDir:                       t.TempDir(),
			Database:                       options.Database,
			Pubsub:                         options.Pubsub,
			GitAuthConfigs:                 options.GitAuthConfigs,

			Auditor:                     options.Auditor,
			AWSCertificates:             options.AWSCertificates,
			AzureCertificates:           options.AzureCertificates,
			GithubOAuth2Config:          options.GithubOAuth2Config,
			RealIPConfig:                options.RealIPConfig,
			OIDCConfig:                  options.OIDCConfig,
			GoogleTokenValidator:        options.GoogleTokenValidator,
			SSHKeygenAlgorithm:          options.SSHKeygenAlgorithm,
			DERPServer:                  derpServer,
			APIRateLimit:                options.APIRateLimit,
			LoginRateLimit:              options.LoginRateLimit,
			FilesRateLimit:              options.FilesRateLimit,
			Authorizer:                  options.Authorizer,
			Telemetry:                   telemetry.NewNoop(),
			TemplateScheduleStore:       &templateScheduleStore,
			TLSCertificates:             options.TLSCertificates,
			TrialGenerator:              options.TrialGenerator,
			DERPMap:                     derpMap,
			MetricsCacheRefreshInterval: options.MetricsCacheRefreshInterval,
			AgentStatsRefreshInterval:   options.AgentStatsRefreshInterval,
			DeploymentValues:            options.DeploymentValues,
			UpdateCheckOptions:          options.UpdateCheckOptions,
			SwaggerEndpoint:             options.SwaggerEndpoint,
			AppSecurityKey:              AppSecurityKey,
			SSHConfig:                   options.ConfigSSH,
			HealthcheckFunc:             options.HealthcheckFunc,
			HealthcheckTimeout:          options.HealthcheckTimeout,
			HealthcheckRefresh:          options.HealthcheckRefresh,
		}
}

// NewWithAPI constructs an in-memory API instance and returns a client to talk to it.
// Most tests never need a reference to the API, but AuthorizationTest in this module uses it.
// Do not expose the API or wrath shall descend upon thee.
func NewWithAPI(t testing.TB, options *Options) (*codersdk.Client, io.Closer, *coderd.API) {
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
func NewProvisionerDaemon(t testing.TB, coderAPI *coderd.API) io.Closer {
	t.Helper()

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
		Logger:              coderAPI.Logger.Named("provisionerd").Leveled(slog.LevelDebug),
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
	serveDone := make(chan struct{})
	t.Cleanup(func() {
		_ = echoClient.Close()
		_ = echoServer.Close()
		cancelFunc()
		<-serveDone
	})
	fs := afero.NewMemMapFs()
	go func() {
		defer close(serveDone)
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
func CreateFirstUser(t testing.TB, client *codersdk.Client) codersdk.CreateFirstUserResponse {
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
	return createAnotherUserRetry(t, client, organizationID, 5, roles)
}

func CreateAnotherUserMutators(t *testing.T, client *codersdk.Client, organizationID uuid.UUID, roles []string, mutators ...func(r *codersdk.CreateUserRequest)) (*codersdk.Client, codersdk.User) {
	return createAnotherUserRetry(t, client, organizationID, 5, roles, mutators...)
}

func createAnotherUserRetry(t *testing.T, client *codersdk.Client, organizationID uuid.UUID, retries int, roles []string, mutators ...func(r *codersdk.CreateUserRequest)) (*codersdk.Client, codersdk.User) {
	req := codersdk.CreateUserRequest{
		Email:          namesgenerator.GetRandomName(10) + "@coder.com",
		Username:       randomUsername(t),
		Password:       "SomeSecurePassword!",
		OrganizationID: organizationID,
	}
	for _, m := range mutators {
		m(&req)
	}

	user, err := client.CreateUser(context.Background(), req)
	var apiError *codersdk.Error
	// If the user already exists by username or email conflict, try again up to "retries" times.
	if err != nil && retries >= 0 && xerrors.As(err, &apiError) {
		if apiError.StatusCode() == http.StatusConflict {
			retries--
			return createAnotherUserRetry(t, client, organizationID, retries, roles)
		}
	}
	require.NoError(t, err)

	var sessionToken string
	if !req.DisableLogin {
		login, err := client.LoginWithPassword(context.Background(), codersdk.LoginWithPasswordRequest{
			Email:    req.Email,
			Password: req.Password,
		})
		require.NoError(t, err)
		sessionToken = login.SessionToken
	} else {
		// Cannot log in with a disabled login user. So make it an api key from
		// the client making this user.
		token, err := client.CreateToken(context.Background(), user.ID.String(), codersdk.CreateTokenRequest{
			Lifetime:  time.Hour * 24,
			Scope:     codersdk.APIKeyScopeAll,
			TokenName: "no-password-user-token",
		})
		require.NoError(t, err)
		sessionToken = token.Key
	}

	other := codersdk.New(client.URL)
	other.SetSessionToken(sessionToken)
	t.Cleanup(func() {
		other.HTTPClient.CloseIdleConnections()
	})

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
func CreateTemplateVersion(t *testing.T, client *codersdk.Client, organizationID uuid.UUID, res *echo.Responses, mutators ...func(*codersdk.CreateTemplateVersionRequest)) codersdk.TemplateVersion {
	t.Helper()
	data, err := echo.Tar(res)
	require.NoError(t, err)
	file, err := client.Upload(context.Background(), codersdk.ContentTypeTar, bytes.NewReader(data))
	require.NoError(t, err)

	req := codersdk.CreateTemplateVersionRequest{
		FileID:        file.ID,
		StorageMethod: codersdk.ProvisionerStorageMethodFile,
		Provisioner:   codersdk.ProvisionerTypeEcho,
	}
	for _, mut := range mutators {
		mut(&req)
	}

	templateVersion, err := client.CreateTemplateVersion(context.Background(), organizationID, req)
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
		Name:      randomUsername(t),
		VersionID: version,
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

// AwaitWorkspaceAgents waits for all resources with agents to be connected. If
// specific agents are provided, it will wait for those agents to be connected
// but will not fail if other agents are not connected.
func AwaitWorkspaceAgents(t *testing.T, client *codersdk.Client, workspaceID uuid.UUID, agentNames ...string) []codersdk.WorkspaceResource {
	t.Helper()

	agentNamesMap := make(map[string]struct{}, len(agentNames))
	for _, name := range agentNames {
		agentNamesMap[name] = struct{}{}
	}

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
				if len(agentNames) > 0 {
					if _, ok := agentNamesMap[agent.Name]; !ok {
						continue
					}
				}

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
		Name:              randomUsername(t),
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

// RequestGitAuthCallback makes a request with the proper OAuth2 state cookie
// to the git auth callback endpoint.
func RequestGitAuthCallback(t *testing.T, providerID string, client *codersdk.Client) *http.Response {
	client.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	state := "somestate"
	oauthURL, err := client.URL.Parse(fmt.Sprintf("/gitauth/%s/callback?code=asd&state=%s", providerID, state))
	require.NoError(t, err)
	req, err := http.NewRequestWithContext(context.Background(), "GET", oauthURL.String(), nil)
	require.NoError(t, err)
	req.AddCookie(&http.Cookie{
		Name:  codersdk.OAuth2StateCookie,
		Value: state,
	})
	req.AddCookie(&http.Cookie{
		Name:  codersdk.SessionTokenCookie,
		Value: client.SessionToken(),
	})
	res, err := client.HTTPClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = res.Body.Close()
	})
	return res
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

func (o *OIDCConfig) OIDCConfig(t *testing.T, userInfoClaims jwt.MapClaims, opts ...func(cfg *coderd.OIDCConfig)) *coderd.OIDCConfig {
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
	cfg := &coderd.OIDCConfig{
		OAuth2Config: o,
		Verifier: oidc.NewVerifier(o.issuer, &oidc.StaticKeySet{
			PublicKeys: []crypto.PublicKey{o.key.Public()},
		}, &oidc.Config{
			SkipClientIDCheck: true,
		}),
		Provider:      provider,
		UsernameField: "preferred_username",
		EmailField:    "email",
		AuthURLParams: map[string]string{"access_type": "offline"},
		GroupField:    "groups",
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
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

func randomUsername(t testing.TB) string {
	suffix, err := cryptorand.String(3)
	require.NoError(t, err)
	suffix = "-" + suffix
	n := strings.ReplaceAll(namesgenerator.GetRandomName(10), "_", "-") + suffix
	if len(n) > 32 {
		n = n[:32-len(suffix)] + suffix
	}
	return n
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

func DeploymentValues(t testing.TB) *codersdk.DeploymentValues {
	var cfg codersdk.DeploymentValues
	opts := cfg.Options()
	err := opts.SetDefaults()
	require.NoError(t, err)
	return &cfg
}

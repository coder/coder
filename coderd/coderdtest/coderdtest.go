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
	"github.com/fullsailor/pkcs7"
	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"github.com/moby/moby/pkg/namesgenerator"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
	"google.golang.org/api/idtoken"
	"google.golang.org/api/option"
	"tailscale.com/derp"
	"tailscale.com/net/stun/stuntest"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"
	"tailscale.com/types/nettype"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/autobuild"
	"github.com/coder/coder/v2/coderd/awsidentity"
	"github.com/coder/coder/v2/coderd/batchstats"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/externalauth"
	"github.com/coder/coder/v2/coderd/gitsshkey"
	"github.com/coder/coder/v2/coderd/healthcheck"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/schedule"
	"github.com/coder/coder/v2/coderd/telemetry"
	"github.com/coder/coder/v2/coderd/unhanger"
	"github.com/coder/coder/v2/coderd/updatecheck"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/workspaceapps"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/codersdk/drpc"
	"github.com/coder/coder/v2/cryptorand"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionerd"
	provisionerdproto "github.com/coder/coder/v2/provisionerd/proto"
	"github.com/coder/coder/v2/provisionersdk"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/testutil"
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
	ExternalAuthConfigs   []*externalauth.Config
	TrialGenerator        func(context.Context, string) error
	TemplateScheduleStore schedule.TemplateScheduleStore
	Coordinator           tailnet.Coordinator

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
	Logger       *slog.Logger
	StatsBatcher *batchstats.Batcher

	WorkspaceAppsStatsCollectorOptions workspaceapps.StatsCollectorOptions
}

// New constructs a codersdk client connected to an in-memory API instance.
func New(t testing.TB, options *Options) *codersdk.Client {
	client, _ := newWithCloser(t, options)
	return client
}

// NewWithDatabase constructs a codersdk client connected to an in-memory API instance.
// The database is returned to provide direct data manipulation for tests.
func NewWithDatabase(t testing.TB, options *Options) (*codersdk.Client, database.Store) {
	client, _, api := NewWithAPI(t, options)
	return client, api.Database
}

// NewWithProvisionerCloser returns a client as well as a handle to close
// the provisioner. This is a temporary function while work is done to
// standardize how provisioners are registered with coderd. The option
// to include a provisioner is set to true for convenience.
func NewWithProvisionerCloser(t testing.TB, options *Options) (*codersdk.Client, io.Closer) {
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
	if options.Logger == nil {
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		options.Logger = &logger
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
	}

	accessControlStore := &atomic.Pointer[dbauthz.AccessControlStore]{}
	var acs dbauthz.AccessControlStore = dbauthz.AGPLTemplateAccessControlStore{}
	accessControlStore.Store(&acs)

	options.Database = dbauthz.New(options.Database, options.Authorizer, *options.Logger, accessControlStore)

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
	if options.StatsBatcher == nil {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		batcher, closeBatcher, err := batchstats.New(ctx,
			batchstats.WithStore(options.Database),
			// Avoid cluttering up test output.
			batchstats.WithLogger(slog.Make(sloghuman.Sink(io.Discard))),
		)
		require.NoError(t, err, "create stats batcher")
		options.StatsBatcher = batcher
		t.Cleanup(closeBatcher)
	}

	var templateScheduleStore atomic.Pointer[schedule.TemplateScheduleStore]
	if options.TemplateScheduleStore == nil {
		options.TemplateScheduleStore = schedule.NewAGPLTemplateScheduleStore()
	}
	templateScheduleStore.Store(&options.TemplateScheduleStore)

	var auditor atomic.Pointer[audit.Auditor]
	if options.Auditor == nil {
		options.Auditor = audit.NewNop()
	}
	auditor.Store(&options.Auditor)

	ctx, cancelFunc := context.WithCancel(context.Background())
	lifecycleExecutor := autobuild.NewExecutor(
		ctx,
		options.Database,
		options.Pubsub,
		&templateScheduleStore,
		&auditor,
		accessControlStore,
		*options.Logger,
		options.AutobuildTicker,
	).WithStatsChannel(options.AutobuildStats)
	lifecycleExecutor.Run()

	hangDetectorTicker := time.NewTicker(options.DeploymentValues.JobHangDetectorInterval.Value())
	defer hangDetectorTicker.Stop()
	hangDetector := unhanger.New(ctx, options.Database, options.Pubsub, options.Logger.Named("unhanger.detector"), hangDetectorTicker.C)
	hangDetector.Start()
	t.Cleanup(hangDetector.Close)

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

	// If the STUNAddresses setting is empty or the default, start a STUN
	// server. Otherwise, use the value as is.
	var (
		stunAddresses   []string
		dvStunAddresses = options.DeploymentValues.DERP.Server.STUNAddresses.Value()
	)
	if len(dvStunAddresses) == 0 || dvStunAddresses[0] == "stun.l.google.com:19302" {
		stunAddr, stunCleanup := stuntest.ServeWithPacketListener(t, nettype.Std{})
		stunAddr.IP = net.ParseIP("127.0.0.1")
		t.Cleanup(stunCleanup)
		stunAddresses = []string{stunAddr.String()}
		options.DeploymentValues.DERP.Server.STUNAddresses = stunAddresses
	} else if dvStunAddresses[0] != tailnet.DisableSTUN {
		stunAddresses = options.DeploymentValues.DERP.Server.STUNAddresses.Value()
	}

	derpServer := derp.NewServer(key.NewNode(), tailnet.Logger(options.Logger.Named("derp").Leveled(slog.LevelDebug)))
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
	derpMap, err := tailnet.NewDERPMap(ctx, region, stunAddresses, "", "", options.DeploymentValues.DERP.Config.BlockDirect.Value())
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
			ExternalAuthConfigs:            options.ExternalAuthConfigs,

			Auditor:                            options.Auditor,
			AWSCertificates:                    options.AWSCertificates,
			AzureCertificates:                  options.AzureCertificates,
			GithubOAuth2Config:                 options.GithubOAuth2Config,
			RealIPConfig:                       options.RealIPConfig,
			OIDCConfig:                         options.OIDCConfig,
			GoogleTokenValidator:               options.GoogleTokenValidator,
			SSHKeygenAlgorithm:                 options.SSHKeygenAlgorithm,
			DERPServer:                         derpServer,
			APIRateLimit:                       options.APIRateLimit,
			LoginRateLimit:                     options.LoginRateLimit,
			FilesRateLimit:                     options.FilesRateLimit,
			Authorizer:                         options.Authorizer,
			Telemetry:                          telemetry.NewNoop(),
			TemplateScheduleStore:              &templateScheduleStore,
			AccessControlStore:                 accessControlStore,
			TLSCertificates:                    options.TLSCertificates,
			TrialGenerator:                     options.TrialGenerator,
			TailnetCoordinator:                 options.Coordinator,
			BaseDERPMap:                        derpMap,
			DERPMapUpdateFrequency:             150 * time.Millisecond,
			MetricsCacheRefreshInterval:        options.MetricsCacheRefreshInterval,
			AgentStatsRefreshInterval:          options.AgentStatsRefreshInterval,
			DeploymentValues:                   options.DeploymentValues,
			DeploymentOptions:                  codersdk.DeploymentOptionsWithoutSecrets(options.DeploymentValues.Options()),
			UpdateCheckOptions:                 options.UpdateCheckOptions,
			SwaggerEndpoint:                    options.SwaggerEndpoint,
			AppSecurityKey:                     AppSecurityKey,
			SSHConfig:                          options.ConfigSSH,
			HealthcheckFunc:                    options.HealthcheckFunc,
			HealthcheckTimeout:                 options.HealthcheckTimeout,
			HealthcheckRefresh:                 options.HealthcheckRefresh,
			StatsBatcher:                       options.StatsBatcher,
			WorkspaceAppsStatsCollectorOptions: options.WorkspaceAppsStatsCollectorOptions,
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

// provisionerdCloser wraps a provisioner daemon as an io.Closer that can be called multiple times
type provisionerdCloser struct {
	mu     sync.Mutex
	closed bool
	d      *provisionerd.Server
}

func (c *provisionerdCloser) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	shutdownErr := c.d.Shutdown(ctx)
	closeErr := c.d.Close()
	if shutdownErr != nil {
		return shutdownErr
	}
	return closeErr
}

// NewProvisionerDaemon launches a provisionerd instance configured to work
// well with coderd testing. It registers the "echo" provisioner for
// quick testing.
func NewProvisionerDaemon(t testing.TB, coderAPI *coderd.API) io.Closer {
	t.Helper()

	// t.Cleanup runs in last added, first called order. t.TempDir() will delete
	// the directory on cleanup, so we want to make sure the echoServer is closed
	// before we go ahead an attempt to delete it's work directory.
	// seems t.TempDir() is not safe to call from a different goroutine
	workDir := t.TempDir()

	echoClient, echoServer := drpc.MemTransportPipe()
	ctx, cancelFunc := context.WithCancel(context.Background())
	t.Cleanup(func() {
		_ = echoClient.Close()
		_ = echoServer.Close()
		cancelFunc()
	})

	go func() {
		err := echo.Serve(ctx, &provisionersdk.ServeOptions{
			Listener:      echoServer,
			WorkDirectory: workDir,
			Logger:        coderAPI.Logger.Named("echo").Leveled(slog.LevelDebug),
		})
		assert.NoError(t, err)
	}()

	daemon := provisionerd.New(func(ctx context.Context) (provisionerdproto.DRPCProvisionerDaemonClient, error) {
		return coderAPI.CreateInMemoryProvisionerDaemon(ctx, t.Name())
	}, &provisionerd.Options{
		Logger:              coderAPI.Logger.Named("provisionerd").Leveled(slog.LevelDebug),
		UpdateInterval:      250 * time.Millisecond,
		ForceCancelInterval: 5 * time.Second,
		Connector: provisionerd.LocalProvisioners{
			string(database.ProvisionerTypeEcho): sdkproto.NewDRPCProvisionerClient(echoClient),
		},
	})
	closer := &provisionerdCloser{d: daemon}
	t.Cleanup(func() {
		_ = closer.Close()
	})
	return closer
}

func NewExternalProvisionerDaemon(t testing.TB, client *codersdk.Client, org uuid.UUID, tags map[string]string) io.Closer {
	echoClient, echoServer := drpc.MemTransportPipe()
	ctx, cancelFunc := context.WithCancel(context.Background())
	serveDone := make(chan struct{})
	t.Cleanup(func() {
		_ = echoClient.Close()
		_ = echoServer.Close()
		cancelFunc()
		<-serveDone
	})
	go func() {
		defer close(serveDone)
		err := echo.Serve(ctx, &provisionersdk.ServeOptions{
			Listener:      echoServer,
			WorkDirectory: t.TempDir(),
		})
		assert.NoError(t, err)
	}()

	daemon := provisionerd.New(func(ctx context.Context) (provisionerdproto.DRPCProvisionerDaemonClient, error) {
		return client.ServeProvisionerDaemon(ctx, codersdk.ServeProvisionerDaemonRequest{
			ID:           uuid.New(),
			Name:         t.Name(),
			Organization: org,
			Provisioners: []codersdk.ProvisionerType{codersdk.ProvisionerTypeEcho},
			Tags:         tags,
		})
	}, &provisionerd.Options{
		Logger:              slogtest.Make(t, nil).Named("provisionerd").Leveled(slog.LevelDebug),
		UpdateInterval:      250 * time.Millisecond,
		ForceCancelInterval: 5 * time.Second,
		Connector: provisionerd.LocalProvisioners{
			string(database.ProvisionerTypeEcho): sdkproto.NewDRPCProvisionerClient(echoClient),
		},
	})
	closer := &provisionerdCloser{d: daemon}
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
func CreateAnotherUser(t testing.TB, client *codersdk.Client, organizationID uuid.UUID, roles ...string) (*codersdk.Client, codersdk.User) {
	return createAnotherUserRetry(t, client, organizationID, 5, roles)
}

func CreateAnotherUserMutators(t testing.TB, client *codersdk.Client, organizationID uuid.UUID, roles []string, mutators ...func(r *codersdk.CreateUserRequest)) (*codersdk.Client, codersdk.User) {
	return createAnotherUserRetry(t, client, organizationID, 5, roles, mutators...)
}

// AuthzUserSubject does not include the user's groups.
func AuthzUserSubject(user codersdk.User, orgID uuid.UUID) rbac.Subject {
	roles := make(rbac.RoleNames, 0, len(user.Roles))
	// Member role is always implied
	roles = append(roles, rbac.RoleMember())
	for _, r := range user.Roles {
		roles = append(roles, r.Name)
	}
	// We assume only 1 org exists
	roles = append(roles, rbac.RoleOrgMember(orgID))

	return rbac.Subject{
		ID:     user.ID.String(),
		Roles:  roles,
		Groups: []string{},
		Scope:  rbac.ScopeAll,
	}
}

func createAnotherUserRetry(t testing.TB, client *codersdk.Client, organizationID uuid.UUID, retries int, roles []string, mutators ...func(r *codersdk.CreateUserRequest)) (*codersdk.Client, codersdk.User) {
	req := codersdk.CreateUserRequest{
		Email:          namesgenerator.GetRandomName(10) + "@coder.com",
		Username:       RandomUsername(t),
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
	if req.DisableLogin || req.UserLoginType == codersdk.LoginTypeNone {
		// Cannot log in with a disabled login user. So make it an api key from
		// the client making this user.
		token, err := client.CreateToken(context.Background(), user.ID.String(), codersdk.CreateTokenRequest{
			Lifetime:  time.Hour * 24,
			Scope:     codersdk.APIKeyScopeAll,
			TokenName: "no-password-user-token",
		})
		require.NoError(t, err)
		sessionToken = token.Key
	} else {
		login, err := client.LoginWithPassword(context.Background(), codersdk.LoginWithPasswordRequest{
			Email:    req.Email,
			Password: req.Password,
		})
		require.NoError(t, err)
		sessionToken = login.SessionToken
	}

	if user.Status == codersdk.UserStatusDormant {
		// Use admin client so that user's LastSeenAt is not updated.
		// In general we need to refresh the user status, which should
		// transition from "dormant" to "active".
		user, err = client.User(context.Background(), user.Username)
		require.NoError(t, err)
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

		user, err = client.UpdateUserRoles(context.Background(), user.ID.String(), codersdk.UpdateRoles{Roles: siteRoles})
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
func CreateTemplateVersion(t testing.TB, client *codersdk.Client, organizationID uuid.UUID, res *echo.Responses, mutators ...func(*codersdk.CreateTemplateVersionRequest)) codersdk.TemplateVersion {
	t.Helper()
	data, err := echo.TarWithOptions(context.Background(), client.Logger(), res)
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
	mutators ...func(*codersdk.CreateWorkspaceBuildRequest),
) codersdk.WorkspaceBuild {
	t.Helper()

	req := codersdk.CreateWorkspaceBuildRequest{
		Transition: codersdk.WorkspaceTransition(transition),
	}
	for _, mut := range mutators {
		mut(&req)
	}
	build, err := client.CreateWorkspaceBuild(context.Background(), workspace.ID, req)
	require.NoError(t, err)
	return build
}

// CreateTemplate creates a template with the "echo" provisioner for
// compatibility with testing. The name assigned is randomly generated.
func CreateTemplate(t testing.TB, client *codersdk.Client, organization uuid.UUID, version uuid.UUID, mutators ...func(*codersdk.CreateTemplateRequest)) codersdk.Template {
	req := codersdk.CreateTemplateRequest{
		Name:      RandomUsername(t),
		VersionID: version,
	}
	for _, mut := range mutators {
		mut(&req)
	}
	template, err := client.CreateTemplate(context.Background(), organization, req)
	require.NoError(t, err)
	return template
}

// CreateGroup creates a group with the given name and members.
func CreateGroup(t testing.TB, client *codersdk.Client, organizationID uuid.UUID, name string, members ...codersdk.User) codersdk.Group {
	t.Helper()
	group, err := client.CreateGroup(context.Background(), organizationID, codersdk.CreateGroupRequest{
		Name: name,
	})
	require.NoError(t, err, "failed to create group")
	memberIDs := make([]string, 0)
	for _, member := range members {
		memberIDs = append(memberIDs, member.ID.String())
	}
	group, err = client.PatchGroup(context.Background(), group.ID, codersdk.PatchGroupRequest{
		AddUsers: memberIDs,
	})

	require.NoError(t, err, "failed to add members to group")
	return group
}

// UpdateTemplateVersion creates a new template version with the "echo" provisioner
// and associates it with the given templateID.
func UpdateTemplateVersion(t testing.TB, client *codersdk.Client, organizationID uuid.UUID, res *echo.Responses, templateID uuid.UUID) codersdk.TemplateVersion {
	ctx := context.Background()
	data, err := echo.Tar(res)
	require.NoError(t, err)
	file, err := client.Upload(ctx, codersdk.ContentTypeTar, bytes.NewReader(data))
	require.NoError(t, err)
	templateVersion, err := client.CreateTemplateVersion(ctx, organizationID, codersdk.CreateTemplateVersionRequest{
		TemplateID:    templateID,
		FileID:        file.ID,
		StorageMethod: codersdk.ProvisionerStorageMethodFile,
		Provisioner:   codersdk.ProvisionerTypeEcho,
	})
	require.NoError(t, err)
	return templateVersion
}

func UpdateActiveTemplateVersion(t testing.TB, client *codersdk.Client, templateID, versionID uuid.UUID) {
	err := client.UpdateActiveTemplateVersion(context.Background(), templateID, codersdk.UpdateActiveTemplateVersion{
		ID: versionID,
	})
	require.NoError(t, err)
}

// UpdateTemplateMeta updates the template meta for the given template.
func UpdateTemplateMeta(t testing.TB, client *codersdk.Client, templateID uuid.UUID, meta codersdk.UpdateTemplateMeta) codersdk.Template {
	t.Helper()
	updated, err := client.UpdateTemplateMeta(context.Background(), templateID, meta)
	require.NoError(t, err)
	return updated
}

// AwaitTemplateVersionJobRunning waits for the build to be picked up by a provisioner.
func AwaitTemplateVersionJobRunning(t testing.TB, client *codersdk.Client, version uuid.UUID) codersdk.TemplateVersion {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	t.Logf("waiting for template version %s build job to start", version)
	var templateVersion codersdk.TemplateVersion
	require.Eventually(t, func() bool {
		var err error
		templateVersion, err = client.TemplateVersion(ctx, version)
		if err != nil {
			return false
		}
		t.Logf("template version job status: %s", templateVersion.Job.Status)
		switch templateVersion.Job.Status {
		case codersdk.ProvisionerJobPending:
			return false
		case codersdk.ProvisionerJobRunning:
			return true
		default:
			t.FailNow()
			return false
		}
	}, testutil.WaitShort, testutil.IntervalFast, "make sure you set `IncludeProvisionerDaemon`!")
	t.Logf("template version %s job has started", version)
	return templateVersion
}

// AwaitTemplateVersionJobCompleted waits for the build to be completed. This may result
// from cancelation, an error, or from completing successfully.
func AwaitTemplateVersionJobCompleted(t testing.TB, client *codersdk.Client, version uuid.UUID) codersdk.TemplateVersion {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	t.Logf("waiting for template version %s build job to complete", version)
	var templateVersion codersdk.TemplateVersion
	require.Eventually(t, func() bool {
		var err error
		templateVersion, err = client.TemplateVersion(ctx, version)
		t.Logf("template version job status: %s", templateVersion.Job.Status)
		return assert.NoError(t, err) && templateVersion.Job.CompletedAt != nil
	}, testutil.WaitLong, testutil.IntervalMedium, "make sure you set `IncludeProvisionerDaemon`!")
	t.Logf("template version %s job has completed", version)
	return templateVersion
}

// AwaitWorkspaceBuildJobCompleted waits for a workspace provision job to reach completed status.
func AwaitWorkspaceBuildJobCompleted(t testing.TB, client *codersdk.Client, build uuid.UUID) codersdk.WorkspaceBuild {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	t.Logf("waiting for workspace build job %s", build)
	var workspaceBuild codersdk.WorkspaceBuild
	require.Eventually(t, func() bool {
		var err error
		workspaceBuild, err = client.WorkspaceBuild(ctx, build)
		return assert.NoError(t, err) && workspaceBuild.Job.CompletedAt != nil
	}, testutil.WaitMedium, testutil.IntervalMedium)
	t.Logf("got workspace build job %s", build)
	return workspaceBuild
}

// AwaitWorkspaceAgents waits for all resources with agents to be connected. If
// specific agents are provided, it will wait for those agents to be connected
// but will not fail if other agents are not connected.
func AwaitWorkspaceAgents(t testing.TB, client *codersdk.Client, workspaceID uuid.UUID, agentNames ...string) []codersdk.WorkspaceResource {
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
		if workspace.LatestBuild.Job.CompletedAt == nil {
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
	}, testutil.WaitLong, testutil.IntervalMedium)
	t.Logf("got workspace agents (workspace %s)", workspaceID)
	return resources
}

// CreateWorkspace creates a workspace for the user and template provided.
// A random name is generated for it.
// To customize the defaults, pass a mutator func.
func CreateWorkspace(t testing.TB, client *codersdk.Client, organization uuid.UUID, templateID uuid.UUID, mutators ...func(*codersdk.CreateWorkspaceRequest)) codersdk.Workspace {
	t.Helper()
	req := codersdk.CreateWorkspaceRequest{
		TemplateID:        templateID,
		Name:              RandomUsername(t),
		AutostartSchedule: ptr.Ref("CRON_TZ=US/Central 30 9 * * 1-5"),
		TTLMillis:         ptr.Ref((8 * time.Hour).Milliseconds()),
		AutomaticUpdates:  codersdk.AutomaticUpdatesNever,
	}
	for _, mutator := range mutators {
		mutator(&req)
	}
	workspace, err := client.CreateWorkspace(context.Background(), organization, codersdk.Me, req)
	require.NoError(t, err)
	return workspace
}

// TransitionWorkspace is a convenience method for transitioning a workspace from one state to another.
func MustTransitionWorkspace(t testing.TB, client *codersdk.Client, workspaceID uuid.UUID, from, to database.WorkspaceTransition, muts ...func(req *codersdk.CreateWorkspaceBuildRequest)) codersdk.Workspace {
	t.Helper()
	ctx := context.Background()
	workspace, err := client.Workspace(ctx, workspaceID)
	require.NoError(t, err, "unexpected error fetching workspace")
	require.Equal(t, workspace.LatestBuild.Transition, codersdk.WorkspaceTransition(from), "expected workspace state: %s got: %s", from, workspace.LatestBuild.Transition)

	req := codersdk.CreateWorkspaceBuildRequest{
		TemplateVersionID: workspace.LatestBuild.TemplateVersionID,
		Transition:        codersdk.WorkspaceTransition(to),
	}

	for _, mut := range muts {
		mut(&req)
	}

	build, err := client.CreateWorkspaceBuild(ctx, workspace.ID, req)
	require.NoError(t, err, "unexpected error transitioning workspace to %s", to)

	_ = AwaitWorkspaceBuildJobCompleted(t, client, build.ID)

	updated := MustWorkspace(t, client, workspace.ID)
	require.Equal(t, codersdk.WorkspaceTransition(to), updated.LatestBuild.Transition, "expected workspace to be in state %s but got %s", to, updated.LatestBuild.Transition)
	return updated
}

// MustWorkspace is a convenience method for fetching a workspace that should exist.
func MustWorkspace(t testing.TB, client *codersdk.Client, workspaceID uuid.UUID) codersdk.Workspace {
	t.Helper()
	ctx := context.Background()
	ws, err := client.Workspace(ctx, workspaceID)
	if err != nil && strings.Contains(err.Error(), "status code 410") {
		ws, err = client.DeletedWorkspace(ctx, workspaceID)
	}
	require.NoError(t, err, "no workspace found with id %s", workspaceID)
	return ws
}

// RequestExternalAuthCallback makes a request with the proper OAuth2 state cookie
// to the external auth callback endpoint.
func RequestExternalAuthCallback(t testing.TB, providerID string, client *codersdk.Client) *http.Response {
	client.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	state := "somestate"
	oauthURL, err := client.URL.Parse(fmt.Sprintf("/external-auth/%s/callback?code=asd&state=%s", providerID, state))
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
func NewGoogleInstanceIdentity(t testing.TB, instanceID string, expired bool) (*idtoken.Validator, *metadata.Client) {
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
func NewAWSInstanceIdentity(t testing.TB, instanceID string) (awsidentity.Certificates, *http.Client) {
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

// NewAzureInstanceIdentity returns a metadata client and ID token validator for faking
// instance authentication for Azure.
func NewAzureInstanceIdentity(t testing.TB, instanceID string) (x509.VerifyOptions, *http.Client) {
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

func RandomUsername(t testing.TB) string {
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
func SDKError(t testing.TB, err error) *codersdk.Error {
	var cerr *codersdk.Error
	require.True(t, errors.As(err, &cerr))
	return cerr
}

func DeploymentValues(t testing.TB) *codersdk.DeploymentValues {
	var cfg codersdk.DeploymentValues
	opts := cfg.Options()
	err := opts.SetDefaults()
	require.NoError(t, err)
	return &cfg
}

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
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/compute/metadata"
	"github.com/fullsailor/pkcs7"
	"github.com/golang-jwt/jwt"
	"github.com/google/uuid"
	"github.com/moby/moby/pkg/namesgenerator"
	"github.com/spf13/afero"
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
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/audit"
	"github.com/coder/coder/coderd/autobuild/executor"
	"github.com/coder/coder/coderd/awsidentity"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbtestutil"
	"github.com/coder/coder/coderd/gitsshkey"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/coderd/telemetry"
	"github.com/coder/coder/coderd/util/ptr"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/cryptorand"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionerd"
	"github.com/coder/coder/provisionersdk"
	"github.com/coder/coder/provisionersdk/proto"
	"github.com/coder/coder/tailnet"
	"github.com/coder/coder/testutil"
)

type Options struct {
	AppHostname          string
	AWSCertificates      awsidentity.Certificates
	Authorizer           rbac.Authorizer
	Experimental         bool
	AzureCertificates    x509.VerifyOptions
	GithubOAuth2Config   *coderd.GithubOAuth2Config
	OIDCConfig           *coderd.OIDCConfig
	GoogleTokenValidator *idtoken.Validator
	SSHKeygenAlgorithm   gitsshkey.Algorithm
	APIRateLimit         int
	AutoImportTemplates  []coderd.AutoImportTemplate
	AutobuildTicker      <-chan time.Time
	AutobuildStats       chan<- executor.Stats
	Auditor              audit.Auditor
	TLSCertificates      []tls.Certificate

	// IncludeProvisionerDaemon when true means to start an in-memory provisionerD
	IncludeProvisionerDaemon    bool
	MetricsCacheRefreshInterval time.Duration
	AgentStatsRefreshInterval   time.Duration
	DeploymentFlags             *codersdk.DeploymentFlags

	// Overriding the database is heavily discouraged.
	// It should only be used in cases where multiple Coder
	// test instances are running against the same database.
	Database database.Store
	Pubsub   database.Pubsub
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

func NewOptions(t *testing.T, options *Options) (func(http.Handler), context.CancelFunc, *coderd.Options) {
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

	return func(h http.Handler) {
			mutex.Lock()
			defer mutex.Unlock()
			handler = h
		}, cancelFunc, &coderd.Options{
			AgentConnectionUpdateFrequency: 150 * time.Millisecond,
			// Force a long disconnection timeout to ensure
			// agents are not marked as disconnected during slow tests.
			AgentInactiveDisconnectTimeout: testutil.WaitShort,
			AccessURL:                      serverURL,
			AppHostname:                    options.AppHostname,
			AppHostnameRegex:               appHostnameRegex,
			Logger:                         slogtest.Make(t, nil).Leveled(slog.LevelDebug),
			CacheDir:                       t.TempDir(),
			Database:                       options.Database,
			Pubsub:                         options.Pubsub,
			Experimental:                   options.Experimental,

			Auditor:              options.Auditor,
			AWSCertificates:      options.AWSCertificates,
			AzureCertificates:    options.AzureCertificates,
			GithubOAuth2Config:   options.GithubOAuth2Config,
			OIDCConfig:           options.OIDCConfig,
			GoogleTokenValidator: options.GoogleTokenValidator,
			SSHKeygenAlgorithm:   options.SSHKeygenAlgorithm,
			DERPServer:           derpServer,
			APIRateLimit:         options.APIRateLimit,
			Authorizer:           options.Authorizer,
			Telemetry:            telemetry.NewNoop(),
			TLSCertificates:      options.TLSCertificates,
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
			AutoImportTemplates:         options.AutoImportTemplates,
			MetricsCacheRefreshInterval: options.MetricsCacheRefreshInterval,
			AgentStatsRefreshInterval:   options.AgentStatsRefreshInterval,
			DeploymentFlags:             options.DeploymentFlags,
		}
}

// NewWithAPI constructs an in-memory API instance and returns a client to talk to it.
// Most tests never need a reference to the API, but AuthorizationTest in this module uses it.
// Do not expose the API or wrath shall descend upon thee.
func NewWithAPI(t *testing.T, options *Options) (*codersdk.Client, io.Closer, *coderd.API) {
	if options == nil {
		options = &Options{}
	}
	setHandler, cancelFunc, newOptions := NewOptions(t, options)
	// We set the handler after server creation for the access URL.
	coderAPI := coderd.New(newOptions)
	setHandler(coderAPI.RootHandler)
	var provisionerCloser io.Closer = nopcloser{}
	if options.IncludeProvisionerDaemon {
		provisionerCloser = NewProvisionerDaemon(t, coderAPI)
	}
	t.Cleanup(func() {
		cancelFunc()
		_ = provisionerCloser.Close()
		_ = coderAPI.Close()
	})
	return codersdk.New(coderAPI.AccessURL), provisionerCloser, coderAPI
}

// NewProvisionerDaemon launches a provisionerd instance configured to work
// well with coderd testing. It registers the "echo" provisioner for
// quick testing.
func NewProvisionerDaemon(t *testing.T, coderAPI *coderd.API) io.Closer {
	echoClient, echoServer := provisionersdk.TransportPipe()
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

	closer := provisionerd.New(coderAPI.ListenProvisionerDaemon, &provisionerd.Options{
		Filesystem:          fs,
		Logger:              slogtest.Make(t, nil).Named("provisionerd").Leveled(slog.LevelDebug),
		PollInterval:        50 * time.Millisecond,
		UpdateInterval:      250 * time.Millisecond,
		ForceCancelInterval: time.Second,
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

var FirstUserParams = codersdk.CreateFirstUserRequest{
	Email:            "testuser@coder.com",
	Username:         "testuser",
	Password:         "testpass",
	OrganizationName: "testorg",
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
	client.SessionToken = login.SessionToken
	return resp
}

// CreateAnotherUser creates and authenticates a new user.
func CreateAnotherUser(t *testing.T, client *codersdk.Client, organizationID uuid.UUID, roles ...string) *codersdk.Client {
	userClient, _ := createAnotherUserRetry(t, client, organizationID, 5, roles...)
	return userClient
}

func CreateAnotherUserWithUser(t *testing.T, client *codersdk.Client, organizationID uuid.UUID, roles ...string) (*codersdk.Client, codersdk.User) {
	return createAnotherUserRetry(t, client, organizationID, 5, roles...)
}

func createAnotherUserRetry(t *testing.T, client *codersdk.Client, organizationID uuid.UUID, retries int, roles ...string) (*codersdk.Client, codersdk.User) {
	req := codersdk.CreateUserRequest{
		Email:          namesgenerator.GetRandomName(10) + "@coder.com",
		Username:       randomUsername(),
		Password:       "testpass",
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
	other.SessionToken = login.SessionToken

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
	file, err := client.Upload(context.Background(), codersdk.ContentTypeTar, data)
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
	file, err := client.Upload(context.Background(), codersdk.ContentTypeTar, data)
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

	t.Logf("waiting for template version job %s", version)
	var templateVersion codersdk.TemplateVersion
	require.Eventually(t, func() bool {
		var err error
		templateVersion, err = client.TemplateVersion(context.Background(), version)
		return assert.NoError(t, err) && templateVersion.Job.CompletedAt != nil
	}, testutil.WaitMedium, testutil.IntervalFast)
	return templateVersion
}

// AwaitWorkspaceBuildJob waits for a workspace provision job to reach completed status.
func AwaitWorkspaceBuildJob(t *testing.T, client *codersdk.Client, build uuid.UUID) codersdk.WorkspaceBuild {
	t.Helper()

	t.Logf("waiting for workspace build job %s", build)
	var workspaceBuild codersdk.WorkspaceBuild
	require.Eventually(t, func() bool {
		workspaceBuild, err := client.WorkspaceBuild(context.Background(), build)
		return assert.NoError(t, err) && workspaceBuild.Job.CompletedAt != nil
	}, testutil.WaitShort, testutil.IntervalFast)
	return workspaceBuild
}

// AwaitWorkspaceAgents waits for all resources with agents to be connected.
func AwaitWorkspaceAgents(t *testing.T, client *codersdk.Client, workspaceID uuid.UUID) []codersdk.WorkspaceResource {
	t.Helper()

	t.Logf("waiting for workspace agents (workspace %s)", workspaceID)
	var resources []codersdk.WorkspaceResource
	require.Eventually(t, func() bool {
		var err error
		workspace, err := client.Workspace(context.Background(), workspaceID)
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

	payload, err := json.Marshal(codersdk.AzureInstanceIdentityToken{
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

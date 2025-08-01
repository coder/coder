package coderd

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"errors"
	"expvar"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/coder/v2/coderd/oauth2provider"
	"github.com/coder/coder/v2/coderd/prebuilds"
	"github.com/coder/coder/v2/coderd/wsbuilder"

	"github.com/andybalholm/brotli"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/klauspost/compress/zstd"
	"github.com/prometheus/client_golang/prometheus"
	httpSwagger "github.com/swaggo/http-swagger/v2"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"
	"google.golang.org/api/idtoken"
	"storj.io/drpc/drpcmux"
	"storj.io/drpc/drpcserver"
	"tailscale.com/derp"
	"tailscale.com/derp/derphttp"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"
	"tailscale.com/util/singleflight"

	"cdr.dev/slog"
	"github.com/coder/quartz"
	"github.com/coder/serpent"

	"github.com/coder/coder/v2/codersdk/drpcsdk"

	"github.com/coder/coder/v2/coderd/cryptokeys"
	"github.com/coder/coder/v2/coderd/entitlements"
	"github.com/coder/coder/v2/coderd/files"
	"github.com/coder/coder/v2/coderd/idpsync"
	"github.com/coder/coder/v2/coderd/runtimeconfig"
	"github.com/coder/coder/v2/coderd/webpush"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/buildinfo"
	_ "github.com/coder/coder/v2/coderd/apidoc" // Used for swagger docs.
	"github.com/coder/coder/v2/coderd/appearance"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/awsidentity"
	"github.com/coder/coder/v2/coderd/connectionlog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbrollup"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/externalauth"
	"github.com/coder/coder/v2/coderd/gitsshkey"
	"github.com/coder/coder/v2/coderd/healthcheck"
	"github.com/coder/coder/v2/coderd/healthcheck/derphealth"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/httpmw/loggermw"
	"github.com/coder/coder/v2/coderd/metricscache"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/portsharing"
	"github.com/coder/coder/v2/coderd/prometheusmetrics"
	"github.com/coder/coder/v2/coderd/provisionerdserver"
	"github.com/coder/coder/v2/coderd/proxyhealth"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/rbac/rolestore"
	"github.com/coder/coder/v2/coderd/schedule"
	"github.com/coder/coder/v2/coderd/telemetry"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/coderd/updatecheck"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/coderd/workspaceapps"
	"github.com/coder/coder/v2/coderd/workspaceapps/appurl"
	"github.com/coder/coder/v2/coderd/workspacestats"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/healthsdk"
	"github.com/coder/coder/v2/provisionerd/proto"
	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/coder/v2/site"
	"github.com/coder/coder/v2/tailnet"
)

// We must only ever instantiate one httpSwagger.Handler because of a data race
// inside the handler. This issue is triggered by tests that create multiple
// coderd instances.
//
// See https://github.com/swaggo/http-swagger/issues/78
var globalHTTPSwaggerHandler http.HandlerFunc

func init() {
	globalHTTPSwaggerHandler = httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
		// The swagger UI has an "Authorize" button that will input the
		// credentials into the Coder-Session-Token header. This bypasses
		// CSRF checks **if** there is no cookie auth also present.
		// (If the cookie matches, then it's ok too)
		//
		// Because swagger is hosted on the same domain, we have the cookie
		// auth and the header auth competing. This can cause CSRF errors,
		// and can be confusing what authentication is being used.
		//
		// So remove authenticating via a cookie, and rely on the authorization
		// header passed in.
		httpSwagger.UIConfig(map[string]string{
			// Pulled from https://swagger.io/docs/open-source-tools/swagger-ui/usage/configuration/
			// 'withCredentials' should disable fetch sending browser credentials, but
			// for whatever reason it does not.
			// So this `requestInterceptor` ensures browser credentials are
			// omitted from all requests.
			"requestInterceptor": `(a => {
				a.credentials = "omit";
				return a;
			})`,
			"withCredentials": "false",
		}))
}

var expDERPOnce = sync.Once{}

// Options are requires parameters for Coder to start.
type Options struct {
	AccessURL *url.URL
	// AppHostname should be the wildcard hostname to use for workspace
	// applications INCLUDING the asterisk, (optional) suffix and leading dot.
	// It will use the same scheme and port number as the access URL.
	// E.g. "*.apps.coder.com" or "*-apps.coder.com" or "*.apps.coder.com:8080".
	AppHostname string
	// AppHostnameRegex contains the regex version of options.AppHostname as
	// generated by appurl.CompileHostnamePattern(). It MUST be set if
	// options.AppHostname is set.
	AppHostnameRegex *regexp.Regexp
	Logger           slog.Logger
	Database         database.Store
	Pubsub           pubsub.Pubsub
	RuntimeConfig    *runtimeconfig.Manager

	// CacheDir is used for caching files served by the API.
	CacheDir string

	Auditor                        audit.Auditor
	ConnectionLogger               connectionlog.ConnectionLogger
	AgentConnectionUpdateFrequency time.Duration
	AgentInactiveDisconnectTimeout time.Duration
	AWSCertificates                awsidentity.Certificates
	Authorizer                     rbac.Authorizer
	AzureCertificates              x509.VerifyOptions
	GoogleTokenValidator           *idtoken.Validator
	GithubOAuth2Config             *GithubOAuth2Config
	OIDCConfig                     *OIDCConfig
	PrometheusRegistry             *prometheus.Registry
	StrictTransportSecurityCfg     httpmw.HSTSConfig
	SSHKeygenAlgorithm             gitsshkey.Algorithm
	Telemetry                      telemetry.Reporter
	TracerProvider                 trace.TracerProvider
	ExternalAuthConfigs            []*externalauth.Config
	RealIPConfig                   *httpmw.RealIPConfig
	TrialGenerator                 func(ctx context.Context, body codersdk.LicensorTrialRequest) error
	// RefreshEntitlements is used to set correct entitlements after creating first user and generating trial license.
	RefreshEntitlements func(ctx context.Context) error
	// Entitlements can come from the enterprise caller if enterprise code is
	// included.
	Entitlements *entitlements.Set
	// PostAuthAdditionalHeadersFunc is used to add additional headers to the response
	// after a successful authentication.
	// This is somewhat janky, but seemingly the only reasonable way to add a header
	// for all authenticated users under a condition, only in Enterprise.
	PostAuthAdditionalHeadersFunc func(auth rbac.Subject, header http.Header)

	// TLSCertificates is used to mesh DERP servers securely.
	TLSCertificates    []tls.Certificate
	TailnetCoordinator tailnet.Coordinator
	DERPServer         *derp.Server
	// BaseDERPMap is used as the base DERP map for all clients and agents.
	// Proxies are added to this list.
	BaseDERPMap                    *tailcfg.DERPMap
	DERPMapUpdateFrequency         time.Duration
	NetworkTelemetryBatchFrequency time.Duration
	NetworkTelemetryBatchMaxSize   int
	SwaggerEndpoint                bool
	TemplateScheduleStore          *atomic.Pointer[schedule.TemplateScheduleStore]
	UserQuietHoursScheduleStore    *atomic.Pointer[schedule.UserQuietHoursScheduleStore]
	AccessControlStore             *atomic.Pointer[dbauthz.AccessControlStore]
	// CoordinatorResumeTokenProvider is used to provide and validate resume
	// tokens issued by and passed to the coordinator DRPC API.
	CoordinatorResumeTokenProvider tailnet.ResumeTokenProvider

	HealthcheckFunc              func(ctx context.Context, apiKey string) *healthsdk.HealthcheckReport
	HealthcheckTimeout           time.Duration
	HealthcheckRefresh           time.Duration
	WorkspaceProxiesFetchUpdater *atomic.Pointer[healthcheck.WorkspaceProxiesFetchUpdater]

	// OAuthSigningKey is the crypto key used to sign and encrypt state strings
	// related to OAuth. This is a symmetric secret key using hmac to sign payloads.
	// So this secret should **never** be exposed to the client.
	OAuthSigningKey [32]byte

	// APIRateLimit is the minutely throughput rate limit per user or ip.
	// Setting a rate limit <0 will disable the rate limiter across the entire
	// app. Some specific routes have their own configurable rate limits.
	APIRateLimit   int
	LoginRateLimit int
	FilesRateLimit int

	MetricsCacheRefreshInterval time.Duration
	AgentStatsRefreshInterval   time.Duration
	DeploymentValues            *codersdk.DeploymentValues
	// DeploymentOptions do contain the copy of DeploymentValues, and contain
	// contextual information about how the values were set.
	// Do not use DeploymentOptions to retrieve values, use DeploymentValues instead.
	// All secrets values are stripped.
	DeploymentOptions  serpent.OptionSet
	UpdateCheckOptions *updatecheck.Options // Set non-nil to enable update checking.

	// SSHConfig is the response clients use to configure config-ssh locally.
	SSHConfig codersdk.SSHConfigResponse

	HTTPClient *http.Client

	UpdateAgentMetrics func(ctx context.Context, labels prometheusmetrics.AgentMetricLabels, metrics []*agentproto.Stats_Metric)
	StatsBatcher       workspacestats.Batcher

	// WorkspaceAppAuditSessionTimeout allows changing the timeout for audit
	// sessions. Raising or lowering this value will directly affect the write
	// load of the audit log table. This is used for testing. Default 1 hour.
	WorkspaceAppAuditSessionTimeout    time.Duration
	WorkspaceAppsStatsCollectorOptions workspaceapps.StatsCollectorOptions

	// This janky function is used in telemetry to parse fields out of the raw
	// JWT. It needs to be passed through like this because license parsing is
	// under the enterprise license, and can't be imported into AGPL.
	ParseLicenseClaims    func(rawJWT string) (email string, trial bool, err error)
	AllowWorkspaceRenames bool

	// NewTicker is used for unit tests to replace "time.NewTicker".
	NewTicker func(duration time.Duration) (tick <-chan time.Time, done func())

	// DatabaseRolluper rolls up template usage stats from raw agent and app
	// stats. This is used to provide insights in the WebUI.
	DatabaseRolluper *dbrollup.Rolluper
	// WorkspaceUsageTracker tracks workspace usage by the CLI.
	WorkspaceUsageTracker *workspacestats.UsageTracker
	// NotificationsEnqueuer handles enqueueing notifications for delivery by SMTP, webhook, etc.
	NotificationsEnqueuer notifications.Enqueuer

	// IDPSync holds all configured values for syncing external IDP users into Coder.
	IDPSync idpsync.IDPSync

	// OneTimePasscodeValidityPeriod specifies how long a one time passcode should be valid for.
	OneTimePasscodeValidityPeriod time.Duration

	// Keycaches
	AppSigningKeyCache    cryptokeys.SigningKeycache
	AppEncryptionKeyCache cryptokeys.EncryptionKeycache
	OIDCConvertKeyCache   cryptokeys.SigningKeycache
	Clock                 quartz.Clock

	// WebPushDispatcher is a way to send notifications over Web Push.
	WebPushDispatcher webpush.Dispatcher
}

// @title Coder API
// @version 2.0
// @description Coderd is the service created by running coder server. It is a thin API that connects workspaces, provisioners and users. coderd stores its state in Postgres and is the only service that communicates with Postgres.
// @termsOfService https://coder.com/legal/terms-of-service

// @contact.name API Support
// @contact.url https://coder.com
// @contact.email support@coder.com

// @license.name AGPL-3.0
// @license.url https://github.com/coder/coder/blob/main/LICENSE

// @BasePath /api/v2

// @securitydefinitions.apiKey Authorization
// @in header
// @name Authorizaiton

// @securitydefinitions.apiKey CoderSessionToken
// @in header
// @name Coder-Session-Token
// New constructs a Coder API handler.
func New(options *Options) *API {
	if options == nil {
		options = &Options{}
	}
	if options.Entitlements == nil {
		options.Entitlements = entitlements.New()
	}
	if options.NewTicker == nil {
		options.NewTicker = func(duration time.Duration) (tick <-chan time.Time, done func()) {
			ticker := time.NewTicker(duration)
			return ticker.C, ticker.Stop
		}
	}

	// Safety check: if we're not running a unit test, we *must* have a Prometheus registry.
	if options.PrometheusRegistry == nil && flag.Lookup("test.v") == nil {
		panic("developer error: options.PrometheusRegistry is nil and not running a unit test")
	}

	if options.DeploymentValues.DisableOwnerWorkspaceExec {
		rbac.ReloadBuiltinRoles(&rbac.RoleOptions{
			NoOwnerWorkspaceExec: true,
		})
	}

	if options.Authorizer == nil {
		options.Authorizer = rbac.NewCachingAuthorizer(options.PrometheusRegistry)
		if buildinfo.IsDev() {
			options.Authorizer = rbac.Recorder(options.Authorizer)
		}
	}

	if options.AccessControlStore == nil {
		options.AccessControlStore = &atomic.Pointer[dbauthz.AccessControlStore]{}
		var tacs dbauthz.AccessControlStore = dbauthz.AGPLTemplateAccessControlStore{}
		options.AccessControlStore.Store(&tacs)
	}

	options.Database = dbauthz.New(
		options.Database,
		options.Authorizer,
		options.Logger.Named("authz_querier"),
		options.AccessControlStore,
	)

	if options.IDPSync == nil {
		options.IDPSync = idpsync.NewAGPLSync(options.Logger, options.RuntimeConfig, idpsync.FromDeploymentValues(options.DeploymentValues))
	}

	experiments := ReadExperiments(
		options.Logger, options.DeploymentValues.Experiments.Value(),
	)
	if options.AppHostname != "" && options.AppHostnameRegex == nil || options.AppHostname == "" && options.AppHostnameRegex != nil {
		panic("coderd: both AppHostname and AppHostnameRegex must be set or unset")
	}
	if options.AgentConnectionUpdateFrequency == 0 {
		options.AgentConnectionUpdateFrequency = 15 * time.Second
	}
	if options.AgentInactiveDisconnectTimeout == 0 {
		// Multiply the update by two to allow for some lag-time.
		options.AgentInactiveDisconnectTimeout = options.AgentConnectionUpdateFrequency * 2
		// Set a minimum timeout to avoid disconnecting too soon.
		if options.AgentInactiveDisconnectTimeout < 2*time.Second {
			options.AgentInactiveDisconnectTimeout = 2 * time.Second
		}
	}
	if options.AgentStatsRefreshInterval == 0 {
		options.AgentStatsRefreshInterval = 5 * time.Minute
	}
	if options.MetricsCacheRefreshInterval == 0 {
		options.MetricsCacheRefreshInterval = time.Hour
	}
	if options.APIRateLimit == 0 {
		options.APIRateLimit = 512
	}
	if options.LoginRateLimit == 0 {
		options.LoginRateLimit = 60
	}
	if options.FilesRateLimit == 0 {
		options.FilesRateLimit = 12
	}
	if options.PrometheusRegistry == nil {
		options.PrometheusRegistry = prometheus.NewRegistry()
	}
	if options.Clock == nil {
		options.Clock = quartz.NewReal()
	}
	if options.DERPServer == nil && options.DeploymentValues.DERP.Server.Enable {
		options.DERPServer = derp.NewServer(key.NewNode(), tailnet.Logger(options.Logger.Named("derp")))
	}
	if options.DERPMapUpdateFrequency == 0 {
		options.DERPMapUpdateFrequency = 5 * time.Second
	}
	if options.NetworkTelemetryBatchFrequency == 0 {
		options.NetworkTelemetryBatchFrequency = 1 * time.Minute
	}
	if options.NetworkTelemetryBatchMaxSize == 0 {
		options.NetworkTelemetryBatchMaxSize = 1_000
	}
	if options.TailnetCoordinator == nil {
		options.TailnetCoordinator = tailnet.NewCoordinator(options.Logger)
	}
	if options.Auditor == nil {
		options.Auditor = audit.NewNop()
	}
	if options.ConnectionLogger == nil {
		options.ConnectionLogger = connectionlog.NewNop()
	}
	if options.SSHConfig.HostnamePrefix == "" {
		options.SSHConfig.HostnamePrefix = "coder."
	}
	if options.TracerProvider == nil {
		options.TracerProvider = trace.NewNoopTracerProvider()
	}
	if options.TemplateScheduleStore == nil {
		options.TemplateScheduleStore = &atomic.Pointer[schedule.TemplateScheduleStore]{}
	}
	if options.TemplateScheduleStore.Load() == nil {
		v := schedule.NewAGPLTemplateScheduleStore()
		options.TemplateScheduleStore.Store(&v)
	}
	if options.UserQuietHoursScheduleStore == nil {
		options.UserQuietHoursScheduleStore = &atomic.Pointer[schedule.UserQuietHoursScheduleStore]{}
	}
	if options.UserQuietHoursScheduleStore.Load() == nil {
		v := schedule.NewAGPLUserQuietHoursScheduleStore()
		options.UserQuietHoursScheduleStore.Store(&v)
	}
	if options.OneTimePasscodeValidityPeriod == 0 {
		options.OneTimePasscodeValidityPeriod = 20 * time.Minute
	}

	if options.StatsBatcher == nil {
		panic("developer error: options.StatsBatcher is nil")
	}

	siteCacheDir := options.CacheDir
	if siteCacheDir != "" {
		siteCacheDir = filepath.Join(siteCacheDir, "site")
	}
	binFS, binHashes, err := site.ExtractOrReadBinFS(siteCacheDir, site.FS())
	if err != nil {
		panic(xerrors.Errorf("read site bin failed: %w", err))
	}

	metricsCache := metricscache.New(
		options.Database,
		options.Logger.Named("metrics_cache"),
		options.Clock,
		metricscache.Intervals{
			TemplateBuildTimes: options.MetricsCacheRefreshInterval,
			DeploymentStats:    options.AgentStatsRefreshInterval,
		},
		experiments.Enabled(codersdk.ExperimentWorkspaceUsage),
	)

	oauthConfigs := &httpmw.OAuth2Configs{
		Github: options.GithubOAuth2Config,
		OIDC:   options.OIDCConfig,
	}

	if options.DatabaseRolluper == nil {
		options.DatabaseRolluper = dbrollup.New(options.Logger.Named("dbrollup"), options.Database)
	}

	if options.WorkspaceUsageTracker == nil {
		options.WorkspaceUsageTracker = workspacestats.NewTracker(options.Database,
			workspacestats.TrackerWithLogger(options.Logger.Named("workspace_usage_tracker")),
		)
	}

	if options.NotificationsEnqueuer == nil {
		options.NotificationsEnqueuer = notifications.NewNoopEnqueuer()
	}

	r := chi.NewRouter()
	// We add this middleware early, to make sure that authorization checks made
	// by other middleware get recorded.
	//nolint:revive,staticcheck // This block will be re-enabled, not going to remove it
	if buildinfo.IsDev() {
		// TODO: Find another solution to opt into these checks.
		//   If the header grows too large, it breaks `fetch()` requests.
		//   Temporarily disabling this until we can find a better solution.
		//	 One idea is to include checking the request for `X-Authz-Record=true`
		//   header. To opt in on a per-request basis.
		//   Some authz calls (like filtering lists) might be able to be
		//   summarized better to condense the header payload.
		// r.Use(httpmw.RecordAuthzChecks)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// nolint:gocritic // Load deployment ID. This never changes
	depID, err := options.Database.GetDeploymentID(dbauthz.AsSystemRestricted(ctx))
	if err != nil {
		panic(xerrors.Errorf("get deployment ID: %w", err))
	}

	fetcher := &cryptokeys.DBFetcher{
		DB: options.Database,
	}

	if options.OIDCConvertKeyCache == nil {
		options.OIDCConvertKeyCache, err = cryptokeys.NewSigningCache(ctx,
			options.Logger.Named("oidc_convert_keycache"),
			fetcher,
			codersdk.CryptoKeyFeatureOIDCConvert,
		)
		if err != nil {
			options.Logger.Fatal(ctx, "failed to properly instantiate oidc convert signing cache", slog.Error(err))
		}
	}

	if options.AppSigningKeyCache == nil {
		options.AppSigningKeyCache, err = cryptokeys.NewSigningCache(ctx,
			options.Logger.Named("app_signing_keycache"),
			fetcher,
			codersdk.CryptoKeyFeatureWorkspaceAppsToken,
		)
		if err != nil {
			options.Logger.Fatal(ctx, "failed to properly instantiate app signing key cache", slog.Error(err))
		}
	}

	if options.AppEncryptionKeyCache == nil {
		options.AppEncryptionKeyCache, err = cryptokeys.NewEncryptionCache(ctx,
			options.Logger,
			fetcher,
			codersdk.CryptoKeyFeatureWorkspaceAppsAPIKey,
		)
		if err != nil {
			options.Logger.Fatal(ctx, "failed to properly instantiate app encryption key cache", slog.Error(err))
		}
	}

	if options.CoordinatorResumeTokenProvider == nil {
		fetcher := &cryptokeys.DBFetcher{
			DB: options.Database,
		}

		resumeKeycache, err := cryptokeys.NewSigningCache(ctx,
			options.Logger,
			fetcher,
			codersdk.CryptoKeyFeatureTailnetResume,
		)
		if err != nil {
			options.Logger.Fatal(ctx, "failed to properly instantiate tailnet resume signing cache", slog.Error(err))
		}
		options.CoordinatorResumeTokenProvider = tailnet.NewResumeTokenKeyProvider(
			resumeKeycache,
			options.Clock,
			tailnet.DefaultResumeTokenExpiry,
		)
	}

	updatesProvider := NewUpdatesProvider(options.Logger.Named("workspace_updates"), options.Pubsub, options.Database, options.Authorizer)

	// Start a background process that rotates keys. We intentionally start this after the caches
	// are created to force initial requests for a key to populate the caches. This helps catch
	// bugs that may only occur when a key isn't precached in tests and the latency cost is minimal.
	cryptokeys.StartRotator(ctx, options.Logger, options.Database)

	// AGPL uses a no-op build usage checker as there are no license
	// entitlements to enforce. This is swapped out in
	// enterprise/coderd/coderd.go.
	var buildUsageChecker atomic.Pointer[wsbuilder.UsageChecker]
	var noopUsageChecker wsbuilder.UsageChecker = wsbuilder.NoopUsageChecker{}
	buildUsageChecker.Store(&noopUsageChecker)

	api := &API{
		ctx:          ctx,
		cancel:       cancel,
		DeploymentID: depID,

		ID:          uuid.New(),
		Options:     options,
		RootHandler: r,
		HTTPAuth: &HTTPAuthorizer{
			Authorizer: options.Authorizer,
			Logger:     options.Logger,
		},
		metricsCache:                metricsCache,
		Auditor:                     atomic.Pointer[audit.Auditor]{},
		ConnectionLogger:            atomic.Pointer[connectionlog.ConnectionLogger]{},
		TailnetCoordinator:          atomic.Pointer[tailnet.Coordinator]{},
		UpdatesProvider:             updatesProvider,
		TemplateScheduleStore:       options.TemplateScheduleStore,
		UserQuietHoursScheduleStore: options.UserQuietHoursScheduleStore,
		AccessControlStore:          options.AccessControlStore,
		BuildUsageChecker:           &buildUsageChecker,
		FileCache:                   files.New(options.PrometheusRegistry, options.Authorizer),
		Experiments:                 experiments,
		WebpushDispatcher:           options.WebPushDispatcher,
		healthCheckGroup:            &singleflight.Group[string, *healthsdk.HealthcheckReport]{},
		Acquirer: provisionerdserver.NewAcquirer(
			ctx,
			options.Logger.Named("acquirer"),
			options.Database,
			options.Pubsub,
		),
		dbRolluper: options.DatabaseRolluper,
	}
	api.WorkspaceAppsProvider = workspaceapps.NewDBTokenProvider(
		options.Logger.Named("workspaceapps"),
		options.AccessURL,
		options.Authorizer,
		&api.ConnectionLogger,
		options.Database,
		options.DeploymentValues,
		oauthConfigs,
		options.AgentInactiveDisconnectTimeout,
		options.WorkspaceAppAuditSessionTimeout,
		options.AppSigningKeyCache,
	)

	f := appearance.NewDefaultFetcher(api.DeploymentValues.DocsURL.String())
	api.AppearanceFetcher.Store(&f)
	api.PortSharer.Store(&portsharing.DefaultPortSharer)
	api.PrebuildsClaimer.Store(&prebuilds.DefaultClaimer)
	api.PrebuildsReconciler.Store(&prebuilds.DefaultReconciler)
	buildInfo := codersdk.BuildInfoResponse{
		ExternalURL:           buildinfo.ExternalURL(),
		Version:               buildinfo.Version(),
		AgentAPIVersion:       AgentAPIVersionREST,
		ProvisionerAPIVersion: proto.CurrentVersion.String(),
		DashboardURL:          api.AccessURL.String(),
		WorkspaceProxy:        false,
		UpgradeMessage:        api.DeploymentValues.CLIUpgradeMessage.String(),
		DeploymentID:          api.DeploymentID,
		WebPushPublicKey:      api.WebpushDispatcher.PublicKey(),
		Telemetry:             api.Telemetry.Enabled(),
	}
	api.SiteHandler = site.New(&site.Options{
		BinFS:             binFS,
		BinHashes:         binHashes,
		Database:          options.Database,
		SiteFS:            site.FS(),
		OAuth2Configs:     oauthConfigs,
		DocsURL:           options.DeploymentValues.DocsURL.String(),
		AppearanceFetcher: &api.AppearanceFetcher,
		BuildInfo:         buildInfo,
		Entitlements:      options.Entitlements,
		Telemetry:         options.Telemetry,
		Logger:            options.Logger.Named("site"),
		HideAITasks:       options.DeploymentValues.HideAITasks.Value(),
	})
	api.SiteHandler.Experiments.Store(&experiments)

	if options.UpdateCheckOptions != nil {
		api.updateChecker = updatecheck.New(
			options.Database,
			options.Logger.Named("update_checker"),
			*options.UpdateCheckOptions,
		)
	}

	if options.WorkspaceProxiesFetchUpdater == nil {
		options.WorkspaceProxiesFetchUpdater = &atomic.Pointer[healthcheck.WorkspaceProxiesFetchUpdater]{}
		var wpfu healthcheck.WorkspaceProxiesFetchUpdater = &healthcheck.AGPLWorkspaceProxiesFetchUpdater{}
		options.WorkspaceProxiesFetchUpdater.Store(&wpfu)
	}

	if options.HealthcheckFunc == nil {
		options.HealthcheckFunc = func(ctx context.Context, apiKey string) *healthsdk.HealthcheckReport {
			// NOTE: dismissed healthchecks are marked in formatHealthcheck.
			// Not here, as this result gets cached.
			return healthcheck.Run(ctx, &healthcheck.ReportOptions{
				Database: healthcheck.DatabaseReportOptions{
					DB:        options.Database,
					Threshold: options.DeploymentValues.Healthcheck.ThresholdDatabase.Value(),
				},
				Websocket: healthcheck.WebsocketReportOptions{
					AccessURL: options.AccessURL,
					APIKey:    apiKey,
				},
				AccessURL: healthcheck.AccessURLReportOptions{
					AccessURL: options.AccessURL,
				},
				DerpHealth: derphealth.ReportOptions{
					DERPMap: api.DERPMap(),
				},
				WorkspaceProxy: healthcheck.WorkspaceProxyReportOptions{
					WorkspaceProxiesFetchUpdater: *(options.WorkspaceProxiesFetchUpdater).Load(),
				},
				ProvisionerDaemons: healthcheck.ProvisionerDaemonsReportDeps{
					CurrentVersion:         buildinfo.Version(),
					CurrentAPIMajorVersion: proto.CurrentMajor,
					Store:                  options.Database,
					StaleInterval:          provisionerdserver.StaleInterval,
					// TimeNow set to default, see healthcheck/provisioner.go
				},
			})
		}
	}

	if options.HealthcheckTimeout == 0 {
		options.HealthcheckTimeout = 30 * time.Second
	}
	if options.HealthcheckRefresh == 0 {
		options.HealthcheckRefresh = options.DeploymentValues.Healthcheck.Refresh.Value()
	}

	var oidcAuthURLParams map[string]string
	if options.OIDCConfig != nil {
		oidcAuthURLParams = options.OIDCConfig.AuthURLParams
	}

	api.Auditor.Store(&options.Auditor)
	api.ConnectionLogger.Store(&options.ConnectionLogger)
	api.TailnetCoordinator.Store(&options.TailnetCoordinator)
	dialer := &InmemTailnetDialer{
		CoordPtr:            &api.TailnetCoordinator,
		DERPFn:              api.DERPMap,
		Logger:              options.Logger,
		ClientID:            uuid.New(),
		DatabaseHealthCheck: api.Database,
	}
	stn, err := NewServerTailnet(api.ctx,
		options.Logger,
		options.DERPServer,
		dialer,
		options.DeploymentValues.DERP.Config.ForceWebSockets.Value(),
		options.DeploymentValues.DERP.Config.BlockDirect.Value(),
		api.TracerProvider,
	)
	if err != nil {
		panic("failed to setup server tailnet: " + err.Error())
	}
	api.agentProvider = stn
	if options.DeploymentValues.Prometheus.Enable {
		options.PrometheusRegistry.MustRegister(stn)
	}
	api.NetworkTelemetryBatcher = tailnet.NewNetworkTelemetryBatcher(
		quartz.NewReal(),
		api.Options.NetworkTelemetryBatchFrequency,
		api.Options.NetworkTelemetryBatchMaxSize,
		api.handleNetworkTelemetry,
	)
	if options.CoordinatorResumeTokenProvider == nil {
		panic("CoordinatorResumeTokenProvider is nil")
	}
	api.TailnetClientService, err = tailnet.NewClientService(tailnet.ClientServiceOptions{
		Logger:                   api.Logger.Named("tailnetclient"),
		CoordPtr:                 &api.TailnetCoordinator,
		DERPMapUpdateFrequency:   api.Options.DERPMapUpdateFrequency,
		DERPMapFn:                api.DERPMap,
		NetworkTelemetryHandler:  api.NetworkTelemetryBatcher.Handler,
		ResumeTokenProvider:      api.Options.CoordinatorResumeTokenProvider,
		WorkspaceUpdatesProvider: api.UpdatesProvider,
	})
	if err != nil {
		api.Logger.Fatal(context.Background(), "failed to initialize tailnet client service", slog.Error(err))
	}

	api.statsReporter = workspacestats.NewReporter(workspacestats.ReporterOptions{
		Database:              options.Database,
		Logger:                options.Logger.Named("workspacestats"),
		Pubsub:                options.Pubsub,
		TemplateScheduleStore: options.TemplateScheduleStore,
		StatsBatcher:          options.StatsBatcher,
		UsageTracker:          options.WorkspaceUsageTracker,
		UpdateAgentMetricsFn:  options.UpdateAgentMetrics,
		AppStatBatchSize:      workspaceapps.DefaultStatsDBReporterBatchSize,
	})
	workspaceAppsLogger := options.Logger.Named("workspaceapps")
	if options.WorkspaceAppsStatsCollectorOptions.Logger == nil {
		named := workspaceAppsLogger.Named("stats_collector")
		options.WorkspaceAppsStatsCollectorOptions.Logger = &named
	}
	if options.WorkspaceAppsStatsCollectorOptions.Reporter == nil {
		options.WorkspaceAppsStatsCollectorOptions.Reporter = api.statsReporter
	}

	api.workspaceAppServer = &workspaceapps.Server{
		Logger: workspaceAppsLogger,

		DashboardURL:  api.AccessURL,
		AccessURL:     api.AccessURL,
		Hostname:      api.AppHostname,
		HostnameRegex: api.AppHostnameRegex,
		RealIPConfig:  options.RealIPConfig,

		SignedTokenProvider: api.WorkspaceAppsProvider,
		AgentProvider:       api.agentProvider,
		StatsCollector:      workspaceapps.NewStatsCollector(options.WorkspaceAppsStatsCollectorOptions),

		DisablePathApps:          options.DeploymentValues.DisablePathApps.Value(),
		Cookies:                  options.DeploymentValues.HTTPCookies,
		APIKeyEncryptionKeycache: options.AppEncryptionKeyCache,
	}

	apiKeyMiddleware := httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
		DB:                            options.Database,
		ActivateDormantUser:           ActivateDormantUser(options.Logger, &api.Auditor, options.Database),
		OAuth2Configs:                 oauthConfigs,
		RedirectToLogin:               false,
		DisableSessionExpiryRefresh:   options.DeploymentValues.Sessions.DisableExpiryRefresh.Value(),
		Optional:                      false,
		SessionTokenFunc:              nil, // Default behavior
		PostAuthAdditionalHeadersFunc: options.PostAuthAdditionalHeadersFunc,
		Logger:                        options.Logger,
		AccessURL:                     options.AccessURL,
	})
	// Same as above but it redirects to the login page.
	apiKeyMiddlewareRedirect := httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
		DB:                            options.Database,
		OAuth2Configs:                 oauthConfigs,
		RedirectToLogin:               true,
		DisableSessionExpiryRefresh:   options.DeploymentValues.Sessions.DisableExpiryRefresh.Value(),
		Optional:                      false,
		SessionTokenFunc:              nil, // Default behavior
		PostAuthAdditionalHeadersFunc: options.PostAuthAdditionalHeadersFunc,
		Logger:                        options.Logger,
		AccessURL:                     options.AccessURL,
	})
	// Same as the first but it's optional.
	apiKeyMiddlewareOptional := httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
		DB:                            options.Database,
		OAuth2Configs:                 oauthConfigs,
		RedirectToLogin:               false,
		DisableSessionExpiryRefresh:   options.DeploymentValues.Sessions.DisableExpiryRefresh.Value(),
		Optional:                      true,
		SessionTokenFunc:              nil, // Default behavior
		PostAuthAdditionalHeadersFunc: options.PostAuthAdditionalHeadersFunc,
		Logger:                        options.Logger,
		AccessURL:                     options.AccessURL,
	})

	workspaceAgentInfo := httpmw.ExtractWorkspaceAgentAndLatestBuild(httpmw.ExtractWorkspaceAgentAndLatestBuildConfig{
		DB:       options.Database,
		Optional: false,
	})

	// API rate limit middleware. The counter is local and not shared between
	// replicas or instances of this middleware.
	apiRateLimiter := httpmw.RateLimit(options.APIRateLimit, time.Minute)

	// Register DERP on expvar HTTP handler, which we serve below in the router, c.f. expvar.Handler()
	// These are the metrics the DERP server exposes.
	// TODO: export via prometheus
	expDERPOnce.Do(func() {
		// We need to do this via a global Once because expvar registry is global and panics if we
		// register multiple times.  In production there is only one Coderd and one DERP server per
		// process, but in testing, we create multiple of both, so the Once protects us from
		// panicking.
		if options.DERPServer != nil {
			expvar.Publish("derp", api.DERPServer.ExpVar())
		}
	})
	cors := httpmw.Cors(options.DeploymentValues.Dangerous.AllowAllCors.Value())
	prometheusMW := httpmw.Prometheus(options.PrometheusRegistry)

	r.Use(
		httpmw.Recover(api.Logger),
		tracing.StatusWriterMiddleware,
		tracing.Middleware(api.TracerProvider),
		httpmw.AttachRequestID,
		httpmw.ExtractRealIP(api.RealIPConfig),
		loggermw.Logger(api.Logger),
		singleSlashMW,
		rolestore.CustomRoleMW,
		prometheusMW,
		// Build-Version is helpful for debugging.
		func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Add(codersdk.BuildVersionHeader, buildinfo.Version())
				next.ServeHTTP(w, r)
			})
		},
		// SubdomainAppMW checks if the first subdomain is a valid app URL. If
		// it is, it will serve that application.
		//
		// Workspace apps do their own auth and CORS and must be BEFORE the auth
		// and CORS middleware.
		api.workspaceAppServer.HandleSubdomain(apiRateLimiter),
		cors,
		// This header stops a browser from trying to MIME-sniff the content type and
		// forces it to stick with the declared content-type. This is the only valid
		// value for this header.
		// See: https://github.com/coder/security/issues/12
		func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Add("X-Content-Type-Options", "nosniff")
				next.ServeHTTP(w, r)
			})
		},
		httpmw.CSRF(options.DeploymentValues.HTTPCookies),
	)

	// This incurs a performance hit from the middleware, but is required to make sure
	// we do not override subdomain app routes.
	r.Get("/latency-check", tracing.StatusWriterMiddleware(prometheusMW(LatencyCheck())).ServeHTTP)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("OK")) })

	// Attach workspace apps routes.
	r.Group(func(r chi.Router) {
		r.Use(apiRateLimiter)
		api.workspaceAppServer.Attach(r)
	})

	if options.DERPServer != nil {
		derpHandler := derphttp.Handler(api.DERPServer)
		derpHandler, api.derpCloseFunc = tailnet.WithWebsocketSupport(api.DERPServer, derpHandler)

		r.Route("/derp", func(r chi.Router) {
			r.Get("/", derpHandler.ServeHTTP)
			// This is used when UDP is blocked, and latency must be checked via HTTP(s).
			r.Get("/latency-check", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
		})
	}

	// Register callback handlers for each OAuth2 provider.
	// We must support gitauth and externalauth for backwards compatibility.
	for _, route := range []string{"gitauth", "external-auth"} {
		r.Route("/"+route, func(r chi.Router) {
			for _, externalAuthConfig := range options.ExternalAuthConfigs {
				// We don't need to register a callback handler for device auth.
				if externalAuthConfig.DeviceAuth != nil {
					continue
				}
				r.Route(fmt.Sprintf("/%s/callback", externalAuthConfig.ID), func(r chi.Router) {
					r.Use(
						apiKeyMiddlewareRedirect,
						httpmw.ExtractOAuth2(externalAuthConfig, options.HTTPClient, options.DeploymentValues.HTTPCookies, nil),
					)
					r.Get("/", api.externalAuthCallback(externalAuthConfig))
				})
			}
		})
	}

	// OAuth2 metadata endpoint for RFC 8414 discovery
	r.Get("/.well-known/oauth-authorization-server", api.oauth2AuthorizationServerMetadata())
	// OAuth2 protected resource metadata endpoint for RFC 9728 discovery
	r.Get("/.well-known/oauth-protected-resource", api.oauth2ProtectedResourceMetadata())

	// OAuth2 linking routes do not make sense under the /api/v2 path.  These are
	// for an external application to use Coder as an OAuth2 provider, not for
	// logging into Coder with an external OAuth2 provider.
	r.Route("/oauth2", func(r chi.Router) {
		r.Use(
			httpmw.RequireExperimentWithDevBypass(api.Experiments, codersdk.ExperimentOAuth2),
		)
		r.Route("/authorize", func(r chi.Router) {
			r.Use(
				// Fetch the app as system for the authorize endpoint
				httpmw.AsAuthzSystem(httpmw.ExtractOAuth2ProviderAppWithOAuth2Errors(options.Database)),
				apiKeyMiddlewareRedirect,
			)
			// GET shows the consent page, POST processes the consent
			r.Get("/", api.getOAuth2ProviderAppAuthorize())
			r.Post("/", api.postOAuth2ProviderAppAuthorize())
		})
		r.Route("/tokens", func(r chi.Router) {
			r.Use(
				// Use OAuth2-compliant error responses for the tokens endpoint
				httpmw.AsAuthzSystem(httpmw.ExtractOAuth2ProviderAppWithOAuth2Errors(options.Database)),
			)
			r.Group(func(r chi.Router) {
				r.Use(apiKeyMiddleware)
				// DELETE on /tokens is not part of the OAuth2 spec.  It is our own
				// route used to revoke permissions from an application.  It is here for
				// parity with POST on /tokens.
				r.Delete("/", api.deleteOAuth2ProviderAppTokens())
			})
			// The POST /tokens endpoint will be called from an unauthorized client so
			// we cannot require an API key.
			r.Post("/", api.postOAuth2ProviderAppToken())
		})

		// RFC 7591 Dynamic Client Registration - Public endpoint
		r.Post("/register", api.postOAuth2ClientRegistration())

		// RFC 7592 Client Configuration Management - Protected by registration access token
		r.Route("/clients/{client_id}", func(r chi.Router) {
			r.Use(
				// Middleware to validate registration access token
				oauth2provider.RequireRegistrationAccessToken(api.Database),
			)
			r.Get("/", api.oauth2ClientConfiguration())          // Read client configuration
			r.Put("/", api.putOAuth2ClientConfiguration())       // Update client configuration
			r.Delete("/", api.deleteOAuth2ClientConfiguration()) // Delete client
		})
	})

	// Experimental routes are not guaranteed to be stable and may change at any time.
	r.Route("/api/experimental", func(r chi.Router) {
		r.Use(apiKeyMiddleware)
		r.Route("/aitasks", func(r chi.Router) {
			r.Get("/prompts", api.aiTasksPrompts)
		})
		r.Route("/mcp", func(r chi.Router) {
			r.Use(
				httpmw.RequireExperimentWithDevBypass(api.Experiments, codersdk.ExperimentOAuth2, codersdk.ExperimentMCPServerHTTP),
			)
			// MCP HTTP transport endpoint with mandatory authentication
			r.Mount("/http", api.mcpHTTPHandler())
		})
	})

	r.Route("/api/v2", func(r chi.Router) {
		api.APIHandler = r

		r.NotFound(func(rw http.ResponseWriter, _ *http.Request) { httpapi.RouteNotFound(rw) })
		r.Use(
			// Specific routes can specify different limits, but every rate
			// limit must be configurable by the admin.
			apiRateLimiter,
			httpmw.ReportCLITelemetry(api.Logger, options.Telemetry),
		)
		r.Get("/", apiRoot)
		// All CSP errors will be logged
		r.Post("/csp/reports", api.logReportCSPViolations)

		r.Get("/buildinfo", buildInfoHandler(buildInfo))
		// /regions is overridden in the enterprise version
		r.Group(func(r chi.Router) {
			r.Use(apiKeyMiddleware)
			r.Get("/regions", api.regions)
		})
		r.Route("/derp-map", func(r chi.Router) {
			// r.Use(apiKeyMiddleware)
			r.Get("/", api.derpMapUpdates)
		})
		r.Route("/deployment", func(r chi.Router) {
			r.Use(apiKeyMiddleware)
			r.Get("/config", api.deploymentValues)
			r.Get("/stats", api.deploymentStats)
			r.Get("/ssh", api.sshConfig)
		})
		r.Route("/experiments", func(r chi.Router) {
			r.Use(apiKeyMiddleware)
			r.Get("/available", handleExperimentsAvailable)
			r.Get("/", api.handleExperimentsGet)
		})
		r.Get("/updatecheck", api.updateCheck)
		r.Route("/audit", func(r chi.Router) {
			r.Use(
				apiKeyMiddleware,
				// This middleware only checks the site and orgs for the audit_log read
				// permission.
				// In the future if it makes sense to have this permission on the user as
				// well we will need to update this middleware to include that check.
				func(next http.Handler) http.Handler {
					return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
						if api.Authorize(r, policy.ActionRead, rbac.ResourceAuditLog) {
							next.ServeHTTP(rw, r)
							return
						}

						if api.Authorize(r, policy.ActionRead, rbac.ResourceAuditLog.AnyOrganization()) {
							next.ServeHTTP(rw, r)
							return
						}

						httpapi.Forbidden(rw)
					})
				},
			)

			r.Get("/", api.auditLogs)
			r.Post("/testgenerate", api.generateFakeAuditLog)
		})
		r.Route("/files", func(r chi.Router) {
			r.Use(
				apiKeyMiddleware,
				httpmw.RateLimit(options.FilesRateLimit, time.Minute),
			)
			r.Get("/{fileID}", api.fileByID)
			r.Post("/", api.postFile)
		})
		r.Route("/external-auth", func(r chi.Router) {
			r.Use(
				apiKeyMiddleware,
			)
			// Get without a specific external auth ID will return all external auths.
			r.Get("/", api.listUserExternalAuths)
			r.Route("/{externalauth}", func(r chi.Router) {
				r.Use(
					httpmw.ExtractExternalAuthParam(options.ExternalAuthConfigs),
				)
				r.Delete("/", api.deleteExternalAuthByID)
				r.Get("/", api.externalAuthByID)
				r.Post("/device", api.postExternalAuthDeviceByID)
				r.Get("/device", api.externalAuthDeviceByID)
			})
		})
		r.Route("/organizations", func(r chi.Router) {
			r.Use(
				apiKeyMiddleware,
			)
			r.Get("/", api.organizations)
			r.Route("/{organization}", func(r chi.Router) {
				r.Use(
					httpmw.ExtractOrganizationParam(options.Database),
				)
				r.Get("/", api.organization)
				r.Post("/templateversions", api.postTemplateVersionsByOrganization)
				r.Route("/templates", func(r chi.Router) {
					r.Post("/", api.postTemplateByOrganization)
					r.Get("/", api.templatesByOrganization())
					r.Get("/examples", api.templateExamplesByOrganization)
					r.Route("/{templatename}", func(r chi.Router) {
						r.Get("/", api.templateByOrganizationAndName)
						r.Route("/versions/{templateversionname}", func(r chi.Router) {
							r.Get("/", api.templateVersionByOrganizationTemplateAndName)
							r.Get("/previous", api.previousTemplateVersionByOrganizationTemplateAndName)
						})
					})
				})
				r.Get("/paginated-members", api.paginatedMembers)
				r.Route("/members", func(r chi.Router) {
					r.Get("/", api.listMembers)
					r.Route("/roles", func(r chi.Router) {
						r.Get("/", api.assignableOrgRoles)
					})

					r.Route("/{user}", func(r chi.Router) {
						r.Group(func(r chi.Router) {
							r.Use(
								// Adding a member requires "read" permission
								// on the site user. So limited to owners and user-admins.
								// TODO: Allow org-admins to add users via some new permission? Or give them
								// 	read on site users.
								httpmw.ExtractUserParam(options.Database),
							)
							r.Post("/", api.postOrganizationMember)
						})

						r.Group(func(r chi.Router) {
							r.Use(
								httpmw.ExtractOrganizationMemberParam(options.Database),
							)
							r.Delete("/", api.deleteOrganizationMember)
							r.Put("/roles", api.putMemberRoles)
							r.Post("/workspaces", api.postWorkspacesByOrganization)
						})
					})
				})
				r.Route("/provisionerdaemons", func(r chi.Router) {
					r.Get("/", api.provisionerDaemons)
				})
				r.Route("/provisionerjobs", func(r chi.Router) {
					r.Get("/{job}", api.provisionerJob)
					r.Get("/", api.provisionerJobs)
				})
			})
		})
		r.Route("/templates", func(r chi.Router) {
			r.Use(
				apiKeyMiddleware,
			)
			r.Get("/", api.fetchTemplates(nil))
			r.Get("/examples", api.templateExamples)
			r.Route("/{template}", func(r chi.Router) {
				r.Use(
					httpmw.ExtractTemplateParam(options.Database),
				)
				r.Get("/daus", api.templateDAUs)
				r.Get("/", api.template)
				r.Delete("/", api.deleteTemplate)
				r.Patch("/", api.patchTemplateMeta)
				r.Route("/versions", func(r chi.Router) {
					r.Post("/archive", api.postArchiveTemplateVersions)
					r.Get("/", api.templateVersionsByTemplate)
					r.Patch("/", api.patchActiveTemplateVersion)
					r.Get("/{templateversionname}", api.templateVersionByName)
				})
			})
		})

		r.Route("/templateversions/{templateversion}", func(r chi.Router) {
			r.Use(
				apiKeyMiddleware,
				httpmw.ExtractTemplateVersionParam(options.Database),
			)
			r.Get("/", api.templateVersion)
			r.Patch("/", api.patchTemplateVersion)
			r.Patch("/cancel", api.patchCancelTemplateVersion)
			r.Post("/archive", api.postArchiveTemplateVersion())
			r.Post("/unarchive", api.postUnarchiveTemplateVersion())
			// Old agents may expect a non-error response from /schema and /parameters endpoints.
			// The idea is to return an empty [], so that the coder CLI won't get blocked accidentally.
			r.Get("/schema", templateVersionSchemaDeprecated)
			r.Get("/parameters", templateVersionParametersDeprecated)
			r.Get("/rich-parameters", api.templateVersionRichParameters)
			r.Get("/external-auth", api.templateVersionExternalAuth)
			r.Get("/variables", api.templateVersionVariables)
			r.Get("/presets", api.templateVersionPresets)
			r.Get("/resources", api.templateVersionResources)
			r.Get("/logs", api.templateVersionLogs)
			r.Route("/dry-run", func(r chi.Router) {
				r.Post("/", api.postTemplateVersionDryRun)
				r.Get("/{jobID}", api.templateVersionDryRun)
				r.Get("/{jobID}/resources", api.templateVersionDryRunResources)
				r.Get("/{jobID}/logs", api.templateVersionDryRunLogs)
				r.Get("/{jobID}/matched-provisioners", api.templateVersionDryRunMatchedProvisioners)
				r.Patch("/{jobID}/cancel", api.patchTemplateVersionDryRunCancel)
			})

			r.Group(func(r chi.Router) {
				r.Route("/dynamic-parameters", func(r chi.Router) {
					r.Post("/evaluate", api.templateVersionDynamicParametersEvaluate)
					r.Get("/", api.templateVersionDynamicParametersWebsocket)
				})
			})
		})
		r.Route("/users", func(r chi.Router) {
			r.Get("/first", api.firstUser)
			r.Post("/first", api.postFirstUser)
			r.Get("/authmethods", api.userAuthMethods)

			r.Group(func(r chi.Router) {
				// We use a tight limit for password login to protect against
				// audit-log write DoS, pbkdf2 DoS, and simple brute-force
				// attacks.
				//
				// This value is intentionally increased during tests.
				r.Use(httpmw.RateLimit(options.LoginRateLimit, time.Minute))
				r.Post("/login", api.postLogin)
				r.Post("/otp/request", api.postRequestOneTimePasscode)
				r.Post("/validate-password", api.validateUserPassword)
				r.Post("/otp/change-password", api.postChangePasswordWithOneTimePasscode)
				r.Route("/oauth2", func(r chi.Router) {
					r.Get("/github/device", api.userOAuth2GithubDevice)
					r.Route("/github", func(r chi.Router) {
						r.Use(
							httpmw.ExtractOAuth2(options.GithubOAuth2Config, options.HTTPClient, options.DeploymentValues.HTTPCookies, nil),
						)
						r.Get("/callback", api.userOAuth2Github)
					})
				})
				r.Route("/oidc/callback", func(r chi.Router) {
					r.Use(
						httpmw.ExtractOAuth2(options.OIDCConfig, options.HTTPClient, options.DeploymentValues.HTTPCookies, oidcAuthURLParams),
					)
					r.Get("/", api.userOIDC)
				})
			})
			r.Group(func(r chi.Router) {
				r.Use(
					apiKeyMiddleware,
				)
				r.Post("/", api.postUser)
				r.Get("/", api.users)
				r.Post("/logout", api.postLogout)
				// These routes query information about site wide roles.
				r.Route("/roles", func(r chi.Router) {
					r.Get("/", api.AssignableSiteRoles)
				})
				r.Route("/{user}", func(r chi.Router) {
					r.Group(func(r chi.Router) {
						r.Use(httpmw.ExtractOrganizationMembersParam(options.Database, api.HTTPAuth.Authorize))
						// Creating workspaces does not require permissions on the user, only the
						// organization member. This endpoint should match the authz story of
						// postWorkspacesByOrganization
						r.Post("/workspaces", api.postUserWorkspaces)
						r.Route("/workspace/{workspacename}", func(r chi.Router) {
							r.Get("/", api.workspaceByOwnerAndName)
							r.Get("/builds/{buildnumber}", api.workspaceBuildByBuildNumber)
						})
					})

					r.Group(func(r chi.Router) {
						r.Use(httpmw.ExtractUserParam(options.Database))

						r.Post("/convert-login", api.postConvertLoginType)
						r.Delete("/", api.deleteUser)
						r.Get("/", api.userByName)
						r.Get("/autofill-parameters", api.userAutofillParameters)
						r.Get("/login-type", api.userLoginType)
						r.Put("/profile", api.putUserProfile)
						r.Route("/status", func(r chi.Router) {
							r.Put("/suspend", api.putSuspendUserAccount())
							r.Put("/activate", api.putActivateUserAccount())
						})
						r.Get("/appearance", api.userAppearanceSettings)
						r.Put("/appearance", api.putUserAppearanceSettings)
						r.Route("/password", func(r chi.Router) {
							r.Use(httpmw.RateLimit(options.LoginRateLimit, time.Minute))
							r.Put("/", api.putUserPassword)
						})
						// These roles apply to the site wide permissions.
						r.Put("/roles", api.putUserRoles)
						r.Get("/roles", api.userRoles)

						r.Route("/keys", func(r chi.Router) {
							r.Post("/", api.postAPIKey)
							r.Route("/tokens", func(r chi.Router) {
								r.Post("/", api.postToken)
								r.Get("/", api.tokens)
								r.Get("/tokenconfig", api.tokenConfig)
								r.Route("/{keyname}", func(r chi.Router) {
									r.Get("/", api.apiKeyByName)
								})
							})
							r.Route("/{keyid}", func(r chi.Router) {
								r.Get("/", api.apiKeyByID)
								r.Delete("/", api.deleteAPIKey)
							})
						})

						r.Route("/organizations", func(r chi.Router) {
							r.Get("/", api.organizationsByUser)
							r.Get("/{organizationname}", api.organizationByUserAndName)
						})

						r.Get("/gitsshkey", api.gitSSHKey)
						r.Put("/gitsshkey", api.regenerateGitSSHKey)
						r.Route("/notifications", func(r chi.Router) {
							r.Route("/preferences", func(r chi.Router) {
								r.Get("/", api.userNotificationPreferences)
								r.Put("/", api.putUserNotificationPreferences)
							})
						})
						r.Route("/webpush", func(r chi.Router) {
							r.Post("/subscription", api.postUserWebpushSubscription)
							r.Delete("/subscription", api.deleteUserWebpushSubscription)
							r.Post("/test", api.postUserPushNotificationTest)
						})
					})
				})
			})
		})
		r.Route("/workspaceagents", func(r chi.Router) {
			r.Post("/azure-instance-identity", api.postWorkspaceAuthAzureInstanceIdentity)
			r.Post("/aws-instance-identity", api.postWorkspaceAuthAWSInstanceIdentity)
			r.Post("/google-instance-identity", api.postWorkspaceAuthGoogleInstanceIdentity)
			r.With(
				apiKeyMiddlewareOptional,
				httpmw.ExtractWorkspaceProxy(httpmw.ExtractWorkspaceProxyConfig{
					DB:       options.Database,
					Optional: true,
				}),
				httpmw.RequireAPIKeyOrWorkspaceProxyAuth(),
			).Get("/connection", api.workspaceAgentConnectionGeneric)
			r.Route("/me", func(r chi.Router) {
				r.Use(workspaceAgentInfo)
				r.Get("/rpc", api.workspaceAgentRPC)
				r.Patch("/logs", api.patchWorkspaceAgentLogs)
				r.Patch("/app-status", api.patchWorkspaceAgentAppStatus)
				// Deprecated: Required to support legacy agents
				r.Get("/gitauth", api.workspaceAgentsGitAuth)
				r.Get("/external-auth", api.workspaceAgentsExternalAuth)
				r.Get("/gitsshkey", api.agentGitSSHKey)
				r.Post("/log-source", api.workspaceAgentPostLogSource)
				r.Get("/reinit", api.workspaceAgentReinit)
			})
			r.Route("/{workspaceagent}", func(r chi.Router) {
				r.Use(
					// Allow either API key or external workspace proxy auth and require it.
					apiKeyMiddlewareOptional,
					httpmw.ExtractWorkspaceProxy(httpmw.ExtractWorkspaceProxyConfig{
						DB:       options.Database,
						Optional: true,
					}),
					httpmw.RequireAPIKeyOrWorkspaceProxyAuth(),

					httpmw.ExtractWorkspaceAgentParam(options.Database),
					httpmw.ExtractWorkspaceParam(options.Database),
				)
				r.Get("/", api.workspaceAgent)
				r.Get("/watch-metadata", api.watchWorkspaceAgentMetadataSSE)
				r.Get("/watch-metadata-ws", api.watchWorkspaceAgentMetadataWS)
				r.Get("/startup-logs", api.workspaceAgentLogsDeprecated)
				r.Get("/logs", api.workspaceAgentLogs)
				r.Get("/listening-ports", api.workspaceAgentListeningPorts)
				r.Get("/connection", api.workspaceAgentConnection)
				r.Get("/containers", api.workspaceAgentListContainers)
				r.Get("/containers/watch", api.watchWorkspaceAgentContainers)
				r.Post("/containers/devcontainers/{devcontainer}/recreate", api.workspaceAgentRecreateDevcontainer)
				r.Get("/coordinate", api.workspaceAgentClientCoordinate)

				// PTY is part of workspaceAppServer.
			})
		})
		r.Route("/workspaces", func(r chi.Router) {
			r.Use(
				apiKeyMiddleware,
			)
			r.Get("/", api.workspaces)
			r.Route("/{workspace}", func(r chi.Router) {
				r.Use(
					httpmw.ExtractWorkspaceParam(options.Database),
				)
				r.Get("/", api.workspace)
				r.Patch("/", api.patchWorkspace)
				r.Route("/builds", func(r chi.Router) {
					r.Get("/", api.workspaceBuilds)
					r.Post("/", api.postWorkspaceBuilds)
				})
				r.Route("/autostart", func(r chi.Router) {
					r.Put("/", api.putWorkspaceAutostart)
				})
				r.Route("/ttl", func(r chi.Router) {
					r.Put("/", api.putWorkspaceTTL)
				})
				r.Get("/watch", api.watchWorkspaceSSE)
				r.Get("/watch-ws", api.watchWorkspaceWS)
				r.Put("/extend", api.putExtendWorkspace)
				r.Post("/usage", api.postWorkspaceUsage)
				r.Put("/dormant", api.putWorkspaceDormant)
				r.Put("/favorite", api.putFavoriteWorkspace)
				r.Delete("/favorite", api.deleteFavoriteWorkspace)
				r.Put("/autoupdates", api.putWorkspaceAutoupdates)
				r.Get("/resolve-autostart", api.resolveAutostart)
				r.Route("/port-share", func(r chi.Router) {
					r.Get("/", api.workspaceAgentPortShares)
					r.Post("/", api.postWorkspaceAgentPortShare)
					r.Delete("/", api.deleteWorkspaceAgentPortShare)
				})
				r.Get("/timings", api.workspaceTimings)
				r.Route("/acl", func(r chi.Router) {
					r.Use(
						httpmw.RequireExperiment(api.Experiments, codersdk.ExperimentWorkspaceSharing))

					r.Patch("/", api.patchWorkspaceACL)
				})
			})
		})
		r.Route("/workspacebuilds/{workspacebuild}", func(r chi.Router) {
			r.Use(
				apiKeyMiddleware,
				httpmw.ExtractWorkspaceBuildParam(options.Database),
				httpmw.ExtractWorkspaceParam(options.Database),
			)
			r.Get("/", api.workspaceBuild)
			r.Patch("/cancel", api.patchCancelWorkspaceBuild)
			r.Get("/logs", api.workspaceBuildLogs)
			r.Get("/parameters", api.workspaceBuildParameters)
			r.Get("/resources", api.workspaceBuildResourcesDeprecated)
			r.Get("/state", api.workspaceBuildState)
			r.Get("/timings", api.workspaceBuildTimings)
		})
		r.Route("/authcheck", func(r chi.Router) {
			r.Use(apiKeyMiddleware)
			r.Post("/", api.checkAuthorization)
		})
		r.Route("/applications", func(r chi.Router) {
			r.Route("/host", func(r chi.Router) {
				// Don't leak the hostname to unauthenticated users.
				r.Use(apiKeyMiddleware)
				r.Get("/", api.appHost)
			})
			r.Route("/auth-redirect", func(r chi.Router) {
				// We want to redirect to login if they are not authenticated.
				r.Use(apiKeyMiddlewareRedirect)

				// This is a GET request as it's redirected to by the subdomain app
				// handler and the login page.
				r.Get("/", api.workspaceApplicationAuth)
			})
		})
		r.Route("/insights", func(r chi.Router) {
			r.Use(apiKeyMiddleware)
			r.Get("/daus", api.deploymentDAUs)
			r.Get("/user-activity", api.insightsUserActivity)
			r.Get("/user-status-counts", api.insightsUserStatusCounts)
			r.Get("/user-latency", api.insightsUserLatency)
			r.Get("/templates", api.insightsTemplates)
		})
		r.Route("/debug", func(r chi.Router) {
			r.Use(
				apiKeyMiddleware,
				// Ensure only owners can access debug endpoints.
				func(next http.Handler) http.Handler {
					return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
						if !api.Authorize(r, policy.ActionRead, rbac.ResourceDebugInfo) {
							httpapi.Forbidden(rw)
							return
						}

						next.ServeHTTP(rw, r)
					})
				},
			)

			r.Get("/coordinator", api.debugCoordinator)
			r.Get("/tailnet", api.debugTailnet)
			r.Route("/health", func(r chi.Router) {
				r.Get("/", api.debugDeploymentHealth)
				r.Route("/settings", func(r chi.Router) {
					r.Get("/", api.deploymentHealthSettings)
					r.Put("/", api.putDeploymentHealthSettings)
				})
			})
			r.Get("/ws", (&healthcheck.WebsocketEchoServer{}).ServeHTTP)
			r.Route("/{user}", func(r chi.Router) {
				r.Use(httpmw.ExtractUserParam(options.Database))
				r.Get("/debug-link", api.userDebugOIDC)
			})
			if options.DERPServer != nil {
				r.Route("/derp", func(r chi.Router) {
					r.Get("/traffic", options.DERPServer.ServeDebugTraffic)
				})
			}
			r.Method("GET", "/expvar", expvar.Handler()) // contains DERP metrics as well as cmdline and memstats
		})
		// Manage OAuth2 applications that can use Coder as an OAuth2 provider.
		r.Route("/oauth2-provider", func(r chi.Router) {
			r.Use(
				apiKeyMiddleware,
				httpmw.RequireExperimentWithDevBypass(api.Experiments, codersdk.ExperimentOAuth2),
			)
			r.Route("/apps", func(r chi.Router) {
				r.Get("/", api.oAuth2ProviderApps())
				r.Post("/", api.postOAuth2ProviderApp())

				r.Route("/{app}", func(r chi.Router) {
					r.Use(httpmw.ExtractOAuth2ProviderApp(options.Database))
					r.Get("/", api.oAuth2ProviderApp())
					r.Put("/", api.putOAuth2ProviderApp())
					r.Delete("/", api.deleteOAuth2ProviderApp())

					r.Route("/secrets", func(r chi.Router) {
						r.Get("/", api.oAuth2ProviderAppSecrets())
						r.Post("/", api.postOAuth2ProviderAppSecret())

						r.Route("/{secretID}", func(r chi.Router) {
							r.Use(httpmw.ExtractOAuth2ProviderAppSecret(options.Database))
							r.Delete("/", api.deleteOAuth2ProviderAppSecret())
						})
					})
				})
			})
		})
		r.Route("/notifications", func(r chi.Router) {
			r.Use(apiKeyMiddleware)
			r.Route("/inbox", func(r chi.Router) {
				r.Get("/", api.listInboxNotifications)
				r.Put("/mark-all-as-read", api.markAllInboxNotificationsAsRead)
				r.Get("/watch", api.watchInboxNotifications)
				r.Put("/{id}/read-status", api.updateInboxNotificationReadStatus)
			})
			r.Get("/settings", api.notificationsSettings)
			r.Put("/settings", api.putNotificationsSettings)
			r.Route("/templates", func(r chi.Router) {
				r.Get("/system", api.systemNotificationTemplates)
			})
			r.Get("/dispatch-methods", api.notificationDispatchMethods)
			r.Post("/test", api.postTestNotification)
		})
		r.Route("/tailnet", func(r chi.Router) {
			r.Use(apiKeyMiddleware)
			r.Get("/", api.tailnetRPCConn)
		})
	})

	if options.SwaggerEndpoint {
		// Swagger UI requires the URL trailing slash. Otherwise, the browser tries to load /assets
		// from http://localhost:8080/assets instead of http://localhost:8080/swagger/assets.
		r.Get("/swagger", http.RedirectHandler("/swagger/", http.StatusTemporaryRedirect).ServeHTTP)
		// See globalHTTPSwaggerHandler comment as to why we use a package
		// global variable here.
		r.Get("/swagger/*", globalHTTPSwaggerHandler)
	} else {
		swaggerDisabled := http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
			httpapi.Write(context.Background(), rw, http.StatusNotFound, codersdk.Response{
				Message: "Swagger documentation is disabled.",
			})
		})
		r.Get("/swagger", swaggerDisabled)
		r.Get("/swagger/*", swaggerDisabled)
	}

	additionalCSPHeaders := make(map[httpmw.CSPFetchDirective][]string, len(api.DeploymentValues.AdditionalCSPPolicy))
	var cspParseErrors error
	for _, v := range api.DeploymentValues.AdditionalCSPPolicy {
		// Format is "<directive> <value> <value> ..."
		v = strings.TrimSpace(v)
		parts := strings.Split(v, " ")
		if len(parts) < 2 {
			cspParseErrors = errors.Join(cspParseErrors, xerrors.Errorf("invalid CSP header %q, not enough parts to be valid", v))
			continue
		}
		additionalCSPHeaders[httpmw.CSPFetchDirective(strings.ToLower(parts[0]))] = parts[1:]
	}

	if cspParseErrors != nil {
		// Do not fail Coder deployment startup because of this. Just log an error
		// and continue
		api.Logger.Error(context.Background(),
			"parsing additional CSP headers", slog.Error(cspParseErrors))
	}

	// Add CSP headers to all static assets and pages. CSP headers only affect
	// browsers, so these don't make sense on api routes.
	cspMW := httpmw.CSPHeaders(
		options.Telemetry.Enabled(), func() []*proxyhealth.ProxyHost {
			if api.DeploymentValues.Dangerous.AllowAllCors {
				// In this mode, allow all external requests.
				return []*proxyhealth.ProxyHost{
					{
						Host:    "*",
						AppHost: "*",
					},
				}
			}
			// Always add the primary, since the app host may be on a sub-domain.
			proxies := []*proxyhealth.ProxyHost{
				{
					Host:    api.AccessURL.Host,
					AppHost: appurl.ConvertAppHostForCSP(api.AccessURL.Host, api.AppHostname),
				},
			}
			if f := api.WorkspaceProxyHostsFn.Load(); f != nil {
				proxies = append(proxies, (*f)()...)
			}
			return proxies
		}, additionalCSPHeaders)

	// Static file handler must be wrapped with HSTS handler if the
	// StrictTransportSecurityAge is set. We only need to set this header on
	// static files since it only affects browsers.
	r.NotFound(cspMW(compressHandler(httpmw.HSTS(api.SiteHandler, options.StrictTransportSecurityCfg))).ServeHTTP)

	api.RootHandler = r

	return api
}

type API struct {
	// ctx is canceled immediately on shutdown, it can be used to abort
	// interruptible tasks.
	ctx    context.Context
	cancel context.CancelFunc

	// DeploymentID is loaded from the database on startup.
	DeploymentID string

	*Options
	// ID is a uniquely generated ID on initialization.
	// This is used to associate objects with a specific
	// Coder API instance, like workspace agents to a
	// specific replica.
	ID                                uuid.UUID
	Auditor                           atomic.Pointer[audit.Auditor]
	ConnectionLogger                  atomic.Pointer[connectionlog.ConnectionLogger]
	WorkspaceClientCoordinateOverride atomic.Pointer[func(rw http.ResponseWriter) bool]
	TailnetCoordinator                atomic.Pointer[tailnet.Coordinator]
	NetworkTelemetryBatcher           *tailnet.NetworkTelemetryBatcher
	TailnetClientService              *tailnet.ClientService
	// WebpushDispatcher is a way to send notifications to users via Web Push.
	WebpushDispatcher webpush.Dispatcher
	QuotaCommitter    atomic.Pointer[proto.QuotaCommitter]
	AppearanceFetcher atomic.Pointer[appearance.Fetcher]
	// WorkspaceProxyHostsFn returns the hosts of healthy workspace proxies
	// for header reasons.
	WorkspaceProxyHostsFn atomic.Pointer[func() []*proxyhealth.ProxyHost]
	// TemplateScheduleStore is a pointer to an atomic pointer because this is
	// passed to another struct, and we want them all to be the same reference.
	TemplateScheduleStore *atomic.Pointer[schedule.TemplateScheduleStore]
	// UserQuietHoursScheduleStore is a pointer to an atomic pointer for the
	// same reason as TemplateScheduleStore.
	UserQuietHoursScheduleStore *atomic.Pointer[schedule.UserQuietHoursScheduleStore]
	// DERPMapper mutates the DERPMap to include workspace proxies.
	DERPMapper atomic.Pointer[func(derpMap *tailcfg.DERPMap) *tailcfg.DERPMap]
	// AccessControlStore is a pointer to an atomic pointer since it is
	// passed to dbauthz.
	AccessControlStore  *atomic.Pointer[dbauthz.AccessControlStore]
	PortSharer          atomic.Pointer[portsharing.PortSharer]
	FileCache           *files.Cache
	PrebuildsClaimer    atomic.Pointer[prebuilds.Claimer]
	PrebuildsReconciler atomic.Pointer[prebuilds.ReconciliationOrchestrator]
	// BuildUsageChecker is a pointer as it's passed around to multiple
	// components.
	BuildUsageChecker *atomic.Pointer[wsbuilder.UsageChecker]

	UpdatesProvider tailnet.WorkspaceUpdatesProvider

	HTTPAuth *HTTPAuthorizer

	// APIHandler serves "/api/v2"
	APIHandler chi.Router
	// RootHandler serves "/"
	RootHandler chi.Router

	// SiteHandler serves static files for the dashboard.
	SiteHandler *site.Handler

	WebsocketWaitMutex sync.Mutex
	WebsocketWaitGroup sync.WaitGroup
	derpCloseFunc      func()

	metricsCache          *metricscache.Cache
	updateChecker         *updatecheck.Checker
	WorkspaceAppsProvider workspaceapps.SignedTokenProvider
	workspaceAppServer    *workspaceapps.Server
	agentProvider         workspaceapps.AgentProvider

	// Experiments contains the list of experiments currently enabled.
	// This is used to gate features that are not yet ready for production.
	Experiments codersdk.Experiments

	healthCheckGroup *singleflight.Group[string, *healthsdk.HealthcheckReport]
	healthCheckCache atomic.Pointer[healthsdk.HealthcheckReport]

	statsReporter *workspacestats.Reporter

	Acquirer *provisionerdserver.Acquirer
	// dbRolluper rolls up template usage stats from raw agent and app
	// stats. This is used to provide insights in the WebUI.
	dbRolluper *dbrollup.Rolluper
}

// Close waits for all WebSocket connections to drain before returning.
func (api *API) Close() error {
	select {
	case <-api.ctx.Done():
		return xerrors.New("API already closed")
	default:
		api.cancel()
	}

	wsDone := make(chan struct{})
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	go func() {
		api.WebsocketWaitMutex.Lock()
		defer api.WebsocketWaitMutex.Unlock()
		api.WebsocketWaitGroup.Wait()
		close(wsDone)
	}()
	// This will technically leak the above func if the timer fires, but this is
	// maintly a last ditch effort to un-stuck coderd on shutdown. This
	// shouldn't affect tests at all.
	select {
	case <-wsDone:
	case <-timer.C:
		api.Logger.Warn(api.ctx, "websocket shutdown timed out after 10 seconds")
	}

	api.dbRolluper.Close()
	api.metricsCache.Close()
	if api.updateChecker != nil {
		api.updateChecker.Close()
	}
	_ = api.workspaceAppServer.Close()
	_ = api.agentProvider.Close()
	if api.derpCloseFunc != nil {
		api.derpCloseFunc()
	}
	// The coordinator should be closed after the agent provider, and the DERP
	// handler.
	coordinator := api.TailnetCoordinator.Load()
	if coordinator != nil {
		_ = (*coordinator).Close()
	}
	_ = api.statsReporter.Close()
	_ = api.NetworkTelemetryBatcher.Close()
	_ = api.OIDCConvertKeyCache.Close()
	_ = api.AppSigningKeyCache.Close()
	_ = api.AppEncryptionKeyCache.Close()
	_ = api.UpdatesProvider.Close()

	if current := api.PrebuildsReconciler.Load(); current != nil {
		ctx, giveUp := context.WithTimeoutCause(context.Background(), time.Second*30, xerrors.New("gave up waiting for reconciler to stop before shutdown"))
		defer giveUp()
		(*current).Stop(ctx, nil)
	}

	return nil
}

func compressHandler(h http.Handler) http.Handler {
	level := 5
	if flag.Lookup("test.v") != nil {
		level = 1
	}

	cmp := middleware.NewCompressor(level,
		"text/*",
		"application/*",
		"image/*",
	)
	cmp.SetEncoder("br", func(w io.Writer, level int) io.Writer {
		return brotli.NewWriterLevel(w, level)
	})
	cmp.SetEncoder("zstd", func(w io.Writer, level int) io.Writer {
		zw, err := zstd.NewWriter(w, zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(level)))
		if err != nil {
			panic("invalid zstd compressor: " + err.Error())
		}
		return zw
	})

	return cmp.Handler(h)
}

type MemoryProvisionerDaemonOption func(*memoryProvisionerDaemonOptions)

func MemoryProvisionerWithVersionOverride(version string) MemoryProvisionerDaemonOption {
	return func(opts *memoryProvisionerDaemonOptions) {
		opts.versionOverride = version
	}
}

type memoryProvisionerDaemonOptions struct {
	versionOverride string
}

// CreateInMemoryProvisionerDaemon is an in-memory connection to a provisionerd.
// Useful when starting coderd and provisionerd in the same process.
func (api *API) CreateInMemoryProvisionerDaemon(dialCtx context.Context, name string, provisionerTypes []codersdk.ProvisionerType) (client proto.DRPCProvisionerDaemonClient, err error) {
	return api.CreateInMemoryTaggedProvisionerDaemon(dialCtx, name, provisionerTypes, nil)
}

func (api *API) CreateInMemoryTaggedProvisionerDaemon(dialCtx context.Context, name string, provisionerTypes []codersdk.ProvisionerType, provisionerTags map[string]string, opts ...MemoryProvisionerDaemonOption) (client proto.DRPCProvisionerDaemonClient, err error) {
	options := &memoryProvisionerDaemonOptions{}
	for _, opt := range opts {
		opt(options)
	}

	tracer := api.TracerProvider.Tracer(tracing.TracerName)
	clientSession, serverSession := drpcsdk.MemTransportPipe()
	defer func() {
		if err != nil {
			_ = clientSession.Close()
			_ = serverSession.Close()
		}
	}()

	// All in memory provisioners will be part of the default org for now.
	//nolint:gocritic // in-memory provisioners are owned by system
	defaultOrg, err := api.Database.GetDefaultOrganization(dbauthz.AsSystemRestricted(dialCtx))
	if err != nil {
		return nil, xerrors.Errorf("unable to fetch default org for in memory provisioner: %w", err)
	}

	dbTypes := make([]database.ProvisionerType, 0, len(provisionerTypes))
	for _, tp := range provisionerTypes {
		dbTypes = append(dbTypes, database.ProvisionerType(tp))
	}

	keyID, err := uuid.Parse(string(codersdk.ProvisionerKeyIDBuiltIn))
	if err != nil {
		return nil, xerrors.Errorf("failed to parse built-in provisioner key ID: %w", err)
	}

	apiVersion := proto.CurrentVersion.String()
	if options.versionOverride != "" && flag.Lookup("test.v") != nil {
		// This should only be usable for unit testing. To fake a different provisioner version
		apiVersion = options.versionOverride
	}

	//nolint:gocritic // in-memory provisioners are owned by system
	daemon, err := api.Database.UpsertProvisionerDaemon(dbauthz.AsSystemRestricted(dialCtx), database.UpsertProvisionerDaemonParams{
		Name:           name,
		OrganizationID: defaultOrg.ID,
		CreatedAt:      dbtime.Now(),
		Provisioners:   dbTypes,
		Tags:           provisionersdk.MutateTags(uuid.Nil, provisionerTags),
		LastSeenAt:     sql.NullTime{Time: dbtime.Now(), Valid: true},
		Version:        buildinfo.Version(),
		APIVersion:     apiVersion,
		KeyID:          keyID,
	})
	if err != nil {
		return nil, xerrors.Errorf("failed to create in-memory provisioner daemon: %w", err)
	}

	mux := drpcmux.New()
	api.Logger.Debug(dialCtx, "starting in-memory provisioner daemon", slog.F("name", name))
	logger := api.Logger.Named(fmt.Sprintf("inmem-provisionerd-%s", name))
	srv, err := provisionerdserver.NewServer(
		api.ctx, // use the same ctx as the API
		daemon.APIVersion,
		api.AccessURL,
		daemon.ID,
		defaultOrg.ID,
		logger,
		daemon.Provisioners,
		provisionerdserver.Tags(daemon.Tags),
		api.Database,
		api.Pubsub,
		api.Acquirer,
		api.Telemetry,
		tracer,
		&api.QuotaCommitter,
		&api.Auditor,
		api.TemplateScheduleStore,
		api.UserQuietHoursScheduleStore,
		api.DeploymentValues,
		provisionerdserver.Options{
			OIDCConfig:          api.OIDCConfig,
			ExternalAuthConfigs: api.ExternalAuthConfigs,
			Clock:               api.Clock,
		},
		api.NotificationsEnqueuer,
		&api.PrebuildsReconciler,
	)
	if err != nil {
		return nil, err
	}
	err = proto.DRPCRegisterProvisionerDaemon(mux, srv)
	if err != nil {
		return nil, err
	}
	server := drpcserver.NewWithOptions(&tracing.DRPCHandler{Handler: mux},
		drpcserver.Options{
			Manager: drpcsdk.DefaultDRPCOptions(nil),
			Log: func(err error) {
				if xerrors.Is(err, io.EOF) {
					return
				}
				logger.Debug(dialCtx, "drpc server error", slog.Error(err))
			},
		},
	)
	// in-mem pipes aren't technically "websockets" but they have the same properties as far as the
	// API is concerned: they are long-lived connections that we need to close before completing
	// shutdown of the API.
	api.WebsocketWaitMutex.Lock()
	api.WebsocketWaitGroup.Add(1)
	api.WebsocketWaitMutex.Unlock()
	go func() {
		defer api.WebsocketWaitGroup.Done()
		// here we pass the background context, since we want the server to keep serving until the
		// client hangs up.  If we, say, pass the API context, then when it is canceled, we could
		// drop a job that we locked in the database but never passed to the provisionerd.  The
		// provisionerd is local, in-mem, so there isn't a danger of losing contact with it and
		// having a dead connection we don't know the status of.
		err := server.Serve(context.Background(), serverSession)
		logger.Info(dialCtx, "provisioner daemon disconnected", slog.Error(err))
		// close the sessions, so we don't leak goroutines serving them.
		_ = clientSession.Close()
		_ = serverSession.Close()
	}()

	return proto.NewDRPCProvisionerDaemonClient(clientSession), nil
}

func (api *API) DERPMap() *tailcfg.DERPMap {
	fn := api.DERPMapper.Load()
	if fn != nil {
		return (*fn)(api.Options.BaseDERPMap)
	}

	return api.Options.BaseDERPMap
}

// nolint:revive
func ReadExperiments(log slog.Logger, raw []string) codersdk.Experiments {
	exps := make([]codersdk.Experiment, 0, len(raw))
	for _, v := range raw {
		switch v {
		case "*":
			exps = append(exps, codersdk.ExperimentsSafe...)
		default:
			ex := codersdk.Experiment(strings.ToLower(v))
			if !slice.Contains(codersdk.ExperimentsKnown, ex) {
				log.Warn(context.Background(), "ignoring unknown experiment", slog.F("experiment", ex))
			} else if !slice.Contains(codersdk.ExperimentsSafe, ex) {
				log.Warn(context.Background(), "🐉 HERE BE DRAGONS: opting into hidden experiment", slog.F("experiment", ex))
			}
			exps = append(exps, ex)
		}
	}
	return exps
}

var multipleSlashesRe = regexp.MustCompile(`/+`)

func singleSlashMW(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		var path string
		rctx := chi.RouteContext(r.Context())
		if rctx != nil && rctx.RoutePath != "" {
			path = rctx.RoutePath
		} else {
			path = r.URL.Path
		}

		// Normalize multiple slashes to a single slash
		newPath := multipleSlashesRe.ReplaceAllString(path, "/")

		// Apply the cleaned path
		// The approach is consistent with: https://github.com/go-chi/chi/blob/e846b8304c769c4f1a51c9de06bebfaa4576bd88/middleware/strip.go#L24-L28
		if rctx != nil {
			rctx.RoutePath = newPath
		} else {
			r.URL.Path = newPath
		}

		next.ServeHTTP(w, r)
	}
	return http.HandlerFunc(fn)
}

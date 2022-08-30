package coderd

import (
	"crypto/x509"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"sync"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/klauspost/compress/zstd"
	"github.com/pion/webrtc/v3"
	"github.com/prometheus/client_golang/prometheus"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"golang.org/x/xerrors"
	"google.golang.org/api/idtoken"

	"cdr.dev/slog"
	"github.com/coder/coder/buildinfo"
	"github.com/coder/coder/coderd/awsidentity"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/gitsshkey"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/coderd/telemetry"
	"github.com/coder/coder/coderd/tracing"
	"github.com/coder/coder/coderd/turnconn"
	"github.com/coder/coder/coderd/wsconncache"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/site"
)

// Options are requires parameters for Coder to start.
type Options struct {
	AccessURL *url.URL
	Logger    slog.Logger
	Database  database.Store
	Pubsub    database.Pubsub

	// CacheDir is used for caching files served by the API.
	CacheDir string

	AgentConnectionUpdateFrequency time.Duration
	AgentInactiveDisconnectTimeout time.Duration
	// APIRateLimit is the minutely throughput rate limit per user or ip.
	// Setting a rate limit <0 will disable the rate limiter across the entire
	// app. Specific routes may have their own limiters.
	APIRateLimit         int
	AWSCertificates      awsidentity.Certificates
	Authorizer           rbac.Authorizer
	AzureCertificates    x509.VerifyOptions
	GoogleTokenValidator *idtoken.Validator
	GithubOAuth2Config   *GithubOAuth2Config
	OIDCConfig           *OIDCConfig
	PrometheusRegistry   *prometheus.Registry
	ICEServers           []webrtc.ICEServer
	SecureAuthCookie     bool
	SSHKeygenAlgorithm   gitsshkey.Algorithm
	Telemetry            telemetry.Reporter
	TURNServer           *turnconn.Server
	TracerProvider       *sdktrace.TracerProvider
	AutoImportTemplates  []AutoImportTemplate
	LicenseHandler       http.Handler
	FeaturesService      FeaturesService
}

// New constructs a Coder API handler.
func New(options *Options) *API {
	if options.AgentConnectionUpdateFrequency == 0 {
		options.AgentConnectionUpdateFrequency = 3 * time.Second
	}
	if options.AgentInactiveDisconnectTimeout == 0 {
		// Multiply the update by two to allow for some lag-time.
		options.AgentInactiveDisconnectTimeout = options.AgentConnectionUpdateFrequency * 2
	}
	if options.APIRateLimit == 0 {
		options.APIRateLimit = 512
	}
	if options.Authorizer == nil {
		var err error
		options.Authorizer, err = rbac.NewAuthorizer()
		if err != nil {
			// This should never happen, as the unit tests would fail if the
			// default built in authorizer failed.
			panic(xerrors.Errorf("rego authorize panic: %w", err))
		}
	}
	if options.PrometheusRegistry == nil {
		options.PrometheusRegistry = prometheus.NewRegistry()
	}
	if options.LicenseHandler == nil {
		options.LicenseHandler = licenses()
	}
	if options.FeaturesService == nil {
		options.FeaturesService = featuresService{}
	}

	siteCacheDir := options.CacheDir
	if siteCacheDir != "" {
		siteCacheDir = filepath.Join(siteCacheDir, "site")
	}
	binFS, err := site.ExtractOrReadBinFS(siteCacheDir, site.FS())
	if err != nil {
		panic(xerrors.Errorf("read site bin failed: %w", err))
	}

	r := chi.NewRouter()
	api := &API{
		Options:     options,
		Handler:     r,
		siteHandler: site.Handler(site.FS(), binFS),
		httpAuth: &HTTPAuthorizer{
			Authorizer: options.Authorizer,
			Logger:     options.Logger,
		},
	}
	api.workspaceAgentCache = wsconncache.New(api.dialWorkspaceAgent, 0)
	oauthConfigs := &httpmw.OAuth2Configs{
		Github: options.GithubOAuth2Config,
		OIDC:   options.OIDCConfig,
	}
	apiKeyMiddleware := httpmw.ExtractAPIKey(options.Database, oauthConfigs, false)

	r.Use(
		httpmw.Recover(api.Logger),
		httpmw.Logger(api.Logger),
		httpmw.Prometheus(options.PrometheusRegistry),
	)

	apps := func(r chi.Router) {
		r.Use(
			httpmw.RateLimitPerMinute(options.APIRateLimit),
			tracing.HTTPMW(api.TracerProvider, "coderd.http"),
			httpmw.ExtractAPIKey(options.Database, oauthConfigs, true),
			httpmw.ExtractUserParam(api.Database),
			// Extracts the <workspace.agent> from the url
			httpmw.ExtractWorkspaceAndAgentParam(api.Database),
		)
		r.HandleFunc("/*", api.workspaceAppsProxyPath)
	}
	// %40 is the encoded character of the @ symbol. VS Code Web does
	// not handle character encoding properly, so it's safe to assume
	// other applications might not as well.
	r.Route("/%40{user}/{workspace_and_agent}/apps/{workspaceapp}", apps)
	r.Route("/@{user}/{workspace_and_agent}/apps/{workspaceapp}", apps)

	r.Route("/api/v2", func(r chi.Router) {
		r.NotFound(func(rw http.ResponseWriter, r *http.Request) {
			httpapi.Write(rw, http.StatusNotFound, codersdk.Response{
				Message: "Route not found.",
			})
		})
		r.Use(
			// Specific routes can specify smaller limits.
			httpmw.RateLimitPerMinute(options.APIRateLimit),
			tracing.HTTPMW(api.TracerProvider, "coderd.http"),
		)
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			httpapi.Write(w, http.StatusOK, codersdk.Response{
				//nolint:gocritic
				Message: "👋",
			})
		})
		// All CSP errors will be logged
		r.Post("/csp/reports", api.logReportCSPViolations)

		r.Route("/buildinfo", func(r chi.Router) {
			r.Get("/", func(rw http.ResponseWriter, r *http.Request) {
				httpapi.Write(rw, http.StatusOK, codersdk.BuildInfoResponse{
					ExternalURL: buildinfo.ExternalURL(),
					Version:     buildinfo.Version(),
				})
			})
		})
		r.Route("/files", func(r chi.Router) {
			r.Use(
				apiKeyMiddleware,
				// This number is arbitrary, but reading/writing
				// file content is expensive so it should be small.
				httpmw.RateLimitPerMinute(12),
			)
			r.Get("/{hash}", api.fileByHash)
			r.Post("/", api.postFile)
		})
		r.Route("/provisionerdaemons", func(r chi.Router) {
			r.Use(
				apiKeyMiddleware,
			)
			r.Get("/", api.provisionerDaemons)
		})
		r.Route("/organizations", func(r chi.Router) {
			r.Use(
				apiKeyMiddleware,
			)
			r.Post("/", api.postOrganizations)
			r.Route("/{organization}", func(r chi.Router) {
				r.Use(
					httpmw.ExtractOrganizationParam(options.Database),
				)
				r.Get("/", api.organization)
				r.Post("/templateversions", api.postTemplateVersionsByOrganization)
				r.Route("/templates", func(r chi.Router) {
					r.Post("/", api.postTemplateByOrganization)
					r.Get("/", api.templatesByOrganization)
					r.Get("/{templatename}", api.templateByOrganizationAndName)
				})
				r.Post("/workspaces", api.postWorkspacesByOrganization)
				r.Route("/members", func(r chi.Router) {
					r.Get("/roles", api.assignableOrgRoles)
					r.Route("/{user}", func(r chi.Router) {
						r.Use(
							httpmw.ExtractUserParam(options.Database),
							httpmw.ExtractOrganizationMemberParam(options.Database),
						)
						r.Put("/roles", api.putMemberRoles)
					})
				})
			})
		})
		r.Route("/parameters/{scope}/{id}", func(r chi.Router) {
			r.Use(apiKeyMiddleware)
			r.Post("/", api.postParameter)
			r.Get("/", api.parameters)
			r.Route("/{name}", func(r chi.Router) {
				r.Delete("/", api.deleteParameter)
			})
		})
		r.Route("/templates/{template}", func(r chi.Router) {
			r.Use(
				apiKeyMiddleware,
				httpmw.ExtractTemplateParam(options.Database),
			)

			r.Get("/", api.template)
			r.Delete("/", api.deleteTemplate)
			r.Patch("/", api.patchTemplateMeta)
			r.Route("/versions", func(r chi.Router) {
				r.Get("/", api.templateVersionsByTemplate)
				r.Patch("/", api.patchActiveTemplateVersion)
				r.Get("/{templateversionname}", api.templateVersionByName)
			})
		})
		r.Route("/templateversions/{templateversion}", func(r chi.Router) {
			r.Use(
				apiKeyMiddleware,
				httpmw.ExtractTemplateVersionParam(options.Database),
			)

			r.Get("/", api.templateVersion)
			r.Patch("/cancel", api.patchCancelTemplateVersion)
			r.Get("/schema", api.templateVersionSchema)
			r.Get("/parameters", api.templateVersionParameters)
			r.Get("/resources", api.templateVersionResources)
			r.Get("/logs", api.templateVersionLogs)
			r.Route("/dry-run", func(r chi.Router) {
				r.Post("/", api.postTemplateVersionDryRun)
				r.Get("/{jobID}", api.templateVersionDryRun)
				r.Get("/{jobID}/resources", api.templateVersionDryRunResources)
				r.Get("/{jobID}/logs", api.templateVersionDryRunLogs)
				r.Patch("/{jobID}/cancel", api.patchTemplateVersionDryRunCancel)
			})
		})
		r.Route("/users", func(r chi.Router) {
			r.Get("/first", api.firstUser)
			r.Post("/first", api.postFirstUser)
			r.Post("/login", api.postLogin)
			r.Get("/authmethods", api.userAuthMethods)
			r.Route("/oauth2", func(r chi.Router) {
				r.Route("/github", func(r chi.Router) {
					r.Use(httpmw.ExtractOAuth2(options.GithubOAuth2Config))
					r.Get("/callback", api.userOAuth2Github)
				})
			})
			r.Route("/oidc/callback", func(r chi.Router) {
				r.Use(httpmw.ExtractOAuth2(options.OIDCConfig))
				r.Get("/", api.userOIDC)
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
					r.Get("/", api.assignableSiteRoles)
				})
				r.Route("/{user}", func(r chi.Router) {
					r.Use(httpmw.ExtractUserParam(options.Database))
					r.Get("/", api.userByName)
					r.Put("/profile", api.putUserProfile)
					r.Route("/status", func(r chi.Router) {
						r.Put("/suspend", api.putUserStatus(database.UserStatusSuspended))
						r.Put("/activate", api.putUserStatus(database.UserStatusActive))
					})
					r.Route("/password", func(r chi.Router) {
						r.Put("/", api.putUserPassword)
					})
					// These roles apply to the site wide permissions.
					r.Put("/roles", api.putUserRoles)
					r.Get("/roles", api.userRoles)

					r.Post("/authorization", api.checkPermissions)

					r.Route("/keys", func(r chi.Router) {
						r.Post("/", api.postAPIKey)
						r.Get("/{keyid}", api.apiKey)
					})

					r.Route("/organizations", func(r chi.Router) {
						r.Get("/", api.organizationsByUser)
						r.Get("/{organizationname}", api.organizationByUserAndName)
					})
					r.Route("/workspace/{workspacename}", func(r chi.Router) {
						r.Get("/", api.workspaceByOwnerAndName)
						r.Get("/builds/{buildnumber}", api.workspaceBuildByBuildNumber)
					})
					r.Get("/gitsshkey", api.gitSSHKey)
					r.Put("/gitsshkey", api.regenerateGitSSHKey)
				})
			})
		})
		r.Route("/workspaceagents", func(r chi.Router) {
			r.Post("/azure-instance-identity", api.postWorkspaceAuthAzureInstanceIdentity)
			r.Post("/aws-instance-identity", api.postWorkspaceAuthAWSInstanceIdentity)
			r.Post("/google-instance-identity", api.postWorkspaceAuthGoogleInstanceIdentity)
			r.Route("/me", func(r chi.Router) {
				r.Use(httpmw.ExtractWorkspaceAgent(options.Database))
				r.Get("/metadata", api.workspaceAgentMetadata)
				r.Get("/listen", api.workspaceAgentListen)
				r.Get("/gitsshkey", api.agentGitSSHKey)
				r.Get("/turn", api.workspaceAgentTurn)
				r.Get("/iceservers", api.workspaceAgentICEServers)
				r.Get("/wireguardlisten", api.workspaceAgentWireguardListener)
				r.Post("/keys", api.postWorkspaceAgentKeys)
				r.Get("/derp", api.derpMap)
			})
			r.Route("/{workspaceagent}", func(r chi.Router) {
				r.Use(
					apiKeyMiddleware,
					httpmw.ExtractWorkspaceAgentParam(options.Database),
					httpmw.ExtractWorkspaceParam(options.Database),
				)
				r.Get("/", api.workspaceAgent)
				r.Post("/peer", api.postWorkspaceAgentWireguardPeer)
				r.Get("/dial", api.workspaceAgentDial)
				r.Get("/turn", api.userWorkspaceAgentTurn)
				r.Get("/pty", api.workspaceAgentPTY)
				r.Get("/iceservers", api.workspaceAgentICEServers)
				r.Get("/derp", api.derpMap)
			})
		})
		r.Route("/workspaceresources/{workspaceresource}", func(r chi.Router) {
			r.Use(
				apiKeyMiddleware,
				httpmw.ExtractWorkspaceResourceParam(options.Database),
				httpmw.ExtractWorkspaceParam(options.Database),
			)
			r.Get("/", api.workspaceResource)
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
					r.Get("/{workspacebuildname}", api.workspaceBuildByName)
				})
				r.Route("/autostart", func(r chi.Router) {
					r.Put("/", api.putWorkspaceAutostart)
				})
				r.Route("/ttl", func(r chi.Router) {
					r.Put("/", api.putWorkspaceTTL)
				})
				r.Get("/watch", api.watchWorkspace)
				r.Put("/extend", api.putExtendWorkspace)
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
			r.Get("/resources", api.workspaceBuildResources)
			r.Get("/state", api.workspaceBuildState)
		})
		r.Route("/entitlements", func(r chi.Router) {
			r.Use(apiKeyMiddleware)
			r.Get("/", api.FeaturesService.EntitlementsAPI)
		})
		r.Route("/licenses", func(r chi.Router) {
			r.Use(apiKeyMiddleware)
			r.Mount("/", options.LicenseHandler)
		})
	})

	r.NotFound(compressHandler(http.HandlerFunc(api.siteHandler.ServeHTTP)).ServeHTTP)
	return api
}

type API struct {
	*Options

	Handler             chi.Router
	siteHandler         http.Handler
	websocketWaitMutex  sync.Mutex
	websocketWaitGroup  sync.WaitGroup
	workspaceAgentCache *wsconncache.Cache
	httpAuth            *HTTPAuthorizer
}

// Close waits for all WebSocket connections to drain before returning.
func (api *API) Close() error {
	api.websocketWaitMutex.Lock()
	api.websocketWaitGroup.Wait()
	api.websocketWaitMutex.Unlock()

	return api.workspaceAgentCache.Close()
}

func compressHandler(h http.Handler) http.Handler {
	cmp := middleware.NewCompressor(5,
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

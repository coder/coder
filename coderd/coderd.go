package coderd

import (
	"context"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/pion/webrtc/v3"
	"golang.org/x/xerrors"
	"google.golang.org/api/idtoken"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"cdr.dev/slog"
	"github.com/coder/coder/buildinfo"
	"github.com/coder/coder/coderd/awsidentity"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/gitsshkey"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/coderd/tracing"
	"github.com/coder/coder/coderd/turnconn"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisionerd/proto"
	"github.com/coder/coder/site"
)

// Options are requires parameters for Coder to start.
type Options struct {
	AccessURL *url.URL
	Logger    slog.Logger
	Database  database.Store
	Pubsub    database.Pubsub

	AgentConnectionUpdateFrequency time.Duration
	// APIRateLimit is the minutely throughput rate limit per user or ip.
	// Setting a rate limit <0 will disable the rate limiter across the entire
	// app. Specific routes may have their own limiters.
	APIRateLimit         int
	AWSCertificates      awsidentity.Certificates
	AzureCertificates    x509.VerifyOptions
	GoogleTokenValidator *idtoken.Validator
	GithubOAuth2Config   *GithubOAuth2Config
	ICEServers           []webrtc.ICEServer
	SecureAuthCookie     bool
	SSHKeygenAlgorithm   gitsshkey.Algorithm
	TURNServer           *turnconn.Server
	Authorizer           rbac.Authorizer
	TracerProvider       *sdktrace.TracerProvider
}

type CoderD interface {
	Handler() http.Handler
	CloseWait()

	// An in-process provisionerd connection.
	ListenProvisionerDaemon(ctx context.Context) (proto.DRPCProvisionerDaemonClient, error)
}

type coderD struct {
	api     *api
	router  chi.Router
	options *Options
}

// newRouter constructs the Chi Router for the given API.
func newRouter(options *Options, a *api) chi.Router {
	if options.AgentConnectionUpdateFrequency == 0 {
		options.AgentConnectionUpdateFrequency = 3 * time.Second
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
	apiKeyMiddleware := httpmw.ExtractAPIKey(options.Database, &httpmw.OAuth2Configs{
		Github: options.GithubOAuth2Config,
	})

	// TODO: @emyrk we should just move this into 'ExtractAPIKey'.
	authRolesMiddleware := httpmw.ExtractUserRoles(options.Database)

	r := chi.NewRouter()

	r.Use(
		func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				next.ServeHTTP(middleware.NewWrapResponseWriter(w, r.ProtoMajor), r)
			})
		},
		httpmw.Prometheus,
		tracing.HTTPMW(a.TracerProvider, "coderd.http"),
	)

	r.Route("/api/v2", func(r chi.Router) {
		r.NotFound(func(rw http.ResponseWriter, r *http.Request) {
			httpapi.Write(rw, http.StatusNotFound, httpapi.Response{
				Message: "Route not found.",
			})
		})

		r.Use(
			// Specific routes can specify smaller limits.
			httpmw.RateLimitPerMinute(options.APIRateLimit),
			debugLogRequest(a.Logger),
		)
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			httpapi.Write(w, http.StatusOK, httpapi.Response{
				Message: "👋",
			})
		})
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
			r.Get("/{hash}", a.fileByHash)
			r.Post("/", a.postFile)
		})
		r.Route("/organizations/{organization}", func(r chi.Router) {
			r.Use(
				apiKeyMiddleware,
				httpmw.ExtractOrganizationParam(options.Database),
				authRolesMiddleware,
			)
			r.Get("/", a.organization)
			r.Get("/provisionerdaemons", a.provisionerDaemonsByOrganization)
			r.Post("/templateversions", a.postTemplateVersionsByOrganization)
			r.Route("/templates", func(r chi.Router) {
				r.Post("/", a.postTemplateByOrganization)
				r.Get("/", a.templatesByOrganization)
				r.Get("/{templatename}", a.templateByOrganizationAndName)
			})
			r.Route("/workspaces", func(r chi.Router) {
				r.Post("/", a.postWorkspacesByOrganization)
				r.Get("/", a.workspacesByOrganization)
				r.Route("/{user}", func(r chi.Router) {
					r.Use(httpmw.ExtractUserParam(options.Database))
					r.Get("/{workspacename}", a.workspaceByOwnerAndName)
					r.Get("/", a.workspacesByOwner)
				})
			})
			r.Route("/members", func(r chi.Router) {
				r.Get("/roles", a.assignableOrgRoles)
				r.Route("/{user}", func(r chi.Router) {
					r.Use(
						httpmw.ExtractUserParam(options.Database),
					)
					r.Put("/roles", a.putMemberRoles)
				})
			})
		})
		r.Route("/parameters/{scope}/{id}", func(r chi.Router) {
			r.Use(apiKeyMiddleware)
			r.Post("/", a.postParameter)
			r.Get("/", a.parameters)
			r.Route("/{name}", func(r chi.Router) {
				r.Delete("/", a.deleteParameter)
			})
		})
		r.Route("/templates/{template}", func(r chi.Router) {
			r.Use(
				apiKeyMiddleware,
				httpmw.ExtractTemplateParam(options.Database),
			)

			r.Get("/", a.template)
			r.Delete("/", a.deleteTemplate)
			r.Route("/versions", func(r chi.Router) {
				r.Get("/", a.templateVersionsByTemplate)
				r.Patch("/", a.patchActiveTemplateVersion)
				r.Get("/{templateversionname}", a.templateVersionByName)
			})
		})
		r.Route("/templateversions/{templateversion}", func(r chi.Router) {
			r.Use(
				apiKeyMiddleware,
				httpmw.ExtractTemplateVersionParam(options.Database),
			)

			r.Get("/", a.templateVersion)
			r.Patch("/cancel", a.patchCancelTemplateVersion)
			r.Get("/schema", a.templateVersionSchema)
			r.Get("/parameters", a.templateVersionParameters)
			r.Get("/resources", a.templateVersionResources)
			r.Get("/logs", a.templateVersionLogs)
			r.Route("/plan", func(r chi.Router) {
				r.Post("/", a.createTemplateVersionPlan)
				r.Get("/{jobID}", a.templateVersionPlan)
				r.Get("/{jobID}/resources", a.templateVersionPlanResources)
				r.Get("/{jobID}/logs", a.templateVersionPlanLogs)
				r.Patch("/{jobID}/cancel", a.templateVersionPlanCancel)
			})
		})
		r.Route("/users", func(r chi.Router) {
			r.Get("/first", a.firstUser)
			r.Post("/first", a.postFirstUser)
			r.Post("/login", a.postLogin)
			r.Post("/logout", a.postLogout)
			r.Get("/authmethods", a.userAuthMethods)
			r.Route("/oauth2", func(r chi.Router) {
				r.Route("/github", func(r chi.Router) {
					r.Use(httpmw.ExtractOAuth2(options.GithubOAuth2Config))
					r.Get("/callback", a.userOAuth2Github)
				})
			})
			r.Group(func(r chi.Router) {
				r.Use(
					apiKeyMiddleware,
					authRolesMiddleware,
				)
				r.Post("/", a.postUser)
				r.Get("/", a.users)
				// These routes query information about site wide roles.
				r.Route("/roles", func(r chi.Router) {
					r.Get("/", a.assignableSiteRoles)
				})
				r.Route("/{user}", func(r chi.Router) {
					r.Use(httpmw.ExtractUserParam(options.Database))
					r.Get("/", a.userByName)
					r.Put("/profile", a.putUserProfile)
					r.Route("/status", func(r chi.Router) {
						r.Put("/suspend", a.putUserStatus(database.UserStatusSuspended))
						r.Put("/active", a.putUserStatus(database.UserStatusActive))
					})
					r.Route("/password", func(r chi.Router) {
						r.Put("/", a.putUserPassword)
					})
					// These roles apply to the site wide permissions.
					r.Put("/roles", a.putUserRoles)
					r.Get("/roles", a.userRoles)

					r.Post("/authorization", a.checkPermissions)

					r.Post("/keys", a.postAPIKey)
					r.Route("/organizations", func(r chi.Router) {
						r.Post("/", a.postOrganizationsByUser)
						r.Get("/", a.organizationsByUser)
						r.Get("/{organizationname}", a.organizationByUserAndName)
					})
					r.Get("/gitsshkey", a.gitSSHKey)
					r.Put("/gitsshkey", a.regenerateGitSSHKey)
				})
			})
		})
		r.Route("/workspaceagents", func(r chi.Router) {
			r.Post("/azure-instance-identity", a.postWorkspaceAuthAzureInstanceIdentity)
			r.Post("/aws-instance-identity", a.postWorkspaceAuthAWSInstanceIdentity)
			r.Post("/google-instance-identity", a.postWorkspaceAuthGoogleInstanceIdentity)
			r.Route("/me", func(r chi.Router) {
				r.Use(httpmw.ExtractWorkspaceAgent(options.Database))
				r.Get("/metadata", a.workspaceAgentMetadata)
				r.Get("/listen", a.workspaceAgentListen)
				r.Get("/gitsshkey", a.agentGitSSHKey)
				r.Get("/turn", a.workspaceAgentTurn)
				r.Get("/iceservers", a.workspaceAgentICEServers)
			})
			r.Route("/{workspaceagent}", func(r chi.Router) {
				r.Use(
					apiKeyMiddleware,
					httpmw.ExtractWorkspaceAgentParam(options.Database),
				)
				r.Get("/", a.workspaceAgent)
				r.Get("/dial", a.workspaceAgentDial)
				r.Get("/turn", a.workspaceAgentTurn)
				r.Get("/pty", a.workspaceAgentPTY)
				r.Get("/iceservers", a.workspaceAgentICEServers)
			})
		})
		r.Route("/workspaceresources/{workspaceresource}", func(r chi.Router) {
			r.Use(
				apiKeyMiddleware,
				httpmw.ExtractWorkspaceResourceParam(options.Database),
				httpmw.ExtractWorkspaceParam(options.Database),
			)
			r.Get("/", a.workspaceResource)
		})
		r.Route("/workspaces", func(r chi.Router) {
			r.Use(
				apiKeyMiddleware,
				authRolesMiddleware,
			)
			r.Get("/", a.workspaces)
			r.Route("/{workspace}", func(r chi.Router) {
				r.Use(
					httpmw.ExtractWorkspaceParam(options.Database),
				)
				r.Get("/", a.workspace)
				r.Route("/builds", func(r chi.Router) {
					r.Get("/", a.workspaceBuilds)
					r.Post("/", a.postWorkspaceBuilds)
					r.Get("/{workspacebuildname}", a.workspaceBuildByName)
				})
				r.Route("/autostart", func(r chi.Router) {
					r.Put("/", a.putWorkspaceAutostart)
				})
				r.Route("/ttl", func(r chi.Router) {
					r.Put("/", a.putWorkspaceTTL)
				})
				r.Get("/watch", a.watchWorkspace)
			})
		})
		r.Route("/workspacebuilds/{workspacebuild}", func(r chi.Router) {
			r.Use(
				apiKeyMiddleware,
				authRolesMiddleware,
				httpmw.ExtractWorkspaceBuildParam(options.Database),
				httpmw.ExtractWorkspaceParam(options.Database),
			)
			r.Get("/", a.workspaceBuild)
			r.Patch("/cancel", a.patchCancelWorkspaceBuild)
			r.Get("/logs", a.workspaceBuildLogs)
			r.Get("/resources", a.workspaceBuildResources)
			r.Get("/state", a.workspaceBuildState)
		})
	})

	var _ = xerrors.New("test")

	r.NotFound(site.DefaultHandler().ServeHTTP)
	return r
}

func New(options *Options) CoderD {
	a := &api{Options: options}
	return &coderD{
		api:     a,
		router:  newRouter(options, a),
		options: options,
	}
}

func (c *coderD) CloseWait() {
	c.api.websocketWaitMutex.Lock()
	c.api.websocketWaitGroup.Wait()
	c.api.websocketWaitMutex.Unlock()
}

func (c *coderD) Handler() http.Handler {
	return c.router
}

// API contains all route handlers. Only HTTP handlers should
// be added to this struct for code clarity.
type api struct {
	*Options

	websocketWaitMutex sync.Mutex
	websocketWaitGroup sync.WaitGroup
}

func debugLogRequest(log slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			log.Debug(context.Background(), fmt.Sprintf("%s %s", r.Method, r.URL.Path))
			next.ServeHTTP(rw, r)
		})
	}
}

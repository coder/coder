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

	chitrace "gopkg.in/DataDog/dd-trace-go.v1/contrib/go-chi/chi.v5"

	"cdr.dev/slog"
	"github.com/coder/coder/buildinfo"
	"github.com/coder/coder/coderd/awsidentity"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/gitsshkey"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/coderd/turnconn"
	"github.com/coder/coder/codersdk"
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
}

// New constructs the Coder API into an HTTP handler.
//
// A wait function is returned to handle awaiting closure of hijacked HTTP
// requests.
func New(options *Options) (http.Handler, func()) {
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
	api := &api{
		Options: options,
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
		chitrace.Middleware(),
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
			debugLogRequest(api.Logger),
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
			r.Get("/{hash}", api.fileByHash)
			r.Post("/", api.postFile)
		})
		r.Route("/organizations/{organization}", func(r chi.Router) {
			r.Use(
				apiKeyMiddleware,
				httpmw.ExtractOrganizationParam(options.Database),
				authRolesMiddleware,
			)
			r.Get("/", api.organization)
			r.Get("/provisionerdaemons", api.provisionerDaemonsByOrganization)
			r.Post("/templateversions", api.postTemplateVersionsByOrganization)
			r.Route("/templates", func(r chi.Router) {
				r.Post("/", api.postTemplateByOrganization)
				r.Get("/", api.templatesByOrganization)
				r.Get("/{templatename}", api.templateByOrganizationAndName)
			})
			r.Route("/workspaces", func(r chi.Router) {
				r.Post("/", api.postWorkspacesByOrganization)
				r.Get("/", api.workspacesByOrganization)
				r.Route("/{user}", func(r chi.Router) {
					r.Use(httpmw.ExtractUserParam(options.Database))
					r.Get("/{workspacename}", api.workspaceByOwnerAndName)
					r.Get("/", api.workspacesByOwner)
				})
			})
			r.Route("/members", func(r chi.Router) {
				r.Get("/roles", api.assignableOrgRoles)
				r.Route("/{user}", func(r chi.Router) {
					r.Use(
						httpmw.ExtractUserParam(options.Database),
					)
					r.Put("/roles", api.putMemberRoles)
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
		})
		r.Route("/provisionerdaemons", func(r chi.Router) {
			r.Route("/me", func(r chi.Router) {
				r.Get("/listen", api.provisionerDaemonsListen)
			})
		})
		r.Route("/users", func(r chi.Router) {
			r.Get("/first", api.firstUser)
			r.Post("/first", api.postFirstUser)
			r.Post("/login", api.postLogin)
			r.Post("/logout", api.postLogout)
			r.Get("/authmethods", api.userAuthMethods)
			r.Route("/oauth2", func(r chi.Router) {
				r.Route("/github", func(r chi.Router) {
					r.Use(httpmw.ExtractOAuth2(options.GithubOAuth2Config))
					r.Get("/callback", api.userOAuth2Github)
				})
			})
			r.Group(func(r chi.Router) {
				r.Use(
					apiKeyMiddleware,
					authRolesMiddleware,
				)
				r.Post("/", api.postUser)
				r.Get("/", api.users)
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
						r.Put("/active", api.putUserStatus(database.UserStatusActive))
					})
					r.Route("/password", func(r chi.Router) {
						r.Put("/", api.putUserPassword)
					})
					// These roles apply to the site wide permissions.
					r.Put("/roles", api.putUserRoles)
					r.Get("/roles", api.userRoles)

					r.Post("/authorization", api.checkPermissions)

					r.Post("/keys", api.postAPIKey)
					r.Route("/organizations", func(r chi.Router) {
						r.Post("/", api.postOrganizationsByUser)
						r.Get("/", api.organizationsByUser)
						r.Get("/{organizationname}", api.organizationByUserAndName)
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
			})
			r.Route("/{workspaceagent}", func(r chi.Router) {
				r.Use(
					apiKeyMiddleware,
					httpmw.ExtractWorkspaceAgentParam(options.Database),
				)
				r.Get("/", api.workspaceAgent)
				r.Get("/dial", api.workspaceAgentDial)
				r.Get("/turn", api.workspaceAgentTurn)
				r.Get("/pty", api.workspaceAgentPTY)
				r.Get("/iceservers", api.workspaceAgentICEServers)
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
				authRolesMiddleware,
			)
			r.Get("/", api.workspaces)
			r.Route("/{workspace}", func(r chi.Router) {
				r.Use(
					httpmw.ExtractWorkspaceParam(options.Database),
				)
				r.Get("/", api.workspace)
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
			})
		})
		r.Route("/workspacebuilds/{workspacebuild}", func(r chi.Router) {
			r.Use(
				apiKeyMiddleware,
				authRolesMiddleware,
				httpmw.ExtractWorkspaceBuildParam(options.Database),
				httpmw.ExtractWorkspaceParam(options.Database),
			)
			r.Get("/", api.workspaceBuild)
			r.Patch("/cancel", api.patchCancelWorkspaceBuild)
			r.Get("/logs", api.workspaceBuildLogs)
			r.Get("/resources", api.workspaceBuildResources)
			r.Get("/state", api.workspaceBuildState)
		})
	})

	var _ = xerrors.New("test")

	r.NotFound(site.DefaultHandler().ServeHTTP)
	return r, func() {
		api.websocketWaitMutex.Lock()
		api.websocketWaitGroup.Wait()
		api.websocketWaitMutex.Unlock()
	}
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

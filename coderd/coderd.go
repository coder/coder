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

	authorize := func(f http.HandlerFunc, actions ...rbac.Action) http.HandlerFunc {
		return httpmw.Authorize(api.Logger, api.Authorizer, actions...)(f).ServeHTTP
	}

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
				authRolesMiddleware,
				// This number is arbitrary, but reading/writing
				// file content is expensive so it should be small.
				httpmw.RateLimitPerMinute(12),
				// TODO: @emyrk (rbac) Currently files are owned by the site?
				//	Should files be org scoped? User scoped?
				httpmw.WithRBACObject(rbac.ResourceFile),
			)
			r.Get("/{hash}", authorize(api.fileByHash, rbac.ActionRead))
			r.Post("/", authorize(api.postFile, rbac.ActionCreate, rbac.ActionUpdate))
		})
		r.Route("/organizations/{organization}", func(r chi.Router) {
			r.Use(
				apiKeyMiddleware,
				authRolesMiddleware,
				httpmw.ExtractOrganizationParam(options.Database),
			)
			r.With(httpmw.WithRBACObject(rbac.ResourceOrganization)).
				Get("/", authorize(api.organization, rbac.ActionRead))
			r.Get("/provisionerdaemons", api.provisionerDaemonsByOrganization)
			r.Post("/templateversions", api.postTemplateVersionsByOrganization)
			r.Route("/templates", func(r chi.Router) {
				r.Post("/", api.postTemplatesByOrganization)
				r.Get("/", api.templatesByOrganization)
				r.Get("/{templatename}", api.templateByOrganizationAndName)
			})
			r.Route("/workspaces", func(r chi.Router) {
				r.Use(httpmw.WithRBACObject(rbac.ResourceWorkspace))
				// Posting a workspace is inherently owned by the api key creating it.
				r.With(httpmw.WithAPIKeyAsOwner()).
					Post("/", authorize(api.postWorkspacesByOrganization, rbac.ActionCreate))
				r.Get("/", authorize(api.workspacesByOrganization, rbac.ActionRead))
				r.Route("/{user}", func(r chi.Router) {
					r.Use(httpmw.ExtractUserParam(options.Database))
					// TODO: @emyrk add the resource id to this authorize.
					r.Get("/{workspace}", authorize(api.workspaceByOwnerAndName, rbac.ActionRead))
					r.Get("/", authorize(api.workspacesByOwner, rbac.ActionRead))
				})
			})
			r.Route("/members", func(r chi.Router) {
				r.Route("/roles", func(r chi.Router) {
					r.Use(httpmw.WithRBACObject(rbac.ResourceUserRole))
					r.Get("/", authorize(api.assignableOrgRoles, rbac.ActionRead))
				})
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
				httpmw.ExtractOrganizationParam(options.Database),
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
				httpmw.ExtractOrganizationParam(options.Database),
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
				r.Group(func(r chi.Router) {
					// Site wide, all users
					r.Use(httpmw.WithRBACObject(rbac.ResourceUser))
					r.Post("/", authorize(api.postUser, rbac.ActionCreate))
					r.Get("/", authorize(api.users, rbac.ActionRead))
				})
				// These routes query information about site wide roles.
				r.Route("/roles", func(r chi.Router) {
					r.Use(httpmw.WithRBACObject(rbac.ResourceUserRole))
					r.Get("/", authorize(api.assignableSiteRoles, rbac.ActionRead))
				})
				r.Route("/{user}", func(r chi.Router) {
					r.Use(httpmw.ExtractUserParam(options.Database))
					r.Group(func(r chi.Router) {
						r.Use(httpmw.WithRBACObject(rbac.ResourceUser))
						r.Get("/", authorize(api.userByName, rbac.ActionRead))
						r.Put("/profile", authorize(api.putUserProfile, rbac.ActionUpdate))
						// suspension is deleting for a user
						r.Put("/suspend", authorize(api.putUserSuspend, rbac.ActionDelete))
						r.Route("/password", func(r chi.Router) {
							r.Put("/", authorize(api.putUserPassword, rbac.ActionUpdate))
						})
						// This route technically also fetches the organization member struct, but only
						// returns the roles.
						r.Get("/roles", authorize(api.userRoles, rbac.ActionRead))

						// This has 2 authorize calls. The second is explicitly called
						// in the handler.
						r.Put("/roles", authorize(api.putUserRoles, rbac.ActionUpdate))

						// For now, just use the "user" role for their ssh keys.
						// We can always split this out to it's own resource if we need to.
						r.Get("/gitsshkey", authorize(api.gitSSHKey, rbac.ActionRead))
						r.Put("/gitsshkey", authorize(api.regenerateGitSSHKey, rbac.ActionUpdate))
					})

					r.With(httpmw.WithRBACObject(rbac.ResourceAPIKey)).Post("/keys", authorize(api.postAPIKey, rbac.ActionCreate))

					r.Route("/organizations", func(r chi.Router) {
						// TODO: @emyrk This creates an organization, so why is it nested under {user}?
						//	Shouldn't this be outside the {user} param subpath? Maybe in the organizations/
						//	path?
						r.Post("/", api.postOrganizationsByUser)

						r.Get("/", api.organizationsByUser)

						// TODO: @emyrk why is this nested under {user} when the user param is not used?
						r.Get("/{organizationname}", api.organizationByUserAndName)
					})
					r.Get("/workspaces", api.workspacesByUser)
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
		r.Route("/workspaces/{workspace}", func(r chi.Router) {
			r.Use(
				apiKeyMiddleware,
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
			r.Route("/autostop", func(r chi.Router) {
				r.Put("/", api.putWorkspaceAutostop)
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
	})
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

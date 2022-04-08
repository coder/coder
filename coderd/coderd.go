package coderd

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"google.golang.org/api/idtoken"

	chitrace "gopkg.in/DataDog/dd-trace-go.v1/contrib/go-chi/chi.v5"

	"cdr.dev/slog"
	"github.com/coder/coder/buildinfo"
	"github.com/coder/coder/coderd/awsidentity"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/gitsshkey"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/site"
)

// Options are requires parameters for Coder to start.
type Options struct {
	AgentConnectionUpdateFrequency time.Duration
	AccessURL                      *url.URL
	Logger                         slog.Logger
	Database                       database.Store
	Pubsub                         database.Pubsub

	AWSCertificates      awsidentity.Certificates
	GoogleTokenValidator *idtoken.Validator

	SecureAuthCookie   bool
	SSHKeygenAlgorithm gitsshkey.Algorithm
}

// New constructs the Coder API into an HTTP handler.
//
// A wait function is returned to handle awaiting closure of hijacked HTTP
// requests.
func New(options *Options) (http.Handler, func()) {
	if options.AgentConnectionUpdateFrequency == 0 {
		options.AgentConnectionUpdateFrequency = 3 * time.Second
	}
	api := &api{
		Options: options,
	}

	r := chi.NewRouter()
	r.Route("/api/v2", func(r chi.Router) {
		r.Use(
			chitrace.Middleware(),
			// Specific routes can specify smaller limits.
			httpmw.RateLimitPerMinute(512),
			debugLogRequest(api.Logger),
		)
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			httpapi.Write(w, http.StatusOK, httpapi.Response{
				Message: "👋",
			})
		})
		r.Route("/buildinfo", func(r chi.Router) {
			r.Get("/", func(rw http.ResponseWriter, r *http.Request) {
				render.Status(r, http.StatusOK)
				render.JSON(rw, r, codersdk.BuildInfoResponse{
					ExternalURL: buildinfo.ExternalURL(),
					Version:     buildinfo.Version(),
				})
			})
		})
		r.Route("/files", func(r chi.Router) {
			r.Use(
				httpmw.ExtractAPIKey(options.Database, nil),
				// This number is arbitrary, but reading/writing
				// file content is expensive so it should be small.
				httpmw.RateLimitPerMinute(12),
			)
			r.Get("/{hash}", api.fileByHash)
			r.Post("/", api.postFile)
		})
		r.Route("/organizations/{organization}", func(r chi.Router) {
			r.Use(
				httpmw.ExtractAPIKey(options.Database, nil),
				httpmw.ExtractOrganizationParam(options.Database),
			)
			r.Get("/", api.organization)
			r.Get("/provisionerdaemons", api.provisionerDaemonsByOrganization)
			r.Post("/templateversions", api.postTemplateVersionsByOrganization)
			r.Route("/templates", func(r chi.Router) {
				r.Post("/", api.postTemplatesByOrganization)
				r.Get("/", api.templatesByOrganization)
				r.Get("/{templatename}", api.templateByOrganizationAndName)
			})
		})
		r.Route("/parameters/{scope}/{id}", func(r chi.Router) {
			r.Use(httpmw.ExtractAPIKey(options.Database, nil))
			r.Post("/", api.postParameter)
			r.Get("/", api.parameters)
			r.Route("/{name}", func(r chi.Router) {
				r.Delete("/", api.deleteParameter)
			})
		})
		r.Route("/templates/{template}", func(r chi.Router) {
			r.Use(
				httpmw.ExtractAPIKey(options.Database, nil),
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
				httpmw.ExtractAPIKey(options.Database, nil),
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
			r.Group(func(r chi.Router) {
				r.Use(httpmw.ExtractAPIKey(options.Database, nil))
				r.Post("/", api.postUsers)
				r.Route("/{user}", func(r chi.Router) {
					r.Use(httpmw.ExtractUserParam(options.Database))
					r.Get("/", api.userByName)
					r.Get("/organizations", api.organizationsByUser)
					r.Post("/organizations", api.postOrganizationsByUser)
					r.Post("/keys", api.postAPIKey)
					r.Route("/organizations", func(r chi.Router) {
						r.Post("/", api.postOrganizationsByUser)
						r.Get("/", api.organizationsByUser)
						r.Get("/{organizationname}", api.organizationByUserAndName)
					})
					r.Route("/workspaces", func(r chi.Router) {
						r.Post("/", api.postWorkspacesByUser)
						r.Get("/", api.workspacesByUser)
						r.Get("/{workspacename}", api.workspaceByUserAndName)
					})
					r.Get("/gitsshkey", api.gitSSHKey)
					r.Put("/gitsshkey", api.regenerateGitSSHKey)
				})
			})
		})
		r.Route("/workspaceresources", func(r chi.Router) {
			r.Route("/auth", func(r chi.Router) {
				r.Post("/aws-instance-identity", api.postWorkspaceAuthAWSInstanceIdentity)
				r.Post("/google-instance-identity", api.postWorkspaceAuthGoogleInstanceIdentity)
			})
			r.Route("/agent", func(r chi.Router) {
				r.Use(httpmw.ExtractWorkspaceAgent(options.Database))
				r.Get("/", api.workspaceAgentListen)
				r.Get("/gitsshkey", api.agentGitSSHKey)
			})
			r.Route("/{workspaceresource}", func(r chi.Router) {
				r.Use(
					httpmw.ExtractAPIKey(options.Database, nil),
					httpmw.ExtractWorkspaceResourceParam(options.Database),
					httpmw.ExtractWorkspaceParam(options.Database),
				)
				r.Get("/", api.workspaceResource)
				r.Get("/dial", api.workspaceResourceDial)
			})
		})
		r.Route("/workspaces/{workspace}", func(r chi.Router) {
			r.Use(
				httpmw.ExtractAPIKey(options.Database, nil),
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
				httpmw.ExtractAPIKey(options.Database, nil),
				httpmw.ExtractWorkspaceBuildParam(options.Database),
				httpmw.ExtractWorkspaceParam(options.Database),
			)
			r.Get("/", api.workspaceBuild)
			r.Patch("/cancel", api.patchCancelWorkspaceBuild)
			r.Get("/logs", api.workspaceBuildLogs)
			r.Get("/resources", api.workspaceBuildResources)
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

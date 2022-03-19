package coderd

import (
	"net/http"
	"net/url"
	"sync"

	"github.com/go-chi/chi/v5"
	"google.golang.org/api/idtoken"

	"cdr.dev/slog"
	"github.com/coder/coder/database"
	"github.com/coder/coder/httpapi"
	"github.com/coder/coder/httpmw"
	"github.com/coder/coder/site"
)

// Options are requires parameters for Coder to start.
type Options struct {
	AccessURL *url.URL
	Logger    slog.Logger
	Database  database.Store
	Pubsub    database.Pubsub

	GoogleTokenValidator *idtoken.Validator
}

// New constructs the Coder API into an HTTP handler.
//
// A wait function is returned to handle awaiting closure of hijacked HTTP
// requests.
func New(options *Options) (http.Handler, func()) {
	api := &api{
		Options: options,
	}

	r := chi.NewRouter()
	r.Route("/api/v2", func(r chi.Router) {
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			httpapi.Write(w, http.StatusOK, httpapi.Response{
				Message: "👋",
			})
		})
		r.Route("/files", func(r chi.Router) {
			r.Use(httpmw.ExtractAPIKey(options.Database, nil))
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
			r.Post("/projectversions", api.postProjectVersionsByOrganization)
			r.Route("/projects", func(r chi.Router) {
				r.Post("/", api.postProjectsByOrganization)
				r.Get("/", api.projectsByOrganization)
				r.Get("/{projectname}", api.projectByOrganizationAndName)
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
		r.Route("/projects/{project}", func(r chi.Router) {
			r.Use(
				httpmw.ExtractAPIKey(options.Database, nil),
				httpmw.ExtractProjectParam(options.Database),
				httpmw.ExtractOrganizationParam(options.Database),
			)
			r.Get("/", api.project)
			r.Route("/versions", func(r chi.Router) {
				r.Get("/", api.projectVersionsByProject)
				r.Patch("/versions", nil)
				r.Get("/{projectversionname}", api.projectVersionByName)
			})
		})
		r.Route("/projectversions/{projectversion}", func(r chi.Router) {
			r.Use(
				httpmw.ExtractAPIKey(options.Database, nil),
				httpmw.ExtractProjectVersionParam(options.Database),
				httpmw.ExtractOrganizationParam(options.Database),
			)

			r.Get("/", api.projectVersion)
			r.Get("/schema", api.projectVersionSchema)
			r.Get("/parameters", api.projectVersionParameters)
			r.Get("/resources", api.projectVersionResources)
			r.Get("/logs", api.projectVersionLogs)
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
				})
			})
		})
		r.Route("/workspaceresources", func(r chi.Router) {
			r.Route("/auth", func(r chi.Router) {
				r.Post("/google-instance-identity", api.postWorkspaceAuthGoogleInstanceIdentity)
			})
			r.Route("/agent", func(r chi.Router) {
				r.Use(httpmw.ExtractWorkspaceAgent(options.Database))
				r.Get("/", api.workspaceAgentListen)
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
				r.Get("/latest", api.workspaceBuildLatest)
				r.Get("/{workspacebuildname}", api.workspaceBuildByName)
			})
		})
		r.Route("/workspacebuilds/{workspacebuild}", func(r chi.Router) {
			r.Use(
				httpmw.ExtractAPIKey(options.Database, nil),
				httpmw.ExtractWorkspaceBuildParam(options.Database),
				httpmw.ExtractWorkspaceParam(options.Database),
			)
			r.Get("/", api.workspaceBuild)
			r.Get("/logs", api.workspaceBuildLogs)
			r.Get("/resources", api.workspaceBuildResources)
		})
	})
	r.NotFound(site.DefaultHandler().ServeHTTP)
	return r, api.websocketWaitGroup.Wait
}

// API contains all route handlers. Only HTTP handlers should
// be added to this struct for code clarity.
type api struct {
	*Options

	websocketWaitGroup sync.WaitGroup
}

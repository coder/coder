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
// A wait function is returned to handle awaiting closure
// of hijacked HTTP requests.
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
		r.Post("/login", api.postLogin)
		r.Post("/logout", api.postLogout)

		// Used for setup.
		r.Get("/user", api.user)
		r.Post("/user", api.postUser)
		r.Route("/users", func(r chi.Router) {
			r.Use(
				httpmw.ExtractAPIKey(options.Database, nil),
			)
			r.Post("/", api.postUsers)

			r.Route("/{user}", func(r chi.Router) {
				r.Use(httpmw.ExtractUserParam(options.Database))
				r.Get("/", api.userByName)
				r.Get("/organizations", api.organizationsByUser)
				r.Post("/keys", api.postKeyForUser)
			})
		})
		r.Route("/organization/{organization}", func(r chi.Router) {
			r.Use(
				httpmw.ExtractAPIKey(options.Database, nil),
				httpmw.ExtractOrganizationParam(options.Database),
			)
			r.Route("/projects", func(r chi.Router) {
				r.Post("/", api.postProjectsByOrganization)
				r.Get("/", api.projectsByOrganization)
				r.Get("/{projectname}", api.projectByOrganizationAndName)
			})
		})
		r.Route("/project/{project}", func(r chi.Router) {
			r.Use(
				httpmw.ExtractAPIKey(options.Database, nil),
				httpmw.ExtractProjectParam(options.Database),
			)
			r.Get("/", api.projectByOrganization)
			r.Get("/workspaces", api.workspacesByProject)
			r.Route("/parameters", func(r chi.Router) {
				r.Get("/", api.parametersByProject)
				r.Post("/", api.postParametersByProject)
			})
			r.Route("/versions", func(r chi.Router) {
				r.Get("/", api.projectVersionsByOrganization)
				r.Post("/", api.postProjectVersionByOrganization)
				r.Route("/{projectversion}", func(r chi.Router) {
					r.Use(httpmw.ExtractProjectVersionParam(api.Database))
					r.Get("/", api.projectVersionByOrganizationAndName)
				})
			})
			r.Get("/", api.projects)
			r.Route("/{organization}", func(r chi.Router) {
				r.Use(httpmw.ExtractOrganizationParam(options.Database))
				r.Get("/", api.projectsByOrganization)
				r.Post("/", api.postProjectsByOrganization)
				r.Route("/{project}", func(r chi.Router) {

				})
			})
		})

		// Upload - Uploads a file.

		// CreateProject(organization) - Creates a project with a version.
		// ImportProjectVersion(organization, project?) - If a project is provided, it's attached. If a project isn't, it's detached.
		// ProjectVersions - Returns a list of project versions by project.
		// ProjectVersionSchema(projectversion) - Return parameter schemas for a job.
		// ProjectVersionParameters(projectversion) - Returns computed parameters for a job.
		// ProjectVersionParameters(projectversion) - Returns computed parameters for a job.
		// ProjectVersionLogs(projectversion) - Returns logs for an executing project version.
		// ProjectVersionLogsAfter(projectversion, timestamp) - Streams logs that occur after a specific timestamp.
		// ProjectVersionResources(projectversion, resources) - Returns resources to be created for a project version.

		// CreateWorkspace - Creates a workspace for a project.
		// ProvisionWorkspace - Creates a new build.
		// Workspaces - Returns all workspaces the user has access to.
		// WorkspacesByProject - Returns workspaces inside a project.
		// WorkspaceByName - Returns a workspace by name.
		// Workspace - Returns a single workspace by ID.
		// WorkspaceProvisions - Returns a timeline of provisions for a workspace.
		// WorkspaceProvisionResources - List resources for a specific workspace version.
		// WorkspaceProvisionResource - Get a specific resource.
		// WorkspaceProvisionLogs - Returns a stream of logs.
		// WorkspaceProvisionLogsAfter - Returns a stream of logs after.
		// DialWorkspaceAgent - Creates the connection to a workspace agent.
		// ListenWorkspaceAgent - Listens to the workspace agent as the ID.
		// AuthWorkspaceAgentWithGoogleInstanceIdentity - Exchanges SA for token.

		// User - Returns the currently authenticated user.
		// HasFirstUser - Returns whether the first user has been created.
		// CreateFirstUser - Creates a new user and the organization provided.
		// CreateUser - Creates a new user and adds them to the organization.
		// CreateAPIKey - Creates a new API key.
		// LoginWithPassword - Authenticates with email and password.
		// Logout - Should clear the session token.

		// ProvisionerDaemons
		// ListenProvisionerDaemon

		// OrganizationsByUser - Returns organizations by user.

		r.Route("/agent", func(r chi.Router) {

		})

		r.Route("/daemon", func(r chi.Router) {

		})

		r.Route("/job", func(r chi.Router) {
			r.Use(
				httpmw.ExtractAPIKey(options.Database, nil),
				httpmw.ExtractOrganizationParam(options.Database),
			)
			r.Post("/", api.postProjectImportByOrganization)
		})

		r.Route("/provisioner/job/{job}/resources", func(r chi.Router) {

		})

		// ProjectVersionLogs

		// Listing operations specific to resources should go under
		// their respective routes. eg. /orgs/<name>/workspaces
		r.Route("/workspaces", func(r chi.Router) {
			r.Use(httpmw.ExtractAPIKey(options.Database, nil))
			r.Get("/", api.workspaces)
			r.Route("/{user}", func(r chi.Router) {
				r.Use(httpmw.ExtractUserParam(options.Database))
				r.Post("/", api.postWorkspaceByUser)
				r.Route("/{workspace}", func(r chi.Router) {
					r.Use(httpmw.ExtractWorkspaceParam(options.Database))
					r.Get("/", api.workspaceByUser)
					r.Route("/version", func(r chi.Router) {
						r.Post("/", api.postWorkspaceBuildByUser)
						r.Get("/", api.workspaceBuildByUser)
						r.Route("/{workspacebuild}", func(r chi.Router) {
							r.Use(httpmw.ExtractWorkspaceBuildParam(options.Database))
							r.Get("/", api.workspaceBuildByName)
						})
					})
				})
			})
		})

		r.Route("/workspaceagent", func(r chi.Router) {
			r.Route("/authenticate", func(r chi.Router) {
				r.Post("/google-instance-identity", api.postAuthenticateWorkspaceAgentUsingGoogleInstanceIdentity)
			})
			r.Group(func(r chi.Router) {
				r.Use(httpmw.ExtractWorkspaceAgent(options.Database))
				r.Get("/serve", api.workspaceAgentServe)
			})
		})

		r.Route("/upload", func(r chi.Router) {
			r.Use(httpmw.ExtractAPIKey(options.Database, nil))
			r.Post("/", api.postUpload)
		})

		r.Route("/projectimport/{organization}", func(r chi.Router) {
			r.Use(
				httpmw.ExtractAPIKey(options.Database, nil),
				httpmw.ExtractOrganizationParam(options.Database),
			)
			r.Post("/", api.postProjectImportByOrganization)
			r.Route("/{provisionerjob}", func(r chi.Router) {
				r.Use(httpmw.ExtractProvisionerJobParam(options.Database))
				r.Get("/", api.provisionerJobByID)
				r.Get("/schemas", api.projectImportJobSchemasByID)
				r.Get("/parameters", api.projectImportJobParametersByID)
				r.Get("/resources", api.provisionerJobResourcesByID)
				r.Get("/logs", api.provisionerJobLogsByID)
			})
		})

		r.Route("/workspaceprovision/{organization}", func(r chi.Router) {
			r.Use(
				httpmw.ExtractAPIKey(options.Database, nil),
				httpmw.ExtractOrganizationParam(options.Database),
			)
			r.Route("/{provisionerjob}", func(r chi.Router) {
				r.Use(httpmw.ExtractProvisionerJobParam(options.Database))
				r.Get("/", api.provisionerJobByID)
				r.Get("/logs", api.provisionerJobLogsByID)
				r.Route("/resources", func(r chi.Router) {
					r.Get("/", api.provisionerJobResourcesByID)
					r.Route("/{workspaceresource}", func(r chi.Router) {
						r.Use(httpmw.ExtractWorkspaceResourceParam(options.Database))
						r.Get("/", api.provisionerJobResourceByID)
						r.Get("/agent", api.workspaceAgentConnectByResource)
					})
				})
			})
		})

		r.Route("/provisioners/daemons", func(r chi.Router) {
			r.Get("/", api.provisionerDaemons)
			r.Get("/serve", api.provisionerDaemonsServe)
		})
	})
	r.NotFound(site.Handler(options.Logger).ServeHTTP)
	return r, api.websocketWaitGroup.Wait
}

// API contains all route handlers. Only HTTP handlers should
// be added to this struct for code clarity.
type api struct {
	*Options

	websocketWaitGroup sync.WaitGroup
}

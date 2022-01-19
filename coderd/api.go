package coderd

// API offers an HTTP API. Routes are located in routes.go.
type API struct {
	// Services.
	projectService   *projectService
	workspaceService *workspaceService
}

// New returns an instantiated API.
func NewAPI() *API {
	api := &API{
		projectService:   newProjectService(),
		workspaceService: newWorkspaceService(),
	}
	return api
}

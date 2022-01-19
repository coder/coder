package coderd

import "context"

// API offers an HTTP API. Routes are located in routes.go.
type API struct {
	// Services.
	projectService *projectService
}

// New returns an instantiated API.
func NewAPI(ctx context.Context) *API {
	api := &API{
		projectService: newProjectService(),
	}
	return api
}

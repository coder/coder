package authz

type ResourceType string

const (
	ResourceTypeWorkspace = "workspace"
	ResourceTypeProject   = "project"
	ResourceTypeDevURL    = "devurl"
)

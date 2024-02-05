package codersdk

import (
	"github.com/google/uuid"
)

type WorkspaceAppHealth string

const (
	WorkspaceAppHealthDisabled     WorkspaceAppHealth = "disabled"
	WorkspaceAppHealthInitializing WorkspaceAppHealth = "initializing"
	WorkspaceAppHealthHealthy      WorkspaceAppHealth = "healthy"
	WorkspaceAppHealthUnhealthy    WorkspaceAppHealth = "unhealthy"
)

var MapWorkspaceAppHealths = map[WorkspaceAppHealth]struct{}{
	WorkspaceAppHealthDisabled:     {},
	WorkspaceAppHealthInitializing: {},
	WorkspaceAppHealthHealthy:      {},
	WorkspaceAppHealthUnhealthy:    {},
}

type WorkspaceAppSharingLevel string

const (
	WorkspaceAppSharingLevelOwner         WorkspaceAppSharingLevel = "owner"
	WorkspaceAppSharingLevelAuthenticated WorkspaceAppSharingLevel = "authenticated"
	WorkspaceAppSharingLevelPublic        WorkspaceAppSharingLevel = "public"
)

var MapWorkspaceAppSharingLevels = map[WorkspaceAppSharingLevel]struct{}{
	WorkspaceAppSharingLevelOwner:         {},
	WorkspaceAppSharingLevelAuthenticated: {},
	WorkspaceAppSharingLevelPublic:        {},
}

type WorkspaceApp struct {
	ID uuid.UUID `json:"id" format:"uuid"`
	// URL is the address being proxied to inside the workspace.
	// If external is specified, this will be opened on the client.
	URL string `json:"url"`
	// External specifies whether the URL should be opened externally on
	// the client or not.
	External bool `json:"external"`
	// Slug is a unique identifier within the agent.
	Slug string `json:"slug"`
	// DisplayName is a friendly name for the app.
	DisplayName string `json:"display_name"`
	Command     string `json:"command,omitempty"`
	// Icon is a relative path or external URL that specifies
	// an icon to be displayed in the dashboard.
	Icon string `json:"icon,omitempty"`
	// Subdomain denotes whether the app should be accessed via a path on the
	// `coder server` or via a hostname-based dev URL. If this is set to true
	// and there is no app wildcard configured on the server, the app will not
	// be accessible in the UI.
	Subdomain bool `json:"subdomain"`
	// SubdomainName is the application domain exposed on the `coder server`.
	SubdomainName string                   `json:"subdomain_name,omitempty"`
	SharingLevel  WorkspaceAppSharingLevel `json:"sharing_level" enums:"owner,authenticated,public"`
	// Healthcheck specifies the configuration for checking app health.
	Healthcheck Healthcheck        `json:"healthcheck"`
	Health      WorkspaceAppHealth `json:"health"`
}

type Healthcheck struct {
	// URL specifies the endpoint to check for the app health.
	URL string `json:"url"`
	// Interval specifies the seconds between each health check.
	Interval int32 `json:"interval"`
	// Threshold specifies the number of consecutive failed health checks before returning "unhealthy".
	Threshold int32 `json:"threshold"`
}

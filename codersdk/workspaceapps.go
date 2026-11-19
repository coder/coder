package codersdk

import (
	"time"

	"github.com/google/uuid"
)

type WorkspaceAppHealth string

const (
	WorkspaceAppHealthDisabled     WorkspaceAppHealth = "disabled"
	WorkspaceAppHealthInitializing WorkspaceAppHealth = "initializing"
	WorkspaceAppHealthHealthy      WorkspaceAppHealth = "healthy"
	WorkspaceAppHealthUnhealthy    WorkspaceAppHealth = "unhealthy"
)

type WorkspaceAppStatusState string

const (
	WorkspaceAppStatusStateWorking  WorkspaceAppStatusState = "working"
	WorkspaceAppStatusStateIdle     WorkspaceAppStatusState = "idle"
	WorkspaceAppStatusStateComplete WorkspaceAppStatusState = "complete"
	WorkspaceAppStatusStateFailure  WorkspaceAppStatusState = "failure"
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
	WorkspaceAppSharingLevelOrganization  WorkspaceAppSharingLevel = "organization"
	WorkspaceAppSharingLevelPublic        WorkspaceAppSharingLevel = "public"
)

var MapWorkspaceAppSharingLevels = map[WorkspaceAppSharingLevel]struct{}{
	WorkspaceAppSharingLevelOwner:         {},
	WorkspaceAppSharingLevelAuthenticated: {},
	WorkspaceAppSharingLevelOrganization:  {},
	WorkspaceAppSharingLevelPublic:        {},
}

type WorkspaceAppOpenIn string

const (
	WorkspaceAppOpenInSlimWindow WorkspaceAppOpenIn = "slim-window"
	WorkspaceAppOpenInTab        WorkspaceAppOpenIn = "tab"
)

var MapWorkspaceAppOpenIns = map[WorkspaceAppOpenIn]struct{}{
	WorkspaceAppOpenInSlimWindow: {},
	WorkspaceAppOpenInTab:        {},
}

type WorkspaceApp struct {
	ID uuid.UUID `json:"id" format:"uuid" wsrelated:"latest_build.resources.agents.apps"`
	// URL is the address being proxied to inside the workspace.
	// If external is specified, this will be opened on the client.
	URL string `json:"url,omitempty" wsrelated:"latest_build.resources.agents.apps"`
	// External specifies whether the URL should be opened externally on
	// the client or not.
	External bool `json:"external" wsrelated:"latest_build.resources.agents.apps"`
	// Slug is a unique identifier within the agent.
	Slug string `json:"slug" wsrelated:"latest_build.resources.agents.apps"`
	// DisplayName is a friendly name for the app.
	DisplayName string `json:"display_name,omitempty" wsrelated:"latest_build.resources.agents.apps"`
	Command     string `json:"command,omitempty" wsrelated:"latest_build.resources.agents.apps"`
	// Icon is a relative path or external URL that specifies
	// an icon to be displayed in the dashboard.
	Icon string `json:"icon,omitempty" wsrelated:"latest_build.resources.agents.apps"`
	// Subdomain denotes whether the app should be accessed via a path on the
	// `coder server` or via a hostname-based dev URL. If this is set to true
	// and there is no app wildcard configured on the server, the app will not
	// be accessible in the UI.
	Subdomain bool `json:"subdomain" wsrelated:"latest_build.resources.agents.apps"`
	// SubdomainName is the application domain exposed on the `coder server`.
	SubdomainName string                   `json:"subdomain_name,omitempty" wsrelated:"latest_build.resources.agents.apps"`
	SharingLevel  WorkspaceAppSharingLevel `json:"sharing_level" wsrelated:"latest_build.resources.agents.apps"`
	// Healthcheck specifies the configuration for checking app health.
	Healthcheck Healthcheck        `json:"healthcheck,omitempty" wsrelated:"latest_build.resources.agents.apps"`
	Health      WorkspaceAppHealth `json:"health" wsrelated:"latest_build.resources.agents.apps"`
	Group       string             `json:"group,omitempty" wsrelated:"latest_build.resources.agents.apps"`
	Hidden      bool               `json:"hidden" wsrelated:"latest_build.resources.agents.apps"`
	OpenIn      WorkspaceAppOpenIn `json:"open_in" wsrelated:"latest_build.resources.agents.apps"`
	// Tooltip is an optional markdown supported field that is displayed
	// when hovering over workspace apps in the UI.
	Tooltip string `json:"tooltip,omitempty" wsrelated:"latest_build.resources.agents.apps"`

	// Statuses is a list of statuses for the app.
	Statuses []WorkspaceAppStatus `json:"statuses" wsrelated:"latest_build.resources.agents.apps.statuses"`
}

type Healthcheck struct {
	// URL specifies the endpoint to check for the app health.
	URL string `json:"url"`
	// Interval specifies the seconds between each health check.
	Interval int32 `json:"interval"`
	// Threshold specifies the number of consecutive failed health checks before returning "unhealthy".
	Threshold int32 `json:"threshold"`
}

type WorkspaceAppStatus struct {
	ID          uuid.UUID               `json:"id" format:"uuid" wsrelated:"latest_build.resources.agents.apps.statuses"`
	CreatedAt   time.Time               `json:"created_at" format:"date-time" wsrelated:"latest_build.resources.agents.apps.statuses"`
	WorkspaceID uuid.UUID               `json:"workspace_id" format:"uuid" wsrelated:"latest_build.resources.agents.apps.statuses"`
	AgentID     uuid.UUID               `json:"agent_id" format:"uuid" wsrelated:"latest_build.resources.agents.apps.statuses"`
	AppID       uuid.UUID               `json:"app_id" format:"uuid" wsrelated:"latest_build.resources.agents.apps.statuses"`
	State       WorkspaceAppStatusState `json:"state" wsrelated:"latest_build.resources.agents.apps.statuses"`
	Message     string                  `json:"message" wsrelated:"latest_build.resources.agents.apps.statuses"`
	// URI is the URI of the resource that the status is for.
	// e.g. https://github.com/org/repo/pull/123
	// e.g. file:///path/to/file
	URI string `json:"uri" wsrelated:"latest_build.resources.agents.apps.statuses"`

	// Deprecated: This field is unused and will be removed in a future version.
	// Icon is an external URL to an icon that will be rendered in the UI.
	Icon string `json:"icon"`
	// Deprecated: This field is unused and will be removed in a future version.
	// NeedsUserAttention specifies whether the status needs user attention.
	NeedsUserAttention bool `json:"needs_user_attention"`
}

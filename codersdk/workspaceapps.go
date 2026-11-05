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

// VSCodeCoderExtensionID is the Coder VS Code extension, the authority of its
// deep links such as `vscode://coder.coder-remote/open?...`.
const VSCodeCoderExtensionID = "coder.coder-remote"

// VSCodeSessionTokenScheme is a VS Code-family scheme whose Coder extension
// links, `<scheme>://` + VSCodeCoderExtensionID + a VSCodeSessionTokenPath, may
// receive the user's session token in place of `$SESSION_TOKEN` in an
// external app URL. The dashboard and `coder open app` both use this list, with
// JetBrainsSessionTokenHandler.
type VSCodeSessionTokenScheme string

//nolint:gosec // G101: URL schemes, not credentials.
const (
	VSCodeSessionTokenSchemeVSCode         VSCodeSessionTokenScheme = "vscode"
	VSCodeSessionTokenSchemeVSCodeInsiders VSCodeSessionTokenScheme = "vscode-insiders"
	VSCodeSessionTokenSchemeWindsurf       VSCodeSessionTokenScheme = "windsurf"
	VSCodeSessionTokenSchemeDevin          VSCodeSessionTokenScheme = "devin"
	VSCodeSessionTokenSchemeCursor         VSCodeSessionTokenScheme = "cursor"
	VSCodeSessionTokenSchemeKiro           VSCodeSessionTokenScheme = "kiro"
	VSCodeSessionTokenSchemePositron       VSCodeSessionTokenScheme = "positron"
	VSCodeSessionTokenSchemeAntigravity    VSCodeSessionTokenScheme = "antigravity"
	VSCodeSessionTokenSchemeAntigravityIDE VSCodeSessionTokenScheme = "antigravity-ide"
)

// VSCodeSessionTokenSchemes lists every VSCodeSessionTokenScheme.
var VSCodeSessionTokenSchemes = []VSCodeSessionTokenScheme{
	VSCodeSessionTokenSchemeVSCode,
	VSCodeSessionTokenSchemeVSCodeInsiders,
	VSCodeSessionTokenSchemeWindsurf,
	VSCodeSessionTokenSchemeDevin,
	VSCodeSessionTokenSchemeCursor,
	VSCodeSessionTokenSchemeKiro,
	VSCodeSessionTokenSchemePositron,
	VSCodeSessionTokenSchemeAntigravity,
	VSCodeSessionTokenSchemeAntigravityIDE,
}

// VSCodeSessionTokenPath is a Coder VS Code extension route that takes a
// session token, through the start of its query.
type VSCodeSessionTokenPath string

//nolint:gosec // G101: URL paths, not credentials.
const (
	VSCodeSessionTokenPathOpen             VSCodeSessionTokenPath = "/open?"
	VSCodeSessionTokenPathOpenDevContainer VSCodeSessionTokenPath = "/openDevContainer?"
)

// VSCodeSessionTokenPaths lists every VSCodeSessionTokenPath.
var VSCodeSessionTokenPaths = []VSCodeSessionTokenPath{
	VSCodeSessionTokenPathOpen,
	VSCodeSessionTokenPathOpenDevContainer,
}

// JetBrainsSessionTokenHandler is a JetBrains deep-link prefix that may receive
// the session token. The scheme and authority match ASCII case-insensitively,
// the rest exactly.
type JetBrainsSessionTokenHandler string

//nolint:gosec // G101: link prefixes, not credentials.
const (
	JetBrainsSessionTokenHandlerToolbox      JetBrainsSessionTokenHandler = "jetbrains://gateway/coder?"
	JetBrainsSessionTokenHandlerToolboxCoder JetBrainsSessionTokenHandler = "jetbrains://gateway/com.coder.toolbox?"
	// Gateway also handles non-Coder providers, such as `type=ssh`.
	JetBrainsSessionTokenHandlerGateway JetBrainsSessionTokenHandler = "jetbrains-gateway://connect#type=coder&"
)

// JetBrainsSessionTokenHandlers lists every JetBrainsSessionTokenHandler.
var JetBrainsSessionTokenHandlers = []JetBrainsSessionTokenHandler{
	JetBrainsSessionTokenHandlerToolbox,
	JetBrainsSessionTokenHandlerToolboxCoder,
	JetBrainsSessionTokenHandlerGateway,
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
	ID uuid.UUID `json:"id" format:"uuid"`
	// URL is the address being proxied to inside the workspace.
	// If external is specified, this will be opened on the client.
	URL string `json:"url,omitempty"`
	// External specifies whether the URL should be opened externally on
	// the client or not.
	External bool `json:"external"`
	// Slug is a unique identifier within the agent.
	Slug string `json:"slug"`
	// DisplayName is a friendly name for the app.
	DisplayName string `json:"display_name,omitempty"`
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
	SharingLevel  WorkspaceAppSharingLevel `json:"sharing_level" enums:"owner,authenticated,organization,public"`
	// Healthcheck specifies the configuration for checking app health.
	Healthcheck Healthcheck        `json:"healthcheck,omitempty"`
	Health      WorkspaceAppHealth `json:"health"`
	Group       string             `json:"group,omitempty"`
	Hidden      bool               `json:"hidden"`
	OpenIn      WorkspaceAppOpenIn `json:"open_in"`
	// Tooltip is an optional markdown supported field that is displayed
	// when hovering over workspace apps in the UI.
	Tooltip string `json:"tooltip,omitempty"`

	// Statuses is a list of statuses for the app.
	Statuses []WorkspaceAppStatus `json:"statuses"`
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
	ID          uuid.UUID               `json:"id" format:"uuid"`
	CreatedAt   time.Time               `json:"created_at" format:"date-time"`
	WorkspaceID uuid.UUID               `json:"workspace_id" format:"uuid"`
	AgentID     uuid.UUID               `json:"agent_id" format:"uuid"`
	AppID       uuid.UUID               `json:"app_id" format:"uuid"`
	State       WorkspaceAppStatusState `json:"state"`
	Message     string                  `json:"message"`
	// URI is the URI of the resource that the status is for.
	// e.g. https://github.com/org/repo/pull/123
	// e.g. file:///path/to/file
	URI string `json:"uri"`

	// Deprecated: This field is unused and will be removed in a future version.
	// Icon is an external URL to an icon that will be rendered in the UI.
	Icon string `json:"icon"`
	// Deprecated: This field is unused and will be removed in a future version.
	// NeedsUserAttention specifies whether the status needs user attention.
	NeedsUserAttention bool `json:"needs_user_attention"`
}

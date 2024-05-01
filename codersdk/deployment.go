package codersdk

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/mod/semver"
	"golang.org/x/xerrors"

	"github.com/coreos/go-oidc/v3/oidc"

	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/coderd/agentmetrics"
	"github.com/coder/coder/v2/coderd/workspaceapps/appurl"
	"github.com/coder/serpent"
)

// Entitlement represents whether a feature is licensed.
type Entitlement string

const (
	EntitlementEntitled    Entitlement = "entitled"
	EntitlementGracePeriod Entitlement = "grace_period"
	EntitlementNotEntitled Entitlement = "not_entitled"
)

// FeatureName represents the internal name of a feature.
// To add a new feature, add it to this set of enums as well as the FeatureNames
// array below.
type FeatureName string

const (
	FeatureUserLimit                  FeatureName = "user_limit"
	FeatureAuditLog                   FeatureName = "audit_log"
	FeatureBrowserOnly                FeatureName = "browser_only"
	FeatureSCIM                       FeatureName = "scim"
	FeatureTemplateRBAC               FeatureName = "template_rbac"
	FeatureUserRoleManagement         FeatureName = "user_role_management"
	FeatureHighAvailability           FeatureName = "high_availability"
	FeatureMultipleExternalAuth       FeatureName = "multiple_external_auth"
	FeatureExternalProvisionerDaemons FeatureName = "external_provisioner_daemons"
	FeatureAppearance                 FeatureName = "appearance"
	FeatureAdvancedTemplateScheduling FeatureName = "advanced_template_scheduling"
	FeatureWorkspaceProxy             FeatureName = "workspace_proxy"
	FeatureExternalTokenEncryption    FeatureName = "external_token_encryption"
	FeatureWorkspaceBatchActions      FeatureName = "workspace_batch_actions"
	FeatureAccessControl              FeatureName = "access_control"
	FeatureControlSharedPorts         FeatureName = "control_shared_ports"
)

// FeatureNames must be kept in-sync with the Feature enum above.
var FeatureNames = []FeatureName{
	FeatureUserLimit,
	FeatureAuditLog,
	FeatureBrowserOnly,
	FeatureSCIM,
	FeatureTemplateRBAC,
	FeatureHighAvailability,
	FeatureMultipleExternalAuth,
	FeatureExternalProvisionerDaemons,
	FeatureAppearance,
	FeatureAdvancedTemplateScheduling,
	FeatureWorkspaceProxy,
	FeatureUserRoleManagement,
	FeatureExternalTokenEncryption,
	FeatureWorkspaceBatchActions,
	FeatureAccessControl,
	FeatureControlSharedPorts,
}

// Humanize returns the feature name in a human-readable format.
func (n FeatureName) Humanize() string {
	switch n {
	case FeatureTemplateRBAC:
		return "Template RBAC"
	case FeatureSCIM:
		return "SCIM"
	default:
		return strings.Title(strings.ReplaceAll(string(n), "_", " "))
	}
}

// AlwaysEnable returns if the feature is always enabled if entitled.
// Warning: We don't know if we need this functionality.
// This method may disappear at any time.
func (n FeatureName) AlwaysEnable() bool {
	return map[FeatureName]bool{
		FeatureMultipleExternalAuth:       true,
		FeatureExternalProvisionerDaemons: true,
		FeatureAppearance:                 true,
		FeatureWorkspaceBatchActions:      true,
		FeatureHighAvailability:           true,
	}[n]
}

type Feature struct {
	Entitlement Entitlement `json:"entitlement"`
	Enabled     bool        `json:"enabled"`
	Limit       *int64      `json:"limit,omitempty"`
	Actual      *int64      `json:"actual,omitempty"`
}

type Entitlements struct {
	Features         map[FeatureName]Feature `json:"features"`
	Warnings         []string                `json:"warnings"`
	Errors           []string                `json:"errors"`
	HasLicense       bool                    `json:"has_license"`
	Trial            bool                    `json:"trial"`
	RequireTelemetry bool                    `json:"require_telemetry"`
	RefreshedAt      time.Time               `json:"refreshed_at" format:"date-time"`
}

func (c *Client) Entitlements(ctx context.Context) (Entitlements, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/entitlements", nil)
	if err != nil {
		return Entitlements{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Entitlements{}, ReadBodyAsError(res)
	}
	var ent Entitlements
	return ent, json.NewDecoder(res.Body).Decode(&ent)
}

type PostgresAuth string

const (
	PostgresAuthPassword  PostgresAuth = "password"
	PostgresAuthAWSIAMRDS PostgresAuth = "awsiamrds"
)

var PostgresAuthDrivers = []string{
	string(PostgresAuthPassword),
	string(PostgresAuthAWSIAMRDS),
}

// DeploymentValues is the central configuration values the coder server.
type DeploymentValues struct {
	Verbose             serpent.Bool   `json:"verbose,omitempty"`
	AccessURL           serpent.URL    `json:"access_url,omitempty"`
	WildcardAccessURL   serpent.String `json:"wildcard_access_url,omitempty"`
	DocsURL             serpent.URL    `json:"docs_url,omitempty"`
	RedirectToAccessURL serpent.Bool   `json:"redirect_to_access_url,omitempty"`
	// HTTPAddress is a string because it may be set to zero to disable.
	HTTPAddress                     serpent.String                       `json:"http_address,omitempty" typescript:",notnull"`
	AutobuildPollInterval           serpent.Duration                     `json:"autobuild_poll_interval,omitempty"`
	JobHangDetectorInterval         serpent.Duration                     `json:"job_hang_detector_interval,omitempty"`
	DERP                            DERP                                 `json:"derp,omitempty" typescript:",notnull"`
	Prometheus                      PrometheusConfig                     `json:"prometheus,omitempty" typescript:",notnull"`
	Pprof                           PprofConfig                          `json:"pprof,omitempty" typescript:",notnull"`
	ProxyTrustedHeaders             serpent.StringArray                  `json:"proxy_trusted_headers,omitempty" typescript:",notnull"`
	ProxyTrustedOrigins             serpent.StringArray                  `json:"proxy_trusted_origins,omitempty" typescript:",notnull"`
	CacheDir                        serpent.String                       `json:"cache_directory,omitempty" typescript:",notnull"`
	InMemoryDatabase                serpent.Bool                         `json:"in_memory_database,omitempty" typescript:",notnull"`
	PostgresURL                     serpent.String                       `json:"pg_connection_url,omitempty" typescript:",notnull"`
	PostgresAuth                    string                               `json:"pg_auth,omitempty" typescript:",notnull"`
	OAuth2                          OAuth2Config                         `json:"oauth2,omitempty" typescript:",notnull"`
	OIDC                            OIDCConfig                           `json:"oidc,omitempty" typescript:",notnull"`
	Telemetry                       TelemetryConfig                      `json:"telemetry,omitempty" typescript:",notnull"`
	TLS                             TLSConfig                            `json:"tls,omitempty" typescript:",notnull"`
	Trace                           TraceConfig                          `json:"trace,omitempty" typescript:",notnull"`
	SecureAuthCookie                serpent.Bool                         `json:"secure_auth_cookie,omitempty" typescript:",notnull"`
	StrictTransportSecurity         serpent.Int64                        `json:"strict_transport_security,omitempty" typescript:",notnull"`
	StrictTransportSecurityOptions  serpent.StringArray                  `json:"strict_transport_security_options,omitempty" typescript:",notnull"`
	SSHKeygenAlgorithm              serpent.String                       `json:"ssh_keygen_algorithm,omitempty" typescript:",notnull"`
	MetricsCacheRefreshInterval     serpent.Duration                     `json:"metrics_cache_refresh_interval,omitempty" typescript:",notnull"`
	AgentStatRefreshInterval        serpent.Duration                     `json:"agent_stat_refresh_interval,omitempty" typescript:",notnull"`
	AgentFallbackTroubleshootingURL serpent.URL                          `json:"agent_fallback_troubleshooting_url,omitempty" typescript:",notnull"`
	BrowserOnly                     serpent.Bool                         `json:"browser_only,omitempty" typescript:",notnull"`
	SCIMAPIKey                      serpent.String                       `json:"scim_api_key,omitempty" typescript:",notnull"`
	ExternalTokenEncryptionKeys     serpent.StringArray                  `json:"external_token_encryption_keys,omitempty" typescript:",notnull"`
	Provisioner                     ProvisionerConfig                    `json:"provisioner,omitempty" typescript:",notnull"`
	RateLimit                       RateLimitConfig                      `json:"rate_limit,omitempty" typescript:",notnull"`
	Experiments                     serpent.StringArray                  `json:"experiments,omitempty" typescript:",notnull"`
	UpdateCheck                     serpent.Bool                         `json:"update_check,omitempty" typescript:",notnull"`
	Swagger                         SwaggerConfig                        `json:"swagger,omitempty" typescript:",notnull"`
	Logging                         LoggingConfig                        `json:"logging,omitempty" typescript:",notnull"`
	Dangerous                       DangerousConfig                      `json:"dangerous,omitempty" typescript:",notnull"`
	DisablePathApps                 serpent.Bool                         `json:"disable_path_apps,omitempty" typescript:",notnull"`
	Sessions                        SessionLifetime                      `json:"session_lifetime,omitempty" typescript:",notnull"`
	DisablePasswordAuth             serpent.Bool                         `json:"disable_password_auth,omitempty" typescript:",notnull"`
	Support                         SupportConfig                        `json:"support,omitempty" typescript:",notnull"`
	ExternalAuthConfigs             serpent.Struct[[]ExternalAuthConfig] `json:"external_auth,omitempty" typescript:",notnull"`
	SSHConfig                       SSHConfig                            `json:"config_ssh,omitempty" typescript:",notnull"`
	WgtunnelHost                    serpent.String                       `json:"wgtunnel_host,omitempty" typescript:",notnull"`
	DisableOwnerWorkspaceExec       serpent.Bool                         `json:"disable_owner_workspace_exec,omitempty" typescript:",notnull"`
	ProxyHealthStatusInterval       serpent.Duration                     `json:"proxy_health_status_interval,omitempty" typescript:",notnull"`
	EnableTerraformDebugMode        serpent.Bool                         `json:"enable_terraform_debug_mode,omitempty" typescript:",notnull"`
	UserQuietHoursSchedule          UserQuietHoursScheduleConfig         `json:"user_quiet_hours_schedule,omitempty" typescript:",notnull"`
	WebTerminalRenderer             serpent.String                       `json:"web_terminal_renderer,omitempty" typescript:",notnull"`
	AllowWorkspaceRenames           serpent.Bool                         `json:"allow_workspace_renames,omitempty" typescript:",notnull"`
	Healthcheck                     HealthcheckConfig                    `json:"healthcheck,omitempty" typescript:",notnull"`
	CLIUpgradeMessage               serpent.String                       `json:"cli_upgrade_message,omitempty" typescript:",notnull"`
	TermsOfServiceURL               serpent.String                       `json:"terms_of_service_url,omitempty" typescript:",notnull"`

	Config      serpent.YAMLConfigPath `json:"config,omitempty" typescript:",notnull"`
	WriteConfig serpent.Bool           `json:"write_config,omitempty" typescript:",notnull"`

	// DEPRECATED: Use HTTPAddress or TLS.Address instead.
	Address serpent.HostPort `json:"address,omitempty" typescript:",notnull"`
}

// SSHConfig is configuration the cli & vscode extension use for configuring
// ssh connections.
type SSHConfig struct {
	// DeploymentName is the config-ssh Hostname prefix
	DeploymentName serpent.String
	// SSHConfigOptions are additional options to add to the ssh config file.
	// This will override defaults.
	SSHConfigOptions serpent.StringArray
}

func (c SSHConfig) ParseOptions() (map[string]string, error) {
	m := make(map[string]string)
	for _, opt := range c.SSHConfigOptions {
		key, value, err := ParseSSHConfigOption(opt)
		if err != nil {
			return nil, err
		}
		m[key] = value
	}
	return m, nil
}

// ParseSSHConfigOption parses a single ssh config option into it's key/value pair.
func ParseSSHConfigOption(opt string) (key string, value string, err error) {
	// An equal sign or whitespace is the separator between the key and value.
	idx := strings.IndexFunc(opt, func(r rune) bool {
		return r == ' ' || r == '='
	})
	if idx == -1 {
		return "", "", xerrors.Errorf("invalid config-ssh option %q", opt)
	}
	return opt[:idx], opt[idx+1:], nil
}

// SessionLifetime refers to "sessions" authenticating into Coderd. Coder has
// multiple different session types: api keys, tokens, workspace app tokens,
// agent tokens, etc. This configuration struct should be used to group all
// settings referring to any of these session lifetime controls.
// TODO: These config options were created back when coder only had api keys.
// Today, the config is ambigously used for all of them. For example:
// - cli based api keys ignore all settings
// - login uses the default lifetime, not the MaximumTokenDuration
// - Tokens use the Default & MaximumTokenDuration
// - ... etc ...
// The rational behind each decision is undocumented. The naming behind these
// config options is also confusing without any clear documentation.
// 'CreateAPIKey' is used to make all sessions, and it's parameters are just
// 'LifetimeSeconds' and 'DefaultLifetime'. Which does not directly correlate to
// the config options here.
type SessionLifetime struct {
	// DisableExpiryRefresh will disable automatically refreshing api
	// keys when they are used from the api. This means the api key lifetime at
	// creation is the lifetime of the api key.
	DisableExpiryRefresh serpent.Bool `json:"disable_expiry_refresh,omitempty" typescript:",notnull"`

	// DefaultDuration is for api keys, not tokens.
	DefaultDuration serpent.Duration `json:"default_duration" typescript:",notnull"`

	MaximumTokenDuration serpent.Duration `json:"max_token_lifetime,omitempty" typescript:",notnull"`
}

type DERP struct {
	Server DERPServerConfig `json:"server" typescript:",notnull"`
	Config DERPConfig       `json:"config" typescript:",notnull"`
}

type DERPServerConfig struct {
	Enable        serpent.Bool        `json:"enable" typescript:",notnull"`
	RegionID      serpent.Int64       `json:"region_id" typescript:",notnull"`
	RegionCode    serpent.String      `json:"region_code" typescript:",notnull"`
	RegionName    serpent.String      `json:"region_name" typescript:",notnull"`
	STUNAddresses serpent.StringArray `json:"stun_addresses" typescript:",notnull"`
	RelayURL      serpent.URL         `json:"relay_url" typescript:",notnull"`
}

type DERPConfig struct {
	BlockDirect     serpent.Bool   `json:"block_direct" typescript:",notnull"`
	ForceWebSockets serpent.Bool   `json:"force_websockets" typescript:",notnull"`
	URL             serpent.String `json:"url" typescript:",notnull"`
	Path            serpent.String `json:"path" typescript:",notnull"`
}

type PrometheusConfig struct {
	Enable                serpent.Bool        `json:"enable" typescript:",notnull"`
	Address               serpent.HostPort    `json:"address" typescript:",notnull"`
	CollectAgentStats     serpent.Bool        `json:"collect_agent_stats" typescript:",notnull"`
	CollectDBMetrics      serpent.Bool        `json:"collect_db_metrics" typescript:",notnull"`
	AggregateAgentStatsBy serpent.StringArray `json:"aggregate_agent_stats_by" typescript:",notnull"`
}

type PprofConfig struct {
	Enable  serpent.Bool     `json:"enable" typescript:",notnull"`
	Address serpent.HostPort `json:"address" typescript:",notnull"`
}

type OAuth2Config struct {
	Github OAuth2GithubConfig `json:"github" typescript:",notnull"`
}

type OAuth2GithubConfig struct {
	ClientID          serpent.String      `json:"client_id" typescript:",notnull"`
	ClientSecret      serpent.String      `json:"client_secret" typescript:",notnull"`
	AllowedOrgs       serpent.StringArray `json:"allowed_orgs" typescript:",notnull"`
	AllowedTeams      serpent.StringArray `json:"allowed_teams" typescript:",notnull"`
	AllowSignups      serpent.Bool        `json:"allow_signups" typescript:",notnull"`
	AllowEveryone     serpent.Bool        `json:"allow_everyone" typescript:",notnull"`
	EnterpriseBaseURL serpent.String      `json:"enterprise_base_url" typescript:",notnull"`
}

type OIDCConfig struct {
	AllowSignups serpent.Bool   `json:"allow_signups" typescript:",notnull"`
	ClientID     serpent.String `json:"client_id" typescript:",notnull"`
	ClientSecret serpent.String `json:"client_secret" typescript:",notnull"`
	// ClientKeyFile & ClientCertFile are used in place of ClientSecret for PKI auth.
	ClientKeyFile       serpent.String                      `json:"client_key_file" typescript:",notnull"`
	ClientCertFile      serpent.String                      `json:"client_cert_file" typescript:",notnull"`
	EmailDomain         serpent.StringArray                 `json:"email_domain" typescript:",notnull"`
	IssuerURL           serpent.String                      `json:"issuer_url" typescript:",notnull"`
	Scopes              serpent.StringArray                 `json:"scopes" typescript:",notnull"`
	IgnoreEmailVerified serpent.Bool                        `json:"ignore_email_verified" typescript:",notnull"`
	UsernameField       serpent.String                      `json:"username_field" typescript:",notnull"`
	EmailField          serpent.String                      `json:"email_field" typescript:",notnull"`
	AuthURLParams       serpent.Struct[map[string]string]   `json:"auth_url_params" typescript:",notnull"`
	IgnoreUserInfo      serpent.Bool                        `json:"ignore_user_info" typescript:",notnull"`
	GroupAutoCreate     serpent.Bool                        `json:"group_auto_create" typescript:",notnull"`
	GroupRegexFilter    serpent.Regexp                      `json:"group_regex_filter" typescript:",notnull"`
	GroupAllowList      serpent.StringArray                 `json:"group_allow_list" typescript:",notnull"`
	GroupField          serpent.String                      `json:"groups_field" typescript:",notnull"`
	GroupMapping        serpent.Struct[map[string]string]   `json:"group_mapping" typescript:",notnull"`
	UserRoleField       serpent.String                      `json:"user_role_field" typescript:",notnull"`
	UserRoleMapping     serpent.Struct[map[string][]string] `json:"user_role_mapping" typescript:",notnull"`
	UserRolesDefault    serpent.StringArray                 `json:"user_roles_default" typescript:",notnull"`
	SignInText          serpent.String                      `json:"sign_in_text" typescript:",notnull"`
	IconURL             serpent.URL                         `json:"icon_url" typescript:",notnull"`
	SignupsDisabledText serpent.String                      `json:"signups_disabled_text" typescript:",notnull"`
}

type TelemetryConfig struct {
	Enable serpent.Bool `json:"enable" typescript:",notnull"`
	Trace  serpent.Bool `json:"trace" typescript:",notnull"`
	URL    serpent.URL  `json:"url" typescript:",notnull"`
}

type TLSConfig struct {
	Enable               serpent.Bool        `json:"enable" typescript:",notnull"`
	Address              serpent.HostPort    `json:"address" typescript:",notnull"`
	RedirectHTTP         serpent.Bool        `json:"redirect_http" typescript:",notnull"`
	CertFiles            serpent.StringArray `json:"cert_file" typescript:",notnull"`
	ClientAuth           serpent.String      `json:"client_auth" typescript:",notnull"`
	ClientCAFile         serpent.String      `json:"client_ca_file" typescript:",notnull"`
	KeyFiles             serpent.StringArray `json:"key_file" typescript:",notnull"`
	MinVersion           serpent.String      `json:"min_version" typescript:",notnull"`
	ClientCertFile       serpent.String      `json:"client_cert_file" typescript:",notnull"`
	ClientKeyFile        serpent.String      `json:"client_key_file" typescript:",notnull"`
	SupportedCiphers     serpent.StringArray `json:"supported_ciphers" typescript:",notnull"`
	AllowInsecureCiphers serpent.Bool        `json:"allow_insecure_ciphers" typescript:",notnull"`
}

type TraceConfig struct {
	Enable          serpent.Bool   `json:"enable" typescript:",notnull"`
	HoneycombAPIKey serpent.String `json:"honeycomb_api_key" typescript:",notnull"`
	CaptureLogs     serpent.Bool   `json:"capture_logs" typescript:",notnull"`
	DataDog         serpent.Bool   `json:"data_dog" typescript:",notnull"`
}

type ExternalAuthConfig struct {
	// Type is the type of external auth config.
	Type         string `json:"type" yaml:"type"`
	ClientID     string `json:"client_id" yaml:"client_id"`
	ClientSecret string `json:"-" yaml:"client_secret"`
	// ID is a unique identifier for the auth config.
	// It defaults to `type` when not provided.
	ID                  string   `json:"id" yaml:"id"`
	AuthURL             string   `json:"auth_url" yaml:"auth_url"`
	TokenURL            string   `json:"token_url" yaml:"token_url"`
	ValidateURL         string   `json:"validate_url" yaml:"validate_url"`
	AppInstallURL       string   `json:"app_install_url" yaml:"app_install_url"`
	AppInstallationsURL string   `json:"app_installations_url" yaml:"app_installations_url"`
	NoRefresh           bool     `json:"no_refresh" yaml:"no_refresh"`
	Scopes              []string `json:"scopes" yaml:"scopes"`
	ExtraTokenKeys      []string `json:"extra_token_keys" yaml:"extra_token_keys"`
	DeviceFlow          bool     `json:"device_flow" yaml:"device_flow"`
	DeviceCodeURL       string   `json:"device_code_url" yaml:"device_code_url"`
	// Regex allows API requesters to match an auth config by
	// a string (e.g. coder.com) instead of by it's type.
	//
	// Git clone makes use of this by parsing the URL from:
	// 'Username for "https://github.com":'
	// And sending it to the Coder server to match against the Regex.
	Regex string `json:"regex" yaml:"regex"`
	// DisplayName is shown in the UI to identify the auth config.
	DisplayName string `json:"display_name" yaml:"display_name"`
	// DisplayIcon is a URL to an icon to display in the UI.
	DisplayIcon string `json:"display_icon" yaml:"display_icon"`
}

type ProvisionerConfig struct {
	Daemons             serpent.Int64    `json:"daemons" typescript:",notnull"`
	DaemonsEcho         serpent.Bool     `json:"daemons_echo" typescript:",notnull"`
	DaemonPollInterval  serpent.Duration `json:"daemon_poll_interval" typescript:",notnull"`
	DaemonPollJitter    serpent.Duration `json:"daemon_poll_jitter" typescript:",notnull"`
	ForceCancelInterval serpent.Duration `json:"force_cancel_interval" typescript:",notnull"`
	DaemonPSK           serpent.String   `json:"daemon_psk" typescript:",notnull"`
}

type RateLimitConfig struct {
	DisableAll serpent.Bool  `json:"disable_all" typescript:",notnull"`
	API        serpent.Int64 `json:"api" typescript:",notnull"`
}

type SwaggerConfig struct {
	Enable serpent.Bool `json:"enable" typescript:",notnull"`
}

type LoggingConfig struct {
	Filter      serpent.StringArray `json:"log_filter" typescript:",notnull"`
	Human       serpent.String      `json:"human" typescript:",notnull"`
	JSON        serpent.String      `json:"json" typescript:",notnull"`
	Stackdriver serpent.String      `json:"stackdriver" typescript:",notnull"`
}

type DangerousConfig struct {
	AllowPathAppSharing         serpent.Bool `json:"allow_path_app_sharing" typescript:",notnull"`
	AllowPathAppSiteOwnerAccess serpent.Bool `json:"allow_path_app_site_owner_access" typescript:",notnull"`
	AllowAllCors                serpent.Bool `json:"allow_all_cors" typescript:",notnull"`
}

type UserQuietHoursScheduleConfig struct {
	DefaultSchedule serpent.String `json:"default_schedule" typescript:",notnull"`
	AllowUserCustom serpent.Bool   `json:"allow_user_custom" typescript:",notnull"`
	// TODO: add WindowDuration and the ability to postpone max_deadline by this
	// amount
	// WindowDuration  serpent.Duration `json:"window_duration" typescript:",notnull"`
}

// HealthcheckConfig contains configuration for healthchecks.
type HealthcheckConfig struct {
	Refresh           serpent.Duration `json:"refresh" typescript:",notnull"`
	ThresholdDatabase serpent.Duration `json:"threshold_database" typescript:",notnull"`
}

const (
	annotationFormatDuration = "format_duration"
	annotationEnterpriseKey  = "enterprise"
	annotationSecretKey      = "secret"
	// annotationExternalProxies is used to mark options that are used by workspace
	// proxies. This is used to filter out options that are not relevant.
	annotationExternalProxies = "external_workspace_proxies"
)

// IsWorkspaceProxies returns true if the cli option is used by workspace proxies.
func IsWorkspaceProxies(opt serpent.Option) bool {
	// If it is a bool, use the bool value.
	b, _ := strconv.ParseBool(opt.Annotations[annotationExternalProxies])
	return b
}

func IsSecretDeploymentOption(opt serpent.Option) bool {
	return opt.Annotations.IsSet(annotationSecretKey)
}

func DefaultCacheDir() string {
	defaultCacheDir, err := os.UserCacheDir()
	if err != nil {
		defaultCacheDir = os.TempDir()
	}
	if dir := os.Getenv("CACHE_DIRECTORY"); dir != "" {
		// For compatibility with systemd.
		defaultCacheDir = dir
	}
	if dir := os.Getenv("CLIDOCGEN_CACHE_DIRECTORY"); dir != "" {
		defaultCacheDir = dir
	}
	return filepath.Join(defaultCacheDir, "coder")
}

// DeploymentConfig contains both the deployment values and how they're set.
type DeploymentConfig struct {
	Values  *DeploymentValues `json:"config,omitempty"`
	Options serpent.OptionSet `json:"options,omitempty"`
}

func (c *DeploymentValues) Options() serpent.OptionSet {
	// The deploymentGroup variables are used to organize the myriad server options.
	var (
		deploymentGroupNetworking = serpent.Group{
			Name: "Networking",
			YAML: "networking",
		}
		deploymentGroupNetworkingTLS = serpent.Group{
			Parent: &deploymentGroupNetworking,
			Name:   "TLS",
			Description: `Configure TLS / HTTPS for your Coder deployment. If you're running
 Coder behind a TLS-terminating reverse proxy or are accessing Coder over a
 secure link, you can safely ignore these settings.`,
			YAML: "tls",
		}
		deploymentGroupNetworkingHTTP = serpent.Group{
			Parent: &deploymentGroupNetworking,
			Name:   "HTTP",
			YAML:   "http",
		}
		deploymentGroupNetworkingDERP = serpent.Group{
			Parent: &deploymentGroupNetworking,
			Name:   "DERP",
			Description: `Most Coder deployments never have to think about DERP because all connections
 between workspaces and users are peer-to-peer. However, when Coder cannot establish
 a peer to peer connection, Coder uses a distributed relay network backed by
 Tailscale and WireGuard.`,
			YAML: "derp",
		}
		deploymentGroupIntrospection = serpent.Group{
			Name:        "Introspection",
			Description: `Configure logging, tracing, and metrics exporting.`,
			YAML:        "introspection",
		}
		deploymentGroupIntrospectionPPROF = serpent.Group{
			Parent: &deploymentGroupIntrospection,
			Name:   "pprof",
			YAML:   "pprof",
		}
		deploymentGroupIntrospectionPrometheus = serpent.Group{
			Parent: &deploymentGroupIntrospection,
			Name:   "Prometheus",
			YAML:   "prometheus",
		}
		deploymentGroupIntrospectionTracing = serpent.Group{
			Parent: &deploymentGroupIntrospection,
			Name:   "Tracing",
			YAML:   "tracing",
		}
		deploymentGroupIntrospectionLogging = serpent.Group{
			Parent: &deploymentGroupIntrospection,
			Name:   "Logging",
			YAML:   "logging",
		}
		deploymentGroupIntrospectionHealthcheck = serpent.Group{
			Parent: &deploymentGroupIntrospection,
			Name:   "Health Check",
			YAML:   "healthcheck",
		}
		deploymentGroupOAuth2 = serpent.Group{
			Name:        "OAuth2",
			Description: `Configure login and user-provisioning with GitHub via oAuth2.`,
			YAML:        "oauth2",
		}
		deploymentGroupOAuth2GitHub = serpent.Group{
			Parent: &deploymentGroupOAuth2,
			Name:   "GitHub",
			YAML:   "github",
		}
		deploymentGroupOIDC = serpent.Group{
			Name: "OIDC",
			YAML: "oidc",
		}
		deploymentGroupTelemetry = serpent.Group{
			Name: "Telemetry",
			YAML: "telemetry",
			Description: `Telemetry is critical to our ability to improve Coder. We strip all personal
information before sending data to our servers. Please only disable telemetry
when required by your organization's security policy.`,
		}
		deploymentGroupProvisioning = serpent.Group{
			Name:        "Provisioning",
			Description: `Tune the behavior of the provisioner, which is responsible for creating, updating, and deleting workspace resources.`,
			YAML:        "provisioning",
		}
		deploymentGroupUserQuietHoursSchedule = serpent.Group{
			Name:        "User Quiet Hours Schedule",
			Description: "Allow users to set quiet hours schedules each day for workspaces to avoid workspaces stopping during the day due to template scheduling.",
			YAML:        "userQuietHoursSchedule",
		}
		deploymentGroupDangerous = serpent.Group{
			Name: "⚠️ Dangerous",
			YAML: "dangerous",
		}
		deploymentGroupClient = serpent.Group{
			Name: "Client",
			Description: "These options change the behavior of how clients interact with the Coder. " +
				"Clients include the coder cli, vs code extension, and the web UI.",
			YAML: "client",
		}
		deploymentGroupConfig = serpent.Group{
			Name:        "Config",
			Description: `Use a YAML configuration file when your server launch become unwieldy.`,
		}
	)

	httpAddress := serpent.Option{
		Name:        "HTTP Address",
		Description: "HTTP bind address of the server. Unset to disable the HTTP endpoint.",
		Flag:        "http-address",
		Env:         "CODER_HTTP_ADDRESS",
		Default:     "127.0.0.1:3000",
		Value:       &c.HTTPAddress,
		Group:       &deploymentGroupNetworkingHTTP,
		YAML:        "httpAddress",
		Annotations: serpent.Annotations{}.Mark(annotationExternalProxies, "true"),
	}
	tlsBindAddress := serpent.Option{
		Name:        "TLS Address",
		Description: "HTTPS bind address of the server.",
		Flag:        "tls-address",
		Env:         "CODER_TLS_ADDRESS",
		Default:     "127.0.0.1:3443",
		Value:       &c.TLS.Address,
		Group:       &deploymentGroupNetworkingTLS,
		YAML:        "address",
		Annotations: serpent.Annotations{}.Mark(annotationExternalProxies, "true"),
	}
	redirectToAccessURL := serpent.Option{
		Name:        "Redirect to Access URL",
		Description: "Specifies whether to redirect requests that do not match the access URL host.",
		Flag:        "redirect-to-access-url",
		Env:         "CODER_REDIRECT_TO_ACCESS_URL",
		Value:       &c.RedirectToAccessURL,
		Group:       &deploymentGroupNetworking,
		YAML:        "redirectToAccessURL",
	}
	logFilter := serpent.Option{
		Name:          "Log Filter",
		Description:   "Filter debug logs by matching against a given regex. Use .* to match all debug logs.",
		Flag:          "log-filter",
		FlagShorthand: "l",
		Env:           "CODER_LOG_FILTER",
		Value:         &c.Logging.Filter,
		Group:         &deploymentGroupIntrospectionLogging,
		YAML:          "filter",
	}
	opts := serpent.OptionSet{
		{
			Name:        "Access URL",
			Description: `The URL that users will use to access the Coder deployment.`,
			Value:       &c.AccessURL,
			Flag:        "access-url",
			Env:         "CODER_ACCESS_URL",
			Group:       &deploymentGroupNetworking,
			YAML:        "accessURL",
			Annotations: serpent.Annotations{}.Mark(annotationExternalProxies, "true"),
		},
		{
			Name:        "Wildcard Access URL",
			Description: "Specifies the wildcard hostname to use for workspace applications in the form \"*.example.com\".",
			Flag:        "wildcard-access-url",
			Env:         "CODER_WILDCARD_ACCESS_URL",
			// Do not use a serpent.URL here. We are intentionally omitting the
			// scheme part of the url (https://), so the standard url parsing
			// will yield unexpected results.
			//
			// We have a validation function to ensure the wildcard url is correct,
			// so use that instead.
			Value: serpent.Validate(&c.WildcardAccessURL, func(value *serpent.String) error {
				if value.Value() == "" {
					return nil
				}
				_, err := appurl.CompileHostnamePattern(value.Value())
				return err
			}),
			Group:       &deploymentGroupNetworking,
			YAML:        "wildcardAccessURL",
			Annotations: serpent.Annotations{}.Mark(annotationExternalProxies, "true"),
		},
		{
			Name:        "Docs URL",
			Description: "Specifies the custom docs URL.",
			Value:       &c.DocsURL,
			Flag:        "docs-url",
			Env:         "CODER_DOCS_URL",
			Group:       &deploymentGroupNetworking,
			YAML:        "docsURL",
			Annotations: serpent.Annotations{}.Mark(annotationExternalProxies, "true"),
		},
		redirectToAccessURL,
		{
			Name:        "Autobuild Poll Interval",
			Description: "Interval to poll for scheduled workspace builds.",
			Flag:        "autobuild-poll-interval",
			Env:         "CODER_AUTOBUILD_POLL_INTERVAL",
			Hidden:      true,
			Default:     time.Minute.String(),
			Value:       &c.AutobuildPollInterval,
			YAML:        "autobuildPollInterval",
			Annotations: serpent.Annotations{}.Mark(annotationFormatDuration, "true"),
		},
		{
			Name:        "Job Hang Detector Interval",
			Description: "Interval to poll for hung jobs and automatically terminate them.",
			Flag:        "job-hang-detector-interval",
			Env:         "CODER_JOB_HANG_DETECTOR_INTERVAL",
			Hidden:      true,
			Default:     time.Minute.String(),
			Value:       &c.JobHangDetectorInterval,
			YAML:        "jobHangDetectorInterval",
			Annotations: serpent.Annotations{}.Mark(annotationFormatDuration, "true"),
		},
		httpAddress,
		tlsBindAddress,
		{
			Name:          "Address",
			Description:   "Bind address of the server.",
			Flag:          "address",
			FlagShorthand: "a",
			Env:           "CODER_ADDRESS",
			Hidden:        true,
			Value:         &c.Address,
			UseInstead: serpent.OptionSet{
				httpAddress,
				tlsBindAddress,
			},
			Group:       &deploymentGroupNetworking,
			Annotations: serpent.Annotations{}.Mark(annotationExternalProxies, "true"),
		},
		// TLS settings
		{
			Name:        "TLS Enable",
			Description: "Whether TLS will be enabled.",
			Flag:        "tls-enable",
			Env:         "CODER_TLS_ENABLE",
			Value:       &c.TLS.Enable,
			Group:       &deploymentGroupNetworkingTLS,
			YAML:        "enable",
			Annotations: serpent.Annotations{}.Mark(annotationExternalProxies, "true"),
		},
		{
			Name:        "Redirect HTTP to HTTPS",
			Description: "Whether HTTP requests will be redirected to the access URL (if it's a https URL and TLS is enabled). Requests to local IP addresses are never redirected regardless of this setting.",
			Flag:        "tls-redirect-http-to-https",
			Env:         "CODER_TLS_REDIRECT_HTTP_TO_HTTPS",
			Default:     "true",
			Hidden:      true,
			Value:       &c.TLS.RedirectHTTP,
			UseInstead:  serpent.OptionSet{redirectToAccessURL},
			Group:       &deploymentGroupNetworkingTLS,
			YAML:        "redirectHTTP",
			Annotations: serpent.Annotations{}.Mark(annotationExternalProxies, "true"),
		},
		{
			Name:        "TLS Certificate Files",
			Description: "Path to each certificate for TLS. It requires a PEM-encoded file. To configure the listener to use a CA certificate, concatenate the primary certificate and the CA certificate together. The primary certificate should appear first in the combined file.",
			Flag:        "tls-cert-file",
			Env:         "CODER_TLS_CERT_FILE",
			Value:       &c.TLS.CertFiles,
			Group:       &deploymentGroupNetworkingTLS,
			YAML:        "certFiles",
			Annotations: serpent.Annotations{}.Mark(annotationExternalProxies, "true"),
		},
		{
			Name:        "TLS Client CA Files",
			Description: "PEM-encoded Certificate Authority file used for checking the authenticity of client.",
			Flag:        "tls-client-ca-file",
			Env:         "CODER_TLS_CLIENT_CA_FILE",
			Value:       &c.TLS.ClientCAFile,
			Group:       &deploymentGroupNetworkingTLS,
			YAML:        "clientCAFile",
			Annotations: serpent.Annotations{}.Mark(annotationExternalProxies, "true"),
		},
		{
			Name:        "TLS Client Auth",
			Description: "Policy the server will follow for TLS Client Authentication. Accepted values are \"none\", \"request\", \"require-any\", \"verify-if-given\", or \"require-and-verify\".",
			Flag:        "tls-client-auth",
			Env:         "CODER_TLS_CLIENT_AUTH",
			Default:     "none",
			Value:       &c.TLS.ClientAuth,
			Group:       &deploymentGroupNetworkingTLS,
			YAML:        "clientAuth",
			Annotations: serpent.Annotations{}.Mark(annotationExternalProxies, "true"),
		},
		{
			Name:        "TLS Key Files",
			Description: "Paths to the private keys for each of the certificates. It requires a PEM-encoded file.",
			Flag:        "tls-key-file",
			Env:         "CODER_TLS_KEY_FILE",
			Value:       &c.TLS.KeyFiles,
			Group:       &deploymentGroupNetworkingTLS,
			YAML:        "keyFiles",
			Annotations: serpent.Annotations{}.Mark(annotationExternalProxies, "true"),
		},
		{
			Name:        "TLS Minimum Version",
			Description: "Minimum supported version of TLS. Accepted values are \"tls10\", \"tls11\", \"tls12\" or \"tls13\".",
			Flag:        "tls-min-version",
			Env:         "CODER_TLS_MIN_VERSION",
			Default:     "tls12",
			Value:       &c.TLS.MinVersion,
			Group:       &deploymentGroupNetworkingTLS,
			YAML:        "minVersion",
			Annotations: serpent.Annotations{}.Mark(annotationExternalProxies, "true"),
		},
		{
			Name:        "TLS Client Cert File",
			Description: "Path to certificate for client TLS authentication. It requires a PEM-encoded file.",
			Flag:        "tls-client-cert-file",
			Env:         "CODER_TLS_CLIENT_CERT_FILE",
			Value:       &c.TLS.ClientCertFile,
			Group:       &deploymentGroupNetworkingTLS,
			YAML:        "clientCertFile",
			Annotations: serpent.Annotations{}.Mark(annotationExternalProxies, "true"),
		},
		{
			Name:        "TLS Client Key File",
			Description: "Path to key for client TLS authentication. It requires a PEM-encoded file.",
			Flag:        "tls-client-key-file",
			Env:         "CODER_TLS_CLIENT_KEY_FILE",
			Value:       &c.TLS.ClientKeyFile,
			Group:       &deploymentGroupNetworkingTLS,
			YAML:        "clientKeyFile",
			Annotations: serpent.Annotations{}.Mark(annotationExternalProxies, "true"),
		},
		{
			Name:        "TLS Ciphers",
			Description: "Specify specific TLS ciphers that allowed to be used. See https://github.com/golang/go/blob/master/src/crypto/tls/cipher_suites.go#L53-L75.",
			Flag:        "tls-ciphers",
			Env:         "CODER_TLS_CIPHERS",
			Default:     "",
			Value:       &c.TLS.SupportedCiphers,
			Group:       &deploymentGroupNetworkingTLS,
			YAML:        "tlsCiphers",
			Annotations: serpent.Annotations{}.Mark(annotationExternalProxies, "true"),
		},
		{
			Name:        "TLS Allow Insecure Ciphers",
			Description: "By default, only ciphers marked as 'secure' are allowed to be used. See https://github.com/golang/go/blob/master/src/crypto/tls/cipher_suites.go#L82-L95.",
			Flag:        "tls-allow-insecure-ciphers",
			Env:         "CODER_TLS_ALLOW_INSECURE_CIPHERS",
			Default:     "false",
			Value:       &c.TLS.AllowInsecureCiphers,
			Group:       &deploymentGroupNetworkingTLS,
			YAML:        "tlsAllowInsecureCiphers",
			Annotations: serpent.Annotations{}.Mark(annotationExternalProxies, "true"),
		},
		// Derp settings
		{
			Name:        "DERP Server Enable",
			Description: "Whether to enable or disable the embedded DERP relay server.",
			Flag:        "derp-server-enable",
			Env:         "CODER_DERP_SERVER_ENABLE",
			Default:     "true",
			Value:       &c.DERP.Server.Enable,
			Group:       &deploymentGroupNetworkingDERP,
			YAML:        "enable",
			Annotations: serpent.Annotations{}.Mark(annotationExternalProxies, "true"),
		},
		{
			Name:        "DERP Server Region ID",
			Description: "Region ID to use for the embedded DERP server.",
			Flag:        "derp-server-region-id",
			Env:         "CODER_DERP_SERVER_REGION_ID",
			Default:     "999",
			Value:       &c.DERP.Server.RegionID,
			Group:       &deploymentGroupNetworkingDERP,
			YAML:        "regionID",
			Hidden:      true,
			// Does not apply to external proxies as this value is generated.
		},
		{
			Name:        "DERP Server Region Code",
			Description: "Region code to use for the embedded DERP server.",
			Flag:        "derp-server-region-code",
			Env:         "CODER_DERP_SERVER_REGION_CODE",
			Default:     "coder",
			Value:       &c.DERP.Server.RegionCode,
			Group:       &deploymentGroupNetworkingDERP,
			YAML:        "regionCode",
			Hidden:      true,
			// Does not apply to external proxies as we use the proxy name.
		},
		{
			Name:        "DERP Server Region Name",
			Description: "Region name that for the embedded DERP server.",
			Flag:        "derp-server-region-name",
			Env:         "CODER_DERP_SERVER_REGION_NAME",
			Default:     "Coder Embedded Relay",
			Value:       &c.DERP.Server.RegionName,
			Group:       &deploymentGroupNetworkingDERP,
			YAML:        "regionName",
			// Does not apply to external proxies as we use the proxy name.
		},
		{
			Name:        "DERP Server STUN Addresses",
			Description: "Addresses for STUN servers to establish P2P connections. It's recommended to have at least two STUN servers to give users the best chance of connecting P2P to workspaces. Each STUN server will get it's own DERP region, with region IDs starting at `--derp-server-region-id + 1`. Use special value 'disable' to turn off STUN completely.",
			Flag:        "derp-server-stun-addresses",
			Env:         "CODER_DERP_SERVER_STUN_ADDRESSES",
			Default:     "stun.l.google.com:19302,stun1.l.google.com:19302,stun2.l.google.com:19302,stun3.l.google.com:19302,stun4.l.google.com:19302",
			Value:       &c.DERP.Server.STUNAddresses,
			Group:       &deploymentGroupNetworkingDERP,
			YAML:        "stunAddresses",
		},
		{
			Name:        "DERP Server Relay URL",
			Description: "An HTTP URL that is accessible by other replicas to relay DERP traffic. Required for high availability.",
			Flag:        "derp-server-relay-url",
			Env:         "CODER_DERP_SERVER_RELAY_URL",
			Value:       &c.DERP.Server.RelayURL,
			Group:       &deploymentGroupNetworkingDERP,
			YAML:        "relayURL",
			Annotations: serpent.Annotations{}.
				Mark(annotationEnterpriseKey, "true").
				Mark(annotationExternalProxies, "true"),
		},
		{
			Name:        "Block Direct Connections",
			Description: "Block peer-to-peer (aka. direct) workspace connections. All workspace connections from the CLI will be proxied through Coder (or custom configured DERP servers) and will never be peer-to-peer when enabled. Workspaces may still reach out to STUN servers to get their address until they are restarted after this change has been made, but new connections will still be proxied regardless.",
			// This cannot be called `disable-direct-connections` because that's
			// already a global CLI flag for CLI connections. This is a
			// deployment-wide flag.
			Flag:  "block-direct-connections",
			Env:   "CODER_BLOCK_DIRECT",
			Value: &c.DERP.Config.BlockDirect,
			Group: &deploymentGroupNetworkingDERP,
			YAML:  "blockDirect", Annotations: serpent.Annotations{}.
				Mark(annotationExternalProxies, "true"),
		},
		{
			Name:        "DERP Force WebSockets",
			Description: "Force clients and agents to always use WebSocket to connect to DERP relay servers. By default, DERP uses `Upgrade: derp`, which may cause issues with some reverse proxies. Clients may automatically fallback to WebSocket if they detect an issue with `Upgrade: derp`, but this does not work in all situations.",
			Flag:        "derp-force-websockets",
			Env:         "CODER_DERP_FORCE_WEBSOCKETS",
			Value:       &c.DERP.Config.ForceWebSockets,
			Group:       &deploymentGroupNetworkingDERP,
			YAML:        "forceWebSockets",
		},
		{
			Name:        "DERP Config URL",
			Description: "URL to fetch a DERP mapping on startup. See: https://tailscale.com/kb/1118/custom-derp-servers/.",
			Flag:        "derp-config-url",
			Env:         "CODER_DERP_CONFIG_URL",
			Value:       &c.DERP.Config.URL,
			Group:       &deploymentGroupNetworkingDERP,
			YAML:        "url",
		},
		{
			Name:        "DERP Config Path",
			Description: "Path to read a DERP mapping from. See: https://tailscale.com/kb/1118/custom-derp-servers/.",
			Flag:        "derp-config-path",
			Env:         "CODER_DERP_CONFIG_PATH",
			Value:       &c.DERP.Config.Path,
			Group:       &deploymentGroupNetworkingDERP,
			YAML:        "configPath",
		},
		// TODO: support Git Auth settings.
		// Prometheus settings
		{
			Name:        "Prometheus Enable",
			Description: "Serve prometheus metrics on the address defined by prometheus address.",
			Flag:        "prometheus-enable",
			Env:         "CODER_PROMETHEUS_ENABLE",
			Value:       &c.Prometheus.Enable,
			Group:       &deploymentGroupIntrospectionPrometheus,
			YAML:        "enable",
			Annotations: serpent.Annotations{}.Mark(annotationExternalProxies, "true"),
		},
		{
			Name:        "Prometheus Address",
			Description: "The bind address to serve prometheus metrics.",
			Flag:        "prometheus-address",
			Env:         "CODER_PROMETHEUS_ADDRESS",
			Default:     "127.0.0.1:2112",
			Value:       &c.Prometheus.Address,
			Group:       &deploymentGroupIntrospectionPrometheus,
			YAML:        "address",
			Annotations: serpent.Annotations{}.Mark(annotationExternalProxies, "true"),
		},
		{
			Name:        "Prometheus Collect Agent Stats",
			Description: "Collect agent stats (may increase charges for metrics storage).",
			Flag:        "prometheus-collect-agent-stats",
			Env:         "CODER_PROMETHEUS_COLLECT_AGENT_STATS",
			Value:       &c.Prometheus.CollectAgentStats,
			Group:       &deploymentGroupIntrospectionPrometheus,
			YAML:        "collect_agent_stats",
		},
		{
			Name:        "Prometheus Aggregate Agent Stats By",
			Description: fmt.Sprintf("When collecting agent stats, aggregate metrics by a given set of comma-separated labels to reduce cardinality. Accepted values are %s.", strings.Join(agentmetrics.LabelAll, ", ")),
			Flag:        "prometheus-aggregate-agent-stats-by",
			Env:         "CODER_PROMETHEUS_AGGREGATE_AGENT_STATS_BY",
			Value: serpent.Validate(&c.Prometheus.AggregateAgentStatsBy, func(value *serpent.StringArray) error {
				if value == nil {
					return nil
				}

				return agentmetrics.ValidateAggregationLabels(value.Value())
			}),
			Group:   &deploymentGroupIntrospectionPrometheus,
			YAML:    "aggregate_agent_stats_by",
			Default: strings.Join(agentmetrics.LabelAll, ","),
		},
		{
			Name:        "Prometheus Collect Database Metrics",
			Description: "Collect database metrics (may increase charges for metrics storage).",
			Flag:        "prometheus-collect-db-metrics",
			Env:         "CODER_PROMETHEUS_COLLECT_DB_METRICS",
			Value:       &c.Prometheus.CollectDBMetrics,
			Group:       &deploymentGroupIntrospectionPrometheus,
			YAML:        "collect_db_metrics",
			Default:     "false",
		},
		// Pprof settings
		{
			Name:        "pprof Enable",
			Description: "Serve pprof metrics on the address defined by pprof address.",
			Flag:        "pprof-enable",
			Env:         "CODER_PPROF_ENABLE",
			Value:       &c.Pprof.Enable,
			Group:       &deploymentGroupIntrospectionPPROF,
			YAML:        "enable",
			Annotations: serpent.Annotations{}.Mark(annotationExternalProxies, "true"),
		},
		{
			Name:        "pprof Address",
			Description: "The bind address to serve pprof.",
			Flag:        "pprof-address",
			Env:         "CODER_PPROF_ADDRESS",
			Default:     "127.0.0.1:6060",
			Value:       &c.Pprof.Address,
			Group:       &deploymentGroupIntrospectionPPROF,
			YAML:        "address",
			Annotations: serpent.Annotations{}.Mark(annotationExternalProxies, "true"),
		},
		// oAuth settings
		{
			Name:        "OAuth2 GitHub Client ID",
			Description: "Client ID for Login with GitHub.",
			Flag:        "oauth2-github-client-id",
			Env:         "CODER_OAUTH2_GITHUB_CLIENT_ID",
			Value:       &c.OAuth2.Github.ClientID,
			Group:       &deploymentGroupOAuth2GitHub,
			YAML:        "clientID",
		},
		{
			Name:        "OAuth2 GitHub Client Secret",
			Description: "Client secret for Login with GitHub.",
			Flag:        "oauth2-github-client-secret",
			Env:         "CODER_OAUTH2_GITHUB_CLIENT_SECRET",
			Value:       &c.OAuth2.Github.ClientSecret,
			Annotations: serpent.Annotations{}.Mark(annotationSecretKey, "true"),
			Group:       &deploymentGroupOAuth2GitHub,
		},
		{
			Name:        "OAuth2 GitHub Allowed Orgs",
			Description: "Organizations the user must be a member of to Login with GitHub.",
			Flag:        "oauth2-github-allowed-orgs",
			Env:         "CODER_OAUTH2_GITHUB_ALLOWED_ORGS",
			Value:       &c.OAuth2.Github.AllowedOrgs,
			Group:       &deploymentGroupOAuth2GitHub,
			YAML:        "allowedOrgs",
		},
		{
			Name:        "OAuth2 GitHub Allowed Teams",
			Description: "Teams inside organizations the user must be a member of to Login with GitHub. Structured as: <organization-name>/<team-slug>.",
			Flag:        "oauth2-github-allowed-teams",
			Env:         "CODER_OAUTH2_GITHUB_ALLOWED_TEAMS",
			Value:       &c.OAuth2.Github.AllowedTeams,
			Group:       &deploymentGroupOAuth2GitHub,
			YAML:        "allowedTeams",
		},
		{
			Name:        "OAuth2 GitHub Allow Signups",
			Description: "Whether new users can sign up with GitHub.",
			Flag:        "oauth2-github-allow-signups",
			Env:         "CODER_OAUTH2_GITHUB_ALLOW_SIGNUPS",
			Value:       &c.OAuth2.Github.AllowSignups,
			Group:       &deploymentGroupOAuth2GitHub,
			YAML:        "allowSignups",
		},
		{
			Name:        "OAuth2 GitHub Allow Everyone",
			Description: "Allow all logins, setting this option means allowed orgs and teams must be empty.",
			Flag:        "oauth2-github-allow-everyone",
			Env:         "CODER_OAUTH2_GITHUB_ALLOW_EVERYONE",
			Value:       &c.OAuth2.Github.AllowEveryone,
			Group:       &deploymentGroupOAuth2GitHub,
			YAML:        "allowEveryone",
		},
		{
			Name:        "OAuth2 GitHub Enterprise Base URL",
			Description: "Base URL of a GitHub Enterprise deployment to use for Login with GitHub.",
			Flag:        "oauth2-github-enterprise-base-url",
			Env:         "CODER_OAUTH2_GITHUB_ENTERPRISE_BASE_URL",
			Value:       &c.OAuth2.Github.EnterpriseBaseURL,
			Group:       &deploymentGroupOAuth2GitHub,
			YAML:        "enterpriseBaseURL",
		},
		// OIDC settings.
		{
			Name:        "OIDC Allow Signups",
			Description: "Whether new users can sign up with OIDC.",
			Flag:        "oidc-allow-signups",
			Env:         "CODER_OIDC_ALLOW_SIGNUPS",
			Default:     "true",
			Value:       &c.OIDC.AllowSignups,
			Group:       &deploymentGroupOIDC,
			YAML:        "allowSignups",
		},
		{
			Name:        "OIDC Client ID",
			Description: "Client ID to use for Login with OIDC.",
			Flag:        "oidc-client-id",
			Env:         "CODER_OIDC_CLIENT_ID",
			Value:       &c.OIDC.ClientID,
			Group:       &deploymentGroupOIDC,
			YAML:        "clientID",
		},
		{
			Name:        "OIDC Client Secret",
			Description: "Client secret to use for Login with OIDC.",
			Flag:        "oidc-client-secret",
			Env:         "CODER_OIDC_CLIENT_SECRET",
			Annotations: serpent.Annotations{}.Mark(annotationSecretKey, "true"),
			Value:       &c.OIDC.ClientSecret,
			Group:       &deploymentGroupOIDC,
		},
		{
			Name: "OIDC Client Key File",
			Description: "Pem encoded RSA private key to use for oauth2 PKI/JWT authorization. " +
				"This can be used instead of oidc-client-secret if your IDP supports it.",
			Flag:  "oidc-client-key-file",
			Env:   "CODER_OIDC_CLIENT_KEY_FILE",
			YAML:  "oidcClientKeyFile",
			Value: &c.OIDC.ClientKeyFile,
			Group: &deploymentGroupOIDC,
		},
		{
			Name: "OIDC Client Cert File",
			Description: "Pem encoded certificate file to use for oauth2 PKI/JWT authorization. " +
				"The public certificate that accompanies oidc-client-key-file. A standard x509 certificate is expected.",
			Flag:  "oidc-client-cert-file",
			Env:   "CODER_OIDC_CLIENT_CERT_FILE",
			YAML:  "oidcClientCertFile",
			Value: &c.OIDC.ClientCertFile,
			Group: &deploymentGroupOIDC,
		},
		{
			Name:        "OIDC Email Domain",
			Description: "Email domains that clients logging in with OIDC must match.",
			Flag:        "oidc-email-domain",
			Env:         "CODER_OIDC_EMAIL_DOMAIN",
			Value:       &c.OIDC.EmailDomain,
			Group:       &deploymentGroupOIDC,
			YAML:        "emailDomain",
		},
		{
			Name:        "OIDC Issuer URL",
			Description: "Issuer URL to use for Login with OIDC.",
			Flag:        "oidc-issuer-url",
			Env:         "CODER_OIDC_ISSUER_URL",
			Value:       &c.OIDC.IssuerURL,
			Group:       &deploymentGroupOIDC,
			YAML:        "issuerURL",
		},
		{
			Name:        "OIDC Scopes",
			Description: "Scopes to grant when authenticating with OIDC.",
			Flag:        "oidc-scopes",
			Env:         "CODER_OIDC_SCOPES",
			Default:     strings.Join([]string{oidc.ScopeOpenID, "profile", "email"}, ","),
			Value:       &c.OIDC.Scopes,
			Group:       &deploymentGroupOIDC,
			YAML:        "scopes",
		},
		{
			Name:        "OIDC Ignore Email Verified",
			Description: "Ignore the email_verified claim from the upstream provider.",
			Flag:        "oidc-ignore-email-verified",
			Env:         "CODER_OIDC_IGNORE_EMAIL_VERIFIED",
			Value:       &c.OIDC.IgnoreEmailVerified,
			Group:       &deploymentGroupOIDC,
			YAML:        "ignoreEmailVerified",
		},
		{
			Name:        "OIDC Username Field",
			Description: "OIDC claim field to use as the username.",
			Flag:        "oidc-username-field",
			Env:         "CODER_OIDC_USERNAME_FIELD",
			Default:     "preferred_username",
			Value:       &c.OIDC.UsernameField,
			Group:       &deploymentGroupOIDC,
			YAML:        "usernameField",
		},
		{
			Name:        "OIDC Email Field",
			Description: "OIDC claim field to use as the email.",
			Flag:        "oidc-email-field",
			Env:         "CODER_OIDC_EMAIL_FIELD",
			Default:     "email",
			Value:       &c.OIDC.EmailField,
			Group:       &deploymentGroupOIDC,
			YAML:        "emailField",
		},
		{
			Name:        "OIDC Auth URL Parameters",
			Description: "OIDC auth URL parameters to pass to the upstream provider.",
			Flag:        "oidc-auth-url-params",
			Env:         "CODER_OIDC_AUTH_URL_PARAMS",
			Default:     `{"access_type": "offline"}`,
			Value:       &c.OIDC.AuthURLParams,
			Group:       &deploymentGroupOIDC,
			YAML:        "authURLParams",
		},
		{
			Name:        "OIDC Ignore UserInfo",
			Description: "Ignore the userinfo endpoint and only use the ID token for user information.",
			Flag:        "oidc-ignore-userinfo",
			Env:         "CODER_OIDC_IGNORE_USERINFO",
			Default:     "false",
			Value:       &c.OIDC.IgnoreUserInfo,
			Group:       &deploymentGroupOIDC,
			YAML:        "ignoreUserInfo",
		},
		{
			Name:        "OIDC Group Field",
			Description: "This field must be set if using the group sync feature and the scope name is not 'groups'. Set to the claim to be used for groups.",
			Flag:        "oidc-group-field",
			Env:         "CODER_OIDC_GROUP_FIELD",
			// This value is intentionally blank. If this is empty, then OIDC group
			// behavior is disabled. If 'oidc-scopes' contains 'groups', then the
			// default value will be 'groups'. If the user wants to use a different claim
			// such as 'memberOf', they can override the default 'groups' claim value
			// that comes from the oidc scopes.
			Default: "",
			Value:   &c.OIDC.GroupField,
			Group:   &deploymentGroupOIDC,
			YAML:    "groupField",
		},
		{
			Name:        "OIDC Group Mapping",
			Description: "A map of OIDC group IDs and the group in Coder it should map to. This is useful for when OIDC providers only return group IDs.",
			Flag:        "oidc-group-mapping",
			Env:         "CODER_OIDC_GROUP_MAPPING",
			Default:     "{}",
			Value:       &c.OIDC.GroupMapping,
			Group:       &deploymentGroupOIDC,
			YAML:        "groupMapping",
		},
		{
			Name:        "Enable OIDC Group Auto Create",
			Description: "Automatically creates missing groups from a user's groups claim.",
			Flag:        "oidc-group-auto-create",
			Env:         "CODER_OIDC_GROUP_AUTO_CREATE",
			Default:     "false",
			Value:       &c.OIDC.GroupAutoCreate,
			Group:       &deploymentGroupOIDC,
			YAML:        "enableGroupAutoCreate",
		},
		{
			Name:        "OIDC Regex Group Filter",
			Description: "If provided any group name not matching the regex is ignored. This allows for filtering out groups that are not needed. This filter is applied after the group mapping.",
			Flag:        "oidc-group-regex-filter",
			Env:         "CODER_OIDC_GROUP_REGEX_FILTER",
			Default:     ".*",
			Value:       &c.OIDC.GroupRegexFilter,
			Group:       &deploymentGroupOIDC,
			YAML:        "groupRegexFilter",
		},
		{
			Name:        "OIDC Allowed Groups",
			Description: "If provided any group name not in the list will not be allowed to authenticate. This allows for restricting access to a specific set of groups. This filter is applied after the group mapping and before the regex filter.",
			Flag:        "oidc-allowed-groups",
			Env:         "CODER_OIDC_ALLOWED_GROUPS",
			Default:     "",
			Value:       &c.OIDC.GroupAllowList,
			Group:       &deploymentGroupOIDC,
			YAML:        "groupAllowed",
		},
		{
			Name:        "OIDC User Role Field",
			Description: "This field must be set if using the user roles sync feature. Set this to the name of the claim used to store the user's role. The roles should be sent as an array of strings.",
			Flag:        "oidc-user-role-field",
			Env:         "CODER_OIDC_USER_ROLE_FIELD",
			// This value is intentionally blank. If this is empty, then OIDC user role
			// sync behavior is disabled.
			Default: "",
			Value:   &c.OIDC.UserRoleField,
			Group:   &deploymentGroupOIDC,
			YAML:    "userRoleField",
		},
		{
			Name:        "OIDC User Role Mapping",
			Description: "A map of the OIDC passed in user roles and the groups in Coder it should map to. This is useful if the group names do not match. If mapped to the empty string, the role will ignored.",
			Flag:        "oidc-user-role-mapping",
			Env:         "CODER_OIDC_USER_ROLE_MAPPING",
			Default:     "{}",
			Value:       &c.OIDC.UserRoleMapping,
			Group:       &deploymentGroupOIDC,
			YAML:        "userRoleMapping",
		},
		{
			Name:        "OIDC User Role Default",
			Description: "If user role sync is enabled, these roles are always included for all authenticated users. The 'member' role is always assigned.",
			Flag:        "oidc-user-role-default",
			Env:         "CODER_OIDC_USER_ROLE_DEFAULT",
			Default:     "",
			Value:       &c.OIDC.UserRolesDefault,
			Group:       &deploymentGroupOIDC,
			YAML:        "userRoleDefault",
		},
		{
			Name:        "OpenID Connect sign in text",
			Description: "The text to show on the OpenID Connect sign in button.",
			Flag:        "oidc-sign-in-text",
			Env:         "CODER_OIDC_SIGN_IN_TEXT",
			Default:     "OpenID Connect",
			Value:       &c.OIDC.SignInText,
			Group:       &deploymentGroupOIDC,
			YAML:        "signInText",
		},
		{
			Name:        "OpenID connect icon URL",
			Description: "URL pointing to the icon to use on the OpenID Connect login button.",
			Flag:        "oidc-icon-url",
			Env:         "CODER_OIDC_ICON_URL",
			Value:       &c.OIDC.IconURL,
			Group:       &deploymentGroupOIDC,
			YAML:        "iconURL",
		},
		{
			Name:        "Signups disabled text",
			Description: "The custom text to show on the error page informing about disabled OIDC signups. Markdown format is supported.",
			Flag:        "oidc-signups-disabled-text",
			Env:         "CODER_OIDC_SIGNUPS_DISABLED_TEXT",
			Value:       &c.OIDC.SignupsDisabledText,
			Group:       &deploymentGroupOIDC,
			YAML:        "signupsDisabledText",
		},
		// Telemetry settings
		{
			Name:        "Telemetry Enable",
			Description: "Whether telemetry is enabled or not. Coder collects anonymized usage data to help improve our product.",
			Flag:        "telemetry",
			Env:         "CODER_TELEMETRY_ENABLE",
			Default:     strconv.FormatBool(flag.Lookup("test.v") == nil),
			Value:       &c.Telemetry.Enable,
			Group:       &deploymentGroupTelemetry,
			YAML:        "enable",
		},
		{
			Name:        "Telemetry URL",
			Description: "URL to send telemetry.",
			Flag:        "telemetry-url",
			Env:         "CODER_TELEMETRY_URL",
			Hidden:      true,
			Default:     "https://telemetry.coder.com",
			Value:       &c.Telemetry.URL,
			Group:       &deploymentGroupTelemetry,
			YAML:        "url",
		},
		// Trace settings
		{
			Name:        "Trace Enable",
			Description: "Whether application tracing data is collected. It exports to a backend configured by environment variables. See: https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/protocol/exporter.md.",
			Flag:        "trace",
			Env:         "CODER_TRACE_ENABLE",
			Value:       &c.Trace.Enable,
			Group:       &deploymentGroupIntrospectionTracing,
			YAML:        "enable",
			Annotations: serpent.Annotations{}.Mark(annotationExternalProxies, "true"),
		},
		{
			Name:        "Trace Honeycomb API Key",
			Description: "Enables trace exporting to Honeycomb.io using the provided API Key.",
			Flag:        "trace-honeycomb-api-key",
			Env:         "CODER_TRACE_HONEYCOMB_API_KEY",
			Annotations: serpent.Annotations{}.Mark(annotationSecretKey, "true").Mark(annotationExternalProxies, "true"),
			Value:       &c.Trace.HoneycombAPIKey,
			Group:       &deploymentGroupIntrospectionTracing,
		},
		{
			Name:        "Capture Logs in Traces",
			Description: "Enables capturing of logs as events in traces. This is useful for debugging, but may result in a very large amount of events being sent to the tracing backend which may incur significant costs.",
			Flag:        "trace-logs",
			Env:         "CODER_TRACE_LOGS",
			Value:       &c.Trace.CaptureLogs,
			Group:       &deploymentGroupIntrospectionTracing,
			YAML:        "captureLogs",
			Annotations: serpent.Annotations{}.Mark(annotationExternalProxies, "true"),
		},
		{
			Name:        "Send Go runtime traces to DataDog",
			Description: "Enables sending Go runtime traces to the local DataDog agent.",
			Flag:        "trace-datadog",
			Env:         "CODER_TRACE_DATADOG",
			Value:       &c.Trace.DataDog,
			Group:       &deploymentGroupIntrospectionTracing,
			YAML:        "dataDog",
			// Hidden until an external user asks for it. For the time being,
			// it's used to detect leaks in dogfood.
			Hidden: true,
			// Default is false because datadog creates a bunch of goroutines that
			// don't get cleaned up and trip the leak detector.
			Default:     "false",
			Annotations: serpent.Annotations{}.Mark(annotationExternalProxies, "true"),
		},
		// Provisioner settings
		{
			Name:        "Provisioner Daemons",
			Description: "Number of provisioner daemons to create on start. If builds are stuck in queued state for a long time, consider increasing this.",
			Flag:        "provisioner-daemons",
			Env:         "CODER_PROVISIONER_DAEMONS",
			Default:     "3",
			Value:       &c.Provisioner.Daemons,
			Group:       &deploymentGroupProvisioning,
			YAML:        "daemons",
		},
		{
			Name:        "Echo Provisioner",
			Description: "Whether to use echo provisioner daemons instead of Terraform. This is for E2E tests.",
			Flag:        "provisioner-daemons-echo",
			Env:         "CODER_PROVISIONER_DAEMONS_ECHO",
			Hidden:      true,
			Default:     "false",
			Value:       &c.Provisioner.DaemonsEcho,
			Group:       &deploymentGroupProvisioning,
			YAML:        "daemonsEcho",
		},
		{
			Name:        "Poll Interval",
			Description: "Deprecated and ignored.",
			Flag:        "provisioner-daemon-poll-interval",
			Env:         "CODER_PROVISIONER_DAEMON_POLL_INTERVAL",
			Default:     time.Second.String(),
			Value:       &c.Provisioner.DaemonPollInterval,
			Group:       &deploymentGroupProvisioning,
			YAML:        "daemonPollInterval",
			Annotations: serpent.Annotations{}.Mark(annotationFormatDuration, "true"),
		},
		{
			Name:        "Poll Jitter",
			Description: "Deprecated and ignored.",
			Flag:        "provisioner-daemon-poll-jitter",
			Env:         "CODER_PROVISIONER_DAEMON_POLL_JITTER",
			Default:     (100 * time.Millisecond).String(),
			Value:       &c.Provisioner.DaemonPollJitter,
			Group:       &deploymentGroupProvisioning,
			YAML:        "daemonPollJitter",
			Annotations: serpent.Annotations{}.Mark(annotationFormatDuration, "true"),
		},
		{
			Name:        "Force Cancel Interval",
			Description: "Time to force cancel provisioning tasks that are stuck.",
			Flag:        "provisioner-force-cancel-interval",
			Env:         "CODER_PROVISIONER_FORCE_CANCEL_INTERVAL",
			Default:     (10 * time.Minute).String(),
			Value:       &c.Provisioner.ForceCancelInterval,
			Group:       &deploymentGroupProvisioning,
			YAML:        "forceCancelInterval",
			Annotations: serpent.Annotations{}.Mark(annotationFormatDuration, "true"),
		},
		{
			Name:        "Provisioner Daemon Pre-shared Key (PSK)",
			Description: "Pre-shared key to authenticate external provisioner daemons to Coder server.",
			Flag:        "provisioner-daemon-psk",
			Env:         "CODER_PROVISIONER_DAEMON_PSK",
			Value:       &c.Provisioner.DaemonPSK,
			Group:       &deploymentGroupProvisioning,
			Annotations: serpent.Annotations{}.Mark(annotationSecretKey, "true"),
		},
		// RateLimit settings
		{
			Name:        "Disable All Rate Limits",
			Description: "Disables all rate limits. This is not recommended in production.",
			Flag:        "dangerous-disable-rate-limits",
			Env:         "CODER_DANGEROUS_DISABLE_RATE_LIMITS",

			Value:       &c.RateLimit.DisableAll,
			Hidden:      true,
			Annotations: serpent.Annotations{}.Mark(annotationExternalProxies, "true"),
		},
		{
			Name:        "API Rate Limit",
			Description: "Maximum number of requests per minute allowed to the API per user, or per IP address for unauthenticated users. Negative values mean no rate limit. Some API endpoints have separate strict rate limits regardless of this value to prevent denial-of-service or brute force attacks.",
			// Change the env from the auto-generated CODER_RATE_LIMIT_API to the
			// old value to avoid breaking existing deployments.
			Env:         "CODER_API_RATE_LIMIT",
			Flag:        "api-rate-limit",
			Default:     "512",
			Value:       &c.RateLimit.API,
			Hidden:      true,
			Annotations: serpent.Annotations{}.Mark(annotationExternalProxies, "true"),
		},
		// Logging settings
		{
			Name:          "Verbose",
			Description:   "Output debug-level logs.",
			Flag:          "verbose",
			Env:           "CODER_VERBOSE",
			FlagShorthand: "v",
			Hidden:        true,
			UseInstead:    []serpent.Option{logFilter},
			Value:         &c.Verbose,
			Group:         &deploymentGroupIntrospectionLogging,
			YAML:          "verbose",
			Annotations:   serpent.Annotations{}.Mark(annotationExternalProxies, "true"),
		},
		logFilter,
		{
			Name:        "Human Log Location",
			Description: "Output human-readable logs to a given file.",
			Flag:        "log-human",
			Env:         "CODER_LOGGING_HUMAN",
			Default:     "/dev/stderr",
			Value:       &c.Logging.Human,
			Group:       &deploymentGroupIntrospectionLogging,
			YAML:        "humanPath",
			Annotations: serpent.Annotations{}.Mark(annotationExternalProxies, "true"),
		},
		{
			Name:        "JSON Log Location",
			Description: "Output JSON logs to a given file.",
			Flag:        "log-json",
			Env:         "CODER_LOGGING_JSON",
			Default:     "",
			Value:       &c.Logging.JSON,
			Group:       &deploymentGroupIntrospectionLogging,
			YAML:        "jsonPath",
			Annotations: serpent.Annotations{}.Mark(annotationExternalProxies, "true"),
		},
		{
			Name:        "Stackdriver Log Location",
			Description: "Output Stackdriver compatible logs to a given file.",
			Flag:        "log-stackdriver",
			Env:         "CODER_LOGGING_STACKDRIVER",
			Default:     "",
			Value:       &c.Logging.Stackdriver,
			Group:       &deploymentGroupIntrospectionLogging,
			YAML:        "stackdriverPath",
			Annotations: serpent.Annotations{}.Mark(annotationExternalProxies, "true"),
		},
		{
			Name:        "Enable Terraform debug mode",
			Description: "Allow administrators to enable Terraform debug output.",
			Flag:        "enable-terraform-debug-mode",
			Env:         "CODER_ENABLE_TERRAFORM_DEBUG_MODE",
			Default:     "false",
			Value:       &c.EnableTerraformDebugMode,
			Group:       &deploymentGroupIntrospectionLogging,
			YAML:        "enableTerraformDebugMode",
		},
		// ☢️ Dangerous settings
		{
			Name:        "DANGEROUS: Allow all CORS requests",
			Description: "For security reasons, CORS requests are blocked except between workspace apps owned by the same user. If external requests are required, setting this to true will set all cors headers as '*'. This should never be used in production.",
			Flag:        "dangerous-allow-cors-requests",
			Env:         "CODER_DANGEROUS_ALLOW_CORS_REQUESTS",
			Hidden:      true, // Hidden, should only be used by yarn dev server
			Value:       &c.Dangerous.AllowAllCors,
			Group:       &deploymentGroupDangerous,
			Annotations: serpent.Annotations{}.Mark(annotationExternalProxies, "true"),
		},
		{
			Name:        "DANGEROUS: Allow Path App Sharing",
			Description: "Allow workspace apps that are not served from subdomains to be shared. Path-based app sharing is DISABLED by default for security purposes. Path-based apps can make requests to the Coder API and pose a security risk when the workspace serves malicious JavaScript. Path-based apps can be disabled entirely with --disable-path-apps for further security.",
			Flag:        "dangerous-allow-path-app-sharing",
			Env:         "CODER_DANGEROUS_ALLOW_PATH_APP_SHARING",

			Value: &c.Dangerous.AllowPathAppSharing,
			Group: &deploymentGroupDangerous,
		},
		{
			Name:        "DANGEROUS: Allow Site Owners to Access Path Apps",
			Description: "Allow site-owners to access workspace apps from workspaces they do not own. Owners cannot access path-based apps they do not own by default. Path-based apps can make requests to the Coder API and pose a security risk when the workspace serves malicious JavaScript. Path-based apps can be disabled entirely with --disable-path-apps for further security.",
			Flag:        "dangerous-allow-path-app-site-owner-access",
			Env:         "CODER_DANGEROUS_ALLOW_PATH_APP_SITE_OWNER_ACCESS",

			Value: &c.Dangerous.AllowPathAppSiteOwnerAccess,
			Group: &deploymentGroupDangerous,
		},
		// Misc. settings
		{
			Name:        "Experiments",
			Description: "Enable one or more experiments. These are not ready for production. Separate multiple experiments with commas, or enter '*' to opt-in to all available experiments.",
			Flag:        "experiments",
			Env:         "CODER_EXPERIMENTS",
			Value:       &c.Experiments,
			YAML:        "experiments",
			Annotations: serpent.Annotations{}.Mark(annotationExternalProxies, "true"),
		},
		{
			Name:        "Update Check",
			Description: "Periodically check for new releases of Coder and inform the owner. The check is performed once per day.",
			Flag:        "update-check",
			Env:         "CODER_UPDATE_CHECK",
			Default: strconv.FormatBool(
				flag.Lookup("test.v") == nil && !buildinfo.IsDev(),
			),
			Value: &c.UpdateCheck,
			YAML:  "updateCheck",
		},
		{
			Name:        "Max Token Lifetime",
			Description: "The maximum lifetime duration users can specify when creating an API token.",
			Flag:        "max-token-lifetime",
			Env:         "CODER_MAX_TOKEN_LIFETIME",
			// The default value is essentially "forever", so just use 100 years.
			// We have to add in the 25 leap days for the frontend to show the
			// "100 years" correctly.
			Default:     ((100 * 365 * time.Hour * 24) + (25 * time.Hour * 24)).String(),
			Value:       &c.Sessions.MaximumTokenDuration,
			Group:       &deploymentGroupNetworkingHTTP,
			YAML:        "maxTokenLifetime",
			Annotations: serpent.Annotations{}.Mark(annotationFormatDuration, "true"),
		},
		{
			Name:        "Enable swagger endpoint",
			Description: "Expose the swagger endpoint via /swagger.",
			Flag:        "swagger-enable",
			Env:         "CODER_SWAGGER_ENABLE",

			Value: &c.Swagger.Enable,
			YAML:  "enableSwagger",
		},
		{
			Name:        "Proxy Trusted Headers",
			Flag:        "proxy-trusted-headers",
			Env:         "CODER_PROXY_TRUSTED_HEADERS",
			Description: "Headers to trust for forwarding IP addresses. e.g. Cf-Connecting-Ip, True-Client-Ip, X-Forwarded-For.",
			Value:       &c.ProxyTrustedHeaders,
			Group:       &deploymentGroupNetworking,
			YAML:        "proxyTrustedHeaders",
			Annotations: serpent.Annotations{}.Mark(annotationExternalProxies, "true"),
		},
		{
			Name:        "Proxy Trusted Origins",
			Flag:        "proxy-trusted-origins",
			Env:         "CODER_PROXY_TRUSTED_ORIGINS",
			Description: "Origin addresses to respect \"proxy-trusted-headers\". e.g. 192.168.1.0/24.",
			Value:       &c.ProxyTrustedOrigins,
			Group:       &deploymentGroupNetworking,
			YAML:        "proxyTrustedOrigins",
			Annotations: serpent.Annotations{}.Mark(annotationExternalProxies, "true"),
		},
		{
			Name:        "Cache Directory",
			Description: "The directory to cache temporary files. If unspecified and $CACHE_DIRECTORY is set, it will be used for compatibility with systemd.",
			Flag:        "cache-dir",
			Env:         "CODER_CACHE_DIRECTORY",
			Default:     DefaultCacheDir(),
			Value:       &c.CacheDir,
			YAML:        "cacheDir",
		},
		{
			Name:        "In Memory Database",
			Description: "Controls whether data will be stored in an in-memory database.",
			Flag:        "in-memory",
			Env:         "CODER_IN_MEMORY",
			Hidden:      true,
			Value:       &c.InMemoryDatabase,
			YAML:        "inMemoryDatabase",
		},
		{
			Name:        "Postgres Connection URL",
			Description: "URL of a PostgreSQL database. If empty, PostgreSQL binaries will be downloaded from Maven (https://repo1.maven.org/maven2) and store all data in the config root. Access the built-in database with \"coder server postgres-builtin-url\".",
			Flag:        "postgres-url",
			Env:         "CODER_PG_CONNECTION_URL",
			Annotations: serpent.Annotations{}.Mark(annotationSecretKey, "true"),
			Value:       &c.PostgresURL,
		},
		{
			Name:        "Postgres Auth",
			Description: "Type of auth to use when connecting to postgres.",
			Flag:        "postgres-auth",
			Env:         "CODER_PG_AUTH",
			Default:     "password",
			Value:       serpent.EnumOf(&c.PostgresAuth, PostgresAuthDrivers...),
			YAML:        "pgAuth",
		},
		{
			Name:        "Secure Auth Cookie",
			Description: "Controls if the 'Secure' property is set on browser session cookies.",
			Flag:        "secure-auth-cookie",
			Env:         "CODER_SECURE_AUTH_COOKIE",
			Value:       &c.SecureAuthCookie,
			Group:       &deploymentGroupNetworking,
			YAML:        "secureAuthCookie",
			Annotations: serpent.Annotations{}.Mark(annotationExternalProxies, "true"),
		},
		{
			Name:        "Terms of Service URL",
			Description: "A URL to an external Terms of Service that must be accepted by users when logging in.",
			Flag:        "terms-of-service-url",
			Env:         "CODER_TERMS_OF_SERVICE_URL",
			YAML:        "termsOfServiceURL",
			Value:       &c.TermsOfServiceURL,
		},
		{
			Name: "Strict-Transport-Security",
			Description: "Controls if the 'Strict-Transport-Security' header is set on all static file responses. " +
				"This header should only be set if the server is accessed via HTTPS. This value is the MaxAge in seconds of " +
				"the header.",
			Default:     "0",
			Flag:        "strict-transport-security",
			Env:         "CODER_STRICT_TRANSPORT_SECURITY",
			Value:       &c.StrictTransportSecurity,
			Group:       &deploymentGroupNetworkingTLS,
			YAML:        "strictTransportSecurity",
			Annotations: serpent.Annotations{}.Mark(annotationExternalProxies, "true"),
		},
		{
			Name: "Strict-Transport-Security Options",
			Description: "Two optional fields can be set in the Strict-Transport-Security header; 'includeSubDomains' and 'preload'. " +
				"The 'strict-transport-security' flag must be set to a non-zero value for these options to be used.",
			Flag:        "strict-transport-security-options",
			Env:         "CODER_STRICT_TRANSPORT_SECURITY_OPTIONS",
			Value:       &c.StrictTransportSecurityOptions,
			Group:       &deploymentGroupNetworkingTLS,
			YAML:        "strictTransportSecurityOptions",
			Annotations: serpent.Annotations{}.Mark(annotationExternalProxies, "true"),
		},
		{
			Name:        "SSH Keygen Algorithm",
			Description: "The algorithm to use for generating ssh keys. Accepted values are \"ed25519\", \"ecdsa\", or \"rsa4096\".",
			Flag:        "ssh-keygen-algorithm",
			Env:         "CODER_SSH_KEYGEN_ALGORITHM",
			Default:     "ed25519",
			Value:       &c.SSHKeygenAlgorithm,
			YAML:        "sshKeygenAlgorithm",
		},
		{
			Name:        "Metrics Cache Refresh Interval",
			Description: "How frequently metrics are refreshed.",
			Flag:        "metrics-cache-refresh-interval",
			Env:         "CODER_METRICS_CACHE_REFRESH_INTERVAL",
			Hidden:      true,
			Default:     (4 * time.Hour).String(),
			Value:       &c.MetricsCacheRefreshInterval,
			Annotations: serpent.Annotations{}.Mark(annotationFormatDuration, "true"),
		},
		{
			Name:        "Agent Stat Refresh Interval",
			Description: "How frequently agent stats are recorded.",
			Flag:        "agent-stats-refresh-interval",
			Env:         "CODER_AGENT_STATS_REFRESH_INTERVAL",
			Hidden:      true,
			Default:     (30 * time.Second).String(),
			Value:       &c.AgentStatRefreshInterval,
			Annotations: serpent.Annotations{}.Mark(annotationFormatDuration, "true"),
		},
		{
			Name:        "Agent Fallback Troubleshooting URL",
			Description: "URL to use for agent troubleshooting when not set in the template.",
			Flag:        "agent-fallback-troubleshooting-url",
			Env:         "CODER_AGENT_FALLBACK_TROUBLESHOOTING_URL",
			Hidden:      true,
			Default:     "https://coder.com/docs/v2/latest/templates/troubleshooting",
			Value:       &c.AgentFallbackTroubleshootingURL,
			YAML:        "agentFallbackTroubleshootingURL",
		},
		{
			Name:        "Browser Only",
			Description: "Whether Coder only allows connections to workspaces via the browser.",
			Flag:        "browser-only",
			Env:         "CODER_BROWSER_ONLY",
			Annotations: serpent.Annotations{}.Mark(annotationEnterpriseKey, "true"),
			Value:       &c.BrowserOnly,
			Group:       &deploymentGroupNetworking,
			YAML:        "browserOnly",
		},
		{
			Name:        "SCIM API Key",
			Description: "Enables SCIM and sets the authentication header for the built-in SCIM server. New users are automatically created with OIDC authentication.",
			Flag:        "scim-auth-header",
			Env:         "CODER_SCIM_AUTH_HEADER",
			Annotations: serpent.Annotations{}.Mark(annotationEnterpriseKey, "true").Mark(annotationSecretKey, "true"),
			Value:       &c.SCIMAPIKey,
		},
		{
			Name:        "External Token Encryption Keys",
			Description: "Encrypt OIDC and Git authentication tokens with AES-256-GCM in the database. The value must be a comma-separated list of base64-encoded keys. Each key, when base64-decoded, must be exactly 32 bytes in length. The first key will be used to encrypt new values. Subsequent keys will be used as a fallback when decrypting. During normal operation it is recommended to only set one key unless you are in the process of rotating keys with the `coder server dbcrypt rotate` command.",
			Flag:        "external-token-encryption-keys",
			Env:         "CODER_EXTERNAL_TOKEN_ENCRYPTION_KEYS",
			Annotations: serpent.Annotations{}.Mark(annotationEnterpriseKey, "true").Mark(annotationSecretKey, "true"),
			Value:       &c.ExternalTokenEncryptionKeys,
		},
		{
			Name:        "Disable Path Apps",
			Description: "Disable workspace apps that are not served from subdomains. Path-based apps can make requests to the Coder API and pose a security risk when the workspace serves malicious JavaScript. This is recommended for security purposes if a --wildcard-access-url is configured.",
			Flag:        "disable-path-apps",
			Env:         "CODER_DISABLE_PATH_APPS",

			Value:       &c.DisablePathApps,
			YAML:        "disablePathApps",
			Annotations: serpent.Annotations{}.Mark(annotationExternalProxies, "true"),
		},
		{
			Name:        "Disable Owner Workspace Access",
			Description: "Remove the permission for the 'owner' role to have workspace execution on all workspaces. This prevents the 'owner' from ssh, apps, and terminal access based on the 'owner' role. They still have their user permissions to access their own workspaces.",
			Flag:        "disable-owner-workspace-access",
			Env:         "CODER_DISABLE_OWNER_WORKSPACE_ACCESS",

			Value:       &c.DisableOwnerWorkspaceExec,
			YAML:        "disableOwnerWorkspaceAccess",
			Annotations: serpent.Annotations{}.Mark(annotationExternalProxies, "true"),
		},
		{
			Name:        "Session Duration",
			Description: "The token expiry duration for browser sessions. Sessions may last longer if they are actively making requests, but this functionality can be disabled via --disable-session-expiry-refresh.",
			Flag:        "session-duration",
			Env:         "CODER_SESSION_DURATION",
			Default:     (24 * time.Hour).String(),
			Value:       &c.Sessions.DefaultDuration,
			Group:       &deploymentGroupNetworkingHTTP,
			YAML:        "sessionDuration",
			Annotations: serpent.Annotations{}.Mark(annotationFormatDuration, "true"),
		},
		{
			Name:        "Disable Session Expiry Refresh",
			Description: "Disable automatic session expiry bumping due to activity. This forces all sessions to become invalid after the session expiry duration has been reached.",
			Flag:        "disable-session-expiry-refresh",
			Env:         "CODER_DISABLE_SESSION_EXPIRY_REFRESH",

			Value: &c.Sessions.DisableExpiryRefresh,
			Group: &deploymentGroupNetworkingHTTP,
			YAML:  "disableSessionExpiryRefresh",
		},
		{
			Name:        "Disable Password Authentication",
			Description: "Disable password authentication. This is recommended for security purposes in production deployments that rely on an identity provider. Any user with the owner role will be able to sign in with their password regardless of this setting to avoid potential lock out. If you are locked out of your account, you can use the `coder server create-admin` command to create a new admin user directly in the database.",
			Flag:        "disable-password-auth",
			Env:         "CODER_DISABLE_PASSWORD_AUTH",

			Value: &c.DisablePasswordAuth,
			Group: &deploymentGroupNetworkingHTTP,
			YAML:  "disablePasswordAuth",
		},
		{
			Name:          "Config Path",
			Description:   `Specify a YAML file to load configuration from.`,
			Flag:          "config",
			Env:           "CODER_CONFIG_PATH",
			FlagShorthand: "c",
			Hidden:        false,
			Group:         &deploymentGroupConfig,
			Value:         &c.Config,
		},
		{
			Name:        "SSH Host Prefix",
			Description: "The SSH deployment prefix is used in the Host of the ssh config.",
			Flag:        "ssh-hostname-prefix",
			Env:         "CODER_SSH_HOSTNAME_PREFIX",
			YAML:        "sshHostnamePrefix",
			Group:       &deploymentGroupClient,
			Value:       &c.SSHConfig.DeploymentName,
			Hidden:      false,
			Default:     "coder.",
		},
		{
			Name: "SSH Config Options",
			Description: "These SSH config options will override the default SSH config options. " +
				"Provide options in \"key=value\" or \"key value\" format separated by commas." +
				"Using this incorrectly can break SSH to your deployment, use cautiously.",
			Flag:   "ssh-config-options",
			Env:    "CODER_SSH_CONFIG_OPTIONS",
			YAML:   "sshConfigOptions",
			Group:  &deploymentGroupClient,
			Value:  &c.SSHConfig.SSHConfigOptions,
			Hidden: false,
		},
		{
			Name:        "CLI Upgrade Message",
			Description: "The upgrade message to display to users when a client/server mismatch is detected. By default it instructs users to update using 'curl -L https://coder.com/install.sh | sh'.",
			Flag:        "cli-upgrade-message",
			Env:         "CODER_CLI_UPGRADE_MESSAGE",
			YAML:        "cliUpgradeMessage",
			Group:       &deploymentGroupClient,
			Value:       &c.CLIUpgradeMessage,
			Hidden:      false,
		},
		{
			Name: "Write Config",
			Description: `
Write out the current server config as YAML to stdout.`,
			Flag:        "write-config",
			Group:       &deploymentGroupConfig,
			Hidden:      false,
			Value:       &c.WriteConfig,
			Annotations: serpent.Annotations{}.Mark(annotationExternalProxies, "true"),
		},
		{
			Name:        "Support Links",
			Description: "Support links to display in the top right drop down menu.",
			Env:         "CODER_SUPPORT_LINKS",
			Flag:        "support-links",
			YAML:        "supportLinks",
			Value:       &c.Support.Links,
			Hidden:      false,
		},
		{
			// Env handling is done in cli.ReadGitAuthFromEnvironment
			Name:        "External Auth Providers",
			Description: "External Authentication providers.",
			YAML:        "externalAuthProviders",
			Flag:        "external-auth-providers",
			Value:       &c.ExternalAuthConfigs,
			Hidden:      true,
		},
		{
			Name:        "Custom wgtunnel Host",
			Description: `Hostname of HTTPS server that runs https://github.com/coder/wgtunnel. By default, this will pick the best available wgtunnel server hosted by Coder. e.g. "tunnel.example.com".`,
			Flag:        "wg-tunnel-host",
			Env:         "WGTUNNEL_HOST",
			YAML:        "wgtunnelHost",
			Value:       &c.WgtunnelHost,
			Default:     "", // empty string means pick best server
			Hidden:      true,
		},
		{
			Name:        "Proxy Health Check Interval",
			Description: "The interval in which coderd should be checking the status of workspace proxies.",
			Flag:        "proxy-health-interval",
			Env:         "CODER_PROXY_HEALTH_INTERVAL",
			Default:     (time.Minute).String(),
			Value:       &c.ProxyHealthStatusInterval,
			Group:       &deploymentGroupNetworkingHTTP,
			YAML:        "proxyHealthInterval",
			Annotations: serpent.Annotations{}.Mark(annotationFormatDuration, "true"),
		},
		{
			Name:        "Default Quiet Hours Schedule",
			Description: "The default daily cron schedule applied to users that haven't set a custom quiet hours schedule themselves. The quiet hours schedule determines when workspaces will be force stopped due to the template's autostop requirement, and will round the max deadline up to be within the user's quiet hours window (or default). The format is the same as the standard cron format, but the day-of-month, month and day-of-week must be *. Only one hour and minute can be specified (ranges or comma separated values are not supported).",
			Flag:        "default-quiet-hours-schedule",
			Env:         "CODER_QUIET_HOURS_DEFAULT_SCHEDULE",
			Default:     "CRON_TZ=UTC 0 0 * * *",
			Value:       &c.UserQuietHoursSchedule.DefaultSchedule,
			Group:       &deploymentGroupUserQuietHoursSchedule,
			YAML:        "defaultQuietHoursSchedule",
		},
		{
			Name:        "Allow Custom Quiet Hours",
			Description: "Allow users to set their own quiet hours schedule for workspaces to stop in (depending on template autostop requirement settings). If false, users can't change their quiet hours schedule and the site default is always used.",
			Flag:        "allow-custom-quiet-hours",
			Env:         "CODER_ALLOW_CUSTOM_QUIET_HOURS",
			Default:     "true",
			Value:       &c.UserQuietHoursSchedule.AllowUserCustom,
			Group:       &deploymentGroupUserQuietHoursSchedule,
			YAML:        "allowCustomQuietHours",
		},
		{
			Name:        "Web Terminal Renderer",
			Description: "The renderer to use when opening a web terminal. Valid values are 'canvas', 'webgl', or 'dom'.",
			Flag:        "web-terminal-renderer",
			Env:         "CODER_WEB_TERMINAL_RENDERER",
			Default:     "canvas",
			Value:       &c.WebTerminalRenderer,
			Group:       &deploymentGroupClient,
			YAML:        "webTerminalRenderer",
		},
		{
			Name:        "Allow Workspace Renames",
			Description: "DEPRECATED: Allow users to rename their workspaces. Use only for temporary compatibility reasons, this will be removed in a future release.",
			Flag:        "allow-workspace-renames",
			Env:         "CODER_ALLOW_WORKSPACE_RENAMES",
			Default:     "false",
			Value:       &c.AllowWorkspaceRenames,
			YAML:        "allowWorkspaceRenames",
		},
		// Healthcheck Options
		{
			Name:        "Health Check Refresh",
			Description: "Refresh interval for healthchecks.",
			Flag:        "health-check-refresh",
			Env:         "CODER_HEALTH_CHECK_REFRESH",
			Default:     (10 * time.Minute).String(),
			Value:       &c.Healthcheck.Refresh,
			Group:       &deploymentGroupIntrospectionHealthcheck,
			YAML:        "refresh",
			Annotations: serpent.Annotations{}.Mark(annotationFormatDuration, "true"),
		},
		{
			Name:        "Health Check Threshold: Database",
			Description: "The threshold for the database health check. If the median latency of the database exceeds this threshold over 5 attempts, the database is considered unhealthy. The default value is 15ms.",
			Flag:        "health-check-threshold-database",
			Env:         "CODER_HEALTH_CHECK_THRESHOLD_DATABASE",
			Default:     (15 * time.Millisecond).String(),
			Value:       &c.Healthcheck.ThresholdDatabase,
			Group:       &deploymentGroupIntrospectionHealthcheck,
			YAML:        "thresholdDatabase",
			Annotations: serpent.Annotations{}.Mark(annotationFormatDuration, "true"),
		},
	}

	return opts
}

type SupportConfig struct {
	Links serpent.Struct[[]LinkConfig] `json:"links" typescript:",notnull"`
}

type LinkConfig struct {
	Name   string `json:"name" yaml:"name"`
	Target string `json:"target" yaml:"target"`
	Icon   string `json:"icon" yaml:"icon" enums:"bug,chat,docs"`
}

// DeploymentOptionsWithoutSecrets returns a copy of the OptionSet with secret values omitted.
func DeploymentOptionsWithoutSecrets(set serpent.OptionSet) serpent.OptionSet {
	cpy := make(serpent.OptionSet, 0, len(set))
	for _, opt := range set {
		cpyOpt := opt
		if IsSecretDeploymentOption(cpyOpt) {
			cpyOpt.Value = nil
		}
		cpy = append(cpy, cpyOpt)
	}
	return cpy
}

// WithoutSecrets returns a copy of the config without secret values.
func (c *DeploymentValues) WithoutSecrets() (*DeploymentValues, error) {
	var ff DeploymentValues

	// Create copy via JSON.
	byt, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(byt, &ff)
	if err != nil {
		return nil, err
	}

	for _, opt := range ff.Options() {
		if !IsSecretDeploymentOption(opt) {
			continue
		}

		// This only works with string values for now.
		switch v := opt.Value.(type) {
		case *serpent.String, *serpent.StringArray:
			err := v.Set("")
			if err != nil {
				panic(err)
			}
		default:
			return nil, xerrors.Errorf("unsupported type %T", v)
		}
	}

	return &ff, nil
}

// DeploymentConfig returns the deployment config for the coder server.
func (c *Client) DeploymentConfig(ctx context.Context) (*DeploymentConfig, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/deployment/config", nil)
	if err != nil {
		return nil, xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}

	conf := &DeploymentValues{}
	resp := &DeploymentConfig{
		Values:  conf,
		Options: conf.Options(),
	}
	return resp, json.NewDecoder(res.Body).Decode(resp)
}

func (c *Client) DeploymentStats(ctx context.Context) (DeploymentStats, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/deployment/stats", nil)
	if err != nil {
		return DeploymentStats{}, xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return DeploymentStats{}, ReadBodyAsError(res)
	}

	var df DeploymentStats
	return df, json.NewDecoder(res.Body).Decode(&df)
}

type AppearanceConfig struct {
	ApplicationName string              `json:"application_name"`
	LogoURL         string              `json:"logo_url"`
	ServiceBanner   ServiceBannerConfig `json:"service_banner"`
	SupportLinks    []LinkConfig        `json:"support_links,omitempty"`
}

type UpdateAppearanceConfig struct {
	ApplicationName string              `json:"application_name"`
	LogoURL         string              `json:"logo_url"`
	ServiceBanner   ServiceBannerConfig `json:"service_banner"`
}

type ServiceBannerConfig struct {
	Enabled         bool   `json:"enabled"`
	Message         string `json:"message,omitempty"`
	BackgroundColor string `json:"background_color,omitempty"`
}

// Appearance returns the configuration that modifies the visual
// display of the dashboard.
func (c *Client) Appearance(ctx context.Context) (AppearanceConfig, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/appearance", nil)
	if err != nil {
		return AppearanceConfig{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return AppearanceConfig{}, ReadBodyAsError(res)
	}
	var cfg AppearanceConfig
	return cfg, json.NewDecoder(res.Body).Decode(&cfg)
}

func (c *Client) UpdateAppearance(ctx context.Context, appearance UpdateAppearanceConfig) error {
	res, err := c.Request(ctx, http.MethodPut, "/api/v2/appearance", appearance)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ReadBodyAsError(res)
	}
	return nil
}

// BuildInfoResponse contains build information for this instance of Coder.
type BuildInfoResponse struct {
	// ExternalURL references the current Coder version.
	// For production builds, this will link directly to a release. For development builds, this will link to a commit.
	ExternalURL string `json:"external_url"`
	// Version returns the semantic version of the build.
	Version string `json:"version"`

	// DashboardURL is the URL to hit the deployment's dashboard.
	// For external workspace proxies, this is the coderd they are connected
	// to.
	DashboardURL string `json:"dashboard_url"`

	WorkspaceProxy bool `json:"workspace_proxy"`

	// AgentAPIVersion is the current version of the Agent API (back versions
	// MAY still be supported).
	AgentAPIVersion string `json:"agent_api_version"`

	// UpgradeMessage is the message displayed to users when an outdated client
	// is detected.
	UpgradeMessage string `json:"upgrade_message"`

	// DeploymentID is the unique identifier for this deployment.
	DeploymentID string `json:"deployment_id"`
}

type WorkspaceProxyBuildInfo struct {
	// TODO: @emyrk what should we include here?
	WorkspaceProxy bool `json:"workspace_proxy"`
	// DashboardURL is the URL of the coderd this proxy is connected to.
	DashboardURL string `json:"dashboard_url"`
}

// CanonicalVersion trims build information from the version.
// E.g. 'v0.7.4-devel+11573034' -> 'v0.7.4'.
func (b BuildInfoResponse) CanonicalVersion() string {
	// We do a little hack here to massage the string into a form
	// that works well with semver.
	trimmed := strings.ReplaceAll(b.Version, "-devel+", "+devel-")
	return semver.Canonical(trimmed)
}

// BuildInfo returns build information for this instance of Coder.
func (c *Client) BuildInfo(ctx context.Context) (BuildInfoResponse, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/buildinfo", nil)
	if err != nil {
		return BuildInfoResponse{}, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK || ExpectJSONMime(res) != nil {
		return BuildInfoResponse{}, ReadBodyAsError(res)
	}

	var buildInfo BuildInfoResponse
	return buildInfo, json.NewDecoder(res.Body).Decode(&buildInfo)
}

type Experiment string

const (
	// Add new experiments here!
	ExperimentExample            Experiment = "example" // This isn't used for anything.
	ExperimentSharedPorts        Experiment = "shared-ports"
	ExperimentAutoFillParameters Experiment = "auto-fill-parameters" // This should not be taken out of experiments until we have redesigned the feature.
)

// ExperimentsAll should include all experiments that are safe for
// users to opt-in to via --experimental='*'.
// Experiments that are not ready for consumption by all users should
// not be included here and will be essentially hidden.
var ExperimentsAll = Experiments{
	ExperimentSharedPorts,
}

// Experiments is a list of experiments.
// Multiple experiments may be enabled at the same time.
// Experiments are not safe for production use, and are not guaranteed to
// be backwards compatible. They may be removed or renamed at any time.
type Experiments []Experiment

// Returns a list of experiments that are enabled for the deployment.
func (e Experiments) Enabled(ex Experiment) bool {
	for _, v := range e {
		if v == ex {
			return true
		}
	}
	return false
}

func (c *Client) Experiments(ctx context.Context) (Experiments, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/experiments", nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var exp []Experiment
	return exp, json.NewDecoder(res.Body).Decode(&exp)
}

// AvailableExperiments is an expandable type that returns all safe experiments
// available to be used with a deployment.
type AvailableExperiments struct {
	Safe []Experiment `json:"safe"`
}

func (c *Client) SafeExperiments(ctx context.Context) (AvailableExperiments, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/experiments/available", nil)
	if err != nil {
		return AvailableExperiments{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return AvailableExperiments{}, ReadBodyAsError(res)
	}
	var exp AvailableExperiments
	return exp, json.NewDecoder(res.Body).Decode(&exp)
}

type DAUsResponse struct {
	Entries      []DAUEntry `json:"entries"`
	TZHourOffset int        `json:"tz_hour_offset"`
}

type DAUEntry struct {
	// Date is a string formatted as 2024-01-31.
	// Timezone and time information is not included.
	Date   string `json:"date"`
	Amount int    `json:"amount"`
}

type DAURequest struct {
	TZHourOffset int
}

func (d DAURequest) asRequestOption() RequestOption {
	return func(r *http.Request) {
		q := r.URL.Query()
		q.Set("tz_offset", strconv.Itoa(d.TZHourOffset))
		r.URL.RawQuery = q.Encode()
	}
}

// TimezoneOffsetHourWithTime is implemented to match the javascript 'getTimezoneOffset()' function.
// This is the amount of time between this date evaluated in UTC and evaluated in the 'loc'
// The trivial case of times being on the same day is:
// 'time.Now().UTC().Hour() - time.Now().In(loc).Hour()'
func TimezoneOffsetHourWithTime(now time.Time, loc *time.Location) int {
	if loc == nil {
		// Default to UTC time to be consistent across all callers.
		loc = time.UTC
	}
	_, offsetSec := now.In(loc).Zone()
	// Convert to hours and flip the sign
	return -1 * offsetSec / 60 / 60
}

func TimezoneOffsetHour(loc *time.Location) int {
	return TimezoneOffsetHourWithTime(time.Now(), loc)
}

func (c *Client) DeploymentDAUsLocalTZ(ctx context.Context) (*DAUsResponse, error) {
	return c.DeploymentDAUs(ctx, TimezoneOffsetHour(time.Local))
}

// DeploymentDAUs requires a tzOffset in hours. Use 0 for UTC, and TimezoneOffsetHour(time.Local) for the
// local timezone.
func (c *Client) DeploymentDAUs(ctx context.Context, tzOffset int) (*DAUsResponse, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/insights/daus", nil, DAURequest{
		TZHourOffset: tzOffset,
	}.asRequestOption())
	if err != nil {
		return nil, xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}

	var resp DAUsResponse
	return &resp, json.NewDecoder(res.Body).Decode(&resp)
}

type AppHostResponse struct {
	// Host is the externally accessible URL for the Coder instance.
	Host string `json:"host"`
}

// AppHost returns the site-wide application wildcard hostname
// e.g. "*--apps.coder.com". Apps are accessible at:
// "<app-name>--<agent-name>--<workspace-name>--<username><app-host>", e.g.
// "my-app--agent--workspace--username--apps.coder.com".
//
// If the app host is not set, the response will contain an empty string.
func (c *Client) AppHost(ctx context.Context) (AppHostResponse, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/applications/host", nil)
	if err != nil {
		return AppHostResponse{}, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return AppHostResponse{}, ReadBodyAsError(res)
	}

	var host AppHostResponse
	return host, json.NewDecoder(res.Body).Decode(&host)
}

type WorkspaceConnectionLatencyMS struct {
	P50 float64
	P95 float64
}

type WorkspaceDeploymentStats struct {
	Pending  int64 `json:"pending"`
	Building int64 `json:"building"`
	Running  int64 `json:"running"`
	Failed   int64 `json:"failed"`
	Stopped  int64 `json:"stopped"`

	ConnectionLatencyMS WorkspaceConnectionLatencyMS `json:"connection_latency_ms"`
	RxBytes             int64                        `json:"rx_bytes"`
	TxBytes             int64                        `json:"tx_bytes"`
}

type SessionCountDeploymentStats struct {
	VSCode          int64 `json:"vscode"`
	SSH             int64 `json:"ssh"`
	JetBrains       int64 `json:"jetbrains"`
	ReconnectingPTY int64 `json:"reconnecting_pty"`
}

type DeploymentStats struct {
	// AggregatedFrom is the time in which stats are aggregated from.
	// This might be back in time a specific duration or interval.
	AggregatedFrom time.Time `json:"aggregated_from" format:"date-time"`
	// CollectedAt is the time in which stats are collected at.
	CollectedAt time.Time `json:"collected_at" format:"date-time"`
	// NextUpdateAt is the time when the next batch of stats will
	// be updated.
	NextUpdateAt time.Time `json:"next_update_at" format:"date-time"`

	Workspaces   WorkspaceDeploymentStats    `json:"workspaces"`
	SessionCount SessionCountDeploymentStats `json:"session_count"`
}

type SSHConfigResponse struct {
	HostnamePrefix   string            `json:"hostname_prefix"`
	SSHConfigOptions map[string]string `json:"ssh_config_options"`
}

// SSHConfiguration returns information about the SSH configuration for the
// Coder instance.
func (c *Client) SSHConfiguration(ctx context.Context) (SSHConfigResponse, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/deployment/ssh", nil)
	if err != nil {
		return SSHConfigResponse{}, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return SSHConfigResponse{}, ReadBodyAsError(res)
	}

	var sshConfig SSHConfigResponse
	return sshConfig, json.NewDecoder(res.Body).Decode(&sshConfig)
}

package codersdk

import (
	"context"
	"encoding/json"
	"flag"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/mod/semver"
	"golang.org/x/xerrors"

	"github.com/coreos/go-oidc/v3/oidc"

	"github.com/coder/coder/buildinfo"
	"github.com/coder/coder/cli/bigcli"
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
	FeatureHighAvailability           FeatureName = "high_availability"
	FeatureMultipleGitAuth            FeatureName = "multiple_git_auth"
	FeatureExternalProvisionerDaemons FeatureName = "external_provisioner_daemons"
	FeatureAppearance                 FeatureName = "appearance"
)

// FeatureNames must be kept in-sync with the Feature enum above.
var FeatureNames = []FeatureName{
	FeatureUserLimit,
	FeatureAuditLog,
	FeatureBrowserOnly,
	FeatureSCIM,
	FeatureTemplateRBAC,
	FeatureHighAvailability,
	FeatureMultipleGitAuth,
	FeatureExternalProvisionerDaemons,
	FeatureAppearance,
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
		FeatureMultipleGitAuth:            true,
		FeatureExternalProvisionerDaemons: true,
		FeatureAppearance:                 true,
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

	// DEPRECATED: use Experiments instead.
	Experimental bool `json:"experimental"`
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

// DeploymentConfig is the central configuration for the coder server.
type DeploymentConfig struct {
	AccessURL                       bigcli.URL
	WildcardAccessURL               bigcli.URL
	RedirectToAccessURL             bigcli.Bool
	HTTPAddress                     bigcli.BindAddress `json:"http_address" typescript:",notnull"`
	AutobuildPollInterval           bigcli.Duration
	DERP                            *DERP              `json:"derp" typescript:",notnull"`
	Prometheus                      *PrometheusConfig  `json:"prometheus" typescript:",notnull"`
	Pprof                           *PprofConfig       `json:"pprof" typescript:",notnull"`
	ProxyTrustedHeaders             bigcli.Strings     `json:"proxy_trusted_headers" typescript:",notnull"`
	ProxyTrustedOrigins             bigcli.Strings     `json:"proxy_trusted_origins" typescript:",notnull"`
	CacheDir                        bigcli.String      `json:"cache_directory" typescript:",notnull"`
	InMemoryDatabase                bigcli.Bool        `json:"in_memory_database" typescript:",notnull"`
	PostgresURL                     bigcli.String      `json:"pg_connection_url" typescript:",notnull"`
	OAuth2                          *OAuth2Config      `json:"oauth2" typescript:",notnull"`
	OIDC                            *OIDCConfig        `json:"oidc" typescript:",notnull"`
	Telemetry                       *TelemetryConfig   `json:"telemetry" typescript:",notnull"`
	TLS                             *TLSConfig         `json:"tls" typescript:",notnull"`
	Trace                           *TraceConfig       `json:"trace" typescript:",notnull"`
	SecureAuthCookie                bigcli.Bool        `json:"secure_auth_cookie" typescript:",notnull"`
	StrictTransportSecurity         bigcli.Int64       `json:"strict_transport_security" typescript:",notnull"`
	StrictTransportSecurityOptions  bigcli.Strings     `json:"strict_transport_security_options" typescript:",notnull"`
	SSHKeygenAlgorithm              bigcli.String      `json:"ssh_keygen_algorithm" typescript:",notnull"`
	MetricsCacheRefreshInterval     bigcli.Duration    `json:"metrics_cache_refresh_interval" typescript:",notnull"`
	AgentStatRefreshInterval        bigcli.Duration    `json:"agent_stat_refresh_interval" typescript:",notnull"`
	AgentFallbackTroubleshootingURL bigcli.URL         `json:"agent_fallback_troubleshooting_url" typescript:",notnull"`
	AuditLogging                    bigcli.Bool        `json:"audit_logging" typescript:",notnull"`
	BrowserOnly                     bigcli.Bool        `json:"browser_only" typescript:",notnull"`
	SCIMAPIKey                      bigcli.String      `json:"scim_api_key" typescript:",notnull"`
	Provisioner                     *ProvisionerConfig `json:"provisioner" typescript:",notnull"`
	RateLimit                       *RateLimitConfig   `json:"rate_limit" typescript:",notnull"`
	Experiments                     bigcli.Strings     `json:"experiments" typescript:",notnull"`
	UpdateCheck                     bigcli.Bool        `json:"update_check" typescript:",notnull"`
	MaxTokenLifetime                bigcli.Duration    `json:"max_token_lifetime" typescript:",notnull"`
	Swagger                         *SwaggerConfig     `json:"swagger" typescript:",notnull"`
	Logging                         *LoggingConfig     `json:"logging" typescript:",notnull"`
	Dangerous                       *DangerousConfig   `json:"dangerous" typescript:",notnull"`
	DisablePathApps                 bigcli.Bool        `json:"disable_path_apps" typescript:",notnull"`
	SessionDuration                 bigcli.Duration    `json:"max_session_expiry" typescript:",notnull"`
	DisableSessionExpiryRefresh     bigcli.Bool        `json:"disable_session_expiry_refresh" typescript:",notnull"`
	DisablePasswordAuth             bigcli.Bool        `json:"disable_password_auth" typescript:",notnull"`

	// DEPRECATED: Use HTTPAddress or TLS.Address instead.
	Address bigcli.BindAddress `json:"address" typescript:",notnull"`
	// DEPRECATED: Use Experiments instead.
	Experimental bigcli.Bool `json:"experimental" typescript:",notnull"`
}

// NewDeploymentConfig returns a new DeploymentConfig without any nil fields.
func NewDeploymentConfig() *DeploymentConfig {
	return &DeploymentConfig{
		TLS:         &TLSConfig{},
		Logging:     &LoggingConfig{},
		Provisioner: &ProvisionerConfig{},
		RateLimit:   &RateLimitConfig{},
		Dangerous:   &DangerousConfig{},
		Trace:       &TraceConfig{},
		Telemetry:   &TelemetryConfig{},
		OIDC:        &OIDCConfig{},
		OAuth2: &OAuth2Config{
			Github: &OAuth2GithubConfig{},
		},
		Pprof:      &PprofConfig{},
		Prometheus: &PrometheusConfig{},
		DERP: &DERP{
			Server: &DERPServerConfig{},
			Config: &DERPConfig{},
		},
		Swagger: &SwaggerConfig{},
	}
}

type DERP struct {
	Server *DERPServerConfig `json:"server" typescript:",notnull"`
	Config *DERPConfig       `json:"config" typescript:",notnull"`
}

type DERPServerConfig struct {
	Enable        bigcli.Bool    `json:"enable" typescript:",notnull"`
	RegionID      bigcli.Int64   `json:"region_id" typescript:",notnull"`
	RegionCode    bigcli.String  `json:"region_code" typescript:",notnull"`
	RegionName    bigcli.String  `json:"region_name" typescript:",notnull"`
	STUNAddresses bigcli.Strings `json:"stun_addresses" typescript:",notnull"`
	RelayURL      bigcli.URL     `json:"relay_url" typescript:",notnull"`
}

type DERPConfig struct {
	URL  bigcli.String `json:"url" typescript:",notnull"`
	Path bigcli.String `json:"path" typescript:",notnull"`
}

type PrometheusConfig struct {
	Enable  bigcli.Bool        `json:"enable" typescript:",notnull"`
	Address bigcli.BindAddress `json:"address" typescript:",notnull"`
}

type PprofConfig struct {
	Enable  bigcli.Bool        `json:"enable" typescript:",notnull"`
	Address bigcli.BindAddress `json:"address" typescript:",notnull"`
}

type OAuth2Config struct {
	Github *OAuth2GithubConfig `json:"github" typescript:",notnull"`
}

type OAuth2GithubConfig struct {
	ClientID          bigcli.String  `json:"client_id" typescript:",notnull"`
	ClientSecret      bigcli.String  `json:"client_secret" typescript:",notnull"`
	AllowedOrgs       bigcli.Strings `json:"allowed_orgs" typescript:",notnull"`
	AllowedTeams      bigcli.Strings `json:"allowed_teams" typescript:",notnull"`
	AllowSignups      bigcli.Bool    `json:"allow_signups" typescript:",notnull"`
	AllowEveryone     bigcli.Bool    `json:"allow_everyone" typescript:",notnull"`
	EnterpriseBaseURL bigcli.String  `json:"enterprise_base_url" typescript:",notnull"`
}

type OIDCConfig struct {
	AllowSignups        bigcli.Bool    `json:"allow_signups" typescript:",notnull"`
	ClientID            bigcli.String  `json:"client_id" typescript:",notnull"`
	ClientSecret        bigcli.String  `json:"client_secret" typescript:",notnull"`
	EmailDomain         bigcli.Strings `json:"email_domain" typescript:",notnull"`
	IssuerURL           bigcli.String  `json:"issuer_url" typescript:",notnull"`
	Scopes              bigcli.Strings `json:"scopes" typescript:",notnull"`
	IgnoreEmailVerified bigcli.Bool    `json:"ignore_email_verified" typescript:",notnull"`
	UsernameField       bigcli.String  `json:"username_field" typescript:",notnull"`
	SignInText          bigcli.String  `json:"sign_in_text" typescript:",notnull"`
	IconURL             bigcli.URL     `json:"icon_url" typescript:",notnull"`
}

type TelemetryConfig struct {
	Enable bigcli.Bool `json:"enable" typescript:",notnull"`
	Trace  bigcli.Bool `json:"trace" typescript:",notnull"`
	URL    bigcli.URL  `json:"url" typescript:",notnull"`
}

type TLSConfig struct {
	Enable         bigcli.Bool        `json:"enable" typescript:",notnull"`
	Address        bigcli.BindAddress `json:"address" typescript:",notnull"`
	RedirectHTTP   bigcli.Bool        `json:"redirect_http" typescript:",notnull"`
	CertFiles      bigcli.Strings     `json:"cert_file" typescript:",notnull"`
	ClientAuth     bigcli.String      `json:"client_auth" typescript:",notnull"`
	ClientCAFile   bigcli.String      `json:"client_ca_file" typescript:",notnull"`
	KeyFiles       bigcli.Strings     `json:"key_file" typescript:",notnull"`
	MinVersion     bigcli.String      `json:"min_version" typescript:",notnull"`
	ClientCertFile bigcli.String      `json:"client_cert_file" typescript:",notnull"`
	ClientKeyFile  bigcli.String      `json:"client_key_file" typescript:",notnull"`
}

type TraceConfig struct {
	Enable          bigcli.Bool   `json:"enable" typescript:",notnull"`
	HoneycombAPIKey bigcli.String `json:"honeycomb_api_key" typescript:",notnull"`
	CaptureLogs     bigcli.Bool   `json:"capture_logs" typescript:",notnull"`
}

type GitAuthConfig struct {
	ID           string   `json:"id"`
	Type         string   `json:"type"`
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"-" yaml:"client_secret"`
	AuthURL      string   `json:"auth_url"`
	TokenURL     string   `json:"token_url"`
	ValidateURL  string   `json:"validate_url"`
	Regex        string   `json:"regex"`
	NoRefresh    bool     `json:"no_refresh"`
	Scopes       []string `json:"scopes"`
}

type ProvisionerConfig struct {
	Daemons             bigcli.Int64    `json:"daemons" typescript:",notnull"`
	DaemonPollInterval  bigcli.Duration `json:"daemon_poll_interval" typescript:",notnull"`
	DaemonPollJitter    bigcli.Duration `json:"daemon_poll_jitter" typescript:",notnull"`
	ForceCancelInterval bigcli.Duration `json:"force_cancel_interval" typescript:",notnull"`
}

type RateLimitConfig struct {
	DisableAll bigcli.Bool  `json:"disable_all" typescript:",notnull"`
	API        bigcli.Int64 `json:"api" typescript:",notnull"`
}

type SwaggerConfig struct {
	Enable bigcli.Bool `json:"enable" typescript:",notnull"`
}

type LoggingConfig struct {
	Human       bigcli.String `json:"human" typescript:",notnull"`
	JSON        bigcli.String `json:"json" typescript:",notnull"`
	Stackdriver bigcli.String `json:"stackdriver" typescript:",notnull"`
}

type DangerousConfig struct {
	AllowPathAppSharing         bigcli.Bool `json:"allow_path_app_sharing" typescript:",notnull"`
	AllowPathAppSiteOwnerAccess bigcli.Bool `json:"allow_path_app_site_owner_access" typescript:",notnull"`
}

const (
	flagEnterpriseKey = "enterprise"
	flagSecretKey     = "secret"
)

func isSecretOpt(an bigcli.Annotations) bool {
	return an.IsSet(flagSecretKey)
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

	return filepath.Join(defaultCacheDir, "coder")
}

func (c *DeploymentConfig) ConfigOptions() bigcli.OptionSet {
	httpAddress := bigcli.Option{
		Name:        "HTTP Address",
		Description: "HTTP bind address of the server. Unset to disable the HTTP endpoint.",
		Flag:        "http-address",
		Default:     "127.0.0.1:3000",
		Value:       &c.HTTPAddress,
		Group:       []string{"Networking"},
	}
	tlsBindAddress := bigcli.Option{
		Name:        "TLS Address",
		Description: "HTTPS bind address of the server.",
		Flag:        "tls-address",
		Default:     "127.0.0.1:3443",
		Value:       &c.TLS.Address,
		Group:       []string{"Networking", "TLS"},
	}
	redirectToAccessURL := bigcli.Option{
		Name:        "Redirect to Access URL",
		Description: "Specifies whether to redirect requests that do not match the access URL host.",
		Flag:        "redirect-to-access-url",
		Value:       &c.RedirectToAccessURL,
		Group:       []string{"Networking"},
	}
	return bigcli.OptionSet{
		{
			Name:        "Access URL",
			Description: `The URL that users will use to access the Coder deployment.`,
			Value:       &c.AccessURL,
			Group:       []string{"Networking"},
		},
		{
			Name:        "Wildcard Access URL",
			Description: "Specifies the wildcard hostname to use for workspace applications in the form \"*.example.com\".",
			Flag:        "wildcard-access-url",
			Value:       &c.WildcardAccessURL,
			Group:       []string{"Networking"},
		},
		redirectToAccessURL,
		{
			Name:        "Autobuild Poll Interval",
			Description: "Interval to poll for scheduled workspace builds.",
			Flag:        "autobuild-poll-interval",
			Hidden:      true,
			Default:     time.Minute.String(),
			Value:       &c.AutobuildPollInterval,
		},
		httpAddress,
		tlsBindAddress,
		{
			Name:          "Address",
			Description:   "Bind address of the server.",
			Flag:          "address",
			FlagShorthand: "a",
			Hidden:        true,
			Value:         &c.Address,
			UseInstead: []bigcli.Option{
				httpAddress,
				tlsBindAddress,
			},
			Group: []string{"Networking"},
		},
		// TLS settings
		{
			Name:        "TLS Enable",
			Description: "Whether TLS will be enabled.",
			Flag:        "tls-enable",
			Value:       &c.TLS.Enable,
			Group:       []string{"Networking", "TLS"},
		},
		{
			Name:        "Redirect HTTP to HTTPS",
			Description: "Whether HTTP requests will be redirected to the access URL (if it's a https URL and TLS is enabled). Requests to local IP addresses are never redirected regardless of this setting.",
			Flag:        "tls-redirect-http-to-https",
			Default:     "true",
			Hidden:      true,
			Value:       &c.TLS.RedirectHTTP,
			UseInstead:  []bigcli.Option{redirectToAccessURL},
			Group:       []string{"Networking", "TLS"},
		},
		{
			Name:        "TLS Certificate Files",
			Description: "Path to each certificate for TLS. It requires a PEM-encoded file. To configure the listener to use a CA certificate, concatenate the primary certificate and the CA certificate together. The primary certificate should appear first in the combined file.",
			Flag:        "tls-cert-file",
			Value:       &c.TLS.CertFiles,
			Group:       []string{"Networking", "TLS"},
		},
		{
			Name:        "TLS Client CA Files",
			Description: "PEM-encoded Certificate Authority file used for checking the authenticity of client",
			Flag:        "tls-client-ca-file",
			Value:       &c.TLS.ClientCAFile,
			Group:       []string{"Networking", "TLS"},
		},
		{
			Name:        "TLS Client Auth",
			Description: "Policy the server will follow for TLS Client Authentication. Accepted values are \"none\", \"request\", \"require-any\", \"verify-if-given\", or \"require-and-verify\".",
			Flag:        "tls-client-auth",
			Default:     "none",
			Value:       &c.TLS.ClientAuth,
			Group:       []string{"Networking", "TLS"},
		},
		{
			Name:        "TLS Key Files",
			Description: "Paths to the private keys for each of the certificates. It requires a PEM-encoded file.",
			Flag:        "tls-key-file",
			Value:       &c.TLS.KeyFiles,
			Group:       []string{"Networking", "TLS"},
		},
		{
			Name:        "TLS Minimum Version",
			Description: "Minimum supported version of TLS. Accepted values are \"tls10\", \"tls11\", \"tls12\" or \"tls13\"",
			Flag:        "tls-min-version",
			Default:     "tls12",
			Value:       &c.TLS.MinVersion,
			Group:       []string{"Networking", "TLS"},
		},
		{
			Name:        "TLS Client Cert File",
			Description: "Path to certificate for client TLS authentication. It requires a PEM-encoded file.",
			Flag:        "tls-client-cert-file",
			Value:       &c.TLS.ClientCertFile,
			Group:       []string{"Networking", "TLS"},
		},
		{
			Name:        "TLS Client Key File",
			Description: "Path to key for client TLS authentication. It requires a PEM-encoded file.",
			Flag:        "tls-client-key-file",
			Value:       &c.TLS.ClientKeyFile,
			Group:       []string{"Networking", "TLS"},
		},
		// Derp settings
		{
			Name:        "DERP Server Enable",
			Description: "Whether to enable or disable the embedded DERP relay server.",
			Flag:        "derp-server-enable",
			Default:     "true",
			Value:       &c.DERP.Server.Enable,
			Group:       []string{"Networking", "DERP"},
		},
		{
			Name:        "DERP Server Region ID",
			Description: "Region ID to use for the embedded DERP server.",
			Flag:        "derp-server-region-id",
			Default:     "999",
			Value:       &c.DERP.Server.RegionID,
			Group:       []string{"Networking", "DERP"},
		},
		{
			Name:        "DERP Server Region Code",
			Description: "Region code to use for the embedded DERP server.",
			Flag:        "derp-server-region-code",
			Default:     "coder",
			Value:       &c.DERP.Server.RegionCode,
			Group:       []string{"Networking", "DERP"},
		},
		{
			Name:        "DERP Server Region Name",
			Description: "Region name that for the embedded DERP server.",
			Flag:        "derp-server-region-name",
			Default:     "Coder Embedded Relay",
			Value:       &c.DERP.Server.RegionName,
			Group:       []string{"Networking", "DERP"},
		},
		{
			Name:        "DERP Server STUN Addresses",
			Description: "Addresses for STUN servers to establish P2P connections. Set empty to disable P2P connections.",
			Flag:        "derp-server-stun-addresses",
			Default:     "stun.l.google.com:19302",
			Value:       &c.DERP.Server.STUNAddresses,
			Group:       []string{"Networking", "DERP"},
		},
		{
			Name:        "DERP Server Relay URL",
			Description: "An HTTP URL that is accessible by other replicas to relay DERP traffic. Required for high availability.",
			Flag:        "derp-server-relay-url",
			Annotations: bigcli.Annotations{}.Mark(flagEnterpriseKey, "true"),
			Value:       &c.DERP.Server.RelayURL,
			Group:       []string{"Networking", "DERP"},
		},
		{
			Name:        "DERP Config URL",
			Description: "URL to fetch a DERP mapping on startup. See: https://tailscale.com/kb/1118/custom-derp-servers/",
			Flag:        "derp-config-url",
			Value:       &c.DERP.Config.URL,
			Group:       []string{"Networking", "DERP"},
		},
		{
			Name:        "DERP Config Path",
			Description: "Path to read a DERP mapping from. See: https://tailscale.com/kb/1118/custom-derp-servers/",
			Flag:        "derp-config-path",
			Value:       &c.DERP.Config.Path,
			Group:       []string{"Networking", "DERP"},
		},
		// TODO: support Git Auth settings.
		// Prometheus settings
		{
			Name:        "Prometheus Enable",
			Description: "Serve prometheus metrics on the address defined by prometheus address.",
			Flag:        "prometheus-enable",
			Value:       &c.Prometheus.Enable,
			Group:       []string{"Introspection"},
		},
		{
			Name:        "Prometheus Address",
			Description: "The bind address to serve prometheus metrics.",
			Flag:        "prometheus-address",
			Default:     "127.0.0.1:2112",
			Value:       &c.Prometheus.Address,
			Group:       []string{"Introspection"},
		},
		// Pprof settings
		{
			Name:        "pprof Enable",
			Description: "Serve pprof metrics on the address defined by pprof address.",
			Flag:        "pprof-enable",
			Value:       &c.Pprof.Enable,
			Group:       []string{"Introspection"},
		},
		{
			Name:        "pprof Address",
			Description: "The bind address to serve pprof.",
			Flag:        "pprof-address",
			Default:     "127.0.0.1:6060",
			Value:       &c.Pprof.Address,
			Group:       []string{"Introspection"},
		},
		// oAuth settings
		{
			Name:        "OAuth2 GitHub Client ID",
			Description: "Client ID for Login with GitHub.",
			Flag:        "oauth2-github-client-id",
			Value:       &c.OAuth2.Github.ClientID,
			Group:       []string{"oAuth2"},
		},
		{
			Name:        "OAuth2 GitHub Client Secret",
			Description: "Client secret for Login with GitHub.",
			Flag:        "oauth2-github-client-secret",
			Value:       &c.OAuth2.Github.ClientSecret,
			Annotations: bigcli.Annotations{}.Mark(flagSecretKey, "true"),
			Group:       []string{"oAuth2"},
		},
		{
			Name:        "OAuth2 GitHub Allowed Orgs",
			Description: "Organizations the user must be a member of to Login with GitHub.",
			Flag:        "oauth2-github-allowed-orgs",
			Value:       &c.OAuth2.Github.AllowedOrgs,
			Group:       []string{"oAuth2"},
		},
		{
			Name:        "OAuth2 GitHub Allowed Teams",
			Description: "Teams inside organizations the user must be a member of to Login with GitHub. Structured as: <organization-name>/<team-slug>.",
			Flag:        "oauth2-github-allowed-teams",
			Value:       &c.OAuth2.Github.AllowedTeams,
			Group:       []string{"oAuth2"},
		},
		{
			Name:        "OAuth2 GitHub Allow Signups",
			Description: "Whether new users can sign up with GitHub.",
			Flag:        "oauth2-github-allow-signups",
			Value:       &c.OAuth2.Github.AllowSignups,
			Group:       []string{"oAuth2"},
		},
		{
			Name:        "OAuth2 GitHub Allow Everyone",
			Description: "Allow all logins, setting this option means allowed orgs and teams must be empty.",
			Flag:        "oauth2-github-allow-everyone",
			Value:       &c.OAuth2.Github.AllowEveryone,
			Group:       []string{"oAuth2"},
		},
		{
			Name:        "OAuth2 GitHub Enterprise Base URL",
			Description: "Base URL of a GitHub Enterprise deployment to use for Login with GitHub.",
			Flag:        "oauth2-github-enterprise-base-url",
			Value:       &c.OAuth2.Github.EnterpriseBaseURL,
			Group:       []string{"oAuth2"},
		},
		// OIDC settings.
		{
			Name:        "OIDC Allow Signups",
			Description: "Whether new users can sign up with OIDC.",
			Flag:        "oidc-allow-signups",
			Default:     "true",
			Value:       &c.OIDC.AllowSignups,
			Group:       []string{"OIDC"},
		},
		{
			Name:        "OIDC Client ID",
			Description: "Client ID to use for Login with OIDC.",
			Flag:        "oidc-client-id",
			Value:       &c.OIDC.ClientID,
			Group:       []string{"OIDC"},
		},
		{
			Name:        "OIDC Client Secret",
			Description: "Client secret to use for Login with OIDC.",
			Flag:        "oidc-client-secret",
			Annotations: bigcli.Annotations{}.Mark(flagSecretKey, "true"),
			Value:       &c.OIDC.ClientSecret,
			Group:       []string{"OIDC"},
		},
		{
			Name:        "OIDC Email Domain",
			Description: "Email domains that clients logging in with OIDC must match.",
			Flag:        "oidc-email-domain",
			Value:       &c.OIDC.EmailDomain,
			Group:       []string{"OIDC"},
		},
		{
			Name:        "OIDC Issuer URL",
			Description: "Issuer URL to use for Login with OIDC.",
			Flag:        "oidc-issuer-url",
			Value:       &c.OIDC.IssuerURL,
			Group:       []string{"OIDC"},
		},
		{
			Name:        "OIDC Scopes",
			Description: "Scopes to grant when authenticating with OIDC.",
			Flag:        "oidc-scopes",
			Default:     strings.Join([]string{oidc.ScopeOpenID, "profile", "email"}, ","),
			Value:       &c.OIDC.Scopes,
			Group:       []string{"OIDC"},
		},
		{
			Name:        "OIDC Ignore Email Verified",
			Description: "Ignore the email_verified claim from the upstream provider.",
			Flag:        "oidc-ignore-email-verified",
			Default:     "false",
			Value:       &c.OIDC.IgnoreEmailVerified,
			Group:       []string{"OIDC"},
		},
		{
			Name:        "OIDC Username Field",
			Description: "OIDC claim field to use as the username.",
			Flag:        "oidc-username-field",
			Default:     "preferred_username",
			Value:       &c.OIDC.UsernameField,
			Group:       []string{"OIDC"},
		},
		{
			Name:        "OpenID Connect sign in text",
			Description: "The text to show on the OpenID Connect sign in button",
			Flag:        "oidc-sign-in-text",
			Default:     "OpenID Connect",
			Value:       &c.OIDC.SignInText,
			Group:       []string{"OIDC"},
		},
		{
			Name:        "OpenID connect icon URL",
			Description: "URL pointing to the icon to use on the OepnID Connect login button",
			Flag:        "oidc-icon-url",
			Value:       &c.OIDC.IconURL,
			Group:       []string{"OIDC"},
		},
		// Telemetry settings
		{
			Name:        "Telemetry Enable",
			Description: "Whether telemetry is enabled or not. Coder collects anonymized usage data to help improve our product.",
			Flag:        "telemetry",
			Default:     strconv.FormatBool(flag.Lookup("test.v") == nil),
			Value:       &c.Telemetry.Enable,
			Group:       []string{"Telemetry"},
		},
		{
			Name:        "Telemetry Trace",
			Description: "Whether Opentelemetry traces are sent to Coder. Coder collects anonymized application tracing to help improve our product. Disabling telemetry also disables this option.",
			Flag:        "telemetry-trace",
			Default:     strconv.FormatBool(flag.Lookup("test.v") == nil),
			Value:       &c.Telemetry.Trace,
			Group:       []string{"Telemetry"},
		},
		{
			Name:        "Telemetry URL",
			Description: "URL to send telemetry.",
			Flag:        "telemetry-url",
			Hidden:      true,
			Default:     "https://telemetry.coder.com",
			Value:       &c.Telemetry.URL,
			Group:       []string{"Telemetry"},
		},
		// Trace settings
		{
			Name:        "Trace Enable",
			Description: "Whether application tracing data is collected. It exports to a backend configured by environment variables. See: https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/protocol/exporter.md",
			Flag:        "trace",
			Value:       &c.Trace.Enable,
			Group:       []string{"Introspection"},
		},
		{
			Name:        "Trace Honeycomb API Key",
			Description: "Enables trace exporting to Honeycomb.io using the provided API Key.",
			Flag:        "trace-honeycomb-api-key",
			Annotations: bigcli.Annotations{}.Mark(flagSecretKey, "true"),
			Value:       &c.Trace.HoneycombAPIKey,
			Group:       []string{"Introspection"},
		},
		{
			Name:        "Capture Logs in Traces",
			Description: "Enables capturing of logs as events in traces. This is useful for debugging, but may result in a very large amount of events being sent to the tracing backend which may incur significant costs. If the verbose flag was supplied, debug-level logs will be included.",
			Flag:        "trace-logs",
			Value:       &c.Trace.CaptureLogs,
			Group:       []string{"Introspection"},
		},
		// Provisioner settings
		{
			Name:        "Provisioner Daemons",
			Description: "Number of provisioner daemons to create on start. If builds are stuck in queued state for a long time, consider increasing this.",
			Flag:        "provisioner-daemons",
			Default:     "3",
			Value:       &c.Provisioner.Daemons,
			Group:       []string{"Provisioning"},
		},
		{
			Name:        "Poll Interval",
			Description: "Time to wait before polling for a new job.",
			Flag:        "provisioner-daemon-poll-interval",
			Default:     time.Second.String(),
			Value:       &c.Provisioner.DaemonPollInterval,
			Group:       []string{"Provisioning"},
		},
		{
			Name:        "Poll Jitter",
			Description: "Random jitter added to the poll interval.",
			Flag:        "provisioner-daemon-poll-jitter",
			Default:     (100 * time.Millisecond).String(),
			Value:       &c.Provisioner.DaemonPollJitter,
			Group:       []string{"Provisioning"},
		},
		{
			Name:        "Force Cancel Interval",
			Description: "Time to force cancel provisioning tasks that are stuck.",
			Flag:        "provisioner-force-cancel-interval",
			Default:     (10 * time.Minute).String(),
			Value:       &c.Provisioner.ForceCancelInterval,
			Group:       []string{"Provisioning"},
		},
		// RateLimit settings
		{
			Name:        "Disable All Rate Limits",
			Description: "Disables all rate limits. This is not recommended in production.",
			Flag:        "dangerous-disable-rate-limits",
			Default:     "false",
			Value:       &c.RateLimit.DisableAll,
			Hidden:      true,
		},
		{
			Name:        "API Rate Limit",
			Description: "Maximum number of requests per minute allowed to the API per user, or per IP address for unauthenticated users. Negative values mean no rate limit. Some API endpoints have separate strict rate limits regardless of this value to prevent denial-of-service or brute force attacks.",
			// Change the env from the auto-generated CODER_RATE_LIMIT_API to the
			// old value to avoid breaking existing deployments.
			Env:     "API_RATE_LIMIT",
			Flag:    "api-rate-limit",
			Default: "512",
			Value:   &c.RateLimit.API,
			Hidden:  true,
		},
		// Logging settings
		{
			Name:        "Human Log Location",
			Description: "Output human-readable logs to a given file.",
			Flag:        "log-human",
			Default:     "/dev/stderr",
			Value:       &c.Logging.Human,
			Group:       []string{"Introspection", "Logging"},
		},
		{
			Name:        "JSON Log Location",
			Description: "Output JSON logs to a given file.",
			Flag:        "log-json",
			Default:     "",
			Value:       &c.Logging.JSON,
			Group:       []string{"Introspection", "Logging"},
		},
		{
			Name:        "Stackdriver Log Location",
			Description: "Output Stackdriver compatible logs to a given file.",
			Flag:        "log-stackdriver",
			Default:     "",
			Value:       &c.Logging.Stackdriver,
			Group:       []string{"Introspection", "Logging"},
		},
		// ☢️ Dangerous settings
		{
			Name:        "DANGEROUS: Allow Path App Sharing",
			Description: "Allow workspace apps that are not served from subdomains to be shared. Path-based app sharing is DISABLED by default for security purposes. Path-based apps can make requests to the Coder API and pose a security risk when the workspace serves malicious JavaScript. Path-based apps can be disabled entirely with --disable-path-apps for further security.",
			Flag:        "dangerous-allow-path-app-sharing",
			Default:     "false",
			Value:       &c.Dangerous.AllowPathAppSharing,
			Group:       []string{"⚠️ Dangerous"},
		},
		{
			Name:        "DANGEROUS: Allow Site Owners to Access Path Apps",
			Description: "Allow site-owners to access workspace apps from workspaces they do not own. Owners cannot access path-based apps they do not own by default. Path-based apps can make requests to the Coder API and pose a security risk when the workspace serves malicious JavaScript. Path-based apps can be disabled entirely with --disable-path-apps for further security.",
			Flag:        "dangerous-allow-path-app-site-owner-access",
			Default:     "false",
			Value:       &c.Dangerous.AllowPathAppSiteOwnerAccess,
			Group:       []string{"⚠️ Dangerous"},
		},
		// Misc. settings
		{
			Name:        "Experiments",
			Description: "Enable one or more experiments. These are not ready for production. Separate multiple experiments with commas, or enter '*' to opt-in to all available experiments.",
			Flag:        "experiments",
			Value:       &c.Experiments,
		},
		{
			Name:        "Update Check",
			Description: "Periodically check for new releases of Coder and inform the owner. The check is performed once per day.",
			Flag:        "update-check",
			Default: strconv.FormatBool(
				flag.Lookup("test.v") == nil && !buildinfo.IsDev(),
			),
			Value: &c.UpdateCheck,
		},
		{
			Name:        "Max Token Lifetime",
			Description: "The maximum lifetime duration users can specify when creating an API token.",
			Flag:        "max-token-lifetime",
			Default:     (24 * 30 * time.Hour).String(),
			Value:       &c.MaxTokenLifetime,
			Group:       []string{"Security"},
		},
		{
			Name:        "Enable swagger endpoint",
			Description: "Expose the swagger endpoint via /swagger.",
			Flag:        "swagger-enable",
			Default:     "false",
			Value:       &c.Swagger.Enable,
		},
		{
			Name:        "Proxy Trusted Headers",
			Flag:        "proxy-trusted-headers",
			Description: "Headers to trust for forwarding IP addresses. e.g. Cf-Connecting-Ip, True-Client-Ip, X-Forwarded-For",
			Value:       &c.ProxyTrustedHeaders,
			Group:       []string{"Networking"},
		},
		{
			Name:        "Proxy Trusted Origins",
			Flag:        "proxy-trusted-origins",
			Description: "Origin addresses to respect \"proxy-trusted-headers\". e.g. 192.168.1.0/24",
			Value:       &c.ProxyTrustedOrigins,
			Group:       []string{"Networking"},
		},
		{
			Name:        "Cache Directory",
			Description: "The directory to cache temporary files. If unspecified and $CACHE_DIRECTORY is set, it will be used for compatibility with systemd.",
			Flag:        "cache-dir",
			Default:     DefaultCacheDir(),
			Value:       &c.CacheDir,
		},
		{
			Name:        "In Memory Database",
			Description: "Controls whether data will be stored in an in-memory database.",
			Flag:        "in-memory",
			Hidden:      true,
			Value:       &c.InMemoryDatabase,
		},
		{
			Name:        "Postgres Connection URL",
			Description: "URL of a PostgreSQL database. If empty, PostgreSQL binaries will be downloaded from Maven (https://repo1.maven.org/maven2) and store all data in the config root. Access the built-in database with \"coder server postgres-builtin-url\".",
			Flag:        "postgres-url",
			Annotations: bigcli.Annotations{}.Mark(flagSecretKey, "true"),
			Value:       &c.PostgresURL,
		},
		{
			Name:        "Secure Auth Cookie",
			Description: "Controls if the 'Secure' property is set on browser session cookies.",
			Flag:        "secure-auth-cookie",
			Value:       &c.SecureAuthCookie,
			Group:       []string{"Networking"},
		},
		{
			Name: "Strict-Transport-Security",
			Description: "Controls if the 'Strict-Transport-Security' header is set on all static file responses. " +
				"This header should only be set if the server is accessed via HTTPS. This value is the MaxAge in seconds of " +
				"the header.",
			Default: "0",
			Flag:    "strict-transport-security",
			Value:   &c.StrictTransportSecurity,
			Group:   []string{"Networking", "TLS"},
		},
		{
			Name: "Strict-Transport-Security Options",
			Description: "Two optional fields can be set in the Strict-Transport-Security header; 'includeSubDomains' and 'preload'. " +
				"The 'strict-transport-security' flag must be set to a non-zero value for these options to be used.",
			Flag:  "strict-transport-security-options",
			Value: &c.StrictTransportSecurityOptions,
			Group: []string{"Networking", "TLS"},
		},
		{
			Name:        "SSH Keygen Algorithm",
			Description: "The algorithm to use for generating ssh keys. Accepted values are \"ed25519\", \"ecdsa\", or \"rsa4096\".",
			Flag:        "ssh-keygen-algorithm",
			Default:     "ed25519",
			Value:       &c.SSHKeygenAlgorithm,
		},
		{
			Name:        "Metrics Cache Refresh Interval",
			Description: "How frequently metrics are refreshed",
			Flag:        "metrics-cache-refresh-interval",
			Hidden:      true,
			Default:     time.Hour.String(),
			Value:       &c.MetricsCacheRefreshInterval,
		},
		{
			Name:        "Agent Stat Refresh Interval",
			Description: "How frequently agent stats are recorded",
			Flag:        "agent-stats-refresh-interval",
			Hidden:      true,
			Default:     (10 * time.Minute).String(),
			Value:       &c.AgentStatRefreshInterval,
		},
		{
			Name:        "Agent Fallback Troubleshooting URL",
			Description: "URL to use for agent troubleshooting when not set in the template",
			Flag:        "agent-fallback-troubleshooting-url",
			Hidden:      true,
			Default:     "https://coder.com/docs/coder-oss/latest/templates#troubleshooting-templates",
			Value:       &c.AgentFallbackTroubleshootingURL,
		},
		{
			Name:        "Audit Logging",
			Description: "Specifies whether audit logging is enabled.",
			Flag:        "audit-logging",
			Default:     "true",
			Annotations: bigcli.Annotations{}.Mark(flagEnterpriseKey, "true"),
			Value:       &c.AuditLogging,
		},
		{
			Name:        "Browser Only",
			Description: "Whether Coder only allows connections to workspaces via the browser.",
			Flag:        "browser-only",
			Annotations: bigcli.Annotations{}.Mark(flagEnterpriseKey, "true"),
			Value:       &c.BrowserOnly,
			Group:       []string{"Networking"},
		},
		{
			Name:        "SCIM API Key",
			Description: "Enables SCIM and sets the authentication header for the built-in SCIM server. New users are automatically created with OIDC authentication.",
			Flag:        "scim-auth-header",
			Annotations: bigcli.Annotations{}.Mark(flagEnterpriseKey, "true").Mark(flagSecretKey, "true"),
			Value:       &c.SCIMAPIKey,
		},

		{
			Name:        "Disable Path Apps",
			Description: "Disable workspace apps that are not served from subdomains. Path-based apps can make requests to the Coder API and pose a security risk when the workspace serves malicious JavaScript. This is recommended for security purposes if a --wildcard-access-url is configured.",
			Flag:        "disable-path-apps",
			Default:     "false",
			Value:       &c.DisablePathApps,
		},
		{
			Name:        "Session Duration",
			Description: "The token expiry duration for browser sessions. Sessions may last longer if they are actively making requests, but this functionality can be disabled via --disable-session-expiry-refresh.",
			Flag:        "session-duration",
			Default:     (24 * time.Hour).String(),
			Value:       &c.SessionDuration,
			Group:       []string{"Security"},
		},
		{
			Name:        "Disable Session Expiry Refresh",
			Description: "Disable automatic session expiry bumping due to activity. This forces all sessions to become invalid after the session expiry duration has been reached.",
			Flag:        "disable-session-expiry-refresh",
			Default:     "false",
			Value:       &c.DisableSessionExpiryRefresh,
			Group:       []string{"Security"},
		},
		{
			Name:        "Disable Password Authentication",
			Description: "Disable password authentication. This is recommended for security purposes in production deployments that rely on an identity provider. Any user with the owner role will be able to sign in with their password regardless of this setting to avoid potential lock out. If you are locked out of your account, you can use the `coder server create-admin` command to create a new admin user directly in the database.",
			Flag:        "disable-password-auth",
			Default:     "false",
			Value:       &c.DisablePasswordAuth,
			Group:       []string{"Security"},
		},
	}
}

type Flaggable interface {
	string | time.Duration | bool | int | []string | []GitAuthConfig
}

// Scrub returns a copy of the config without secret values.
func (c *DeploymentConfig) Scrub() (*DeploymentConfig, error) {
	var ff DeploymentConfig

	// Create copy via JSON.
	byt, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(byt, &ff)
	if err != nil {
		return nil, err
	}

	for _, opt := range ff.ConfigOptions() {
		if !isSecretOpt(opt.Annotations) {
			continue
		}

		// This only works with string values for now.
		switch v := opt.Value.(type) {
		case *bigcli.String:
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
func (c *Client) DeploymentConfig(ctx context.Context) (DeploymentConfig, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/config/deployment", nil)
	if err != nil {
		return DeploymentConfig{}, xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return DeploymentConfig{}, ReadBodyAsError(res)
	}

	var df DeploymentConfig
	return df, json.NewDecoder(res.Body).Decode(&df)
}

type AppearanceConfig struct {
	LogoURL       string              `json:"logo_url"`
	ServiceBanner ServiceBannerConfig `json:"service_banner"`
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

func (c *Client) UpdateAppearance(ctx context.Context, appearance AppearanceConfig) error {
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

	if res.StatusCode != http.StatusOK {
		return BuildInfoResponse{}, ReadBodyAsError(res)
	}

	var buildInfo BuildInfoResponse
	return buildInfo, json.NewDecoder(res.Body).Decode(&buildInfo)
}

type Experiment string

const (
	// ExperimentAuthzQuerier is an internal experiment that enables the ExperimentAuthzQuerier
	// interface for all RBAC operations. NOT READY FOR PRODUCTION USE.
	ExperimentAuthzQuerier Experiment = "authz_querier"

	// ExperimentTemplateEditor is an internal experiment that enables the template editor
	// for all users.
	ExperimentTemplateEditor Experiment = "template_editor"

	// Add new experiments here!
	// ExperimentExample Experiment = "example"
)

// ExperimentsAll should include all experiments that are safe for
// users to opt-in to via --experimental='*'.
// Experiments that are not ready for consumption by all users should
// not be included here and will be essentially hidden.
var ExperimentsAll = Experiments{ExperimentTemplateEditor}

// Experiments is a list of experiments that are enabled for the deployment.
// Multiple experiments may be enabled at the same time.
// Experiments are not safe for production use, and are not guaranteed to
// be backwards compatible. They may be removed or renamed at any time.
type Experiments []Experiment

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

type DeploymentDAUsResponse struct {
	Entries []DAUEntry `json:"entries"`
}

func (c *Client) DeploymentDAUs(ctx context.Context) (*DeploymentDAUsResponse, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/insights/daus", nil)
	if err != nil {
		return nil, xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}

	var resp DeploymentDAUsResponse
	return &resp, json.NewDecoder(res.Body).Decode(&resp)
}

type AppHostResponse struct {
	// Host is the externally accessible URL for the Coder instance.
	Host string `json:"host"`
}

// AppHost returns the site-wide application wildcard hostname without the
// leading "*.", e.g. "apps.coder.com". Apps are accessible at:
// "<app-name>--<agent-name>--<workspace-name>--<username>.<app-host>", e.g.
// "my-app--agent--workspace--username.apps.coder.com".
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

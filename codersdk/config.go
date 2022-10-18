package codersdk

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"golang.org/x/xerrors"
)

// DeploymentConfig is the central configuration for the coder server.
// Secret values should specify `json:"-"` to prevent them from being returned by the API.
// All config values can be set via environment variables in the form of `CODER_<key>` with `.` and `-` replaced by `_`.
// Optional doc comments above fields will generate CLI commands with the following options:
// Usage: - Describe what the setting field does (required)
// Flag: - Long flag name (required)
// Shorthand: - Single character shorthand flag name (optional)
// Default: - Default value for the field as you would write in go code (ex. "string", int, time.Minute, []string{"one", "two"}) (optional)
// Enterprise - Whether or not the field is only available in enterprise (optional)
// Hidden - Whether or not the field should be hidden from the CLI (optional)
type DeploymentConfig struct {
	// Usage: External URL to access your deployment. This must be accessible by all provisioned workspaces.
	// Flag:  access-url
	AccessURL DeploymentConfigField[string] `mapstructure:"access_url" json:"access_url"`
	// Usage: Specifies the wildcard hostname to use for workspace applications in the form "*.example.com".
	// Flag:  wildcard-access-url
	WildcardAccessURL DeploymentConfigField[string] `mapstructure:"wildcard_access_url" json:"wildcard_access_url"`
	// Usage:     Bind address of the server.
	// Flag:      address
	// Shorthand: a
	// Default:   "127.0.0.1:3000"
	Address DeploymentConfigField[string] `mapstructure:"address" json:"address"`
	// Usage:   Interval to poll for scheduled workspace builds.
	// Flag:    autobuild-poll-interval
	// Hidden:  true
	// Default: time.Minute
	AutobuildPollInterval DeploymentConfigField[time.Duration] `mapstructure:"autobuild_poll_interval" json:"autobuild_poll_interval"`
	// Usage:  Whether to enable or disable the embedded DERP relay server.
	// Flag:    derp-server-enable
	// Default: true
	DERPServerEnable DeploymentConfigField[bool] `mapstructure:"enabled" json:"derp_server_enabled"`
	// Usage:   Region ID to use for the embedded DERP server.
	// Flag:    derp-server-region-id
	// Default: 999
	DERPServerRegionID DeploymentConfigField[int] `mapstructure:"region_id" json:"derp_server_region_id"`
	// Usage:   Region code to use for the embedded DERP server.
	// Flag:    derp-server-region-code
	// Default: "coder"
	DERPServerRegionCode DeploymentConfigField[string] `mapstructure:"region_code" json:"derp_server_region_code"`
	// Usage:   Region name that for the embedded DERP server.
	// Flag:    derp-server-region-name
	// Default: "Coder Embedded Relay"
	DERPServerRegionName DeploymentConfigField[string] `mapstructure:"region_name" json:"derp_server_region_name"`
	// Usage:   Addresses for STUN servers to establish P2P connections. Set empty to disable P2P connections.
	// Flag:    derp-server-stun-addresses
	// Default: []string{"stun.l.google.com:19302"}
	DERPServerSTUNAddresses DeploymentConfigField[[]string] `mapstructure:"stun_address" json:"derp_server_stun_address"`
	// Usage:       An HTTP address that is accessible by other replicas to relay DERP traffic. Required for high availability.
	// Flag:        derp-server-relay-address
	// Enterprise:  true
	DERPServerRelayAddress DeploymentConfigField[string] `mapstructure:"relay_address" json:"derp_server_relay_address"`
	// Usage: URL to fetch a DERP mapping on startup. See: https://tailscale.com/kb/1118/custom-derp-servers/
	// Flag:  derp-config-url
	DERPConfigURL DeploymentConfigField[string] `mapstructure:"url" json:"derp_config_url"`
	// Usage: Path to read a DERP mapping from. See: https://tailscale.com/kb/1118/custom-derp-servers/
	// Flag:  derp-config-path
	DERPConfigPath DeploymentConfigField[string] `mapstructure:"path" json:"derp_config_path"`
	// Usage: Serve prometheus metrics on the address defined by prometheus address.
	// Flag:  prometheus-enable
	PrometheusEnable DeploymentConfigField[bool] `mapstructure:"enabled" json:"prometheus_enabled"`
	// Usage:   The bind address to serve prometheus metrics.
	// Flag:    prometheus-address
	// Default: "127.0.0.1:2112"
	PrometheusAddress DeploymentConfigField[string] `mapstructure:"address" json:"prometheus_address"`
	// Usage: Serve pprof metrics on the address defined by pprof address.
	// Flag:  pprof-enable
	PprofEnable DeploymentConfigField[bool] `mapstructure:"enabled" json:"pprof_enabled"`
	// Usage:   The bind address to serve pprof.
	// Flag:    pprof-address
	// Default: "127.0.0.1:6060"
	PprofAddress DeploymentConfigField[string] `mapstructure:"address" json:"pprof_address"`
	// Usage:   The directory to cache temporary files. If unspecified and $CACHE_DIRECTORY is set, it will be used for compatibility with systemd.
	// Flag:    cache-dir
	// Default: defaultCacheDir()
	CacheDir DeploymentConfigField[string] `mapstructure:"cache_dir" json:"cache_dir"`
	// Usage:  Controls whether data will be stored in an in-memory database.
	// Flag:   in-memory
	// Hidden: true
	InMemoryDatabase DeploymentConfigField[bool] `mapstructure:"in_memory_database" json:"in_memory_database"`
	// Usage:   Number of provisioner daemons to create on start. If builds are stuck in queued state for a long time, consider increasing this.
	// Flag:    provisioner-daemons
	// Default: 3
	ProvisionerDaemonCount DeploymentConfigField[int] `mapstructure:"provisioner_daemon_count" json:"provisioner_daemon_count"`
	// Usage: URL of a PostgreSQL database. If empty, PostgreSQL binaries will be downloaded from Maven (https://repo1.maven.org/maven2) and store all data in the config root. Access the built-in database with "coder server postgres-builtin-url".
	// Flag:  postgres-url
	PostgresURL DeploymentConfigField[string] `mapstructure:"postgres_url" json:"-"`
	// Usage: Client ID for Login with GitHub.
	// Flag:  oauth2-github-client-id
	OAuth2GithubClientID DeploymentConfigField[string] `mapstructure:"client_id" json:"oauth2_github_client_id"`
	// Usage: Client secret for Login with GitHub.
	// Flag:  oauth2-github-client-secret
	OAuth2GithubClientSecret DeploymentConfigField[string] `mapstructure:"client_secret" json:"-"`
	// Usage: Organizations the user must be a member of to Login with GitHub.
	// Flag:  oauth2-github-allowed-orgs
	OAuth2GithubAllowedOrganizations DeploymentConfigField[[]string] `mapstructure:"allowed_organizations" json:"oauth2_github_allowed_organizations"`
	// Usage: Teams inside organizations the user must be a member of to Login with GitHub. Structured as: <organization-name>/<team-slug>.
	// Flag:  oauth2-github-allowed-teams
	OAuth2GithubAllowedTeams DeploymentConfigField[[]string] `mapstructure:"allowed_teams" json:"oauth2_github_allowed_teams"`
	// Usage: Whether new users can sign up with GitHub.
	// Flag:  oauth2-github-allow-signups
	OAuth2GithubAllowSignups DeploymentConfigField[bool] `mapstructure:"allow_signups" json:"oauth2_github_allow_signups"`
	// Usage: Base URL of a GitHub Enterprise deployment to use for Login with GitHub.
	// Flag:  oauth2-github-enterprise-base-url
	OAuth2GithubEnterpriseBaseURL DeploymentConfigField[string] `mapstructure:"enterprise_base_url" json:"oauth2_github_enterprise_base_url"`
	// Usage:   Whether new users can sign up with OIDC.
	// Flag:    oidc-allow-signups
	// Default: true
	OIDCAllowSignups DeploymentConfigField[bool] `mapstructure:"allow_signups" json:"oidc_allow_signups"`
	// Usage: Client ID to use for Login with OIDC.
	// Flag:  oidc-client-id
	OIDCClientID DeploymentConfigField[string] `mapstructure:"client_id" json:"oidc_client_id"`
	// Usage: Client secret to use for Login with OIDC.
	// Flag:  oidc-client-secret
	OIDCClientSecret DeploymentConfigField[string] `mapstructure:"cliet_secret" json:"-"`
	// Usage: Email domain that clients logging in with OIDC must match.
	// Flag:  oidc-email-domain
	OIDCEmailDomain DeploymentConfigField[string] `mapstructure:"email_domain" json:"oidc_email_domain"`
	// Usage: Issuer URL to use for Login with OIDC.
	// Flag:  oidc-issuer-url
	OIDCIssuerURL DeploymentConfigField[string] `mapstructure:"issuer_url" json:"oidc_issuer_url"`
	// Usage:   Scopes to grant when authenticating with OIDC.
	// Flag:    oidc-scopes
	// Default: []string{oidc.ScopeOpenID, "profile", "email"}
	OIDCScopes DeploymentConfigField[[]string] `mapstructure:"scopes" json:"oidc_scopes"`
	// Usage:   Whether telemetry is enabled or not. Coder collects anonymized usage data to help improve our product.
	// Flag:    telemetry
	// Default: flag.Lookup("test.v") == nil
	TelemetryEnable DeploymentConfigField[bool] `mapstructure:"enable" json:"telemetry_enable"`
	// Usage:   Whether Opentelemetry traces are sent to Coder. Coder collects anonymized application tracing to help improve our product. Disabling telemetry also disables this option.
	// Flag:    telemetry-trace
	// Default: flag.Lookup("test.v") == nil
	TelemetryTraceEnable DeploymentConfigField[bool] `mapstructure:"trace_enable" json:"telemetry_trace_enable"`
	// Usage:   URL to send telemetry.
	// Flag:    telemetry-url
	// Hidden:  true
	// Default: "https://telemetry.coder.com"
	TelemetryURL DeploymentConfigField[string] `mapstructure:"url" json:"telemetry_url"`
	// Usage: Whether TLS will be enabled.
	// Flag:  tls-enable
	TLSEnable DeploymentConfigField[bool] `mapstructure:"tls_enable" json:"tls_enable"`
	// Usage: Path to each certificate for TLS. It requires a PEM-encoded file. To configure the listener to use a CA certificate, concatenate the primary certificate and the CA certificate together. The primary certificate should appear first in the combined file.
	// Flag:  tls-cert-file
	TLSCertFiles DeploymentConfigField[[]string] `mapstructure:"tls_cert_files" json:"tls_cert_files"`
	// Usage: PEM-encoded Certificate Authority file used for checking the authenticity of client
	// Flag:  tls-client-ca-file
	TLSClientCAFile DeploymentConfigField[string] `mapstructure:"tls_client_ca_file" json:"tls_client_ca_file"`
	// Usage:   Policy the server will follow for TLS Client Authentication. Accepted values are "none", "request", "require-any", "verify-if-given", or "require-and-verify".
	// Flag:    tls-client-auth
	// Default: "request"
	TLSClientAuth DeploymentConfigField[string] `mapstructure:"tls_client_auth" json:"tls_client_auth"`
	// Usage: Paths to the private keys for each of the certificates. It requires a PEM-encoded file.
	// Flag:  tls-key-file
	TLSKeyFiles DeploymentConfigField[[]string] `mapstructure:"tls_key_files" json:"tls_key_files"`
	// Usage:   Minimum supported version of TLS. Accepted values are "tls10", "tls11", "tls12" or "tls13"
	// Flag:    tls-min-version
	// Default: "tls12"
	TLSMinVersion DeploymentConfigField[string] `mapstructure:"tls_min_version" json:"tls_min_version"`
	// Usage: Whether application tracing data is collected.
	// Flag:  trace
	TraceEnable DeploymentConfigField[bool] `mapstructure:"trace_enable" json:"trace_enable"`
	// Usage: Controls if the 'Secure' property is set on browser session cookies.
	// Flag:  secure-auth-cookie
	SecureAuthCookie DeploymentConfigField[bool] `mapstructure:"secure_auth_cookie" json:"secure_auth_cookie"`
	// Usage:   The algorithm to use for generating ssh keys. Accepted values are "ed25519", "ecdsa", or "rsa4096".
	// Flag:    ssh-keygen-algorithm
	// Default: "ed25519"
	SSHKeygenAlgorithm DeploymentConfigField[string] `mapstructure:"ssh_keygen_algorithm" json:"ssh_keygen_algorithm"`
	// Usage:  Templates to auto-import. Available auto-importable templates are: kubernetes
	// Flag:   auto-import-template
	// Hidden: true
	AutoImportTemplates DeploymentConfigField[[]string] `mapstructure:"auto_import_templates" json:"auto_import_templates"`
	// Usage:   How frequently metrics are refreshed
	// Flag:    metrics-cache-refresh-interval
	// Hidden:  true
	// Default: time.Hour
	MetricsCacheRefreshInterval DeploymentConfigField[time.Duration] `mapstructure:"metrics_cache_refresh_interval" json:"metrics_cache_refresh_interval"`
	// Usage:   How frequently agent stats are recorded
	// Flag:    agent-stats-refresh-interval
	// Hidden:  true
	// Default: 10 * time.Minute
	AgentStatRefreshInterval DeploymentConfigField[time.Duration] `mapstructure:"agent_stat_refresh_interval" json:"agent_stat_refresh_interval"`
	// Usage:     Enables verbose logging.
	// Flag:      verbose
	// Shorthand: v
	Verbose DeploymentConfigField[bool] `mapstructure:"verbose" json:"verbose"`
	// Usage:      Specifies whether audit logging is enabled.
	// Flag:       audit-logging
	// Default:    true
	// Enterprise: true
	AuditLogging DeploymentConfigField[bool] `mapstructure:"audit_logging" json:"audit_logging"`
	// Usage:      Whether Coder only allows connections to workspaces via the browser.
	// Flag:       browser-only
	// Enterprise: true
	BrowserOnly DeploymentConfigField[bool] `mapstructure:"browser_only" json:"browser_only"`
	// Usage:      Enables SCIM and sets the authentication header for the built-in SCIM server. New users are automatically created with OIDC authentication.
	// Flag:       scim-auth-header
	// Enterprise: true
	SCIMAuthHeader DeploymentConfigField[string] `mapstructure:"scim_auth_header" json:"-"`
	// Usage:      Enables and sets a limit on how many workspaces each user can create.
	// Flag:       user-workspace-quota
	// Enterprise: true
	UserWorkspaceQuota DeploymentConfigField[int] `mapstructure:"user_workspace_quota" json:"user_workspace_quota"`
}

type Flaggable interface {
	string | bool | int | time.Duration | []string
}

type DeploymentConfigField[T Flaggable] struct {
	Key        string
	Name       string
	Usage      string
	Flag       string
	Shorthand  string
	Enterprise bool
	Hidden     bool
	Value      T
}

// DeploymentConfig returns the deployment config for the coder server.
func (c *Client) DeploymentConfig(ctx context.Context) (DeploymentConfig, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/config/deployment", nil)
	if err != nil {
		return DeploymentConfig{}, xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return DeploymentConfig{}, readBodyAsError(res)
	}

	var df DeploymentConfig
	return df, json.NewDecoder(res.Body).Decode(&df)
}

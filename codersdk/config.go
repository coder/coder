package codersdk

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"golang.org/x/xerrors"
)

type DeploymentConfig struct {
	// External URL to access your deployment. This must be accessible by all provisioned workspaces.
	AccessURL string `json:"access_url"`
	// Specifies the wildcard hostname to use for workspace applications in the form "*.example.com".
	WildcardAccessURL string `json:"wildcard_access_url"`
	// Bind address of the server.
	Address string `json:"address"`
	// Interval to poll for scheduled workspace builds.
	AutobuildPollInterval time.Duration    `json:"autobuild_poll_interval"`
	DERP                  DERPConfig       `json:"derp"`
	Prometheus            PrometheusConfig `json:"prometheus"`
	Pprof                 PprofConfig      `json:"pprof"`
	// The directory to cache temporary files. If unspecified and $CACHE_DIRECTORY is set, it will be used for compatibility with systemd.
	CacheDir string `json:"cache_dir"`
	// Controls whether data will be stored in an in-memory database.
	InMemoryDatabase bool `json:"in_memory_database"`
	// Number of provisioner daemons to create on start. If builds are stuck in queued state for a long time, consider increasing this.
	ProvisionerDaemonCount int `json:"provisioner_daemon_count"`
	// URL of a PostgreSQL database. If empty, PostgreSQL binaries will be downloaded from Maven (https://repo1.maven.org/maven2) and store all data in the config root. Access the built-in database with "coder server postgres-builtin-url".
	PostgresURL  string             `json:"postgres_url"`
	Oauth2Github Oauth2GithubConfig `json:"oauth2_github"`
	OIDC         OIDCConfig         `json:"oidc"`
	Telemetry    TelemetryConfig    `json:"telemetry"`
	TLSConfig    TLSConfig          `json:"tls_config"`
	// Whether application tracing data is collected.
	TraceEnable bool `json:"trace_enable"`
	// Controls if the 'Secure' property is set on browser session cookies.
	SecureAuthCookie bool `json:"secure_auth_cookie"`
	// The algorithm to use for generating ssh keys. Accepted values are "ed25519", "ecdsa", or "rsa4096".
	SSHKeygenAlgorithm string `json:"ssh_keygen_algorithm"`
	// Templates to auto-import. Available auto-importable templates are: kubernetes
	AutoImportTemplates []string `json:"auto_import_templates"`
	// How frequently metrics are refreshed
	MetricsCacheRefreshInterval time.Duration `json:"metrics_cache_refresh_interval"`
	// How frequently agent stats are recorded
	AgentStatRefreshInterval time.Duration `json:"agent_stat_refresh_interval"`
	// Enables verbose logging.
	Verbose bool `json:"verbose"`
	// Specifies whether audit logging is enabled.
	AuditLogging bool `json:"audit_logging"`
	// Whether Coder only allows connections to workspaces via the browser.
	BrowserOnly bool `json:"browser_only"`
	// Enables SCIM and sets the authentication header for the built-in SCIM server. New users are automatically created with OIDC authentication.
	SCIMAuthHeader string `json:"scim_auth_header"`
	// Enables and sets a limit on how many workspaces each user can create.
	UserWorkspaceQuota int `json:"user_workspace_quota"`
}

type DERPConfig struct {
	Server DERPServerConfig `json:"server"`
	Config DERPConfigConfig `json:"config"`
}
type DERPServerConfig struct {
	// Whether to enable or disable the embedded DERP relay server.
	Enable bool `json:"enabled"`
	// Region ID to use for the embedded DERP server.
	RegionID int `json:"region_id"`
	// Region code to use for the embedded DERP server.
	RegionCode string `json:"region_code"`
	// Region name that for the embedded DERP server.
	RegionName string `json:"region_name"`
	// Addresses for STUN servers to establish P2P connections. Set empty to disable P2P connections.
	STUNAddresses []string `json:"stun_address"`
}

type DERPConfigConfig struct {
	// URL to fetch a DERP mapping on startup. See: https://tailscale.com/kb/1118/custom-derp-servers/
	URL string `json:"url"`
	// Path to read a DERP mapping from. See: https://tailscale.com/kb/1118/custom-derp-servers/
	Path string `json:"path"`
}

type PrometheusConfig struct {
	// Serve prometheus metrics on the address defined by prometheus address.
	Enable bool `json:"enabled"`
	// The bind address to serve prometheus metrics.
	Address string `json:"address"`
}

type PprofConfig struct {
	// Serve pprof metrics on the address defined by pprof address.
	Enable bool `json:"enabled"`
	// The bind address to serve pprof.
	Address string `json:"address"`
}

type Oauth2GithubConfig struct {
	// Client ID for Login with GitHub.
	ClientID string `json:"client_id"`
	// Client secret for Login with GitHub.
	ClientSecret string `json:"client_secret"`
	// Organizations the user must be a member of to Login with GitHub.
	AllowedOrganizations []string `json:"allowed_organizations"`
	// Teams inside organizations the user must be a member of to Login with GitHub. Structured as: <organization-name>/<team-slug>.
	AllowedTeams []string `json:"allowed_teams"`
	// Whether new users can sign up with GitHub.
	AllowSignups bool `json:"allow_signups"`
	// Base URL of a GitHub Enterprise deployment to use for Login with GitHub.
	EnterpriseBaseURL string `json:"enterprise_base_url"`
}

type OIDCConfig struct {
	// Whether new users can sign up with OIDC.
	AllowSignups bool `json:"allow_signups"`
	// Client ID to use for Login with OIDC.
	ClientID string `json:"client_id"`
	// Client secret to use for Login with OIDC.
	ClientSecret string `json:"cliet_secret"`
	// Email domain that clients logging in with OIDC must match.
	EmailDomain string `json:"email_domain"`
	// Issuer URL to use for Login with OIDC.
	IssuerURL string `json:"issuer_url"`
	// Scopes to grant when authenticating with OIDC.
	Scopes []string `json:"scopes"`
}

type TelemetryConfig struct {
	// Whether telemetry is enabled or not. Coder collects anonymized usage data to help improve our product.
	Enable bool `json:"enable"`
	// Whether Opentelemetry traces are sent to Coder. Coder collects anonymized application tracing to help improve our product. Disabling telemetry also disables this option.
	TraceEnable bool `json:"trace_enable"`
	// URL to send telemetry.
	URL string `json:"url"`
}

type TLSConfig struct {
	// Whether TLS will be enabled.
	Enable bool `json:"tls_enable"`
	// Path to each certificate for TLS. It requires a PEM-encoded file. To configure the listener to use a CA certificate, concatenate the primary certificate and the CA certificate together. The primary certificate should appear first in the combined file.
	CertFiles []string `json:"tls_cert_files"`
	// PEM-encoded Certificate Authority file used for checking the authenticity of client
	ClientCAFile string `json:"tls_client_ca_file"`
	// Policy the server will follow for TLS Client Authentication. Accepted values are "none", "request", "require-any", "verify-if-given", or "require-and-verify".
	ClientAuth string `json:"tls_client_auth"`
	// Paths to the private keys for each of the certificates. It requires a PEM-encoded file.
	KeyFiles []string `json:"tls_key_tiles"`
	// Minimum supported version of TLS. Accepted values are "tls10", "tls11", "tls12" or "tls13"
	MinVersion string `json:"tls_min_version"`
}

type DeploymentFlags struct {
	AccessURL                        *Flag[string]        `json:"access_url"`
	WildcardAccessURL                *Flag[string]        `json:"wildcard_access_url"`
	Address                          *Flag[string]        `json:"address"`
	AutobuildPollInterval            *Flag[time.Duration] `json:"autobuild_poll_interval"`
	DerpServerEnable                 *Flag[bool]          `json:"enabled"`
	DerpServerRegionID               *Flag[int]           `json:"region_id"`
	DerpServerRegionCode             *Flag[string]        `json:"region_code"`
	DerpServerRegionName             *Flag[string]        `json:"region_name"`
	DerpServerSTUNAddresses          *Flag[[]string]      `json:"stun_address"`
	DerpConfigURL                    *Flag[string]        `json:"derp_config_url"`
	DerpConfigPath                   *Flag[string]        `json:"derp_config_path"`
	PromEnabled                      *Flag[bool]          `json:"prom_enabled"`
	PromAddress                      *Flag[string]        `json:"prom_address"`
	PprofEnabled                     *Flag[bool]          `json:"pprof_enabled"`
	PprofAddress                     *Flag[string]        `json:"pprof_address"`
	CacheDir                         *Flag[string]        `json:"cache_dir"`
	InMemoryDatabase                 *Flag[bool]          `json:"in_memory_database"`
	ProvisionerDaemonCount           *Flag[int]           `json:"provisioner_daemon_count"`
	PostgresURL                      *Flag[string]        `json:"postgres_url"`
	OAuth2GithubClientID             *Flag[string]        `json:"client_id"`
	OAuth2GithubClientSecret         *Flag[string]        `json:"client_secret"`
	OAuth2GithubAllowedOrganizations *Flag[[]string]      `json:"allowed_organizations"`
	OAuth2GithubAllowedTeams         *Flag[[]string]      `json:"allowed_teams"`
	OAuth2GithubAllowSignups         *Flag[bool]          `json:"allow_signups"`
	OAuth2GithubEnterpriseBaseURL    *Flag[string]        `json:"enterprise_base_url"`
	OIDCAllowSignups                 *Flag[bool]          `json:"allow_signups"`
	OIDCClientID                     *Flag[string]        `json:"client_id"`
	OIDCClientSecret                 *Flag[string]        `json:"cliet_secret"`
	OIDCEmailDomain                  *Flag[string]        `json:"email_domain"`
	OIDCIssuerURL                    *Flag[string]        `json:"issuer_url"`
	OIDCScopes                       *Flag[[]string]      `json:"scopes"`
	TelemetryEnable                  *Flag[bool]          `json:"telemetry_enable"`
	TelemetryTraceEnable             *Flag[bool]          `json:"telemetry_trace_enable"`
	TelemetryURL                     *Flag[string]        `json:"telemetry_url"`
	TLSEnable                        *Flag[bool]          `json:"tls_enable"`
	TLSCertFiles                     *Flag[[]string]      `json:"tls_cert_files"`
	TLSClientCAFile                  *Flag[string]        `json:"tls_client_ca_file"`
	TLSClientAuth                    *Flag[string]        `json:"tls_client_auth"`
	TLSKeyFiles                      *Flag[[]string]      `json:"tls_key_tiles"`
	TLSMinVersion                    *Flag[string]        `json:"tls_min_version"`
	TraceEnable                      *Flag[bool]          `json:"trace_enable"`
	SecureAuthCookie                 *Flag[bool]          `json:"secure_auth_cookie"`
	SSHKeygenAlgorithm               *Flag[string]        `json:"ssh_keygen_algorithm"`
	AutoImportTemplates              *Flag[[]string]      `json:"auto_import_templates"`
	MetricsCacheRefreshInterval      *Flag[time.Duration] `json:"metrics_cache_refresh_interval"`
	AgentStatRefreshInterval         *Flag[time.Duration] `json:"agent_stat_refresh_interval"`
	Verbose                          *Flag[bool]          `json:"verbose"`
	AuditLogging                     *Flag[bool]          `json:"audit_logging"`
	BrowserOnly                      *Flag[bool]          `json:"browser_only"`
	SCIMAuthHeader                   *Flag[string]        `json:"scim_auth_header"`
	UserWorkspaceQuota               *Flag[int]           `json:"user_workspace_quota"`
}

type Flaggable interface {
	string | int | bool | time.Duration | []string
}

type Flag[T Flaggable] struct {
	Name        string `json:"name"`
	Flag        string `json:"flag"`
	EnvVar      string `json:"env_var"`
	Shorthand   string `json:"shorthand"`
	Description string `json:"description"`
	Enterprise  bool   `json:"enterprise"`
	Hidden      bool   `json:"hidden"`
	Secret      bool   `json:"secret"`
	Default     T      `json:"default"`
	Value       T      `json:"value"`
}

func (f *Flag[T]) IsEnterprise() bool {
	return f.Enterprise
}

// DeploymentFlags returns the deployment level flags for the coder server.
func (c *Client) DeploymentFlags(ctx context.Context) (DeploymentFlags, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/flags/deployment", nil)
	if err != nil {
		return DeploymentFlags{}, xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return DeploymentFlags{}, readBodyAsError(res)
	}

	var df DeploymentFlags
	return df, json.NewDecoder(res.Body).Decode(&df)
}

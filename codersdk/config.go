package codersdk

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"golang.org/x/xerrors"
)

type DeploymentConfig struct {
	// Usage: External URL to access your deployment. This must be accessible by all provisioned workspaces.
	// Flag:  "access-url"
	AccessURL string `mapstructure:"access_url"`
	// Usage: Specifies the wildcard hostname to use for workspace applications in the form "*.example.com".
	// Flag:  "wildcard-access-url"
	WildcardAccessURL string `mapstructure:"wildcard_access_url"`
	// Usage: Bind address of the server.
	// Flag: "address"
	// Shorthand:   "a"
	// Default:     "127.0.0.1:3000"
	Address string `mapstructure:"address"`
	// Usage: Interval to poll for scheduled workspace builds.
	// Flag: "autobuild-poll-interval"
	// Hidden:      true
	// Default:     time.Minute
	AutobuildPollInterval time.Duration    `mapstructure:"autobuild_poll_interval"`
	DERP                  DERPConfig       `mapstructure:"derp"`
	Prometheus            PrometheusConfig `mapstructure:"prometheus"`
	Pprof                 PprofConfig      `mapstructure:"pprof"`
	// Usage: The directory to cache temporary files. If unspecified and $CACHE_DIRECTORY is set, it will be used for compatibility with systemd.
	// Flag:        "cache-dir"
	// Default:     defaultCacheDir()
	CacheDir string `mapstructure:"cache_dir"`
	// Usage: Controls whether data will be stored in an in-memory database.
	// Flag:        "in-memory"
	// Hidden:      true
	InMemoryDatabase bool `mapstructure:"in_memory_database"`
	// Usage: Number of provisioner daemons to create on start. If builds are stuck in queued state for a long time, consider increasing this.
	// Flag: "provisioner-daemons"
	// Default:     3
	ProvisionerDaemonCount int `mapstructure:"provisioner_daemon_count"`
	// Usage: URL of a PostgreSQL database. If empty, PostgreSQL binaries will be downloaded from Maven (https://repo1.maven.org/maven2) and store all data in the config root. Access the built-in database with "coder server postgres-builtin-url".
	// Flag: "postgres-url"
	PostgresURL  string             `mapstructure:"postgres_url"`
	OAuth2Github OAuth2GithubConfig `mapstructure:"oauth2_github"`
	OIDC         OIDCConfig         `mapstructure:"oidc"`
	Telemetry    TelemetryConfig    `mapstructure:"telemetry"`
	TLS          TLSConfig          `mapstructure:"tls_config"`
	// Usage: Whether application tracing data is collected.
	// Flag:        "trace"
	TraceEnable bool `mapstructure:"trace_enable"`
	// Usage: Controls if the 'Secure' property is set on browser session cookies.
	// Flag:        "secure-auth-cookie"
	SecureAuthCookie bool `mapstructure:"secure_auth_cookie"`
	// Usage: The algorithm to use for generating ssh keys. Accepted values are "ed25519", "ecdsa", or "rsa4096".
	// Flag:   "ssh-keygen-algorithm"
	// Default: "ed25519"
	SSHKeygenAlgorithm string `mapstructure:"ssh_keygen_algorithm"`
	// Usage: Templates to auto-import. Available auto-importable templates are: kubernetes
	// Flag: "auto-import-template"
	// Hidden:      true
	AutoImportTemplates []string `mapstructure:"auto_import_templates"`
	// Usage: How frequently metrics are refreshed
	// Flag:        "metrics-cache-refresh-interval"
	// Hidden:      true
	// Default:     time.Hour
	MetricsCacheRefreshInterval time.Duration `mapstructure:"metrics_cache_refresh_interval"`
	// Usage: How frequently agent stats are recorded
	// Flag:        "agent-stats-refresh-interval"
	// Hidden:      true
	// Default:     10 * time.Minute
	AgentStatRefreshInterval time.Duration `mapstructure:"agent_stat_refresh_interval"`
	// Usage: Enables verbose logging.
	// Flag:        "verbose"
	// Shorthand:   "v"
	Verbose bool `mapstructure:"verbose"`
	// Usage: Specifies whether audit logging is enabled.
	// Flag:        "audit-logging"
	// Default:     true
	// Enterprise:  true
	AuditLogging bool `mapstructure:"audit_logging"`
	// Usage: Whether Coder only allows connections to workspaces via the browser.
	// Flag:        "browser-only"
	// Enterprise:  true
	BrowserOnly bool `mapstructure:"browser_only"`
	// Usage: Enables SCIM and sets the authentication header for the built-in SCIM server. New users are automatically created with OIDC authentication.
	// Flag:        "scim-auth-header"
	// Enterprise:  true
	SCIMAuthHeader string `mapstructure:"scim_auth_header"`
	// Usage: Enables and sets a limit on how many workspaces each user can create.
	// Flag:        "user-workspace-quota"
	// Enterprise:  true
	UserWorkspaceQuota int `mapstructure:"user_workspace_quota"`
}

type DERPConfig struct {
	Server DERPServerConfig `mapstructure:"server"`
	Config DERPConfigConfig `mapstructure:"config"`
}
type DERPServerConfig struct {
	// Usage: Whether to enable or disable the embedded DERP relay server.
	// Flag:        "derp-server-enable"
	// Default:     true
	Enable bool `mapstructure:"enabled"`
	// Usage: Region ID to use for the embedded DERP server.
	// Flag:        "derp-server-region-id"
	// Default:     999
	RegionID int `mapstructure:"region_id"`
	// Usage: Region code to use for the embedded DERP server.
	// Flag:        "derp-server-region-code"
	// Default:     "coder"
	RegionCode string `mapstructure:"region_code"`
	// Usage: Region name that for the embedded DERP server.
	// Flag:        "derp-server-region-name"
	// Default:     "Coder Embedded Relay"
	RegionName string `mapstructure:"region_name"`
	// Usage: Addresses for STUN servers to establish P2P connections. Set empty to disable P2P connections.
	// Flag:        "derp-server-stun-addresses"
	// Default:     []string{"stun.l.google.com:19302"}
	STUNAddresses []string `mapstructure:"stun_address"`
}

type DERPConfigConfig struct {
	// Usage: URL to fetch a DERP mapping on startup. See: https://tailscale.com/kb/1118/custom-derp-servers/
	// Flag:        "derp-config-url"
	URL string `mapstructure:"url"`
	// Usage: Path to read a DERP mapping from. See: https://tailscale.com/kb/1118/custom-derp-servers/
	// Flag:        "derp-config-path"
	Path string `mapstructure:"path"`
}

type PrometheusConfig struct {
	// Usage: Serve prometheus metrics on the address defined by prometheus address.
	// Flag:        "prometheus-enable"
	Enable bool `mapstructure:"enabled"`
	// Usage: The bind address to serve prometheus metrics.
	// Flag:        "prometheus-address"
	// Default:     "127.0.0.1:2112"
	Address string `mapstructure:"address"`
}

type PprofConfig struct {
	// Usage: Serve pprof metrics on the address defined by pprof address.
	// Flag:        "pprof-enable"
	Enable bool `mapstructure:"enabled"`
	// Usage: The bind address to serve pprof.
	// Flag:        "pprof-address"
	// Default:     "127.0.0.1:6060"
	Address string `mapstructure:"address"`
}

type OAuth2GithubConfig struct {
	// Usage: Client ID for Login with GitHub.
	// Flag:        "oauth2-github-client-id"
	ClientID string `mapstructure:"client_id"`
	// Usage: Client secret for Login with GitHub.
	// Flag:        "oauth2-github-client-secret"
	ClientSecret string `mapstructure:"client_secret"`
	// Usage: Organizations the user must be a member of to Login with GitHub.
	// Flag:        "oauth2-github-allowed-orgs"
	AllowedOrganizations []string `mapstructure:"allowed_organizations"`
	// Usage: Teams inside organizations the user must be a member of to Login with GitHub. Structured as: <organization-name>/<team-slug>.
	// Flag:        "oauth2-github-allowed-teams"
	AllowedTeams []string `mapstructure:"allowed_teams"`
	// Usage: Whether new users can sign up with GitHub.
	// Flag:        "oauth2-github-allow-signups"
	AllowSignups bool `mapstructure:"allow_signups"`
	// Usage: Base URL of a GitHub Enterprise deployment to use for Login with GitHub.
	// Flag:        "oauth2-github-enterprise-base-url"
	EnterpriseBaseURL string `mapstructure:"enterprise_base_url"`
}

type OIDCConfig struct {
	// Usage: Whether new users can sign up with OIDC.
	// Flag:        "oidc-allow-signups"
	// Default:     true
	AllowSignups bool `mapstructure:"allow_signups"`
	// Usage: Client ID to use for Login with OIDC.
	// Flag:        "oidc-client-id"
	ClientID string `mapstructure:"client_id"`
	// Usage: Client secret to use for Login with OIDC.
	// Flag:        "oidc-client-secret"
	ClientSecret string `mapstructure:"cliet_secret"`
	// Usage: Email domain that clients logging in with OIDC must match.
	// Flag:        "oidc-email-domain"
	EmailDomain string `mapstructure:"email_domain"`
	// Usage: Issuer URL to use for Login with OIDC.
	// Flag:        "oidc-issuer-url"
	IssuerURL string `mapstructure:"issuer_url"`
	// Usage: Scopes to grant when authenticating with OIDC.
	// Flag:        "oidc-scopes"
	// Default:     []string{oidc.ScopeOpenID, "profile", "email"}
	Scopes []string `mapstructure:"scopes"`
}

type TelemetryConfig struct {
	// Usage: Whether telemetry is enabled or not. Coder collects anonymized usage data to help improve our product.
	// Flag:        "telemetry"
	// Default:     flag.Lookup("test.v") == nil
	Enable bool `mapstructure:"enable"`
	// Usage: Whether Opentelemetry traces are sent to Coder. Coder collects anonymized application tracing to help improve our product. Disabling telemetry also disables this option.
	// Flag:        "telemetry-trace"
	// Default:     flag.Lookup("test.v") == nil
	TraceEnable bool `mapstructure:"trace_enable"`
	// Usage: URL to send telemetry.
	// Flag:        "telemetry-url"
	// Hidden:      true
	// Default:     "https://telemetry.coder.com"
	URL string `mapstructure:"url"`
}

type TLSConfig struct {
	// Usage: Whether TLS will be enabled.
	// Flag:        "tls-enable"
	Enable bool `mapstructure:"tls_enable"`
	// Usage: Path to each certificate for TLS. It requires a PEM-encoded file. To configure the listener to use a CA certificate, concatenate the primary certificate and the CA certificate together. The primary certificate should appear first in the combined file.
	// Flag:   "tls-cert-file"
	CertFiles []string `mapstructure:"tls_cert_files"`
	// Usage: PEM-encoded Certificate Authority file used for checking the authenticity of client
	// Flag:        "tls-client-ca-file"
	ClientCAFile string `mapstructure:"tls_client_ca_file"`
	// Usage: Policy the server will follow for TLS Client Authentication. Accepted values are "none", "request", "require-any", "verify-if-given", or "require-and-verify".
	// Flag:   "tls-client-auth"
	ClientAuth string `mapstructure:"tls_client_auth"`
	// Usage: Paths to the private keys for each of the certificates. It requires a PEM-encoded file.
	// Flag:        "tls-key-file"
	KeyFiles []string `mapstructure:"tls_key_tiles"`
	// Usage: Minimum supported version of TLS. Accepted values are "tls10", "tls11", "tls12" or "tls13"
	// Flag:        "tls-min-version"
	// Default:     "tls12"
	MinVersion string `mapstructure:"tls_min_version"`
}

// DeploymentConfig returns the deployment config for the coder server.
func (c *Client) DeploymentConfig(ctx context.Context) (DeploymentConfig, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/flags/deployment", nil)
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

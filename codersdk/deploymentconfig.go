package codersdk

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"golang.org/x/xerrors"
)

// DeploymentConfig is the central configuration for the coder server.
type DeploymentConfig struct {
	AccessURL                   *DeploymentConfigField[string]          `json:"access_url" typescript:",notnull"`
	WildcardAccessURL           *DeploymentConfigField[string]          `json:"wildcard_access_url" typescript:",notnull"`
	Address                     *DeploymentConfigField[string]          `json:"address" typescript:",notnull"`
	AutobuildPollInterval       *DeploymentConfigField[time.Duration]   `json:"autobuild_poll_interval" typescript:",notnull"`
	DERP                        *DERP                                   `json:"derp" typescript:",notnull"`
	GitAuth                     *DeploymentConfigField[[]GitAuthConfig] `json:"gitauth" typescript:",notnull"`
	Prometheus                  *PrometheusConfig                       `json:"prometheus" typescript:",notnull"`
	Pprof                       *PprofConfig                            `json:"pprof" typescript:",notnull"`
	ProxyTrustedHeaders         *DeploymentConfigField[[]string]        `json:"proxy_trusted_headers" typescript:",notnull"`
	ProxyTrustedOrigins         *DeploymentConfigField[[]string]        `json:"proxy_trusted_origins" typescript:",notnull"`
	CacheDirectory              *DeploymentConfigField[string]          `json:"cache_directory" typescript:",notnull"`
	InMemoryDatabase            *DeploymentConfigField[bool]            `json:"in_memory_database" typescript:",notnull"`
	PostgresURL                 *DeploymentConfigField[string]          `json:"pg_connection_url" typescript:",notnull"`
	OAuth2                      *OAuth2Config                           `json:"oauth2" typescript:",notnull"`
	OIDC                        *OIDCConfig                             `json:"oidc" typescript:",notnull"`
	Telemetry                   *TelemetryConfig                        `json:"telemetry" typescript:",notnull"`
	TLS                         *TLSConfig                              `json:"tls" typescript:",notnull"`
	Trace                       *TraceConfig                            `json:"trace" typescript:",notnull"`
	SecureAuthCookie            *DeploymentConfigField[bool]            `json:"secure_auth_cookie" typescript:",notnull"`
	SSHKeygenAlgorithm          *DeploymentConfigField[string]          `json:"ssh_keygen_algorithm" typescript:",notnull"`
	AutoImportTemplates         *DeploymentConfigField[[]string]        `json:"auto_import_templates" typescript:",notnull"`
	MetricsCacheRefreshInterval *DeploymentConfigField[time.Duration]   `json:"metrics_cache_refresh_interval" typescript:",notnull"`
	AgentStatRefreshInterval    *DeploymentConfigField[time.Duration]   `json:"agent_stat_refresh_interval" typescript:",notnull"`
	AuditLogging                *DeploymentConfigField[bool]            `json:"audit_logging" typescript:",notnull"`
	BrowserOnly                 *DeploymentConfigField[bool]            `json:"browser_only" typescript:",notnull"`
	SCIMAPIKey                  *DeploymentConfigField[string]          `json:"scim_api_key" typescript:",notnull"`
	UserWorkspaceQuota          *DeploymentConfigField[int]             `json:"user_workspace_quota" typescript:",notnull"`
	Provisioner                 *ProvisionerConfig                      `json:"provisioner" typescript:",notnull"`
	APIRateLimit                *DeploymentConfigField[int]             `json:"api_rate_limit" typescript:",notnull"`
	Experimental                *DeploymentConfigField[bool]            `json:"experimental" typescript:",notnull"`
}

type DERP struct {
	Server *DERPServerConfig `json:"server" typescript:",notnull"`
	Config *DERPConfig       `json:"config" typescript:",notnull"`
}

type DERPServerConfig struct {
	Enable        *DeploymentConfigField[bool]     `json:"enable" typescript:",notnull"`
	RegionID      *DeploymentConfigField[int]      `json:"region_id" typescript:",notnull"`
	RegionCode    *DeploymentConfigField[string]   `json:"region_code" typescript:",notnull"`
	RegionName    *DeploymentConfigField[string]   `json:"region_name" typescript:",notnull"`
	STUNAddresses *DeploymentConfigField[[]string] `json:"stun_addresses" typescript:",notnull"`
	RelayURL      *DeploymentConfigField[string]   `json:"relay_url" typescript:",notnull"`
}

type DERPConfig struct {
	URL  *DeploymentConfigField[string] `json:"url" typescript:",notnull"`
	Path *DeploymentConfigField[string] `json:"path" typescript:",notnull"`
}

type PrometheusConfig struct {
	Enable  *DeploymentConfigField[bool]   `json:"enable" typescript:",notnull"`
	Address *DeploymentConfigField[string] `json:"address" typescript:",notnull"`
}

type PprofConfig struct {
	Enable  *DeploymentConfigField[bool]   `json:"enable" typescript:",notnull"`
	Address *DeploymentConfigField[string] `json:"address" typescript:",notnull"`
}

type OAuth2Config struct {
	Github *OAuth2GithubConfig `json:"github" typescript:",notnull"`
}

type OAuth2GithubConfig struct {
	ClientID          *DeploymentConfigField[string]   `json:"client_id" typescript:",notnull"`
	ClientSecret      *DeploymentConfigField[string]   `json:"client_secret" typescript:",notnull"`
	AllowedOrgs       *DeploymentConfigField[[]string] `json:"allowed_orgs" typescript:",notnull"`
	AllowedTeams      *DeploymentConfigField[[]string] `json:"allowed_teams" typescript:",notnull"`
	AllowSignups      *DeploymentConfigField[bool]     `json:"allow_signups" typescript:",notnull"`
	EnterpriseBaseURL *DeploymentConfigField[string]   `json:"enterprise_base_url" typescript:",notnull"`
}

type OIDCConfig struct {
	AllowSignups *DeploymentConfigField[bool]     `json:"allow_signups" typescript:",notnull"`
	ClientID     *DeploymentConfigField[string]   `json:"client_id" typescript:",notnull"`
	ClientSecret *DeploymentConfigField[string]   `json:"client_secret" typescript:",notnull"`
	EmailDomain  *DeploymentConfigField[string]   `json:"email_domain" typescript:",notnull"`
	IssuerURL    *DeploymentConfigField[string]   `json:"issuer_url" typescript:",notnull"`
	Scopes       *DeploymentConfigField[[]string] `json:"scopes" typescript:",notnull"`
}

type TelemetryConfig struct {
	Enable *DeploymentConfigField[bool]   `json:"enable" typescript:",notnull"`
	Trace  *DeploymentConfigField[bool]   `json:"trace" typescript:",notnull"`
	URL    *DeploymentConfigField[string] `json:"url" typescript:",notnull"`
}

type TLSConfig struct {
	Enable         *DeploymentConfigField[bool]     `json:"enable" typescript:",notnull"`
	CertFiles      *DeploymentConfigField[[]string] `json:"cert_file" typescript:",notnull"`
	ClientAuth     *DeploymentConfigField[string]   `json:"client_auth" typescript:",notnull"`
	ClientCAFile   *DeploymentConfigField[string]   `json:"client_ca_file" typescript:",notnull"`
	KeyFiles       *DeploymentConfigField[[]string] `json:"key_file" typescript:",notnull"`
	MinVersion     *DeploymentConfigField[string]   `json:"min_version" typescript:",notnull"`
	ClientCertFile *DeploymentConfigField[string]   `json:"client_cert_file" typescript:",notnull"`
	ClientKeyFile  *DeploymentConfigField[string]   `json:"client_key_file" typescript:",notnull"`
}

type TraceConfig struct {
	Enable          *DeploymentConfigField[bool]   `json:"enable" typescript:",notnull"`
	HoneycombAPIKey *DeploymentConfigField[string] `json:"honeycomb_api_key" typescript:",notnull"`
	CaptureLogs     *DeploymentConfigField[bool]   `json:"capture_logs" typescript:",notnull"`
}

type GitAuthConfig struct {
	ID           string   `json:"id"`
	Type         string   `json:"type"`
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"-" yaml:"client_secret"`
	AuthURL      string   `json:"auth_url"`
	TokenURL     string   `json:"token_url"`
	Regex        string   `json:"regex"`
	Scopes       []string `json:"scopes"`
}

type ProvisionerConfig struct {
	Daemons             *DeploymentConfigField[int]           `json:"daemons" typescript:",notnull"`
	ForceCancelInterval *DeploymentConfigField[time.Duration] `json:"force_cancel_interval" typescript:",notnull"`
}

type Flaggable interface {
	string | time.Duration | bool | int | []string | []GitAuthConfig
}

type DeploymentConfigField[T Flaggable] struct {
	Name       string `json:"name"`
	Usage      string `json:"usage"`
	Flag       string `json:"flag"`
	Shorthand  string `json:"shorthand"`
	Enterprise bool   `json:"enterprise"`
	Hidden     bool   `json:"hidden"`
	Secret     bool   `json:"secret"`
	Default    T      `json:"default"`
	Value      T      `json:"value"`
}

// MarshalJSON removes the Value field from the JSON output of any fields marked Secret.
// nolint:revive
func (f *DeploymentConfigField[T]) MarshalJSON() ([]byte, error) {
	copy := struct {
		Name       string `json:"name"`
		Usage      string `json:"usage"`
		Flag       string `json:"flag"`
		Shorthand  string `json:"shorthand"`
		Enterprise bool   `json:"enterprise"`
		Hidden     bool   `json:"hidden"`
		Secret     bool   `json:"secret"`
		Default    T      `json:"default"`
		Value      T      `json:"value"`
	}{
		Name:       f.Name,
		Usage:      f.Usage,
		Flag:       f.Flag,
		Shorthand:  f.Shorthand,
		Enterprise: f.Enterprise,
		Hidden:     f.Hidden,
		Secret:     f.Secret,
	}

	if !f.Secret {
		copy.Default = f.Default
		copy.Value = f.Value
	}

	return json.Marshal(copy)
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

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
type DeploymentConfig struct {
	AccessURL                   DeploymentConfigField[string]        `json:"access_url"`
	WildcardAccessURL           DeploymentConfigField[string]        `json:"wildcard_access_url"`
	Address                     DeploymentConfigField[string]        `json:"address"`
	AutobuildPollInterval       DeploymentConfigField[time.Duration] `json:"autobuild_poll_interval"`
	DERP                        DERP                                 `json:"derp"`
	Prometheus                  PrometheusConfig                     `json:"prometheus"`
	Pprof                       PprofConfig                          `json:"pprof"`
	CacheDirectory              DeploymentConfigField[string]        `json:"cache_directory"`
	InMemoryDatabase            DeploymentConfigField[bool]          `json:"in_memory_database"`
	ProvisionerDaemons          DeploymentConfigField[int]           `json:"provisioner_daemon_count"`
	PostgresURL                 DeploymentConfigField[string]        `json:"pg_connection_url"`
	OAuth2                      OAuth2Config                         `json:"oauth2"`
	OIDC                        OIDCConfig                           `json:"oidc"`
	Telemetry                   TelemetryConfig                      `json:"telemetry"`
	TLS                         TLSConfig                            `json:"tls"`
	TraceEnable                 DeploymentConfigField[bool]          `json:"trace_enable"`
	SecureAuthCookie            DeploymentConfigField[bool]          `json:"secure_auth_cookie"`
	SSHKeygenAlgorithm          DeploymentConfigField[string]        `json:"ssh_keygen_algorithm"`
	AutoImportTemplates         DeploymentConfigField[[]string]      `json:"auto_import_templates"`
	MetricsCacheRefreshInterval DeploymentConfigField[time.Duration] `json:"metrics_cache_refresh_interval"`
	AgentStatRefreshInterval    DeploymentConfigField[time.Duration] `json:"agent_stat_refresh_interval"`
	AuditLogging                DeploymentConfigField[bool]          `json:"audit_logging"`
	BrowserOnly                 DeploymentConfigField[bool]          `json:"browser_only"`
	SCIMAPIKey                  DeploymentConfigField[string]        `json:"scim_api_key"`
	UserWorkspaceQuota          DeploymentConfigField[int]           `json:"user_workspace_quota"`
}

type DERP struct {
	Server DERPServerConfig `json:"server"`
	Config DERPConfig       `json:"config"`
}

type DERPServerConfig struct {
	Enable        DeploymentConfigField[bool]     `json:"enabled"`
	RegionID      DeploymentConfigField[int]      `json:"region_id"`
	RegionCode    DeploymentConfigField[string]   `json:"region_code"`
	RegionName    DeploymentConfigField[string]   `json:"region_name"`
	STUNAddresses DeploymentConfigField[[]string] `json:"stun_address"`
	RelayAddress  DeploymentConfigField[string]   `json:"relay_address"`
}

type DERPConfig struct {
	URL  DeploymentConfigField[string] `json:"url"`
	Path DeploymentConfigField[string] `json:"path"`
}

type PrometheusConfig struct {
	Enable  DeploymentConfigField[bool]   `json:"enabled"`
	Address DeploymentConfigField[string] `json:"address"`
}

type PprofConfig struct {
	Enable  DeploymentConfigField[bool]   `json:"enabled"`
	Address DeploymentConfigField[string] `json:"address"`
}

type OAuth2Config struct {
	Github OAuth2GithubConfig `json:"github"`
}

type OAuth2GithubConfig struct {
	ClientID             DeploymentConfigField[string]   `json:"client_id"`
	ClientSecret         DeploymentConfigField[string]   `json:"client_secret"`
	AllowedOrganizations DeploymentConfigField[[]string] `json:"allowed_organizations"`
	AllowedTeams         DeploymentConfigField[[]string] `json:"allowed_teams"`
	AllowSignups         DeploymentConfigField[bool]     `json:"allow_signups"`
	EnterpriseBaseURL    DeploymentConfigField[string]   `json:"enterprise_base_url"`
}

type OIDCConfig struct {
	AllowSignups DeploymentConfigField[bool]     `json:"allow_signups"`
	ClientID     DeploymentConfigField[string]   `json:"client_id"`
	ClientSecret DeploymentConfigField[string]   `json:"client_secret"`
	EmailDomain  DeploymentConfigField[string]   `json:"email_domain"`
	IssuerURL    DeploymentConfigField[string]   `json:"issuer_url"`
	Scopes       DeploymentConfigField[[]string] `json:"scopes"`
}

type TelemetryConfig struct {
	Enable DeploymentConfigField[bool]   `json:"enabled"`
	Trace  DeploymentConfigField[bool]   `json:"trace"`
	URL    DeploymentConfigField[string] `json:"url"`
}

type TLSConfig struct {
	Enable       DeploymentConfigField[bool]     `json:"enable"`
	CertFiles    DeploymentConfigField[[]string] `json:"cert_files"`
	ClientAuth   DeploymentConfigField[string]   `json:"client_auth"`
	ClientCAFile DeploymentConfigField[string]   `json:"client_ca_file"`
	KeyFiles     DeploymentConfigField[[]string] `json:"key_files"`
	MinVersion   DeploymentConfigField[string]   `json:"min_version"`
}

type Flaggable interface {
	string | bool | int | time.Duration | []string
}

type DeploymentConfigField[T Flaggable] struct {
	// Key        string `json:"key"`
	Name       string `json:"name"`
	Usage      string `json:"usage"`
	Flag       string `json:"flag"`
	Shorthand  string `json:"shorthand"`
	Enterprise bool   `json:"enterprise"`
	Hidden     bool   `json:"hidden"`
	Value      T      `json:"value"`
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

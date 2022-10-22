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
	AccessURL                     DeploymentConfigField[string]        `json:"access_url"`
	WildcardAccessURL             DeploymentConfigField[string]        `json:"wildcard_access_url"`
	Address                       DeploymentConfigField[string]        `json:"address"`
	AutobuildPollInterval         DeploymentConfigField[time.Duration] `json:"autobuild_poll_interval"`
	DERPServerEnable              DeploymentConfigField[bool]          `json:"derp_server_enabled"`
	DERPServerRegionID            DeploymentConfigField[int]           `json:"derp_server_region_id"`
	DERPServerRegionCode          DeploymentConfigField[string]        `json:"derp_server_region_code"`
	DERPServerRegionName          DeploymentConfigField[string]        `json:"derp_server_region_name"`
	DERPServerSTUNAddresses       DeploymentConfigField[[]string]      `json:"derp_server_stun_address"`
	DERPServerRelayURL            DeploymentConfigField[string]        `json:"derp_server_relay_address"`
	DERPConfigURL                 DeploymentConfigField[string]        `json:"derp_config_url"`
	DERPConfigPath                DeploymentConfigField[string]        `json:"derp_config_path"`
	PrometheusEnable              DeploymentConfigField[bool]          `json:"prometheus_enabled"`
	PrometheusAddress             DeploymentConfigField[string]        `json:"prometheus_address"`
	PprofEnable                   DeploymentConfigField[bool]          `json:"pprof_enabled"`
	PprofAddress                  DeploymentConfigField[string]        `json:"pprof_address"`
	CacheDirectory                DeploymentConfigField[string]        `json:"cache_directory"`
	InMemoryDatabase              DeploymentConfigField[bool]          `json:"in_memory_database"`
	ProvisionerDaemons            DeploymentConfigField[int]           `json:"provisioner_daemon_count"`
	PostgresURL                   DeploymentConfigField[string]        `json:"-"`
	OAuth2GithubClientID          DeploymentConfigField[string]        `json:"oauth2_github_client_id"`
	OAuth2GithubClientSecret      DeploymentConfigField[string]        `json:"-"`
	OAuth2GithubAllowedOrgs       DeploymentConfigField[[]string]      `json:"oauth2_github_allowed_orgs"`
	OAuth2GithubAllowedTeams      DeploymentConfigField[[]string]      `json:"oauth2_github_allowed_teams"`
	OAuth2GithubAllowSignups      DeploymentConfigField[bool]          `json:"oauth2_github_allow_signups"`
	OAuth2GithubEnterpriseBaseURL DeploymentConfigField[string]        `json:"oauth2_github_enterprise_base_url"`
	OIDCAllowSignups              DeploymentConfigField[bool]          `json:"oidc_allow_signups"`
	OIDCClientID                  DeploymentConfigField[string]        `json:"oidc_client_id"`
	OIDCClientSecret              DeploymentConfigField[string]        `json:"-"`
	OIDCEmailDomain               DeploymentConfigField[string]        `json:"oidc_email_domain"`
	OIDCIssuerURL                 DeploymentConfigField[string]        `json:"oidc_issuer_url"`
	OIDCScopes                    DeploymentConfigField[[]string]      `json:"oidc_scopes"`
	TelemetryEnable               DeploymentConfigField[bool]          `json:"telemetry_enable"`
	TelemetryTrace                DeploymentConfigField[bool]          `json:"telemetry_trace_enable"`
	TelemetryURL                  DeploymentConfigField[string]        `json:"telemetry_url"`
	TLSEnable                     DeploymentConfigField[bool]          `json:"tls_enable"`
	TLSCertFiles                  DeploymentConfigField[[]string]      `json:"tls_cert_files"`
	TLSClientCAFile               DeploymentConfigField[string]        `json:"tls_client_ca_file"`
	TLSClientAuth                 DeploymentConfigField[string]        `json:"tls_client_auth"`
	TLSKeyFiles                   DeploymentConfigField[[]string]      `json:"tls_key_files"`
	TLSMinVersion                 DeploymentConfigField[string]        `json:"tls_min_version"`
	TraceEnable                   DeploymentConfigField[bool]          `json:"trace_enable"`
	SecureAuthCookie              DeploymentConfigField[bool]          `json:"secure_auth_cookie"`
	SSHKeygenAlgorithm            DeploymentConfigField[string]        `json:"ssh_keygen_algorithm"`
	AutoImportTemplates           DeploymentConfigField[[]string]      `json:"auto_import_templates"`
	MetricsCacheRefreshInterval   DeploymentConfigField[time.Duration] `json:"metrics_cache_refresh_interval"`
	AgentStatRefreshInterval      DeploymentConfigField[time.Duration] `json:"agent_stat_refresh_interval"`
	AuditLogging                  DeploymentConfigField[bool]          `json:"audit_logging"`
	BrowserOnly                   DeploymentConfigField[bool]          `json:"browser_only"`
	SCIMAPIKey                    DeploymentConfigField[string]        `json:"-"`
	UserWorkspaceQuota            DeploymentConfigField[int]           `json:"user_workspace_quota"`
}

type Flaggable interface {
	string | bool | int | time.Duration | []string
}

type DeploymentConfigField[T Flaggable] struct {
	Key        string `json:"key"`
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

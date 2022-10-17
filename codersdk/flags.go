package codersdk

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"golang.org/x/xerrors"
)

type DeploymentFlags struct {
	AccessURL                        *StringFlag      `json:"access_url" typescript:",notnull"`
	WildcardAccessURL                *StringFlag      `json:"wildcard_access_url" typescript:",notnull"`
	Address                          *StringFlag      `json:"address" typescript:",notnull"`
	AutobuildPollInterval            *DurationFlag    `json:"autobuild_poll_interval" typescript:",notnull"`
	DerpServerEnable                 *BoolFlag        `json:"derp_server_enabled" typescript:",notnull"`
	DerpServerRegionID               *IntFlag         `json:"derp_server_region_id" typescript:",notnull"`
	DerpServerRegionCode             *StringFlag      `json:"derp_server_region_code" typescript:",notnull"`
	DerpServerRegionName             *StringFlag      `json:"derp_server_region_name" typescript:",notnull"`
	DerpServerSTUNAddresses          *StringArrayFlag `json:"derp_server_stun_address" typescript:",notnull"`
	DerpServerRelayAddress           *StringFlag      `json:"derp_server_relay_address" typescript:",notnull"`
	DerpConfigURL                    *StringFlag      `json:"derp_config_url" typescript:",notnull"`
	DerpConfigPath                   *StringFlag      `json:"derp_config_path" typescript:",notnull"`
	PromEnabled                      *BoolFlag        `json:"prom_enabled" typescript:",notnull"`
	PromAddress                      *StringFlag      `json:"prom_address" typescript:",notnull"`
	PprofEnabled                     *BoolFlag        `json:"pprof_enabled" typescript:",notnull"`
	PprofAddress                     *StringFlag      `json:"pprof_address" typescript:",notnull"`
	CacheDir                         *StringFlag      `json:"cache_dir" typescript:",notnull"`
	InMemoryDatabase                 *BoolFlag        `json:"in_memory_database" typescript:",notnull"`
	ProvisionerDaemonCount           *IntFlag         `json:"provisioner_daemon_count" typescript:",notnull"`
	PostgresURL                      *StringFlag      `json:"postgres_url" typescript:",notnull"`
	OAuth2GithubClientID             *StringFlag      `json:"oauth2_github_client_id" typescript:",notnull"`
	OAuth2GithubClientSecret         *StringFlag      `json:"oauth2_github_client_secret" typescript:",notnull"`
	OAuth2GithubAllowedOrganizations *StringArrayFlag `json:"oauth2_github_allowed_organizations" typescript:",notnull"`
	OAuth2GithubAllowedTeams         *StringArrayFlag `json:"oauth2_github_allowed_teams" typescript:",notnull"`
	OAuth2GithubAllowSignups         *BoolFlag        `json:"oauth2_github_allow_signups" typescript:",notnull"`
	OAuth2GithubEnterpriseBaseURL    *StringFlag      `json:"oauth2_github_enterprise_base_url" typescript:",notnull"`
	OIDCAllowSignups                 *BoolFlag        `json:"oidc_allow_signups" typescript:",notnull"`
	OIDCClientID                     *StringFlag      `json:"oidc_client_id" typescript:",notnull"`
	OIDCClientSecret                 *StringFlag      `json:"oidc_client_secret" typescript:",notnull"`
	OIDCEmailDomain                  *StringFlag      `json:"oidc_email_domain" typescript:",notnull"`
	OIDCIssuerURL                    *StringFlag      `json:"oidc_issuer_url" typescript:",notnull"`
	OIDCScopes                       *StringArrayFlag `json:"oidc_scopes" typescript:",notnull"`
	TelemetryEnable                  *BoolFlag        `json:"telemetry_enable" typescript:",notnull"`
	TelemetryTraceEnable             *BoolFlag        `json:"telemetry_trace_enable" typescript:",notnull"`
	TelemetryURL                     *StringFlag      `json:"telemetry_url" typescript:",notnull"`
	TLSEnable                        *BoolFlag        `json:"tls_enable" typescript:",notnull"`
	TLSCertFiles                     *StringArrayFlag `json:"tls_cert_files" typescript:",notnull"`
	TLSClientCAFile                  *StringFlag      `json:"tls_client_ca_file" typescript:",notnull"`
	TLSClientAuth                    *StringFlag      `json:"tls_client_auth" typescript:",notnull"`
	TLSKeyFiles                      *StringArrayFlag `json:"tls_key_files" typescript:",notnull"`
	TLSMinVersion                    *StringFlag      `json:"tls_min_version" typescript:",notnull"`
	TraceEnable                      *BoolFlag        `json:"trace_enable" typescript:",notnull"`
	SecureAuthCookie                 *BoolFlag        `json:"secure_auth_cookie" typescript:",notnull"`
	SSHKeygenAlgorithm               *StringFlag      `json:"ssh_keygen_algorithm" typescript:",notnull"`
	AutoImportTemplates              *StringArrayFlag `json:"auto_import_templates" typescript:",notnull"`
	MetricsCacheRefreshInterval      *DurationFlag    `json:"metrics_cache_refresh_interval" typescript:",notnull"`
	AgentStatRefreshInterval         *DurationFlag    `json:"agent_stat_refresh_interval" typescript:",notnull"`
	Verbose                          *BoolFlag        `json:"verbose" typescript:",notnull"`
	AuditLogging                     *BoolFlag        `json:"audit_logging" typescript:",notnull"`
	BrowserOnly                      *BoolFlag        `json:"browser_only" typescript:",notnull"`
	SCIMAuthHeader                   *StringFlag      `json:"scim_auth_header" typescript:",notnull"`
	UserWorkspaceQuota               *IntFlag         `json:"user_workspace_quota" typescript:",notnull"`
}

type StringFlag struct {
	Name        string `json:"name"`
	Flag        string `json:"flag"`
	EnvVar      string `json:"env_var"`
	Shorthand   string `json:"shorthand"`
	Description string `json:"description"`
	Enterprise  bool   `json:"enterprise"`
	Secret      bool   `json:"secret"`
	Hidden      bool   `json:"hidden"`
	Default     string `json:"default"`
	Value       string `json:"value"`
}

type BoolFlag struct {
	Name        string `json:"name"`
	Flag        string `json:"flag"`
	EnvVar      string `json:"env_var"`
	Shorthand   string `json:"shorthand"`
	Description string `json:"description"`
	Enterprise  bool   `json:"enterprise"`
	Hidden      bool   `json:"hidden"`
	Default     bool   `json:"default"`
	Value       bool   `json:"value"`
}

type IntFlag struct {
	Name        string `json:"name"`
	Flag        string `json:"flag"`
	EnvVar      string `json:"env_var"`
	Shorthand   string `json:"shorthand"`
	Description string `json:"description"`
	Enterprise  bool   `json:"enterprise"`
	Hidden      bool   `json:"hidden"`
	Default     int    `json:"default"`
	Value       int    `json:"value"`
}

type DurationFlag struct {
	Name        string        `json:"name"`
	Flag        string        `json:"flag"`
	EnvVar      string        `json:"env_var"`
	Shorthand   string        `json:"shorthand"`
	Description string        `json:"description"`
	Enterprise  bool          `json:"enterprise"`
	Hidden      bool          `json:"hidden"`
	Default     time.Duration `json:"default"`
	Value       time.Duration `json:"value"`
}

type StringArrayFlag struct {
	Name        string   `json:"name"`
	Flag        string   `json:"flag"`
	EnvVar      string   `json:"env_var"`
	Shorthand   string   `json:"shorthand"`
	Description string   `json:"description"`
	Enterprise  bool     `json:"enterprise"`
	Hidden      bool     `json:"hidden"`
	Default     []string `json:"default"`
	Value       []string `json:"value"`
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

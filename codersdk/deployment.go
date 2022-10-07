package codersdk

import "time"

type DeploymentFlags struct {
	AccessURL                        StringFlag      `json:"access_url"`
	WildcardAccessURL                StringFlag      `json:"wildcard_access_url"`
	Address                          StringFlag      `json:"address"`
	AutobuildPollInterval            DurationFlag    `json:"autobuild_poll_interval"`
	DerpServerEnable                 BoolFlag        `json:"derp_server_enabled"`
	DerpServerRegionID               IntFlag         `json:"derp_server_region_id"`
	DerpServerRegionCode             StringFlag      `json:"derp_server_region_code"`
	DerpServerRegionName             StringFlag      `json:"derp_server_region_name"`
	DerpServerSTUNAddresses          StringArrayFlag `json:"derp_server_stun_address"`
	DerpConfigURL                    StringFlag      `json:"derp_config_url"`
	DerpConfigPath                   StringFlag      `json:"derp_config_path"`
	PromEnabled                      BoolFlag        `json:"prom_enabled"`
	PromAddress                      StringFlag      `json:"prom_address"`
	PprofEnabled                     BoolFlag        `json:"pprof_enabled"`
	PprofAddress                     StringFlag      `json:"pprof_address"`
	CacheDir                         StringFlag      `json:"cache_dir"`
	InMemoryDatabase                 BoolFlag        `json:"in_memory_database"`
	ProvisionerDaemonCount           IntFlag         `json:"provisioner_daemon_count"`
	PostgresURL                      StringFlag      `json:"postgres_url"`
	Oauth2GithubClientID             StringFlag      `json:"oauth2_github_client_id"`
	Oauth2GithubClientSecret         StringFlag      `json:"oauth2_github_client_secret"`
	Oauth2GithubAllowedOrganizations StringArrayFlag `json:"oauth2_github_allowed_organizations"`
	Oauth2GithubAllowedTeams         StringArrayFlag `json:"oauth2_github_allowed_teams"`
	Oauth2GithubAllowSignups         BoolFlag        `json:"oauth2_github_allow_signups"`
	Oauth2GithubEnterpriseBaseURL    StringFlag      `json:"oauth2_github_enterprise_base_url"`
	OidcAllowSignups                 BoolFlag        `json:"oidc_allow_signups"`
	OidcClientID                     StringFlag      `json:"oidc_client_id"`
	OidcClientSecret                 StringFlag      `json:"oidc_cliet_secret"`
	OidcEmailDomain                  StringFlag      `json:"oidc_email_domain"`
	OidcIssuerURL                    StringFlag      `json:"oidc_issuer_url"`
	OidcScopes                       StringArrayFlag `json:"oidc_scopes"`
	TailscaleEnable                  BoolFlag        `json:"tailscale_enable"`
	TelemetryEnable                  BoolFlag        `json:"telemetry_enable"`
	TelemetryTraceEnable             BoolFlag        `json:"telemetry_trace_enable"`
	TelemetryURL                     StringFlag      `json:"telemetry_url"`
	TLSEnable                        BoolFlag        `json:"tls_enable"`
	TLSCertFiles                     StringArrayFlag `json:"tls_cert_files"`
	TLSClientCAFile                  StringFlag      `json:"tls_client_ca_file"`
	TLSClientAuth                    StringFlag      `json:"tls_client_auth"`
	TLSKeyFiles                      StringArrayFlag `json:"tls_key_tiles"`
	TLSMinVersion                    StringFlag      `json:"tls_min_version"`
	TraceEnable                      BoolFlag        `json:"trace_enable"`
	SecureAuthCookie                 BoolFlag        `json:"secure_auth_cookie"`
	SSHKeygenAlgorithm               StringFlag      `json:"ssh_keygen_algorithm"`
	AutoImportTemplates              StringArrayFlag `json:"auto_import_templates"`
	MetricsCacheRefreshInterval      DurationFlag    `json:"metrics_cache_refresh_interval"`
	AgentStatRefreshInterval         DurationFlag    `json:"agent_stat_refresh_interval"`
	Verbose                          BoolFlag        `json:"verbose"`
}

type StringFlag struct {
	Name        string `json:"name"`
	Flag        string `json:"flag"`
	EnvVar      string `json:"env_var"`
	Shorthand   string `json:"shorthand"`
	Description string `json:"description"`
	Enterprise  bool   `json:"enterprise"`
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
	Default     []string `json:"default"`
	Value       []string `json:"value"`
}

package codersdk

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"golang.org/x/mod/semver"
	"golang.org/x/xerrors"
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
	AccessURL                       *DeploymentConfigField[string]          `json:"access_url" typescript:",notnull"`
	WildcardAccessURL               *DeploymentConfigField[string]          `json:"wildcard_access_url" typescript:",notnull"`
	RedirectToAccessURL             *DeploymentConfigField[bool]            `json:"redirect_to_access_url" typescript:",notnull"`
	HTTPAddress                     *DeploymentConfigField[string]          `json:"http_address" typescript:",notnull"`
	AutobuildPollInterval           *DeploymentConfigField[time.Duration]   `json:"autobuild_poll_interval" typescript:",notnull"`
	DERP                            *DERP                                   `json:"derp" typescript:",notnull"`
	GitAuth                         *DeploymentConfigField[[]GitAuthConfig] `json:"gitauth" typescript:",notnull"`
	Prometheus                      *PrometheusConfig                       `json:"prometheus" typescript:",notnull"`
	Pprof                           *PprofConfig                            `json:"pprof" typescript:",notnull"`
	ProxyTrustedHeaders             *DeploymentConfigField[[]string]        `json:"proxy_trusted_headers" typescript:",notnull"`
	ProxyTrustedOrigins             *DeploymentConfigField[[]string]        `json:"proxy_trusted_origins" typescript:",notnull"`
	CacheDirectory                  *DeploymentConfigField[string]          `json:"cache_directory" typescript:",notnull"`
	InMemoryDatabase                *DeploymentConfigField[bool]            `json:"in_memory_database" typescript:",notnull"`
	PostgresURL                     *DeploymentConfigField[string]          `json:"pg_connection_url" typescript:",notnull"`
	OAuth2                          *OAuth2Config                           `json:"oauth2" typescript:",notnull"`
	OIDC                            *OIDCConfig                             `json:"oidc" typescript:",notnull"`
	Telemetry                       *TelemetryConfig                        `json:"telemetry" typescript:",notnull"`
	TLS                             *TLSConfig                              `json:"tls" typescript:",notnull"`
	Trace                           *TraceConfig                            `json:"trace" typescript:",notnull"`
	SecureAuthCookie                *DeploymentConfigField[bool]            `json:"secure_auth_cookie" typescript:",notnull"`
	StrictTransportSecurity         *DeploymentConfigField[int]             `json:"strict_transport_security" typescript:",notnull"`
	StrictTransportSecurityOptions  *DeploymentConfigField[[]string]        `json:"strict_transport_security_options" typescript:",notnull"`
	SSHKeygenAlgorithm              *DeploymentConfigField[string]          `json:"ssh_keygen_algorithm" typescript:",notnull"`
	MetricsCacheRefreshInterval     *DeploymentConfigField[time.Duration]   `json:"metrics_cache_refresh_interval" typescript:",notnull"`
	AgentStatRefreshInterval        *DeploymentConfigField[time.Duration]   `json:"agent_stat_refresh_interval" typescript:",notnull"`
	AgentFallbackTroubleshootingURL *DeploymentConfigField[string]          `json:"agent_fallback_troubleshooting_url" typescript:",notnull"`
	AuditLogging                    *DeploymentConfigField[bool]            `json:"audit_logging" typescript:",notnull"`
	BrowserOnly                     *DeploymentConfigField[bool]            `json:"browser_only" typescript:",notnull"`
	SCIMAPIKey                      *DeploymentConfigField[string]          `json:"scim_api_key" typescript:",notnull"`
	Provisioner                     *ProvisionerConfig                      `json:"provisioner" typescript:",notnull"`
	RateLimit                       *RateLimitConfig                        `json:"rate_limit" typescript:",notnull"`
	Experiments                     *DeploymentConfigField[[]string]        `json:"experiments" typescript:",notnull"`
	UpdateCheck                     *DeploymentConfigField[bool]            `json:"update_check" typescript:",notnull"`
	MaxTokenLifetime                *DeploymentConfigField[time.Duration]   `json:"max_token_lifetime" typescript:",notnull"`
	Swagger                         *SwaggerConfig                          `json:"swagger" typescript:",notnull"`
	Logging                         *LoggingConfig                          `json:"logging" typescript:",notnull"`
	Dangerous                       *DangerousConfig                        `json:"dangerous" typescript:",notnull"`
	DisablePathApps                 *DeploymentConfigField[bool]            `json:"disable_path_apps" typescript:",notnull"`
	SessionDuration                 *DeploymentConfigField[time.Duration]   `json:"max_session_expiry" typescript:",notnull"`
	DisableSessionExpiryRefresh     *DeploymentConfigField[bool]            `json:"disable_session_expiry_refresh" typescript:",notnull"`
	DisablePasswordAuth             *DeploymentConfigField[bool]            `json:"disable_password_auth" typescript:",notnull"`

	// DEPRECATED: Use HTTPAddress or TLS.Address instead.
	Address *DeploymentConfigField[string] `json:"address" typescript:",notnull"`
	// DEPRECATED: Use Experiments instead.
	Experimental *DeploymentConfigField[bool] `json:"experimental" typescript:",notnull"`

	Support *SupportConfig `json:"support" typescript:",notnull"`
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
	AllowEveryone     *DeploymentConfigField[bool]     `json:"allow_everyone" typescript:",notnull"`
	EnterpriseBaseURL *DeploymentConfigField[string]   `json:"enterprise_base_url" typescript:",notnull"`
}

type OIDCConfig struct {
	AllowSignups        *DeploymentConfigField[bool]     `json:"allow_signups" typescript:",notnull"`
	ClientID            *DeploymentConfigField[string]   `json:"client_id" typescript:",notnull"`
	ClientSecret        *DeploymentConfigField[string]   `json:"client_secret" typescript:",notnull"`
	EmailDomain         *DeploymentConfigField[[]string] `json:"email_domain" typescript:",notnull"`
	IssuerURL           *DeploymentConfigField[string]   `json:"issuer_url" typescript:",notnull"`
	Scopes              *DeploymentConfigField[[]string] `json:"scopes" typescript:",notnull"`
	IgnoreEmailVerified *DeploymentConfigField[bool]     `json:"ignore_email_verified" typescript:",notnull"`
	UsernameField       *DeploymentConfigField[string]   `json:"username_field" typescript:",notnull"`
	SignInText          *DeploymentConfigField[string]   `json:"sign_in_text" typescript:",notnull"`
	IconURL             *DeploymentConfigField[string]   `json:"icon_url" typescript:",notnull"`
}

type TelemetryConfig struct {
	Enable *DeploymentConfigField[bool]   `json:"enable" typescript:",notnull"`
	Trace  *DeploymentConfigField[bool]   `json:"trace" typescript:",notnull"`
	URL    *DeploymentConfigField[string] `json:"url" typescript:",notnull"`
}

type TLSConfig struct {
	Enable         *DeploymentConfigField[bool]     `json:"enable" typescript:",notnull"`
	Address        *DeploymentConfigField[string]   `json:"address" typescript:",notnull"`
	RedirectHTTP   *DeploymentConfigField[bool]     `json:"redirect_http" typescript:",notnull"`
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
	ValidateURL  string   `json:"validate_url"`
	Regex        string   `json:"regex"`
	NoRefresh    bool     `json:"no_refresh"`
	Scopes       []string `json:"scopes"`
}

type ProvisionerConfig struct {
	Daemons             *DeploymentConfigField[int]           `json:"daemons" typescript:",notnull"`
	DaemonPollInterval  *DeploymentConfigField[time.Duration] `json:"daemon_poll_interval" typescript:",notnull"`
	DaemonPollJitter    *DeploymentConfigField[time.Duration] `json:"daemon_poll_jitter" typescript:",notnull"`
	ForceCancelInterval *DeploymentConfigField[time.Duration] `json:"force_cancel_interval" typescript:",notnull"`
}

type RateLimitConfig struct {
	DisableAll *DeploymentConfigField[bool] `json:"disable_all" typescript:",notnull"`
	API        *DeploymentConfigField[int]  `json:"api" typescript:",notnull"`
}

type SwaggerConfig struct {
	Enable *DeploymentConfigField[bool] `json:"enable" typescript:",notnull"`
}

type LoggingConfig struct {
	Human       *DeploymentConfigField[string] `json:"human" typescript:",notnull"`
	JSON        *DeploymentConfigField[string] `json:"json" typescript:",notnull"`
	Stackdriver *DeploymentConfigField[string] `json:"stackdriver" typescript:",notnull"`
}

type DangerousConfig struct {
	AllowPathAppSharing         *DeploymentConfigField[bool] `json:"allow_path_app_sharing" typescript:",notnull"`
	AllowPathAppSiteOwnerAccess *DeploymentConfigField[bool] `json:"allow_path_app_site_owner_access" typescript:",notnull"`
}

type SupportConfig struct {
	Links *DeploymentConfigField[[]LinkConfig] `json:"links" typescript:",notnull"`
}

type LinkConfig struct {
	Name   string `json:"name"`
	Target string `json:"target"`
	Icon   string `json:"icon"`
}

type Flaggable interface {
	string | time.Duration | bool | int | []string | []GitAuthConfig | []LinkConfig
}

type DeploymentConfigField[T Flaggable] struct {
	Name  string `json:"name"`
	Usage string `json:"usage"`
	Flag  string `json:"flag"`
	// EnvOverride will override the automatically generated environment
	// variable name. Useful if you're moving values around but need to keep
	// backwards compatibility with old environment variable names.
	//
	// NOTE: this is not supported for array flags.
	EnvOverride string `json:"-"`
	Shorthand   string `json:"shorthand"`
	Enterprise  bool   `json:"enterprise"`
	Hidden      bool   `json:"hidden"`
	Secret      bool   `json:"secret"`
	Default     T      `json:"default"`
	Value       T      `json:"value"`
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
		return DeploymentConfig{}, ReadBodyAsError(res)
	}

	var df DeploymentConfig
	return df, json.NewDecoder(res.Body).Decode(&df)
}

type AppearanceConfig struct {
	LogoURL       string              `json:"logo_url"`
	ServiceBanner ServiceBannerConfig `json:"service_banner"`
	SupportLinks  []LinkConfig        `json:"support_links,omitempty"`
}

type UpdateAppearanceConfig struct {
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

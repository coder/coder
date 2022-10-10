package deployment

import (
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/spf13/pflag"

	"github.com/coder/coder/cli/cliflag"
	"github.com/coder/coder/codersdk"
)

const (
	secretValue = "********"
)

func Flags() codersdk.DeploymentFlags {
	return codersdk.DeploymentFlags{
		AccessURL: codersdk.StringFlag{
			Name:        "Access URL",
			Flag:        "access-url",
			EnvVar:      "CODER_ACCESS_URL",
			Description: "External URL to access your deployment. This must be accessible by all provisioned workspaces.",
		},
		WildcardAccessURL: codersdk.StringFlag{
			Name:        "Wildcard Address URL",
			Flag:        "wildcard-access-url",
			EnvVar:      "CODER_WILDCARD_ACCESS_URL",
			Description: `Specifies the wildcard hostname to use for workspace applications in the form "*.example.com".`,
		},
		Address: codersdk.StringFlag{
			Name:        "Bind Address",
			Flag:        "address",
			EnvVar:      "CODER_ADDRESS",
			Shorthand:   "a",
			Description: "Bind address of the server.",
			Default:     "127.0.0.1:3000",
		},
		AutobuildPollInterval: codersdk.DurationFlag{
			Name:        "Autobuild Poll Interval",
			Flag:        "autobuild-poll-interval",
			EnvVar:      "CODER_AUTOBUILD_POLL_INTERVAL",
			Description: "Interval to poll for scheduled workspace builds.",
			Default:     time.Minute,
		},
		DerpServerEnable: codersdk.BoolFlag{
			Name:        "DERP Server Enabled",
			Flag:        "derp-server-enable",
			EnvVar:      "CODER_DERP_SERVER_ENABLE",
			Description: "Whether to enable or disable the embedded DERP relay server.",
			Default:     true,
		},
		DerpServerRegionID: codersdk.IntFlag{
			Name:        "DERP Server Region ID",
			Flag:        "derp-server-region-id",
			EnvVar:      "CODER_DERP_SERVER_REGION_ID",
			Description: "Region ID to use for the embedded DERP server.",
			Default:     999,
		},
		DerpServerRegionCode: codersdk.StringFlag{
			Name:        "DERP Server Region Code",
			Flag:        "derp-server-region-code",
			EnvVar:      "CODER_DERP_SERVER_REGION_CODE",
			Description: "Region code to use for the embedded DERP server.",
			Default:     "coder",
		},
		DerpServerRegionName: codersdk.StringFlag{
			Name:        "DERP Server Region Name",
			Flag:        "derp-server-region-name",
			EnvVar:      "CODER_DERP_SERVER_REGION_NAME",
			Description: "Region name that for the embedded DERP server.",
			Default:     "Coder Embedded Relay",
		},
		DerpServerSTUNAddresses: codersdk.StringArrayFlag{
			Name:        "DERP Server STUN Addresses",
			Flag:        "derp-server-stun-addresses",
			EnvVar:      "CODER_DERP_SERVER_STUN_ADDRESSES",
			Description: "Addresses for STUN servers to establish P2P connections. Set empty to disable P2P connections.",
			Default:     []string{"stun.l.google.com:19302"},
		},
		DerpConfigURL: codersdk.StringFlag{
			Name:        "DERP Config URL",
			Flag:        "derp-config-url",
			EnvVar:      "CODER_DERP_CONFIG_URL",
			Description: "URL to fetch a DERP mapping on startup. See: https://tailscale.com/kb/1118/custom-derp-servers/",
		},
		DerpConfigPath: codersdk.StringFlag{
			Name:        "DERP Config Path",
			Flag:        "derp-config-path",
			EnvVar:      "CODER_DERP_CONFIG_PATH",
			Description: "Path to read a DERP mapping from. See: https://tailscale.com/kb/1118/custom-derp-servers/",
		},
		PromEnabled: codersdk.BoolFlag{
			Name:        "Prometheus Enabled",
			Flag:        "prometheus-enable",
			EnvVar:      "CODER_PROMETHEUS_ENABLE",
			Description: "Serve prometheus metrics on the address defined by `prometheus-address`.",
		},
		PromAddress: codersdk.StringFlag{
			Name:        "Prometheus Address",
			Flag:        "prometheus-address",
			EnvVar:      "CODER_PROMETHEUS_ADDRESS",
			Description: "The bind address to serve prometheus metrics.",
			Default:     "127.0.0.1:2112",
		},
		PprofEnabled: codersdk.BoolFlag{
			Name:        "pprof Enabled",
			Flag:        "pprof-enable",
			EnvVar:      "CODER_PPROF_ENABLE",
			Description: "Serve pprof metrics on the address defined by `pprof-address`.",
		},
		PprofAddress: codersdk.StringFlag{
			Name:        "pprof Address",
			Flag:        "pprof-address",
			EnvVar:      "CODER_PPROF_ADDRESS",
			Description: "The bind address to serve pprof.",
			Default:     "127.0.0.1:6060",
		},
		CacheDir: codersdk.StringFlag{
			Name:        "Cache Directory",
			Flag:        "cache-dir",
			EnvVar:      "CODER_CACHE_DIRECTORY",
			Description: "The directory to cache temporary files. If unspecified and $CACHE_DIRECTORY is set, it will be used for compatibility with systemd.",
			Default:     defaultCacheDir(),
		},
		InMemoryDatabase: codersdk.BoolFlag{
			Name:        "In-Memory Database",
			Flag:        "in-memory",
			EnvVar:      "CODER_INMEMORY",
			Description: "Controls whether data will be stored in an in-memory database.",
		},
		ProvisionerDaemonCount: codersdk.IntFlag{
			Name:        "Provisioner Daemons",
			Flag:        "provisioner-daemons",
			EnvVar:      "CODER_PROVISIONER_DAEMONS",
			Description: "Number of provisioner daemons to create on start. If builds are stuck in queued state for a long time, consider increasing this.",
			Default:     3,
		},
		PostgresURL: codersdk.StringFlag{
			Name:        "Postgres URL",
			Flag:        "postgres-url",
			EnvVar:      "CODER_PG_CONNECTION_URL",
			Description: "URL of a PostgreSQL database. If empty, PostgreSQL binaries will be downloaded from Maven (https://repo1.maven.org/maven2) and store all data in the config root. Access the built-in database with \"coder server postgres-builtin-url\"",
			Secret:      true,
		},
		OAuth2GithubClientID: codersdk.StringFlag{
			Name:        "Oauth2 Github Client ID",
			Flag:        "oauth2-github-client-id",
			EnvVar:      "CODER_OAUTH2_GITHUB_CLIENT_ID",
			Description: "Client ID for Login with GitHub.",
		},
		OAuth2GithubClientSecret: codersdk.StringFlag{
			Name:        "Oauth2 Github Client Secret",
			Flag:        "oauth2-github-client-secret",
			EnvVar:      "CODER_OAUTH2_GITHUB_CLIENT_SECRET",
			Description: "Client secret for Login with GitHub.",
			Secret:      true,
		},
		OAuth2GithubAllowedOrganizations: codersdk.StringArrayFlag{
			Name:        "Oauth2 Github Allowed Organizations",
			Flag:        "oauth2-github-allowed-orgs",
			EnvVar:      "CODER_OAUTH2_GITHUB_ALLOWED_ORGS",
			Description: "Organizations the user must be a member of to Login with GitHub.",
		},
		OAuth2GithubAllowedTeams: codersdk.StringArrayFlag{
			Name:        "Oauth2 Github Allowed Teams",
			Flag:        "oauth2-github-allowed-teams",
			EnvVar:      "CODER_OAUTH2_GITHUB_ALLOWED_TEAMS",
			Description: "Teams inside organizations the user must be a member of to Login with GitHub. Structured as: <organization-name>/<team-slug>.",
		},
		OAuth2GithubAllowSignups: codersdk.BoolFlag{
			Name:        "Oauth2 Github Allow Signups",
			Flag:        "oauth2-github-allow-signups",
			EnvVar:      "CODER_OAUTH2_GITHUB_ALLOW_SIGNUPS",
			Description: "Whether new users can sign up with GitHub.",
		},
		OAuth2GithubEnterpriseBaseURL: codersdk.StringFlag{
			Name:        "Oauth2 Github Enterprise Base URL",
			Flag:        "oauth2-github-enterprise-base-url",
			EnvVar:      "CODER_OAUTH2_GITHUB_ENTERPRISE_BASE_URL",
			Description: "Base URL of a GitHub Enterprise deployment to use for Login with GitHub.",
		},
		OIDCAllowSignups: codersdk.BoolFlag{
			Name:        "OIDC Allow Signups",
			Flag:        "oidc-allow-signups",
			EnvVar:      "CODER_OIDC_ALLOW_SIGNUPS",
			Description: "Whether new users can sign up with OIDC.",
			Default:     true,
		},
		OIDCClientID: codersdk.StringFlag{
			Name:        "OIDC Client ID",
			Flag:        "oidc-client-id",
			EnvVar:      "CODER_OIDC_CLIENT_ID",
			Description: "Client ID to use for Login with OIDC.",
		},
		OIDCClientSecret: codersdk.StringFlag{
			Name:        "OIDC Client Secret",
			Flag:        "oidc-client-secret",
			EnvVar:      "CODER_OIDC_CLIENT_SECRET",
			Description: "Client secret to use for Login with OIDC.",
			Secret:      true,
		},
		OIDCEmailDomain: codersdk.StringFlag{
			Name:        "OIDC Email Domain",
			Flag:        "oidc-email-domain",
			EnvVar:      "CODER_OIDC_EMAIL_DOMAIN",
			Description: "Email domain that clients logging in with OIDC must match.",
		},
		OIDCIssuerURL: codersdk.StringFlag{
			Name:        "OIDC Issuer URL",
			Flag:        "oidc-issuer-url",
			EnvVar:      "CODER_OIDC_ISSUER_URL",
			Description: "Issuer URL to use for Login with OIDC.",
		},
		OIDCScopes: codersdk.StringArrayFlag{
			Name:        "OIDC Scopes",
			Flag:        "oidc-scopes",
			EnvVar:      "CODER_OIDC_SCOPES",
			Description: "Scopes to grant when authenticating with OIDC.",
			Default:     []string{oidc.ScopeOpenID, "profile", "email"},
		},
		TelemetryEnable: codersdk.BoolFlag{
			Name:        "Telemetry Enabled",
			Flag:        "telemetry",
			EnvVar:      "CODER_TELEMETRY",
			Description: "Whether telemetry is enabled or not. Coder collects anonymized usage data to help improve our product.",
			Default:     flag.Lookup("test.v") == nil,
		},
		TelemetryTraceEnable: codersdk.BoolFlag{
			Name:        "Trace Telemetry Enabled",
			Flag:        "telemetry-trace",
			EnvVar:      "CODER_TELEMETRY_TRACE",
			Shorthand:   "",
			Description: "Whether Opentelemetry traces are sent to Coder. Coder collects anonymized application tracing to help improve our product. Disabling telemetry also disables this option.",
			Default:     flag.Lookup("test.v") == nil,
		},
		TelemetryURL: codersdk.StringFlag{
			Name:        "Telemetry URL",
			Flag:        "telemetry-url",
			EnvVar:      "CODER_TELEMETRY_URL",
			Description: "URL to send telemetry.",
			Default:     "https://telemetry.coder.com",
		},
		TLSEnable: codersdk.BoolFlag{
			Name:        "TLS Enabled",
			Flag:        "tls-enable",
			EnvVar:      "CODER_TLS_ENABLE",
			Description: "Whether TLS will be enabled.",
		},
		TLSCertFiles: codersdk.StringArrayFlag{
			Name:   "TLS Cert Files",
			Flag:   "tls-cert-file",
			EnvVar: "CODER_TLS_CERT_FILE",
			Description: "Path to each certificate for TLS. It requires a PEM-encoded file. " +
				"To configure the listener to use a CA certificate, concatenate the primary certificate " +
				"and the CA certificate together. The primary certificate should appear first in the combined file.",
			Default: []string{},
		},
		TLSClientCAFile: codersdk.StringFlag{
			Name:        "TLS Client CA File",
			Flag:        "tls-client-ca-file",
			EnvVar:      "CODER_TLS_CLIENT_CA_FILE",
			Description: "PEM-encoded Certificate Authority file used for checking the authenticity of client",
		},
		TLSClientAuth: codersdk.StringFlag{
			Name:   "TLS Client Auth",
			Flag:   "tls-client-auth",
			EnvVar: "CODER_TLS_CLIENT_AUTH",
			Description: `Policy the server will follow for TLS Client Authentication. ` +
				`Accepted values are "none", "request", "require-any", "verify-if-given", or "require-and-verify"`,
			Default: "request",
		},
		TLSKeyFiles: codersdk.StringArrayFlag{
			Name:        "TLS Key Files",
			Flag:        "tls-key-file",
			EnvVar:      "CODER_TLS_KEY_FILE",
			Description: "Paths to the private keys for each of the certificates. It requires a PEM-encoded file",
			Default:     []string{},
		},
		TLSMinVersion: codersdk.StringFlag{
			Name:        "TLS Min Version",
			Flag:        "tls-min-version",
			EnvVar:      "CODER_TLS_MIN_VERSION",
			Description: `Minimum supported version of TLS. Accepted values are "tls10", "tls11", "tls12" or "tls13"`,
			Default:     "tls12",
		},
		TraceEnable: codersdk.BoolFlag{
			Name:        "Trace Enabled",
			Flag:        "trace",
			EnvVar:      "CODER_TRACE",
			Description: "Whether application tracing data is collected.",
		},
		SecureAuthCookie: codersdk.BoolFlag{
			Name:        "Secure Auth Cookie",
			Flag:        "secure-auth-cookie",
			EnvVar:      "CODER_SECURE_AUTH_COOKIE",
			Description: "Controls if the 'Secure' property is set on browser session cookies",
		},
		SSHKeygenAlgorithm: codersdk.StringFlag{
			Name:   "SSH Keygen Algorithm",
			Flag:   "ssh-keygen-algorithm",
			EnvVar: "CODER_SSH_KEYGEN_ALGORITHM",
			Description: "The algorithm to use for generating ssh keys. " +
				`Accepted values are "ed25519", "ecdsa", or "rsa4096"`,
			Default: "ed25519",
		},
		AutoImportTemplates: codersdk.StringArrayFlag{
			Name:        "Auto Import Templates",
			Flag:        "auto-import-template",
			EnvVar:      "CODER_TEMPLATE_AUTOIMPORT",
			Description: "Templates to auto-import. Available auto-importable templates are: kubernetes",
			Default:     []string{},
		},
		MetricsCacheRefreshInterval: codersdk.DurationFlag{
			Name:        "Metrics Cache Refresh Interval",
			Flag:        "metrics-cache-refresh-interval",
			EnvVar:      "CODER_METRICS_CACHE_REFRESH_INTERVAL",
			Description: "How frequently metrics are refreshed",
			Default:     time.Hour,
		},
		AgentStatRefreshInterval: codersdk.DurationFlag{
			Name:        "Agent Stats Refresh Interval",
			Flag:        "agent-stats-refresh-interval",
			EnvVar:      "CODER_AGENT_STATS_REFRESH_INTERVAL",
			Description: "How frequently agent stats are recorded",
			Default:     10 * time.Minute,
		},
		Verbose: codersdk.BoolFlag{
			Name:        "Verbose Logging",
			Flag:        "verbose",
			EnvVar:      "CODER_VERBOSE",
			Shorthand:   "v",
			Description: "Enables verbose logging.",
		},
		AuditLogging: codersdk.BoolFlag{
			Name:        "Audit Logging",
			Flag:        "audit-logging",
			EnvVar:      "CODER_AUDIT_LOGGING",
			Description: "Specifies whether audit logging is enabled.",
			Default:     true,
			Enterprise:  true,
		},
		BrowserOnly: codersdk.BoolFlag{
			Name:        "Browser Only",
			Flag:        "browser-only",
			EnvVar:      "CODER_BROWSER_ONLY",
			Description: "Whether Coder only allows connections to workspaces via the browser.",
			Enterprise:  true,
		},
		SCIMAuthHeader: codersdk.StringFlag{
			Name:        "SCIM Authentication Header",
			Flag:        "scim-auth-header",
			EnvVar:      "CODER_SCIM_API_KEY",
			Description: "Enables SCIM and sets the authentication header for the built-in SCIM server. New users are automatically created with OIDC authentication.",
			Secret:      true,
			Enterprise:  true,
		},
		UserWorkspaceQuota: codersdk.IntFlag{
			Name:        "User Workspace Quota",
			Flag:        "user-workspace-quota",
			EnvVar:      "CODER_USER_WORKSPACE_QUOTA",
			Description: "Enables and sets a limit on how many workspaces each user can create.",
			Default:     0,
			Enterprise:  true,
		},
	}
}

func RemoveSensitiveValues(df codersdk.DeploymentFlags) codersdk.DeploymentFlags {
	v := reflect.ValueOf(&df).Elem()
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		fv := v.Field(i)
		if v, ok := fv.Interface().(codersdk.StringFlag); ok {
			if v.Secret && v.Value != "" {
				v.Value = secretValue
				fv.Set(reflect.ValueOf(v))
			}
		}
	}

	return df
}

func StringFlag(flagset *pflag.FlagSet, fl *codersdk.StringFlag) {
	cliflag.StringVarP(flagset,
		&fl.Value,
		fl.Flag,
		fl.Shorthand,
		fl.EnvVar,
		fl.Default,
		fl.Description,
	)
}

func BoolFlag(flagset *pflag.FlagSet, fl *codersdk.BoolFlag) {
	cliflag.BoolVarP(flagset,
		&fl.Value,
		fl.Flag,
		fl.Shorthand,
		fl.EnvVar,
		fl.Default,
		fl.Description,
	)
}

func IntFlag(flagset *pflag.FlagSet, fl *codersdk.IntFlag) {
	cliflag.IntVarP(flagset,
		&fl.Value,
		fl.Flag,
		fl.Shorthand,
		fl.EnvVar,
		fl.Default,
		fl.Description,
	)
}

func DurationFlag(flagset *pflag.FlagSet, fl *codersdk.DurationFlag) {
	cliflag.DurationVarP(flagset,
		&fl.Value,
		fl.Flag,
		fl.Shorthand,
		fl.EnvVar,
		fl.Default,
		fl.Description,
	)
}

func StringArrayFlag(flagset *pflag.FlagSet, fl *codersdk.StringArrayFlag) {
	cliflag.StringArrayVarP(flagset,
		&fl.Value,
		fl.Flag,
		fl.Shorthand,
		fl.EnvVar,
		fl.Default,
		fl.Description,
	)
}

func defaultCacheDir() string {
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

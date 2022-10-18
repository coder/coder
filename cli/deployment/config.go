package deployment

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

func newConfig() codersdk.DeploymentConfig {
	return codersdk.DeploymentConfig{
		AccessURL: codersdk.DeploymentConfigField[string]{
			Usage: "External URL to access your deployment. This must be accessible by all provisioned workspaces.",
			Flag:  "access-url",
		},
		WildcardAccessURL: codersdk.DeploymentConfigField[string]{
			Usage: "Specifies the wildcard hostname to use for workspace applications in the form \"*.example.com\".",
			Flag:  "wildcard-access-url",
		},
		Address: codersdk.DeploymentConfigField[string]{
			Usage:     "Bind address of the server.",
			Flag:      "address",
			Shorthand: "a",
			Value:     "127.0.0.1:3000",
		},
		AutobuildPollInterval: codersdk.DeploymentConfigField[time.Duration]{
			Usage:  "Interval to poll for scheduled workspace builds.",
			Flag:   "autobuild-poll-interval",
			Hidden: true,
			Value:  time.Minute,
		},
		DERPServerEnable: codersdk.DeploymentConfigField[bool]{
			Usage: "Whether to enable or disable the embedded DERP relay server.",
			Flag:  "derp-server-enable",
			Value: true,
		},
		DERPServerRegionID: codersdk.DeploymentConfigField[int]{
			Usage: "Region ID to use for the embedded DERP server.",
			Flag:  "derp-server-region-id",
			Value: 999,
		},
		DERPServerRegionCode: codersdk.DeploymentConfigField[string]{
			Usage: "Region code to use for the embedded DERP server.",
			Flag:  "derp-server-region-code",
			Value: "coder",
		},
		DERPServerRegionName: codersdk.DeploymentConfigField[string]{
			Usage: "Region name that for the embedded DERP server.",
			Flag:  "derp-server-region-name",
			Value: "Coder Embedded Relay",
		},
		DERPServerSTUNAddresses: codersdk.DeploymentConfigField[[]string]{
			Usage: "Addresses for STUN servers to establish P2P connections. Set empty to disable P2P connections.",
			Flag:  "derp-server-stun-addresses",
			Value: []string{"stun.l.google.com:19302"},
		},
		DERPServerRelayAddress: codersdk.DeploymentConfigField[string]{
			Usage:      "An HTTP address that is accessible by other replicas to relay DERP traffic. Required for high availability.",
			Flag:       "derp-server-relay-address",
			Enterprise: true,
		},
		DERPConfigURL: codersdk.DeploymentConfigField[string]{
			Usage: "URL to fetch a DERP mapping on startup. See: https://tailscale.com/kb/1118/custom-derp-servers/",
			Flag:  "derp-config-url",
		},
		DERPConfigPath: codersdk.DeploymentConfigField[string]{
			Usage: "Path to read a DERP mapping from. See: https://tailscale.com/kb/1118/custom-derp-servers/",
			Flag:  "derp-config-path",
		},
		PrometheusEnable: codersdk.DeploymentConfigField[bool]{
			Usage: "Serve prometheus metrics on the address defined by prometheus address.",
			Flag:  "prometheus-enable",
		},
		PrometheusAddress: codersdk.DeploymentConfigField[string]{
			Usage: "The bind address to serve prometheus metrics.",
			Flag:  "prometheus-address",
			Value: "127.0.0.1:2112",
		},
		PprofEnable: codersdk.DeploymentConfigField[bool]{
			Usage: "Serve pprof metrics on the address defined by pprof address.",
			Flag:  "pprof-enable",
		},
		PprofAddress: codersdk.DeploymentConfigField[string]{
			Usage: "The bind address to serve pprof.",
			Flag:  "pprof-address",
			Value: "127.0.0.1:6060",
		},
		CacheDir: codersdk.DeploymentConfigField[string]{
			Usage: "The directory to cache temporary files. If unspecified and $CACHE_DIRECTORY is set, it will be used for compatibility with systemd.",
			Flag:  "cache-dir",
			Value: defaultCacheDir(),
		},
		InMemoryDatabase: codersdk.DeploymentConfigField[bool]{
			Usage:  "Controls whether data will be stored in an in-memory database.",
			Flag:   "in-memory",
			Hidden: true,
		},
		ProvisionerDaemonCount: codersdk.DeploymentConfigField[int]{
			Usage: "Number of provisioner daemons to create on start. If builds are stuck in queued state for a long time, consider increasing this.",
			Flag:  "provisioner-daemons",
			Value: 3,
		},
		PostgresURL: codersdk.DeploymentConfigField[string]{
			Usage: "URL of a PostgreSQL database. If empty, PostgreSQL binaries will be downloaded from Maven (https://repo1.maven.org/maven2) and store all data in the config root. Access the built-in database with \"coder server postgres-builtin-url\".",
			Flag:  "postgres-url",
		},
		OAuth2GithubClientID: codersdk.DeploymentConfigField[string]{
			Usage: "Client ID for Login with GitHub.",
			Flag:  "oauth2-github-client-id",
		},
		OAuth2GithubClientSecret: codersdk.DeploymentConfigField[string]{
			Usage: "Client secret for Login with GitHub.",
			Flag:  "oauth2-github-client-secret",
		},
		OAuth2GithubAllowedOrganizations: codersdk.DeploymentConfigField[[]string]{
			Usage: "Organizations the user must be a member of to Login with GitHub.",
			Flag:  "oauth2-github-allowed-orgs",
		},
		OAuth2GithubAllowedTeams: codersdk.DeploymentConfigField[[]string]{
			Usage: "Teams inside organizations the user must be a member of to Login with GitHub. Structured as: <organization-name>/<team-slug>.",
			Flag:  "oauth2-github-allowed-teams",
		},
		OAuth2GithubAllowSignups: codersdk.DeploymentConfigField[bool]{
			Usage: "Whether new users can sign up with GitHub.",
			Flag:  "oauth2-github-allow-signups",
		},
		OAuth2GithubEnterpriseBaseURL: codersdk.DeploymentConfigField[string]{
			Usage: "Base URL of a GitHub Enterprise deployment to use for Login with GitHub.",
			Flag:  "oauth2-github-enterprise-base-url",
		},
		OIDCAllowSignups: codersdk.DeploymentConfigField[bool]{
			Usage: "Whether new users can sign up with OIDC.",
			Flag:  "oidc-allow-signups",
			Value: true,
		},
		OIDCClientID: codersdk.DeploymentConfigField[string]{
			Usage: "Client ID to use for Login with OIDC.",
			Flag:  "oidc-client-id",
		},
		OIDCClientSecret: codersdk.DeploymentConfigField[string]{
			Usage: "Client secret to use for Login with OIDC.",
			Flag:  "oidc-client-secret",
		},
		OIDCEmailDomain: codersdk.DeploymentConfigField[string]{
			Usage: "Email domain that clients logging in with OIDC must match.",
			Flag:  "oidc-email-domain",
		},
		OIDCIssuerURL: codersdk.DeploymentConfigField[string]{
			Usage: "Issuer URL to use for Login with OIDC.",
			Flag:  "oidc-issuer-url",
		},
		OIDCScopes: codersdk.DeploymentConfigField[[]string]{
			Usage: "Scopes to grant when authenticating with OIDC.",
			Flag:  "oidc-scopes",
			Value: []string{oidc.ScopeOpenID, "profile", "email"},
		},
		TelemetryEnable: codersdk.DeploymentConfigField[bool]{
			Usage: "Whether telemetry is enabled or not. Coder collects anonymized usage data to help improve our product.",
			Flag:  "telemetry",
			Value: flag.Lookup("test.v") == nil,
		},
		TelemetryTraceEnable: codersdk.DeploymentConfigField[bool]{
			Usage: "Whether Opentelemetry traces are sent to Coder. Coder collects anonymized application tracing to help improve our product. Disabling telemetry also disables this option.",
			Flag:  "telemetry-trace",
			Value: flag.Lookup("test.v") == nil,
		},
		TelemetryURL: codersdk.DeploymentConfigField[string]{
			Usage:  "URL to send telemetry.",
			Flag:   "telemetry-url",
			Hidden: true,
			Value:  "https://telemetry.coder.com",
		},
		TLSEnable: codersdk.DeploymentConfigField[bool]{
			Usage: "Whether TLS will be enabled.",
			Flag:  "tls-enable",
		},
		TLSCertFiles: codersdk.DeploymentConfigField[[]string]{
			Usage: "Path to each certificate for TLS. It requires a PEM-encoded file. To configure the listener to use a CA certificate, concatenate the primary certificate and the CA certificate together. The primary certificate should appear first in the combined file.",
			Flag:  "tls-cert-file",
		},
		TLSClientCAFile: codersdk.DeploymentConfigField[string]{
			Usage: "PEM-encoded Certificate Authority file used for checking the authenticity of client",
			Flag:  "tls-client-ca-file",
		},
		TLSClientAuth: codersdk.DeploymentConfigField[string]{
			Usage: "Policy the server will follow for TLS Client Authentication. Accepted values are \"none\", \"request\", \"require-any\", \"verify-if-given\", or \"require-and-verify\".",
			Flag:  "tls-client-auth",
			Value: "request",
		},
		TLSKeyFiles: codersdk.DeploymentConfigField[[]string]{
			Usage: "Paths to the private keys for each of the certificates. It requires a PEM-encoded file.",
			Flag:  "tls-key-file",
		},
		TLSMinVersion: codersdk.DeploymentConfigField[string]{
			Usage: "Minimum supported version of TLS. Accepted values are \"tls10\", \"tls11\", \"tls12\" or \"tls13\"",
			Flag:  "tls-min-version",
			Value: "tls12",
		},
		TraceEnable: codersdk.DeploymentConfigField[bool]{
			Usage: "Whether application tracing data is collected.",
			Flag:  "trace",
		},
		SecureAuthCookie: codersdk.DeploymentConfigField[bool]{
			Usage: "Controls if the 'Secure' property is set on browser session cookies.",
			Flag:  "secure-auth-cookie",
		},
		SSHKeygenAlgorithm: codersdk.DeploymentConfigField[string]{
			Usage: "The algorithm to use for generating ssh keys. Accepted values are \"ed25519\", \"ecdsa\", or \"rsa4096\".",
			Flag:  "ssh-keygen-algorithm",
			Value: "ed25519",
		},
		AutoImportTemplates: codersdk.DeploymentConfigField[[]string]{
			Usage:  "Templates to auto-import. Available auto-importable templates are: kubernetes",
			Flag:   "auto-import-template",
			Hidden: true,
		},
		MetricsCacheRefreshInterval: codersdk.DeploymentConfigField[time.Duration]{
			Usage:  "How frequently metrics are refreshed",
			Flag:   "metrics-cache-refresh-interval",
			Hidden: true,
			Value:  time.Hour,
		},
		AgentStatRefreshInterval: codersdk.DeploymentConfigField[time.Duration]{
			Usage:  "How frequently agent stats are recorded",
			Flag:   "agent-stats-refresh-interval",
			Hidden: true,
			Value:  10 * time.Minute,
		},
		Verbose: codersdk.DeploymentConfigField[bool]{
			Usage:     "Enables verbose logging.",
			Flag:      "verbose",
			Shorthand: "v",
		},
		AuditLogging: codersdk.DeploymentConfigField[bool]{
			Usage:      "Specifies whether audit logging is enabled.",
			Flag:       "audit-logging",
			Value:      true,
			Enterprise: true,
		},
		BrowserOnly: codersdk.DeploymentConfigField[bool]{
			Usage:      "Whether Coder only allows connections to workspaces via the browser.",
			Flag:       "browser-only",
			Enterprise: true,
		},
		SCIMAuthHeader: codersdk.DeploymentConfigField[string]{
			Usage:      "Enables SCIM and sets the authentication header for the built-in SCIM server. New users are automatically created with OIDC authentication.",
			Flag:       "scim-auth-header",
			Enterprise: true,
		},
		UserWorkspaceQuota: codersdk.DeploymentConfigField[int]{
			Usage:      "Enables and sets a limit on how many workspaces each user can create.",
			Flag:       "user-workspace-quota",
			Enterprise: true,
		},
	}
}

func Config(vip *viper.Viper) (codersdk.DeploymentConfig, error) {
	cfg := codersdk.DeploymentConfig{}
	return cfg, vip.Unmarshal(&cfg)
}

func NewViper() *viper.Viper {
	dc := newConfig()
	v := viper.New()
	v.SetEnvPrefix("coder")
	v.AutomaticEnv()

	dcv := reflect.ValueOf(dc).Elem()
	t := dcv.Type()
	for i := 0; i < t.NumField(); i++ {
		fv := dcv.Field(i)
		fve := fv.Elem()
		key := fve.FieldByName("Key").String()
		value := fve.FieldByName("Value").Interface()
		v.SetDefault(key, value)
	}

	return v
}

//nolint:revive
func AttachFlags(flagset *pflag.FlagSet, vip *viper.Viper, enterprise bool) {
	dc := newConfig()
	dcv := reflect.ValueOf(dc).Elem()
	t := dcv.Type()
	for i := 0; i < t.NumField(); i++ {
		fv := dcv.Field(i)
		fve := fv.Elem()
		isEnt := fve.FieldByName("Enterprise").Bool()
		if enterprise != isEnt {
			continue
		}
		key := fve.FieldByName("Key").String()
		flg := fve.FieldByName("Flag").String()
		if flg == "" {
			continue
		}
		usage := fve.FieldByName("Usage").String()
		usage = fmt.Sprintf("%s\n%s", usage, cliui.Styles.Placeholder.Render("Consumes $"+formatEnv(key)))
		shorthand := fve.FieldByName("Shorthand").String()
		hidden := fve.FieldByName("Hidden").Bool()
		value := fve.FieldByName("Value").Interface()

		switch value.(type) {
		case string:
			_ = flagset.StringP(flg, shorthand, vip.GetString(key), usage)
		case bool:
			_ = flagset.BoolP(flg, shorthand, vip.GetBool(key), usage)
		case int:
			_ = flagset.IntP(flg, shorthand, vip.GetInt(key), usage)
		case time.Duration:
			_ = flagset.DurationP(flg, shorthand, vip.GetDuration(key), usage)
		case []string:
			_ = flagset.StringSliceP(flg, shorthand, vip.GetStringSlice(key), usage)
		default:
			continue
		}

		_ = vip.BindPFlag(key, flagset.Lookup(flg))
		if hidden {
			_ = flagset.MarkHidden(flg)
		}
	}
}

func formatEnv(key string) string {
	return "CODER_" + strings.ToUpper(strings.NewReplacer("-", "_", ".", "_").Replace(key))
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

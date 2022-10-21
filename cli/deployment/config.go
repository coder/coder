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
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/cli/config"
	"github.com/coder/coder/codersdk"
)

func newConfig() codersdk.DeploymentConfig {
	return codersdk.DeploymentConfig{
		AccessURL: codersdk.DeploymentConfigField[string]{
			Key:   "access_url",
			Usage: "External URL to access your deployment. This must be accessible by all provisioned workspaces.",
			Flag:  "access-url",
		},
		WildcardAccessURL: codersdk.DeploymentConfigField[string]{
			Key:   "wildcard_access_url",
			Usage: "Specifies the wildcard hostname to use for workspace applications in the form \"*.example.com\".",
			Flag:  "wildcard-access-url",
		},
		Address: codersdk.DeploymentConfigField[string]{
			Key:       "address",
			Usage:     "Bind address of the server.",
			Flag:      "address",
			Shorthand: "a",
			Value:     "127.0.0.1:3000",
		},
		AutobuildPollInterval: codersdk.DeploymentConfigField[time.Duration]{
			Key:    "autobuild_poll_interval",
			Usage:  "Interval to poll for scheduled workspace builds.",
			Flag:   "autobuild-poll-interval",
			Hidden: true,
			Value:  time.Minute,
		},
		DERPServerEnable: codersdk.DeploymentConfigField[bool]{
			Key:   "derp.server.enable",
			Usage: "Whether to enable or disable the embedded DERP relay server.",
			Flag:  "derp-server-enable",
			Value: true,
		},
		DERPServerRegionID: codersdk.DeploymentConfigField[int]{
			Key:   "derp.server.region_id",
			Usage: "Region ID to use for the embedded DERP server.",
			Flag:  "derp-server-region-id",
			Value: 999,
		},
		DERPServerRegionCode: codersdk.DeploymentConfigField[string]{
			Key:   "derp.server.region_code",
			Usage: "Region code to use for the embedded DERP server.",
			Flag:  "derp-server-region-code",
			Value: "coder",
		},
		DERPServerRegionName: codersdk.DeploymentConfigField[string]{
			Key:   "derp.server.region_name",
			Usage: "Region name that for the embedded DERP server.",
			Flag:  "derp-server-region-name",
			Value: "Coder Embedded Relay",
		},
		DERPServerSTUNAddresses: codersdk.DeploymentConfigField[[]string]{
			Key:   "derp.server.stun_addresses",
			Usage: "Addresses for STUN servers to establish P2P connections. Set empty to disable P2P connections.",
			Flag:  "derp-server-stun-addresses",
			Value: []string{"stun.l.google.com:19302"},
		},
		DERPServerRelayURL: codersdk.DeploymentConfigField[string]{
			Key:        "derp.server.relay_url",
			Usage:      "An HTTP URL that is accessible by other replicas to relay DERP traffic. Required for high availability.",
			Flag:       "derp-server-relay-url",
			Enterprise: true,
		},
		DERPConfigURL: codersdk.DeploymentConfigField[string]{
			Key:   "derp.config.url",
			Usage: "URL to fetch a DERP mapping on startup. See: https://tailscale.com/kb/1118/custom-derp-servers/",
			Flag:  "derp-config-url",
		},
		DERPConfigPath: codersdk.DeploymentConfigField[string]{
			Key:   "derp.config.path",
			Usage: "Path to read a DERP mapping from. See: https://tailscale.com/kb/1118/custom-derp-servers/",
			Flag:  "derp-config-path",
		},
		PrometheusEnable: codersdk.DeploymentConfigField[bool]{
			Key:   "prometheus.enable",
			Usage: "Serve prometheus metrics on the address defined by prometheus address.",
			Flag:  "prometheus-enable",
		},
		PrometheusAddress: codersdk.DeploymentConfigField[string]{
			Key:   "prometheus.address",
			Usage: "The bind address to serve prometheus metrics.",
			Flag:  "prometheus-address",
			Value: "127.0.0.1:2112",
		},
		PprofEnable: codersdk.DeploymentConfigField[bool]{
			Key:   "pprof.enable",
			Usage: "Serve pprof metrics on the address defined by pprof address.",
			Flag:  "pprof-enable",
		},
		PprofAddress: codersdk.DeploymentConfigField[string]{
			Key:   "pprof.address",
			Usage: "The bind address to serve pprof.",
			Flag:  "pprof-address",
			Value: "127.0.0.1:6060",
		},
		CacheDirectory: codersdk.DeploymentConfigField[string]{
			Key:   "cache_directory",
			Usage: "The directory to cache temporary files. If unspecified and $CACHE_DIRECTORY is set, it will be used for compatibility with systemd.",
			Flag:  "cache-dir",
			Value: defaultCacheDir(),
		},
		InMemoryDatabase: codersdk.DeploymentConfigField[bool]{
			Key:    "in_memory_database",
			Usage:  "Controls whether data will be stored in an in-memory database.",
			Flag:   "in-memory",
			Hidden: true,
		},
		ProvisionerDaemons: codersdk.DeploymentConfigField[int]{
			Key:   "provisioner.daemons",
			Usage: "Number of provisioner daemons to create on start. If builds are stuck in queued state for a long time, consider increasing this.",
			Flag:  "provisioner-daemons",
			Value: 3,
		},
		PostgresURL: codersdk.DeploymentConfigField[string]{
			Key:   "pg_connection_url",
			Usage: "URL of a PostgreSQL database. If empty, PostgreSQL binaries will be downloaded from Maven (https://repo1.maven.org/maven2) and store all data in the config root. Access the built-in database with \"coder server postgres-builtin-url\".",
			Flag:  "postgres-url",
		},
		OAuth2GithubClientID: codersdk.DeploymentConfigField[string]{
			Key:   "oauth2.github.client_id",
			Usage: "Client ID for Login with GitHub.",
			Flag:  "oauth2-github-client-id",
		},
		OAuth2GithubClientSecret: codersdk.DeploymentConfigField[string]{
			Key:   "oauth2.github.client_secret",
			Usage: "Client secret for Login with GitHub.",
			Flag:  "oauth2-github-client-secret",
		},
		OAuth2GithubAllowedOrgs: codersdk.DeploymentConfigField[[]string]{
			Key:   "oauth2.github.allowed_orgs",
			Usage: "Organizations the user must be a member of to Login with GitHub.",
			Flag:  "oauth2-github-allowed-orgs",
		},
		OAuth2GithubAllowedTeams: codersdk.DeploymentConfigField[[]string]{
			Key:   "oauth2.github.allowed_teams",
			Usage: "Teams inside organizations the user must be a member of to Login with GitHub. Structured as: <organization-name>/<team-slug>.",
			Flag:  "oauth2-github-allowed-teams",
		},
		OAuth2GithubAllowSignups: codersdk.DeploymentConfigField[bool]{
			Key:   "oauth2.github.allow_signups",
			Usage: "Whether new users can sign up with GitHub.",
			Flag:  "oauth2-github-allow-signups",
		},
		OAuth2GithubEnterpriseBaseURL: codersdk.DeploymentConfigField[string]{
			Key:   "oauth2.github.enterprise_base_url",
			Usage: "Base URL of a GitHub Enterprise deployment to use for Login with GitHub.",
			Flag:  "oauth2-github-enterprise-base-url",
		},
		OIDCAllowSignups: codersdk.DeploymentConfigField[bool]{
			Key:   "oidc.allow_signups",
			Usage: "Whether new users can sign up with OIDC.",
			Flag:  "oidc-allow-signups",
			Value: true,
		},
		OIDCClientID: codersdk.DeploymentConfigField[string]{
			Key:   "oidc.client_id",
			Usage: "Client ID to use for Login with OIDC.",
			Flag:  "oidc-client-id",
		},
		OIDCClientSecret: codersdk.DeploymentConfigField[string]{
			Key:   "oidc.client_secret",
			Usage: "Client secret to use for Login with OIDC.",
			Flag:  "oidc-client-secret",
		},
		OIDCEmailDomain: codersdk.DeploymentConfigField[string]{
			Key:   "oidc.email_domain",
			Usage: "Email domain that clients logging in with OIDC must match.",
			Flag:  "oidc-email-domain",
		},
		OIDCIssuerURL: codersdk.DeploymentConfigField[string]{
			Key:   "oidc.issuer_url",
			Usage: "Issuer URL to use for Login with OIDC.",
			Flag:  "oidc-issuer-url",
		},
		OIDCScopes: codersdk.DeploymentConfigField[[]string]{
			Key:   "oidc.scopes",
			Usage: "Scopes to grant when authenticating with OIDC.",
			Flag:  "oidc-scopes",
			Value: []string{oidc.ScopeOpenID, "profile", "email"},
		},
		TelemetryEnable: codersdk.DeploymentConfigField[bool]{
			Key:   "telemetry.enable",
			Usage: "Whether telemetry is enabled or not. Coder collects anonymized usage data to help improve our product.",
			Flag:  "telemetry",
			Value: flag.Lookup("test.v") == nil,
		},
		TelemetryTrace: codersdk.DeploymentConfigField[bool]{
			Key:   "telemetry.trace",
			Usage: "Whether Opentelemetry traces are sent to Coder. Coder collects anonymized application tracing to help improve our product. Disabling telemetry also disables this option.",
			Flag:  "telemetry-trace",
			Value: flag.Lookup("test.v") == nil,
		},
		TelemetryURL: codersdk.DeploymentConfigField[string]{
			Key:    "telemetry.url",
			Usage:  "URL to send telemetry.",
			Flag:   "telemetry-url",
			Hidden: true,
			Value:  "https://telemetry.coder.com",
		},
		TLSEnable: codersdk.DeploymentConfigField[bool]{
			Key:   "tls.enable",
			Usage: "Whether TLS will be enabled.",
			Flag:  "tls-enable",
		},
		TLSCertFiles: codersdk.DeploymentConfigField[[]string]{
			Key:   "tls.cert_file",
			Usage: "Path to each certificate for TLS. It requires a PEM-encoded file. To configure the listener to use a CA certificate, concatenate the primary certificate and the CA certificate together. The primary certificate should appear first in the combined file.",
			Flag:  "tls-cert-file",
		},
		TLSClientCAFile: codersdk.DeploymentConfigField[string]{
			Key:   "tls.client_ca_file",
			Usage: "PEM-encoded Certificate Authority file used for checking the authenticity of client",
			Flag:  "tls-client-ca-file",
		},
		TLSClientAuth: codersdk.DeploymentConfigField[string]{
			Key:   "tls.client_auth",
			Usage: "Policy the server will follow for TLS Client Authentication. Accepted values are \"none\", \"request\", \"require-any\", \"verify-if-given\", or \"require-and-verify\".",
			Flag:  "tls-client-auth",
			Value: "request",
		},
		TLSKeyFiles: codersdk.DeploymentConfigField[[]string]{
			Key:   "tls.key_file",
			Usage: "Paths to the private keys for each of the certificates. It requires a PEM-encoded file.",
			Flag:  "tls-key-file",
		},
		TLSMinVersion: codersdk.DeploymentConfigField[string]{
			Key:   "tls.min_version",
			Usage: "Minimum supported version of TLS. Accepted values are \"tls10\", \"tls11\", \"tls12\" or \"tls13\"",
			Flag:  "tls-min-version",
			Value: "tls12",
		},
		TraceEnable: codersdk.DeploymentConfigField[bool]{
			Key:   "trace",
			Usage: "Whether application tracing data is collected.",
			Flag:  "trace",
		},
		SecureAuthCookie: codersdk.DeploymentConfigField[bool]{
			Key:   "secure_auth_cookie",
			Usage: "Controls if the 'Secure' property is set on browser session cookies.",
			Flag:  "secure-auth-cookie",
		},
		SSHKeygenAlgorithm: codersdk.DeploymentConfigField[string]{
			Key:   "ssh_keygen_algorithm",
			Usage: "The algorithm to use for generating ssh keys. Accepted values are \"ed25519\", \"ecdsa\", or \"rsa4096\".",
			Flag:  "ssh-keygen-algorithm",
			Value: "ed25519",
		},
		AutoImportTemplates: codersdk.DeploymentConfigField[[]string]{
			Key:    "auto_import_templates",
			Usage:  "Templates to auto-import. Available auto-importable templates are: kubernetes",
			Flag:   "auto-import-template",
			Hidden: true,
		},
		MetricsCacheRefreshInterval: codersdk.DeploymentConfigField[time.Duration]{
			Key:    "metrics_cache_refresh_interval",
			Usage:  "How frequently metrics are refreshed",
			Flag:   "metrics-cache-refresh-interval",
			Hidden: true,
			Value:  time.Hour,
		},
		AgentStatRefreshInterval: codersdk.DeploymentConfigField[time.Duration]{
			Key:    "agent_stat_refresh_interval",
			Usage:  "How frequently agent stats are recorded",
			Flag:   "agent-stats-refresh-interval",
			Hidden: true,
			Value:  10 * time.Minute,
		},
		AuditLogging: codersdk.DeploymentConfigField[bool]{
			Key:        "audit_logging",
			Usage:      "Specifies whether audit logging is enabled.",
			Flag:       "audit-logging",
			Value:      true,
			Enterprise: true,
		},
		BrowserOnly: codersdk.DeploymentConfigField[bool]{
			Key:        "browser_only",
			Usage:      "Whether Coder only allows connections to workspaces via the browser.",
			Flag:       "browser-only",
			Enterprise: true,
		},
		SCIMAPIKey: codersdk.DeploymentConfigField[string]{
			Key:        "scim_api_key",
			Usage:      "Enables SCIM and sets the authentication header for the built-in SCIM server. New users are automatically created with OIDC authentication.",
			Flag:       "scim-auth-header",
			Enterprise: true,
		},
		UserWorkspaceQuota: codersdk.DeploymentConfigField[int]{
			Key:        "user_workspace_quota",
			Usage:      "Enables and sets a limit on how many workspaces each user can create.",
			Flag:       "user-workspace-quota",
			Enterprise: true,
		},
	}
}

//nolint:revive
func Config(flagset *pflag.FlagSet, vip *viper.Viper) (codersdk.DeploymentConfig, error) {
	dc := newConfig()
	flg, err := flagset.GetString(config.FlagName)
	if err != nil {
		return dc, xerrors.Errorf("get global config from flag: %w", err)
	}
	vip.SetEnvPrefix("coder")
	vip.AutomaticEnv()

	if flg != "" {
		vip.SetConfigFile(flg + "/server.yaml")
		err = vip.ReadInConfig()
		if err != nil && !xerrors.Is(err, os.ErrNotExist) {
			return dc, xerrors.Errorf("reading deployment config: %w", err)
		}
	}

	dcv := reflect.ValueOf(&dc).Elem()
	t := dcv.Type()
	for i := 0; i < t.NumField(); i++ {
		fve := dcv.Field(i)
		key := fve.FieldByName("Key").String()
		value := fve.FieldByName("Value").Interface()

		switch value.(type) {
		case string:
			fve.FieldByName("Value").SetString(vip.GetString(key))
		case bool:
			fve.FieldByName("Value").SetBool(vip.GetBool(key))
		case int:
			fve.FieldByName("Value").SetInt(int64(vip.GetInt(key)))
		case time.Duration:
			fve.FieldByName("Value").SetInt(int64(vip.GetDuration(key)))
		case []string:
			// As of October 21st, 2022 we supported delimiting a string
			// with a comma, but Viper only supports with a space. This
			// is a small hack around it!
			rawSlice := reflect.ValueOf(vip.GetStringSlice(key)).Interface()
			slice, ok := rawSlice.([]string)
			if !ok {
				return dc, xerrors.Errorf("string slice is of type %T", rawSlice)
			}
			value := make([]string, 0, len(slice))
			for _, entry := range slice {
				value = append(value, strings.Split(entry, ",")...)
			}
			fve.FieldByName("Value").Set(reflect.ValueOf(value))
		default:
			return dc, xerrors.Errorf("unsupported type %T", value)
		}
	}

	return dc, nil
}

func NewViper() *viper.Viper {
	dc := newConfig()
	v := viper.New()
	v.SetEnvPrefix("coder")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))

	dcv := reflect.ValueOf(dc)
	t := dcv.Type()
	for i := 0; i < t.NumField(); i++ {
		fv := dcv.Field(i)
		key := fv.FieldByName("Key").String()
		value := fv.FieldByName("Value").Interface()
		v.SetDefault(key, value)
	}

	return v
}

//nolint:revive
func AttachFlags(flagset *pflag.FlagSet, vip *viper.Viper, enterprise bool) {
	dc := newConfig()
	dcv := reflect.ValueOf(dc)
	t := dcv.Type()
	for i := 0; i < t.NumField(); i++ {
		fv := dcv.Field(i)
		isEnt := fv.FieldByName("Enterprise").Bool()
		if enterprise != isEnt {
			continue
		}
		key := fv.FieldByName("Key").String()
		flg := fv.FieldByName("Flag").String()
		if flg == "" {
			continue
		}
		usage := fv.FieldByName("Usage").String()
		usage = fmt.Sprintf("%s\n%s", usage, cliui.Styles.Placeholder.Render("Consumes $"+formatEnv(key)))
		shorthand := fv.FieldByName("Shorthand").String()
		hidden := fv.FieldByName("Hidden").Bool()
		value := fv.FieldByName("Value").Interface()

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

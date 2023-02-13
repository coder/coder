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

	"github.com/coder/coder/buildinfo"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/cli/config"
	"github.com/coder/coder/codersdk"
)

func newConfig() *codersdk.DeploymentConfig {
	return &codersdk.DeploymentConfig{
		AccessURL: &codersdk.DeploymentConfigField[string]{
			Name:  "Access URL",
			Usage: "External URL to access your deployment. This must be accessible by all provisioned workspaces.",
			Flag:  "access-url",
		},
		WildcardAccessURL: &codersdk.DeploymentConfigField[string]{
			Name:  "Wildcard Access URL",
			Usage: "Specifies the wildcard hostname to use for workspace applications in the form \"*.example.com\".",
			Flag:  "wildcard-access-url",
		},
		RedirectToAccessURL: &codersdk.DeploymentConfigField[bool]{
			Name:  "Redirect to Access URL",
			Usage: "Specifies whether to redirect requests that do not match the access URL host.",
			Flag:  "redirect-to-access-url",
		},
		// DEPRECATED: Use HTTPAddress or TLS.Address instead.
		Address: &codersdk.DeploymentConfigField[string]{
			Name:      "Address",
			Usage:     "Bind address of the server.",
			Flag:      "address",
			Shorthand: "a",
			// Deprecated, so we don't have a default. If set, it will overwrite
			// HTTPAddress and TLS.Address and print a warning.
			Hidden:  true,
			Default: "",
		},
		HTTPAddress: &codersdk.DeploymentConfigField[string]{
			Name:    "Address",
			Usage:   "HTTP bind address of the server. Unset to disable the HTTP endpoint.",
			Flag:    "http-address",
			Default: "127.0.0.1:3000",
		},
		AutobuildPollInterval: &codersdk.DeploymentConfigField[time.Duration]{
			Name:    "Autobuild Poll Interval",
			Usage:   "Interval to poll for scheduled workspace builds.",
			Flag:    "autobuild-poll-interval",
			Hidden:  true,
			Default: time.Minute,
		},
		DERP: &codersdk.DERP{
			Server: &codersdk.DERPServerConfig{
				Enable: &codersdk.DeploymentConfigField[bool]{
					Name:    "DERP Server Enable",
					Usage:   "Whether to enable or disable the embedded DERP relay server.",
					Flag:    "derp-server-enable",
					Default: true,
				},
				RegionID: &codersdk.DeploymentConfigField[int]{
					Name:    "DERP Server Region ID",
					Usage:   "Region ID to use for the embedded DERP server.",
					Flag:    "derp-server-region-id",
					Default: 999,
				},
				RegionCode: &codersdk.DeploymentConfigField[string]{
					Name:    "DERP Server Region Code",
					Usage:   "Region code to use for the embedded DERP server.",
					Flag:    "derp-server-region-code",
					Default: "coder",
				},
				RegionName: &codersdk.DeploymentConfigField[string]{
					Name:    "DERP Server Region Name",
					Usage:   "Region name that for the embedded DERP server.",
					Flag:    "derp-server-region-name",
					Default: "Coder Embedded Relay",
				},
				STUNAddresses: &codersdk.DeploymentConfigField[[]string]{
					Name:    "DERP Server STUN Addresses",
					Usage:   "Addresses for STUN servers to establish P2P connections. Set empty to disable P2P connections.",
					Flag:    "derp-server-stun-addresses",
					Default: []string{"stun.l.google.com:19302"},
				},
				RelayURL: &codersdk.DeploymentConfigField[string]{
					Name:       "DERP Server Relay URL",
					Usage:      "An HTTP URL that is accessible by other replicas to relay DERP traffic. Required for high availability.",
					Flag:       "derp-server-relay-url",
					Enterprise: true,
				},
			},
			Config: &codersdk.DERPConfig{
				URL: &codersdk.DeploymentConfigField[string]{
					Name:  "DERP Config URL",
					Usage: "URL to fetch a DERP mapping on startup. See: https://tailscale.com/kb/1118/custom-derp-servers/",
					Flag:  "derp-config-url",
				},
				Path: &codersdk.DeploymentConfigField[string]{
					Name:  "DERP Config Path",
					Usage: "Path to read a DERP mapping from. See: https://tailscale.com/kb/1118/custom-derp-servers/",
					Flag:  "derp-config-path",
				},
			},
		},
		GitAuth: &codersdk.DeploymentConfigField[[]codersdk.GitAuthConfig]{
			Name:    "Git Auth",
			Usage:   "Automatically authenticate Git inside workspaces.",
			Flag:    "gitauth",
			Default: []codersdk.GitAuthConfig{},
		},
		Prometheus: &codersdk.PrometheusConfig{
			Enable: &codersdk.DeploymentConfigField[bool]{
				Name:  "Prometheus Enable",
				Usage: "Serve prometheus metrics on the address defined by prometheus address.",
				Flag:  "prometheus-enable",
			},
			Address: &codersdk.DeploymentConfigField[string]{
				Name:    "Prometheus Address",
				Usage:   "The bind address to serve prometheus metrics.",
				Flag:    "prometheus-address",
				Default: "127.0.0.1:2112",
			},
		},
		Pprof: &codersdk.PprofConfig{
			Enable: &codersdk.DeploymentConfigField[bool]{
				Name:  "Pprof Enable",
				Usage: "Serve pprof metrics on the address defined by pprof address.",
				Flag:  "pprof-enable",
			},
			Address: &codersdk.DeploymentConfigField[string]{
				Name:    "Pprof Address",
				Usage:   "The bind address to serve pprof.",
				Flag:    "pprof-address",
				Default: "127.0.0.1:6060",
			},
		},
		ProxyTrustedHeaders: &codersdk.DeploymentConfigField[[]string]{
			Name:  "Proxy Trusted Headers",
			Flag:  "proxy-trusted-headers",
			Usage: "Headers to trust for forwarding IP addresses. e.g. Cf-Connecting-Ip, True-Client-Ip, X-Forwarded-For",
		},
		ProxyTrustedOrigins: &codersdk.DeploymentConfigField[[]string]{
			Name:  "Proxy Trusted Origins",
			Flag:  "proxy-trusted-origins",
			Usage: "Origin addresses to respect \"proxy-trusted-headers\". e.g. 192.168.1.0/24",
		},
		CacheDirectory: &codersdk.DeploymentConfigField[string]{
			Name:    "Cache Directory",
			Usage:   "The directory to cache temporary files. If unspecified and $CACHE_DIRECTORY is set, it will be used for compatibility with systemd.",
			Flag:    "cache-dir",
			Default: DefaultCacheDir(),
		},
		InMemoryDatabase: &codersdk.DeploymentConfigField[bool]{
			Name:   "In Memory Database",
			Usage:  "Controls whether data will be stored in an in-memory database.",
			Flag:   "in-memory",
			Hidden: true,
		},
		PostgresURL: &codersdk.DeploymentConfigField[string]{
			Name:   "Postgres Connection URL",
			Usage:  "URL of a PostgreSQL database. If empty, PostgreSQL binaries will be downloaded from Maven (https://repo1.maven.org/maven2) and store all data in the config root. Access the built-in database with \"coder server postgres-builtin-url\".",
			Flag:   "postgres-url",
			Secret: true,
		},
		OAuth2: &codersdk.OAuth2Config{
			Github: &codersdk.OAuth2GithubConfig{
				ClientID: &codersdk.DeploymentConfigField[string]{
					Name:  "OAuth2 GitHub Client ID",
					Usage: "Client ID for Login with GitHub.",
					Flag:  "oauth2-github-client-id",
				},
				ClientSecret: &codersdk.DeploymentConfigField[string]{
					Name:   "OAuth2 GitHub Client Secret",
					Usage:  "Client secret for Login with GitHub.",
					Flag:   "oauth2-github-client-secret",
					Secret: true,
				},
				AllowedOrgs: &codersdk.DeploymentConfigField[[]string]{
					Name:  "OAuth2 GitHub Allowed Orgs",
					Usage: "Organizations the user must be a member of to Login with GitHub.",
					Flag:  "oauth2-github-allowed-orgs",
				},
				AllowedTeams: &codersdk.DeploymentConfigField[[]string]{
					Name:  "OAuth2 GitHub Allowed Teams",
					Usage: "Teams inside organizations the user must be a member of to Login with GitHub. Structured as: <organization-name>/<team-slug>.",
					Flag:  "oauth2-github-allowed-teams",
				},
				AllowSignups: &codersdk.DeploymentConfigField[bool]{
					Name:  "OAuth2 GitHub Allow Signups",
					Usage: "Whether new users can sign up with GitHub.",
					Flag:  "oauth2-github-allow-signups",
				},
				AllowEveryone: &codersdk.DeploymentConfigField[bool]{
					Name:  "OAuth2 GitHub Allow Everyone",
					Usage: "Allow all logins, setting this option means allowed orgs and teams must be empty.",
					Flag:  "oauth2-github-allow-everyone",
				},
				EnterpriseBaseURL: &codersdk.DeploymentConfigField[string]{
					Name:  "OAuth2 GitHub Enterprise Base URL",
					Usage: "Base URL of a GitHub Enterprise deployment to use for Login with GitHub.",
					Flag:  "oauth2-github-enterprise-base-url",
				},
			},
		},
		OIDC: &codersdk.OIDCConfig{
			AllowSignups: &codersdk.DeploymentConfigField[bool]{
				Name:    "OIDC Allow Signups",
				Usage:   "Whether new users can sign up with OIDC.",
				Flag:    "oidc-allow-signups",
				Default: true,
			},
			ClientID: &codersdk.DeploymentConfigField[string]{
				Name:  "OIDC Client ID",
				Usage: "Client ID to use for Login with OIDC.",
				Flag:  "oidc-client-id",
			},
			ClientSecret: &codersdk.DeploymentConfigField[string]{
				Name:   "OIDC Client Secret",
				Usage:  "Client secret to use for Login with OIDC.",
				Flag:   "oidc-client-secret",
				Secret: true,
			},
			EmailDomain: &codersdk.DeploymentConfigField[[]string]{
				Name:  "OIDC Email Domain",
				Usage: "Email domains that clients logging in with OIDC must match.",
				Flag:  "oidc-email-domain",
			},
			IssuerURL: &codersdk.DeploymentConfigField[string]{
				Name:  "OIDC Issuer URL",
				Usage: "Issuer URL to use for Login with OIDC.",
				Flag:  "oidc-issuer-url",
			},
			Scopes: &codersdk.DeploymentConfigField[[]string]{
				Name:    "OIDC Scopes",
				Usage:   "Scopes to grant when authenticating with OIDC.",
				Flag:    "oidc-scopes",
				Default: []string{oidc.ScopeOpenID, "profile", "email"},
			},
			IgnoreEmailVerified: &codersdk.DeploymentConfigField[bool]{
				Name:    "OIDC Ignore Email Verified",
				Usage:   "Ignore the email_verified claim from the upstream provider.",
				Flag:    "oidc-ignore-email-verified",
				Default: false,
			},
			UsernameField: &codersdk.DeploymentConfigField[string]{
				Name:    "OIDC Username Field",
				Usage:   "OIDC claim field to use as the username.",
				Flag:    "oidc-username-field",
				Default: "preferred_username",
			},
			SignInText: &codersdk.DeploymentConfigField[string]{
				Name:    "OpenID Connect sign in text",
				Usage:   "The text to show on the OpenID Connect sign in button",
				Flag:    "oidc-sign-in-text",
				Default: "OpenID Connect",
			},
			IconURL: &codersdk.DeploymentConfigField[string]{
				Name:  "OpenID connect icon URL",
				Usage: "URL pointing to the icon to use on the OepnID Connect login button",
				Flag:  "oidc-icon-url",
			},
		},

		Telemetry: &codersdk.TelemetryConfig{
			Enable: &codersdk.DeploymentConfigField[bool]{
				Name:    "Telemetry Enable",
				Usage:   "Whether telemetry is enabled or not. Coder collects anonymized usage data to help improve our product.",
				Flag:    "telemetry",
				Default: flag.Lookup("test.v") == nil,
			},
			Trace: &codersdk.DeploymentConfigField[bool]{
				Name:    "Telemetry Trace",
				Usage:   "Whether Opentelemetry traces are sent to Coder. Coder collects anonymized application tracing to help improve our product. Disabling telemetry also disables this option.",
				Flag:    "telemetry-trace",
				Default: flag.Lookup("test.v") == nil,
			},
			URL: &codersdk.DeploymentConfigField[string]{
				Name:    "Telemetry URL",
				Usage:   "URL to send telemetry.",
				Flag:    "telemetry-url",
				Hidden:  true,
				Default: "https://telemetry.coder.com",
			},
		},
		TLS: &codersdk.TLSConfig{
			Enable: &codersdk.DeploymentConfigField[bool]{
				Name:  "TLS Enable",
				Usage: "Whether TLS will be enabled.",
				Flag:  "tls-enable",
			},
			Address: &codersdk.DeploymentConfigField[string]{
				Name:    "TLS Address",
				Usage:   "HTTPS bind address of the server.",
				Flag:    "tls-address",
				Default: "127.0.0.1:3443",
			},
			// DEPRECATED: Use RedirectToAccessURL instead.
			RedirectHTTP: &codersdk.DeploymentConfigField[bool]{
				Name:    "Redirect HTTP to HTTPS",
				Usage:   "Whether HTTP requests will be redirected to the access URL (if it's a https URL and TLS is enabled). Requests to local IP addresses are never redirected regardless of this setting.",
				Flag:    "tls-redirect-http-to-https",
				Default: true,
				Hidden:  true,
			},
			CertFiles: &codersdk.DeploymentConfigField[[]string]{
				Name:  "TLS Certificate Files",
				Usage: "Path to each certificate for TLS. It requires a PEM-encoded file. To configure the listener to use a CA certificate, concatenate the primary certificate and the CA certificate together. The primary certificate should appear first in the combined file.",
				Flag:  "tls-cert-file",
			},
			ClientCAFile: &codersdk.DeploymentConfigField[string]{
				Name:  "TLS Client CA Files",
				Usage: "PEM-encoded Certificate Authority file used for checking the authenticity of client",
				Flag:  "tls-client-ca-file",
			},
			ClientAuth: &codersdk.DeploymentConfigField[string]{
				Name:    "TLS Client Auth",
				Usage:   "Policy the server will follow for TLS Client Authentication. Accepted values are \"none\", \"request\", \"require-any\", \"verify-if-given\", or \"require-and-verify\".",
				Flag:    "tls-client-auth",
				Default: "none",
			},
			KeyFiles: &codersdk.DeploymentConfigField[[]string]{
				Name:  "TLS Key Files",
				Usage: "Paths to the private keys for each of the certificates. It requires a PEM-encoded file.",
				Flag:  "tls-key-file",
			},
			MinVersion: &codersdk.DeploymentConfigField[string]{
				Name:    "TLS Minimum Version",
				Usage:   "Minimum supported version of TLS. Accepted values are \"tls10\", \"tls11\", \"tls12\" or \"tls13\"",
				Flag:    "tls-min-version",
				Default: "tls12",
			},
			ClientCertFile: &codersdk.DeploymentConfigField[string]{
				Name:  "TLS Client Cert File",
				Usage: "Path to certificate for client TLS authentication. It requires a PEM-encoded file.",
				Flag:  "tls-client-cert-file",
			},
			ClientKeyFile: &codersdk.DeploymentConfigField[string]{
				Name:  "TLS Client Key File",
				Usage: "Path to key for client TLS authentication. It requires a PEM-encoded file.",
				Flag:  "tls-client-key-file",
			},
		},
		Trace: &codersdk.TraceConfig{
			Enable: &codersdk.DeploymentConfigField[bool]{
				Name:  "Trace Enable",
				Usage: "Whether application tracing data is collected. It exports to a backend configured by environment variables. See: https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/protocol/exporter.md",
				Flag:  "trace",
			},
			HoneycombAPIKey: &codersdk.DeploymentConfigField[string]{
				Name:   "Trace Honeycomb API Key",
				Usage:  "Enables trace exporting to Honeycomb.io using the provided API Key.",
				Flag:   "trace-honeycomb-api-key",
				Secret: true,
			},
			CaptureLogs: &codersdk.DeploymentConfigField[bool]{
				Name:  "Capture Logs in Traces",
				Usage: "Enables capturing of logs as events in traces. This is useful for debugging, but may result in a very large amount of events being sent to the tracing backend which may incur significant costs. If the verbose flag was supplied, debug-level logs will be included.",
				Flag:  "trace-logs",
			},
		},
		SecureAuthCookie: &codersdk.DeploymentConfigField[bool]{
			Name:  "Secure Auth Cookie",
			Usage: "Controls if the 'Secure' property is set on browser session cookies.",
			Flag:  "secure-auth-cookie",
		},
		StrictTransportSecurity: &codersdk.DeploymentConfigField[int]{
			Name: "Strict-Transport-Security",
			Usage: "Controls if the 'Strict-Transport-Security' header is set on all static file responses. " +
				"This header should only be set if the server is accessed via HTTPS. This value is the MaxAge in seconds of " +
				"the header.",
			Default: 0,
			Flag:    "strict-transport-security",
		},
		StrictTransportSecurityOptions: &codersdk.DeploymentConfigField[[]string]{
			Name: "Strict-Transport-Security Options",
			Usage: "Two optional fields can be set in the Strict-Transport-Security header; 'includeSubDomains' and 'preload'. " +
				"The 'strict-transport-security' flag must be set to a non-zero value for these options to be used.",
			Flag: "strict-transport-security-options",
		},
		SSHKeygenAlgorithm: &codersdk.DeploymentConfigField[string]{
			Name:    "SSH Keygen Algorithm",
			Usage:   "The algorithm to use for generating ssh keys. Accepted values are \"ed25519\", \"ecdsa\", or \"rsa4096\".",
			Flag:    "ssh-keygen-algorithm",
			Default: "ed25519",
		},
		MetricsCacheRefreshInterval: &codersdk.DeploymentConfigField[time.Duration]{
			Name:    "Metrics Cache Refresh Interval",
			Usage:   "How frequently metrics are refreshed",
			Flag:    "metrics-cache-refresh-interval",
			Hidden:  true,
			Default: time.Hour,
		},
		AgentStatRefreshInterval: &codersdk.DeploymentConfigField[time.Duration]{
			Name:    "Agent Stat Refresh Interval",
			Usage:   "How frequently agent stats are recorded",
			Flag:    "agent-stats-refresh-interval",
			Hidden:  true,
			Default: 10 * time.Minute,
		},
		AgentFallbackTroubleshootingURL: &codersdk.DeploymentConfigField[string]{
			Name:    "Agent Fallback Troubleshooting URL",
			Usage:   "URL to use for agent troubleshooting when not set in the template",
			Flag:    "agent-fallback-troubleshooting-url",
			Hidden:  true,
			Default: "https://coder.com/docs/coder-oss/latest/templates#troubleshooting-templates",
		},
		AuditLogging: &codersdk.DeploymentConfigField[bool]{
			Name:       "Audit Logging",
			Usage:      "Specifies whether audit logging is enabled.",
			Flag:       "audit-logging",
			Default:    true,
			Enterprise: true,
		},
		BrowserOnly: &codersdk.DeploymentConfigField[bool]{
			Name:       "Browser Only",
			Usage:      "Whether Coder only allows connections to workspaces via the browser.",
			Flag:       "browser-only",
			Enterprise: true,
		},
		SCIMAPIKey: &codersdk.DeploymentConfigField[string]{
			Name:       "SCIM API Key",
			Usage:      "Enables SCIM and sets the authentication header for the built-in SCIM server. New users are automatically created with OIDC authentication.",
			Flag:       "scim-auth-header",
			Enterprise: true,
			Secret:     true,
		},
		Provisioner: &codersdk.ProvisionerConfig{
			Daemons: &codersdk.DeploymentConfigField[int]{
				Name:    "Provisioner Daemons",
				Usage:   "Number of provisioner daemons to create on start. If builds are stuck in queued state for a long time, consider increasing this.",
				Flag:    "provisioner-daemons",
				Default: 3,
			},
			DaemonPollInterval: &codersdk.DeploymentConfigField[time.Duration]{
				Name:    "Poll Interval",
				Usage:   "Time to wait before polling for a new job.",
				Flag:    "provisioner-daemon-poll-interval",
				Default: time.Second,
			},
			DaemonPollJitter: &codersdk.DeploymentConfigField[time.Duration]{
				Name:    "Poll Jitter",
				Usage:   "Random jitter added to the poll interval.",
				Flag:    "provisioner-daemon-poll-jitter",
				Default: 100 * time.Millisecond,
			},
			ForceCancelInterval: &codersdk.DeploymentConfigField[time.Duration]{
				Name:    "Force Cancel Interval",
				Usage:   "Time to force cancel provisioning tasks that are stuck.",
				Flag:    "provisioner-force-cancel-interval",
				Default: 10 * time.Minute,
			},
		},
		RateLimit: &codersdk.RateLimitConfig{
			DisableAll: &codersdk.DeploymentConfigField[bool]{
				Name:    "Disable All Rate Limits",
				Usage:   "Disables all rate limits. This is not recommended in production.",
				Flag:    "dangerous-disable-rate-limits",
				Default: false,
			},
			API: &codersdk.DeploymentConfigField[int]{
				Name:  "API Rate Limit",
				Usage: "Maximum number of requests per minute allowed to the API per user, or per IP address for unauthenticated users. Negative values mean no rate limit. Some API endpoints have separate strict rate limits regardless of this value to prevent denial-of-service or brute force attacks.",
				// Change the env from the auto-generated CODER_RATE_LIMIT_API to the
				// old value to avoid breaking existing deployments.
				EnvOverride: "CODER_API_RATE_LIMIT",
				Flag:        "api-rate-limit",
				Default:     512,
			},
		},
		// DEPRECATED: use Experiments instead.
		Experimental: &codersdk.DeploymentConfigField[bool]{
			Name:    "Experimental",
			Usage:   "Enable experimental features. Experimental features are not ready for production.",
			Flag:    "experimental",
			Default: false,
			Hidden:  true,
		},
		Experiments: &codersdk.DeploymentConfigField[[]string]{
			Name:    "Experiments",
			Usage:   "Enable one or more experiments. These are not ready for production. Separate multiple experiments with commas, or enter '*' to opt-in to all available experiments.",
			Flag:    "experiments",
			Default: []string{},
		},
		UpdateCheck: &codersdk.DeploymentConfigField[bool]{
			Name:    "Update Check",
			Usage:   "Periodically check for new releases of Coder and inform the owner. The check is performed once per day.",
			Flag:    "update-check",
			Default: flag.Lookup("test.v") == nil && !buildinfo.IsDev(),
		},
		MaxTokenLifetime: &codersdk.DeploymentConfigField[time.Duration]{
			Name:    "Max Token Lifetime",
			Usage:   "The maximum lifetime duration users can specify when creating an API token.",
			Flag:    "max-token-lifetime",
			Default: 24 * 30 * time.Hour,
		},
		Swagger: &codersdk.SwaggerConfig{
			Enable: &codersdk.DeploymentConfigField[bool]{
				Name:    "Enable swagger endpoint",
				Usage:   "Expose the swagger endpoint via /swagger.",
				Flag:    "swagger-enable",
				Default: false,
			},
		},
		Logging: &codersdk.LoggingConfig{
			Human: &codersdk.DeploymentConfigField[string]{
				Name:    "Human Log Location",
				Usage:   "Output human-readable logs to a given file.",
				Flag:    "log-human",
				Default: "/dev/stderr",
			},
			JSON: &codersdk.DeploymentConfigField[string]{
				Name:    "JSON Log Location",
				Usage:   "Output JSON logs to a given file.",
				Flag:    "log-json",
				Default: "",
			},
			Stackdriver: &codersdk.DeploymentConfigField[string]{
				Name:    "Stackdriver Log Location",
				Usage:   "Output Stackdriver compatible logs to a given file.",
				Flag:    "log-stackdriver",
				Default: "",
			},
		},
		Dangerous: &codersdk.DangerousConfig{
			AllowPathAppSharing: &codersdk.DeploymentConfigField[bool]{
				Name:    "DANGEROUS: Allow Path App Sharing",
				Usage:   "Allow workspace apps that are not served from subdomains to be shared. Path-based app sharing is DISABLED by default for security purposes. Path-based apps can make requests to the Coder API and pose a security risk when the workspace serves malicious JavaScript. Path-based apps can be disabled entirely with --disable-path-apps for further security.",
				Flag:    "dangerous-allow-path-app-sharing",
				Default: false,
			},
			AllowPathAppSiteOwnerAccess: &codersdk.DeploymentConfigField[bool]{
				Name:    "DANGEROUS: Allow Site Owners to Access Path Apps",
				Usage:   "Allow site-owners to access workspace apps from workspaces they do not own. Owners cannot access path-based apps they do not own by default. Path-based apps can make requests to the Coder API and pose a security risk when the workspace serves malicious JavaScript. Path-based apps can be disabled entirely with --disable-path-apps for further security.",
				Flag:    "dangerous-allow-path-app-site-owner-access",
				Default: false,
			},
		},
		DisablePathApps: &codersdk.DeploymentConfigField[bool]{
			Name:    "Disable Path Apps",
			Usage:   "Disable workspace apps that are not served from subdomains. Path-based apps can make requests to the Coder API and pose a security risk when the workspace serves malicious JavaScript. This is recommended for security purposes if a --wildcard-access-url is configured.",
			Flag:    "disable-path-apps",
			Default: false,
		},
		SessionDuration: &codersdk.DeploymentConfigField[time.Duration]{
			Name:    "Session Duration",
			Usage:   "The token expiry duration for browser sessions. Sessions may last longer if they are actively making requests, but this functionality can be disabled via --disable-session-expiry-refresh.",
			Flag:    "session-duration",
			Default: 24 * time.Hour,
		},
		DisableSessionExpiryRefresh: &codersdk.DeploymentConfigField[bool]{
			Name:    "Disable Session Expiry Refresh",
			Usage:   "Disable automatic session expiry bumping due to activity. This forces all sessions to become invalid after the session expiry duration has been reached.",
			Flag:    "disable-session-expiry-refresh",
			Default: false,
		},
		DisablePasswordAuth: &codersdk.DeploymentConfigField[bool]{
			Name:    "Disable Password Authentication",
			Usage:   "Disable password authentication. This is recommended for security purposes in production deployments that rely on an identity provider. Any user with the owner role will be able to sign in with their password regardless of this setting to avoid potential lock out. If you are locked out of your account, you can use the `coder server create-admin` command to create a new admin user directly in the database.",
			Flag:    "disable-password-auth",
			Default: false,
		},
	}
}

//nolint:revive
func Config(flagset *pflag.FlagSet, vip *viper.Viper) (*codersdk.DeploymentConfig, error) {
	dc := newConfig()
	flg, err := flagset.GetString(config.FlagName)
	if err != nil {
		return nil, xerrors.Errorf("get global config from flag: %w", err)
	}
	vip.SetEnvPrefix("coder")

	if flg != "" {
		vip.SetConfigFile(flg + "/server.yaml")
		err = vip.ReadInConfig()
		if err != nil && !xerrors.Is(err, os.ErrNotExist) {
			return dc, xerrors.Errorf("reading deployment config: %w", err)
		}
	}

	setConfig("", vip, &dc)

	return dc, nil
}

func setConfig(prefix string, vip *viper.Viper, target interface{}) {
	val := reflect.Indirect(reflect.ValueOf(target))
	typ := val.Type()
	if typ.Kind() != reflect.Struct {
		val = val.Elem()
		typ = val.Type()
	}

	// Ensure that we only bind env variables to proper fields,
	// otherwise Viper will get confused if the parent struct is
	// assigned a value.
	if strings.HasPrefix(typ.Name(), "DeploymentConfigField[") {
		value := val.FieldByName("Value").Interface()

		env, ok := val.FieldByName("EnvOverride").Interface().(string)
		if !ok {
			panic("DeploymentConfigField[].EnvOverride must be a string")
		}
		if env == "" {
			env = formatEnv(prefix)
		}

		switch value.(type) {
		case string:
			vip.MustBindEnv(prefix, env)
			val.FieldByName("Value").SetString(vip.GetString(prefix))
		case bool:
			vip.MustBindEnv(prefix, env)
			val.FieldByName("Value").SetBool(vip.GetBool(prefix))
		case int:
			vip.MustBindEnv(prefix, env)
			val.FieldByName("Value").SetInt(int64(vip.GetInt(prefix)))
		case time.Duration:
			vip.MustBindEnv(prefix, env)
			val.FieldByName("Value").SetInt(int64(vip.GetDuration(prefix)))
		case []string:
			vip.MustBindEnv(prefix, env)
			// As of October 21st, 2022 we supported delimiting a string
			// with a comma, but Viper only supports with a space. This
			// is a small hack around it!
			rawSlice := reflect.ValueOf(vip.GetStringSlice(prefix)).Interface()
			stringSlice, ok := rawSlice.([]string)
			if !ok {
				panic(fmt.Sprintf("string slice is of type %T", rawSlice))
			}
			value := make([]string, 0, len(stringSlice))
			for _, entry := range stringSlice {
				value = append(value, strings.Split(entry, ",")...)
			}
			val.FieldByName("Value").Set(reflect.ValueOf(value))
		case []codersdk.GitAuthConfig:
			// Do not bind to CODER_GITAUTH, instead bind to CODER_GITAUTH_0_*, etc.
			values := readSliceFromViper[codersdk.GitAuthConfig](vip, prefix, value)
			val.FieldByName("Value").Set(reflect.ValueOf(values))
		default:
			panic(fmt.Sprintf("unsupported type %T", value))
		}
		return
	}

	for i := 0; i < typ.NumField(); i++ {
		fv := val.Field(i)
		ft := fv.Type()
		tag := typ.Field(i).Tag.Get("json")
		var key string
		if prefix == "" {
			key = tag
		} else {
			key = fmt.Sprintf("%s.%s", prefix, tag)
		}
		switch ft.Kind() {
		case reflect.Ptr:
			setConfig(key, vip, fv.Interface())
		case reflect.Slice:
			for j := 0; j < fv.Len(); j++ {
				key := fmt.Sprintf("%s.%d", key, j)
				setConfig(key, vip, fv.Index(j).Interface())
			}
		default:
			panic(fmt.Sprintf("unsupported type %T", ft))
		}
	}
}

// readSliceFromViper reads a typed mapping from the key provided.
// This enables environment variables like CODER_GITAUTH_<index>_CLIENT_ID.
func readSliceFromViper[T any](vip *viper.Viper, key string, value any) []T {
	elementType := reflect.TypeOf(value).Elem()
	returnValues := make([]T, 0)
	for entry := 0; true; entry++ {
		// Only create an instance when the entry exists in viper...
		// otherwise we risk
		var instance *reflect.Value
		for i := 0; i < elementType.NumField(); i++ {
			fve := elementType.Field(i)
			prop := fve.Tag.Get("json")
			// For fields that are omitted in JSON, we use a YAML tag.
			if prop == "-" {
				prop = fve.Tag.Get("yaml")
			}
			configKey := fmt.Sprintf("%s.%d.%s", key, entry, prop)

			// Ensure the env entry for this key is registered
			// before checking value.
			//
			// We don't support DeploymentConfigField[].EnvOverride for array flags so
			// this is fine to just use `formatEnv` here.
			vip.MustBindEnv(configKey, formatEnv(configKey))

			value := vip.Get(configKey)
			if value == nil {
				continue
			}
			if instance == nil {
				newType := reflect.Indirect(reflect.New(elementType))
				instance = &newType
			}
			switch v := instance.Field(i).Type().String(); v {
			case "[]string":
				value = vip.GetStringSlice(configKey)
			case "bool":
				value = vip.GetBool(configKey)
			default:
			}
			instance.Field(i).Set(reflect.ValueOf(value))
		}
		if instance == nil {
			break
		}
		value, ok := instance.Interface().(T)
		if !ok {
			continue
		}
		returnValues = append(returnValues, value)
	}
	return returnValues
}

func NewViper() *viper.Viper {
	dc := newConfig()
	vip := viper.New()
	vip.SetEnvPrefix("coder")
	vip.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))

	setViperDefaults("", vip, dc)

	return vip
}

func setViperDefaults(prefix string, vip *viper.Viper, target interface{}) {
	val := reflect.ValueOf(target).Elem()
	val = reflect.Indirect(val)
	typ := val.Type()
	if strings.HasPrefix(typ.Name(), "DeploymentConfigField[") {
		value := val.FieldByName("Default").Interface()
		vip.SetDefault(prefix, value)
		return
	}

	for i := 0; i < typ.NumField(); i++ {
		fv := val.Field(i)
		ft := fv.Type()
		tag := typ.Field(i).Tag.Get("json")
		var key string
		if prefix == "" {
			key = tag
		} else {
			key = fmt.Sprintf("%s.%s", prefix, tag)
		}
		switch ft.Kind() {
		case reflect.Ptr:
			setViperDefaults(key, vip, fv.Interface())
		case reflect.Slice:
			// we currently don't support default values on structured slices
			continue
		default:
			panic(fmt.Sprintf("unsupported type %T", ft))
		}
	}
}

//nolint:revive
func AttachFlags(flagset *pflag.FlagSet, vip *viper.Viper, enterprise bool) {
	setFlags("", flagset, vip, newConfig(), enterprise)
}

//nolint:revive
func setFlags(prefix string, flagset *pflag.FlagSet, vip *viper.Viper, target interface{}, enterprise bool) {
	val := reflect.Indirect(reflect.ValueOf(target))
	typ := val.Type()
	if strings.HasPrefix(typ.Name(), "DeploymentConfigField[") {
		isEnt := val.FieldByName("Enterprise").Bool()
		if enterprise != isEnt {
			return
		}
		flg := val.FieldByName("Flag").String()
		if flg == "" {
			return
		}

		env, ok := val.FieldByName("EnvOverride").Interface().(string)
		if !ok {
			panic("DeploymentConfigField[].EnvOverride must be a string")
		}
		if env == "" {
			env = formatEnv(prefix)
		}

		usage := val.FieldByName("Usage").String()
		usage = fmt.Sprintf("%s\n%s", usage, cliui.Styles.Placeholder.Render("Consumes $"+env))
		shorthand := val.FieldByName("Shorthand").String()
		hidden := val.FieldByName("Hidden").Bool()
		value := val.FieldByName("Default").Interface()

		// Allow currently set environment variables
		// to override default values in help output.
		vip.MustBindEnv(prefix, env)

		switch value.(type) {
		case string:
			_ = flagset.StringP(flg, shorthand, vip.GetString(prefix), usage)
		case bool:
			_ = flagset.BoolP(flg, shorthand, vip.GetBool(prefix), usage)
		case int:
			_ = flagset.IntP(flg, shorthand, vip.GetInt(prefix), usage)
		case time.Duration:
			_ = flagset.DurationP(flg, shorthand, vip.GetDuration(prefix), usage)
		case []string:
			_ = flagset.StringSliceP(flg, shorthand, vip.GetStringSlice(prefix), usage)
		case []codersdk.GitAuthConfig:
			// Ignore this one!
		default:
			panic(fmt.Sprintf("unsupported type %T", typ))
		}

		_ = vip.BindPFlag(prefix, flagset.Lookup(flg))
		if hidden {
			_ = flagset.MarkHidden(flg)
		}

		return
	}

	for i := 0; i < typ.NumField(); i++ {
		fv := val.Field(i)
		ft := fv.Type()
		tag := typ.Field(i).Tag.Get("json")
		var key string
		if prefix == "" {
			key = tag
		} else {
			key = fmt.Sprintf("%s.%s", prefix, tag)
		}
		switch ft.Kind() {
		case reflect.Ptr:
			setFlags(key, flagset, vip, fv.Interface(), enterprise)
		case reflect.Slice:
			for j := 0; j < fv.Len(); j++ {
				key := fmt.Sprintf("%s.%d", key, j)
				setFlags(key, flagset, vip, fv.Index(j).Interface(), enterprise)
			}
		default:
			panic(fmt.Sprintf("unsupported type %T", ft))
		}
	}
}

func formatEnv(key string) string {
	return "CODER_" + strings.ToUpper(strings.NewReplacer("-", "_", ".", "_").Replace(key))
}

func DefaultCacheDir() string {
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

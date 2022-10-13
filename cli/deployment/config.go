package deployment

import (
	"flag"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"

	"github.com/coder/coder/codersdk"
)

func Config() codersdk.DeploymentConfig {
	return codersdk.DeploymentConfig{
		// External URL to access your deployment. This must be accessible by all provisioned workspaces.
		AccessURL: "",
		// Specifies the wildcard hostname to use for workspace applications in the form "*.example.com".
		WildcardAccessURL: "",
		// Bind address of the server.
		Address: "127.0.0.1:3000",
		// Interval to poll for scheduled workspace builds.
		AutobuildPollInterval: time.Minute,
		DERP: codersdk.DERPConfig{
			Server: codersdk.DERPServerConfig{
				// Whether to enable or disable the embedded DERP relay server.
				Enable: true,
				// Region ID to use for the embedded DERP server.
				RegionID: 999,
				// Region code to use for the embedded DERP server.
				RegionCode: "coder",
				// Region name that for the embedded DERP server.
				RegionName: "Coder Embedded Relay",
				// Addresses for STUN servers to establish P2P connections. Set empty to disable P2P connections.
				STUNAddresses: []string{"stun.l.google.com:19302"},
			},
			Config: codersdk.DERPConfigConfig{
				// URL to fetch a DERP mapping on startup. See: https://tailscale.com/kb/1118/custom-derp-servers/
				URL: "",
				// Path to read a DERP mapping from. See: https://tailscale.com/kb/1118/custom-derp-servers/
				Path: "",
			},
		},
		Prometheus: codersdk.PrometheusConfig{
			// Serve prometheus metrics on the address defined by `prometheus.address`.
			Enable: false,
			// The bind address to serve prometheus metrics.
			Address: "127.0.0.1:2112",
		},
		Pprof: codersdk.PprofConfig{
			// Serve pprof metrics on the address defined by `pprof.address`.
			Enable: false,
			// The bind address to serve pprof.
			Address: "127.0.0.1:6060",
		},
		// The directory to cache temporary files. If unspecified and $CACHE_DIRECTORY is set, it will be used for compatibility with systemd.
		CacheDir: defaultCacheDir(),
		// Controls whether data will be stored in an in-memory database.
		InMemoryDatabase: false,
		// Number of provisioner daemons to create on start. If builds are stuck in queued state for a long time, consider increasing this.
		ProvisionerDaemonCount: 3,
		// URL of a PostgreSQL database. If empty, PostgreSQL binaries will be downloaded from Maven (https://repo1.maven.org/maven2) and store all data in the config root. Access the built-in database with "coder server postgres-builtin-url".
		PostgresURL: "",
		Oauth2Github: codersdk.Oauth2GithubConfig{
			// Client ID for Login with GitHub.
			ClientID: "",
			// Client secret for Login with GitHub.
			ClientSecret: "",
			// Organizations the user must be a member of to Login with GitHub.
			AllowedOrganizations: []string{},
			// Teams inside organizations the user must be a member of to Login with GitHub. Structured as: <organization-name>/<team-slug>.
			AllowedTeams: []string{},
			// Whether new users can sign up with GitHub.
			AllowSignups: true,
			// Base URL of a GitHub Enterprise deployment to use for Login with GitHub.
			EnterpriseBaseURL: "",
		},

		OIDC: codersdk.OIDCConfig{
			// Whether new users can sign up with OIDC.
			AllowSignups: true,
			// Client ID to use for Login with OIDC.
			ClientID: "",
			// Client secret to use for Login with OIDC.
			ClientSecret: "",
			// Email domain that clients logging in with OIDC must match.
			EmailDomain: "",
			// Issuer URL to use for Login with OIDC.
			IssuerURL: "",
			// Scopes to grant when authenticating with OIDC.
			Scopes: []string{oidc.ScopeOpenID, "profile", "email"},
		},
		Telemetry: codersdk.TelemetryConfig{
			// Whether telemetry is enabled or not. Coder collects anonymized usage data to help improve our product.
			Enable: flag.Lookup("test.v") == nil,
			// Whether Opentelemetry traces are sent to Coder. Coder collects anonymized application tracing to help improve our product. Disabling telemetry also disables this option.
			TraceEnable: flag.Lookup("test.v") == nil,
			// URL to send telemetry.
			URL: "https://telemetry.coder.com",
		},
		TLSConfig: codersdk.TLSConfig{
			// Whether TLS will be enabled.
			Enable: false,
			// Path to each certificate for TLS. It requires a PEM-encoded file. To configure the listener to use a CA certificate, concatenate the primary certificate and the CA certificate together. The primary certificate should appear first in the combined file.
			CertFiles: []string{},
			// PEM-encoded Certificate Authority file used for checking the authenticity of client
			ClientCAFile: "",
			// Policy the server will follow for TLS Client Authentication. Accepted values are "none", "request", "require-any", "verify-if-given", or "require-and-verify".
			ClientAuth: "request",
			// Paths to the private keys for each of the certificates. It requires a PEM-encoded file.
			KeyFiles: []string{},
			// Minimum supported version of TLS. Accepted values are "tls10", "tls11", "tls12" or "tls13"
			MinVersion: "tls12",
		},
		// Whether application tracing data is collected.
		TraceEnable: false,
		// Controls if the 'Secure' property is set on browser session cookies.
		SecureAuthCookie: false,
		// The algorithm to use for generating ssh keys. Accepted values are "ed25519", "ecdsa", or "rsa4096".
		SSHKeygenAlgorithm: "ed25519",
		// Templates to auto-import. Available auto-importable templates are: kubernetes
		AutoImportTemplates: []string{},
		// How frequently metrics are refreshed
		MetricsCacheRefreshInterval: time.Hour,
		// How frequently agent stats are recorded
		AgentStatRefreshInterval: 10 * time.Minute,
		// Enables verbose logging.
		Verbose: false,
		// Specifies whether audit logging is enabled.
		AuditLogging: true,
		// Whether Coder only allows connections to workspaces via the browser.
		BrowserOnly: false,
		// Enables SCIM and sets the authentication header for the built-in SCIM server. New users are automatically created with OIDC authentication.
		SCIMAuthHeader: "",
		// Enables and sets a limit on how many workspaces each user can create.
		UserWorkspaceQuota: 0,
	}
}

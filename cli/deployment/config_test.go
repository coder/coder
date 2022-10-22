package deployment_test

import (
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/config"
	"github.com/coder/coder/cli/deployment"
	"github.com/coder/coder/codersdk"
)

// nolint:paralleltest
func TestConfig(t *testing.T) {
	viper := deployment.NewViper()
	flagSet := pflag.NewFlagSet("", pflag.ContinueOnError)
	flagSet.String(config.FlagName, "", "")
	deployment.AttachFlags(flagSet, viper, true)

	for _, tc := range []struct {
		Name  string
		Env   map[string]string
		Valid func(config codersdk.DeploymentConfig)
	}{{
		Name: "Deployment",
		Env: map[string]string{
			"CODER_ADDRESS":              "0.0.0.0:8443",
			"CODER_ACCESS_URL":           "https://dev.coder.com",
			"CODER_PG_CONNECTION_URL":    "some-url",
			"CODER_PPROF_ADDRESS":        "something",
			"CODER_PPROF_ENABLE":         "true",
			"CODER_PROMETHEUS_ADDRESS":   "hello-world",
			"CODER_PROMETHEUS_ENABLE":    "true",
			"CODER_PROVISIONER_DAEMONS":  "5",
			"CODER_SECURE_AUTH_COOKIE":   "true",
			"CODER_SSH_KEYGEN_ALGORITHM": "potato",
			"CODER_TELEMETRY":            "false",
			"CODER_TELEMETRY_TRACE":      "false",
			"CODER_WILDCARD_ACCESS_URL":  "something-wildcard.com",
		},
		Valid: func(config codersdk.DeploymentConfig) {
			require.Equal(t, config.Address.Value, "0.0.0.0:8443")
			require.Equal(t, config.AccessURL.Value, "https://dev.coder.com")
			require.Equal(t, config.PostgresURL.Value, "some-url")
			require.Equal(t, config.PprofAddress.Value, "something")
			require.Equal(t, config.PprofEnable.Value, true)
			require.Equal(t, config.PrometheusAddress.Value, "hello-world")
			require.Equal(t, config.PrometheusEnable.Value, true)
			require.Equal(t, config.ProvisionerDaemons.Value, 5)
			require.Equal(t, config.SecureAuthCookie.Value, true)
			require.Equal(t, config.SSHKeygenAlgorithm.Value, "potato")
			require.Equal(t, config.TelemetryEnable.Value, false)
			require.Equal(t, config.TelemetryTrace.Value, false)
			require.Equal(t, config.WildcardAccessURL.Value, "something-wildcard.com")
		},
	}, {
		Name: "DERP",
		Env: map[string]string{
			"CODER_DERP_CONFIG_PATH":           "/example/path",
			"CODER_DERP_CONFIG_URL":            "https://google.com",
			"CODER_DERP_SERVER_ENABLE":         "false",
			"CODER_DERP_SERVER_REGION_CODE":    "something",
			"CODER_DERP_SERVER_REGION_ID":      "123",
			"CODER_DERP_SERVER_REGION_NAME":    "Code-Land",
			"CODER_DERP_SERVER_RELAY_URL":      "1.1.1.1",
			"CODER_DERP_SERVER_STUN_ADDRESSES": "google.org",
		},
		Valid: func(config codersdk.DeploymentConfig) {
			require.Equal(t, config.DERPConfigPath.Value, "/example/path")
			require.Equal(t, config.DERPConfigURL.Value, "https://google.com")
			require.Equal(t, config.DERPServerEnable.Value, false)
			require.Equal(t, config.DERPServerRegionCode.Value, "something")
			require.Equal(t, config.DERPServerRegionID.Value, 123)
			require.Equal(t, config.DERPServerRegionName.Value, "Code-Land")
			require.Equal(t, config.DERPServerRelayURL.Value, "1.1.1.1")
			require.Equal(t, config.DERPServerSTUNAddresses.Value, []string{"google.org"})
		},
	}, {
		Name: "Enterprise",
		Env: map[string]string{
			"CODER_AUDIT_LOGGING":        "false",
			"CODER_BROWSER_ONLY":         "true",
			"CODER_SCIM_API_KEY":         "some-key",
			"CODER_USER_WORKSPACE_QUOTA": "10",
		},
		Valid: func(config codersdk.DeploymentConfig) {
			require.Equal(t, config.AuditLogging.Value, false)
			require.Equal(t, config.BrowserOnly.Value, true)
			require.Equal(t, config.SCIMAPIKey.Value, "some-key")
			require.Equal(t, config.UserWorkspaceQuota.Value, 10)
		},
	}, {
		Name: "TLS",
		Env: map[string]string{
			"CODER_TLS_CERT_FILE":      "/etc/acme-sh/dev.coder.com,/etc/acme-sh/*.dev.coder.com",
			"CODER_TLS_KEY_FILE":       "/etc/acme-sh/dev.coder.com,/etc/acme-sh/*.dev.coder.com",
			"CODER_TLS_CLIENT_AUTH":    "/some/path",
			"CODER_TLS_CLIENT_CA_FILE": "/some/path",
			"CODER_TLS_ENABLE":         "true",
			"CODER_TLS_MIN_VERSION":    "tls10",
		},
		Valid: func(config codersdk.DeploymentConfig) {
			require.Len(t, config.TLSCertFiles.Value, 2)
			require.Equal(t, config.TLSCertFiles.Value[0], "/etc/acme-sh/dev.coder.com")
			require.Equal(t, config.TLSCertFiles.Value[1], "/etc/acme-sh/*.dev.coder.com")

			require.Len(t, config.TLSKeyFiles.Value, 2)
			require.Equal(t, config.TLSKeyFiles.Value[0], "/etc/acme-sh/dev.coder.com")
			require.Equal(t, config.TLSKeyFiles.Value[1], "/etc/acme-sh/*.dev.coder.com")

			require.Equal(t, config.TLSClientAuth.Value, "/some/path")
			require.Equal(t, config.TLSClientCAFile.Value, "/some/path")
			require.Equal(t, config.TLSEnable.Value, true)
			require.Equal(t, config.TLSMinVersion.Value, "tls10")
		},
	}, {
		Name: "OIDC",
		Env: map[string]string{
			"CODER_OIDC_ISSUER_URL":    "https://accounts.google.com",
			"CODER_OIDC_EMAIL_DOMAIN":  "coder.com",
			"CODER_OIDC_CLIENT_ID":     "client",
			"CODER_OIDC_CLIENT_SECRET": "secret",
			"CODER_OIDC_ALLOW_SIGNUPS": "false",
			"CODER_OIDC_SCOPES":        "something,here",
		},
		Valid: func(config codersdk.DeploymentConfig) {
			require.Equal(t, config.OIDCIssuerURL.Value, "https://accounts.google.com")
			require.Equal(t, config.OIDCEmailDomain.Value, "coder.com")
			require.Equal(t, config.OIDCClientID.Value, "client")
			require.Equal(t, config.OIDCClientSecret.Value, "secret")
			require.Equal(t, config.OIDCAllowSignups.Value, false)
			require.Equal(t, config.OIDCScopes.Value, []string{"something", "here"})
		},
	}, {
		Name: "GitHub",
		Env: map[string]string{
			"CODER_OAUTH2_GITHUB_CLIENT_ID":     "client",
			"CODER_OAUTH2_GITHUB_CLIENT_SECRET": "secret",
			"CODER_OAUTH2_GITHUB_ALLOWED_ORGS":  "coder",
			"CODER_OAUTH2_GITHUB_ALLOWED_TEAMS": "coder",
			"CODER_OAUTH2_GITHUB_ALLOW_SIGNUPS": "true",
		},
		Valid: func(config codersdk.DeploymentConfig) {
			require.Equal(t, config.OAuth2GithubClientID.Value, "client")
			require.Equal(t, config.OAuth2GithubClientSecret.Value, "secret")
			require.Equal(t, []string{"coder"}, config.OAuth2GithubAllowedOrgs.Value)
			require.Equal(t, []string{"coder"}, config.OAuth2GithubAllowedTeams.Value)
			require.Equal(t, config.OAuth2GithubAllowSignups.Value, true)
		},
	}} {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			for key, value := range tc.Env {
				t.Setenv(key, value)
			}
			config, err := deployment.Config(flagSet, viper)
			require.NoError(t, err)
			tc.Valid(config)
		})
	}
}

package deployment_test

import (
	"testing"
	"time"

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
		Valid func(config *codersdk.DeploymentConfig)
	}{{
		Name: "Deployment",
		Env: map[string]string{
			"CODER_ADDRESS":                          "0.0.0.0:8443",
			"CODER_ACCESS_URL":                       "https://dev.coder.com",
			"CODER_PG_CONNECTION_URL":                "some-url",
			"CODER_PPROF_ADDRESS":                    "something",
			"CODER_PPROF_ENABLE":                     "true",
			"CODER_PROMETHEUS_ADDRESS":               "hello-world",
			"CODER_PROMETHEUS_ENABLE":                "true",
			"CODER_PROVISIONER_DAEMONS":              "5",
			"CODER_PROVISIONER_DAEMON_POLL_INTERVAL": "5s",
			"CODER_PROVISIONER_DAEMON_POLL_JITTER":   "1s",
			"CODER_SECURE_AUTH_COOKIE":               "true",
			"CODER_SSH_KEYGEN_ALGORITHM":             "potato",
			"CODER_TELEMETRY":                        "false",
			"CODER_TELEMETRY_TRACE":                  "false",
			"CODER_WILDCARD_ACCESS_URL":              "something-wildcard.com",
			"CODER_UPDATE_CHECK":                     "false",
		},
		Valid: func(config *codersdk.DeploymentConfig) {
			require.Equal(t, config.Address.Value, "0.0.0.0:8443")
			require.Equal(t, config.AccessURL.Value, "https://dev.coder.com")
			require.Equal(t, config.PostgresURL.Value, "some-url")
			require.Equal(t, config.Pprof.Address.Value, "something")
			require.Equal(t, config.Pprof.Enable.Value, true)
			require.Equal(t, config.Prometheus.Address.Value, "hello-world")
			require.Equal(t, config.Prometheus.Enable.Value, true)
			require.Equal(t, config.Provisioner.Daemons.Value, 5)
			require.Equal(t, config.Provisioner.DaemonPollInterval.Value, 5*time.Second)
			require.Equal(t, config.Provisioner.DaemonPollJitter.Value, 1*time.Second)
			require.Equal(t, config.SecureAuthCookie.Value, true)
			require.Equal(t, config.SSHKeygenAlgorithm.Value, "potato")
			require.Equal(t, config.Telemetry.Enable.Value, false)
			require.Equal(t, config.Telemetry.Trace.Value, false)
			require.Equal(t, config.WildcardAccessURL.Value, "something-wildcard.com")
			require.Equal(t, config.UpdateCheck.Value, false)
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
		Valid: func(config *codersdk.DeploymentConfig) {
			require.Equal(t, config.DERP.Config.Path.Value, "/example/path")
			require.Equal(t, config.DERP.Config.URL.Value, "https://google.com")
			require.Equal(t, config.DERP.Server.Enable.Value, false)
			require.Equal(t, config.DERP.Server.RegionCode.Value, "something")
			require.Equal(t, config.DERP.Server.RegionID.Value, 123)
			require.Equal(t, config.DERP.Server.RegionName.Value, "Code-Land")
			require.Equal(t, config.DERP.Server.RelayURL.Value, "1.1.1.1")
			require.Equal(t, config.DERP.Server.STUNAddresses.Value, []string{"google.org"})
		},
	}, {
		Name: "Enterprise",
		Env: map[string]string{
			"CODER_AUDIT_LOGGING": "false",
			"CODER_BROWSER_ONLY":  "true",
			"CODER_SCIM_API_KEY":  "some-key",
		},
		Valid: func(config *codersdk.DeploymentConfig) {
			require.Equal(t, config.AuditLogging.Value, false)
			require.Equal(t, config.BrowserOnly.Value, true)
			require.Equal(t, config.SCIMAPIKey.Value, "some-key")
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
		Valid: func(config *codersdk.DeploymentConfig) {
			require.Len(t, config.TLS.CertFiles.Value, 2)
			require.Equal(t, config.TLS.CertFiles.Value[0], "/etc/acme-sh/dev.coder.com")
			require.Equal(t, config.TLS.CertFiles.Value[1], "/etc/acme-sh/*.dev.coder.com")

			require.Len(t, config.TLS.KeyFiles.Value, 2)
			require.Equal(t, config.TLS.KeyFiles.Value[0], "/etc/acme-sh/dev.coder.com")
			require.Equal(t, config.TLS.KeyFiles.Value[1], "/etc/acme-sh/*.dev.coder.com")

			require.Equal(t, config.TLS.ClientAuth.Value, "/some/path")
			require.Equal(t, config.TLS.ClientCAFile.Value, "/some/path")
			require.Equal(t, config.TLS.Enable.Value, true)
			require.Equal(t, config.TLS.MinVersion.Value, "tls10")
		},
	}, {
		Name: "Trace",
		Env: map[string]string{
			"CODER_TRACE_ENABLE":            "true",
			"CODER_TRACE_HONEYCOMB_API_KEY": "my-honeycomb-key",
		},
		Valid: func(config *codersdk.DeploymentConfig) {
			require.Equal(t, config.Trace.Enable.Value, true)
			require.Equal(t, config.Trace.HoneycombAPIKey.Value, "my-honeycomb-key")
		},
	}, {
		Name: "OIDC_Defaults",
		Env:  map[string]string{},
		Valid: func(config *codersdk.DeploymentConfig) {
			require.Empty(t, config.OIDC.IssuerURL.Value)
			require.Empty(t, config.OIDC.EmailDomain.Value)
			require.Empty(t, config.OIDC.ClientID.Value)
			require.Empty(t, config.OIDC.ClientSecret.Value)
			require.True(t, config.OIDC.AllowSignups.Value)
			require.ElementsMatch(t, config.OIDC.Scopes.Value, []string{"openid", "email", "profile"})
			require.False(t, config.OIDC.IgnoreEmailVerified.Value)
		},
	}, {
		Name: "OIDC",
		Env: map[string]string{
			"CODER_OIDC_ISSUER_URL":            "https://accounts.google.com",
			"CODER_OIDC_EMAIL_DOMAIN":          "coder.com",
			"CODER_OIDC_CLIENT_ID":             "client",
			"CODER_OIDC_CLIENT_SECRET":         "secret",
			"CODER_OIDC_ALLOW_SIGNUPS":         "false",
			"CODER_OIDC_SCOPES":                "something,here",
			"CODER_OIDC_IGNORE_EMAIL_VERIFIED": "true",
		},
		Valid: func(config *codersdk.DeploymentConfig) {
			require.Equal(t, config.OIDC.IssuerURL.Value, "https://accounts.google.com")
			require.Equal(t, config.OIDC.EmailDomain.Value, []string{"coder.com"})
			require.Equal(t, config.OIDC.ClientID.Value, "client")
			require.Equal(t, config.OIDC.ClientSecret.Value, "secret")
			require.False(t, config.OIDC.AllowSignups.Value)
			require.Equal(t, config.OIDC.Scopes.Value, []string{"something", "here"})
			require.True(t, config.OIDC.IgnoreEmailVerified.Value)
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
		Valid: func(config *codersdk.DeploymentConfig) {
			require.Equal(t, config.OAuth2.Github.ClientID.Value, "client")
			require.Equal(t, config.OAuth2.Github.ClientSecret.Value, "secret")
			require.Equal(t, []string{"coder"}, config.OAuth2.Github.AllowedOrgs.Value)
			require.Equal(t, []string{"coder"}, config.OAuth2.Github.AllowedTeams.Value)
			require.Equal(t, config.OAuth2.Github.AllowSignups.Value, true)
		},
	}, {
		Name: "GitAuth",
		Env: map[string]string{
			"CODER_GITAUTH_0_ID":            "hello",
			"CODER_GITAUTH_0_TYPE":          "github",
			"CODER_GITAUTH_0_CLIENT_ID":     "client",
			"CODER_GITAUTH_0_CLIENT_SECRET": "secret",
			"CODER_GITAUTH_0_AUTH_URL":      "https://auth.com",
			"CODER_GITAUTH_0_TOKEN_URL":     "https://token.com",
			"CODER_GITAUTH_0_VALIDATE_URL":  "https://validate.com",
			"CODER_GITAUTH_0_REGEX":         "github.com",
			"CODER_GITAUTH_0_SCOPES":        "read write",
			"CODER_GITAUTH_0_NO_REFRESH":    "true",

			"CODER_GITAUTH_1_ID":            "another",
			"CODER_GITAUTH_1_TYPE":          "gitlab",
			"CODER_GITAUTH_1_CLIENT_ID":     "client-2",
			"CODER_GITAUTH_1_CLIENT_SECRET": "secret-2",
			"CODER_GITAUTH_1_AUTH_URL":      "https://auth-2.com",
			"CODER_GITAUTH_1_TOKEN_URL":     "https://token-2.com",
			"CODER_GITAUTH_1_REGEX":         "gitlab.com",
		},
		Valid: func(config *codersdk.DeploymentConfig) {
			require.Len(t, config.GitAuth.Value, 2)
			require.Equal(t, []codersdk.GitAuthConfig{{
				ID:           "hello",
				Type:         "github",
				ClientID:     "client",
				ClientSecret: "secret",
				AuthURL:      "https://auth.com",
				TokenURL:     "https://token.com",
				ValidateURL:  "https://validate.com",
				Regex:        "github.com",
				Scopes:       []string{"read", "write"},
				NoRefresh:    true,
			}, {
				ID:           "another",
				Type:         "gitlab",
				ClientID:     "client-2",
				ClientSecret: "secret-2",
				AuthURL:      "https://auth-2.com",
				TokenURL:     "https://token-2.com",
				Regex:        "gitlab.com",
			}}, config.GitAuth.Value)
		},
	}, {
		Name: "Support links",
		Env: map[string]string{
			"CODER_SUPPORT_LINKS_0_NAME":   "First link",
			"CODER_SUPPORT_LINKS_0_TARGET": "http://target-link-1",
			"CODER_SUPPORT_LINKS_0_ICON":   "bug",

			"CODER_SUPPORT_LINKS_1_NAME":   "Second link",
			"CODER_SUPPORT_LINKS_1_TARGET": "http://target-link-2",
			"CODER_SUPPORT_LINKS_1_ICON":   "chat",
		},
		Valid: func(config *codersdk.DeploymentConfig) {
			require.Len(t, config.Support.Links.Value, 2)
			require.Equal(t, []codersdk.LinkConfig{{
				Name:   "First link",
				Target: "http://target-link-1",
				Icon:   "bug",
			}, {
				Name:   "Second link",
				Target: "http://target-link-2",
				Icon:   "chat",
			}}, config.Support.Links.Value)
		},
	}, {
		Name: "Wrong env must not break default values",
		Env: map[string]string{
			"CODER_PROMETHEUS_ENABLE": "true",
			"CODER_PROMETHEUS":        "true", // Wrong env name, must not break prom addr.
		},
		Valid: func(config *codersdk.DeploymentConfig) {
			require.Equal(t, config.Prometheus.Enable.Value, true)
			require.Equal(t, config.Prometheus.Address.Value, config.Prometheus.Address.Default)
		},
	}, {
		Name: "Experiments - no features",
		Env: map[string]string{
			"CODER_EXPERIMENTS": "",
		},
		Valid: func(config *codersdk.DeploymentConfig) {
			require.Empty(t, config.Experiments.Value)
		},
	}, {
		Name: "Experiments - multiple features",
		Env: map[string]string{
			"CODER_EXPERIMENTS": "foo,bar",
		},
		Valid: func(config *codersdk.DeploymentConfig) {
			expected := []string{"foo", "bar"}
			require.ElementsMatch(t, expected, config.Experiments.Value)
		},
	}} {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			t.Helper()
			for key, value := range tc.Env {
				t.Setenv(key, value)
			}
			config, err := deployment.Config(flagSet, viper)
			require.NoError(t, err)
			tc.Valid(config)
		})
	}
}

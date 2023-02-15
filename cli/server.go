//go:build !slim

package cli

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/pprof"
	"net/url"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/coreos/go-systemd/daemon"
	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"github.com/google/go-github/v43/github"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/mod/semver"
	"golang.org/x/oauth2"
	xgithub "golang.org/x/oauth2/github"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"
	"google.golang.org/api/idtoken"
	"google.golang.org/api/option"
	"tailscale.com/tailcfg"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"cdr.dev/slog/sloggers/slogjson"
	"cdr.dev/slog/sloggers/slogstackdriver"
	"github.com/coder/coder/buildinfo"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/cli/config"
	"github.com/coder/coder/cli/deployment"
	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/autobuild/executor"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbfake"
	"github.com/coder/coder/coderd/database/migrations"
	"github.com/coder/coder/coderd/devtunnel"
	"github.com/coder/coder/coderd/gitauth"
	"github.com/coder/coder/coderd/gitsshkey"
	"github.com/coder/coder/coderd/health"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/prometheusmetrics"
	"github.com/coder/coder/coderd/telemetry"
	"github.com/coder/coder/coderd/tracing"
	"github.com/coder/coder/coderd/updatecheck"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/cryptorand"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisioner/terraform"
	"github.com/coder/coder/provisionerd"
	"github.com/coder/coder/provisionerd/proto"
	"github.com/coder/coder/provisionersdk"
	sdkproto "github.com/coder/coder/provisionersdk/proto"
	"github.com/coder/coder/tailnet"
)

// nolint:gocyclo
func Server(vip *viper.Viper, newAPI func(context.Context, *coderd.Options) (*coderd.API, io.Closer, error)) *cobra.Command {
	root := &cobra.Command{
		Use:   "server",
		Short: "Start a Coder server",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Main command context for managing cancellation of running
			// services.
			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()

			go dumpHandler(ctx)

			cfg, err := deployment.Config(cmd.Flags(), vip)
			if err != nil {
				return xerrors.Errorf("getting deployment config: %w", err)
			}

			// Validate bind addresses.
			if cfg.Address.Value != "" {
				cmd.PrintErr(cliui.Styles.Warn.Render("WARN:") + " --address and -a are deprecated, please use --http-address and --tls-address instead")
				if cfg.TLS.Enable.Value {
					cfg.HTTPAddress.Value = ""
					cfg.TLS.Address.Value = cfg.Address.Value
				} else {
					cfg.HTTPAddress.Value = cfg.Address.Value
					cfg.TLS.Address.Value = ""
				}
			}
			if cfg.TLS.Enable.Value && cfg.TLS.Address.Value == "" {
				return xerrors.Errorf("TLS address must be set if TLS is enabled")
			}
			if !cfg.TLS.Enable.Value && cfg.HTTPAddress.Value == "" {
				return xerrors.Errorf("TLS is disabled. Enable with --tls-enable or specify a HTTP address")
			}

			// Disable rate limits if the `--dangerous-disable-rate-limits` flag
			// was specified.
			loginRateLimit := 60
			filesRateLimit := 12
			if cfg.RateLimit.DisableAll.Value {
				cfg.RateLimit.API.Value = -1
				loginRateLimit = -1
				filesRateLimit = -1
			}

			printLogo(cmd)
			logger, logCloser, err := buildLogger(cmd, cfg)
			if err != nil {
				return xerrors.Errorf("make logger: %w", err)
			}
			defer logCloser()

			// Register signals early on so that graceful shutdown can't
			// be interrupted by additional signals. Note that we avoid
			// shadowing cancel() (from above) here because notifyStop()
			// restores default behavior for the signals. This protects
			// the shutdown sequence from abruptly terminating things
			// like: database migrations, provisioner work, workspace
			// cleanup in dev-mode, etc.
			//
			// To get out of a graceful shutdown, the user can send
			// SIGQUIT with ctrl+\ or SIGKILL with `kill -9`.
			notifyCtx, notifyStop := signal.NotifyContext(ctx, InterruptSignals...)
			defer notifyStop()

			// Ensure we have a unique cache directory for this process.
			cacheDir := filepath.Join(cfg.CacheDirectory.Value, uuid.NewString())
			err = os.MkdirAll(cacheDir, 0o700)
			if err != nil {
				return xerrors.Errorf("create cache directory: %w", err)
			}
			defer os.RemoveAll(cacheDir)

			// Clean up idle connections at the end, e.g.
			// embedded-postgres can leave an idle connection
			// which is caught by goleaks.
			defer http.DefaultClient.CloseIdleConnections()

			var (
				tracerProvider trace.TracerProvider
				sqlDriver      = "postgres"
			)

			// Coder tracing should be disabled if telemetry is disabled unless
			// --telemetry-trace was explicitly provided.
			shouldCoderTrace := cfg.Telemetry.Enable.Value && !isTest()
			// Only override if telemetryTraceEnable was specifically set.
			// By default we want it to be controlled by telemetryEnable.
			if cmd.Flags().Changed("telemetry-trace") {
				shouldCoderTrace = cfg.Telemetry.Trace.Value
			}

			if cfg.Trace.Enable.Value || shouldCoderTrace || cfg.Trace.HoneycombAPIKey.Value != "" {
				sdkTracerProvider, closeTracing, err := tracing.TracerProvider(ctx, "coderd", tracing.TracerOpts{
					Default:   cfg.Trace.Enable.Value,
					Coder:     shouldCoderTrace,
					Honeycomb: cfg.Trace.HoneycombAPIKey.Value,
				})
				if err != nil {
					logger.Warn(ctx, "start telemetry exporter", slog.Error(err))
				} else {
					// allow time for traces to flush even if command context is canceled
					defer func() {
						_ = shutdownWithTimeout(closeTracing, 5*time.Second)
					}()

					d, err := tracing.PostgresDriver(sdkTracerProvider, "coderd.database")
					if err != nil {
						logger.Warn(ctx, "start postgres tracing driver", slog.Error(err))
					} else {
						sqlDriver = d
					}

					tracerProvider = sdkTracerProvider
				}
			}

			config := createConfig(cmd)
			builtinPostgres := false
			// Only use built-in if PostgreSQL URL isn't specified!
			if !cfg.InMemoryDatabase.Value && cfg.PostgresURL.Value == "" {
				var closeFunc func() error
				cmd.Printf("Using built-in PostgreSQL (%s)\n", config.PostgresPath())
				cfg.PostgresURL.Value, closeFunc, err = startBuiltinPostgres(ctx, config, logger)
				if err != nil {
					return err
				}
				builtinPostgres = true
				defer func() {
					cmd.Printf("Stopping built-in PostgreSQL...\n")
					// Gracefully shut PostgreSQL down!
					if err := closeFunc(); err != nil {
						cmd.Printf("Failed to stop built-in PostgreSQL: %v\n", err)
					} else {
						cmd.Printf("Stopped built-in PostgreSQL\n")
					}
				}()
			}

			var (
				httpListener net.Listener
				httpURL      *url.URL
			)
			if cfg.HTTPAddress.Value != "" {
				httpListener, err = net.Listen("tcp", cfg.HTTPAddress.Value)
				if err != nil {
					return xerrors.Errorf("listen %q: %w", cfg.HTTPAddress.Value, err)
				}
				defer httpListener.Close()

				listenAddrStr := httpListener.Addr().String()
				// For some reason if 0.0.0.0:x is provided as the http address,
				// httpListener.Addr().String() likes to return it as an ipv6
				// address (i.e. [::]:x). If the input ip is 0.0.0.0, try to
				// coerce the output back to ipv4 to make it less confusing.
				if strings.Contains(cfg.HTTPAddress.Value, "0.0.0.0") {
					listenAddrStr = strings.ReplaceAll(listenAddrStr, "[::]", "0.0.0.0")
				}

				// We want to print out the address the user supplied, not the
				// loopback device.
				cmd.Println("Started HTTP listener at", (&url.URL{Scheme: "http", Host: listenAddrStr}).String())

				// Set the http URL we want to use when connecting to ourselves.
				tcpAddr, tcpAddrValid := httpListener.Addr().(*net.TCPAddr)
				if !tcpAddrValid {
					return xerrors.Errorf("invalid TCP address type %T", httpListener.Addr())
				}
				if tcpAddr.IP.IsUnspecified() {
					tcpAddr.IP = net.IPv4(127, 0, 0, 1)
				}
				httpURL = &url.URL{
					Scheme: "http",
					Host:   tcpAddr.String(),
				}
			}

			var (
				tlsConfig     *tls.Config
				httpsListener net.Listener
				httpsURL      *url.URL
			)
			if cfg.TLS.Enable.Value {
				if cfg.TLS.Address.Value == "" {
					return xerrors.New("tls address must be set if tls is enabled")
				}

				// DEPRECATED: This redirect used to default to true.
				// It made more sense to have the redirect be opt-in.
				if os.Getenv("CODER_TLS_REDIRECT_HTTP") == "true" || cmd.Flags().Changed("tls-redirect-http-to-https") {
					cmd.PrintErr(cliui.Styles.Warn.Render("WARN:") + " --tls-redirect-http-to-https is deprecated, please use --redirect-to-access-url instead\n")
					cfg.RedirectToAccessURL.Value = cfg.TLS.RedirectHTTP.Value
				}

				tlsConfig, err = configureTLS(
					cfg.TLS.MinVersion.Value,
					cfg.TLS.ClientAuth.Value,
					cfg.TLS.CertFiles.Value,
					cfg.TLS.KeyFiles.Value,
					cfg.TLS.ClientCAFile.Value,
				)
				if err != nil {
					return xerrors.Errorf("configure tls: %w", err)
				}
				httpsListenerInner, err := net.Listen("tcp", cfg.TLS.Address.Value)
				if err != nil {
					return xerrors.Errorf("listen %q: %w", cfg.TLS.Address.Value, err)
				}
				defer httpsListenerInner.Close()

				httpsListener = tls.NewListener(httpsListenerInner, tlsConfig)
				defer httpsListener.Close()

				listenAddrStr := httpsListener.Addr().String()
				// For some reason if 0.0.0.0:x is provided as the https
				// address, httpsListener.Addr().String() likes to return it as
				// an ipv6 address (i.e. [::]:x). If the input ip is 0.0.0.0,
				// try to coerce the output back to ipv4 to make it less
				// confusing.
				if strings.Contains(cfg.HTTPAddress.Value, "0.0.0.0") {
					listenAddrStr = strings.ReplaceAll(listenAddrStr, "[::]", "0.0.0.0")
				}

				// We want to print out the address the user supplied, not the
				// loopback device.
				cmd.Println("Started TLS/HTTPS listener at", (&url.URL{Scheme: "https", Host: listenAddrStr}).String())

				// Set the https URL we want to use when connecting to
				// ourselves.
				tcpAddr, tcpAddrValid := httpsListener.Addr().(*net.TCPAddr)
				if !tcpAddrValid {
					return xerrors.Errorf("invalid TCP address type %T", httpsListener.Addr())
				}
				if tcpAddr.IP.IsUnspecified() {
					tcpAddr.IP = net.IPv4(127, 0, 0, 1)
				}
				httpsURL = &url.URL{
					Scheme: "https",
					Host:   tcpAddr.String(),
				}
			}

			// Sanity check that at least one listener was started.
			if httpListener == nil && httpsListener == nil {
				return xerrors.New("must listen on at least one address")
			}

			// Prefer HTTP because it's less prone to TLS errors over localhost.
			localURL := httpsURL
			if httpURL != nil {
				localURL = httpURL
			}

			ctx, httpClient, err := configureHTTPClient(
				ctx,
				cfg.TLS.ClientCertFile.Value,
				cfg.TLS.ClientKeyFile.Value,
				cfg.TLS.ClientCAFile.Value,
			)
			if err != nil {
				return xerrors.Errorf("configure http client: %w", err)
			}

			var (
				ctxTunnel, closeTunnel = context.WithCancel(ctx)
				tunnel                 *devtunnel.Tunnel
				tunnelErr              <-chan error
			)
			defer closeTunnel()

			// If the access URL is empty, we attempt to run a reverse-proxy
			// tunnel to make the initial setup really simple.
			if cfg.AccessURL.Value == "" {
				cmd.Printf("Opening tunnel so workspaces can connect to your deployment. For production scenarios, specify an external access URL\n")
				tunnel, tunnelErr, err = devtunnel.New(ctxTunnel, logger.Named("devtunnel"))
				if err != nil {
					return xerrors.Errorf("create tunnel: %w", err)
				}
				cfg.AccessURL.Value = tunnel.URL

				if cfg.WildcardAccessURL.Value == "" {
					u, err := parseURL(tunnel.URL)
					if err != nil {
						return xerrors.Errorf("parse tunnel url: %w", err)
					}

					// Suffixed wildcard access URL.
					cfg.WildcardAccessURL.Value = fmt.Sprintf("*--%s", u.Hostname())
				}
			}

			accessURLParsed, err := parseURL(cfg.AccessURL.Value)
			if err != nil {
				return xerrors.Errorf("parse URL: %w", err)
			}
			accessURLPortRaw := accessURLParsed.Port()
			if accessURLPortRaw == "" {
				accessURLPortRaw = "80"
				if accessURLParsed.Scheme == "https" {
					accessURLPortRaw = "443"
				}
			}
			accessURLPort, err := strconv.Atoi(accessURLPortRaw)
			if err != nil {
				return xerrors.Errorf("parse access URL port: %w", err)
			}

			// Warn the user if the access URL appears to be a loopback address.
			isLocal, err := isLocalURL(ctx, accessURLParsed)
			if isLocal || err != nil {
				reason := "could not be resolved"
				if isLocal {
					reason = "isn't externally reachable"
				}
				cmd.Printf("%s The access URL %s %s, this may cause unexpected problems when creating workspaces. Generate a unique *.try.coder.app URL by not specifying an access URL.\n", cliui.Styles.Warn.Render("Warning:"), cliui.Styles.Field.Render(accessURLParsed.String()), reason)
			}

			// A newline is added before for visibility in terminal output.
			cmd.Printf("\nView the Web UI: %s\n", accessURLParsed.String())

			// Used for zero-trust instance identity with Google Cloud.
			googleTokenValidator, err := idtoken.NewValidator(ctx, option.WithoutAuthentication())
			if err != nil {
				return err
			}

			sshKeygenAlgorithm, err := gitsshkey.ParseAlgorithm(cfg.SSHKeygenAlgorithm.Value)
			if err != nil {
				return xerrors.Errorf("parse ssh keygen algorithm %s: %w", cfg.SSHKeygenAlgorithm.Value, err)
			}

			defaultRegion := &tailcfg.DERPRegion{
				EmbeddedRelay: true,
				RegionID:      cfg.DERP.Server.RegionID.Value,
				RegionCode:    cfg.DERP.Server.RegionCode.Value,
				RegionName:    cfg.DERP.Server.RegionName.Value,
				Nodes: []*tailcfg.DERPNode{{
					Name:      fmt.Sprintf("%db", cfg.DERP.Server.RegionID.Value),
					RegionID:  cfg.DERP.Server.RegionID.Value,
					HostName:  accessURLParsed.Hostname(),
					DERPPort:  accessURLPort,
					STUNPort:  -1,
					ForceHTTP: accessURLParsed.Scheme == "http",
				}},
			}
			if !cfg.DERP.Server.Enable.Value {
				defaultRegion = nil
			}
			derpMap, err := tailnet.NewDERPMap(ctx, defaultRegion, cfg.DERP.Server.STUNAddresses.Value, cfg.DERP.Config.URL.Value, cfg.DERP.Config.Path.Value)
			if err != nil {
				return xerrors.Errorf("create derp map: %w", err)
			}

			appHostname := strings.TrimSpace(cfg.WildcardAccessURL.Value)
			var appHostnameRegex *regexp.Regexp
			if appHostname != "" {
				appHostnameRegex, err = httpapi.CompileHostnamePattern(appHostname)
				if err != nil {
					return xerrors.Errorf("parse wildcard access URL %q: %w", appHostname, err)
				}
			}

			gitAuthConfigs, err := gitauth.ConvertConfig(cfg.GitAuth.Value, accessURLParsed)
			if err != nil {
				return xerrors.Errorf("parse git auth config: %w", err)
			}

			realIPConfig, err := httpmw.ParseRealIPConfig(cfg.ProxyTrustedHeaders.Value, cfg.ProxyTrustedOrigins.Value)
			if err != nil {
				return xerrors.Errorf("parse real ip config: %w", err)
			}

			options := &coderd.Options{
				AccessURL:                   accessURLParsed,
				AppHostname:                 appHostname,
				AppHostnameRegex:            appHostnameRegex,
				Logger:                      logger.Named("coderd"),
				Database:                    dbfake.New(),
				DERPMap:                     derpMap,
				Pubsub:                      database.NewPubsubInMemory(),
				CacheDir:                    cacheDir,
				GoogleTokenValidator:        googleTokenValidator,
				GitAuthConfigs:              gitAuthConfigs,
				RealIPConfig:                realIPConfig,
				SecureAuthCookie:            cfg.SecureAuthCookie.Value,
				SSHKeygenAlgorithm:          sshKeygenAlgorithm,
				TracerProvider:              tracerProvider,
				Telemetry:                   telemetry.NewNoop(),
				MetricsCacheRefreshInterval: cfg.MetricsCacheRefreshInterval.Value,
				AgentStatsRefreshInterval:   cfg.AgentStatRefreshInterval.Value,
				DeploymentConfig:            cfg,
				PrometheusRegistry:          prometheus.NewRegistry(),
				APIRateLimit:                cfg.RateLimit.API.Value,
				LoginRateLimit:              loginRateLimit,
				FilesRateLimit:              filesRateLimit,
				HTTPClient:                  httpClient,
			}
			if tlsConfig != nil {
				options.TLSCertificates = tlsConfig.Certificates
			}

			if cfg.StrictTransportSecurity.Value > 0 {
				options.StrictTransportSecurityCfg, err = httpmw.HSTSConfigOptions(cfg.StrictTransportSecurity.Value, cfg.StrictTransportSecurityOptions.Value)
				if err != nil {
					return xerrors.Errorf("coderd: setting hsts header failed (options: %v): %w", cfg.StrictTransportSecurityOptions.Value, err)
				}
			}

			if cfg.UpdateCheck.Value {
				options.UpdateCheckOptions = &updatecheck.Options{
					// Avoid spamming GitHub API checking for updates.
					Interval: 24 * time.Hour,
					// Inform server admins of new versions.
					Notify: func(r updatecheck.Result) {
						if semver.Compare(r.Version, buildinfo.Version()) > 0 {
							options.Logger.Info(
								context.Background(),
								"new version of coder available",
								slog.F("new_version", r.Version),
								slog.F("url", r.URL),
								slog.F("upgrade_instructions", "https://coder.com/docs/coder-oss/latest/admin/upgrade"),
							)
						}
					},
				}
			}

			if cfg.OAuth2.Github.ClientSecret.Value != "" {
				options.GithubOAuth2Config, err = configureGithubOAuth2(accessURLParsed,
					cfg.OAuth2.Github.ClientID.Value,
					cfg.OAuth2.Github.ClientSecret.Value,
					cfg.OAuth2.Github.AllowSignups.Value,
					cfg.OAuth2.Github.AllowEveryone.Value,
					cfg.OAuth2.Github.AllowedOrgs.Value,
					cfg.OAuth2.Github.AllowedTeams.Value,
					cfg.OAuth2.Github.EnterpriseBaseURL.Value,
				)
				if err != nil {
					return xerrors.Errorf("configure github oauth2: %w", err)
				}
			}

			if cfg.OIDC.ClientSecret.Value != "" {
				if cfg.OIDC.ClientID.Value == "" {
					return xerrors.Errorf("OIDC client ID be set!")
				}
				if cfg.OIDC.IssuerURL.Value == "" {
					return xerrors.Errorf("OIDC issuer URL must be set!")
				}

				if cfg.OIDC.IgnoreEmailVerified.Value {
					logger.Warn(ctx, "coder will not check email_verified for OIDC logins")
				}

				oidcProvider, err := oidc.NewProvider(ctx, cfg.OIDC.IssuerURL.Value)
				if err != nil {
					return xerrors.Errorf("configure oidc provider: %w", err)
				}
				redirectURL, err := accessURLParsed.Parse("/api/v2/users/oidc/callback")
				if err != nil {
					return xerrors.Errorf("parse oidc oauth callback url: %w", err)
				}
				options.OIDCConfig = &coderd.OIDCConfig{
					OAuth2Config: &oauth2.Config{
						ClientID:     cfg.OIDC.ClientID.Value,
						ClientSecret: cfg.OIDC.ClientSecret.Value,
						RedirectURL:  redirectURL.String(),
						Endpoint:     oidcProvider.Endpoint(),
						Scopes:       cfg.OIDC.Scopes.Value,
					},
					Provider: oidcProvider,
					Verifier: oidcProvider.Verifier(&oidc.Config{
						ClientID: cfg.OIDC.ClientID.Value,
					}),
					EmailDomain:         cfg.OIDC.EmailDomain.Value,
					AllowSignups:        cfg.OIDC.AllowSignups.Value,
					UsernameField:       cfg.OIDC.UsernameField.Value,
					SignInText:          cfg.OIDC.SignInText.Value,
					IconURL:             cfg.OIDC.IconURL.Value,
					IgnoreEmailVerified: cfg.OIDC.IgnoreEmailVerified.Value,
				}
			}

			if cfg.InMemoryDatabase.Value {
				options.Database = dbfake.New()
				options.Pubsub = database.NewPubsubInMemory()
			} else {
				sqlDB, err := connectToPostgres(ctx, logger, sqlDriver, cfg.PostgresURL.Value)
				if err != nil {
					return xerrors.Errorf("connect to postgres: %w", err)
				}
				defer func() {
					_ = sqlDB.Close()
				}()

				options.Database = database.New(sqlDB)
				options.Pubsub, err = database.NewPubsub(ctx, sqlDB, cfg.PostgresURL.Value)
				if err != nil {
					return xerrors.Errorf("create pubsub: %w", err)
				}
				defer options.Pubsub.Close()
			}

			deploymentID, err := options.Database.GetDeploymentID(ctx)
			if errors.Is(err, sql.ErrNoRows) {
				err = nil
			}
			if err != nil {
				return xerrors.Errorf("get deployment id: %w", err)
			}
			if deploymentID == "" {
				deploymentID = uuid.NewString()
				err = options.Database.InsertDeploymentID(ctx, deploymentID)
				if err != nil {
					return xerrors.Errorf("set deployment id: %w", err)
				}
			}

			// Disable telemetry if the in-memory database is used unless explicitly defined!
			if cfg.InMemoryDatabase.Value && !cmd.Flags().Changed(cfg.Telemetry.Enable.Flag) {
				cfg.Telemetry.Enable.Value = false
			}
			if cfg.Telemetry.Enable.Value {
				// Parse the raw telemetry URL!
				telemetryURL, err := parseURL(cfg.Telemetry.URL.Value)
				if err != nil {
					return xerrors.Errorf("parse telemetry url: %w", err)
				}

				gitAuth := make([]telemetry.GitAuth, 0)
				for _, cfg := range gitAuthConfigs {
					gitAuth = append(gitAuth, telemetry.GitAuth{
						Type: string(cfg.Type),
					})
				}

				options.Telemetry, err = telemetry.New(telemetry.Options{
					BuiltinPostgres:    builtinPostgres,
					DeploymentID:       deploymentID,
					Database:           options.Database,
					Logger:             logger.Named("telemetry"),
					URL:                telemetryURL,
					Wildcard:           cfg.WildcardAccessURL.Value != "",
					DERPServerRelayURL: cfg.DERP.Server.RelayURL.Value,
					GitAuth:            gitAuth,
					GitHubOAuth:        cfg.OAuth2.Github.ClientID.Value != "",
					OIDCAuth:           cfg.OIDC.ClientID.Value != "",
					OIDCIssuerURL:      cfg.OIDC.IssuerURL.Value,
					Prometheus:         cfg.Prometheus.Enable.Value,
					STUN:               len(cfg.DERP.Server.STUNAddresses.Value) != 0,
					Tunnel:             tunnel != nil,
				})
				if err != nil {
					return xerrors.Errorf("create telemetry reporter: %w", err)
				}
				defer options.Telemetry.Close()
			}

			// This prevents the pprof import from being accidentally deleted.
			_ = pprof.Handler
			if cfg.Pprof.Enable.Value {
				//nolint:revive
				defer serveHandler(ctx, logger, nil, cfg.Pprof.Address.Value, "pprof")()
			}
			if cfg.Prometheus.Enable.Value {
				options.PrometheusRegistry.MustRegister(collectors.NewGoCollector())
				options.PrometheusRegistry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

				closeUsersFunc, err := prometheusmetrics.ActiveUsers(ctx, options.PrometheusRegistry, options.Database, 0)
				if err != nil {
					return xerrors.Errorf("register active users prometheus metric: %w", err)
				}
				defer closeUsersFunc()

				closeWorkspacesFunc, err := prometheusmetrics.Workspaces(ctx, options.PrometheusRegistry, options.Database, 0)
				if err != nil {
					return xerrors.Errorf("register workspaces prometheus metric: %w", err)
				}
				defer closeWorkspacesFunc()

				//nolint:revive
				defer serveHandler(ctx, logger, promhttp.InstrumentMetricHandler(
					options.PrometheusRegistry, promhttp.HandlerFor(options.PrometheusRegistry, promhttp.HandlerOpts{}),
				), cfg.Prometheus.Address.Value, "prometheus")()
			}

			if cfg.Swagger.Enable.Value {
				options.SwaggerEndpoint = cfg.Swagger.Enable.Value
			}

			// TODO(cian): make this configurable.
			healthPoller := time.NewTicker(10 * time.Second)
			defer healthPoller.Stop()
			healthChecker := health.NewChecker(healthPoller.C)
			defer healthChecker.Stop()
			healthChecker.Add("access-url", health.AccessURLAccessible(cfg.AccessURL.Value, time.Second))
			options.Healthchecker = healthChecker

			// We use a separate coderAPICloser so the Enterprise API
			// can have it's own close functions. This is cleaner
			// than abstracting the Coder API itself.
			coderAPI, coderAPICloser, err := newAPI(ctx, options)
			if err != nil {
				return err
			}

			client := codersdk.New(localURL)
			if localURL.Scheme == "https" && isLocalhost(localURL.Hostname()) {
				// The certificate will likely be self-signed or for a different
				// hostname, so we need to skip verification.
				client.HTTPClient.Transport = &http.Transport{
					TLSClientConfig: &tls.Config{
						//nolint:gosec
						InsecureSkipVerify: true,
					},
				}
			}
			defer client.HTTPClient.CloseIdleConnections()

			// This is helpful for tests, but can be silently ignored.
			// Coder may be ran as users that don't have permission to write in the homedir,
			// such as via the systemd service.
			_ = config.URL().Write(client.URL.String())

			// Since errCh only has one buffered slot, all routines
			// sending on it must be wrapped in a select/default to
			// avoid leaving dangling goroutines waiting for the
			// channel to be consumed.
			errCh := make(chan error, 1)
			provisionerDaemons := make([]*provisionerd.Server, 0)
			defer func() {
				// We have no graceful shutdown of provisionerDaemons
				// here because that's handled at the end of main, this
				// is here in case the program exits early.
				for _, daemon := range provisionerDaemons {
					_ = daemon.Close()
				}
			}()
			provisionerdMetrics := provisionerd.NewMetrics(options.PrometheusRegistry)
			for i := 0; i < cfg.Provisioner.Daemons.Value; i++ {
				daemonCacheDir := filepath.Join(cacheDir, fmt.Sprintf("provisioner-%d", i))
				daemon, err := newProvisionerDaemon(ctx, coderAPI, provisionerdMetrics, logger, cfg, daemonCacheDir, errCh, false)
				if err != nil {
					return xerrors.Errorf("create provisioner daemon: %w", err)
				}
				provisionerDaemons = append(provisionerDaemons, daemon)
			}

			shutdownConnsCtx, shutdownConns := context.WithCancel(ctx)
			defer shutdownConns()

			// Wrap the server in middleware that redirects to the access URL if
			// the request is not to a local IP.
			var handler http.Handler = coderAPI.RootHandler
			if cfg.RedirectToAccessURL.Value {
				handler = redirectToAccessURL(handler, accessURLParsed, tunnel != nil, appHostnameRegex)
			}

			// ReadHeaderTimeout is purposefully not enabled. It caused some
			// issues with websockets over the dev tunnel.
			// See: https://github.com/coder/coder/pull/3730
			//nolint:gosec
			httpServer := &http.Server{
				// These errors are typically noise like "TLS: EOF". Vault does
				// similar:
				// https://github.com/hashicorp/vault/blob/e2490059d0711635e529a4efcbaa1b26998d6e1c/command/server.go#L2714
				ErrorLog: log.New(io.Discard, "", 0),
				Handler:  handler,
				BaseContext: func(_ net.Listener) context.Context {
					return shutdownConnsCtx
				},
			}
			defer func() {
				_ = shutdownWithTimeout(httpServer.Shutdown, 5*time.Second)
			}()

			// We call this in the routine so we can kill the other listeners if
			// one of them fails.
			closeListenersNow := func() {
				if httpListener != nil {
					_ = httpListener.Close()
				}
				if httpsListener != nil {
					_ = httpsListener.Close()
				}
				if tunnel != nil {
					_ = tunnel.Listener.Close()
				}
			}

			eg := errgroup.Group{}
			if httpListener != nil {
				eg.Go(func() error {
					defer closeListenersNow()
					return httpServer.Serve(httpListener)
				})
			}
			if httpsListener != nil {
				eg.Go(func() error {
					defer closeListenersNow()
					return httpServer.Serve(httpsListener)
				})
			}
			if tunnel != nil {
				eg.Go(func() error {
					defer closeListenersNow()
					return httpServer.Serve(tunnel.Listener)
				})
			}

			go func() {
				select {
				case errCh <- eg.Wait():
				default:
				}
			}()

			hasFirstUser, err := client.HasFirstUser(ctx)
			if err != nil {
				cmd.Println("\nFailed to check for the first user: " + err.Error())
			} else if !hasFirstUser {
				cmd.Println("\nGet started by creating the first user (in a new terminal):")
				cmd.Println(cliui.Styles.Code.Render("coder login " + accessURLParsed.String()))
			}

			cmd.Println("\n==> Logs will stream in below (press ctrl+c to gracefully exit):")

			// Updates the systemd status from activating to activated.
			_, err = daemon.SdNotify(false, daemon.SdNotifyReady)
			if err != nil {
				return xerrors.Errorf("notify systemd: %w", err)
			}

			autobuildPoller := time.NewTicker(cfg.AutobuildPollInterval.Value)
			defer autobuildPoller.Stop()
			autobuildExecutor := executor.New(ctx, options.Database, logger, autobuildPoller.C)
			autobuildExecutor.Run()

			// Currently there is no way to ask the server to shut
			// itself down, so any exit signal will result in a non-zero
			// exit of the server.
			var exitErr error
			select {
			case <-notifyCtx.Done():
				exitErr = notifyCtx.Err()
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), cliui.Styles.Bold.Render(
					"Interrupt caught, gracefully exiting. Use ctrl+\\ to force quit",
				))
			case exitErr = <-tunnelErr:
				if exitErr == nil {
					exitErr = xerrors.New("dev tunnel closed unexpectedly")
				}
			case exitErr = <-errCh:
			}
			if exitErr != nil && !xerrors.Is(exitErr, context.Canceled) {
				cmd.Printf("Unexpected error, shutting down server: %s\n", exitErr)
			}

			// Begin clean shut down stage, we try to shut down services
			// gracefully in an order that gives the best experience.
			// This procedure should not differ greatly from the order
			// of `defer`s in this function, but allows us to inform
			// the user about what's going on and handle errors more
			// explicitly.

			_, err = daemon.SdNotify(false, daemon.SdNotifyStopping)
			if err != nil {
				cmd.Printf("Notify systemd failed: %s", err)
			}

			// Stop accepting new connections without interrupting
			// in-flight requests, give in-flight requests 5 seconds to
			// complete.
			cmd.Println("Shutting down API server...")
			err = shutdownWithTimeout(httpServer.Shutdown, 3*time.Second)
			if err != nil {
				cmd.Printf("API server shutdown took longer than 3s: %s\n", err)
			} else {
				cmd.Printf("Gracefully shut down API server\n")
			}
			// Cancel any remaining in-flight requests.
			shutdownConns()

			// Shut down provisioners before waiting for WebSockets
			// connections to close.
			var wg sync.WaitGroup
			for i, provisionerDaemon := range provisionerDaemons {
				id := i + 1
				provisionerDaemon := provisionerDaemon
				wg.Add(1)
				go func() {
					defer wg.Done()

					if ok, _ := cmd.Flags().GetBool(varVerbose); ok {
						cmd.Printf("Shutting down provisioner daemon %d...\n", id)
					}
					err := shutdownWithTimeout(provisionerDaemon.Shutdown, 5*time.Second)
					if err != nil {
						cmd.PrintErrf("Failed to shutdown provisioner daemon %d: %s\n", id, err)
						return
					}
					err = provisionerDaemon.Close()
					if err != nil {
						cmd.PrintErrf("Close provisioner daemon %d: %s\n", id, err)
						return
					}
					if ok, _ := cmd.Flags().GetBool(varVerbose); ok {
						cmd.Printf("Gracefully shut down provisioner daemon %d\n", id)
					}
				}()
			}
			wg.Wait()

			cmd.Println("Waiting for WebSocket connections to close...")
			_ = coderAPICloser.Close()
			cmd.Println("Done waiting for WebSocket connections")

			// Close tunnel after we no longer have in-flight connections.
			if tunnel != nil {
				cmd.Println("Waiting for tunnel to close...")
				closeTunnel()
				<-tunnelErr
				cmd.Println("Done waiting for tunnel")
			}

			// Ensures a last report can be sent before exit!
			options.Telemetry.Close()

			// Trigger context cancellation for any remaining services.
			cancel()

			if xerrors.Is(exitErr, context.Canceled) {
				return nil
			}
			return exitErr
		},
	}

	var pgRawURL bool
	postgresBuiltinURLCmd := &cobra.Command{
		Use:   "postgres-builtin-url",
		Short: "Output the connection URL for the built-in PostgreSQL deployment.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg := createConfig(cmd)
			url, err := embeddedPostgresURL(cfg)
			if err != nil {
				return err
			}
			if pgRawURL {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", url)
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", cliui.Styles.Code.Render(fmt.Sprintf("psql %q", url)))
			}
			return nil
		},
	}
	postgresBuiltinServeCmd := &cobra.Command{
		Use:   "postgres-builtin-serve",
		Short: "Run the built-in PostgreSQL deployment.",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			cfg := createConfig(cmd)
			logger := slog.Make(sloghuman.Sink(cmd.ErrOrStderr()))
			if ok, _ := cmd.Flags().GetBool(varVerbose); ok {
				logger = logger.Leveled(slog.LevelDebug)
			}

			ctx, cancel := signal.NotifyContext(ctx, InterruptSignals...)
			defer cancel()

			url, closePg, err := startBuiltinPostgres(ctx, cfg, logger)
			if err != nil {
				return err
			}
			defer func() { _ = closePg() }()

			if pgRawURL {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", url)
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", cliui.Styles.Code.Render(fmt.Sprintf("psql %q", url)))
			}

			<-ctx.Done()
			return nil
		},
	}
	postgresBuiltinURLCmd.Flags().BoolVar(&pgRawURL, "raw-url", false, "Output the raw connection URL instead of a psql command.")
	postgresBuiltinServeCmd.Flags().BoolVar(&pgRawURL, "raw-url", false, "Output the raw connection URL instead of a psql command.")

	createAdminUserCommand := newCreateAdminUserCommand()
	root.AddCommand(postgresBuiltinURLCmd, postgresBuiltinServeCmd, createAdminUserCommand)

	deployment.AttachFlags(root.Flags(), vip, false)

	return root
}

// parseURL parses a string into a URL.
func parseURL(u string) (*url.URL, error) {
	hasScheme := strings.HasPrefix(u, "http:") || strings.HasPrefix(u, "https:")

	if !hasScheme {
		return nil, xerrors.Errorf("URL %q must have a scheme of either http or https", u)
	}

	parsed, err := url.Parse(u)
	if err != nil {
		return nil, err
	}

	return parsed, nil
}

// isLocalURL returns true if the hostname of the provided URL appears to
// resolve to a loopback address.
func isLocalURL(ctx context.Context, u *url.URL) (bool, error) {
	resolver := &net.Resolver{}
	ips, err := resolver.LookupIPAddr(ctx, u.Hostname())
	if err != nil {
		return false, err
	}

	for _, ip := range ips {
		if ip.IP.IsLoopback() {
			return true, nil
		}
	}
	return false, nil
}

func shutdownWithTimeout(shutdown func(context.Context) error, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return shutdown(ctx)
}

// nolint:revive
func newProvisionerDaemon(
	ctx context.Context,
	coderAPI *coderd.API,
	metrics provisionerd.Metrics,
	logger slog.Logger,
	cfg *codersdk.DeploymentConfig,
	cacheDir string,
	errCh chan error,
	dev bool,
) (srv *provisionerd.Server, err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer func() {
		if err != nil {
			cancel()
		}
	}()

	err = os.MkdirAll(cacheDir, 0o700)
	if err != nil {
		return nil, xerrors.Errorf("mkdir %q: %w", cacheDir, err)
	}

	terraformClient, terraformServer := provisionersdk.MemTransportPipe()
	go func() {
		<-ctx.Done()
		_ = terraformClient.Close()
		_ = terraformServer.Close()
	}()
	go func() {
		defer cancel()

		err := terraform.Serve(ctx, &terraform.ServeOptions{
			ServeOptions: &provisionersdk.ServeOptions{
				Listener: terraformServer,
			},
			CachePath: cacheDir,
			Logger:    logger,
		})
		if err != nil && !xerrors.Is(err, context.Canceled) {
			select {
			case errCh <- err:
			default:
			}
		}
	}()

	tempDir, err := os.MkdirTemp("", "provisionerd")
	if err != nil {
		return nil, err
	}

	provisioners := provisionerd.Provisioners{
		string(database.ProvisionerTypeTerraform): sdkproto.NewDRPCProvisionerClient(terraformClient),
	}
	// include echo provisioner when in dev mode
	if dev {
		echoClient, echoServer := provisionersdk.MemTransportPipe()
		go func() {
			<-ctx.Done()
			_ = echoClient.Close()
			_ = echoServer.Close()
		}()
		go func() {
			defer cancel()

			err := echo.Serve(ctx, afero.NewOsFs(), &provisionersdk.ServeOptions{Listener: echoServer})
			if err != nil {
				select {
				case errCh <- err:
				default:
				}
			}
		}()
		provisioners[string(database.ProvisionerTypeEcho)] = sdkproto.NewDRPCProvisionerClient(echoClient)
	}
	debounce := time.Second
	return provisionerd.New(func(ctx context.Context) (proto.DRPCProvisionerDaemonClient, error) {
		// This debounces calls to listen every second. Read the comment
		// in provisionerdserver.go to learn more!
		return coderAPI.CreateInMemoryProvisionerDaemon(ctx, debounce)
	}, &provisionerd.Options{
		Logger:              logger,
		JobPollInterval:     cfg.Provisioner.DaemonPollInterval.Value,
		JobPollJitter:       cfg.Provisioner.DaemonPollJitter.Value,
		JobPollDebounce:     debounce,
		UpdateInterval:      500 * time.Millisecond,
		ForceCancelInterval: cfg.Provisioner.ForceCancelInterval.Value,
		Provisioners:        provisioners,
		WorkDirectory:       tempDir,
		TracerProvider:      coderAPI.TracerProvider,
		Metrics:             &metrics,
	}), nil
}

// nolint: revive
func printLogo(cmd *cobra.Command) {
	// Only print the logo in TTYs.
	if !isTTYOut(cmd) {
		return
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s - Software development on your infrastucture\n", cliui.Styles.Bold.Render("Coder "+buildinfo.Version()))
}

func loadCertificates(tlsCertFiles, tlsKeyFiles []string) ([]tls.Certificate, error) {
	if len(tlsCertFiles) != len(tlsKeyFiles) {
		return nil, xerrors.New("--tls-cert-file and --tls-key-file must be used the same amount of times")
	}

	certs := make([]tls.Certificate, len(tlsCertFiles))
	for i := range tlsCertFiles {
		certFile, keyFile := tlsCertFiles[i], tlsKeyFiles[i]
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, xerrors.Errorf("load TLS key pair %d (%q, %q): %w", i, certFile, keyFile, err)
		}

		certs[i] = cert
	}

	return certs, nil
}

// generateSelfSignedCertificate creates an unsafe self-signed certificate
// at random that allows users to proceed with setup in the event they
// haven't configured any TLS certificates.
func generateSelfSignedCertificate() (*tls.Certificate, error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour * 24 * 180),

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, err
	}

	var cert tls.Certificate
	cert.Certificate = append(cert.Certificate, derBytes)
	cert.PrivateKey = privateKey
	return &cert, nil
}

func configureTLS(tlsMinVersion, tlsClientAuth string, tlsCertFiles, tlsKeyFiles []string, tlsClientCAFile string) (*tls.Config, error) {
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}
	switch tlsMinVersion {
	case "tls10":
		tlsConfig.MinVersion = tls.VersionTLS10
	case "tls11":
		tlsConfig.MinVersion = tls.VersionTLS11
	case "tls12":
		tlsConfig.MinVersion = tls.VersionTLS12
	case "tls13":
		tlsConfig.MinVersion = tls.VersionTLS13
	default:
		return nil, xerrors.Errorf("unrecognized tls version: %q", tlsMinVersion)
	}

	switch tlsClientAuth {
	case "none":
		tlsConfig.ClientAuth = tls.NoClientCert
	case "request":
		tlsConfig.ClientAuth = tls.RequestClientCert
	case "require-any":
		tlsConfig.ClientAuth = tls.RequireAnyClientCert
	case "verify-if-given":
		tlsConfig.ClientAuth = tls.VerifyClientCertIfGiven
	case "require-and-verify":
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	default:
		return nil, xerrors.Errorf("unrecognized tls client auth: %q", tlsClientAuth)
	}

	certs, err := loadCertificates(tlsCertFiles, tlsKeyFiles)
	if err != nil {
		return nil, xerrors.Errorf("load certificates: %w", err)
	}
	if len(certs) == 0 {
		selfSignedCertificate, err := generateSelfSignedCertificate()
		if err != nil {
			return nil, xerrors.Errorf("generate self signed certificate: %w", err)
		}
		certs = append(certs, *selfSignedCertificate)
	}

	tlsConfig.Certificates = certs
	tlsConfig.GetCertificate = func(hi *tls.ClientHelloInfo) (*tls.Certificate, error) {
		// If there's only one certificate, return it.
		if len(certs) == 1 {
			return &certs[0], nil
		}

		// Expensively check which certificate matches the client hello.
		for _, cert := range certs {
			cert := cert
			if err := hi.SupportsCertificate(&cert); err == nil {
				return &cert, nil
			}
		}

		// Return the first certificate if we have one, or return nil so the
		// server doesn't fail.
		if len(certs) > 0 {
			return &certs[0], nil
		}
		return nil, nil //nolint:nilnil
	}

	err = configureCAPool(tlsClientCAFile, tlsConfig)
	if err != nil {
		return nil, err
	}

	return tlsConfig, nil
}

func configureCAPool(tlsClientCAFile string, tlsConfig *tls.Config) error {
	if tlsClientCAFile != "" {
		caPool := x509.NewCertPool()
		data, err := os.ReadFile(tlsClientCAFile)
		if err != nil {
			return xerrors.Errorf("read %q: %w", tlsClientCAFile, err)
		}
		if !caPool.AppendCertsFromPEM(data) {
			return xerrors.Errorf("failed to parse CA certificate in tls-client-ca-file")
		}
		tlsConfig.ClientCAs = caPool
	}
	return nil
}

//nolint:revive // Ignore flag-parameter: parameter 'allowEveryone' seems to be a control flag, avoid control coupling (revive)
func configureGithubOAuth2(accessURL *url.URL, clientID, clientSecret string, allowSignups, allowEveryone bool, allowOrgs []string, rawTeams []string, enterpriseBaseURL string) (*coderd.GithubOAuth2Config, error) {
	redirectURL, err := accessURL.Parse("/api/v2/users/oauth2/github/callback")
	if err != nil {
		return nil, xerrors.Errorf("parse github oauth callback url: %w", err)
	}
	if allowEveryone && len(allowOrgs) > 0 {
		return nil, xerrors.New("allow everyone and allowed orgs cannot be used together")
	}
	if allowEveryone && len(rawTeams) > 0 {
		return nil, xerrors.New("allow everyone and allowed teams cannot be used together")
	}
	if !allowEveryone && len(allowOrgs) == 0 {
		return nil, xerrors.New("allowed orgs is empty: must specify at least one org or allow everyone")
	}
	allowTeams := make([]coderd.GithubOAuth2Team, 0, len(rawTeams))
	for _, rawTeam := range rawTeams {
		parts := strings.SplitN(rawTeam, "/", 2)
		if len(parts) != 2 {
			return nil, xerrors.Errorf("github team allowlist is formatted incorrectly. got %s; wanted <organization>/<team>", rawTeam)
		}
		allowTeams = append(allowTeams, coderd.GithubOAuth2Team{
			Organization: parts[0],
			Slug:         parts[1],
		})
	}
	createClient := func(client *http.Client) (*github.Client, error) {
		if enterpriseBaseURL != "" {
			return github.NewEnterpriseClient(enterpriseBaseURL, "", client)
		}
		return github.NewClient(client), nil
	}

	endpoint := xgithub.Endpoint
	if enterpriseBaseURL != "" {
		enterpriseURL, err := url.Parse(enterpriseBaseURL)
		if err != nil {
			return nil, xerrors.Errorf("parse enterprise base url: %w", err)
		}
		authURL, err := enterpriseURL.Parse("/login/oauth/authorize")
		if err != nil {
			return nil, xerrors.Errorf("parse enterprise auth url: %w", err)
		}
		tokenURL, err := enterpriseURL.Parse("/login/oauth/access_token")
		if err != nil {
			return nil, xerrors.Errorf("parse enterprise token url: %w", err)
		}
		endpoint = oauth2.Endpoint{
			AuthURL:  authURL.String(),
			TokenURL: tokenURL.String(),
		}
	}

	return &coderd.GithubOAuth2Config{
		OAuth2Config: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Endpoint:     endpoint,
			RedirectURL:  redirectURL.String(),
			Scopes: []string{
				"read:user",
				"read:org",
				"user:email",
			},
		},
		AllowSignups:       allowSignups,
		AllowEveryone:      allowEveryone,
		AllowOrganizations: allowOrgs,
		AllowTeams:         allowTeams,
		AuthenticatedUser: func(ctx context.Context, client *http.Client) (*github.User, error) {
			api, err := createClient(client)
			if err != nil {
				return nil, err
			}
			user, _, err := api.Users.Get(ctx, "")
			return user, err
		},
		ListEmails: func(ctx context.Context, client *http.Client) ([]*github.UserEmail, error) {
			api, err := createClient(client)
			if err != nil {
				return nil, err
			}
			emails, _, err := api.Users.ListEmails(ctx, &github.ListOptions{})
			return emails, err
		},
		ListOrganizationMemberships: func(ctx context.Context, client *http.Client) ([]*github.Membership, error) {
			api, err := createClient(client)
			if err != nil {
				return nil, err
			}
			memberships, _, err := api.Organizations.ListOrgMemberships(ctx, &github.ListOrgMembershipsOptions{
				State: "active",
				ListOptions: github.ListOptions{
					PerPage: 100,
				},
			})
			return memberships, err
		},
		TeamMembership: func(ctx context.Context, client *http.Client, org, teamSlug, username string) (*github.Membership, error) {
			api, err := createClient(client)
			if err != nil {
				return nil, err
			}
			team, _, err := api.Teams.GetTeamMembershipBySlug(ctx, org, teamSlug, username)
			return team, err
		},
	}, nil
}

// embeddedPostgresURL returns the URL for the embedded PostgreSQL deployment.
func embeddedPostgresURL(cfg config.Root) (string, error) {
	pgPassword, err := cfg.PostgresPassword().Read()
	if errors.Is(err, os.ErrNotExist) {
		pgPassword, err = cryptorand.String(16)
		if err != nil {
			return "", xerrors.Errorf("generate password: %w", err)
		}
		err = cfg.PostgresPassword().Write(pgPassword)
		if err != nil {
			return "", xerrors.Errorf("write password: %w", err)
		}
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	pgPort, err := cfg.PostgresPort().Read()
	if errors.Is(err, os.ErrNotExist) {
		listener, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			return "", xerrors.Errorf("listen for random port: %w", err)
		}
		_ = listener.Close()
		tcpAddr, valid := listener.Addr().(*net.TCPAddr)
		if !valid {
			return "", xerrors.Errorf("listener returned non TCP addr: %T", tcpAddr)
		}
		pgPort = strconv.Itoa(tcpAddr.Port)
		err = cfg.PostgresPort().Write(pgPort)
		if err != nil {
			return "", xerrors.Errorf("write postgres port: %w", err)
		}
	}
	return fmt.Sprintf("postgres://coder@localhost:%s/coder?sslmode=disable&password=%s", pgPort, pgPassword), nil
}

func startBuiltinPostgres(ctx context.Context, cfg config.Root, logger slog.Logger) (string, func() error, error) {
	usr, err := user.Current()
	if err != nil {
		return "", nil, err
	}
	if usr.Uid == "0" {
		return "", nil, xerrors.New("The built-in PostgreSQL cannot run as the root user. Create a non-root user and run again!")
	}

	// Ensure a password and port have been generated!
	connectionURL, err := embeddedPostgresURL(cfg)
	if err != nil {
		return "", nil, err
	}
	pgPassword, err := cfg.PostgresPassword().Read()
	if err != nil {
		return "", nil, xerrors.Errorf("read postgres password: %w", err)
	}
	pgPortRaw, err := cfg.PostgresPort().Read()
	if err != nil {
		return "", nil, xerrors.Errorf("read postgres port: %w", err)
	}
	pgPort, err := strconv.ParseUint(pgPortRaw, 10, 16)
	if err != nil {
		return "", nil, xerrors.Errorf("parse postgres port: %w", err)
	}

	stdlibLogger := slog.Stdlib(ctx, logger.Named("postgres"), slog.LevelDebug)
	ep := embeddedpostgres.NewDatabase(
		embeddedpostgres.DefaultConfig().
			Version(embeddedpostgres.V13).
			BinariesPath(filepath.Join(cfg.PostgresPath(), "bin")).
			DataPath(filepath.Join(cfg.PostgresPath(), "data")).
			RuntimePath(filepath.Join(cfg.PostgresPath(), "runtime")).
			CachePath(filepath.Join(cfg.PostgresPath(), "cache")).
			Username("coder").
			Password(pgPassword).
			Database("coder").
			Port(uint32(pgPort)).
			Logger(stdlibLogger.Writer()),
	)
	err = ep.Start()
	if err != nil {
		return "", nil, xerrors.Errorf("Failed to start built-in PostgreSQL. Optionally, specify an external deployment with `--postgres-url`: %w", err)
	}
	return connectionURL, ep.Stop, nil
}

func configureHTTPClient(ctx context.Context, clientCertFile, clientKeyFile string, tlsClientCAFile string) (context.Context, *http.Client, error) {
	if clientCertFile != "" && clientKeyFile != "" {
		certificates, err := loadCertificates([]string{clientCertFile}, []string{clientKeyFile})
		if err != nil {
			return ctx, nil, err
		}

		tlsClientConfig := &tls.Config{ //nolint:gosec
			Certificates: certificates,
		}
		err = configureCAPool(tlsClientCAFile, tlsClientConfig)
		if err != nil {
			return nil, nil, err
		}

		httpClient := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: tlsClientConfig,
			},
		}
		return context.WithValue(ctx, oauth2.HTTPClient, httpClient), httpClient, nil
	}
	return ctx, &http.Client{}, nil
}

// nolint:revive
func redirectToAccessURL(handler http.Handler, accessURL *url.URL, tunnel bool, appHostnameRegex *regexp.Regexp) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirect := func() {
			http.Redirect(w, r, accessURL.String(), http.StatusTemporaryRedirect)
		}

		// Only do this if we aren't tunneling.
		// If we are tunneling, we want to allow the request to go through
		// because the tunnel doesn't proxy with TLS.
		if !tunnel && accessURL.Scheme == "https" && r.TLS == nil {
			redirect()
			return
		}

		if r.Host == accessURL.Host {
			handler.ServeHTTP(w, r)
			return
		}

		if appHostnameRegex != nil && appHostnameRegex.MatchString(r.Host) {
			handler.ServeHTTP(w, r)
			return
		}

		redirect()
	})
}

// isLocalhost returns true if the host points to the local machine. Intended to
// be called with `u.Hostname()`.
func isLocalhost(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func buildLogger(cmd *cobra.Command, cfg *codersdk.DeploymentConfig) (slog.Logger, func(), error) {
	var (
		sinks   = []slog.Sink{}
		closers = []func() error{}
	)

	addSinkIfProvided := func(sinkFn func(io.Writer) slog.Sink, loc string) error {
		switch loc {
		case "":

		case "/dev/stdout":
			sinks = append(sinks, sinkFn(cmd.OutOrStdout()))

		case "/dev/stderr":
			sinks = append(sinks, sinkFn(cmd.ErrOrStderr()))

		default:
			fi, err := os.OpenFile(loc, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
			if err != nil {
				return xerrors.Errorf("open log file %q: %w", loc, err)
			}

			closers = append(closers, fi.Close)
			sinks = append(sinks, sinkFn(fi))
		}
		return nil
	}

	err := addSinkIfProvided(sloghuman.Sink, cfg.Logging.Human.Value)
	if err != nil {
		return slog.Logger{}, nil, xerrors.Errorf("add human sink: %w", err)
	}
	err = addSinkIfProvided(slogjson.Sink, cfg.Logging.JSON.Value)
	if err != nil {
		return slog.Logger{}, nil, xerrors.Errorf("add json sink: %w", err)
	}
	err = addSinkIfProvided(slogstackdriver.Sink, cfg.Logging.Stackdriver.Value)
	if err != nil {
		return slog.Logger{}, nil, xerrors.Errorf("add stackdriver sink: %w", err)
	}

	if cfg.Trace.CaptureLogs.Value {
		sinks = append(sinks, tracing.SlogSink{})
	}

	level := slog.LevelInfo
	if ok, _ := cmd.Flags().GetBool(varVerbose); ok {
		level = slog.LevelDebug
	}

	if len(sinks) == 0 {
		return slog.Logger{}, nil, xerrors.New("no loggers provided")
	}

	return slog.Make(sinks...).Leveled(level), func() {
		for _, closer := range closers {
			_ = closer()
		}
	}, nil
}

func connectToPostgres(ctx context.Context, logger slog.Logger, driver string, dbURL string) (*sql.DB, error) {
	logger.Debug(ctx, "connecting to postgresql")
	sqlDB, err := sql.Open(driver, dbURL)
	if err != nil {
		return nil, xerrors.Errorf("dial postgres: %w", err)
	}

	ok := false
	defer func() {
		if !ok {
			_ = sqlDB.Close()
		}
	}()

	pingCtx, pingCancel := context.WithTimeout(ctx, 15*time.Second)
	defer pingCancel()

	err = sqlDB.PingContext(pingCtx)
	if err != nil {
		return nil, xerrors.Errorf("ping postgres: %w", err)
	}

	// Ensure the PostgreSQL version is >=13.0.0!
	version, err := sqlDB.QueryContext(ctx, "SHOW server_version;")
	if err != nil {
		return nil, xerrors.Errorf("get postgres version: %w", err)
	}
	if !version.Next() {
		return nil, xerrors.Errorf("no rows returned for version select")
	}
	var versionStr string
	err = version.Scan(&versionStr)
	if err != nil {
		return nil, xerrors.Errorf("scan version: %w", err)
	}
	_ = version.Close()
	versionStr = strings.Split(versionStr, " ")[0]
	if semver.Compare("v"+versionStr, "v13") < 0 {
		return nil, xerrors.New("PostgreSQL version must be v13.0.0 or higher!")
	}
	logger.Debug(ctx, "connected to postgresql", slog.F("version", versionStr))

	err = migrations.Up(sqlDB)
	if err != nil {
		return nil, xerrors.Errorf("migrate up: %w", err)
	}
	// The default is 0 but the request will fail with a 500 if the DB
	// cannot accept new connections, so we try to limit that here.
	// Requests will wait for a new connection instead of a hard error
	// if a limit is set.
	sqlDB.SetMaxOpenConns(10)
	// Allow a max of 3 idle connections at a time. Lower values end up
	// creating a lot of connection churn. Since each connection uses about
	// 10MB of memory, we're allocating 30MB to Postgres connections per
	// replica, but is better than causing Postgres to spawn a thread 15-20
	// times/sec. PGBouncer's transaction pooling is not the greatest so
	// it's not optimal for us to deploy.
	//
	// This was set to 10 before we started doing HA deployments, but 3 was
	// later determined to be a better middle ground as to not use up all
	// of PGs default connection limit while simultaneously avoiding a lot
	// of connection churn.
	sqlDB.SetMaxIdleConns(3)

	ok = true
	return sqlDB, nil
}

package cli

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log"
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
	"github.com/coder/coder/buildinfo"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/cli/config"
	"github.com/coder/coder/cli/deployment"
	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/autobuild/executor"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/databasefake"
	"github.com/coder/coder/coderd/database/migrations"
	"github.com/coder/coder/coderd/devtunnel"
	"github.com/coder/coder/coderd/gitauth"
	"github.com/coder/coder/coderd/gitsshkey"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/prometheusmetrics"
	"github.com/coder/coder/coderd/telemetry"
	"github.com/coder/coder/coderd/tracing"
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
			cfg, err := deployment.Config(cmd.Flags(), vip)
			if err != nil {
				return xerrors.Errorf("getting deployment config: %w", err)
			}
			printLogo(cmd)
			logger := slog.Make(sloghuman.Sink(cmd.ErrOrStderr()))
			if ok, _ := cmd.Flags().GetBool(varVerbose); ok {
				logger = logger.Leveled(slog.LevelDebug)
			}
			if cfg.Trace.CaptureLogs.Value {
				logger = logger.AppendSinks(tracing.SlogSink{})
			}

			// Main command context for managing cancellation
			// of running services.
			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()

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
			notifyCtx, notifyStop := signal.NotifyContext(ctx, interruptSignals...)
			defer notifyStop()

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
					_ = closeFunc()
					cmd.Printf("Stopped built-in PostgreSQL\n")
				}()
			}

			listener, err := net.Listen("tcp", cfg.Address.Value)
			if err != nil {
				return xerrors.Errorf("listen %q: %w", cfg.Address.Value, err)
			}
			defer listener.Close()

			var tlsConfig *tls.Config
			if cfg.TLS.Enable.Value {
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
				listener = tls.NewListener(listener, tlsConfig)
			}

			tcpAddr, valid := listener.Addr().(*net.TCPAddr)
			if !valid {
				return xerrors.New("must be listening on tcp")
			}
			// If just a port is specified, assume localhost.
			if tcpAddr.IP.IsUnspecified() {
				tcpAddr.IP = net.IPv4(127, 0, 0, 1)
			}
			// If no access URL is specified, fallback to the
			// bounds URL.
			localURL := &url.URL{
				Scheme: "http",
				Host:   tcpAddr.String(),
			}
			if cfg.TLS.Enable.Value {
				localURL.Scheme = "https"
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

			// Validate provided auto-import templates.
			var (
				validatedAutoImportTemplates     = make([]coderd.AutoImportTemplate, len(cfg.AutoImportTemplates.Value))
				seenValidatedAutoImportTemplates = make(map[coderd.AutoImportTemplate]struct{}, len(cfg.AutoImportTemplates.Value))
			)
			for i, autoImportTemplate := range cfg.AutoImportTemplates.Value {
				var v coderd.AutoImportTemplate
				switch autoImportTemplate {
				case "kubernetes":
					v = coderd.AutoImportTemplateKubernetes
				default:
					return xerrors.Errorf("auto import template %q is not supported", autoImportTemplate)
				}

				if _, ok := seenValidatedAutoImportTemplates[v]; ok {
					return xerrors.Errorf("auto import template %q is specified more than once", v)
				}
				seenValidatedAutoImportTemplates[v] = struct{}{}
				validatedAutoImportTemplates[i] = v
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
				Database:                    databasefake.New(),
				DERPMap:                     derpMap,
				Pubsub:                      database.NewPubsubInMemory(),
				CacheDir:                    cfg.CacheDirectory.Value,
				GoogleTokenValidator:        googleTokenValidator,
				GitAuthConfigs:              gitAuthConfigs,
				RealIPConfig:                realIPConfig,
				SecureAuthCookie:            cfg.SecureAuthCookie.Value,
				SSHKeygenAlgorithm:          sshKeygenAlgorithm,
				TracerProvider:              tracerProvider,
				Telemetry:                   telemetry.NewNoop(),
				AutoImportTemplates:         validatedAutoImportTemplates,
				MetricsCacheRefreshInterval: cfg.MetricsCacheRefreshInterval.Value,
				AgentStatsRefreshInterval:   cfg.AgentStatRefreshInterval.Value,
				DeploymentConfig:            cfg,
				PrometheusRegistry:          prometheus.NewRegistry(),
				APIRateLimit:                cfg.APIRateLimit.Value,
			}
			if tlsConfig != nil {
				options.TLSCertificates = tlsConfig.Certificates
			}

			if cfg.OAuth2.Github.ClientSecret.Value != "" {
				options.GithubOAuth2Config, err = configureGithubOAuth2(accessURLParsed,
					cfg.OAuth2.Github.ClientID.Value,
					cfg.OAuth2.Github.ClientSecret.Value,
					cfg.OAuth2.Github.AllowSignups.Value,
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

				ctx, err := handleOauth2ClientCertificates(ctx, cfg)
				if err != nil {
					return xerrors.Errorf("configure oidc client certificates: %w", err)
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
					Verifier: oidcProvider.Verifier(&oidc.Config{
						ClientID: cfg.OIDC.ClientID.Value,
					}),
					EmailDomain:  cfg.OIDC.EmailDomain.Value,
					AllowSignups: cfg.OIDC.AllowSignups.Value,
				}
			}

			if cfg.InMemoryDatabase.Value {
				options.Database = databasefake.New()
				options.Pubsub = database.NewPubsubInMemory()
			} else {
				sqlDB, err := sql.Open(sqlDriver, cfg.PostgresURL.Value)
				if err != nil {
					return xerrors.Errorf("dial postgres: %w", err)
				}
				defer sqlDB.Close()
				// Ensure the PostgreSQL version is >=13.0.0!
				version, err := sqlDB.QueryContext(ctx, "SHOW server_version;")
				if err != nil {
					return xerrors.Errorf("get postgres version: %w", err)
				}
				if !version.Next() {
					return xerrors.Errorf("no rows returned for version select")
				}
				var versionStr string
				err = version.Scan(&versionStr)
				if err != nil {
					return xerrors.Errorf("scan version: %w", err)
				}
				_ = version.Close()
				versionStr = strings.Split(versionStr, " ")[0]
				if semver.Compare("v"+versionStr, "v13") < 0 {
					return xerrors.New("PostgreSQL version must be v13.0.0 or higher!")
				}

				err = sqlDB.Ping()
				if err != nil {
					return xerrors.Errorf("ping postgres: %w", err)
				}
				err = migrations.Up(sqlDB)
				if err != nil {
					return xerrors.Errorf("migrate up: %w", err)
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

				options.Telemetry, err = telemetry.New(telemetry.Options{
					BuiltinPostgres: builtinPostgres,
					DeploymentID:    deploymentID,
					Database:        options.Database,
					Logger:          logger.Named("telemetry"),
					URL:             telemetryURL,
					GitHubOAuth:     cfg.OAuth2.Github.ClientID.Value != "",
					OIDCAuth:        cfg.OIDC.ClientID.Value != "",
					OIDCIssuerURL:   cfg.OIDC.IssuerURL.Value,
					Prometheus:      cfg.Prometheus.Enable.Value,
					STUN:            len(cfg.DERP.Server.STUNAddresses.Value) != 0,
					Tunnel:          tunnel != nil,
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

			// We use a separate coderAPICloser so the Enterprise API
			// can have it's own close functions. This is cleaner
			// than abstracting the Coder API itself.
			coderAPI, coderAPICloser, err := newAPI(ctx, options)
			if err != nil {
				return err
			}

			client := codersdk.New(localURL)
			if cfg.TLS.Enable.Value {
				// Secure transport isn't needed for locally communicating!
				client.HTTPClient.Transport = &http.Transport{
					TLSClientConfig: &tls.Config{
						//nolint:gosec
						InsecureSkipVerify: true,
					},
				}
				defer client.HTTPClient.CloseIdleConnections()
			}

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
				daemon, err := newProvisionerDaemon(ctx, coderAPI, provisionerdMetrics, logger, cfg, errCh, false)
				if err != nil {
					return xerrors.Errorf("create provisioner daemon: %w", err)
				}
				provisionerDaemons = append(provisionerDaemons, daemon)
			}

			shutdownConnsCtx, shutdownConns := context.WithCancel(ctx)
			defer shutdownConns()

			// ReadHeaderTimeout is purposefully not enabled. It caused some issues with
			// websockets over the dev tunnel.
			// See: https://github.com/coder/coder/pull/3730
			//nolint:gosec
			server := &http.Server{
				// These errors are typically noise like "TLS: EOF". Vault does similar:
				// https://github.com/hashicorp/vault/blob/e2490059d0711635e529a4efcbaa1b26998d6e1c/command/server.go#L2714
				ErrorLog: log.New(io.Discard, "", 0),
				Handler:  coderAPI.RootHandler,
				BaseContext: func(_ net.Listener) context.Context {
					return shutdownConnsCtx
				},
			}
			defer func() {
				_ = shutdownWithTimeout(server.Shutdown, 5*time.Second)
			}()

			eg := errgroup.Group{}
			eg.Go(func() error {
				// Make sure to close the tunnel listener if we exit so the
				// errgroup doesn't wait forever!
				if tunnel != nil {
					defer tunnel.Listener.Close()
				}

				return server.Serve(listener)
			})
			if tunnel != nil {
				eg.Go(func() error {
					defer listener.Close()

					return server.Serve(tunnel.Listener)
				})
			}
			go func() {
				select {
				case errCh <- eg.Wait():
				default:
				}
			}()

			hasFirstUser, err := client.HasFirstUser(ctx)
			if !hasFirstUser && err == nil {
				cmd.Println()
				cmd.Println("Get started by creating the first user (in a new terminal):")
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

			// This is helpful for tests, but can be silently ignored.
			// Coder may be ran as users that don't have permission to write in the homedir,
			// such as via the systemd service.
			_ = config.URL().Write(client.URL.String())

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
			err = shutdownWithTimeout(server.Shutdown, 3*time.Second)
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

	root.AddCommand(&cobra.Command{
		Use:   "postgres-builtin-url",
		Short: "Output the connection URL for the built-in PostgreSQL deployment.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg := createConfig(cmd)
			url, err := embeddedPostgresURL(cfg)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "psql %q\n", url)
			return nil
		},
	})

	root.AddCommand(&cobra.Command{
		Use:   "postgres-builtin-serve",
		Short: "Run the built-in PostgreSQL deployment.",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := createConfig(cmd)
			logger := slog.Make(sloghuman.Sink(cmd.ErrOrStderr()))
			if ok, _ := cmd.Flags().GetBool(varVerbose); ok {
				logger = logger.Leveled(slog.LevelDebug)
			}

			url, closePg, err := startBuiltinPostgres(cmd.Context(), cfg, logger)
			if err != nil {
				return err
			}
			defer func() { _ = closePg() }()

			cmd.Println(cliui.Styles.Code.Render("psql \"" + url + "\""))

			stopChan := make(chan os.Signal, 1)
			defer signal.Stop(stopChan)
			signal.Notify(stopChan, os.Interrupt)

			<-stopChan
			return nil
		},
	})

	deployment.AttachFlags(root.Flags(), vip, false)

	return root
}

// parseURL parses a string into a URL.
func parseURL(u string) (*url.URL, error) {
	var (
		hasScheme = strings.HasPrefix(u, "http:") || strings.HasPrefix(u, "https:")
	)

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
	errCh chan error,
	dev bool,
) (srv *provisionerd.Server, err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer func() {
		if err != nil {
			cancel()
		}
	}()

	err = os.MkdirAll(cfg.CacheDirectory.Value, 0o700)
	if err != nil {
		return nil, xerrors.Errorf("mkdir %q: %w", cfg.CacheDirectory.Value, err)
	}

	terraformClient, terraformServer := provisionersdk.TransportPipe()
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
			CachePath: cfg.CacheDirectory.Value,
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
		string(database.ProvisionerTypeTerraform): sdkproto.NewDRPCProvisionerClient(provisionersdk.Conn(terraformClient)),
	}
	// include echo provisioner when in dev mode
	if dev {
		echoClient, echoServer := provisionersdk.TransportPipe()
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
		provisioners[string(database.ProvisionerTypeEcho)] = sdkproto.NewDRPCProvisionerClient(provisionersdk.Conn(echoClient))
	}
	return provisionerd.New(func(ctx context.Context) (proto.DRPCProvisionerDaemonClient, error) {
		// This debounces calls to listen every second. Read the comment
		// in provisionerdserver.go to learn more!
		return coderAPI.ListenProvisionerDaemon(ctx, time.Second)
	}, &provisionerd.Options{
		Logger:              logger,
		PollInterval:        500 * time.Millisecond,
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
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s - Remote development on your infrastucture\n", cliui.Styles.Bold.Render("Coder "+buildinfo.Version()))
}

func loadCertificates(tlsCertFiles, tlsKeyFiles []string) ([]tls.Certificate, error) {
	if len(tlsCertFiles) != len(tlsKeyFiles) {
		return nil, xerrors.New("--tls-cert-file and --tls-key-file must be used the same amount of times")
	}
	if len(tlsCertFiles) == 0 {
		return nil, xerrors.New("--tls-cert-file is required when tls is enabled")
	}
	if len(tlsKeyFiles) == 0 {
		return nil, xerrors.New("--tls-key-file is required when tls is enabled")
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

	if tlsClientCAFile != "" {
		caPool := x509.NewCertPool()
		data, err := os.ReadFile(tlsClientCAFile)
		if err != nil {
			return nil, xerrors.Errorf("read %q: %w", tlsClientCAFile, err)
		}
		if !caPool.AppendCertsFromPEM(data) {
			return nil, xerrors.Errorf("failed to parse CA certificate in tls-client-ca-file")
		}
		tlsConfig.ClientCAs = caPool
	}

	return tlsConfig, nil
}

func configureGithubOAuth2(accessURL *url.URL, clientID, clientSecret string, allowSignups bool, allowOrgs []string, rawTeams []string, enterpriseBaseURL string) (*coderd.GithubOAuth2Config, error) {
	redirectURL, err := accessURL.Parse("/api/v2/users/oauth2/github/callback")
	if err != nil {
		return nil, xerrors.Errorf("parse github oauth callback url: %w", err)
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

func serveHandler(ctx context.Context, logger slog.Logger, handler http.Handler, addr, name string) (closeFunc func()) {
	logger.Debug(ctx, "http server listening", slog.F("addr", addr), slog.F("name", name))

	// ReadHeaderTimeout is purposefully not enabled. It caused some issues with
	// websockets over the dev tunnel.
	// See: https://github.com/coder/coder/pull/3730
	//nolint:gosec
	srv := &http.Server{
		Addr:    addr,
		Handler: handler,
	}
	go func() {
		err := srv.ListenAndServe()
		if err != nil && !xerrors.Is(err, http.ErrServerClosed) {
			logger.Error(ctx, "http server listen", slog.F("name", name), slog.Error(err))
		}
	}()

	return func() {
		_ = srv.Close()
	}
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
	pgPort, err := strconv.Atoi(pgPortRaw)
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

func handleOauth2ClientCertificates(ctx context.Context, cfg *codersdk.DeploymentConfig) (context.Context, error) {
	if cfg.TLS.ClientCertFile.Value != "" && cfg.TLS.ClientKeyFile.Value != "" {
		certificates, err := loadCertificates([]string{cfg.TLS.ClientCertFile.Value}, []string{cfg.TLS.ClientKeyFile.Value})
		if err != nil {
			return nil, err
		}

		return context.WithValue(ctx, oauth2.HTTPClient, &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{ //nolint:gosec
					Certificates: certificates,
				},
			},
		}), nil
	}
	return ctx, nil
}

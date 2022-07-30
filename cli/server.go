package cli

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/pem"
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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-systemd/daemon"
	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"github.com/google/go-github/v43/github"
	"github.com/google/uuid"
	"github.com/pion/turn/v2"
	"github.com/pion/webrtc/v3"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"golang.org/x/oauth2"
	xgithub "golang.org/x/oauth2/github"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"
	"google.golang.org/api/idtoken"
	"google.golang.org/api/option"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/buildinfo"
	"github.com/coder/coder/cli/cliflag"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/cli/config"
	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/autobuild/executor"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/databasefake"
	"github.com/coder/coder/coderd/devtunnel"
	"github.com/coder/coder/coderd/gitsshkey"
	"github.com/coder/coder/coderd/telemetry"
	"github.com/coder/coder/coderd/tracing"
	"github.com/coder/coder/coderd/turnconn"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/cryptorand"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisioner/terraform"
	"github.com/coder/coder/provisionerd"
	"github.com/coder/coder/provisionersdk"
	"github.com/coder/coder/provisionersdk/proto"
)

// nolint:gocyclo
func server() *cobra.Command {
	var (
		accessURL             string
		address               string
		autobuildPollInterval time.Duration
		promEnabled           bool
		promAddress           string
		pprofEnabled          bool
		pprofAddress          string
		cacheDir              string
		inMemoryDatabase      bool
		// provisionerDaemonCount is a uint8 to ensure a number > 0.
		provisionerDaemonCount           uint8
		postgresURL                      string
		oauth2GithubClientID             string
		oauth2GithubClientSecret         string
		oauth2GithubAllowedOrganizations []string
		oauth2GithubAllowedTeams         []string
		oauth2GithubAllowSignups         bool
		telemetryEnable                  bool
		telemetryURL                     string
		tlsCertFile                      string
		tlsClientCAFile                  string
		tlsClientAuth                    string
		tlsEnable                        bool
		tlsKeyFile                       string
		tlsMinVersion                    string
		turnRelayAddress                 string
		tunnel                           bool
		stunServers                      []string
		trace                            bool
		secureAuthCookie                 bool
		sshKeygenAlgorithmRaw            string
		spooky                           bool
		verbose                          bool
	)

	root := &cobra.Command{
		Use:   "server",
		Short: "Start a Coder server",
		RunE: func(cmd *cobra.Command, args []string) error {
			printLogo(cmd, spooky)
			logger := slog.Make(sloghuman.Sink(cmd.ErrOrStderr()))
			if verbose {
				logger = logger.Leveled(slog.LevelDebug)
			}

			// Main command context for managing cancellation
			// of running services.
			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()

			// Clean up idle connections at the end, e.g.
			// embedded-postgres can leave an idle connection
			// which is caught by goleaks.
			defer http.DefaultClient.CloseIdleConnections()

			var (
				tracerProvider *sdktrace.TracerProvider
				err            error
				sqlDriver      = "postgres"
			)
			if trace {
				tracerProvider, err = tracing.TracerProvider(ctx, "coderd")
				if err != nil {
					logger.Warn(ctx, "failed to start telemetry exporter", slog.Error(err))
				} else {
					// allow time for traces to flush even if command context is canceled
					defer func() {
						_ = shutdownWithTimeout(tracerProvider, 5*time.Second)
					}()

					d, err := tracing.PostgresDriver(tracerProvider, "coderd.database")
					if err != nil {
						logger.Warn(ctx, "failed to start postgres tracing driver", slog.Error(err))
					} else {
						sqlDriver = d
					}
				}
			}

			config := createConfig(cmd)
			builtinPostgres := false
			// Only use built-in if PostgreSQL URL isn't specified!
			if !inMemoryDatabase && postgresURL == "" {
				var closeFunc func() error
				cmd.Printf("Using built-in PostgreSQL (%s)\n", config.PostgresPath())
				postgresURL, closeFunc, err = startBuiltinPostgres(ctx, config, logger)
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

			listener, err := net.Listen("tcp", address)
			if err != nil {
				return xerrors.Errorf("listen %q: %w", address, err)
			}
			defer listener.Close()

			if tlsEnable {
				listener, err = configureTLS(listener, tlsMinVersion, tlsClientAuth, tlsCertFile, tlsKeyFile, tlsClientCAFile)
				if err != nil {
					return xerrors.Errorf("configure tls: %w", err)
				}
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
			if tlsEnable {
				localURL.Scheme = "https"
			}
			if accessURL == "" {
				accessURL = localURL.String()
			}

			var (
				ctxTunnel, closeTunnel = context.WithCancel(ctx)
				devTunnel              *devtunnel.Tunnel
				devTunnelErr           <-chan error
			)
			defer closeTunnel()

			// If we're attempting to tunnel in dev-mode, the access URL
			// needs to be changed to use the tunnel.
			if tunnel {
				cmd.Printf("Opening tunnel so workspaces can connect to your deployment\n")
				devTunnel, devTunnelErr, err = devtunnel.New(ctxTunnel, logger.Named("devtunnel"))
				if err != nil {
					return xerrors.Errorf("create tunnel: %w", err)
				}
				accessURL = devTunnel.URL
			}

			// Warn the user if the access URL appears to be a loopback address.
			isLocal, err := isLocalURL(ctx, accessURL)
			if isLocal || err != nil {
				reason := "could not be resolved"
				if isLocal {
					reason = "isn't externally reachable"
				}
				cmd.Printf("%s The access URL %s %s, this may cause unexpected problems when creating workspaces. Generate a unique *.try.coder.app URL with:\n", cliui.Styles.Warn.Render("Warning:"), cliui.Styles.Field.Render(accessURL), reason)
				cmd.Println(cliui.Styles.Code.Render(strings.Join(os.Args, " ") + " --tunnel"))
			}
			cmd.Printf("View the Web UI: %s\n", accessURL)

			accessURLParsed, err := url.Parse(accessURL)
			if err != nil {
				return xerrors.Errorf("parse access url %q: %w", accessURL, err)
			}

			// Used for zero-trust instance identity with Google Cloud.
			googleTokenValidator, err := idtoken.NewValidator(ctx, option.WithoutAuthentication())
			if err != nil {
				return err
			}

			sshKeygenAlgorithm, err := gitsshkey.ParseAlgorithm(sshKeygenAlgorithmRaw)
			if err != nil {
				return xerrors.Errorf("parse ssh keygen algorithm %s: %w", sshKeygenAlgorithmRaw, err)
			}

			turnServer, err := turnconn.New(&turn.RelayAddressGeneratorStatic{
				RelayAddress: net.ParseIP(turnRelayAddress),
				Address:      turnRelayAddress,
			})
			if err != nil {
				return xerrors.Errorf("create turn server: %w", err)
			}
			defer turnServer.Close()

			iceServers := make([]webrtc.ICEServer, 0)
			for _, stunServer := range stunServers {
				iceServers = append(iceServers, webrtc.ICEServer{
					URLs: []string{stunServer},
				})
			}
			options := &coderd.Options{
				AccessURL:            accessURLParsed,
				ICEServers:           iceServers,
				Logger:               logger.Named("coderd"),
				Database:             databasefake.New(),
				Pubsub:               database.NewPubsubInMemory(),
				CacheDir:             cacheDir,
				GoogleTokenValidator: googleTokenValidator,
				SecureAuthCookie:     secureAuthCookie,
				SSHKeygenAlgorithm:   sshKeygenAlgorithm,
				TURNServer:           turnServer,
				TracerProvider:       tracerProvider,
				Telemetry:            telemetry.NewNoop(),
			}

			if oauth2GithubClientSecret != "" {
				options.GithubOAuth2Config, err = configureGithubOAuth2(accessURLParsed, oauth2GithubClientID, oauth2GithubClientSecret, oauth2GithubAllowSignups, oauth2GithubAllowedOrganizations, oauth2GithubAllowedTeams)
				if err != nil {
					return xerrors.Errorf("configure github oauth2: %w", err)
				}
			}

			if inMemoryDatabase {
				options.Database = databasefake.New()
				options.Pubsub = database.NewPubsubInMemory()
			} else {
				sqlDB, err := sql.Open(sqlDriver, postgresURL)
				if err != nil {
					return xerrors.Errorf("dial postgres: %w", err)
				}
				defer sqlDB.Close()

				err = sqlDB.Ping()
				if err != nil {
					return xerrors.Errorf("ping postgres: %w", err)
				}
				err = database.MigrateUp(sqlDB)
				if err != nil {
					return xerrors.Errorf("migrate up: %w", err)
				}
				options.Database = database.New(sqlDB)
				options.Pubsub, err = database.NewPubsub(ctx, sqlDB, postgresURL)
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

			// Parse the raw telemetry URL!
			telemetryURL, err := url.Parse(telemetryURL)
			if err != nil {
				return xerrors.Errorf("parse telemetry url: %w", err)
			}
			// Disable telemetry if the in-memory database is used unless explicitly defined!
			if inMemoryDatabase && !cmd.Flags().Changed("telemetry") {
				telemetryEnable = false
			}
			if telemetryEnable {
				options.Telemetry, err = telemetry.New(telemetry.Options{
					BuiltinPostgres: builtinPostgres,
					DeploymentID:    deploymentID,
					Database:        options.Database,
					Logger:          logger.Named("telemetry"),
					URL:             telemetryURL,
					GitHubOAuth:     oauth2GithubClientID != "",
					Prometheus:      promEnabled,
					STUN:            len(stunServers) != 0,
					Tunnel:          tunnel,
				})
				if err != nil {
					return xerrors.Errorf("create telemetry reporter: %w", err)
				}
				defer options.Telemetry.Close()
			}

			coderAPI := coderd.New(options)
			defer coderAPI.Close()

			client := codersdk.New(localURL)
			if tlsEnable {
				// Secure transport isn't needed for locally communicating!
				client.HTTPClient.Transport = &http.Transport{
					TLSClientConfig: &tls.Config{
						//nolint:gosec
						InsecureSkipVerify: true,
					},
				}
			}

			// This prevents the pprof import from being accidentally deleted.
			_ = pprof.Handler
			if pprofEnabled {
				//nolint:revive
				defer serveHandler(ctx, logger, nil, pprofAddress, "pprof")()
			}
			if promEnabled {
				//nolint:revive
				defer serveHandler(ctx, logger, promhttp.Handler(), promAddress, "prometheus")()
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
			for i := 0; uint8(i) < provisionerDaemonCount; i++ {
				daemon, err := newProvisionerDaemon(ctx, coderAPI, logger, cacheDir, errCh, false)
				if err != nil {
					return xerrors.Errorf("create provisioner daemon: %w", err)
				}
				provisionerDaemons = append(provisionerDaemons, daemon)
			}

			shutdownConnsCtx, shutdownConns := context.WithCancel(ctx)
			defer shutdownConns()
			server := &http.Server{
				// These errors are typically noise like "TLS: EOF". Vault does similar:
				// https://github.com/hashicorp/vault/blob/e2490059d0711635e529a4efcbaa1b26998d6e1c/command/server.go#L2714
				ErrorLog: log.New(io.Discard, "", 0),
				Handler:  coderAPI.Handler,
				BaseContext: func(_ net.Listener) context.Context {
					return shutdownConnsCtx
				},
			}
			defer func() {
				_ = shutdownWithTimeout(server, 5*time.Second)
			}()

			eg := errgroup.Group{}
			eg.Go(func() error {
				// Make sure to close the tunnel listener if we exit so the
				// errgroup doesn't wait forever!
				if tunnel {
					defer devTunnel.Listener.Close()
				}

				return server.Serve(listener)
			})
			if tunnel {
				eg.Go(func() error {
					defer listener.Close()

					return server.Serve(devTunnel.Listener)
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
				cmd.Println(cliui.Styles.Code.Render("coder login " + accessURL))
			}

			cmd.Println("\n==> Logs will stream in below (press ctrl+c to gracefully exit):")

			// Updates the systemd status from activating to activated.
			_, err = daemon.SdNotify(false, daemon.SdNotifyReady)
			if err != nil {
				return xerrors.Errorf("notify systemd: %w", err)
			}

			autobuildPoller := time.NewTicker(autobuildPollInterval)
			defer autobuildPoller.Stop()
			autobuildExecutor := executor.New(ctx, options.Database, logger, autobuildPoller.C)
			autobuildExecutor.Run()

			// This is helpful for tests, but can be silently ignored.
			// Coder may be ran as users that don't have permission to write in the homedir,
			// such as via the systemd service.
			_ = config.URL().Write(client.URL.String())

			// Because the graceful shutdown includes cleaning up workspaces in dev mode, we're
			// going to make it harder to accidentally skip the graceful shutdown by hitting ctrl+c
			// two or more times.  So the stopChan is unlimited in size and we don't call
			// signal.Stop() until graceful shutdown finished--this means we swallow additional
			// SIGINT after the first.  To get out of a graceful shutdown, the user can send SIGQUIT
			// with ctrl+\ or SIGTERM with `kill`.
			ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
			defer stop()

			// Currently there is no way to ask the server to shut
			// itself down, so any exit signal will result in a non-zero
			// exit of the server.
			var exitErr error
			select {
			case <-ctx.Done():
				exitErr = ctx.Err()
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), cliui.Styles.Bold.Render(
					"Interrupt caught, gracefully exiting. Use ctrl+\\ to force quit",
				))
			case exitErr = <-devTunnelErr:
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
			err = shutdownWithTimeout(server, 5*time.Second)
			if err != nil {
				cmd.Printf("API server shutdown took longer than 5s: %s", err)
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

					if verbose {
						cmd.Printf("Shutting down provisioner daemon %d...\n", id)
					}
					err := shutdownWithTimeout(provisionerDaemon, 5*time.Second)
					if err != nil {
						cmd.PrintErrf("Failed to shutdown provisioner daemon %d: %s\n", id, err)
						return
					}
					err = provisionerDaemon.Close()
					if err != nil {
						cmd.PrintErrf("Close provisioner daemon %d: %s\n", id, err)
						return
					}
					if verbose {
						cmd.Printf("Gracefully shut down provisioner daemon %d\n", id)
					}
				}()
			}
			wg.Wait()

			cmd.Println("Waiting for WebSocket connections to close...")
			_ = coderAPI.Close()
			cmd.Println("Done wainting for WebSocket connections")

			// Close tunnel after we no longer have in-flight connections.
			if tunnel {
				cmd.Println("Waiting for tunnel to close...")
				closeTunnel()
				<-devTunnelErr
				cmd.Println("Done waiting for tunnel")
			}

			// Ensures a last report can be sent before exit!
			options.Telemetry.Close()

			// Trigger context cancellation for any remaining services.
			cancel()

			return exitErr
		},
	}

	root.AddCommand(&cobra.Command{
		Use:   "postgres-builtin-url",
		Short: "Output the connection URL for the built-in PostgreSQL deployment.",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := createConfig(cmd)
			url, err := embeddedPostgresURL(cfg)
			if err != nil {
				return err
			}
			cmd.Println(cliui.Styles.Code.Render("psql \"" + url + "\""))
			return nil
		},
	})

	root.AddCommand(&cobra.Command{
		Use:   "postgres-builtin-serve",
		Short: "Run the built-in PostgreSQL deployment.",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := createConfig(cmd)
			logger := slog.Make(sloghuman.Sink(cmd.ErrOrStderr()))
			if verbose {
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

	cliflag.DurationVarP(root.Flags(), &autobuildPollInterval, "autobuild-poll-interval", "", "CODER_AUTOBUILD_POLL_INTERVAL", time.Minute, "Specifies the interval at which to poll for and execute automated workspace build operations.")
	cliflag.StringVarP(root.Flags(), &accessURL, "access-url", "", "CODER_ACCESS_URL", "", "Specifies the external URL to access Coder.")
	cliflag.StringVarP(root.Flags(), &address, "address", "a", "CODER_ADDRESS", "127.0.0.1:3000", "The address to serve the API and dashboard.")
	cliflag.BoolVarP(root.Flags(), &promEnabled, "prometheus-enable", "", "CODER_PROMETHEUS_ENABLE", false, "Enable serving prometheus metrics on the addressdefined by --prometheus-address.")
	cliflag.StringVarP(root.Flags(), &promAddress, "prometheus-address", "", "CODER_PROMETHEUS_ADDRESS", "127.0.0.1:2112", "The address to serve prometheus metrics.")
	cliflag.BoolVarP(root.Flags(), &pprofEnabled, "pprof-enable", "", "CODER_PPROF_ENABLE", false, "Enable serving pprof metrics on the address defined by --pprof-address.")
	cliflag.StringVarP(root.Flags(), &pprofAddress, "pprof-address", "", "CODER_PPROF_ADDRESS", "127.0.0.1:6060", "The address to serve pprof.")
	defaultCacheDir := filepath.Join(os.TempDir(), "coder-cache")
	if dir := os.Getenv("CACHE_DIRECTORY"); dir != "" {
		// For compatibility with systemd.
		defaultCacheDir = dir
	}
	cliflag.StringVarP(root.Flags(), &cacheDir, "cache-dir", "", "CODER_CACHE_DIRECTORY", defaultCacheDir, "Specifies a directory to cache binaries for provision operations. If unspecified and $CACHE_DIRECTORY is set, it will be used for compatibility with systemd.")
	cliflag.BoolVarP(root.Flags(), &inMemoryDatabase, "in-memory", "", "CODER_INMEMORY", false,
		"Specifies whether data will be stored in an in-memory database.")
	_ = root.Flags().MarkHidden("in-memory")
	cliflag.StringVarP(root.Flags(), &postgresURL, "postgres-url", "", "CODER_PG_CONNECTION_URL", "", "The URL of a PostgreSQL database to connect to. If empty, PostgreSQL binaries will be downloaded from Maven (https://repo1.maven.org/maven2) and store all data in the config root. Access the built-in database with \"coder server postgres-builtin-url\"")
	cliflag.Uint8VarP(root.Flags(), &provisionerDaemonCount, "provisioner-daemons", "", "CODER_PROVISIONER_DAEMONS", 3, "The amount of provisioner daemons to create on start.")
	cliflag.StringVarP(root.Flags(), &oauth2GithubClientID, "oauth2-github-client-id", "", "CODER_OAUTH2_GITHUB_CLIENT_ID", "",
		"Specifies a client ID to use for oauth2 with GitHub.")
	cliflag.StringVarP(root.Flags(), &oauth2GithubClientSecret, "oauth2-github-client-secret", "", "CODER_OAUTH2_GITHUB_CLIENT_SECRET", "",
		"Specifies a client secret to use for oauth2 with GitHub.")
	cliflag.StringArrayVarP(root.Flags(), &oauth2GithubAllowedOrganizations, "oauth2-github-allowed-orgs", "", "CODER_OAUTH2_GITHUB_ALLOWED_ORGS", nil,
		"Specifies organizations the user must be a member of to authenticate with GitHub.")
	cliflag.StringArrayVarP(root.Flags(), &oauth2GithubAllowedTeams, "oauth2-github-allowed-teams", "", "CODER_OAUTH2_GITHUB_ALLOWED_TEAMS", nil,
		"Specifies teams inside organizations the user must be a member of to authenticate with GitHub. Formatted as: <organization-name>/<team-slug>.")
	cliflag.BoolVarP(root.Flags(), &oauth2GithubAllowSignups, "oauth2-github-allow-signups", "", "CODER_OAUTH2_GITHUB_ALLOW_SIGNUPS", false,
		"Specifies whether new users can sign up with GitHub.")
	enableTelemetryByDefault := !isTest()
	cliflag.BoolVarP(root.Flags(), &telemetryEnable, "telemetry", "", "CODER_TELEMETRY", enableTelemetryByDefault, "Specifies whether telemetry is enabled or not. Coder collects anonymized usage data to help improve our product.")
	cliflag.StringVarP(root.Flags(), &telemetryURL, "telemetry-url", "", "CODER_TELEMETRY_URL", "https://telemetry.coder.com", "Specifies a URL to send telemetry to.")
	_ = root.Flags().MarkHidden("telemetry-url")
	cliflag.BoolVarP(root.Flags(), &tlsEnable, "tls-enable", "", "CODER_TLS_ENABLE", false, "Specifies if TLS will be enabled")
	cliflag.StringVarP(root.Flags(), &tlsCertFile, "tls-cert-file", "", "CODER_TLS_CERT_FILE", "",
		"Specifies the path to the certificate for TLS. It requires a PEM-encoded file. "+
			"To configure the listener to use a CA certificate, concatenate the primary certificate "+
			"and the CA certificate together. The primary certificate should appear first in the combined file")
	cliflag.StringVarP(root.Flags(), &tlsClientCAFile, "tls-client-ca-file", "", "CODER_TLS_CLIENT_CA_FILE", "",
		"PEM-encoded Certificate Authority file used for checking the authenticity of client")
	cliflag.StringVarP(root.Flags(), &tlsClientAuth, "tls-client-auth", "", "CODER_TLS_CLIENT_AUTH", "request",
		`Specifies the policy the server will follow for TLS Client Authentication. `+
			`Accepted values are "none", "request", "require-any", "verify-if-given", or "require-and-verify"`)
	cliflag.StringVarP(root.Flags(), &tlsKeyFile, "tls-key-file", "", "CODER_TLS_KEY_FILE", "",
		"Specifies the path to the private key for the certificate. It requires a PEM-encoded file")
	cliflag.StringVarP(root.Flags(), &tlsMinVersion, "tls-min-version", "", "CODER_TLS_MIN_VERSION", "tls12",
		`Specifies the minimum supported version of TLS. Accepted values are "tls10", "tls11", "tls12" or "tls13"`)
	cliflag.BoolVarP(root.Flags(), &tunnel, "tunnel", "", "CODER_TUNNEL", false,
		"Workspaces must be able to reach the `access-url`. This overrides your access URL with a public access URL that tunnels your Coder deployment.")
	cliflag.StringArrayVarP(root.Flags(), &stunServers, "stun-server", "", "CODER_STUN_SERVERS", []string{
		"stun:stun.l.google.com:19302",
	}, "Specify URLs for STUN servers to enable P2P connections.")
	cliflag.BoolVarP(root.Flags(), &trace, "trace", "", "CODER_TRACE", false, "Specifies if application tracing data is collected")
	cliflag.StringVarP(root.Flags(), &turnRelayAddress, "turn-relay-address", "", "CODER_TURN_RELAY_ADDRESS", "127.0.0.1",
		"Specifies the address to bind TURN connections.")
	cliflag.BoolVarP(root.Flags(), &secureAuthCookie, "secure-auth-cookie", "", "CODER_SECURE_AUTH_COOKIE", false, "Specifies if the 'Secure' property is set on browser session cookies")
	cliflag.StringVarP(root.Flags(), &sshKeygenAlgorithmRaw, "ssh-keygen-algorithm", "", "CODER_SSH_KEYGEN_ALGORITHM", "ed25519", "Specifies the algorithm to use for generating ssh keys. "+
		`Accepted values are "ed25519", "ecdsa", or "rsa4096"`)
	cliflag.BoolVarP(root.Flags(), &spooky, "spooky", "", "", false, "Specifies spookiness level")
	cliflag.BoolVarP(root.Flags(), &verbose, "verbose", "v", "CODER_VERBOSE", false, "Enables verbose logging.")
	_ = root.Flags().MarkHidden("spooky")

	return root
}

func shutdownWithTimeout(s interface{ Shutdown(context.Context) error }, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return s.Shutdown(ctx)
}

// nolint:revive
func newProvisionerDaemon(ctx context.Context, coderAPI *coderd.API,
	logger slog.Logger, cacheDir string, errCh chan error, dev bool,
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
		string(database.ProvisionerTypeTerraform): proto.NewDRPCProvisionerClient(provisionersdk.Conn(terraformClient)),
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
		provisioners[string(database.ProvisionerTypeEcho)] = proto.NewDRPCProvisionerClient(provisionersdk.Conn(echoClient))
	}
	return provisionerd.New(coderAPI.ListenProvisionerDaemon, &provisionerd.Options{
		Logger:         logger,
		PollInterval:   500 * time.Millisecond,
		UpdateInterval: 500 * time.Millisecond,
		Provisioners:   provisioners,
		WorkDirectory:  tempDir,
	}), nil
}

// nolint: revive
func printLogo(cmd *cobra.Command, spooky bool) {
	if spooky {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), `▄████▄   ▒█████  ▓█████▄ ▓█████  ██▀███  
▒██▀ ▀█  ▒██▒  ██▒▒██▀ ██▌▓█   ▀ ▓██ ▒ ██▒
▒▓█    ▄ ▒██░  ██▒░██   █▌▒███   ▓██ ░▄█ ▒
▒▓▓▄ ▄██▒▒██   ██░░▓█▄   ▌▒▓█  ▄ ▒██▀▀█▄  
▒ ▓███▀ ░░ ████▓▒░░▒████▓ ░▒████▒░██▓ ▒██▒
░ ░▒ ▒  ░░ ▒░▒░▒░  ▒▒▓  ▒ ░░ ▒░ ░░ ▒▓ ░▒▓░
  ░  ▒     ░ ▒ ▒░  ░ ▒  ▒  ░ ░  ░  ░▒ ░ ▒░
░        ░ ░ ░ ▒   ░ ░  ░    ░     ░░   ░ 
░ ░          ░ ░     ░       ░  ░   ░     
░                  ░                      		
`)
		return
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s - Remote development on your infrastucture\n", cliui.Styles.Bold.Render("Coder "+buildinfo.Version()))
}

func configureTLS(listener net.Listener, tlsMinVersion, tlsClientAuth, tlsCertFile, tlsKeyFile, tlsClientCAFile string) (net.Listener, error) {
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

	if tlsCertFile == "" {
		return nil, xerrors.New("tls-cert-file is required when tls is enabled")
	}
	if tlsKeyFile == "" {
		return nil, xerrors.New("tls-key-file is required when tls is enabled")
	}

	certPEMBlock, err := os.ReadFile(tlsCertFile)
	if err != nil {
		return nil, xerrors.Errorf("read file %q: %w", tlsCertFile, err)
	}
	keyPEMBlock, err := os.ReadFile(tlsKeyFile)
	if err != nil {
		return nil, xerrors.Errorf("read file %q: %w", tlsKeyFile, err)
	}
	keyBlock, _ := pem.Decode(keyPEMBlock)
	if keyBlock == nil {
		return nil, xerrors.New("decoded pem is blank")
	}
	cert, err := tls.X509KeyPair(certPEMBlock, keyPEMBlock)
	if err != nil {
		return nil, xerrors.Errorf("create key pair: %w", err)
	}
	tlsConfig.GetCertificate = func(chi *tls.ClientHelloInfo) (*tls.Certificate, error) {
		return &cert, nil
	}

	certPool := x509.NewCertPool()
	certPool.AppendCertsFromPEM(certPEMBlock)
	tlsConfig.RootCAs = certPool

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

	return tls.NewListener(listener, tlsConfig), nil
}

func configureGithubOAuth2(accessURL *url.URL, clientID, clientSecret string, allowSignups bool, allowOrgs []string, rawTeams []string) (*coderd.GithubOAuth2Config, error) {
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
	return &coderd.GithubOAuth2Config{
		OAuth2Config: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Endpoint:     xgithub.Endpoint,
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
			user, _, err := github.NewClient(client).Users.Get(ctx, "")
			return user, err
		},
		ListEmails: func(ctx context.Context, client *http.Client) ([]*github.UserEmail, error) {
			emails, _, err := github.NewClient(client).Users.ListEmails(ctx, &github.ListOptions{})
			return emails, err
		},
		ListOrganizationMemberships: func(ctx context.Context, client *http.Client) ([]*github.Membership, error) {
			memberships, _, err := github.NewClient(client).Organizations.ListOrgMemberships(ctx, &github.ListOrgMembershipsOptions{
				State: "active",
				ListOptions: github.ListOptions{
					PerPage: 100,
				},
			})
			return memberships, err
		},
		TeamMembership: func(ctx context.Context, client *http.Client, org, teamSlug, username string) (*github.Membership, error) {
			team, _, err := github.NewClient(client).Teams.GetTeamMembershipBySlug(ctx, org, teamSlug, username)
			return team, err
		},
	}, nil
}

func serveHandler(ctx context.Context, logger slog.Logger, handler http.Handler, addr, name string) (closeFunc func()) {
	logger.Debug(ctx, "http server listening", slog.F("addr", addr), slog.F("name", name))

	srv := &http.Server{Addr: addr, Handler: handler}
	go func() {
		err := srv.ListenAndServe()
		if err != nil && !xerrors.Is(err, http.ErrServerClosed) {
			logger.Error(ctx, "http server listen", slog.F("name", name), slog.Error(err))
		}
	}()

	return func() { _ = srv.Close() }
}

// isLocalURL returns true if the hostname of the provided URL appears to
// resolve to a loopback address.
func isLocalURL(ctx context.Context, urlString string) (bool, error) {
	parsedURL, err := url.Parse(urlString)
	if err != nil {
		return false, err
	}
	resolver := &net.Resolver{}
	ips, err := resolver.LookupIPAddr(ctx, parsedURL.Hostname())
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

package cli

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/briandowns/spinner"
	"github.com/coreos/go-systemd/daemon"
	"github.com/google/go-github/v43/github"
	"github.com/pion/turn/v2"
	"github.com/pion/webrtc/v3"
	"github.com/spf13/cobra"
	"golang.org/x/oauth2"
	xgithub "golang.org/x/oauth2/github"
	"golang.org/x/xerrors"
	"google.golang.org/api/idtoken"
	"google.golang.org/api/option"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/cli/cliflag"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/cli/config"
	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/databasefake"
	"github.com/coder/coder/coderd/devtunnel"
	"github.com/coder/coder/coderd/gitsshkey"
	"github.com/coder/coder/coderd/turnconn"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/cryptorand"
	"github.com/coder/coder/provisioner/terraform"
	"github.com/coder/coder/provisionerd"
	"github.com/coder/coder/provisionersdk"
	"github.com/coder/coder/provisionersdk/proto"
)

// nolint:gocyclo
func server() *cobra.Command {
	var (
		accessURL       string
		address         string
		cacheDir        string
		dev             bool
		devUserEmail    string
		devUserPassword string
		postgresURL     string
		// provisionerDaemonCount is a uint8 to ensure a number > 0.
		provisionerDaemonCount           uint8
		oauth2GithubClientID             string
		oauth2GithubClientSecret         string
		oauth2GithubAllowedOrganizations []string
		oauth2GithubAllowSignups         bool
		tlsCertFile                      string
		tlsClientCAFile                  string
		tlsClientAuth                    string
		tlsEnable                        bool
		tlsKeyFile                       string
		tlsMinVersion                    string
		turnRelayAddress                 string
		skipTunnel                       bool
		stunServers                      []string
		traceDatadog                     bool
		secureAuthCookie                 bool
		sshKeygenAlgorithmRaw            string
		spooky                           bool
	)

	root := &cobra.Command{
		Use: "server",
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := slog.Make(sloghuman.Sink(os.Stderr))
			if traceDatadog {
				tracer.Start(tracer.WithLogStartup(false), tracer.WithLogger(&datadogLogger{
					logger: logger.Named("datadog"),
				}))
				defer tracer.Stop()
			}

			printLogo(cmd, spooky)
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

			localURL := &url.URL{
				Scheme: "http",
				Host:   tcpAddr.String(),
			}
			if tlsEnable {
				localURL.Scheme = "https"
			}
			if accessURL == "" {
				accessURL = localURL.String()
			} else {
				// If an access URL is specified, always skip tunneling.
				skipTunnel = true
			}

			var (
				tunnelErrChan          <-chan error
				ctxTunnel, closeTunnel = context.WithCancel(cmd.Context())
			)
			defer closeTunnel()

			// If we're attempting to tunnel in dev-mode, the access URL
			// needs to be changed to use the tunnel.
			if dev && !skipTunnel {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), cliui.Styles.Wrap.Render(
					"Coder requires a URL accessible by workspaces you provision. "+
						"A free tunnel can be created for simple setup. This will "+
						"expose your Coder deployment to a publicly accessible URL. "+
						cliui.Styles.Field.Render("--access-url")+" can be specified instead.\n",
				))

				_, err = cliui.Prompt(cmd, cliui.PromptOptions{
					Text:      "Would you like to start a tunnel for simple setup?",
					IsConfirm: true,
				})
				if errors.Is(err, cliui.Canceled) {
					return err
				}
				if err == nil {
					accessURL, tunnelErrChan, err = devtunnel.New(ctxTunnel, localURL)
					if err != nil {
						return xerrors.Errorf("create tunnel: %w", err)
					}
				}
				_, _ = fmt.Fprintln(cmd.ErrOrStderr())
			}

			validator, err := idtoken.NewValidator(cmd.Context(), option.WithoutAuthentication())
			if err != nil {
				return err
			}

			accessURLParsed, err := url.Parse(accessURL)
			if err != nil {
				return xerrors.Errorf("parse access url %q: %w", accessURL, err)
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
				GoogleTokenValidator: validator,
				SecureAuthCookie:     secureAuthCookie,
				SSHKeygenAlgorithm:   sshKeygenAlgorithm,
				TURNServer:           turnServer,
			}

			if oauth2GithubClientSecret != "" {
				options.GithubOAuth2Config, err = configureGithubOAuth2(accessURLParsed, oauth2GithubClientID, oauth2GithubClientSecret, oauth2GithubAllowSignups, oauth2GithubAllowedOrganizations)
				if err != nil {
					return xerrors.Errorf("configure github oauth2: %w", err)
				}
			}

			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "access-url: %s\n", accessURL)
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "provisioner-daemons: %d\n", provisionerDaemonCount)
			_, _ = fmt.Fprintln(cmd.ErrOrStderr())

			if !dev {
				sqlDB, err := sql.Open("postgres", postgresURL)
				if err != nil {
					return xerrors.Errorf("dial postgres: %w", err)
				}
				err = sqlDB.Ping()
				if err != nil {
					return xerrors.Errorf("ping postgres: %w", err)
				}
				err = database.MigrateUp(sqlDB)
				if err != nil {
					return xerrors.Errorf("migrate up: %w", err)
				}
				options.Database = database.New(sqlDB)
				options.Pubsub, err = database.NewPubsub(cmd.Context(), sqlDB, postgresURL)
				if err != nil {
					return xerrors.Errorf("create pubsub: %w", err)
				}
			}

			handler, closeCoderd := coderd.New(options)
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

			errCh := make(chan error, 1)
			provisionerDaemons := make([]*provisionerd.Server, 0)
			for i := 0; uint8(i) < provisionerDaemonCount; i++ {
				daemonClose, err := newProvisionerDaemon(cmd.Context(), client, logger, cacheDir, errCh)
				if err != nil {
					return xerrors.Errorf("create provisioner daemon: %w", err)
				}
				provisionerDaemons = append(provisionerDaemons, daemonClose)
			}
			defer func() {
				for _, provisionerDaemon := range provisionerDaemons {
					_ = provisionerDaemon.Close()
				}
			}()

			shutdownConnsCtx, shutdownConns := context.WithCancel(cmd.Context())
			defer shutdownConns()
			go func() {
				defer close(errCh)
				server := http.Server{
					Handler: handler,
					BaseContext: func(_ net.Listener) context.Context {
						return shutdownConnsCtx
					},
				}
				errCh <- server.Serve(listener)
			}()

			config := createConfig(cmd)

			if dev {
				if devUserPassword == "" {
					devUserPassword, err = cryptorand.String(10)
					if err != nil {
						return xerrors.Errorf("generate random admin password for dev: %w", err)
					}
				}
				err = createFirstUser(cmd, client, config, devUserEmail, devUserPassword)
				if err != nil {
					return xerrors.Errorf("create first user: %w", err)
				}
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "email: %s\n", devUserEmail)
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "password: %s\n", devUserPassword)
				_, _ = fmt.Fprintln(cmd.ErrOrStderr())

				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), cliui.Styles.Wrap.Render(`Started in dev mode. All data is in-memory! `+cliui.Styles.Bold.Render("Do not use in production")+`. Press `+
					cliui.Styles.Field.Render("ctrl+c")+` to clean up provisioned infrastructure.`)+"\n\n")
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), cliui.Styles.Wrap.Render(`Run `+cliui.Styles.Code.Render("coder templates init")+
					" in a new terminal to start creating workspaces.")+"\n")
			} else {
				// This is helpful for tests, but can be silently ignored.
				// Coder may be ran as users that don't have permission to write in the homedir,
				// such as via the systemd service.
				_ = config.URL().Write(client.URL.String())

				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), cliui.Styles.Paragraph.Render(cliui.Styles.Wrap.Render(cliui.Styles.Prompt.String()+`Started in `+
					cliui.Styles.Field.Render("production")+` mode. All data is stored in the PostgreSQL provided! Press `+cliui.Styles.Field.Render("ctrl+c")+` to gracefully shutdown.`))+"\n")

				hasFirstUser, err := client.HasFirstUser(cmd.Context())
				if !hasFirstUser && err == nil {
					// This could fail for a variety of TLS-related reasons.
					// This is a helpful starter message, and not critical for user interaction.
					_, _ = fmt.Fprint(cmd.ErrOrStderr(), cliui.Styles.Paragraph.Render(cliui.Styles.Wrap.Render(cliui.Styles.FocusedPrompt.String()+`Run `+cliui.Styles.Code.Render("coder login "+accessURL)+" in a new terminal to get started.\n")))
				}
			}

			// Updates the systemd status from activating to activated.
			_, err = daemon.SdNotify(false, daemon.SdNotifyReady)
			if err != nil {
				return xerrors.Errorf("notify systemd: %w", err)
			}

			stopChan := make(chan os.Signal, 1)
			defer signal.Stop(stopChan)
			signal.Notify(stopChan, os.Interrupt)
			select {
			case <-cmd.Context().Done():
				closeCoderd()
				return cmd.Context().Err()
			case err := <-tunnelErrChan:
				if err != nil {
					return err
				}
			case err := <-errCh:
				closeCoderd()
				return err
			case <-stopChan:
			}
			signal.Stop(stopChan)
			_, err = daemon.SdNotify(false, daemon.SdNotifyStopping)
			if err != nil {
				return xerrors.Errorf("notify systemd: %w", err)
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "\n\n"+cliui.Styles.Bold.Render("Interrupt caught. Gracefully exiting..."))

			if dev {
				organizations, err := client.OrganizationsByUser(cmd.Context(), codersdk.Me)
				if err != nil {
					return xerrors.Errorf("get organizations: %w", err)
				}
				workspaces, err := client.WorkspacesByOwner(cmd.Context(), organizations[0].ID, codersdk.Me)
				if err != nil {
					return xerrors.Errorf("get workspaces: %w", err)
				}
				for _, workspace := range workspaces {
					before := time.Now()
					build, err := client.CreateWorkspaceBuild(cmd.Context(), workspace.ID, codersdk.CreateWorkspaceBuildRequest{
						Transition: database.WorkspaceTransitionDelete,
					})
					if err != nil {
						return xerrors.Errorf("delete workspace: %w", err)
					}

					err = cliui.ProvisionerJob(cmd.Context(), cmd.OutOrStdout(), cliui.ProvisionerJobOptions{
						Fetch: func() (codersdk.ProvisionerJob, error) {
							build, err := client.WorkspaceBuild(cmd.Context(), build.ID)
							return build.Job, err
						},
						Cancel: func() error {
							return client.CancelWorkspaceBuild(cmd.Context(), build.ID)
						},
						Logs: func() (<-chan codersdk.ProvisionerJobLog, error) {
							return client.WorkspaceBuildLogsAfter(cmd.Context(), build.ID, before)
						},
					})
					if err != nil {
						return xerrors.Errorf("delete workspace %s: %w", workspace.Name, err)
					}
				}
			}

			for _, provisionerDaemon := range provisionerDaemons {
				spin := spinner.New(spinner.CharSets[5], 100*time.Millisecond)
				spin.Writer = cmd.OutOrStdout()
				spin.Suffix = cliui.Styles.Keyword.Render(" Shutting down provisioner daemon...")
				spin.Start()
				err = provisionerDaemon.Shutdown(cmd.Context())
				if err != nil {
					spin.FinalMSG = cliui.Styles.Prompt.String() + "Failed to shutdown provisioner daemon: " + err.Error()
					spin.Stop()
				}
				err = provisionerDaemon.Close()
				if err != nil {
					spin.Stop()
					return xerrors.Errorf("close provisioner daemon: %w", err)
				}
				spin.FinalMSG = cliui.Styles.Prompt.String() + "Gracefully shut down provisioner daemon!\n"
				spin.Stop()
			}

			if dev && !skipTunnel {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), cliui.Styles.Prompt.String()+"Waiting for dev tunnel to close...\n")
				closeTunnel()
				<-tunnelErrChan
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), cliui.Styles.Prompt.String()+"Waiting for WebSocket connections to close...\n")
			shutdownConns()
			closeCoderd()
			return nil
		},
	}

	cliflag.StringVarP(root.Flags(), &accessURL, "access-url", "", "CODER_ACCESS_URL", "", "Specifies the external URL to access Coder")
	cliflag.StringVarP(root.Flags(), &address, "address", "a", "CODER_ADDRESS", "127.0.0.1:3000", "The address to serve the API and dashboard")
	// systemd uses the CACHE_DIRECTORY environment variable!
	cliflag.StringVarP(root.Flags(), &cacheDir, "cache-dir", "", "CACHE_DIRECTORY", filepath.Join(os.TempDir(), "coder-cache"), "Specifies a directory to cache binaries for provision operations.")
	cliflag.BoolVarP(root.Flags(), &dev, "dev", "", "CODER_DEV_MODE", false, "Serve Coder in dev mode for tinkering")
	cliflag.StringVarP(root.Flags(), &devUserEmail, "dev-admin-email", "", "CODER_DEV_ADMIN_EMAIL", "admin@coder.com", "Specifies the admin email to be used in dev mode (--dev)")
	cliflag.StringVarP(root.Flags(), &devUserPassword, "dev-admin-password", "", "CODER_DEV_ADMIN_PASSWORD", "", "Specifies the admin password to be used in dev mode (--dev) instead of a randomly generated one")
	cliflag.StringVarP(root.Flags(), &postgresURL, "postgres-url", "", "CODER_PG_CONNECTION_URL", "", "URL of a PostgreSQL database to connect to")
	cliflag.Uint8VarP(root.Flags(), &provisionerDaemonCount, "provisioner-daemons", "", "CODER_PROVISIONER_DAEMONS", 3, "The amount of provisioner daemons to create on start.")
	cliflag.StringVarP(root.Flags(), &oauth2GithubClientID, "oauth2-github-client-id", "", "CODER_OAUTH2_GITHUB_CLIENT_ID", "",
		"Specifies a client ID to use for oauth2 with GitHub.")
	cliflag.StringVarP(root.Flags(), &oauth2GithubClientSecret, "oauth2-github-client-secret", "", "CODER_OAUTH2_GITHUB_CLIENT_SECRET", "",
		"Specifies a client secret to use for oauth2 with GitHub.")
	cliflag.StringArrayVarP(root.Flags(), &oauth2GithubAllowedOrganizations, "oauth2-github-allowed-orgs", "", "CODER_OAUTH2_GITHUB_ALLOWED_ORGS", nil,
		"Specifies organizations the user must be a member of to authenticate with GitHub.")
	cliflag.BoolVarP(root.Flags(), &oauth2GithubAllowSignups, "oauth2-github-allow-signups", "", "CODER_OAUTH2_GITHUB_ALLOW_SIGNUPS", false,
		"Specifies whether new users can sign up with GitHub.")
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
	cliflag.BoolVarP(root.Flags(), &skipTunnel, "skip-tunnel", "", "CODER_DEV_SKIP_TUNNEL", false, "Skip serving dev mode through an exposed tunnel for simple setup.")
	_ = root.Flags().MarkHidden("skip-tunnel")
	cliflag.StringArrayVarP(root.Flags(), &stunServers, "stun-server", "", "CODER_STUN_SERVERS", []string{
		"stun:stun.l.google.com:19302",
	}, "Specify URLs for STUN servers to enable P2P connections.")
	cliflag.BoolVarP(root.Flags(), &traceDatadog, "trace-datadog", "", "CODER_TRACE_DATADOG", false, "Send tracing data to a datadog agent")
	cliflag.StringVarP(root.Flags(), &turnRelayAddress, "turn-relay-address", "", "CODER_TURN_RELAY_ADDRESS", "127.0.0.1",
		"Specifies the address to bind TURN connections.")
	cliflag.BoolVarP(root.Flags(), &secureAuthCookie, "secure-auth-cookie", "", "CODER_SECURE_AUTH_COOKIE", false, "Specifies if the 'Secure' property is set on browser session cookies")
	cliflag.StringVarP(root.Flags(), &sshKeygenAlgorithmRaw, "ssh-keygen-algorithm", "", "CODER_SSH_KEYGEN_ALGORITHM", "ed25519", "Specifies the algorithm to use for generating ssh keys. "+
		`Accepted values are "ed25519", "ecdsa", or "rsa4096"`)
	cliflag.BoolVarP(root.Flags(), &spooky, "spooky", "", "", false, "Specifies spookiness level")
	_ = root.Flags().MarkHidden("spooky")

	return root
}

func createFirstUser(cmd *cobra.Command, client *codersdk.Client, cfg config.Root, email, password string) error {
	if email == "" {
		return xerrors.New("email is empty")
	}
	if password == "" {
		return xerrors.New("password is empty")
	}
	_, err := client.CreateFirstUser(cmd.Context(), codersdk.CreateFirstUserRequest{
		Email:            email,
		Username:         "developer",
		Password:         password,
		OrganizationName: "acme-corp",
	})
	if err != nil {
		return xerrors.Errorf("create first user: %w", err)
	}
	token, err := client.LoginWithPassword(cmd.Context(), codersdk.LoginWithPasswordRequest{
		Email:    email,
		Password: password,
	})
	if err != nil {
		return xerrors.Errorf("login with first user: %w", err)
	}
	client.SessionToken = token.SessionToken

	err = cfg.URL().Write(client.URL.String())
	if err != nil {
		return xerrors.Errorf("write local url: %w", err)
	}
	err = cfg.Session().Write(token.SessionToken)
	if err != nil {
		return xerrors.Errorf("write session token: %w", err)
	}
	return nil
}

func newProvisionerDaemon(ctx context.Context, client *codersdk.Client, logger slog.Logger, cacheDir string, errChan chan error) (*provisionerd.Server, error) {
	err := os.MkdirAll(cacheDir, 0700)
	if err != nil {
		return nil, xerrors.Errorf("mkdir %q: %w", cacheDir, err)
	}

	terraformClient, terraformServer := provisionersdk.TransportPipe()
	go func() {
		err := terraform.Serve(ctx, &terraform.ServeOptions{
			ServeOptions: &provisionersdk.ServeOptions{
				Listener: terraformServer,
			},
			CachePath: cacheDir,
			Logger:    logger,
		})
		if err != nil {
			errChan <- err
		}
	}()

	tempDir, err := os.MkdirTemp("", "provisionerd")
	if err != nil {
		return nil, err
	}

	return provisionerd.New(client.ListenProvisionerDaemon, &provisionerd.Options{
		Logger:         logger,
		PollInterval:   500 * time.Millisecond,
		UpdateInterval: 500 * time.Millisecond,
		Provisioners: provisionerd.Provisioners{
			string(database.ProvisionerTypeTerraform): proto.NewDRPCProvisionerClient(provisionersdk.Conn(terraformClient)),
		},
		WorkDirectory: tempDir,
	}), nil
}

// nolint: revive
func printLogo(cmd *cobra.Command, spooky bool) {
	if spooky {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), `
		▄████▄   ▒█████  ▓█████▄ ▓█████  ██▀███  
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
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), `    ▄█▀    ▀█▄
     ▄▄ ▀▀▀  █▌   ██▀▀█▄          ▐█
 ▄▄██▀▀█▄▄▄  ██  ██      █▀▀█ ▐█▀▀██ ▄█▀▀█ █▀▀
█▌   ▄▌   ▐█ █▌  ▀█▄▄▄█▌ █  █ ▐█  ██ ██▀▀  █
     ██████▀▄█    ▀▀▀▀   ▀▀▀▀  ▀▀▀▀▀  ▀▀▀▀ ▀

`)
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

func configureGithubOAuth2(accessURL *url.URL, clientID, clientSecret string, allowSignups bool, allowOrgs []string) (*coderd.GithubOAuth2Config, error) {
	redirectURL, err := accessURL.Parse("/api/v2/users/oauth2/github/callback")
	if err != nil {
		return nil, xerrors.Errorf("parse github oauth callback url: %w", err)
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
			})
			return memberships, err
		},
	}, nil
}

type datadogLogger struct {
	logger slog.Logger
}

func (d *datadogLogger) Log(msg string) {
	d.logger.Debug(context.Background(), msg)
}

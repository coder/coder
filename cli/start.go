package cli

import (
	"context"
	"database/sql"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"time"

	"github.com/briandowns/spinner"
	"github.com/coreos/go-systemd/daemon"
	"github.com/spf13/cobra"
	"golang.org/x/xerrors"
	"google.golang.org/api/idtoken"
	"google.golang.org/api/option"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/cli/config"
	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/tunnel"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/database"
	"github.com/coder/coder/database/databasefake"
	"github.com/coder/coder/provisioner/terraform"
	"github.com/coder/coder/provisionerd"
	"github.com/coder/coder/provisionersdk"
	"github.com/coder/coder/provisionersdk/proto"
)

func start() *cobra.Command {
	var (
		address                string
		postgresURL            string
		provisionerDaemonCount uint8
		dev                    bool
		useTunnel              bool
	)
	root := &cobra.Command{
		Use: "start",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), `    ▄█▀    ▀█▄
     ▄▄ ▀▀▀  █▌   ██▀▀█▄          ▐█
 ▄▄██▀▀█▄▄▄  ██  ██      █▀▀█ ▐█▀▀██ ▄█▀▀█ █▀▀
█▌   ▄▌   ▐█ █▌  ▀█▄▄▄█▌ █  █ ▐█  ██ ██▀▀  █
     ██████▀▄█    ▀▀▀▀   ▀▀▀▀  ▀▀▀▀▀  ▀▀▀▀ ▀

`)

			if postgresURL == "" {
				// Default to the environment variable!
				postgresURL = os.Getenv("CODER_PG_CONNECTION_URL")
			}

			listener, err := net.Listen("tcp", address)
			if err != nil {
				return xerrors.Errorf("listen %q: %w", address, err)
			}
			defer listener.Close()
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
			accessURL := localURL
			var tunnelErr <-chan error
			// If we're attempting to tunnel in dev-mode, the access URL
			// needs to be changed to use the tunnel.
			if dev && useTunnel {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), cliui.Styles.Paragraph.Render("Coder requires a network endpoint that can be accessed by provisioned workspaces. In dev mode, a free tunnel can be created for you. This will expose your Coder deployment to the internet.")+"\n")

				_, err = cliui.Prompt(cmd, cliui.PromptOptions{
					Text:      "Would you like Coder to start a tunnel for simple setup?",
					IsConfirm: true,
				})
				if err == nil {
					var accessURLRaw string
					accessURLRaw, tunnelErr, err = tunnel.New(cmd.Context(), localURL.String())
					if err != nil {
						return xerrors.Errorf("create tunnel: %w", err)
					}
					accessURL, err = url.Parse(accessURLRaw)
					if err != nil {
						return xerrors.Errorf("parse: %w", err)
					}

					_, _ = fmt.Fprintf(cmd.OutOrStdout(), cliui.Styles.Paragraph.Render(cliui.Styles.Wrap.Render(cliui.Styles.Prompt.String()+`Tunnel started. Your deployment is accessible at:`))+"\n  "+cliui.Styles.Field.Render(accessURL.String()))
				}
			}
			validator, err := idtoken.NewValidator(cmd.Context(), option.WithoutAuthentication())
			if err != nil {
				return err
			}

			logger := slog.Make(sloghuman.Sink(os.Stderr))
			options := &coderd.Options{
				AccessURL:            accessURL,
				Logger:               logger.Named("coderd"),
				Database:             databasefake.New(),
				Pubsub:               database.NewPubsubInMemory(),
				GoogleTokenValidator: validator,
			}

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

			provisionerDaemons := make([]*provisionerd.Server, 0)
			for i := uint8(0); i < provisionerDaemonCount; i++ {
				daemonClose, err := newProvisionerDaemon(cmd.Context(), client, logger)
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

			errCh := make(chan error)
			go func() {
				defer close(errCh)
				errCh <- http.Serve(listener, handler)
			}()

			config := createConfig(cmd)

			if dev {
				err = createFirstUser(cmd, client, config)
				if err != nil {
					return xerrors.Errorf("create first user: %w", err)
				}

				_, _ = fmt.Fprintf(cmd.OutOrStdout(), cliui.Styles.Paragraph.Render(cliui.Styles.Wrap.Render(cliui.Styles.Prompt.String()+`Started in `+
					cliui.Styles.Field.Render("dev")+` mode. All data is in-memory! Do not use in production. Press `+cliui.Styles.Field.Render("ctrl+c")+` to clean up provisioned infrastructure.`))+
					`
`+
					cliui.Styles.Paragraph.Render(cliui.Styles.Wrap.Render(cliui.Styles.Prompt.String()+`Run `+cliui.Styles.Code.Render("coder projects init")+" in a new terminal to get started.\n"))+`
`)
			} else {
				// This is helpful for tests, but can be silently ignored.
				// Coder may be ran as users that don't have permission to write in the homedir,
				// such as via the systemd service.
				_ = config.URL().Write(client.URL.String())

				hasFirstUser, err := client.HasFirstUser(cmd.Context())
				if err != nil {
					return xerrors.Errorf("check for first user: %w", err)
				}

				_, _ = fmt.Fprintf(cmd.OutOrStdout(), cliui.Styles.Paragraph.Render(cliui.Styles.Wrap.Render(cliui.Styles.Prompt.String()+`Started in `+
					cliui.Styles.Field.Render("production")+` mode. All data is stored in the PostgreSQL provided! Press `+cliui.Styles.Field.Render("ctrl+c")+` to gracefully shutdown.`))+"\n")

				if !hasFirstUser {
					_, _ = fmt.Fprint(cmd.OutOrStdout(), cliui.Styles.Paragraph.Render(cliui.Styles.Wrap.Render(cliui.Styles.FocusedPrompt.String()+`Run `+cliui.Styles.Code.Render("coder login "+client.URL.String())+" in a new terminal to get started.\n")))
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
			case err := <-tunnelErr:
				return err
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
				workspaces, err := client.WorkspacesByUser(cmd.Context(), "")
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

					_, err = cliui.Job(cmd, cliui.JobOptions{
						Title: fmt.Sprintf("Deleting workspace %s...", cliui.Styles.Keyword.Render(workspace.Name)),
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

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), cliui.Styles.Prompt.String()+"Waiting for WebSocket connections to close...\n")
			closeCoderd()
			return nil
		},
	}
	defaultAddress := os.Getenv("CODER_ADDRESS")
	if defaultAddress == "" {
		defaultAddress = "127.0.0.1:3000"
	}
	root.Flags().StringVarP(&address, "address", "a", defaultAddress, "The address to serve the API and dashboard.")
	root.Flags().BoolVarP(&dev, "dev", "", false, "Serve Coder in dev mode for tinkering.")
	root.Flags().StringVarP(&postgresURL, "postgres-url", "", "", "URL of a PostgreSQL database to connect to (defaults to $CODER_PG_CONNECTION_URL).")
	root.Flags().Uint8VarP(&provisionerDaemonCount, "provisioner-daemons", "", 1, "The amount of provisioner daemons to create on start.")
	root.Flags().BoolVarP(&useTunnel, "tunnel", "", true, "Serve dev mode through a Cloudflare Tunnel for easy setup.")
	_ = root.Flags().MarkHidden("tunnel")

	return root
}

func createFirstUser(cmd *cobra.Command, client *codersdk.Client, cfg config.Root) error {
	_, err := client.CreateFirstUser(cmd.Context(), codersdk.CreateFirstUserRequest{
		Email:        "admin@coder.com",
		Username:     "developer",
		Password:     "password",
		Organization: "acme-corp",
	})
	if err != nil {
		return xerrors.Errorf("create first user: %w", err)
	}
	token, err := client.LoginWithPassword(cmd.Context(), codersdk.LoginWithPasswordRequest{
		Email:    "admin@coder.com",
		Password: "password",
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

func newProvisionerDaemon(ctx context.Context, client *codersdk.Client, logger slog.Logger) (*provisionerd.Server, error) {
	terraformClient, terraformServer := provisionersdk.TransportPipe()
	go func() {
		err := terraform.Serve(ctx, &terraform.ServeOptions{
			ServeOptions: &provisionersdk.ServeOptions{
				Listener: terraformServer,
			},
			Logger: logger,
		})
		if err != nil {
			panic(err)
		}
	}()
	tempDir, err := ioutil.TempDir("", "provisionerd")
	if err != nil {
		return nil, err
	}
	return provisionerd.New(client.ListenProvisionerDaemon, &provisionerd.Options{
		Logger:         logger,
		PollInterval:   50 * time.Millisecond,
		UpdateInterval: 50 * time.Millisecond,
		Provisioners: provisionerd.Provisioners{
			string(database.ProvisionerTypeTerraform): proto.NewDRPCProvisionerClient(provisionersdk.Conn(terraformClient)),
		},
		WorkDirectory: tempDir,
	}), nil
}

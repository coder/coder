package cli

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"
	"google.golang.org/api/idtoken"
	"google.golang.org/api/option"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/cli/cliui"
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
		address string
		dev     bool
	)
	root := &cobra.Command{
		Use: "start",
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := slog.Make(sloghuman.Sink(os.Stderr))
			listener, err := net.Listen("tcp", address)
			if err != nil {
				return xerrors.Errorf("listen %q: %w", address, err)
			}
			defer listener.Close()

			localURL := &url.URL{
				Scheme: "http",
				Host:   address,
			}
			accessURL := localURL
			var tunnelErr <-chan error
			if dev {
				var accessURLRaw string
				accessURLRaw, tunnelErr, err = tunnel.New(cmd.Context(), localURL.String())
				if err != nil {
					return xerrors.Errorf("create tunnel: %w", err)
				}
				accessURL, err = url.Parse(accessURLRaw)
				if err != nil {
					return xerrors.Errorf("parse: %w", err)
				}

				_, _ = fmt.Fprintf(cmd.OutOrStdout(), `    ▄█▀    ▀█▄
     ▄▄ ▀▀▀  █▌   ██▀▀█▄          ▐█
 ▄▄██▀▀█▄▄▄  ██  ██      █▀▀█ ▐█▀▀██ ▄█▀▀█ █▀▀
█▌   ▄▌   ▐█ █▌  ▀█▄▄▄█▌ █  █ ▐█  ██ ██▀▀  █
     ██████▀▄█    ▀▀▀▀   ▀▀▀▀  ▀▀▀▀▀  ▀▀▀▀ ▀

`+cliui.Styles.Paragraph.Render(cliui.Styles.Wrap.Render(cliui.Styles.Prompt.String()+`Started in `+
					cliui.Styles.Field.Render("dev")+` mode. All data is in-memory! Learn how to setup and manage a production Coder deployment here: `+cliui.Styles.Prompt.Render("https://coder.com/docs/TODO")))+
					`
`+
					cliui.Styles.Paragraph.Render(cliui.Styles.Wrap.Render(cliui.Styles.FocusedPrompt.String()+`Run `+cliui.Styles.Code.Render("coder login "+localURL.String())+" in a new terminal to get started.\n"))+`
`)
			}

			validator, err := idtoken.NewValidator(cmd.Context(), option.WithoutAuthentication())
			if err != nil {
				return err
			}
			logger.Info(cmd.Context(), "opened tunnel", slog.F("url", accessURL.String()))
			handler, closeCoderd := coderd.New(&coderd.Options{
				AccessURL:            accessURL,
				Logger:               logger,
				Database:             databasefake.New(),
				Pubsub:               database.NewPubsubInMemory(),
				GoogleTokenValidator: validator,
			})
			client := codersdk.New(localURL)
			daemonClose, err := newProvisionerDaemon(cmd.Context(), client, logger)
			if err != nil {
				return xerrors.Errorf("create provisioner daemon: %w", err)
			}
			defer daemonClose.Close()

			errCh := make(chan error)
			go func() {
				defer close(errCh)
				errCh <- http.Serve(listener, handler)
			}()

			closeCoderd()
			select {
			case <-cmd.Context().Done():
				return cmd.Context().Err()
			case err := <-tunnelErr:
				return err
			case err := <-errCh:
				return err
			}
		},
	}
	defaultAddress, ok := os.LookupEnv("ADDRESS")
	if !ok {
		defaultAddress = "127.0.0.1:3000"
	}
	root.Flags().StringVarP(&address, "address", "a", defaultAddress, "The address to serve the API and dashboard.")
	root.Flags().BoolVarP(&dev, "dev", "", false, "Serve Coder in dev mode for tinkering.")

	return root
}

func newProvisionerDaemon(ctx context.Context, client *codersdk.Client, logger slog.Logger) (io.Closer, error) {
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

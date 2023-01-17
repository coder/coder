package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	agpl "github.com/coder/coder/cli"
	"github.com/coder/coder/cli/cliflag"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/cli/deployment"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisioner/terraform"
	"github.com/coder/coder/provisionerd"
	provisionerdproto "github.com/coder/coder/provisionerd/proto"
	"github.com/coder/coder/provisionersdk"
	"github.com/coder/coder/provisionersdk/proto"
)

func provisionerDaemons() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "provisionerd",
		Short: "Manage provisioner daemons",
	}
	cmd.AddCommand(provisionerDaemonStart())

	return cmd
}

func provisionerDaemonStart() *cobra.Command {
	var (
		cacheDir     string
		rawTags      []string
		pollInterval time.Duration
		pollJitter   time.Duration
	)
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Run a provisioner daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()

			notifyCtx, notifyStop := signal.NotifyContext(ctx, agpl.InterruptSignals...)
			defer notifyStop()

			client, err := agpl.CreateClient(cmd)
			if err != nil {
				return xerrors.Errorf("create client: %w", err)
			}
			org, err := agpl.CurrentOrganization(cmd, client)
			if err != nil {
				return xerrors.Errorf("get current organization: %w", err)
			}

			tags, err := agpl.ParseProvisionerTags(rawTags)
			if err != nil {
				return err
			}

			err = os.MkdirAll(cacheDir, 0o700)
			if err != nil {
				return xerrors.Errorf("mkdir %q: %w", cacheDir, err)
			}

			terraformClient, terraformServer := provisionersdk.MemTransportPipe()
			go func() {
				<-ctx.Done()
				_ = terraformClient.Close()
				_ = terraformServer.Close()
			}()

			logger := slog.Make(sloghuman.Sink(cmd.ErrOrStderr()))
			errCh := make(chan error, 1)
			go func() {
				defer cancel()

				err := terraform.Serve(ctx, &terraform.ServeOptions{
					ServeOptions: &provisionersdk.ServeOptions{
						Listener: terraformServer,
					},
					CachePath: cacheDir,
					Logger:    logger.Named("terraform"),
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
				return err
			}

			logger.Info(ctx, "starting provisioner daemon", slog.F("tags", tags))

			provisioners := provisionerd.Provisioners{
				string(database.ProvisionerTypeTerraform): proto.NewDRPCProvisionerClient(terraformClient),
			}
			srv := provisionerd.New(func(ctx context.Context) (provisionerdproto.DRPCProvisionerDaemonClient, error) {
				return client.ServeProvisionerDaemon(ctx, org.ID, []codersdk.ProvisionerType{
					codersdk.ProvisionerTypeTerraform,
				}, tags)
			}, &provisionerd.Options{
				Logger:          logger,
				JobPollInterval: pollInterval,
				JobPollJitter:   pollJitter,
				UpdateInterval:  500 * time.Millisecond,
				Provisioners:    provisioners,
				WorkDirectory:   tempDir,
			})

			var exitErr error
			select {
			case <-notifyCtx.Done():
				exitErr = notifyCtx.Err()
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), cliui.Styles.Bold.Render(
					"Interrupt caught, gracefully exiting. Use ctrl+\\ to force quit",
				))
			case exitErr = <-errCh:
			}
			if exitErr != nil && !xerrors.Is(exitErr, context.Canceled) {
				cmd.Printf("Unexpected error, shutting down server: %s\n", exitErr)
			}

			shutdown, shutdownCancel := context.WithTimeout(ctx, time.Minute)
			defer shutdownCancel()
			err = srv.Shutdown(shutdown)
			if err != nil {
				return xerrors.Errorf("shutdown: %w", err)
			}

			cancel()
			if xerrors.Is(exitErr, context.Canceled) {
				return nil
			}
			return exitErr
		},
	}

	cliflag.StringVarP(cmd.Flags(), &cacheDir, "cache-dir", "c", "CODER_CACHE_DIRECTORY", deployment.DefaultCacheDir(),
		"Specify a directory to cache provisioner job files.")
	cliflag.StringArrayVarP(cmd.Flags(), &rawTags, "tag", "t", "CODER_PROVISIONERD_TAGS", []string{},
		"Specify a list of tags to target provisioner jobs.")
	cliflag.DurationVarP(cmd.Flags(), &pollInterval, "poll-interval", "", "CODER_PROVISIONERD_POLL_INTERVAL", time.Second,
		"Specify the interval for which the provisioner daemon should poll for jobs.")
	cliflag.DurationVarP(cmd.Flags(), &pollJitter, "poll-jitter", "", "CODER_PROVISIONERD_POLL_JITTER", 100*time.Millisecond,
		"Random jitter added to the poll interval.")

	return cmd
}

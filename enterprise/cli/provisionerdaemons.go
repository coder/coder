//go:build !slim

package cli

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	agpl "github.com/coder/coder/v2/cli"
	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/cli/cliutil"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/provisionerdserver"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisioner/terraform"
	"github.com/coder/coder/v2/provisionerd"
	provisionerdproto "github.com/coder/coder/v2/provisionerd/proto"
	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
)

func (r *RootCmd) provisionerDaemons() *clibase.Cmd {
	cmd := &clibase.Cmd{
		Use:   "provisionerd",
		Short: "Manage provisioner daemons",
		Handler: func(inv *clibase.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*clibase.Cmd{
			r.provisionerDaemonStart(),
		},
	}

	return cmd
}

func validateProvisionerDaemonName(name string) error {
	if len(name) > 64 {
		return xerrors.Errorf("name cannot be greater than 64 characters in length")
	}
	if ok, err := regexp.MatchString(`^[a-zA-Z0-9][a-zA-Z0-9-]{0,61}[a-zA-Z0-9]$`, name); err != nil || !ok {
		return xerrors.Errorf("name %q is not a valid hostname", name)
	}
	return nil
}

func (r *RootCmd) provisionerDaemonStart() *clibase.Cmd {
	var (
		cacheDir     string
		rawTags      []string
		pollInterval time.Duration
		pollJitter   time.Duration
		preSharedKey string
		name         string
	)
	client := new(codersdk.Client)
	cmd := &clibase.Cmd{
		Use:   "start",
		Short: "Run a provisioner daemon",
		Middleware: clibase.Chain(
			r.InitClientMissingTokenOK(client),
		),
		Handler: func(inv *clibase.Invocation) error {
			ctx, cancel := context.WithCancel(inv.Context())
			defer cancel()

			notifyCtx, notifyStop := inv.SignalNotifyContext(ctx, agpl.InterruptSignals...)
			defer notifyStop()

			tags, err := agpl.ParseProvisionerTags(rawTags)
			if err != nil {
				return err
			}

			if name == "" {
				name = cliutil.Hostname()
			}

			if err := validateProvisionerDaemonName(name); err != nil {
				return err
			}

			logger := slog.Make(sloghuman.Sink(inv.Stderr))
			if ok, _ := inv.ParsedFlags().GetBool("verbose"); ok {
				logger = logger.Leveled(slog.LevelDebug)
			}

			if len(tags) != 0 {
				logger.Info(ctx, "note: tagged provisioners can currently pick up jobs from untagged templates")
				logger.Info(ctx, "see https://github.com/coder/coder/issues/6442 for details")
			}

			// When authorizing with a PSK, we automatically scope the provisionerd
			// to organization. Scoping to user with PSK auth is not a valid configuration.
			if preSharedKey != "" {
				logger.Info(ctx, "psk auth automatically sets tag "+provisionerdserver.TagScope+"="+provisionerdserver.ScopeOrganization)
				tags[provisionerdserver.TagScope] = provisionerdserver.ScopeOrganization
			}

			err = os.MkdirAll(cacheDir, 0o700)
			if err != nil {
				return xerrors.Errorf("mkdir %q: %w", cacheDir, err)
			}

			tempDir, err := os.MkdirTemp("", "provisionerd")
			if err != nil {
				return err
			}

			terraformClient, terraformServer := provisionersdk.MemTransportPipe()
			go func() {
				<-ctx.Done()
				_ = terraformClient.Close()
				_ = terraformServer.Close()
			}()

			errCh := make(chan error, 1)
			go func() {
				defer cancel()

				err := terraform.Serve(ctx, &terraform.ServeOptions{
					ServeOptions: &provisionersdk.ServeOptions{
						Listener:      terraformServer,
						Logger:        logger.Named("terraform"),
						WorkDirectory: tempDir,
					},
					CachePath: cacheDir,
				})
				if err != nil && !xerrors.Is(err, context.Canceled) {
					select {
					case errCh <- err:
					default:
					}
				}
			}()

			logger.Info(ctx, "starting provisioner daemon", slog.F("tags", tags), slog.F("name", name))

			connector := provisionerd.LocalProvisioners{
				string(database.ProvisionerTypeTerraform): proto.NewDRPCProvisionerClient(terraformClient),
			}
			id := uuid.New()
			srv := provisionerd.New(func(ctx context.Context) (provisionerdproto.DRPCProvisionerDaemonClient, error) {
				return client.ServeProvisionerDaemon(ctx, codersdk.ServeProvisionerDaemonRequest{
					ID:   id,
					Name: name,
					Provisioners: []codersdk.ProvisionerType{
						codersdk.ProvisionerTypeTerraform,
					},
					Tags:         tags,
					PreSharedKey: preSharedKey,
				})
			}, &provisionerd.Options{
				Logger:         logger,
				UpdateInterval: 500 * time.Millisecond,
				Connector:      connector,
			})

			var exitErr error
			select {
			case <-notifyCtx.Done():
				exitErr = notifyCtx.Err()
				_, _ = fmt.Fprintln(inv.Stdout, cliui.Bold(
					"Interrupt caught, gracefully exiting. Use ctrl+\\ to force quit",
				))
			case exitErr = <-errCh:
			}
			if exitErr != nil && !xerrors.Is(exitErr, context.Canceled) {
				cliui.Errorf(inv.Stderr, "Unexpected error, shutting down server: %s\n", exitErr)
			}

			err = srv.Shutdown(ctx)
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

	cmd.Options = clibase.OptionSet{
		{
			Flag:          "cache-dir",
			FlagShorthand: "c",
			Env:           "CODER_CACHE_DIRECTORY",
			Description:   "Directory to store cached data.",
			Default:       codersdk.DefaultCacheDir(),
			Value:         clibase.StringOf(&cacheDir),
		},
		{
			Flag:          "tag",
			FlagShorthand: "t",
			Env:           "CODER_PROVISIONERD_TAGS",
			Description:   "Tags to filter provisioner jobs by.",
			Value:         clibase.StringArrayOf(&rawTags),
		},
		{
			Flag:        "poll-interval",
			Env:         "CODER_PROVISIONERD_POLL_INTERVAL",
			Default:     time.Second.String(),
			Description: "Deprecated and ignored.",
			Value:       clibase.DurationOf(&pollInterval),
		},
		{
			Flag:        "poll-jitter",
			Env:         "CODER_PROVISIONERD_POLL_JITTER",
			Description: "Deprecated and ignored.",
			Default:     (100 * time.Millisecond).String(),
			Value:       clibase.DurationOf(&pollJitter),
		},
		{
			Flag:        "psk",
			Env:         "CODER_PROVISIONER_DAEMON_PSK",
			Description: "Pre-shared key to authenticate with Coder server.",
			Value:       clibase.StringOf(&preSharedKey),
		},
		{
			Flag:        "name",
			Env:         "CODER_PROVISIONER_DAEMON_NAME",
			Description: "Name of this provisioner daemon. Defaults to the current hostname without FQDN.",
			Value:       clibase.StringOf(&name),
			Default:     "",
		},
	}

	return cmd
}

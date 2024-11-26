//go:build !slim

package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	agpl "github.com/coder/coder/v2/cli"
	"github.com/coder/coder/v2/cli/clilog"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/cli/cliutil"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/drpc"
	"github.com/coder/coder/v2/provisioner/terraform"
	"github.com/coder/coder/v2/provisionerd"
	provisionerdproto "github.com/coder/coder/v2/provisionerd/proto"
	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/serpent"
)

func (r *RootCmd) provisionerDaemonStart() *serpent.Command {
	var (
		cacheDir       string
		logHuman       string
		logJSON        string
		logStackdriver string
		logFilter      []string
		name           string
		rawTags        []string
		pollInterval   time.Duration
		pollJitter     time.Duration
		preSharedKey   string
		provisionerKey string
		verbose        bool

		prometheusEnable  bool
		prometheusAddress string
	)
	orgContext := agpl.NewOrganizationContext()
	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Use:   "start",
		Short: "Run a provisioner daemon",
		Middleware: serpent.Chain(
			// disable checks and warnings because this command starts a daemon; it is
			// not meant for humans typing commands.  Furthermore, the checks are
			// incompatible with PSK auth that this command uses
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			ctx, cancel := context.WithCancel(inv.Context())
			defer cancel()

			stopCtx, stopCancel := inv.SignalNotifyContext(ctx, agpl.StopSignalsNoInterrupt...)
			defer stopCancel()
			interruptCtx, interruptCancel := inv.SignalNotifyContext(ctx, agpl.InterruptSignals...)
			defer interruptCancel()

			orgID := uuid.Nil
			if preSharedKey == "" && provisionerKey == "" {
				// We can only select an organization if using user auth
				org, err := orgContext.Selected(inv, client)
				if err != nil {
					var cErr *codersdk.Error
					if !errors.As(err, &cErr) || cErr.StatusCode() != http.StatusUnauthorized {
						return xerrors.Errorf("current organization: %w", err)
					}

					return xerrors.New("must provide a pre-shared key or provisioner key when not authenticated as a user")
				}

				orgID = org.ID
			} else if orgContext.FlagSelect != "" {
				return xerrors.New("cannot provide --org value with --psk or --key flags")
			}

			if provisionerKey != "" {
				if preSharedKey != "" {
					return xerrors.New("cannot provide both provisioner key --key and pre-shared key --psk")
				}
				if len(rawTags) > 0 {
					return xerrors.New("cannot provide tags when using provisioner key")
				}
			}

			tags, err := agpl.ParseProvisionerTags(rawTags)
			if err != nil {
				return err
			}

			displayedTags := make(map[string]string, len(tags))
			if provisionerKey != "" {
				pkDetails, err := client.GetProvisionerKey(ctx, provisionerKey)
				if err != nil {
					return xerrors.New("unable to get provisioner key details")
				}

				for k, v := range pkDetails.Tags {
					displayedTags[k] = v
				}
			} else {
				for key, val := range tags {
					displayedTags[key] = val
				}
			}

			if name == "" {
				name = cliutil.Hostname()
			}

			if err := validateProvisionerDaemonName(name); err != nil {
				return err
			}

			logOpts := []clilog.Option{
				clilog.WithFilter(logFilter...),
				clilog.WithHuman(logHuman),
				clilog.WithJSON(logJSON),
				clilog.WithStackdriver(logStackdriver),
			}
			if verbose {
				logOpts = append(logOpts, clilog.WithVerbose())
			}

			logger, closeLogger, err := clilog.New(logOpts...).Build(inv)
			if err != nil {
				// Fall back to a basic logger
				logger = slog.Make(sloghuman.Sink(inv.Stderr))
				logger.Error(ctx, "failed to initialize logger", slog.Error(err))
			} else {
				defer closeLogger()
			}

			if len(displayedTags) == 0 {
				logger.Info(ctx, "note: untagged provisioners can only pick up jobs from untagged templates")
			}

			// When authorizing with a PSK / provisioner key, we automatically scope the provisionerd
			// to organization. Scoping to user with PSK / provisioner key auth is not a valid configuration.
			if preSharedKey != "" {
				logger.Info(ctx, "psk automatically sets tag "+provisionersdk.TagScope+"="+provisionersdk.ScopeOrganization)
				tags[provisionersdk.TagScope] = provisionersdk.ScopeOrganization
			}
			if provisionerKey != "" {
				logger.Info(ctx, "provisioner key auth automatically sets tag "+provisionersdk.TagScope+" empty")
				// no scope tag will default to org scope
				delete(tags, provisionersdk.TagScope)
			}

			err = os.MkdirAll(cacheDir, 0o700)
			if err != nil {
				return xerrors.Errorf("mkdir %q: %w", cacheDir, err)
			}

			tempDir, err := os.MkdirTemp("", "provisionerd")
			if err != nil {
				return err
			}

			terraformClient, terraformServer := drpc.MemTransportPipe()
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

			var metrics *provisionerd.Metrics
			if prometheusEnable {
				logger.Info(ctx, "starting Prometheus endpoint", slog.F("address", prometheusAddress))

				prometheusRegistry := prometheus.NewRegistry()
				prometheusRegistry.MustRegister(collectors.NewGoCollector())
				prometheusRegistry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

				m := provisionerd.NewMetrics(prometheusRegistry)
				m.Runner.NumDaemons.Set(float64(1)) // Set numDaemons to 1 as this is standalone mode.
				metrics = &m

				closeFunc := agpl.ServeHandler(ctx, logger, promhttp.InstrumentMetricHandler(
					prometheusRegistry, promhttp.HandlerFor(prometheusRegistry, promhttp.HandlerOpts{}),
				), prometheusAddress, "prometheus")
				defer closeFunc()
			}

			logger.Info(ctx, "starting provisioner daemon", slog.F("tags", displayedTags), slog.F("name", name))

			connector := provisionerd.LocalProvisioners{
				string(database.ProvisionerTypeTerraform): proto.NewDRPCProvisionerClient(terraformClient),
			}
			srv := provisionerd.New(func(ctx context.Context) (provisionerdproto.DRPCProvisionerDaemonClient, error) {
				return client.ServeProvisionerDaemon(ctx, codersdk.ServeProvisionerDaemonRequest{
					ID:   uuid.New(),
					Name: name,
					Provisioners: []codersdk.ProvisionerType{
						codersdk.ProvisionerTypeTerraform,
					},
					Tags:           tags,
					PreSharedKey:   preSharedKey,
					Organization:   orgID,
					ProvisionerKey: provisionerKey,
				})
			}, &provisionerd.Options{
				Logger:         logger,
				UpdateInterval: 500 * time.Millisecond,
				Connector:      connector,
				Metrics:        metrics,
			})

			waitForProvisionerJobs := false
			var exitErr error
			select {
			case <-stopCtx.Done():
				exitErr = stopCtx.Err()
				_, _ = fmt.Fprintln(inv.Stdout, cliui.Bold(
					"Stop caught, waiting for provisioner jobs to complete and gracefully exiting. Use ctrl+\\ to force quit",
				))
				waitForProvisionerJobs = true
			case <-interruptCtx.Done():
				exitErr = interruptCtx.Err()
				_, _ = fmt.Fprintln(inv.Stdout, cliui.Bold(
					"Interrupt caught, gracefully exiting. Use ctrl+\\ to force quit",
				))
			case exitErr = <-errCh:
			}
			if exitErr != nil && !xerrors.Is(exitErr, context.Canceled) {
				cliui.Errorf(inv.Stderr, "Unexpected error, shutting down server: %s\n", exitErr)
			}

			err = srv.Shutdown(ctx, !waitForProvisionerJobs)
			if err != nil {
				return xerrors.Errorf("shutdown: %w", err)
			}

			// Shutdown does not call close. Must call it manually.
			err = srv.Close()
			if err != nil {
				return xerrors.Errorf("close server: %w", err)
			}

			cancel()
			if xerrors.Is(exitErr, context.Canceled) {
				return nil
			}
			return exitErr
		},
	}

	keyOption := serpent.Option{
		Flag:        "key",
		Env:         "CODER_PROVISIONER_DAEMON_KEY",
		Description: "Provisioner key to authenticate with Coder server.",
		Value:       serpent.StringOf(&provisionerKey),
	}
	cmd.Options = serpent.OptionSet{
		{
			Flag:          "cache-dir",
			FlagShorthand: "c",
			Env:           "CODER_CACHE_DIRECTORY",
			Description:   "Directory to store cached data.",
			Default:       codersdk.DefaultCacheDir(),
			Value:         serpent.StringOf(&cacheDir),
		},
		{
			Flag:          "tag",
			FlagShorthand: "t",
			Env:           "CODER_PROVISIONERD_TAGS",
			Description:   "Tags to filter provisioner jobs by.",
			Value:         serpent.StringArrayOf(&rawTags),
		},
		{
			Flag:        "poll-interval",
			Env:         "CODER_PROVISIONERD_POLL_INTERVAL",
			Default:     time.Second.String(),
			Description: "Deprecated and ignored.",
			Value:       serpent.DurationOf(&pollInterval),
		},
		{
			Flag:        "poll-jitter",
			Env:         "CODER_PROVISIONERD_POLL_JITTER",
			Description: "Deprecated and ignored.",
			Default:     (100 * time.Millisecond).String(),
			Value:       serpent.DurationOf(&pollJitter),
		},
		{
			Flag:        "psk",
			Env:         "CODER_PROVISIONER_DAEMON_PSK",
			Description: "Pre-shared key to authenticate with Coder server.",
			Value:       serpent.StringOf(&preSharedKey),
			UseInstead:  []serpent.Option{keyOption},
		},
		keyOption,
		{
			Flag:        "name",
			Env:         "CODER_PROVISIONER_DAEMON_NAME",
			Description: "Name of this provisioner daemon. Defaults to the current hostname without FQDN.",
			Value:       serpent.StringOf(&name),
			Default:     "",
		},
		{
			Flag:        "verbose",
			Env:         "CODER_PROVISIONER_DAEMON_VERBOSE",
			Description: "Output debug-level logs.",
			Value:       serpent.BoolOf(&verbose),
			Default:     "false",
		},
		{
			Flag:        "log-human",
			Env:         "CODER_PROVISIONER_DAEMON_LOGGING_HUMAN",
			Description: "Output human-readable logs to a given file.",
			Value:       serpent.StringOf(&logHuman),
			Default:     "/dev/stderr",
		},
		{
			Flag:        "log-json",
			Env:         "CODER_PROVISIONER_DAEMON_LOGGING_JSON",
			Description: "Output JSON logs to a given file.",
			Value:       serpent.StringOf(&logJSON),
			Default:     "",
		},
		{
			Flag:        "log-stackdriver",
			Env:         "CODER_PROVISIONER_DAEMON_LOGGING_STACKDRIVER",
			Description: "Output Stackdriver compatible logs to a given file.",
			Value:       serpent.StringOf(&logStackdriver),
			Default:     "",
		},
		{
			Flag:        "log-filter",
			Env:         "CODER_PROVISIONER_DAEMON_LOG_FILTER",
			Description: "Filter debug logs by matching against a given regex. Use .* to match all debug logs.",
			Value:       serpent.StringArrayOf(&logFilter),
			Default:     "",
		},
		{
			Flag:        "prometheus-enable",
			Env:         "CODER_PROMETHEUS_ENABLE",
			Description: "Serve prometheus metrics on the address defined by prometheus address.",
			Value:       serpent.BoolOf(&prometheusEnable),
			Default:     "false",
		},
		{
			Flag:        "prometheus-address",
			Env:         "CODER_PROMETHEUS_ADDRESS",
			Description: "The bind address to serve prometheus metrics.",
			Value:       serpent.StringOf(&prometheusAddress),
			Default:     "127.0.0.1:2112",
		},
	}
	orgContext.AttachOptions(cmd)

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

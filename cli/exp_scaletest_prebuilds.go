//go:build !slim

package cli

import (
	"fmt"
	"net/http"
	"os/signal"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/harness"
	"github.com/coder/coder/v2/scaletest/prebuilds"
	"github.com/coder/quartz"
	"github.com/coder/serpent"
)

func (r *RootCmd) scaletestPrebuilds() *serpent.Command {
	var (
		numTemplates              int64
		numPresets                int64
		numPresetPrebuilds        int64
		templateVersionJobTimeout time.Duration
		prebuildWorkspaceTimeout  time.Duration
		noCleanup                 bool

		tracingFlags    = &scaletestTracingFlags{}
		timeoutStrategy = &timeoutFlags{}
		cleanupStrategy = newScaletestCleanupStrategy()
		output          = &scaletestOutputFlags{}
		prometheusFlags = &scaletestPrometheusFlags{}
	)

	cmd := &serpent.Command{
		Use:   "prebuilds",
		Short: "Creates prebuild workspaces on the Coder server.",
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			notifyCtx, stop := signal.NotifyContext(ctx, StopSignals...)
			defer stop()
			ctx = notifyCtx

			me, err := requireAdmin(ctx, client)
			if err != nil {
				return err
			}

			client.HTTPClient = &http.Client{
				Transport: &codersdk.HeaderTransport{
					Transport: http.DefaultTransport,
					Header: map[string][]string{
						codersdk.BypassRatelimitHeader: {"true"},
					},
				},
			}

			if numTemplates <= 0 {
				return xerrors.Errorf("--num-templates must be greater than 0")
			}
			if numPresets <= 0 {
				return xerrors.Errorf("--num-presets must be greater than 0")
			}
			if numPresetPrebuilds <= 0 {
				return xerrors.Errorf("--num-preset-prebuilds must be greater than 0")
			}

			outputs, err := output.parse()
			if err != nil {
				return xerrors.Errorf("parse output flags: %w", err)
			}

			tracerProvider, closeTracing, tracingEnabled, err := tracingFlags.provider(ctx)
			if err != nil {
				return xerrors.Errorf("create tracer provider: %w", err)
			}
			defer func() {
				_, _ = fmt.Fprintln(inv.Stderr, "\nUploading traces...")
				if err := closeTracing(ctx); err != nil {
					_, _ = fmt.Fprintf(inv.Stderr, "\nError uploading traces: %+v\n", err)
				}
				_, _ = fmt.Fprintf(inv.Stderr, "Waiting %s for prometheus metrics to be scraped\n", prometheusFlags.Wait)
				<-time.After(prometheusFlags.Wait)
			}()
			tracer := tracerProvider.Tracer(scaletestTracerName)

			reg := prometheus.NewRegistry()
			metrics := prebuilds.NewMetrics(reg)

			logger := inv.Logger
			prometheusSrvClose := ServeHandler(ctx, logger, promhttp.HandlerFor(reg, promhttp.HandlerOpts{}), prometheusFlags.Address, "prometheus")
			defer prometheusSrvClose()

			err = client.PutPrebuildsSettings(ctx, codersdk.PrebuildsSettings{
				ReconciliationPaused: true,
			})
			if err != nil {
				return xerrors.Errorf("pause prebuilds: %w", err)
			}

			setupBarrier := new(sync.WaitGroup)
			setupBarrier.Add(int(numTemplates))
			creationBarrier := new(sync.WaitGroup)
			creationBarrier.Add(int(numTemplates))
			deletionBarrier := new(sync.WaitGroup)
			deletionBarrier.Add(int(numTemplates))

			th := harness.NewTestHarness(timeoutStrategy.wrapStrategy(harness.ConcurrentExecutionStrategy{}), cleanupStrategy.toStrategy())

			for i := range numTemplates {
				id := strconv.Itoa(int(i))
				cfg := prebuilds.Config{
					OrganizationID:            me.OrganizationIDs[0],
					NumPresets:                int(numPresets),
					NumPresetPrebuilds:        int(numPresetPrebuilds),
					TemplateVersionJobTimeout: templateVersionJobTimeout,
					PrebuildWorkspaceTimeout:  prebuildWorkspaceTimeout,
					Metrics:                   metrics,
					SetupBarrier:              setupBarrier,
					CreationBarrier:           creationBarrier,
					DeletionBarrier:           deletionBarrier,
					Clock:                     quartz.NewReal(),
				}
				err := cfg.Validate()
				if err != nil {
					return xerrors.Errorf("validate config: %w", err)
				}

				var runner harness.Runnable = prebuilds.NewRunner(client, cfg)
				if tracingEnabled {
					runner = &runnableTraceWrapper{
						tracer:   tracer,
						spanName: fmt.Sprintf("prebuilds/%s", id),
						runner:   runner,
					}
				}

				th.AddRun("prebuilds", id, runner)
			}

			_, _ = fmt.Fprintf(inv.Stderr, "Creating %d templates with %d presets and %d prebuilds per preset...\n",
				numTemplates, numPresets, numPresetPrebuilds)
			_, _ = fmt.Fprintf(inv.Stderr, "Total expected prebuilds: %d\n", numTemplates*numPresets*numPresetPrebuilds)

			testCtx, testCancel := timeoutStrategy.toContext(ctx)
			defer testCancel()

			runErrCh := make(chan error, 1)
			go func() {
				runErrCh <- th.Run(testCtx)
			}()

			_, _ = fmt.Fprintln(inv.Stderr, "Waiting for all templates to be created...")
			setupBarrier.Wait()
			_, _ = fmt.Fprintln(inv.Stderr, "All templates created")

			err = client.PutPrebuildsSettings(ctx, codersdk.PrebuildsSettings{
				ReconciliationPaused: false,
			})
			if err != nil {
				return xerrors.Errorf("resume prebuilds: %w", err)
			}

			_, _ = fmt.Fprintln(inv.Stderr, "Waiting for all prebuilds to be created...")
			creationBarrier.Wait()
			_, _ = fmt.Fprintln(inv.Stderr, "All prebuilds created")

			err = client.PutPrebuildsSettings(ctx, codersdk.PrebuildsSettings{
				ReconciliationPaused: true,
			})
			if err != nil {
				return xerrors.Errorf("pause prebuilds before deletion: %w", err)
			}

			_, _ = fmt.Fprintln(inv.Stderr, "Waiting for all templates to be updated with 0 prebuilds...")
			deletionBarrier.Wait()
			_, _ = fmt.Fprintln(inv.Stderr, "All templates updated")

			err = client.PutPrebuildsSettings(ctx, codersdk.PrebuildsSettings{
				ReconciliationPaused: false,
			})
			if err != nil {
				return xerrors.Errorf("resume prebuilds for deletion: %w", err)
			}

			_, _ = fmt.Fprintln(inv.Stderr, "Waiting for all prebuilds to be deleted...")
			err = <-runErrCh
			if err != nil {
				return xerrors.Errorf("run test harness (harness failure, not a test failure): %w", err)
			}

			// If the command was interrupted, skip cleanup & stats
			if notifyCtx.Err() != nil {
				return notifyCtx.Err()
			}

			if !noCleanup {
				_, _ = fmt.Fprintln(inv.Stderr, "\nStarting cleanup (deleting templates)...")

				cleanupCtx, cleanupCancel := cleanupStrategy.toContext(ctx)
				defer cleanupCancel()

				err = th.Cleanup(cleanupCtx)
				if err != nil {
					return xerrors.Errorf("cleanup tests: %w", err)
				}

				// If the cleanup was interrupted, skip stats
				if notifyCtx.Err() != nil {
					return notifyCtx.Err()
				}
			}

			res := th.Results()
			for _, o := range outputs {
				err = o.write(res, inv.Stdout)
				if err != nil {
					return xerrors.Errorf("write output %q to %q: %w", o.format, o.path, err)
				}
			}

			if res.TotalFail > 0 {
				return xerrors.New("prebuild creation test failed, see above for more details")
			}

			return nil
		},
	}

	cmd.Options = serpent.OptionSet{
		{
			Flag:        "num-templates",
			Env:         "CODER_SCALETEST_PREBUILDS_NUM_TEMPLATES",
			Default:     "1",
			Description: "Number of templates to create for the test.",
			Value:       serpent.Int64Of(&numTemplates),
		},
		{
			Flag:        "num-presets",
			Env:         "CODER_SCALETEST_PREBUILDS_NUM_PRESETS",
			Default:     "1",
			Description: "Number of presets per template.",
			Value:       serpent.Int64Of(&numPresets),
		},
		{
			Flag:        "num-preset-prebuilds",
			Env:         "CODER_SCALETEST_PREBUILDS_NUM_PRESET_PREBUILDS",
			Default:     "1",
			Description: "Number of prebuilds per preset.",
			Value:       serpent.Int64Of(&numPresetPrebuilds),
		},
		{
			Flag:        "template-version-job-timeout",
			Env:         "CODER_SCALETEST_PREBUILDS_TEMPLATE_VERSION_JOB_TIMEOUT",
			Default:     "5m",
			Description: "Timeout for template version provisioning jobs.",
			Value:       serpent.DurationOf(&templateVersionJobTimeout),
		},
		{
			Flag:        "prebuild-workspace-timeout",
			Env:         "CODER_SCALETEST_PREBUILDS_WORKSPACE_TIMEOUT",
			Default:     "10m",
			Description: "Timeout for all prebuild workspaces to be created/deleted.",
			Value:       serpent.DurationOf(&prebuildWorkspaceTimeout),
		},
		{
			Flag:        "skip-cleanup",
			Env:         "CODER_SCALETEST_PREBUILDS_SKIP_CLEANUP",
			Description: "Skip cleanup (deletion test) and leave resources intact.",
			Value:       serpent.BoolOf(&noCleanup),
		},
	}

	tracingFlags.attach(&cmd.Options)
	timeoutStrategy.attach(&cmd.Options)
	cleanupStrategy.attach(&cmd.Options)
	output.attach(&cmd.Options)
	prometheusFlags.attach(&cmd.Options)

	return cmd
}

//go:build !slim

package cli

import (
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"
	"github.com/coder/coder/v2/scaletest/dynamicparameters"
	"github.com/coder/coder/v2/scaletest/harness"
	"github.com/coder/coder/v2/scaletest/loadtestutil"
	"github.com/coder/serpent"
)

const (
	dynamicParametersTestName = "dynamic-parameters"
)

func (r *RootCmd) scaletestDynamicParameters() *serpent.Command {
	var (
		templateName    string
		provisionerTags []string
		numEvals        int64
		tracingFlags    = &scaletestTracingFlags{}
		prometheusFlags = &scaletestPrometheusFlags{}
		// This test requires unlimited concurrency
		timeoutStrategy = &timeoutFlags{}
	)
	orgContext := NewOrganizationContext()
	output := &scaletestOutputFlags{}

	cmd := &serpent.Command{
		Use:   "dynamic-parameters",
		Short: "Generates load on the Coder server evaluating dynamic parameters",
		Long:  `It is recommended that all rate limits are disabled on the server before running this scaletest. This test generates many login events which will be rate limited against the (most likely single) IP.`,
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()

			outputs, err := output.parse()
			if err != nil {
				return xerrors.Errorf("could not parse --output flags")
			}

			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}
			if templateName == "" {
				return xerrors.Errorf("template cannot be empty")
			}

			tags, err := ParseProvisionerTags(provisionerTags)
			if err != nil {
				return err
			}

			org, err := orgContext.Selected(inv, client)
			if err != nil {
				return err
			}

			_, err = requireAdmin(ctx, client)
			if err != nil {
				return err
			}

			reg := prometheus.NewRegistry()
			metrics := dynamicparameters.NewMetrics(reg, "concurrent_evaluations")

			logger := slog.Make(sloghuman.Sink(inv.Stdout)).Leveled(slog.LevelDebug)
			prometheusSrvClose := ServeHandler(ctx, logger, promhttp.HandlerFor(reg, promhttp.HandlerOpts{}), prometheusFlags.Address, "prometheus")
			defer prometheusSrvClose()

			tracerProvider, closeTracing, tracingEnabled, err := tracingFlags.provider(ctx)
			if err != nil {
				return xerrors.Errorf("create tracer provider: %w", err)
			}
			defer func() {
				// Allow time for traces to flush even if command context is
				// canceled. This is a no-op if tracing is not enabled.
				_, _ = fmt.Fprintln(inv.Stderr, "\nUploading traces...")
				if err := closeTracing(ctx); err != nil {
					_, _ = fmt.Fprintf(inv.Stderr, "\nError uploading traces: %+v\n", err)
				}
				// Wait for prometheus metrics to be scraped
				_, _ = fmt.Fprintf(inv.Stderr, "Waiting %s for prometheus metrics to be scraped\n", prometheusFlags.Wait)
				<-time.After(prometheusFlags.Wait)
			}()
			tracer := tracerProvider.Tracer(scaletestTracerName)

			partitions, err := dynamicparameters.SetupPartitions(ctx, client, org.ID, templateName, tags, numEvals, logger)
			if err != nil {
				return xerrors.Errorf("setup dynamic parameters partitions: %w", err)
			}

			th := harness.NewTestHarness(
				timeoutStrategy.wrapStrategy(harness.ConcurrentExecutionStrategy{}),
				// there is no cleanup since it's just a connection that we sever.
				nil)

			for i, part := range partitions {
				for j := range part.ConcurrentEvaluations {
					cfg := dynamicparameters.Config{
						TemplateVersion:   part.TemplateVersion.ID,
						Metrics:           metrics,
						MetricLabelValues: []string{fmt.Sprintf("%d", part.ConcurrentEvaluations)},
					}
					// use an independent client for each Runner, so they don't reuse TCP connections. This can lead to
					// requests being unbalanced among Coder instances.
					runnerClient, err := loadtestutil.DupClientCopyingHeaders(client, BypassHeader)
					if err != nil {
						return xerrors.Errorf("create runner client: %w", err)
					}
					var runner harness.Runnable = dynamicparameters.NewRunner(runnerClient, cfg)
					if tracingEnabled {
						runner = &runnableTraceWrapper{
							tracer:   tracer,
							spanName: fmt.Sprintf("%s/%d/%d", dynamicParametersTestName, i, j),
							runner:   runner,
						}
					}
					th.AddRun(dynamicParametersTestName, fmt.Sprintf("%d/%d", j, i), runner)
				}
			}

			testCtx, testCancel := timeoutStrategy.toContext(ctx)
			defer testCancel()
			err = th.Run(testCtx)
			if err != nil {
				return xerrors.Errorf("run test harness: %w", err)
			}

			res := th.Results()
			for _, o := range outputs {
				err = o.write(res, inv.Stdout)
				if err != nil {
					return xerrors.Errorf("write output %q to %q: %w", o.format, o.path, err)
				}
			}

			return nil
		},
	}

	cmd.Options = serpent.OptionSet{
		{
			Flag:        "template",
			Description: "Name of the template to use. If it does not exist, it will be created.",
			Default:     "scaletest-dynamic-parameters",
			Value:       serpent.StringOf(&templateName),
		},
		{
			Flag:        "concurrent-evaluations",
			Description: "Number of concurrent dynamic parameter evaluations to perform.",
			Default:     "100",
			Value:       serpent.Int64Of(&numEvals),
		},
		{
			Flag:        "provisioner-tag",
			Description: "Specify a set of tags to target provisioner daemons.",
			Value:       serpent.StringArrayOf(&provisionerTags),
		},
	}
	orgContext.AttachOptions(cmd)
	output.attach(&cmd.Options)
	tracingFlags.attach(&cmd.Options)
	prometheusFlags.attach(&cmd.Options)
	timeoutStrategy.attach(&cmd.Options)
	return cmd
}

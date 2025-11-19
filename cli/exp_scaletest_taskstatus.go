//go:build !slim

package cli

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/serpent"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/harness"
	"github.com/coder/coder/v2/scaletest/taskstatus"
)

const (
	taskStatusTestName = "task-status"
)

func (r *RootCmd) scaletestTaskStatus() *serpent.Command {
	var (
		count                int64
		template             string
		workspaceNamePrefix  string
		appSlug              string
		reportStatusPeriod   time.Duration
		reportStatusDuration time.Duration
		baselineDuration     time.Duration
		tracingFlags         = &scaletestTracingFlags{}
		prometheusFlags      = &scaletestPrometheusFlags{}
		timeoutStrategy      = &timeoutFlags{}
		cleanupStrategy      = newScaletestCleanupStrategy()
		output               = &scaletestOutputFlags{}
	)
	orgContext := NewOrganizationContext()

	cmd := &serpent.Command{
		Use:   "task-status",
		Short: "Generates load on the Coder server by simulating task status reporting",
		Long: `This test creates external workspaces and simulates AI agents reporting task status.
After all runners connect, it waits for the baseline duration before triggering status reporting.`,
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()

			outputs, err := output.parse()
			if err != nil {
				return xerrors.Errorf("could not parse --output flags: %w", err)
			}

			client, err := r.InitClient(inv)
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

			// Disable rate limits for this test
			client.HTTPClient = &http.Client{
				Transport: &codersdk.HeaderTransport{
					Transport: http.DefaultTransport,
					Header: map[string][]string{
						codersdk.BypassRatelimitHeader: {"true"},
					},
				},
			}

			// Find the template
			tpl, err := parseTemplate(ctx, client, []uuid.UUID{org.ID}, template)
			if err != nil {
				return xerrors.Errorf("parse template %q: %w", template, err)
			}
			templateID := tpl.ID

			reg := prometheus.NewRegistry()
			metrics := taskstatus.NewMetrics(reg)

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

			// Setup shared resources for coordination
			connectedWaitGroup := &sync.WaitGroup{}
			connectedWaitGroup.Add(int(count))
			startReporting := make(chan struct{})

			// Create the test harness
			th := harness.NewTestHarness(
				timeoutStrategy.wrapStrategy(harness.ConcurrentExecutionStrategy{}),
				cleanupStrategy.toStrategy(),
			)

			// Create runners
			for i := range count {
				workspaceName := fmt.Sprintf("%s-%d", workspaceNamePrefix, i)
				cfg := taskstatus.Config{
					TemplateID:           templateID,
					WorkspaceName:        workspaceName,
					AppSlug:              appSlug,
					ConnectedWaitGroup:   connectedWaitGroup,
					StartReporting:       startReporting,
					ReportStatusPeriod:   reportStatusPeriod,
					ReportStatusDuration: reportStatusDuration,
					Metrics:              metrics,
					MetricLabelValues:    []string{},
				}

				if err := cfg.Validate(); err != nil {
					return xerrors.Errorf("validate config for runner %d: %w", i, err)
				}

				var runner harness.Runnable = taskstatus.NewRunner(client, cfg)
				if tracingEnabled {
					runner = &runnableTraceWrapper{
						tracer:   tracer,
						spanName: fmt.Sprintf("%s/%d", taskStatusTestName, i),
						runner:   runner,
					}
				}
				th.AddRun(taskStatusTestName, workspaceName, runner)
			}

			// Start the test in a separate goroutine so we can coordinate timing
			testCtx, testCancel := timeoutStrategy.toContext(ctx)
			defer testCancel()
			testDone := make(chan error)
			go func() {
				testDone <- th.Run(testCtx)
			}()

			// Wait for all runners to connect
			logger.Info(ctx, "waiting for all runners to connect")
			waitCtx, waitCancel := context.WithTimeout(ctx, 5*time.Minute)
			defer waitCancel()

			connectDone := make(chan struct{})
			go func() {
				connectedWaitGroup.Wait()
				close(connectDone)
			}()

			select {
			case <-waitCtx.Done():
				return xerrors.Errorf("timeout waiting for runners to connect")
			case <-connectDone:
				logger.Info(ctx, "all runners connected")
			}

			// Wait for baseline duration
			logger.Info(ctx, "waiting for baseline duration", slog.F("duration", baselineDuration))
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(baselineDuration):
			}

			// Trigger all runners to start reporting
			logger.Info(ctx, "triggering runners to start reporting task status")
			close(startReporting)

			// Wait for the test to complete
			err = <-testDone
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

			cleanupCtx, cleanupCancel := cleanupStrategy.toContext(ctx)
			defer cleanupCancel()
			err = th.Cleanup(cleanupCtx)
			if err != nil {
				return xerrors.Errorf("cleanup tests: %w", err)
			}

			if res.TotalFail > 0 {
				return xerrors.New("load test failed, see above for more details")
			}

			return nil
		},
	}

	cmd.Options = serpent.OptionSet{
		{
			Flag:        "count",
			Description: "Number of concurrent runners to create.",
			Default:     "10",
			Value:       serpent.Int64Of(&count),
		},
		{
			Flag:        "template",
			Description: "Name or UUID of the template to use for the scale test. The template MUST include a coder_external_agent and a coder_app.",
			Default:     "scaletest-task-status",
			Value:       serpent.StringOf(&template),
		},
		{
			Flag:        "workspace-name-prefix",
			Description: "Prefix for workspace names (will be suffixed with index).",
			Default:     "scaletest-task-status",
			Value:       serpent.StringOf(&workspaceNamePrefix),
		},
		{
			Flag:        "app-slug",
			Description: "Slug of the app designated as the AI Agent.",
			Default:     "ai-agent",
			Value:       serpent.StringOf(&appSlug),
		},
		{
			Flag:        "report-status-period",
			Description: "Time between reporting task statuses.",
			Default:     "10s",
			Value:       serpent.DurationOf(&reportStatusPeriod),
		},
		{
			Flag:        "report-status-duration",
			Description: "Total time to report task statuses after baseline.",
			Default:     "15m",
			Value:       serpent.DurationOf(&reportStatusDuration),
		},
		{
			Flag:        "baseline-duration",
			Description: "Duration to wait after all runners connect before starting to report status.",
			Default:     "10m",
			Value:       serpent.DurationOf(&baselineDuration),
		},
	}
	orgContext.AttachOptions(cmd)
	output.attach(&cmd.Options)
	tracingFlags.attach(&cmd.Options)
	prometheusFlags.attach(&cmd.Options)
	timeoutStrategy.attach(&cmd.Options)
	cleanupStrategy.attach(&cmd.Options)
	return cmd
}

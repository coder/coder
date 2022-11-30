package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/cliflag"
	"github.com/coder/coder/coderd/tracing"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/loadtest/harness"
)

const loadtestTracerName = "coder_loadtest"

func loadtest() *cobra.Command {
	var (
		configPath  string
		outputSpecs []string

		traceEnable          bool
		traceCoder           bool
		traceHoneycombAPIKey string
		tracePropagate       bool
	)
	cmd := &cobra.Command{
		Use:   "loadtest --config <path> [--output json[:path]] [--output text[:path]]]",
		Short: "Load test the Coder API",
		// TODO: documentation and a JSON schema file
		Long: "Perform load tests against the Coder server. The load tests are configurable via a JSON file.",
		Example: formatExamples(
			example{
				Description: "Run a loadtest with the given configuration file",
				Command:     "coder loadtest --config path/to/config.json",
			},
			example{
				Description: "Run a loadtest, reading the configuration from stdin",
				Command:     "cat path/to/config.json | coder loadtest --config -",
			},
			example{
				Description: "Run a loadtest outputting JSON results instead",
				Command:     "coder loadtest --config path/to/config.json --output json",
			},
			example{
				Description: "Run a loadtest outputting JSON results to a file",
				Command:     "coder loadtest --config path/to/config.json --output json:path/to/results.json",
			},
			example{
				Description: "Run a loadtest outputting text results to stdout and JSON results to a file",
				Command:     "coder loadtest --config path/to/config.json --output text --output json:path/to/results.json",
			},
		),
		Hidden: true,
		Args:   cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := tracing.SetTracerName(cmd.Context(), loadtestTracerName)

			config, err := loadLoadTestConfigFile(configPath, cmd.InOrStdin())
			if err != nil {
				return err
			}
			outputs, err := parseLoadTestOutputs(outputSpecs)
			if err != nil {
				return err
			}

			client, err := CreateClient(cmd)
			if err != nil {
				return err
			}

			me, err := client.User(ctx, codersdk.Me)
			if err != nil {
				return xerrors.Errorf("fetch current user: %w", err)
			}

			// Only owners can do loadtests. This isn't a very strong check but
			// there's not much else we can do. Ratelimits are enforced for
			// non-owners so hopefully that limits the damage if someone
			// disables this check and runs it against a non-owner account.
			ok := false
			for _, role := range me.Roles {
				if role.Name == "owner" {
					ok = true
					break
				}
			}
			if !ok {
				return xerrors.Errorf("Not logged in as a site owner. Load testing is only available to site owners.")
			}

			// Setup tracing and start a span.
			var (
				shouldTrace                           = traceEnable || traceCoder || traceHoneycombAPIKey != ""
				tracerProvider   trace.TracerProvider = trace.NewNoopTracerProvider()
				closeTracingOnce sync.Once
				closeTracing     = func(_ context.Context) error {
					return nil
				}
			)
			if shouldTrace {
				tracerProvider, closeTracing, err = tracing.TracerProvider(ctx, loadtestTracerName, tracing.TracerOpts{
					Default:   traceEnable,
					Coder:     traceCoder,
					Honeycomb: traceHoneycombAPIKey,
				})
				if err != nil {
					return xerrors.Errorf("initialize tracing: %w", err)
				}
				defer func() {
					closeTracingOnce.Do(func() {
						// Allow time for traces to flush even if command
						// context is canceled.
						ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
						defer cancel()
						_ = closeTracing(ctx)
					})
				}()
			}
			tracer := tracerProvider.Tracer(loadtestTracerName)

			// Disable ratelimits and propagate tracing spans for future
			// requests. Individual tests will setup their own loggers.
			client.BypassRatelimits = true
			client.PropagateTracing = tracePropagate

			// Prepare the test.
			runStrategy := config.Strategy.ExecutionStrategy()
			cleanupStrategy := config.CleanupStrategy.ExecutionStrategy()
			th := harness.NewTestHarness(runStrategy, cleanupStrategy)

			for i, t := range config.Tests {
				name := fmt.Sprintf("%s-%d", t.Type, i)

				for j := 0; j < t.Count; j++ {
					id := strconv.Itoa(j)
					runner, err := t.NewRunner(client.Clone())
					if err != nil {
						return xerrors.Errorf("create %q runner for %s/%s: %w", t.Type, name, id, err)
					}

					th.AddRun(name, id, &runnableTraceWrapper{
						tracer:   tracer,
						spanName: fmt.Sprintf("%s/%s", name, id),
						runner:   runner,
					})
				}
			}

			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "Running load test...")

			testCtx := ctx
			if config.Timeout > 0 {
				var cancel func()
				testCtx, cancel = context.WithTimeout(testCtx, time.Duration(config.Timeout))
				defer cancel()
			}

			// TODO: live progress output
			err = th.Run(testCtx)
			if err != nil {
				return xerrors.Errorf("run test harness (harness failure, not a test failure): %w", err)
			}

			// Print the results.
			res := th.Results()
			for _, output := range outputs {
				var (
					w = cmd.OutOrStdout()
					c io.Closer
				)
				if output.path != "-" {
					f, err := os.Create(output.path)
					if err != nil {
						return xerrors.Errorf("create output file: %w", err)
					}
					w, c = f, f
				}

				switch output.format {
				case loadTestOutputFormatText:
					res.PrintText(w)
				case loadTestOutputFormatJSON:
					err = json.NewEncoder(w).Encode(res)
					if err != nil {
						return xerrors.Errorf("encode JSON: %w", err)
					}
				}

				if c != nil {
					err = c.Close()
					if err != nil {
						return xerrors.Errorf("close output file: %w", err)
					}
				}
			}

			// Cleanup.
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "\nCleaning up...")
			err = th.Cleanup(ctx)
			if err != nil {
				return xerrors.Errorf("cleanup tests: %w", err)
			}

			// Upload traces.
			if shouldTrace {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "\nUploading traces...")
				closeTracingOnce.Do(func() {
					ctx, cancel := context.WithTimeout(ctx, 1*time.Minute)
					defer cancel()
					err := closeTracing(ctx)
					if err != nil {
						_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "\nError uploading traces: %+v\n", err)
					}
				})
			}

			if res.TotalFail > 0 {
				return xerrors.New("load test failed, see above for more details")
			}

			return nil
		},
	}

	cliflag.StringVarP(cmd.Flags(), &configPath, "config", "", "CODER_LOADTEST_CONFIG_PATH", "", "Path to the load test configuration file, or - to read from stdin.")
	cliflag.StringArrayVarP(cmd.Flags(), &outputSpecs, "output", "", "CODER_LOADTEST_OUTPUTS", []string{"text"}, "Output formats, see usage for more information.")

	cliflag.BoolVarP(cmd.Flags(), &traceEnable, "trace", "", "CODER_LOADTEST_TRACE", false, "Whether application tracing data is collected. It exports to a backend configured by environment variables. See: https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/protocol/exporter.md")
	cliflag.BoolVarP(cmd.Flags(), &traceCoder, "trace-coder", "", "CODER_LOADTEST_TRACE_CODER", false, "Whether opentelemetry traces are sent to Coder. We recommend keeping this disabled unless we advise you to enable it.")
	cliflag.StringVarP(cmd.Flags(), &traceHoneycombAPIKey, "trace-honeycomb-api-key", "", "CODER_LOADTEST_TRACE_HONEYCOMB_API_KEY", "", "Enables trace exporting to Honeycomb.io using the provided API key.")
	cliflag.BoolVarP(cmd.Flags(), &tracePropagate, "trace-propagate", "", "CODER_LOADTEST_TRACE_PROPAGATE", false, "Enables trace propagation to the Coder backend, which will be used to correlate server-side spans with client-side spans. Only enable this if the server is configured with the exact same tracing configuration as the client.")

	return cmd
}

func loadLoadTestConfigFile(configPath string, stdin io.Reader) (LoadTestConfig, error) {
	if configPath == "" {
		return LoadTestConfig{}, xerrors.New("config is required")
	}

	var (
		configReader io.ReadCloser
	)
	if configPath == "-" {
		configReader = io.NopCloser(stdin)
	} else {
		f, err := os.Open(configPath)
		if err != nil {
			return LoadTestConfig{}, xerrors.Errorf("open config file %q: %w", configPath, err)
		}
		configReader = f
	}

	var config LoadTestConfig
	err := json.NewDecoder(configReader).Decode(&config)
	_ = configReader.Close()
	if err != nil {
		return LoadTestConfig{}, xerrors.Errorf("read config file %q: %w", configPath, err)
	}

	err = config.Validate()
	if err != nil {
		return LoadTestConfig{}, xerrors.Errorf("validate config: %w", err)
	}

	return config, nil
}

type loadTestOutputFormat string

const (
	loadTestOutputFormatText loadTestOutputFormat = "text"
	loadTestOutputFormatJSON loadTestOutputFormat = "json"
	// TODO: html format
)

type loadTestOutput struct {
	format loadTestOutputFormat
	// Up to one path (the first path) will have the value "-" which signifies
	// stdout.
	path string
}

func parseLoadTestOutputs(outputs []string) ([]loadTestOutput, error) {
	var stdoutFormat loadTestOutputFormat

	validFormats := map[loadTestOutputFormat]struct{}{
		loadTestOutputFormatText: {},
		loadTestOutputFormatJSON: {},
	}

	var out []loadTestOutput
	for i, o := range outputs {
		parts := strings.SplitN(o, ":", 2)
		format := loadTestOutputFormat(parts[0])
		if _, ok := validFormats[format]; !ok {
			return nil, xerrors.Errorf("invalid output format %q in output flag %d", parts[0], i)
		}

		if len(parts) == 1 {
			if stdoutFormat != "" {
				return nil, xerrors.Errorf("multiple output flags specified for stdout")
			}
			stdoutFormat = format
			continue
		}
		if len(parts) != 2 {
			return nil, xerrors.Errorf("invalid output flag %d: %q", i, o)
		}

		out = append(out, loadTestOutput{
			format: format,
			path:   parts[1],
		})
	}

	// Default to --output text
	if stdoutFormat == "" && len(out) == 0 {
		stdoutFormat = loadTestOutputFormatText
	}

	if stdoutFormat != "" {
		out = append([]loadTestOutput{{
			format: stdoutFormat,
			path:   "-",
		}}, out...)
	}

	return out, nil
}

type runnableTraceWrapper struct {
	tracer   trace.Tracer
	spanName string
	runner   harness.Runnable

	span trace.Span
}

var _ harness.Runnable = &runnableTraceWrapper{}
var _ harness.Cleanable = &runnableTraceWrapper{}

func (r *runnableTraceWrapper) Run(ctx context.Context, id string, logs io.Writer) error {
	ctx, span := r.tracer.Start(ctx, r.spanName, trace.WithNewRoot())
	defer span.End()
	r.span = span

	traceID := "unknown trace ID"
	spanID := "unknown span ID"
	if span.SpanContext().HasTraceID() {
		traceID = span.SpanContext().TraceID().String()
	}
	if span.SpanContext().HasSpanID() {
		spanID = span.SpanContext().SpanID().String()
	}
	_, _ = fmt.Fprintf(logs, "Trace ID: %s\n", traceID)
	_, _ = fmt.Fprintf(logs, "Span ID: %s\n\n", spanID)

	// Make a separate span for the run itself so the sub-spans are grouped
	// neatly. The cleanup span is also a child of the above span so this is
	// important for readability.
	ctx2, span2 := r.tracer.Start(ctx, r.spanName+" run")
	defer span2.End()
	return r.runner.Run(ctx2, id, logs)
}

func (r *runnableTraceWrapper) Cleanup(ctx context.Context, id string) error {
	c, ok := r.runner.(harness.Cleanable)
	if !ok {
		return nil
	}

	if r.span != nil {
		ctx = trace.ContextWithSpanContext(ctx, r.span.SpanContext())
	}
	ctx, span := r.tracer.Start(ctx, r.spanName+" cleanup")
	defer span.End()

	return c.Cleanup(ctx, id)
}

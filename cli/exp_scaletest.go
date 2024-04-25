//go:build !slim

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/exp/slices"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/cryptorand"
	"github.com/coder/coder/v2/scaletest/agentconn"
	"github.com/coder/coder/v2/scaletest/createworkspaces"
	"github.com/coder/coder/v2/scaletest/dashboard"
	"github.com/coder/coder/v2/scaletest/harness"
	"github.com/coder/coder/v2/scaletest/reconnectingpty"
	"github.com/coder/coder/v2/scaletest/workspacebuild"
	"github.com/coder/coder/v2/scaletest/workspacetraffic"
	"github.com/coder/serpent"
)

const scaletestTracerName = "coder_scaletest"

func (r *RootCmd) scaletestCmd() *serpent.Command {
	cmd := &serpent.Command{
		Use:   "scaletest",
		Short: "Run a scale test against the Coder API",
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*serpent.Command{
			r.scaletestCleanup(),
			r.scaletestDashboard(),
			r.scaletestCreateWorkspaces(),
			r.scaletestWorkspaceTraffic(),
		},
	}

	return cmd
}

type scaletestTracingFlags struct {
	traceEnable          bool
	traceCoder           bool
	traceHoneycombAPIKey string
	tracePropagate       bool
}

func (s *scaletestTracingFlags) attach(opts *serpent.OptionSet) {
	*opts = append(
		*opts,
		serpent.Option{
			Flag:        "trace",
			Env:         "CODER_SCALETEST_TRACE",
			Description: "Whether application tracing data is collected. It exports to a backend configured by environment variables. See: https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/protocol/exporter.md.",
			Value:       serpent.BoolOf(&s.traceEnable),
		},
		serpent.Option{
			Flag:        "trace-coder",
			Env:         "CODER_SCALETEST_TRACE_CODER",
			Description: "Whether opentelemetry traces are sent to Coder. We recommend keeping this disabled unless we advise you to enable it.",
			Value:       serpent.BoolOf(&s.traceCoder),
		},
		serpent.Option{
			Flag:        "trace-honeycomb-api-key",
			Env:         "CODER_SCALETEST_TRACE_HONEYCOMB_API_KEY",
			Description: "Enables trace exporting to Honeycomb.io using the provided API key.",
			Value:       serpent.StringOf(&s.traceHoneycombAPIKey),
		},
		serpent.Option{
			Flag:        "trace-propagate",
			Env:         "CODER_SCALETEST_TRACE_PROPAGATE",
			Description: "Enables trace propagation to the Coder backend, which will be used to correlate server-side spans with client-side spans. Only enable this if the server is configured with the exact same tracing configuration as the client.",
			Value:       serpent.BoolOf(&s.tracePropagate),
		},
	)
}

// provider returns a trace.TracerProvider, a close function and a bool showing
// whether tracing is enabled or not.
func (s *scaletestTracingFlags) provider(ctx context.Context) (trace.TracerProvider, func(context.Context) error, bool, error) {
	shouldTrace := s.traceEnable || s.traceCoder || s.traceHoneycombAPIKey != ""
	if !shouldTrace {
		tracerProvider := trace.NewNoopTracerProvider()
		return tracerProvider, func(_ context.Context) error { return nil }, false, nil
	}

	tracerProvider, closeTracing, err := tracing.TracerProvider(ctx, scaletestTracerName, tracing.TracerOpts{
		Default:   s.traceEnable,
		Honeycomb: s.traceHoneycombAPIKey,
	})
	if err != nil {
		return nil, nil, false, xerrors.Errorf("initialize tracing: %w", err)
	}

	var closeTracingOnce sync.Once
	return tracerProvider, func(ctx context.Context) error {
		var err error
		closeTracingOnce.Do(func() {
			// Allow time to upload traces even if ctx is canceled
			traceCtx, traceCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer traceCancel()
			err = closeTracing(traceCtx)
		})

		return err
	}, true, nil
}

type scaletestStrategyFlags struct {
	cleanup       bool
	concurrency   int64
	timeout       time.Duration
	timeoutPerJob time.Duration
}

func (s *scaletestStrategyFlags) attach(opts *serpent.OptionSet) {
	concurrencyLong, concurrencyEnv, concurrencyDescription := "concurrency", "CODER_SCALETEST_CONCURRENCY", "Number of concurrent jobs to run. 0 means unlimited."
	timeoutLong, timeoutEnv, timeoutDescription := "timeout", "CODER_SCALETEST_TIMEOUT", "Timeout for the entire test run. 0 means unlimited."
	jobTimeoutLong, jobTimeoutEnv, jobTimeoutDescription := "job-timeout", "CODER_SCALETEST_JOB_TIMEOUT", "Timeout per job. Jobs may take longer to complete under higher concurrency limits."
	if s.cleanup {
		concurrencyLong, concurrencyEnv, concurrencyDescription = "cleanup-"+concurrencyLong, "CODER_SCALETEST_CLEANUP_CONCURRENCY", strings.ReplaceAll(concurrencyDescription, "jobs", "cleanup jobs")
		timeoutLong, timeoutEnv, timeoutDescription = "cleanup-"+timeoutLong, "CODER_SCALETEST_CLEANUP_TIMEOUT", strings.ReplaceAll(timeoutDescription, "test", "cleanup")
		jobTimeoutLong, jobTimeoutEnv, jobTimeoutDescription = "cleanup-"+jobTimeoutLong, "CODER_SCALETEST_CLEANUP_JOB_TIMEOUT", strings.ReplaceAll(jobTimeoutDescription, "jobs", "cleanup jobs")
	}

	*opts = append(
		*opts,
		serpent.Option{
			Flag:        concurrencyLong,
			Env:         concurrencyEnv,
			Description: concurrencyDescription,
			Default:     "1",
			Value:       serpent.Int64Of(&s.concurrency),
		},
		serpent.Option{
			Flag:        timeoutLong,
			Env:         timeoutEnv,
			Description: timeoutDescription,
			Default:     "30m",
			Value:       serpent.DurationOf(&s.timeout),
		},
		serpent.Option{
			Flag:        jobTimeoutLong,
			Env:         jobTimeoutEnv,
			Description: jobTimeoutDescription,
			Default:     "5m",
			Value:       serpent.DurationOf(&s.timeoutPerJob),
		},
	)
}

func (s *scaletestStrategyFlags) toStrategy() harness.ExecutionStrategy {
	var strategy harness.ExecutionStrategy
	switch s.concurrency {
	case 1:
		strategy = harness.LinearExecutionStrategy{}
	case 0:
		strategy = harness.ConcurrentExecutionStrategy{}
	default:
		strategy = harness.ParallelExecutionStrategy{
			Limit: int(s.concurrency),
		}
	}

	if s.timeoutPerJob > 0 {
		strategy = harness.TimeoutExecutionStrategyWrapper{
			Timeout: s.timeoutPerJob,
			Inner:   strategy,
		}
	}

	return strategy
}

func (s *scaletestStrategyFlags) toContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if s.timeout > 0 {
		return context.WithTimeout(ctx, s.timeout)
	}

	return context.WithCancel(ctx)
}

type scaleTestOutputFormat string

const (
	scaleTestOutputFormatText scaleTestOutputFormat = "text"
	scaleTestOutputFormatJSON scaleTestOutputFormat = "json"
	// TODO: html format
)

type scaleTestOutput struct {
	format scaleTestOutputFormat
	// Zero or one (the first) path will have the path set to "-" to indicate
	// stdout.
	path string
}

func (o *scaleTestOutput) write(res harness.Results, stdout io.Writer) error {
	var (
		w = stdout
		c io.Closer
	)
	if o.path != "-" {
		f, err := os.Create(o.path)
		if err != nil {
			return xerrors.Errorf("create output file: %w", err)
		}
		w, c = f, f
	}

	switch o.format {
	case scaleTestOutputFormatText:
		res.PrintText(w)
	case scaleTestOutputFormatJSON:
		err := json.NewEncoder(w).Encode(res)
		if err != nil {
			return xerrors.Errorf("encode JSON: %w", err)
		}
	}

	// Sync the file to disk if it's a file.
	if s, ok := w.(interface{ Sync() error }); ok {
		// Best effort. If we get an error from syncing, just ignore it.
		_ = s.Sync()
	}

	if c != nil {
		err := c.Close()
		if err != nil {
			return xerrors.Errorf("close output file: %w", err)
		}
	}

	return nil
}

type scaletestOutputFlags struct {
	outputSpecs []string
}

func (s *scaletestOutputFlags) attach(opts *serpent.OptionSet) {
	*opts = append(*opts, serpent.Option{
		Flag:        "output",
		Env:         "CODER_SCALETEST_OUTPUTS",
		Description: `Output format specs in the format "<format>[:<path>]". Not specifying a path will default to stdout. Available formats: text, json.`,
		Default:     "text",
		Value:       serpent.StringArrayOf(&s.outputSpecs),
	})
}

func (s *scaletestOutputFlags) parse() ([]scaleTestOutput, error) {
	var stdoutFormat scaleTestOutputFormat

	validFormats := map[scaleTestOutputFormat]struct{}{
		scaleTestOutputFormatText: {},
		scaleTestOutputFormatJSON: {},
	}

	var out []scaleTestOutput
	for i, o := range s.outputSpecs {
		parts := strings.SplitN(o, ":", 2)
		format := scaleTestOutputFormat(parts[0])
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

		out = append(out, scaleTestOutput{
			format: format,
			path:   parts[1],
		})
	}

	// Default to --output text
	if stdoutFormat == "" && len(out) == 0 {
		stdoutFormat = scaleTestOutputFormatText
	}

	if stdoutFormat != "" {
		out = append([]scaleTestOutput{{
			format: stdoutFormat,
			path:   "-",
		}}, out...)
	}

	return out, nil
}

type scaletestPrometheusFlags struct {
	Address string
	Wait    time.Duration
}

func (s *scaletestPrometheusFlags) attach(opts *serpent.OptionSet) {
	*opts = append(*opts,
		serpent.Option{
			Flag:        "scaletest-prometheus-address",
			Env:         "CODER_SCALETEST_PROMETHEUS_ADDRESS",
			Default:     "0.0.0.0:21112",
			Description: "Address on which to expose scaletest Prometheus metrics.",
			Value:       serpent.StringOf(&s.Address),
		},
		serpent.Option{
			Flag:        "scaletest-prometheus-wait",
			Env:         "CODER_SCALETEST_PROMETHEUS_WAIT",
			Default:     "15s",
			Description: "How long to wait before exiting in order to allow Prometheus metrics to be scraped.",
			Value:       serpent.DurationOf(&s.Wait),
		},
	)
}

func requireAdmin(ctx context.Context, client *codersdk.Client) (codersdk.User, error) {
	me, err := client.User(ctx, codersdk.Me)
	if err != nil {
		return codersdk.User{}, xerrors.Errorf("fetch current user: %w", err)
	}

	// Only owners can do scaletests. This isn't a very strong check but there's
	// not much else we can do. Ratelimits are enforced for non-owners so
	// hopefully that limits the damage if someone disables this check and runs
	// it against a non-owner account on a production deployment.
	ok := false
	for _, role := range me.Roles {
		if role.Name == "owner" {
			ok = true
			break
		}
	}
	if !ok {
		return me, xerrors.Errorf("Not logged in as a site owner. Scale testing is only available to site owners.")
	}

	return me, nil
}

// userCleanupRunner is a runner that deletes a user in the Run phase.
type userCleanupRunner struct {
	client *codersdk.Client
	userID uuid.UUID
}

var _ harness.Runnable = &userCleanupRunner{}

// Run implements Runnable.
func (r *userCleanupRunner) Run(ctx context.Context, _ string, _ io.Writer) error {
	if r.userID == uuid.Nil {
		return nil
	}
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	err := r.client.DeleteUser(ctx, r.userID)
	if err != nil {
		return xerrors.Errorf("delete user %q: %w", r.userID, err)
	}

	return nil
}

func (r *RootCmd) scaletestCleanup() *serpent.Command {
	var template string

	cleanupStrategy := &scaletestStrategyFlags{cleanup: true}
	client := new(codersdk.Client)

	cmd := &serpent.Command{
		Use:   "cleanup",
		Short: "Cleanup scaletest workspaces, then cleanup scaletest users.",
		Long:  "The strategy flags will apply to each stage of the cleanup process.",
		Middleware: serpent.Chain(
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()

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

			if template != "" {
				_, err := parseTemplate(ctx, client, me.OrganizationIDs, template)
				if err != nil {
					return xerrors.Errorf("parse template: %w", err)
				}
			}

			cliui.Infof(inv.Stdout, "Fetching scaletest workspaces...")
			workspaces, err := getScaletestWorkspaces(ctx, client, template)
			if err != nil {
				return err
			}

			cliui.Errorf(inv.Stderr, "Found %d scaletest workspaces\n", len(workspaces))
			if len(workspaces) != 0 {
				cliui.Infof(inv.Stdout, "Deleting scaletest workspaces...")
				harness := harness.NewTestHarness(cleanupStrategy.toStrategy(), harness.ConcurrentExecutionStrategy{})

				for i, w := range workspaces {
					const testName = "cleanup-workspace"
					r := workspacebuild.NewCleanupRunner(client, w.ID)
					harness.AddRun(testName, strconv.Itoa(i), r)
				}

				ctx, cancel := cleanupStrategy.toContext(ctx)
				defer cancel()
				err := harness.Run(ctx)
				if err != nil {
					return xerrors.Errorf("run test harness to delete workspaces (harness failure, not a test failure): %w", err)
				}

				cliui.Infof(inv.Stdout, "Done deleting scaletest workspaces:")
				res := harness.Results()
				res.PrintText(inv.Stderr)

				if res.TotalFail > 0 {
					return xerrors.Errorf("failed to delete scaletest workspaces")
				}
			}

			cliui.Infof(inv.Stdout, "Fetching scaletest users...")
			users, err := getScaletestUsers(ctx, client)
			if err != nil {
				return err
			}

			cliui.Errorf(inv.Stderr, "Found %d scaletest users\n", len(users))
			if len(users) != 0 {
				cliui.Infof(inv.Stdout, "Deleting scaletest users...")
				harness := harness.NewTestHarness(cleanupStrategy.toStrategy(), harness.ConcurrentExecutionStrategy{})

				for i, u := range users {
					const testName = "cleanup-users"
					r := &userCleanupRunner{
						client: client,
						userID: u.ID,
					}
					harness.AddRun(testName, strconv.Itoa(i), r)
				}

				ctx, cancel := cleanupStrategy.toContext(ctx)
				defer cancel()
				err := harness.Run(ctx)
				if err != nil {
					return xerrors.Errorf("run test harness to delete users (harness failure, not a test failure): %w", err)
				}

				cliui.Infof(inv.Stdout, "Done deleting scaletest users:")
				res := harness.Results()
				res.PrintText(inv.Stderr)

				if res.TotalFail > 0 {
					return xerrors.Errorf("failed to delete scaletest users")
				}
			}

			return nil
		},
	}

	cmd.Options = serpent.OptionSet{
		{
			Flag:        "template",
			Env:         "CODER_SCALETEST_CLEANUP_TEMPLATE",
			Description: "Name or ID of the template. Only delete workspaces created from the given template.",
			Value:       serpent.StringOf(&template),
		},
	}

	cleanupStrategy.attach(&cmd.Options)
	return cmd
}

func (r *RootCmd) scaletestCreateWorkspaces() *serpent.Command {
	var (
		count    int64
		retry    int64
		template string

		noCleanup bool
		// TODO: implement this flag
		// noCleanupFailures bool
		noWaitForAgents bool

		runCommand       string
		runTimeout       time.Duration
		runExpectTimeout bool
		runExpectOutput  string
		runLogOutput     bool

		// TODO: customizable agent, currently defaults to the first agent found
		// if there are multiple
		connectURL      string // http://localhost:4/
		connectMode     string // derp or direct
		connectHold     time.Duration
		connectInterval time.Duration
		connectTimeout  time.Duration

		useHostUser bool

		parameterFlags workspaceParameterFlags

		tracingFlags    = &scaletestTracingFlags{}
		strategy        = &scaletestStrategyFlags{}
		cleanupStrategy = &scaletestStrategyFlags{cleanup: true}
		output          = &scaletestOutputFlags{}
	)

	client := new(codersdk.Client)

	cmd := &serpent.Command{
		Use:        "create-workspaces",
		Short:      "Creates many users, then creates a workspace for each user and waits for them finish building and fully come online. Optionally runs a command inside each workspace, and connects to the workspace over WireGuard.",
		Long:       `It is recommended that all rate limits are disabled on the server before running this scaletest. This test generates many login events which will be rate limited against the (most likely single) IP.`,
		Middleware: r.InitClient(client),
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()

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

			if count <= 0 {
				return xerrors.Errorf("--count is required and must be greater than 0")
			}
			outputs, err := output.parse()
			if err != nil {
				return xerrors.Errorf("could not parse --output flags")
			}

			if template == "" {
				return xerrors.Errorf("--template is required")
			}
			tpl, err := parseTemplate(ctx, client, me.OrganizationIDs, template)
			if err != nil {
				return xerrors.Errorf("parse template: %w", err)
			}

			cliRichParameters, err := asWorkspaceBuildParameters(parameterFlags.richParameters)
			if err != nil {
				return xerrors.Errorf("can't parse given parameter values: %w", err)
			}

			richParameters, err := prepWorkspaceBuild(inv, client, prepWorkspaceBuildArgs{
				Action:            WorkspaceCreate,
				TemplateVersionID: tpl.ActiveVersionID,
				NewWorkspaceName:  "scaletest-N", // TODO: the scaletest runner will pass in a different name here. Does this matter?

				RichParameterFile: parameterFlags.richParameterFile,
				RichParameters:    cliRichParameters,
			})
			if err != nil {
				return xerrors.Errorf("prepare build: %w", err)
			}

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
			}()
			tracer := tracerProvider.Tracer(scaletestTracerName)

			th := harness.NewTestHarness(strategy.toStrategy(), cleanupStrategy.toStrategy())
			for i := 0; i < int(count); i++ {
				const name = "workspacebuild"
				id := strconv.Itoa(i)

				config := createworkspaces.Config{
					User: createworkspaces.UserConfig{
						// TODO: configurable org
						OrganizationID: me.OrganizationIDs[0],
					},
					Workspace: workspacebuild.Config{
						OrganizationID: me.OrganizationIDs[0],
						// UserID is set by the test automatically.
						Request: codersdk.CreateWorkspaceRequest{
							TemplateID:          tpl.ID,
							RichParameterValues: richParameters,
						},
						NoWaitForAgents: noWaitForAgents,
						Retry:           int(retry),
					},
					NoCleanup: noCleanup,
				}

				if useHostUser {
					config.User.SessionToken = client.SessionToken()
				} else {
					config.User.Username, config.User.Email, err = newScaleTestUser(id)
					if err != nil {
						return xerrors.Errorf("create scaletest username and email: %w", err)
					}
				}

				config.Workspace.Request.Name, err = newScaleTestWorkspace(id)
				if err != nil {
					return xerrors.Errorf("create scaletest workspace name: %w", err)
				}

				if runCommand != "" {
					config.ReconnectingPTY = &reconnectingpty.Config{
						// AgentID is set by the test automatically.
						Init: workspacesdk.AgentReconnectingPTYInit{
							ID:      uuid.Nil,
							Height:  24,
							Width:   80,
							Command: runCommand,
						},
						Timeout:       httpapi.Duration(runTimeout),
						ExpectTimeout: runExpectTimeout,
						ExpectOutput:  runExpectOutput,
						LogOutput:     runLogOutput,
					}
				}
				if connectURL != "" {
					config.AgentConn = &agentconn.Config{
						// AgentID is set by the test automatically.
						// The ConnectionMode gets validated by the Validate()
						// call below.
						ConnectionMode: agentconn.ConnectionMode(connectMode),
						HoldDuration:   httpapi.Duration(connectHold),
						Connections: []agentconn.Connection{
							{
								URL:      connectURL,
								Interval: httpapi.Duration(connectInterval),
								Timeout:  httpapi.Duration(connectTimeout),
							},
						},
					}
				}

				err = config.Validate()
				if err != nil {
					return xerrors.Errorf("validate config: %w", err)
				}

				var runner harness.Runnable = createworkspaces.NewRunner(client, config)
				if tracingEnabled {
					runner = &runnableTraceWrapper{
						tracer:   tracer,
						spanName: fmt.Sprintf("%s/%s", name, id),
						runner:   runner,
					}
				}

				th.AddRun(name, id, runner)
			}

			// TODO: live progress output
			_, _ = fmt.Fprintln(inv.Stderr, "Running load test...")
			testCtx, testCancel := strategy.toContext(ctx)
			defer testCancel()
			err = th.Run(testCtx)
			if err != nil {
				return xerrors.Errorf("run test harness (harness failure, not a test failure): %w", err)
			}

			res := th.Results()
			for _, o := range outputs {
				err = o.write(res, inv.Stdout)
				if err != nil {
					return xerrors.Errorf("write output %q to %q: %w", o.format, o.path, err)
				}
			}

			_, _ = fmt.Fprintln(inv.Stderr, "\nCleaning up...")
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
			Flag:          "count",
			FlagShorthand: "c",
			Env:           "CODER_SCALETEST_COUNT",
			Default:       "1",
			Description:   "Required: Number of workspaces to create.",
			Value:         serpent.Int64Of(&count),
		},
		{
			Flag:        "retry",
			Env:         "CODER_SCALETEST_RETRY",
			Default:     "0",
			Description: "Number of tries to create and bring up the workspace.",
			Value:       serpent.Int64Of(&retry),
		},
		{
			Flag:          "template",
			FlagShorthand: "t",
			Env:           "CODER_SCALETEST_TEMPLATE",
			Description:   "Required: Name or ID of the template to use for workspaces.",
			Value:         serpent.StringOf(&template),
		},
		{
			Flag:        "no-cleanup",
			Env:         "CODER_SCALETEST_NO_CLEANUP",
			Description: "Do not clean up resources after the test completes. You can cleanup manually using coder scaletest cleanup.",
			Value:       serpent.BoolOf(&noCleanup),
		},
		{
			Flag:        "no-wait-for-agents",
			Env:         "CODER_SCALETEST_NO_WAIT_FOR_AGENTS",
			Description: `Do not wait for agents to start before marking the test as succeeded. This can be useful if you are running the test against a template that does not start the agent quickly.`,
			Value:       serpent.BoolOf(&noWaitForAgents),
		},
		{
			Flag:        "run-command",
			Env:         "CODER_SCALETEST_RUN_COMMAND",
			Description: "Command to run inside each workspace using reconnecting-pty (i.e. web terminal protocol). " + "If not specified, no command will be run.",
			Value:       serpent.StringOf(&runCommand),
		},
		{
			Flag:        "run-timeout",
			Env:         "CODER_SCALETEST_RUN_TIMEOUT",
			Default:     "5s",
			Description: "Timeout for the command to complete.",
			Value:       serpent.DurationOf(&runTimeout),
		},
		{
			Flag: "run-expect-timeout",
			Env:  "CODER_SCALETEST_RUN_EXPECT_TIMEOUT",

			Description: "Expect the command to timeout." + " If the command does not finish within the given --run-timeout, it will be marked as succeeded." + " If the command finishes before the timeout, it will be marked as failed.",
			Value:       serpent.BoolOf(&runExpectTimeout),
		},
		{
			Flag:        "run-expect-output",
			Env:         "CODER_SCALETEST_RUN_EXPECT_OUTPUT",
			Description: "Expect the command to output the given string (on a single line). " + "If the command does not output the given string, it will be marked as failed.",
			Value:       serpent.StringOf(&runExpectOutput),
		},
		{
			Flag:        "run-log-output",
			Env:         "CODER_SCALETEST_RUN_LOG_OUTPUT",
			Description: "Log the output of the command to the test logs. " + "This should be left off unless you expect small amounts of output. " + "Large amounts of output will cause high memory usage.",
			Value:       serpent.BoolOf(&runLogOutput),
		},
		{
			Flag:        "connect-url",
			Env:         "CODER_SCALETEST_CONNECT_URL",
			Description: "URL to connect to inside the the workspace over WireGuard. " + "If not specified, no connections will be made over WireGuard.",
			Value:       serpent.StringOf(&connectURL),
		},
		{
			Flag:        "connect-mode",
			Env:         "CODER_SCALETEST_CONNECT_MODE",
			Default:     "derp",
			Description: "Mode to use for connecting to the workspace.",
			Value:       serpent.EnumOf(&connectMode, "derp", "direct"),
		},
		{
			Flag:        "connect-hold",
			Env:         "CODER_SCALETEST_CONNECT_HOLD",
			Default:     "30s",
			Description: "How long to hold the WireGuard connection open for.",
			Value:       serpent.DurationOf(&connectHold),
		},
		{
			Flag:        "connect-interval",
			Env:         "CODER_SCALETEST_CONNECT_INTERVAL",
			Default:     "1s",
			Value:       serpent.DurationOf(&connectInterval),
			Description: "How long to wait between making requests to the --connect-url once the connection is established.",
		},
		{
			Flag:        "connect-timeout",
			Env:         "CODER_SCALETEST_CONNECT_TIMEOUT",
			Default:     "5s",
			Description: "Timeout for each request to the --connect-url.",
			Value:       serpent.DurationOf(&connectTimeout),
		},
		{
			Flag:        "use-host-login",
			Env:         "CODER_SCALETEST_USE_HOST_LOGIN",
			Default:     "false",
			Description: "Use the user logged in on the host machine, instead of creating users.",
			Value:       serpent.BoolOf(&useHostUser),
		},
	}

	cmd.Options = append(cmd.Options, parameterFlags.cliParameters()...)
	tracingFlags.attach(&cmd.Options)
	strategy.attach(&cmd.Options)
	cleanupStrategy.attach(&cmd.Options)
	output.attach(&cmd.Options)
	return cmd
}

func (r *RootCmd) scaletestWorkspaceTraffic() *serpent.Command {
	var (
		tickInterval     time.Duration
		bytesPerTick     int64
		ssh              bool
		app              string
		template         string
		targetWorkspaces string

		client          = &codersdk.Client{}
		tracingFlags    = &scaletestTracingFlags{}
		strategy        = &scaletestStrategyFlags{}
		cleanupStrategy = &scaletestStrategyFlags{cleanup: true}
		output          = &scaletestOutputFlags{}
		prometheusFlags = &scaletestPrometheusFlags{}
	)

	cmd := &serpent.Command{
		Use:   "workspace-traffic",
		Short: "Generate traffic to scaletest workspaces through coderd",
		Middleware: serpent.Chain(
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) (err error) {
			ctx := inv.Context()

			notifyCtx, stop := signal.NotifyContext(ctx, StopSignals...) // Checked later.
			defer stop()
			ctx = notifyCtx

			me, err := requireAdmin(ctx, client)
			if err != nil {
				return err
			}

			reg := prometheus.NewRegistry()
			metrics := workspacetraffic.NewMetrics(reg, "username", "workspace_name", "agent_name")

			logger := inv.Logger
			prometheusSrvClose := ServeHandler(ctx, logger, promhttp.HandlerFor(reg, promhttp.HandlerOpts{}), prometheusFlags.Address, "prometheus")
			defer prometheusSrvClose()

			// Bypass rate limiting
			client.HTTPClient = &http.Client{
				Transport: &codersdk.HeaderTransport{
					Transport: http.DefaultTransport,
					Header: map[string][]string{
						codersdk.BypassRatelimitHeader: {"true"},
					},
				},
			}

			if template != "" {
				_, err := parseTemplate(ctx, client, me.OrganizationIDs, template)
				if err != nil {
					return xerrors.Errorf("parse template: %w", err)
				}
			}
			targetWorkspaceStart, targetWorkspaceEnd, err := parseTargetRange("workspaces", targetWorkspaces)
			if err != nil {
				return xerrors.Errorf("parse target workspaces: %w", err)
			}

			appHost, err := client.AppHost(ctx)
			if err != nil {
				return xerrors.Errorf("get app host: %w", err)
			}

			workspaces, err := getScaletestWorkspaces(inv.Context(), client, template)
			if err != nil {
				return err
			}

			if targetWorkspaceEnd == 0 {
				targetWorkspaceEnd = len(workspaces)
			}

			if len(workspaces) == 0 {
				return xerrors.Errorf("no scaletest workspaces exist")
			}
			if targetWorkspaceEnd > len(workspaces) {
				return xerrors.Errorf("target workspace end %d is greater than the number of workspaces %d", targetWorkspaceEnd, len(workspaces))
			}

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

			outputs, err := output.parse()
			if err != nil {
				return xerrors.Errorf("could not parse --output flags")
			}

			th := harness.NewTestHarness(strategy.toStrategy(), cleanupStrategy.toStrategy())
			for idx, ws := range workspaces {
				if idx < targetWorkspaceStart || idx >= targetWorkspaceEnd {
					continue
				}

				var (
					agent codersdk.WorkspaceAgent
					name  = "workspace-traffic"
					id    = strconv.Itoa(idx)
				)

				for _, res := range ws.LatestBuild.Resources {
					if len(res.Agents) == 0 {
						continue
					}
					agent = res.Agents[0]
				}

				if agent.ID == uuid.Nil {
					_, _ = fmt.Fprintf(inv.Stderr, "WARN: skipping workspace %s: no agent\n", ws.Name)
					continue
				}

				appConfig, err := createWorkspaceAppConfig(client, appHost.Host, app, ws, agent)
				if err != nil {
					return xerrors.Errorf("configure workspace app: %w", err)
				}

				// Setup our workspace agent connection.
				config := workspacetraffic.Config{
					AgentID:      agent.ID,
					BytesPerTick: bytesPerTick,
					Duration:     strategy.timeout,
					TickInterval: tickInterval,
					ReadMetrics:  metrics.ReadMetrics(ws.OwnerName, ws.Name, agent.Name),
					WriteMetrics: metrics.WriteMetrics(ws.OwnerName, ws.Name, agent.Name),
					SSH:          ssh,
					Echo:         ssh,
					App:          appConfig,
				}

				if err := config.Validate(); err != nil {
					return xerrors.Errorf("validate config: %w", err)
				}
				var runner harness.Runnable = workspacetraffic.NewRunner(client, config)
				if tracingEnabled {
					runner = &runnableTraceWrapper{
						tracer:   tracer,
						spanName: fmt.Sprintf("%s/%s", name, id),
						runner:   runner,
					}
				}

				th.AddRun(name, id, runner)
			}

			_, _ = fmt.Fprintln(inv.Stderr, "Running load test...")
			testCtx, testCancel := strategy.toContext(ctx)
			defer testCancel()
			err = th.Run(testCtx)
			if err != nil {
				return xerrors.Errorf("run test harness (harness failure, not a test failure): %w", err)
			}

			// If the command was interrupted, skip stats.
			if notifyCtx.Err() != nil {
				return notifyCtx.Err()
			}

			res := th.Results()
			for _, o := range outputs {
				err = o.write(res, inv.Stdout)
				if err != nil {
					return xerrors.Errorf("write output %q to %q: %w", o.format, o.path, err)
				}
			}

			if res.TotalFail > 0 {
				return xerrors.New("load test failed, see above for more details")
			}

			return nil
		},
	}

	cmd.Options = []serpent.Option{
		{
			Flag:          "template",
			FlagShorthand: "t",
			Env:           "CODER_SCALETEST_TEMPLATE",
			Description:   "Name or ID of the template. Traffic generation will be limited to workspaces created from this template.",
			Value:         serpent.StringOf(&template),
		},
		{
			Flag:        "target-workspaces",
			Env:         "CODER_SCALETEST_TARGET_WORKSPACES",
			Description: "Target a specific range of workspaces in the format [START]:[END] (exclusive). Example: 0:10 will target the 10 first alphabetically sorted workspaces (0-9).",
			Value:       serpent.StringOf(&targetWorkspaces),
		},
		{
			Flag:        "bytes-per-tick",
			Env:         "CODER_SCALETEST_WORKSPACE_TRAFFIC_BYTES_PER_TICK",
			Default:     "1024",
			Description: "How much traffic to generate per tick.",
			Value:       serpent.Int64Of(&bytesPerTick),
		},
		{
			Flag:        "tick-interval",
			Env:         "CODER_SCALETEST_WORKSPACE_TRAFFIC_TICK_INTERVAL",
			Default:     "100ms",
			Description: "How often to send traffic.",
			Value:       serpent.DurationOf(&tickInterval),
		},
		{
			Flag:        "ssh",
			Env:         "CODER_SCALETEST_WORKSPACE_TRAFFIC_SSH",
			Default:     "",
			Description: "Send traffic over SSH, cannot be used with --app.",
			Value:       serpent.BoolOf(&ssh),
		},
		{
			Flag:        "app",
			Env:         "CODER_SCALETEST_WORKSPACE_TRAFFIC_APP",
			Default:     "",
			Description: "Send WebSocket traffic to a workspace app (proxied via coderd), cannot be used with --ssh.",
			Value:       serpent.StringOf(&app),
		},
	}

	tracingFlags.attach(&cmd.Options)
	strategy.attach(&cmd.Options)
	cleanupStrategy.attach(&cmd.Options)
	output.attach(&cmd.Options)
	prometheusFlags.attach(&cmd.Options)

	return cmd
}

func (r *RootCmd) scaletestDashboard() *serpent.Command {
	var (
		interval    time.Duration
		jitter      time.Duration
		headless    bool
		randSeed    int64
		targetUsers string

		client          = &codersdk.Client{}
		tracingFlags    = &scaletestTracingFlags{}
		strategy        = &scaletestStrategyFlags{}
		cleanupStrategy = &scaletestStrategyFlags{cleanup: true}
		output          = &scaletestOutputFlags{}
		prometheusFlags = &scaletestPrometheusFlags{}
	)

	cmd := &serpent.Command{
		Use:   "dashboard",
		Short: "Generate traffic to the HTTP API to simulate use of the dashboard.",
		Middleware: serpent.Chain(
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			if !(interval > 0) {
				return xerrors.Errorf("--interval must be greater than zero")
			}
			if !(jitter < interval) {
				return xerrors.Errorf("--jitter must be less than --interval")
			}
			targetUserStart, targetUserEnd, err := parseTargetRange("users", targetUsers)
			if err != nil {
				return xerrors.Errorf("parse target users: %w", err)
			}
			ctx := inv.Context()
			logger := inv.Logger.AppendSinks(sloghuman.Sink(inv.Stdout))
			if r.verbose {
				logger = logger.Leveled(slog.LevelDebug)
			}
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
			outputs, err := output.parse()
			if err != nil {
				return xerrors.Errorf("could not parse --output flags")
			}
			reg := prometheus.NewRegistry()
			prometheusSrvClose := ServeHandler(ctx, logger, promhttp.HandlerFor(reg, promhttp.HandlerOpts{}), prometheusFlags.Address, "prometheus")
			defer prometheusSrvClose()
			metrics := dashboard.NewMetrics(reg)

			th := harness.NewTestHarness(strategy.toStrategy(), cleanupStrategy.toStrategy())

			users, err := getScaletestUsers(ctx, client)
			if err != nil {
				return xerrors.Errorf("get scaletest users")
			}
			if targetUserEnd == 0 {
				targetUserEnd = len(users)
			}

			for idx, usr := range users {
				if idx < targetUserStart || idx >= targetUserEnd {
					continue
				}

				//nolint:gosec // not used for cryptographic purposes
				rndGen := rand.New(rand.NewSource(randSeed))
				name := fmt.Sprintf("dashboard-%s", usr.Username)
				userTokResp, err := client.CreateToken(ctx, usr.ID.String(), codersdk.CreateTokenRequest{
					Lifetime:  30 * 24 * time.Hour,
					Scope:     "",
					TokenName: fmt.Sprintf("scaletest-%d", time.Now().Unix()),
				})
				if err != nil {
					return xerrors.Errorf("create token for user: %w", err)
				}

				userClient := codersdk.New(client.URL)
				userClient.SetSessionToken(userTokResp.Key)

				config := dashboard.Config{
					Interval: interval,
					Jitter:   jitter,
					Trace:    tracingEnabled,
					Logger:   logger.Named(name),
					Headless: headless,
					RandIntn: rndGen.Intn,
				}
				// Only take a screenshot if we're in verbose mode.
				// This could be useful for debugging, but it will blow up the disk.
				if r.verbose {
					config.Screenshot = dashboard.Screenshot
				} else {
					// Disable screenshots otherwise.
					config.Screenshot = func(context.Context, string) (string, error) {
						return "/dev/null", nil
					}
				}
				//nolint:gocritic
				logger.Info(ctx, "runner config", slog.F("interval", interval), slog.F("jitter", jitter), slog.F("headless", headless), slog.F("trace", tracingEnabled))
				if err := config.Validate(); err != nil {
					logger.Fatal(ctx, "validate config", slog.Error(err))
					return err
				}
				var runner harness.Runnable = dashboard.NewRunner(userClient, metrics, config)
				if tracingEnabled {
					runner = &runnableTraceWrapper{
						tracer:   tracer,
						spanName: name,
						runner:   runner,
					}
				}
				th.AddRun("dashboard", name, runner)
			}

			_, _ = fmt.Fprintln(inv.Stderr, "Running load test...")
			testCtx, testCancel := strategy.toContext(ctx)
			defer testCancel()
			err = th.Run(testCtx)
			if err != nil {
				return xerrors.Errorf("run test harness (harness failure, not a test failure): %w", err)
			}

			res := th.Results()
			for _, o := range outputs {
				err = o.write(res, inv.Stdout)
				if err != nil {
					return xerrors.Errorf("write output %q to %q: %w", o.format, o.path, err)
				}
			}

			if res.TotalFail > 0 {
				return xerrors.New("load test failed, see above for more details")
			}

			return nil
		},
	}

	cmd.Options = []serpent.Option{
		{
			Flag:        "target-users",
			Env:         "CODER_SCALETEST_DASHBOARD_TARGET_USERS",
			Description: "Target a specific range of users in the format [START]:[END] (exclusive). Example: 0:10 will target the 10 first alphabetically sorted users (0-9).",
			Value:       serpent.StringOf(&targetUsers),
		},
		{
			Flag:        "interval",
			Env:         "CODER_SCALETEST_DASHBOARD_INTERVAL",
			Default:     "10s",
			Description: "Interval between actions.",
			Value:       serpent.DurationOf(&interval),
		},
		{
			Flag:        "jitter",
			Env:         "CODER_SCALETEST_DASHBOARD_JITTER",
			Default:     "5s",
			Description: "Jitter between actions.",
			Value:       serpent.DurationOf(&jitter),
		},
		{
			Flag:        "headless",
			Env:         "CODER_SCALETEST_DASHBOARD_HEADLESS",
			Default:     "true",
			Description: "Controls headless mode. Setting to false is useful for debugging.",
			Value:       serpent.BoolOf(&headless),
		},
		{
			Flag:        "rand-seed",
			Env:         "CODER_SCALETEST_DASHBOARD_RAND_SEED",
			Default:     "0",
			Description: "Seed for the random number generator.",
			Value:       serpent.Int64Of(&randSeed),
		},
	}

	tracingFlags.attach(&cmd.Options)
	strategy.attach(&cmd.Options)
	cleanupStrategy.attach(&cmd.Options)
	output.attach(&cmd.Options)
	prometheusFlags.attach(&cmd.Options)

	return cmd
}

type runnableTraceWrapper struct {
	tracer   trace.Tracer
	spanName string
	runner   harness.Runnable

	span trace.Span
}

var (
	_ harness.Runnable  = &runnableTraceWrapper{}
	_ harness.Cleanable = &runnableTraceWrapper{}
)

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

func (r *runnableTraceWrapper) Cleanup(ctx context.Context, id string, logs io.Writer) error {
	c, ok := r.runner.(harness.Cleanable)
	if !ok {
		return nil
	}

	if r.span != nil {
		ctx = trace.ContextWithSpanContext(ctx, r.span.SpanContext())
	}
	ctx, span := r.tracer.Start(ctx, r.spanName+" cleanup")
	defer span.End()

	return c.Cleanup(ctx, id, logs)
}

// newScaleTestUser returns a random username and email address that can be used
// for scale testing. The returned username is prefixed with "scaletest-" and
// the returned email address is suffixed with "@scaletest.local".
func newScaleTestUser(id string) (username string, email string, err error) {
	randStr, err := cryptorand.String(8)
	return fmt.Sprintf("scaletest-%s-%s", randStr, id), fmt.Sprintf("%s-%s@scaletest.local", randStr, id), err
}

// newScaleTestWorkspace returns a random workspace name that can be used for
// scale testing. The returned workspace name is prefixed with "scaletest-" and
// suffixed with the given id.
func newScaleTestWorkspace(id string) (name string, err error) {
	randStr, err := cryptorand.String(8)
	return fmt.Sprintf("scaletest-%s-%s", randStr, id), err
}

func isScaleTestUser(user codersdk.User) bool {
	return strings.HasSuffix(user.Email, "@scaletest.local")
}

func isScaleTestWorkspace(workspace codersdk.Workspace) bool {
	return strings.HasPrefix(workspace.OwnerName, "scaletest-") ||
		strings.HasPrefix(workspace.Name, "scaletest-")
}

func getScaletestWorkspaces(ctx context.Context, client *codersdk.Client, template string) ([]codersdk.Workspace, error) {
	var (
		pageNumber = 0
		limit      = 100
		workspaces []codersdk.Workspace
	)

	for {
		page, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
			Name:     "scaletest-",
			Template: template,
			Offset:   pageNumber * limit,
			Limit:    limit,
		})
		if err != nil {
			return nil, xerrors.Errorf("fetch scaletest workspaces page %d: %w", pageNumber, err)
		}

		pageNumber++
		if len(page.Workspaces) == 0 {
			break
		}

		pageWorkspaces := make([]codersdk.Workspace, 0, len(page.Workspaces))
		for _, w := range page.Workspaces {
			if isScaleTestWorkspace(w) {
				pageWorkspaces = append(pageWorkspaces, w)
			}
		}
		workspaces = append(workspaces, pageWorkspaces...)
	}
	return workspaces, nil
}

func getScaletestUsers(ctx context.Context, client *codersdk.Client) ([]codersdk.User, error) {
	var (
		pageNumber = 0
		limit      = 100
		users      []codersdk.User
	)

	for {
		page, err := client.Users(ctx, codersdk.UsersRequest{
			Search: "scaletest-",
			Pagination: codersdk.Pagination{
				Offset: pageNumber * limit,
				Limit:  limit,
			},
		})
		if err != nil {
			return nil, xerrors.Errorf("fetch scaletest users page %d: %w", pageNumber, err)
		}

		pageNumber++
		if len(page.Users) == 0 {
			break
		}

		pageUsers := make([]codersdk.User, 0, len(page.Users))
		for _, u := range page.Users {
			if isScaleTestUser(u) {
				pageUsers = append(pageUsers, u)
			}
		}
		users = append(users, pageUsers...)
	}

	return users, nil
}

func parseTemplate(ctx context.Context, client *codersdk.Client, organizationIDs []uuid.UUID, template string) (tpl codersdk.Template, err error) {
	if id, err := uuid.Parse(template); err == nil && id != uuid.Nil {
		tpl, err = client.Template(ctx, id)
		if err != nil {
			return tpl, xerrors.Errorf("get template by ID %q: %w", template, err)
		}
	} else {
		// List templates in all orgs until we find a match.
	orgLoop:
		for _, orgID := range organizationIDs {
			tpls, err := client.TemplatesByOrganization(ctx, orgID)
			if err != nil {
				return tpl, xerrors.Errorf("list templates in org %q: %w", orgID, err)
			}

			for _, t := range tpls {
				if t.Name == template {
					tpl = t
					break orgLoop
				}
			}
		}
	}
	if tpl.ID == uuid.Nil {
		return tpl, xerrors.Errorf("could not find template %q in any organization", template)
	}

	return tpl, nil
}

func parseTargetRange(name, targets string) (start, end int, err error) {
	if targets == "" {
		return 0, 0, nil
	}

	parts := strings.Split(targets, ":")
	if len(parts) != 2 {
		return 0, 0, xerrors.Errorf("invalid target %s %q", name, targets)
	}

	start, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, xerrors.Errorf("invalid target %s %q: %w", name, targets, err)
	}

	end, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, xerrors.Errorf("invalid target %s %q: %w", name, targets, err)
	}

	if start == end {
		return 0, 0, xerrors.Errorf("invalid target %s %q: start and end cannot be equal", name, targets)
	}
	if end < start {
		return 0, 0, xerrors.Errorf("invalid target %s %q: end cannot be less than start", name, targets)
	}

	return start, end, nil
}

func createWorkspaceAppConfig(client *codersdk.Client, appHost, app string, workspace codersdk.Workspace, agent codersdk.WorkspaceAgent) (workspacetraffic.AppConfig, error) {
	if app == "" {
		return workspacetraffic.AppConfig{}, nil
	}

	i := slices.IndexFunc(agent.Apps, func(a codersdk.WorkspaceApp) bool { return a.Slug == app })
	if i == -1 {
		return workspacetraffic.AppConfig{}, xerrors.Errorf("app %q not found in workspace %q", app, workspace.Name)
	}

	c := workspacetraffic.AppConfig{
		Name: agent.Apps[i].Slug,
	}
	if agent.Apps[i].Subdomain {
		if appHost == "" {
			return workspacetraffic.AppConfig{}, xerrors.Errorf("app %q is a subdomain app but no app host is configured", app)
		}

		c.URL = fmt.Sprintf("%s://%s", client.URL.Scheme, strings.Replace(appHost, "*", agent.Apps[i].SubdomainName, 1))
	} else {
		c.URL = fmt.Sprintf("%s/@%s/%s.%s/apps/%s", client.URL.String(), workspace.OwnerName, workspace.Name, agent.Name, agent.Apps[i].Slug)
	}

	return c, nil
}

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/tracing"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/cryptorand"
	"github.com/coder/coder/scaletest/agentconn"
	"github.com/coder/coder/scaletest/createworkspaces"
	"github.com/coder/coder/scaletest/harness"
	"github.com/coder/coder/scaletest/reconnectingpty"
	"github.com/coder/coder/scaletest/workspacebuild"
)

const scaletestTracerName = "coder_scaletest"

func (r *RootCmd) scaletest() *clibase.Cmd {
	cmd := &clibase.Cmd{
		Use:   "scaletest",
		Short: "Run a scale test against the Coder API",
		Handler: func(inv *clibase.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*clibase.Cmd{
			r.scaletestCleanup(),
			r.scaletestCreateWorkspaces(),
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

func (s *scaletestTracingFlags) attach(opts *clibase.OptionSet) {
	*opts = append(
		*opts,
		clibase.Option{
			Flag:        "trace",
			Env:         "CODER_SCALETEST_TRACE",
			Description: "Whether application tracing data is collected. It exports to a backend configured by environment variables. See: https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/protocol/exporter.md.",
			Value:       clibase.BoolOf(&s.traceEnable),
		},
		clibase.Option{
			Flag:        "trace-coder",
			Env:         "CODER_SCALETEST_TRACE_CODER",
			Description: "Whether opentelemetry traces are sent to Coder. We recommend keeping this disabled unless we advise you to enable it.",
			Value:       clibase.BoolOf(&s.traceCoder),
		},
		clibase.Option{
			Flag:        "trace-honeycomb-api-key",
			Env:         "CODER_SCALETEST_TRACE_HONEYCOMB_API_KEY",
			Description: "Enables trace exporting to Honeycomb.io using the provided API key.",
			Value:       clibase.StringOf(&s.traceHoneycombAPIKey),
		},
		clibase.Option{
			Flag:        "trace-propagate",
			Env:         "CODER_SCALETEST_TRACE_PROPAGATE",
			Description: "Enables trace propagation to the Coder backend, which will be used to correlate server-side spans with client-side spans. Only enable this if the server is configured with the exact same tracing configuration as the client.",
			Value:       clibase.BoolOf(&s.tracePropagate),
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
		Coder:     s.traceCoder,
		Honeycomb: s.traceHoneycombAPIKey,
	})
	if err != nil {
		return nil, nil, false, xerrors.Errorf("initialize tracing: %w", err)
	}

	var closeTracingOnce sync.Once
	return tracerProvider, func(ctx context.Context) error {
		var err error
		closeTracingOnce.Do(func() {
			err = closeTracing(ctx)
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

func (s *scaletestStrategyFlags) attach(opts *clibase.OptionSet) {
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
		clibase.Option{
			Flag:        concurrencyLong,
			Env:         concurrencyEnv,
			Description: concurrencyDescription,
			Default:     "1",
			Value:       clibase.Int64Of(&s.concurrency),
		},
		clibase.Option{
			Flag:        timeoutLong,
			Env:         timeoutEnv,
			Description: timeoutDescription,
			Default:     "30m",
			Value:       clibase.DurationOf(&s.timeout),
		},
		clibase.Option{
			Flag:        jobTimeoutLong,
			Env:         jobTimeoutEnv,
			Description: jobTimeoutDescription,
			Default:     "5m",
			Value:       clibase.DurationOf(&s.timeoutPerJob),
		},
	)
}

func (s *scaletestStrategyFlags) toStrategy() harness.ExecutionStrategy {
	var strategy harness.ExecutionStrategy
	if s.concurrency == 1 {
		strategy = harness.LinearExecutionStrategy{}
	} else if s.concurrency == 0 {
		strategy = harness.ConcurrentExecutionStrategy{}
	} else {
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
		err := s.Sync()
		// On Linux, EINVAL is returned when calling fsync on /dev/stdout. We
		// can safely ignore this error.
		if err != nil && !xerrors.Is(err, syscall.EINVAL) {
			return xerrors.Errorf("flush output file: %w", err)
		}
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

func (s *scaletestOutputFlags) attach(opts *clibase.OptionSet) {
	*opts = append(*opts, clibase.Option{
		Flag:        "output",
		Env:         "CODER_SCALETEST_OUTPUTS",
		Description: `Output format specs in the format "<format>[:<path>]". Not specifying a path will default to stdout. Available formats: text, json.`,
		Default:     "text",
		Value:       clibase.StringArrayOf(&s.outputSpecs),
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

func (r *RootCmd) scaletestCleanup() *clibase.Cmd {
	cleanupStrategy := &scaletestStrategyFlags{cleanup: true}
	client := new(codersdk.Client)

	cmd := &clibase.Cmd{
		Use:   "cleanup",
		Short: "Cleanup scaletest workspaces, then cleanup scaletest users.",
		Long:  "The strategy flags will apply to each stage of the cleanup process.",
		Middleware: clibase.Chain(
			r.InitClient(client),
		),
		Handler: func(inv *clibase.Invocation) error {
			ctx := inv.Context()

			_, err := requireAdmin(ctx, client)
			if err != nil {
				return err
			}

			client.HTTPClient = &http.Client{
				Transport: &headerTransport{
					transport: http.DefaultTransport,
					header: map[string][]string{
						codersdk.BypassRatelimitHeader: {"true"},
					},
				},
			}

			cliui.Infof(inv.Stdout, "Fetching scaletest workspaces...")
			var (
				pageNumber = 0
				limit      = 100
				workspaces []codersdk.Workspace
			)
			for {
				page, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
					Name:   "scaletest-",
					Offset: pageNumber * limit,
					Limit:  limit,
				})
				if err != nil {
					return xerrors.Errorf("fetch scaletest workspaces page %d: %w", pageNumber, err)
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

			cliui.Errorf(inv.Stderr, "Found %d scaletest workspaces\n", len(workspaces))
			if len(workspaces) != 0 {
				cliui.Infof(inv.Stdout, "Deleting scaletest workspaces..."+"\n")
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

				cliui.Infof(inv.Stdout, "Done deleting scaletest workspaces:"+"\n")
				res := harness.Results()
				res.PrintText(inv.Stderr)

				if res.TotalFail > 0 {
					return xerrors.Errorf("failed to delete scaletest workspaces")
				}
			}

			cliui.Infof(inv.Stdout, "Fetching scaletest users...")
			pageNumber = 0
			limit = 100
			var users []codersdk.User
			for {
				page, err := client.Users(ctx, codersdk.UsersRequest{
					Search: "scaletest-",
					Pagination: codersdk.Pagination{
						Offset: pageNumber * limit,
						Limit:  limit,
					},
				})
				if err != nil {
					return xerrors.Errorf("fetch scaletest users page %d: %w", pageNumber, err)
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

			cliui.Errorf(inv.Stderr, "Found %d scaletest users\n", len(users))
			if len(users) != 0 {
				cliui.Infof(inv.Stdout, "Deleting scaletest users..."+"\n")
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

				cliui.Infof(inv.Stdout, "Done deleting scaletest users:"+"\n")
				res := harness.Results()
				res.PrintText(inv.Stderr)

				if res.TotalFail > 0 {
					return xerrors.Errorf("failed to delete scaletest users")
				}
			}

			return nil
		},
	}

	cleanupStrategy.attach(&cmd.Options)
	return cmd
}

func (r *RootCmd) scaletestCreateWorkspaces() *clibase.Cmd {
	var (
		count          int64
		template       string
		parametersFile string
		parameters     []string // key=value

		noPlan    bool
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

		tracingFlags    = &scaletestTracingFlags{}
		strategy        = &scaletestStrategyFlags{}
		cleanupStrategy = &scaletestStrategyFlags{cleanup: true}
		output          = &scaletestOutputFlags{}
	)

	client := new(codersdk.Client)

	cmd := &clibase.Cmd{
		Use:        "create-workspaces",
		Short:      "Creates many users, then creates a workspace for each user and waits for them finish building and fully come online. Optionally runs a command inside each workspace, and connects to the workspace over WireGuard.",
		Long:       `It is recommended that all rate limits are disabled on the server before running this scaletest. This test generates many login events which will be rate limited against the (most likely single) IP.`,
		Middleware: r.InitClient(client),
		Handler: func(inv *clibase.Invocation) error {
			ctx := inv.Context()

			me, err := requireAdmin(ctx, client)
			if err != nil {
				return err
			}

			client.HTTPClient = &http.Client{
				Transport: &headerTransport{
					transport: http.DefaultTransport,
					header: map[string][]string{
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

			var tpl codersdk.Template
			if template == "" {
				return xerrors.Errorf("--template is required")
			}
			if id, err := uuid.Parse(template); err == nil && id != uuid.Nil {
				tpl, err = client.Template(ctx, id)
				if err != nil {
					return xerrors.Errorf("get template by ID %q: %w", template, err)
				}
			} else {
				// List templates in all orgs until we find a match.
			orgLoop:
				for _, orgID := range me.OrganizationIDs {
					tpls, err := client.TemplatesByOrganization(ctx, orgID)
					if err != nil {
						return xerrors.Errorf("list templates in org %q: %w", orgID, err)
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
				return xerrors.Errorf("could not find template %q in any organization", template)
			}
			templateVersion, err := client.TemplateVersion(ctx, tpl.ActiveVersionID)
			if err != nil {
				return xerrors.Errorf("get template version %q: %w", tpl.ActiveVersionID, err)
			}

			parameterSchemas, err := client.TemplateVersionSchema(ctx, templateVersion.ID)
			if err != nil {
				return xerrors.Errorf("get template version schema %q: %w", templateVersion.ID, err)
			}

			paramsMap := map[string]string{}
			if parametersFile != "" {
				fileMap, err := createParameterMapFromFile(parametersFile)
				if err != nil {
					return xerrors.Errorf("read parameters file %q: %w", parametersFile, err)
				}

				paramsMap = fileMap
			}

			for _, p := range parameters {
				parts := strings.SplitN(p, "=", 2)
				if len(parts) != 2 {
					return xerrors.Errorf("invalid parameter %q", p)
				}

				paramsMap[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
			}

			params := []codersdk.CreateParameterRequest{}
			for _, p := range parameterSchemas {
				value, ok := paramsMap[p.Name]
				if !ok {
					value = ""
				}

				params = append(params, codersdk.CreateParameterRequest{
					Name:              p.Name,
					SourceValue:       value,
					SourceScheme:      codersdk.ParameterSourceSchemeData,
					DestinationScheme: p.DefaultDestinationScheme,
				})
			}

			// Do a dry-run to ensure the template and parameters are valid
			// before we start creating users and workspaces.
			if !noPlan {
				dryRun, err := client.CreateTemplateVersionDryRun(ctx, templateVersion.ID, codersdk.CreateTemplateVersionDryRunRequest{
					WorkspaceName:   "scaletest",
					ParameterValues: params,
				})
				if err != nil {
					return xerrors.Errorf("start dry run workspace creation: %w", err)
				}
				_, _ = fmt.Fprintln(inv.Stdout, "Planning workspace...")
				err = cliui.ProvisionerJob(inv.Context(), inv.Stdout, cliui.ProvisionerJobOptions{
					Fetch: func() (codersdk.ProvisionerJob, error) {
						return client.TemplateVersionDryRun(inv.Context(), templateVersion.ID, dryRun.ID)
					},
					Cancel: func() error {
						return client.CancelTemplateVersionDryRun(inv.Context(), templateVersion.ID, dryRun.ID)
					},
					Logs: func() (<-chan codersdk.ProvisionerJobLog, io.Closer, error) {
						return client.TemplateVersionDryRunLogsAfter(inv.Context(), templateVersion.ID, dryRun.ID, 0)
					},
					// Don't show log output for the dry-run unless there's an error.
					Silent: true,
				})
				if err != nil {
					return xerrors.Errorf("dry-run workspace: %w", err)
				}
			}

			tracerProvider, closeTracing, tracingEnabled, err := tracingFlags.provider(ctx)
			if err != nil {
				return xerrors.Errorf("create tracer provider: %w", err)
			}
			defer func() {
				// Allow time for traces to flush even if command context is
				// canceled.
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				_ = closeTracing(ctx)
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
							TemplateID:      tpl.ID,
							ParameterValues: params,
						},
						NoWaitForAgents: noWaitForAgents,
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
						Init: codersdk.WorkspaceAgentReconnectingPTYInit{
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

			// Upload traces.
			if tracingEnabled {
				_, _ = fmt.Fprintln(inv.Stderr, "\nUploading traces...")
				ctx, cancel := context.WithTimeout(ctx, 1*time.Minute)
				defer cancel()
				err := closeTracing(ctx)
				if err != nil {
					_, _ = fmt.Fprintf(inv.Stderr, "\nError uploading traces: %+v\n", err)
				}
			}

			if res.TotalFail > 0 {
				return xerrors.New("load test failed, see above for more details")
			}

			return nil
		},
	}

	cmd.Options = clibase.OptionSet{
		{
			Flag:          "count",
			FlagShorthand: "c",
			Env:           "CODER_SCALETEST_COUNT",
			Default:       "1",
			Description:   "Required: Number of workspaces to create.",
			Value:         clibase.Int64Of(&count),
		},
		{
			Flag:          "template",
			FlagShorthand: "t",
			Env:           "CODER_SCALETEST_TEMPLATE",
			Description:   "Required: Name or ID of the template to use for workspaces.",
			Value:         clibase.StringOf(&template),
		},
		{
			Flag:        "parameters-file",
			Env:         "CODER_SCALETEST_PARAMETERS_FILE",
			Description: "Path to a YAML file containing the parameters to use for each workspace.",
			Value:       clibase.StringOf(&parametersFile),
		},
		{
			Flag:        "parameter",
			Env:         "CODER_SCALETEST_PARAMETERS",
			Description: "Parameters to use for each workspace. Can be specified multiple times. Overrides any existing parameters with the same name from --parameters-file. Format: key=value.",
			Value:       clibase.StringArrayOf(&parameters),
		},
		{
			Flag:        "no-plan",
			Env:         "CODER_SCALETEST_NO_PLAN",
			Description: `Skip the dry-run step to plan the workspace creation. This step ensures that the given parameters are valid for the given template.`,
			Value:       clibase.BoolOf(&noPlan),
		},
		{
			Flag:        "no-cleanup",
			Env:         "CODER_SCALETEST_NO_CLEANUP",
			Description: "Do not clean up resources after the test completes. You can cleanup manually using coder scaletest cleanup.",
			Value:       clibase.BoolOf(&noCleanup),
		},
		{
			Flag:        "no-wait-for-agents",
			Env:         "CODER_SCALETEST_NO_WAIT_FOR_AGENTS",
			Description: `Do not wait for agents to start before marking the test as succeeded. This can be useful if you are running the test against a template that does not start the agent quickly.`,
			Value:       clibase.BoolOf(&noWaitForAgents),
		},
		{
			Flag:        "run-command",
			Env:         "CODER_SCALETEST_RUN_COMMAND",
			Description: "Command to run inside each workspace using reconnecting-pty (i.e. web terminal protocol). " + "If not specified, no command will be run.",
			Value:       clibase.StringOf(&runCommand),
		},
		{
			Flag:        "run-timeout",
			Env:         "CODER_SCALETEST_RUN_TIMEOUT",
			Default:     "5s",
			Description: "Timeout for the command to complete.",
			Value:       clibase.DurationOf(&runTimeout),
		},
		{
			Flag: "run-expect-timeout",
			Env:  "CODER_SCALETEST_RUN_EXPECT_TIMEOUT",

			Description: "Expect the command to timeout." + " If the command does not finish within the given --run-timeout, it will be marked as succeeded." + " If the command finishes before the timeout, it will be marked as failed.",
			Value:       clibase.BoolOf(&runExpectTimeout),
		},
		{
			Flag:        "run-expect-output",
			Env:         "CODER_SCALETEST_RUN_EXPECT_OUTPUT",
			Description: "Expect the command to output the given string (on a single line). " + "If the command does not output the given string, it will be marked as failed.",
			Value:       clibase.StringOf(&runExpectOutput),
		},
		{
			Flag:        "run-log-output",
			Env:         "CODER_SCALETEST_RUN_LOG_OUTPUT",
			Description: "Log the output of the command to the test logs. " + "This should be left off unless you expect small amounts of output. " + "Large amounts of output will cause high memory usage.",
			Value:       clibase.BoolOf(&runLogOutput),
		},
		{
			Flag:        "connect-url",
			Env:         "CODER_SCALETEST_CONNECT_URL",
			Description: "URL to connect to inside the the workspace over WireGuard. " + "If not specified, no connections will be made over WireGuard.",
			Value:       clibase.StringOf(&connectURL),
		},
		{
			Flag:        "connect-mode",
			Env:         "CODER_SCALETEST_CONNECT_MODE",
			Default:     "derp",
			Description: "Mode to use for connecting to the workspace.",
			Value:       clibase.EnumOf(&connectMode, "derp", "direct"),
		},
		{
			Flag:        "connect-hold",
			Env:         "CODER_SCALETEST_CONNECT_HOLD",
			Default:     "30s",
			Description: "How long to hold the WireGuard connection open for.",
			Value:       clibase.DurationOf(&connectHold),
		},
		{
			Flag:        "connect-interval",
			Env:         "CODER_SCALETEST_CONNECT_INTERVAL",
			Default:     "1s",
			Value:       clibase.DurationOf(&connectInterval),
			Description: "How long to wait between making requests to the --connect-url once the connection is established.",
		},
		{
			Flag:        "connect-timeout",
			Env:         "CODER_SCALETEST_CONNECT_TIMEOUT",
			Default:     "5s",
			Description: "Timeout for each request to the --connect-url.",
			Value:       clibase.DurationOf(&connectTimeout),
		},
		{
			Flag:        "use-host-login",
			Env:         "CODER_SCALETEST_USE_HOST_LOGIN",
			Default:     "false",
			Description: "Use the use logged in on the host machine, instead of creating users.",
			Value:       clibase.BoolOf(&useHostUser),
		},
	}

	tracingFlags.attach(&cmd.Options)
	strategy.attach(&cmd.Options)
	cleanupStrategy.attach(&cmd.Options)
	output.attach(&cmd.Options)
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

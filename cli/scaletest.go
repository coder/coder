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
	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/cliflag"
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

func scaletest() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scaletest",
		Short: "Run a scale test against the Coder API",
		Long:  "Perform scale tests against the Coder server.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(
		scaletestCleanup(),
		scaletestCreateWorkspaces(),
	)

	return cmd
}

type scaletestTracingFlags struct {
	traceEnable          bool
	traceCoder           bool
	traceHoneycombAPIKey string
	tracePropagate       bool
}

func (s *scaletestTracingFlags) attach(cmd *cobra.Command) {
	cliflag.BoolVarP(cmd.Flags(), &s.traceEnable, "trace", "", "CODER_LOADTEST_TRACE", false, "Whether application tracing data is collected. It exports to a backend configured by environment variables. See: https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/protocol/exporter.md")
	cliflag.BoolVarP(cmd.Flags(), &s.traceCoder, "trace-coder", "", "CODER_LOADTEST_TRACE_CODER", false, "Whether opentelemetry traces are sent to Coder. We recommend keeping this disabled unless we advise you to enable it.")
	cliflag.StringVarP(cmd.Flags(), &s.traceHoneycombAPIKey, "trace-honeycomb-api-key", "", "CODER_LOADTEST_TRACE_HONEYCOMB_API_KEY", "", "Enables trace exporting to Honeycomb.io using the provided API key.")
	cliflag.BoolVarP(cmd.Flags(), &s.tracePropagate, "trace-propagate", "", "CODER_LOADTEST_TRACE_PROPAGATE", false, "Enables trace propagation to the Coder backend, which will be used to correlate server-side spans with client-side spans. Only enable this if the server is configured with the exact same tracing configuration as the client.")
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
	concurrency   int
	timeout       time.Duration
	timeoutPerJob time.Duration
}

func (s *scaletestStrategyFlags) attach(cmd *cobra.Command) {
	concurrencyLong, concurrencyEnv, concurrencyDescription := "concurrency", "CODER_LOADTEST_CONCURRENCY", "Number of concurrent jobs to run. 0 means unlimited."
	timeoutLong, timeoutEnv, timeoutDescription := "timeout", "CODER_LOADTEST_TIMEOUT", "Timeout for the entire test run. 0 means unlimited."
	jobTimeoutLong, jobTimeoutEnv, jobTimeoutDescription := "job-timeout", "CODER_LOADTEST_JOB_TIMEOUT", "Timeout per job. Jobs may take longer to complete under higher concurrency limits."
	if s.cleanup {
		concurrencyLong, concurrencyEnv, concurrencyDescription = "cleanup-"+concurrencyLong, "CODER_LOADTEST_CLEANUP_CONCURRENCY", strings.ReplaceAll(concurrencyDescription, "jobs", "cleanup jobs")
		timeoutLong, timeoutEnv, timeoutDescription = "cleanup-"+timeoutLong, "CODER_LOADTEST_CLEANUP_TIMEOUT", strings.ReplaceAll(timeoutDescription, "test", "cleanup")
		jobTimeoutLong, jobTimeoutEnv, jobTimeoutDescription = "cleanup-"+jobTimeoutLong, "CODER_LOADTEST_CLEANUP_JOB_TIMEOUT", strings.ReplaceAll(jobTimeoutDescription, "jobs", "cleanup jobs")
	}

	cliflag.IntVarP(cmd.Flags(), &s.concurrency, concurrencyLong, "", concurrencyEnv, 1, concurrencyDescription)
	cliflag.DurationVarP(cmd.Flags(), &s.timeout, timeoutLong, "", timeoutEnv, 30*time.Minute, timeoutDescription)
	cliflag.DurationVarP(cmd.Flags(), &s.timeoutPerJob, jobTimeoutLong, "", jobTimeoutEnv, 5*time.Minute, jobTimeoutDescription)
}

func (s *scaletestStrategyFlags) toStrategy() harness.ExecutionStrategy {
	var strategy harness.ExecutionStrategy
	if s.concurrency == 1 {
		strategy = harness.LinearExecutionStrategy{}
	} else if s.concurrency == 0 {
		strategy = harness.ConcurrentExecutionStrategy{}
	} else {
		strategy = harness.ParallelExecutionStrategy{
			Limit: s.concurrency,
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

func (s *scaletestOutputFlags) attach(cmd *cobra.Command) {
	cliflag.StringArrayVarP(cmd.Flags(), &s.outputSpecs, "output", "", "CODER_SCALETEST_OUTPUTS", []string{"text"}, `Output format specs in the format "<format>[:<path>]". Not specifying a path will default to stdout. Available formats: text, json.`)
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

func scaletestCleanup() *cobra.Command {
	var (
		cleanupStrategy = &scaletestStrategyFlags{cleanup: true}
	)

	cmd := &cobra.Command{
		Use:   "cleanup",
		Short: "Cleanup any orphaned scaletest resources",
		Long:  "Cleanup scaletest workspaces, then cleanup scaletest users. The strategy flags will apply to each stage of the cleanup process.",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			client, err := CreateClient(cmd)
			if err != nil {
				return err
			}

			_, err = requireAdmin(ctx, client)
			if err != nil {
				return err
			}

			client.HTTPClient = &http.Client{
				Transport: &headerTransport{
					transport: http.DefaultTransport,
					headers: map[string]string{
						codersdk.BypassRatelimitHeader: "true",
					},
				},
			}

			cmd.PrintErrln("Fetching scaletest workspaces...")
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

			cmd.PrintErrf("Found %d scaletest workspaces\n", len(workspaces))
			if len(workspaces) != 0 {
				cmd.Println("Deleting scaletest workspaces...")
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

				cmd.Println("Done deleting scaletest workspaces:")
				res := harness.Results()
				res.PrintText(cmd.ErrOrStderr())

				if res.TotalFail > 0 {
					return xerrors.Errorf("failed to delete scaletest workspaces")
				}
			}

			cmd.PrintErrln("Fetching scaletest users...")
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

			cmd.PrintErrf("Found %d scaletest users\n", len(users))
			if len(workspaces) != 0 {
				cmd.Println("Deleting scaletest users...")
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

				cmd.Println("Done deleting scaletest users:")
				res := harness.Results()
				res.PrintText(cmd.ErrOrStderr())

				if res.TotalFail > 0 {
					return xerrors.Errorf("failed to delete scaletest users")
				}
			}

			return nil
		},
	}

	cleanupStrategy.attach(cmd)
	return cmd
}

func scaletestCreateWorkspaces() *cobra.Command {
	var (
		count          int
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

		tracingFlags    = &scaletestTracingFlags{}
		strategy        = &scaletestStrategyFlags{}
		cleanupStrategy = &scaletestStrategyFlags{cleanup: true}
		output          = &scaletestOutputFlags{}
	)

	cmd := &cobra.Command{
		Use:   "create-workspaces",
		Short: "Creates many workspaces and waits for them to be ready",
		Long: `Creates many users, then creates a workspace for each user and waits for them finish building and fully come online. Optionally runs a command inside each workspace, and connects to the workspace over WireGuard.

It is recommended that all rate limits are disabled on the server before running this scaletest. This test generates many login events which will be rate limited against the (most likely single) IP.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			client, err := CreateClient(cmd)
			if err != nil {
				return err
			}

			me, err := requireAdmin(ctx, client)
			if err != nil {
				return err
			}

			client.HTTPClient = &http.Client{
				Transport: &headerTransport{
					transport: http.DefaultTransport,
					headers: map[string]string{
						codersdk.BypassRatelimitHeader: "true",
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
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Planning workspace...")
				err = cliui.ProvisionerJob(cmd.Context(), cmd.OutOrStdout(), cliui.ProvisionerJobOptions{
					Fetch: func() (codersdk.ProvisionerJob, error) {
						return client.TemplateVersionDryRun(cmd.Context(), templateVersion.ID, dryRun.ID)
					},
					Cancel: func() error {
						return client.CancelTemplateVersionDryRun(cmd.Context(), templateVersion.ID, dryRun.ID)
					},
					Logs: func() (<-chan codersdk.ProvisionerJobLog, io.Closer, error) {
						return client.TemplateVersionDryRunLogsAfter(cmd.Context(), templateVersion.ID, dryRun.ID, 0)
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
			for i := 0; i < count; i++ {
				const name = "workspacebuild"
				id := strconv.Itoa(i)

				username, email, err := newScaleTestUser(id)
				if err != nil {
					return xerrors.Errorf("create scaletest username and email: %w", err)
				}
				workspaceName, err := newScaleTestWorkspace(id)
				if err != nil {
					return xerrors.Errorf("create scaletest workspace name: %w", err)
				}

				config := createworkspaces.Config{
					User: createworkspaces.UserConfig{
						// TODO: configurable org
						OrganizationID: me.OrganizationIDs[0],
						Username:       username,
						Email:          email,
					},
					Workspace: workspacebuild.Config{
						OrganizationID: me.OrganizationIDs[0],
						// UserID is set by the test automatically.
						Request: codersdk.CreateWorkspaceRequest{
							TemplateID:      tpl.ID,
							Name:            workspaceName,
							ParameterValues: params,
						},
						NoWaitForAgents: noWaitForAgents,
					},
					NoCleanup: noCleanup,
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
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "Running load test...")
			testCtx, testCancel := strategy.toContext(ctx)
			defer testCancel()
			err = th.Run(testCtx)
			if err != nil {
				return xerrors.Errorf("run test harness (harness failure, not a test failure): %w", err)
			}

			res := th.Results()
			for _, o := range outputs {
				err = o.write(res, cmd.OutOrStdout())
				if err != nil {
					return xerrors.Errorf("write output %q to %q: %w", o.format, o.path, err)
				}
			}

			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "\nCleaning up...")
			cleanupCtx, cleanupCancel := cleanupStrategy.toContext(ctx)
			defer cleanupCancel()
			err = th.Cleanup(cleanupCtx)
			if err != nil {
				return xerrors.Errorf("cleanup tests: %w", err)
			}

			// Upload traces.
			if tracingEnabled {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "\nUploading traces...")
				ctx, cancel := context.WithTimeout(ctx, 1*time.Minute)
				defer cancel()
				err := closeTracing(ctx)
				if err != nil {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "\nError uploading traces: %+v\n", err)
				}
			}

			if res.TotalFail > 0 {
				return xerrors.New("load test failed, see above for more details")
			}

			return nil
		},
	}

	cliflag.IntVarP(cmd.Flags(), &count, "count", "c", "CODER_LOADTEST_COUNT", 1, "Required: Number of workspaces to create.")
	cliflag.StringVarP(cmd.Flags(), &template, "template", "t", "CODER_LOADTEST_TEMPLATE", "", "Required: Name or ID of the template to use for workspaces.")
	cliflag.StringVarP(cmd.Flags(), &parametersFile, "parameters-file", "", "CODER_LOADTEST_PARAMETERS_FILE", "", "Path to a YAML file containing the parameters to use for each workspace.")
	cliflag.StringArrayVarP(cmd.Flags(), &parameters, "parameter", "", "CODER_LOADTEST_PARAMETERS", []string{}, "Parameters to use for each workspace. Can be specified multiple times. Overrides any existing parameters with the same name from --parameters-file. Format: key=value")

	cliflag.BoolVarP(cmd.Flags(), &noPlan, "no-plan", "", "CODER_LOADTEST_NO_PLAN", false, "Skip the dry-run step to plan the workspace creation. This step ensures that the given parameters are valid for the given template.")
	cliflag.BoolVarP(cmd.Flags(), &noCleanup, "no-cleanup", "", "CODER_LOADTEST_NO_CLEANUP", false, "Do not clean up resources after the test completes. You can cleanup manually using `coder scaletest cleanup`.")
	// cliflag.BoolVarP(cmd.Flags(), &noCleanupFailures, "no-cleanup-failures", "", "CODER_LOADTEST_NO_CLEANUP_FAILURES", false, "Do not clean up resources from failed jobs to aid in debugging failures. You can cleanup manually using `coder scaletest cleanup`.")
	cliflag.BoolVarP(cmd.Flags(), &noWaitForAgents, "no-wait-for-agents", "", "CODER_LOADTEST_NO_WAIT_FOR_AGENTS", false, "Do not wait for agents to start before marking the test as succeeded. This can be useful if you are running the test against a template that does not start the agent quickly.")

	cliflag.StringVarP(cmd.Flags(), &runCommand, "run-command", "", "CODER_LOADTEST_RUN_COMMAND", "", "Command to run inside each workspace using reconnecting-pty (i.e. web terminal protocol). If not specified, no command will be run.")
	cliflag.DurationVarP(cmd.Flags(), &runTimeout, "run-timeout", "", "CODER_LOADTEST_RUN_TIMEOUT", 5*time.Second, "Timeout for the command to complete.")
	cliflag.BoolVarP(cmd.Flags(), &runExpectTimeout, "run-expect-timeout", "", "CODER_LOADTEST_RUN_EXPECT_TIMEOUT", false, "Expect the command to timeout. If the command does not finish within the given --run-timeout, it will be marked as succeeded. If the command finishes before the timeout, it will be marked as failed.")
	cliflag.StringVarP(cmd.Flags(), &runExpectOutput, "run-expect-output", "", "CODER_LOADTEST_RUN_EXPECT_OUTPUT", "", "Expect the command to output the given string (on a single line). If the command does not output the given string, it will be marked as failed.")
	cliflag.BoolVarP(cmd.Flags(), &runLogOutput, "run-log-output", "", "CODER_LOADTEST_RUN_LOG_OUTPUT", false, "Log the output of the command to the test logs. This should be left off unless you expect small amounts of output. Large amounts of output will cause high memory usage.")

	cliflag.StringVarP(cmd.Flags(), &connectURL, "connect-url", "", "CODER_LOADTEST_CONNECT_URL", "", "URL to connect to inside the the workspace over WireGuard. If not specified, no connections will be made over WireGuard.")
	cliflag.StringVarP(cmd.Flags(), &connectMode, "connect-mode", "", "CODER_LOADTEST_CONNECT_MODE", "derp", "Mode to use for connecting to the workspace. Can be 'derp' or 'direct'.")
	cliflag.DurationVarP(cmd.Flags(), &connectHold, "connect-hold", "", "CODER_LOADTEST_CONNECT_HOLD", 30*time.Second, "How long to hold the WireGuard connection open for.")
	cliflag.DurationVarP(cmd.Flags(), &connectInterval, "connect-interval", "", "CODER_LOADTEST_CONNECT_INTERVAL", time.Second, "How long to wait between making requests to the --connect-url once the connection is established.")
	cliflag.DurationVarP(cmd.Flags(), &connectTimeout, "connect-timeout", "", "CODER_LOADTEST_CONNECT_TIMEOUT", 5*time.Second, "Timeout for each request to the --connect-url.")

	tracingFlags.attach(cmd)
	strategy.attach(cmd)
	cleanupStrategy.attach(cmd)
	output.attach(cmd)
	return cmd
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
	if !strings.HasPrefix(workspace.OwnerName, "scaletest-") {
		return false
	}

	return strings.HasPrefix(workspace.Name, "scaletest-")
}

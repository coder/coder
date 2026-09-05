//go:build !slim

package cli

import (
	"context"
	"fmt"
	"os/signal"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/createusers"
	"github.com/coder/coder/v2/scaletest/harness"
	"github.com/coder/coder/v2/scaletest/loadtestutil"
	"github.com/coder/coder/v2/scaletest/workspacebuild"
	"github.com/coder/coder/v2/scaletest/workspaceupdates"
	"github.com/coder/serpent"
)

func (r *RootCmd) scaletestWorkspaceUpdates() *serpent.Command {
	var (
		workspaceCount          int64
		powerUserWorkspaces     int64
		powerUserPercentage     float64
		workspaceUpdatesTimeout time.Duration
		dialTimeout             time.Duration
		template                string
		noCleanup               bool
		reuseUsers              bool
		usernameInfix           string

		parameterFlags workspaceParameterFlags
		tracingFlags   = &scaletestTracingFlags{}
		// This test requires unlimited concurrency
		timeoutStrategy = &timeoutFlags{}
		cleanupStrategy = newScaletestCleanupStrategy()
		output          = &scaletestOutputFlags{}
		prometheusFlags = &scaletestPrometheusFlags{}
	)

	cmd := &serpent.Command{
		Use:   "workspace-updates",
		Short: "Simulate the load of Coder Desktop clients receiving workspace updates",
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			client, err := r.TryInitClient(inv)
			if err != nil {
				return err
			}

			notifyCtx, stop := signal.NotifyContext(ctx, StopSignals...) // Checked later.
			defer stop()
			ctx = notifyCtx

			me, err := RequireAdmin(ctx, client)
			if err != nil {
				return err
			}

			if workspaceCount <= 0 {
				return xerrors.Errorf("--workspace-count must be greater than 0")
			}
			if powerUserWorkspaces <= 1 {
				return xerrors.Errorf("--power-user-workspaces must be greater than 1")
			}
			if powerUserPercentage < 0 || powerUserPercentage > 100 {
				return xerrors.Errorf("--power-user-proportion must be between 0 and 100")
			}

			powerUserWorkspaceCount := int64(float64(workspaceCount) * powerUserPercentage / 100)
			remainder := powerUserWorkspaceCount % powerUserWorkspaces
			// If the power user workspaces can't be evenly divided, round down
			// to the nearest multiple so that we only have two groups of users.
			workspaceCount -= remainder
			powerUserWorkspaceCount -= remainder
			powerUserCount := powerUserWorkspaceCount / powerUserWorkspaces
			regularWorkspaceCount := workspaceCount - powerUserWorkspaceCount
			regularUserCount := regularWorkspaceCount
			regularUserWorkspaceCount := 1

			_, _ = fmt.Fprintf(inv.Stderr, "Distribution plan:\n")
			_, _ = fmt.Fprintf(inv.Stderr, "  Total workspaces: %d\n", workspaceCount)
			_, _ = fmt.Fprintf(inv.Stderr, "  Power users: %d (each owning %d workspaces = %d total)\n",
				powerUserCount, powerUserWorkspaces, powerUserWorkspaceCount)
			_, _ = fmt.Fprintf(inv.Stderr, "  Regular users: %d (each owning %d workspace = %d total)\n",
				regularUserCount, regularUserWorkspaceCount, regularWorkspaceCount)

			outputs, err := output.parse()
			if err != nil {
				return xerrors.Errorf("could not parse --output flags")
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
				Owner:             codersdk.Me,

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
			tracer := tracerProvider.Tracer(scaletestTracerName)

			reg := prometheus.NewRegistry()
			metrics := workspaceupdates.NewMetrics(reg)

			logger := inv.Logger
			prometheusSrvClose := ServeHandler(ctx, logger, promhttp.HandlerFor(reg, promhttp.HandlerOpts{}), prometheusFlags.Address, "prometheus")
			defer prometheusSrvClose()

			defer func() {
				_, _ = fmt.Fprintln(inv.Stderr, "\nUploading traces...")
				if err := closeTracing(ctx); err != nil {
					_, _ = fmt.Fprintf(inv.Stderr, "\nError uploading traces: %+v\n", err)
				}
				// Wait for prometheus metrics to be scraped
				_, _ = fmt.Fprintf(inv.Stderr, "Waiting %s for prometheus metrics to be scraped\n", prometheusFlags.Wait)
				<-time.After(prometheusFlags.Wait)
			}()

			_, _ = fmt.Fprintln(inv.Stderr, "Creating users...")

			dialBarrier := new(sync.WaitGroup)
			dialBarrier.Add(int(powerUserCount + regularUserCount))

			var reuse []workspaceUpdatesReuseUser
			if reuseUsers {
				// Bound token lifetime to just beyond the run so tokens orphaned by a
				// hard kill expire quickly rather than at the deployment default.
				tokenLifetime := workspaceUpdatesTimeout + dialTimeout + time.Hour
				reuse, err = selectWorkspaceUpdatesReuseUsers(ctx, client, usernameInfix, int(powerUserCount+regularUserCount), tokenLifetime)
				if err != nil {
					return err
				}
			}

			configs := make([]workspaceupdates.Config, 0, powerUserCount+regularUserCount)

			for i := range powerUserCount {
				config := workspaceupdates.Config{
					User: createusers.Config{
						OrganizationID: me.OrganizationIDs[0],
					},
					Workspace: workspacebuild.Config{
						OrganizationID: me.OrganizationIDs[0],
						Request: codersdk.CreateWorkspaceRequest{
							TemplateID:          tpl.ID,
							RichParameterValues: richParameters,
						},
						NoWaitForAgents: true,
					},
					WorkspaceCount:          powerUserWorkspaces,
					WorkspaceUpdatesTimeout: workspaceUpdatesTimeout,
					DialTimeout:             dialTimeout,
					Metrics:                 metrics,
					DialBarrier:             dialBarrier,
				}
				if reuseUsers {
					config.SessionToken = reuse[i].sessionToken
					config.PreCreatedUser = reuse[i].user
				}
				if err := config.Validate(); err != nil {
					return xerrors.Errorf("validate config: %w", err)
				}
				configs = append(configs, config)
			}

			for i := range regularUserCount {
				config := workspaceupdates.Config{
					User: createusers.Config{
						OrganizationID: me.OrganizationIDs[0],
					},
					Workspace: workspacebuild.Config{
						OrganizationID: me.OrganizationIDs[0],
						Request: codersdk.CreateWorkspaceRequest{
							TemplateID:          tpl.ID,
							RichParameterValues: richParameters,
						},
						NoWaitForAgents: true,
					},
					WorkspaceCount:          int64(regularUserWorkspaceCount),
					WorkspaceUpdatesTimeout: workspaceUpdatesTimeout,
					DialTimeout:             dialTimeout,
					Metrics:                 metrics,
					DialBarrier:             dialBarrier,
				}
				if reuseUsers {
					reuseIdx := powerUserCount + i
					config.SessionToken = reuse[reuseIdx].sessionToken
					config.PreCreatedUser = reuse[reuseIdx].user
				}
				if err := config.Validate(); err != nil {
					return xerrors.Errorf("validate config: %w", err)
				}
				configs = append(configs, config)
			}

			th := harness.NewTestHarness(timeoutStrategy.wrapStrategy(harness.ConcurrentExecutionStrategy{}), cleanupStrategy.toStrategy())
			for i, config := range configs {
				name := fmt.Sprintf("workspaceupdates-%dw", config.WorkspaceCount)
				id := strconv.Itoa(i)

				// use an independent client for each Runner, so they don't reuse TCP connections. This can lead to
				// requests being unbalanced among Coder instances.
				runnerClient, err := loadtestutil.DupClientCopyingHeaders(client, BypassHeader)
				if err != nil {
					return xerrors.Errorf("create runner client: %w", err)
				}
				var runner harness.Runnable = workspaceupdates.NewRunner(runnerClient, config)
				if tracingEnabled {
					runner = &runnableTraceWrapper{
						tracer:   tracer,
						spanName: fmt.Sprintf("%s/%s", name, id),
						runner:   runner,
					}
				}

				th.AddRun(name, id, runner)
			}

			_, _ = fmt.Fprintln(inv.Stderr, "Running workspace updates scaletest...")
			testCtx, testCancel := timeoutStrategy.toContext(ctx)
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

			if !noCleanup {
				_, _ = fmt.Fprintln(inv.Stderr, "\nCleaning up...")
				cleanupCtx, cleanupCancel := cleanupStrategy.toContext(ctx)
				defer cleanupCancel()
				err = th.Cleanup(cleanupCtx)
				if err != nil {
					return xerrors.Errorf("cleanup tests: %w", err)
				}
			}

			if res.TotalFail > 0 {
				return xerrors.New("load test failed, see above for more details")
			}

			return nil
		},
	}

	cmd.Options = serpent.OptionSet{
		{
			Flag:          "workspace-count",
			FlagShorthand: "c",
			Env:           "CODER_SCALETEST_WORKSPACE_COUNT",
			Description:   "Required: Total number of workspaces to create.",
			Value:         serpent.Int64Of(&workspaceCount),
			Required:      true,
		},
		{
			Flag:        "power-user-workspaces",
			Env:         "CODER_SCALETEST_POWER_USER_WORKSPACES",
			Description: "Number of workspaces each power-user owns.",
			Value:       serpent.Int64Of(&powerUserWorkspaces),
			Required:    true,
		},
		{
			Flag:        "power-user-percentage",
			Env:         "CODER_SCALETEST_POWER_USER_PERCENTAGE",
			Default:     "50.0",
			Description: "Percentage of total workspaces owned by power-users (0-100).",
			Value:       serpent.Float64Of(&powerUserPercentage),
		},
		{
			Flag:        "workspace-updates-timeout",
			Env:         "CODER_SCALETEST_WORKSPACE_UPDATES_TIMEOUT",
			Default:     "5m",
			Description: "How long to wait for all expected workspace updates.",
			Value:       serpent.DurationOf(&workspaceUpdatesTimeout),
		},
		{
			Flag:        "dial-timeout",
			Env:         "CODER_SCALETEST_DIAL_TIMEOUT",
			Default:     "2m",
			Description: "Timeout for dialing the tailnet endpoint.",
			Value:       serpent.DurationOf(&dialTimeout),
		},
		{
			Flag:          "template",
			FlagShorthand: "t",
			Env:           "CODER_SCALETEST_TEMPLATE",
			Description:   "Required: Name or ID of the template to use for workspaces.",
			Value:         serpent.StringOf(&template),
			Required:      true,
		},
		{
			Flag:        "no-cleanup",
			Env:         "CODER_SCALETEST_NO_CLEANUP",
			Description: "Do not clean up resources after the test completes.",
			Value:       serpent.BoolOf(&noCleanup),
		},
		{
			Flag:        "reuse-users",
			Env:         "CODER_SCALETEST_WORKSPACE_UPDATES_REUSE_USERS",
			Description: "Reuse existing scaletest users instead of creating new ones. At least --workspace-count worth of users (one per power user plus one per regular user) must already exist (see \"coder exp scaletest create-users\") or the command errors. Run only one user-selecting scaletest command at a time to avoid overlapping selections.",
			Value:       serpent.BoolOf(&reuseUsers),
		},
		{
			Flag:        "username-infix",
			Env:         "CODER_SCALETEST_WORKSPACE_UPDATES_USERNAME_INFIX",
			Description: "Username infix identifying the user pool to reuse when --reuse-users is set. It must match the --username-infix used by the create-users run that provisioned the pool: for example asdf selects users named scaletest-asdf-<random>-<id> so this test reuses only its own pool and does not compete with other load generators. Leave empty to select any scaletest- user. Ignored without --reuse-users.",
			Value:       serpent.StringOf(&usernameInfix),
		},
	}

	cmd.Options = append(cmd.Options, parameterFlags.cliParameters()...)
	tracingFlags.attach(&cmd.Options)
	timeoutStrategy.attach(&cmd.Options)
	cleanupStrategy.attach(&cmd.Options)
	output.attach(&cmd.Options)
	prometheusFlags.attach(&cmd.Options)
	return cmd
}

type workspaceUpdatesReuseUser struct {
	user         codersdk.User
	sessionToken string
}

// selectWorkspaceUpdatesReuseUsers selects count existing scaletest users
// belonging to the pool identified by usernameInfix and mints a token for each,
// erroring if the pool lacks enough users. usernameInfix is inserted between the
// mandatory "scaletest-" root and the rest of the username (empty selects any
// scaletest- user), so it isolates this run from other load generators that
// reuse scaletest users concurrently. Unlike the notifications test, the
// workspace-updates test does not distinguish user roles: power users and
// regular users differ only in how many workspaces they own.
func selectWorkspaceUpdatesReuseUsers(ctx context.Context, client *codersdk.Client, usernameInfix string, count int, tokenLifetime time.Duration) ([]workspaceUpdatesReuseUser, error) {
	// The scaletest- root is always kept so selection matches the users that
	// create-users provisions; usernameInfix, when set, narrows to that pool.
	searchPrefix := loadtestutil.ScaleTestPrefix + "-"
	if usernameInfix != "" {
		searchPrefix += usernameInfix + "-"
	}
	users, err := getScaletestUsersWithPrefix(ctx, client, searchPrefix)
	if err != nil {
		return nil, xerrors.Errorf("list scaletest users: %w", err)
	}

	if len(users) < count {
		hint := fmt.Sprintf("coder exp scaletest create-users --count %d --no-cleanup", count)
		if usernameInfix != "" {
			hint = fmt.Sprintf("coder exp scaletest create-users --count %d --username-infix %q --no-cleanup", count, usernameInfix)
		}
		return nil, xerrors.Errorf(
			"not enough scaletest users to reuse: found %d, need %d. Create them first, for example: %s",
			len(users), count, hint)
	}

	// Token names are auto-generated by the server; we only need the returned key.
	reuse := make([]workspaceUpdatesReuseUser, 0, count)
	for _, u := range users[:count] {
		res, err := client.CreateToken(ctx, u.ID.String(), codersdk.CreateTokenRequest{
			Lifetime: tokenLifetime,
		})
		if err != nil {
			return nil, xerrors.Errorf("mint token for user %q: %w", u.Username, err)
		}
		reuse = append(reuse, workspaceUpdatesReuseUser{user: u, sessionToken: res.Key})
	}

	return reuse, nil
}

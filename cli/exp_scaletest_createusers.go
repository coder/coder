//go:build !slim

package cli

import (
	"context"
	"fmt"
	"io"
	"os/signal"
	"strconv"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/createusers"
	"github.com/coder/coder/v2/scaletest/harness"
	"github.com/coder/coder/v2/scaletest/loadtestutil"
	"github.com/coder/serpent"
)

func (r *RootCmd) scaletestCreateUsers() *serpent.Command {
	var (
		count                   int64
		templateAdminPercentage float64
		usernamePrefix          string
		noCleanup               bool

		tracingFlags    = &scaletestTracingFlags{}
		strategy        = &scaletestStrategyFlags{}
		cleanupStrategy = newScaletestCleanupStrategy()
		output          = &scaletestOutputFlags{}
	)

	cmd := &serpent.Command{
		Use:   "create-users",
		Short: "Create many users for use by other scale tests.",
		Long: "Pre-provision a fixed number of users (optionally promoting a percentage to " +
			"template admin) so that other load generators can target existing users instead " +
			"of creating their own inline. Intended to be run against dedicated, throwaway " +
			"scaletest deployments.",
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			notifyCtx, stop := signal.NotifyContext(ctx, StopSignals...)
			defer stop()
			ctx = notifyCtx

			me, err := RequireAdmin(ctx, client)
			if err != nil {
				return err
			}

			if count <= 0 {
				return xerrors.New("--count must be greater than 0")
			}
			if templateAdminPercentage < 0 || templateAdminPercentage > 100 {
				return xerrors.New("--template-admin-percentage must be between 0 and 100")
			}

			// int64 truncates toward zero, so this floors the count. Ensure at
			// least one admin whenever a nonzero percentage was requested.
			templateAdminCount := int64(float64(count) * templateAdminPercentage / 100)
			if templateAdminPercentage > 0 {
				templateAdminCount = max(templateAdminCount, 1)
			}
			regularUserCount := count - templateAdminCount

			_, _ = fmt.Fprintf(inv.Stderr, "Distribution plan:\n")
			_, _ = fmt.Fprintf(inv.Stderr, "  Total users: %d\n", count)
			_, _ = fmt.Fprintf(inv.Stderr, "  Template admins: %d (%.1f%%)\n", templateAdminCount, templateAdminPercentage)
			_, _ = fmt.Fprintf(inv.Stderr, "  Regular users: %d (%.1f%%)\n", regularUserCount, 100.0-templateAdminPercentage)

			outputs, err := output.parse()
			if err != nil {
				return xerrors.Errorf("could not parse --output flags")
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
			}()
			tracer := tracerProvider.Tracer(scaletestTracerName)

			th := harness.NewTestHarness(strategy.toStrategy(), cleanupStrategy.toStrategy())

			// The scaletest- root is always kept so the cleanup command still
			// finds these users; --username-prefix is inserted between it and the
			// random suffix, e.g. --username-prefix asdf produces usernames like
			// scaletest-asdf-<random>-<id>.
			userPrefix := loadtestutil.ScaleTestPrefix + "-"
			if usernamePrefix != "" {
				userPrefix += usernamePrefix + "-"
			}

			_, _ = fmt.Fprintln(inv.Stderr, "Creating users...")
			for i := range count {
				id := strconv.FormatInt(i, 10)
				name := fmt.Sprintf("createusers-%s", id)

				var roles []string
				if i < templateAdminCount {
					roles = []string{codersdk.RoleTemplateAdmin}
				}

				// Use an independent client per runner so they don't reuse TCP
				// connections, which can unbalance requests among Coder replicas.
				runnerClient, err := loadtestutil.DupClientCopyingHeaders(client, BypassHeader)
				if err != nil {
					return xerrors.Errorf("create runner client: %w", err)
				}

				username, email, err := loadtestutil.GenerateUserIdentifierWithPrefix(userPrefix, id)
				if err != nil {
					return xerrors.Errorf("generate user identifier: %w", err)
				}

				config := createusers.Config{
					OrganizationID: me.OrganizationIDs[0],
					Username:       username,
					Email:          email,
				}
				if err := config.Validate(); err != nil {
					return xerrors.Errorf("validate config: %w", err)
				}

				var runner harness.Runnable = &createUsersRunner{
					client: runnerClient,
					runner: createusers.NewRunner(runnerClient, config),
					roles:  roles,
				}
				if tracingEnabled {
					runner = &runnableTraceWrapper{
						tracer:   tracer,
						spanName: name,
						runner:   runner,
					}
				}

				th.AddRun(name, id, runner)
			}

			_, _ = fmt.Fprintln(inv.Stderr, "Running user creation scaletest...")
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

			if noCleanup {
				_, _ = fmt.Fprintln(inv.Stderr, "\nSkipping cleanup (users left in place for other scale tests).")
			} else {
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
			Flag:          "count",
			FlagShorthand: "c",
			Env:           "CODER_SCALETEST_CREATE_USERS_COUNT",
			Description:   "Required: Total number of users to create.",
			Value:         serpent.Int64Of(&count),
			Required:      true,
		},
		{
			Flag:        "template-admin-percentage",
			Env:         "CODER_SCALETEST_CREATE_USERS_TEMPLATE_ADMIN_PERCENTAGE",
			Default:     "20.0",
			Description: "Percentage of users to assign the Template Admin role to (0-100).",
			Value:       serpent.Float64Of(&templateAdminPercentage),
		},
		{
			Flag:        "username-prefix",
			Env:         "CODER_SCALETEST_CREATE_USERS_USERNAME_PREFIX",
			Description: "Optional sub-prefix inserted between the mandatory \"scaletest-\" root and the rest of each generated username. For example --username-prefix asdf produces usernames like scaletest-asdf-<random>-<id>; leave empty for the default scaletest-<random>-<id> names. Use it to partition users into disjoint pools that a matching --reuse-users run can select. The scaletest- root is always kept so the cleanup command still discovers the users.",
			Value:       serpent.StringOf(&usernamePrefix),
		},
		{
			Flag:        "no-cleanup",
			Env:         "CODER_SCALETEST_NO_CLEANUP",
			Description: "Do not clean up resources after the test completes. Use this when other scale tests will reuse the created users.",
			Value:       serpent.BoolOf(&noCleanup),
		},
	}

	tracingFlags.attach(&cmd.Options)
	strategy.attach(&cmd.Options)
	cleanupStrategy.attach(&cmd.Options)
	output.attach(&cmd.Options)
	return cmd
}

// createUsersRunner adapts createusers.Runner to harness.Runnable and assigns
// roles after creation. The client must be a site admin to assign roles.
type createUsersRunner struct {
	client *codersdk.Client
	runner *createusers.Runner
	roles  []string
}

var (
	_ harness.Runnable  = &createUsersRunner{}
	_ harness.Cleanable = &createUsersRunner{}
)

func (r *createUsersRunner) Run(ctx context.Context, id string, logs io.Writer) error {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	user, err := r.runner.RunReturningUser(ctx, id, logs)
	if err != nil {
		return xerrors.Errorf("create user: %w", err)
	}

	if len(r.roles) > 0 {
		_, _ = fmt.Fprintf(logs, "Assigning roles to user %q: %v\n", user.Username, r.roles)
		if _, err := r.client.UpdateUserRoles(ctx, user.ID.String(), codersdk.UpdateRoles{
			Roles: r.roles,
		}); err != nil {
			return xerrors.Errorf("assign roles %v to user %q: %w", r.roles, user.Username, err)
		}
	}

	return nil
}

func (r *createUsersRunner) Cleanup(ctx context.Context, id string, logs io.Writer) error {
	return r.runner.Cleanup(ctx, id, logs)
}

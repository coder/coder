package cli_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/loadtestutil"
	"github.com/coder/coder/v2/testutil"
)

func TestScaleTestCreateWorkspaces(t *testing.T) {
	t.Parallel()

	if testutil.RaceEnabled() {
		t.Skip("Skipping due to race detector")
	}

	// This test only validates that the CLI command accepts known arguments.
	// More thorough testing is done in scaletest/createworkspaces/run_test.go.
	ctx, cancelFunc := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancelFunc()

	log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	client := coderdtest.New(t, &coderdtest.Options{
		// We are not including any provisioner daemons because we do not actually
		// build any workspaces here.
		Logger: &log,
	})
	_ = coderdtest.CreateFirstUser(t, client)

	// Write a parameters file.
	tDir := t.TempDir()
	outputFile := filepath.Join(tDir, "output.json")

	inv, root := clitest.New(t, "exp", "scaletest", "create-workspaces",
		"--count", "2",
		"--template", "doesnotexist",
		"--no-cleanup",
		"--no-wait-for-agents",
		"--concurrency", "2",
		"--timeout", "30s",
		"--job-timeout", "15s",
		"--cleanup-concurrency", "1",
		"--cleanup-timeout", "30s",
		"--cleanup-job-timeout", "15s",
		"--output", "text",
		"--output", "json:"+outputFile,
		"--parameter", "foo=baz",
		"--rich-parameter-file", "/path/to/some/parameter/file.ext",
		"--max-failures", "1",
	)
	clitest.SetupConfig(t, client, root)
	err := inv.WithContext(ctx).Run()
	require.ErrorContains(t, err, "could not find template \"doesnotexist\" in any organization")
}

// This test just validates that the CLI command accepts its known arguments.
// A more comprehensive test is performed in workspacetraffic/run_test.go
func TestScaleTestWorkspaceTraffic(t *testing.T) {
	t.Parallel()

	if testutil.RaceEnabled() {
		t.Skip("Skipping due to race detector")
	}

	ctx, cancelFunc := context.WithTimeout(context.Background(), testutil.WaitMedium)
	defer cancelFunc()

	log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	client := coderdtest.New(t, &coderdtest.Options{
		Logger: &log,
	})
	_ = coderdtest.CreateFirstUser(t, client)

	inv, root := clitest.New(t, "exp", "scaletest", "workspace-traffic",
		"--timeout", "1s",
		"--bytes-per-tick", "1024",
		"--tick-interval", "100ms",
		"--scaletest-prometheus-address", "127.0.0.1:0",
		"--scaletest-prometheus-wait", "0s",
		"--ssh",
	)
	clitest.SetupConfig(t, client, root)
	err := inv.WithContext(ctx).Run()
	require.ErrorContains(t, err, "no scaletest workspaces exist")
}

// This test just validates that the CLI command accepts its known arguments.
func TestScaleTestWorkspaceTraffic_Template(t *testing.T) {
	t.Parallel()

	if testutil.RaceEnabled() {
		t.Skip("Skipping due to race detector")
	}

	ctx, cancelFunc := context.WithTimeout(context.Background(), testutil.WaitMedium)
	defer cancelFunc()

	log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	client := coderdtest.New(t, &coderdtest.Options{
		Logger: &log,
	})
	_ = coderdtest.CreateFirstUser(t, client)

	inv, root := clitest.New(t, "exp", "scaletest", "workspace-traffic",
		"--template", "doesnotexist",
	)
	clitest.SetupConfig(t, client, root)
	err := inv.WithContext(ctx).Run()
	require.ErrorContains(t, err, "could not find template \"doesnotexist\" in any organization")
}

// This test just validates that the CLI command accepts its known arguments.
func TestScaleTestWorkspaceTraffic_TargetWorkspaces(t *testing.T) {
	t.Parallel()

	if testutil.RaceEnabled() {
		t.Skip("Skipping due to race detector")
	}

	ctx, cancelFunc := context.WithTimeout(context.Background(), testutil.WaitMedium)
	defer cancelFunc()

	log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	client := coderdtest.New(t, &coderdtest.Options{
		Logger: &log,
	})
	_ = coderdtest.CreateFirstUser(t, client)

	inv, root := clitest.New(t, "exp", "scaletest", "workspace-traffic",
		"--target-workspaces", "0:0",
	)
	clitest.SetupConfig(t, client, root)
	err := inv.WithContext(ctx).Run()
	require.ErrorContains(t, err, "invalid target workspaces \"0:0\": start and end cannot be equal")
}

// This test just validates that the CLI command accepts its known arguments.
func TestScaleTestCleanup_Template(t *testing.T) {
	t.Parallel()

	if testutil.RaceEnabled() {
		t.Skip("Skipping due to race detector")
	}

	ctx, cancelFunc := context.WithTimeout(context.Background(), testutil.WaitMedium)
	defer cancelFunc()

	log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	client := coderdtest.New(t, &coderdtest.Options{
		Logger: &log,
	})
	_ = coderdtest.CreateFirstUser(t, client)

	inv, root := clitest.New(t, "exp", "scaletest", "cleanup",
		"--template", "doesnotexist",
	)
	clitest.SetupConfig(t, client, root)
	err := inv.WithContext(ctx).Run()
	require.ErrorContains(t, err, "could not find template \"doesnotexist\" in any organization")
}

// This test just validates that the CLI command accepts its known arguments.
func TestScaleTestDashboard(t *testing.T) {
	t.Parallel()
	if testutil.RaceEnabled() {
		t.Skip("Skipping due to race detector")
	}

	t.Run("MinWait", func(t *testing.T) {
		t.Parallel()
		ctx, cancelFunc := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancelFunc()

		log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
		client := coderdtest.New(t, &coderdtest.Options{
			Logger: &log,
		})
		_ = coderdtest.CreateFirstUser(t, client)

		inv, root := clitest.New(t, "exp", "scaletest", "dashboard",
			"--interval", "0s",
		)
		clitest.SetupConfig(t, client, root)
		err := inv.WithContext(ctx).Run()
		require.ErrorContains(t, err, "--interval must be greater than zero")
	})

	t.Run("MaxWait", func(t *testing.T) {
		t.Parallel()
		ctx, cancelFunc := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancelFunc()

		log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
		client := coderdtest.New(t, &coderdtest.Options{
			Logger: &log,
		})
		_ = coderdtest.CreateFirstUser(t, client)

		inv, root := clitest.New(t, "exp", "scaletest", "dashboard",
			"--interval", "1s",
			"--jitter", "1s",
		)
		clitest.SetupConfig(t, client, root)
		err := inv.WithContext(ctx).Run()
		require.ErrorContains(t, err, "--jitter must be less than --interval")
	})

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		ctx, cancelFunc := context.WithTimeout(context.Background(), testutil.WaitMedium)
		defer cancelFunc()

		log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
		client := coderdtest.New(t, &coderdtest.Options{
			Logger: &log,
		})
		_ = coderdtest.CreateFirstUser(t, client)

		inv, root := clitest.New(t, "exp", "scaletest", "dashboard",
			"--interval", "1s",
			"--jitter", "500ms",
			"--timeout", "5s",
			"--scaletest-prometheus-address", "127.0.0.1:0",
			"--scaletest-prometheus-wait", "0s",
			"--rand-seed", "1234567890",
		)
		clitest.SetupConfig(t, client, root)
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err, "")
	})

	t.Run("TargetUsers", func(t *testing.T) {
		t.Parallel()
		ctx, cancelFunc := context.WithTimeout(context.Background(), testutil.WaitMedium)
		defer cancelFunc()

		log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
		client := coderdtest.New(t, &coderdtest.Options{
			Logger: &log,
		})
		_ = coderdtest.CreateFirstUser(t, client)

		inv, root := clitest.New(t, "exp", "scaletest", "dashboard",
			"--target-users", "0:0",
		)
		clitest.SetupConfig(t, client, root)
		err := inv.WithContext(ctx).Run()
		require.ErrorContains(t, err, "invalid target users \"0:0\": start and end cannot be equal")
	})
}

// TestScaleTestNotifications_ValidatesArgs checks the degenerate configurations the
// command must reject before doing any work, each of which would otherwise produce
// a run that measures nothing and still exits 0.
func TestScaleTestNotifications_ValidatesArgs(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name          string
		args          []string
		errorContains string
	}{
		{
			name:          "ZeroTemplateAdminPercentage",
			args:          []string{"--user-count", "1", "--template-admin-percentage", "0"},
			errorContains: "--template-admin-percentage must be greater than 0",
		},
		{
			name:          "ZeroSetupConcurrency",
			args:          []string{"--user-count", "1", "--setup-concurrency", "0"},
			errorContains: "--setup-concurrency must be greater than 0",
		},
		{
			name:          "ZeroTimeout",
			args:          []string{"--user-count", "1", "--timeout", "0"},
			errorContains: "--timeout must be greater than 0",
		},
		{
			name:          "ZeroCleanupTimeout",
			args:          []string{"--user-count", "1", "--cleanup-timeout", "0"},
			errorContains: "--cleanup-timeout must be greater than 0",
		},
		{
			name:          "ZeroSetupTimeout",
			args:          []string{"--user-count", "1", "--setup-timeout", "0"},
			errorContains: "--setup-timeout must be greater than 0",
		},
		{
			// Skipping cleanup in reuse mode would leave real accounts promoted with
			// live tokens and no tooling to find them.
			name:          "NoCleanupWithReuse",
			args:          []string{"--user-count", "1", "--reuse-users", "--no-cleanup"},
			errorContains: "--no-cleanup cannot be used with --reuse-users",
		},
		{
			// The connect phase must leave room to trigger and observe, or the run
			// measures nothing.
			name:          "DialTimeoutNotLessThanTimeout",
			args:          []string{"--user-count", "1", "--timeout", "30m", "--dial-timeout", "30m"},
			errorContains: "--dial-timeout (30m0s) must be less than --timeout (30m0s)",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitShort)
			log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
			client := coderdtest.New(t, &coderdtest.Options{Logger: &log})
			_ = coderdtest.CreateFirstUser(t, client)

			args := append([]string{"exp", "scaletest", "notifications"}, tc.args...)
			args = append(args,
				"--scaletest-prometheus-address", "127.0.0.1:0",
				"--scaletest-prometheus-wait", "0s",
			)
			inv, root := clitest.New(t, args...)
			clitest.SetupConfig(t, client, root)
			err := inv.WithContext(ctx).Run()
			require.ErrorContains(t, err, tc.errorContains)
		})
	}
}

// TestScaleTestNotifications_ReuseFailsFast checks that --reuse-users refuses to run
// when too few eligible scaletest users exist, rather than promoting whatever it can
// find, and that the error tells the operator what eligible means.
func TestScaleTestNotifications_ReuseFailsFast(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitMedium)
	log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	client := coderdtest.New(t, &coderdtest.Options{Logger: &log})
	firstUser := coderdtest.CreateFirstUser(t, client)

	// A real (non-scaletest) user must not be considered eligible for promotion.
	_, realUser := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)

	inv, root := clitest.New(t, "exp", "scaletest", "notifications",
		"--user-count", "2",
		"--reuse-users",
		"--timeout", "10s",
		"--dial-timeout", "1s",
		"--scaletest-prometheus-address", "127.0.0.1:0",
		"--scaletest-prometheus-wait", "0s",
	)
	// The command requires an admin: it lists users, updates roles, mints tokens,
	// and creates templates.
	//nolint:gocritic // This scaletest command must run as an admin.
	clitest.SetupConfig(t, client, root)
	err := inv.WithContext(ctx).Run()
	require.ErrorContains(t, err, "requires 2 eligible users")

	// The real user must be untouched: no promotion, no minted token.
	got, err := client.User(ctx, realUser.ID.String())
	require.NoError(t, err)
	for _, role := range got.Roles {
		require.NotEqual(t, codersdk.RoleTemplateAdmin, role.Name, "a real user must not be promoted")
	}
	tokens, err := client.Tokens(ctx, realUser.ID.String(), codersdk.TokensFilter{})
	require.NoError(t, err)
	require.Empty(t, tokens)
}

// TestScaleTestNotifications_CleanupRunsAfterFailure checks the guarantee that the
// round 1 review found broken: users created during setup are cleaned up even when
// the run fails after setup, not only on the fully successful path.
func TestScaleTestNotifications_CleanupRunsAfterFailure(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	client := coderdtest.New(t, &coderdtest.Options{Logger: &log})
	firstUser := coderdtest.CreateFirstUser(t, client)

	// A dial timeout this short guarantees the run fails after setup created its
	// users, which is the window the cleanup must still cover.
	inv, root := clitest.New(t, "exp", "scaletest", "notifications",
		"--user-count", "2",
		"--template-admin-percentage", "50",
		"--timeout", "5s",
		"--dial-timeout", "1s",
		"--scaletest-prometheus-address", "127.0.0.1:0",
		"--scaletest-prometheus-wait", "0s",
	)
	// The command requires an admin: it lists users, updates roles, mints tokens,
	// and creates templates.
	//nolint:gocritic // This scaletest command must run as an admin.
	clitest.SetupConfig(t, client, root)
	err := inv.WithContext(ctx).Run()
	require.Error(t, err, "the run must fail")

	// Every user the run created must be gone, leaving only the owner.
	users, err := client.Users(ctx, codersdk.UsersRequest{})
	require.NoError(t, err)
	require.Len(t, users.Users, 1, "created users must be cleaned up after a post-setup failure")
	require.Equal(t, firstUser.UserID, users.Users[0].ID)

	// The trigger template must not be left behind to poison later runs.
	templates, err := client.TemplatesByOrganization(ctx, firstUser.OrganizationID)
	require.NoError(t, err)
	for _, tpl := range templates {
		require.NotContains(t, tpl.Name, "notifications-", "trigger template must be cleaned up")
	}
}

// TestScaleTestNotifications_ReuseCleanupRunsAfterFailure is the reuse-path
// counterpart to TestScaleTestNotifications_CleanupRunsAfterFailure. It pins the
// half of the cleanup wiring where the residue lands on accounts the run does not
// own: a reused user left holding template-admin and a live token is the failure
// mode this command was reviewed hardest for.
func TestScaleTestNotifications_ReuseCleanupRunsAfterFailure(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	client := coderdtest.New(t, &coderdtest.Options{Logger: &log})
	firstUser := coderdtest.CreateFirstUser(t, client)

	// Seed a pool the reuse path is allowed to select from. Only usernames carrying
	// the scaletest prefix are eligible.
	const poolSize = 2
	pool := make([]codersdk.User, 0, poolSize)
	for i := range poolSize {
		_, user := coderdtest.CreateAnotherUserMutators(t, client, firstUser.OrganizationID, nil,
			func(r *codersdk.CreateUserRequestWithOrgs) {
				r.Username = fmt.Sprintf("%s-reuse-%d", loadtestutil.ScaleTestPrefix, i)
			})
		pool = append(pool, user)
	}

	// A dial timeout this short guarantees the run fails after setup promoted the
	// users and minted their tokens, which is the window cleanup must cover.
	inv, root := clitest.New(t, "exp", "scaletest", "notifications",
		"--user-count", "2",
		"--template-admin-percentage", "50",
		"--reuse-users",
		"--timeout", "5s",
		"--dial-timeout", "1s",
		"--scaletest-prometheus-address", "127.0.0.1:0",
		"--scaletest-prometheus-wait", "0s",
	)
	// The command requires an admin: it lists users, updates roles, mints tokens,
	// and creates templates.
	//nolint:gocritic // This scaletest command must run as an admin.
	clitest.SetupConfig(t, client, root)
	err := inv.WithContext(ctx).Run()
	require.Error(t, err, "the run must fail")

	// Reused users are restored, never deleted: they belong to the deployment.
	users, err := client.Users(ctx, codersdk.UsersRequest{})
	require.NoError(t, err)
	require.Len(t, users.Users, poolSize+1, "reused users must survive cleanup")

	for _, seeded := range pool {
		got, err := client.User(ctx, seeded.ID.String())
		require.NoError(t, err)

		// The promotion is undone, so no reused account keeps elevated access.
		for _, role := range got.Roles {
			require.NotEqual(t, codersdk.RoleTemplateAdmin, role.Name,
				"reused user %q must not keep the template-admin role", got.Username)
		}

		// The minted token is revoked, so no reused account keeps a live credential.
		tokens, err := client.Tokens(ctx, got.ID.String(), codersdk.TokensFilter{})
		require.NoError(t, err)
		require.Empty(t, tokens, "reused user %q must not keep a live token", got.Username)
	}

	// The trigger template must not be left behind to poison later runs.
	templates, err := client.TemplatesByOrganization(ctx, firstUser.OrganizationID)
	require.NoError(t, err)
	for _, tpl := range templates {
		require.NotContains(t, tpl.Name, "notifications-", "trigger template must be cleaned up")
	}
}

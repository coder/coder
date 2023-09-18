package cli_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

func TestScaleTestCreateWorkspaces(t *testing.T) {
	t.Parallel()

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
	)
	clitest.SetupConfig(t, client, root)
	pty := ptytest.New(t)
	inv.Stdout = pty.Output()
	inv.Stderr = pty.Output()

	err := inv.WithContext(ctx).Run()
	require.ErrorContains(t, err, "could not find template \"doesnotexist\" in any organization")
}

// This test just validates that the CLI command accepts its known arguments.
// A more comprehensive test is performed in workspacetraffic/run_test.go
func TestScaleTestWorkspaceTraffic(t *testing.T) {
	t.Parallel()

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
	pty := ptytest.New(t)
	inv.Stdout = pty.Output()
	inv.Stderr = pty.Output()

	err := inv.WithContext(ctx).Run()
	require.ErrorContains(t, err, "no scaletest workspaces exist")
}

// This test just validates that the CLI command accepts its known arguments.
func TestScaleTestDashboard(t *testing.T) {
	t.Parallel()
	ctx, cancelFunc := context.WithTimeout(context.Background(), testutil.WaitMedium)
	defer cancelFunc()

	log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	client := coderdtest.New(t, &coderdtest.Options{
		Logger: &log,
	})
	_ = coderdtest.CreateFirstUser(t, client)

	inv, root := clitest.New(t, "exp", "scaletest", "dashboard",
		"--count", "1",
		"--min-wait", "100ms",
		"--max-wait", "1s",
		"--timeout", "5s",
		"--scaletest-prometheus-address", "127.0.0.1:0",
		"--scaletest-prometheus-wait", "0s",
	)
	clitest.SetupConfig(t, client, root)
	pty := ptytest.New(t)
	inv.Stdout = pty.Output()
	inv.Stderr = pty.Output()

	err := inv.WithContext(ctx).Run()
	require.NoError(t, err, "")
}

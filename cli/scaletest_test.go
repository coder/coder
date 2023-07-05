package cli_test

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/pty/ptytest"
	"github.com/coder/coder/testutil"
)

func TestScaleTestCreateWorkspaces(t *testing.T) {
	t.Parallel()

	// This test only validates that the CLI command accepts known arguments.
	// More thorough testing is done in scaletest/createworkspaces/run_test.go.
	ctx, cancelFunc := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancelFunc()

	client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
	_ = coderdtest.CreateFirstUser(t, client)

	// Write a parameters file.
	tDir := t.TempDir()
	outputFile := filepath.Join(tDir, "output.json")

	inv, root := clitest.New(t, "scaletest", "create-workspaces",
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

	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)

	inv, root := clitest.New(t, "scaletest", "workspace-traffic",
		"--timeout", "1s",
		"--bytes-per-tick", "1024",
		"--tick-interval", "100ms",
		"--scaletest-prometheus-address", "127.0.0.1:0",
		"--scaletest-prometheus-wait", "0s",
	)
	clitest.SetupConfig(t, client, root)
	var stdout, stderr bytes.Buffer
	inv.Stdout = &stdout
	inv.Stderr = &stderr
	err := inv.WithContext(ctx).Run()
	require.ErrorContains(t, err, "no scaletest workspaces exist")
}

package cli_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/pty/ptytest"
	"github.com/coder/coder/scaletest/harness"
	"github.com/coder/coder/testutil"
)

func TestScaleTest(t *testing.T) {
	// t.Skipf("This test is flakey. See https://github.com/coder/coder/issues/4942")
	t.Parallel()

	t.Run("WorkspaceBuild", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)

		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		// Write a parameters file.
		tDir := t.TempDir()
		paramsFile := filepath.Join(tDir, "params.yaml")
		outputFile := filepath.Join(tDir, "output.json")

		f, err := os.Create(paramsFile)
		require.NoError(t, err)
		defer f.Close()
		_, err = f.WriteString(`---
param1: foo
param2: true
param3: 1
`)
		require.NoError(t, err)
		err = f.Close()
		require.NoError(t, err)

		cmd, root := clitest.New(t, "scaletest", "create-workspaces",
			"--count", "2",
			"--template", template.Name,
			"--parameters-file", paramsFile,
			"--parameter", "param1=bar",
			"--parameter", "param4=baz",
			// This flag is important for tests because agents will never be
			// started.
			"--no-wait-for-agents",
			// Run and connect flags cannot be tested because they require an
			// agent.
			"--concurrency", "2",
			"--timeout", "30s",
			"--job-timeout", "15s",
			"--cleanup-concurrency", "1",
			"--cleanup-timeout", "30s",
			"--cleanup-job-timeout", "15s",
			"--output text",
			"--output json:"+outputFile,
		)
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t)
		cmd.SetOut(pty.Output())
		cmd.SetErr(pty.Output())

		ctx, cancelFunc := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancelFunc()

		done := make(chan any)
		go func() {
			err := cmd.ExecuteContext(ctx)
			assert.NoError(t, err)
			close(done)
		}()
		pty.ExpectMatch("Test results:")
		pty.ExpectMatch("Pass:  2")
		select {
		case <-done:
		case <-ctx.Done():
		}
		cancelFunc()
		<-done

		// Verify the output file.
		f, err = os.Open(outputFile)
		require.NoError(t, err)
		defer f.Close()
		var res harness.Results
		err = json.NewDecoder(f).Decode(&res)
		require.NoError(t, err)

		require.EqualValues(t, 2, res.TotalRuns)
		require.EqualValues(t, 2, res.TotalPass)
	})
}

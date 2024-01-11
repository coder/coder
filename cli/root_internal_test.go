package cli

import (
	"bytes"
	"fmt"
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/pretty"
)

func Test_formatExamples(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		examples    []example
		wantMatches []string
	}{
		{
			name:        "No examples",
			examples:    nil,
			wantMatches: nil,
		},
		{
			name: "Output examples",
			examples: []example{
				{
					Description: "Hello world.",
					Command:     "echo hello",
				},
				{
					Description: "Bye bye.",
					Command:     "echo bye",
				},
			},
			wantMatches: []string{
				"Hello world", "echo hello",
				"Bye bye", "echo bye",
			},
		},
		{
			name: "No description outputs commands",
			examples: []example{
				{
					Command: "echo hello",
				},
			},
			wantMatches: []string{
				"echo hello",
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := formatExamples(tt.examples...)
			if len(tt.wantMatches) == 0 {
				require.Empty(t, got)
			} else {
				for _, want := range tt.wantMatches {
					require.Contains(t, got, want)
				}
			}
		})
	}
}

func TestMain(m *testing.M) {
	if runtime.GOOS == "windows" {
		// Don't run goleak on windows tests, they're super flaky right now.
		// See: https://github.com/coder/coder/issues/8954
		os.Exit(m.Run())
	}
	goleak.VerifyTestMain(m,
		// The lumberjack library is used by by agent and seems to leave
		// goroutines after Close(), fails TestGitSSH tests.
		// https://github.com/natefinch/lumberjack/pull/100
		goleak.IgnoreTopFunction("gopkg.in/natefinch/lumberjack%2ev2.(*Logger).millRun"),
		goleak.IgnoreTopFunction("gopkg.in/natefinch/lumberjack%2ev2.(*Logger).mill.func1"),
		// The pq library appears to leave around a goroutine after Close().
		goleak.IgnoreTopFunction("github.com/lib/pq.NewDialListener"),
	)
}

func Test_checkVersions(t *testing.T) {
	t.Parallel()

	t.Run("CustomInstallMessage", func(t *testing.T) {
		t.Parallel()

		var (
			expectedUpgradeMessage = "My custom upgrade message"
			dv                     = coderdtest.DeploymentValues(t)
		)
		dv.CLIUpgradeMessage = clibase.String(expectedUpgradeMessage)

		ownerClient := coderdtest.New(t, &coderdtest.Options{
			DeploymentValues: dv,
		})
		owner := coderdtest.CreateFirstUser(t, ownerClient)

		// Create an unprivileged user to ensure the message can be printed
		// to any Coder user.
		memberClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)

		r := &RootCmd{}

		cmd, err := r.Command(nil)
		require.NoError(t, err)

		var buf bytes.Buffer
		inv := cmd.Invoke()
		inv.Stderr = &buf

		err = r.checkVersions(inv, memberClient, true)
		require.NoError(t, err)

		expectedOutput := fmt.Sprintln(pretty.Sprint(cliui.DefaultStyles.Warn, expectedUpgradeMessage))
		require.Equal(t, expectedOutput, buf.String())
	})
}

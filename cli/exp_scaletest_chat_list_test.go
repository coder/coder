//go:build !slim

package cli_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestScaleTestChatList(t *testing.T) {
	t.Parallel()

	for _, batchChatAuthz := range []bool{false, true} {
		name := "PerChatAuthz"
		if batchChatAuthz {
			name = "BatchChatAuthz"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			values := coderdtest.DeploymentValues(t, func(dv *codersdk.DeploymentValues) {
				require.NoError(t, dv.AI.BridgeConfig.Enabled.Set("true"))
				if batchChatAuthz {
					dv.Experiments = []string{string(codersdk.ExperimentChatBatchAuthz)}
				}
			})
			client := coderdtest.New(t, &coderdtest.Options{DeploymentValues: values})
			coderdtest.CreateFirstUser(t, client)

			run := func(chats string) string {
				inv, root := clitest.New(t,
					"exp", "scaletest", "chat-list",
					"--chats", chats,
					"--list-requests", "3",
					"--page-size", "2",
					"--concurrency", "2",
					"--timeout", "30s",
					"--job-timeout", "30s",
					"--scaletest-prometheus-address", "127.0.0.1:0",
					"--scaletest-prometheus-wait", "0s",
					"--provider-propagation-wait", "0s",
					// Chats are created idle, so the mock is never contacted.
					"--llm-mock-url", "http://127.0.0.1:1/v1",
				)
				//nolint:gocritic // The scaletest chat-list command requires an admin client.
				clitest.SetupConfig(t, client, root)
				var stderr bytes.Buffer
				inv.Stdout = io.Discard
				inv.Stderr = &stderr
				require.NoError(t, inv.WithContext(ctx).Run(), stderr.String())
				return stderr.String()
			}

			out := run("3")
			require.Contains(t, out, `User "scaletest-chat-list" has 3 scaletest chats (0 reused, 3 created).`)
			require.Contains(t, out, "Scale test passed: 2/2 runs succeeded")

			out = run("4")
			require.Contains(t, out, `User "scaletest-chat-list" has 4 scaletest chats (3 reused, 1 created).`)

			user, err := client.User(ctx, "scaletest-chat-list")
			require.NoError(t, err)
			keys, err := client.Tokens(ctx, user.ID.String(), codersdk.TokensFilter{})
			require.NoError(t, err)
			require.Empty(t, keys, "scaletest token should be deleted")
		})
	}
}

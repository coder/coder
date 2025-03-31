package cli_test

import (
	"context"
	"encoding/json"
	"runtime"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

func TestExpMcp(t *testing.T) {
	t.Parallel()

	// Reading to / writing from the PTY is flaky on non-linux systems.
	if runtime.GOOS != "linux" {
		t.Skip("skipping on non-linux")
	}

	t.Run("AllowedTools", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		cancelCtx, cancel := context.WithCancel(ctx)
		t.Cleanup(cancel)

		// Given: a running coder deployment
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		// Given: we run the exp mcp command with allowed tools set
		inv, root := clitest.New(t, "exp", "mcp", "server", "--allowed-tools=coder_whoami,coder_list_templates")
		inv = inv.WithContext(cancelCtx)

		pty := ptytest.New(t)
		inv.Stdin = pty.Input()
		inv.Stdout = pty.Output()
		clitest.SetupConfig(t, client, root)

		cmdDone := make(chan struct{})
		go func() {
			defer close(cmdDone)
			err := inv.Run()
			assert.NoError(t, err)
		}()

		// When: we send a tools/list request
		toolsPayload := `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`
		pty.WriteLine(toolsPayload)
		_ = pty.ReadLine(ctx) // ignore echoed output
		output := pty.ReadLine(ctx)

		cancel()
		<-cmdDone

		// Then: we should only see the allowed tools in the response
		var toolsResponse struct {
			Result struct {
				Tools []struct {
					Name string `json:"name"`
				} `json:"tools"`
			} `json:"result"`
		}
		err := json.Unmarshal([]byte(output), &toolsResponse)
		require.NoError(t, err)
		require.Len(t, toolsResponse.Result.Tools, 2, "should have exactly 2 tools")
		foundTools := make([]string, 0, 2)
		for _, tool := range toolsResponse.Result.Tools {
			foundTools = append(foundTools, tool.Name)
		}
		slices.Sort(foundTools)
		require.Equal(t, []string{"coder_list_templates", "coder_whoami"}, foundTools)
	})

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		cancelCtx, cancel := context.WithCancel(ctx)
		t.Cleanup(cancel)

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		inv, root := clitest.New(t, "exp", "mcp", "server")
		inv = inv.WithContext(cancelCtx)

		pty := ptytest.New(t)
		inv.Stdin = pty.Input()
		inv.Stdout = pty.Output()
		clitest.SetupConfig(t, client, root)

		cmdDone := make(chan struct{})
		go func() {
			defer close(cmdDone)
			err := inv.Run()
			assert.NoError(t, err)
		}()

		payload := `{"jsonrpc":"2.0","id":1,"method":"initialize"}`
		pty.WriteLine(payload)
		_ = pty.ReadLine(ctx) // ignore echoed output
		output := pty.ReadLine(ctx)
		cancel()
		<-cmdDone

		// Ensure the initialize output is valid JSON
		t.Logf("/initialize output: %s", output)
		var initializeResponse map[string]interface{}
		err := json.Unmarshal([]byte(output), &initializeResponse)
		require.NoError(t, err)
		require.Equal(t, "2.0", initializeResponse["jsonrpc"])
		require.Equal(t, 1.0, initializeResponse["id"])
		require.NotNil(t, initializeResponse["result"])
	})

	t.Run("NoCredentials", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		cancelCtx, cancel := context.WithCancel(ctx)
		t.Cleanup(cancel)

		client := coderdtest.New(t, nil)
		inv, root := clitest.New(t, "exp", "mcp", "server")
		inv = inv.WithContext(cancelCtx)

		pty := ptytest.New(t)
		inv.Stdin = pty.Input()
		inv.Stdout = pty.Output()
		clitest.SetupConfig(t, client, root)

		err := inv.Run()
		assert.ErrorContains(t, err, "your session has expired")
	})
}

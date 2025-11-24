package coderd_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
)

func TestInitScript(t *testing.T) {
	t.Parallel()

	t.Run("OK Windows amd64", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		script, err := client.InitScript(context.Background(), "windows", "amd64")
		require.NoError(t, err)
		require.NotEmpty(t, script)
		require.Contains(t, script, "$env:CODER_AGENT_AUTH = \"token\"")
		require.Contains(t, script, "/bin/coder-windows-amd64.exe")
	})

	t.Run("OK Windows arm64", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		script, err := client.InitScript(context.Background(), "windows", "arm64")
		require.NoError(t, err)
		require.NotEmpty(t, script)
		require.Contains(t, script, "$env:CODER_AGENT_AUTH = \"token\"")
		require.Contains(t, script, "/bin/coder-windows-arm64.exe")
	})

	t.Run("OK Linux amd64", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		script, err := client.InitScript(context.Background(), "linux", "amd64")
		require.NoError(t, err)
		require.NotEmpty(t, script)
		require.Contains(t, script, "export CODER_AGENT_AUTH=\"token\"")
		require.Contains(t, script, "/bin/coder-linux-amd64")
	})

	t.Run("OK Linux arm64", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		script, err := client.InitScript(context.Background(), "linux", "arm64")
		require.NoError(t, err)
		require.NotEmpty(t, script)
		require.Contains(t, script, "export CODER_AGENT_AUTH=\"token\"")
		require.Contains(t, script, "/bin/coder-linux-arm64")
	})

	t.Run("BadRequest", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_, err := client.InitScript(context.Background(), "darwin", "armv7")
		require.Error(t, err)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
		require.Equal(t, "Unknown os/arch: darwin/armv7", apiErr.Message)
	})
}

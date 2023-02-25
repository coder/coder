package bigcli_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/bigcli"
)

func TestOptionSet_ParseFlags(t *testing.T) {
	t.Parallel()

	t.Run("SimpleString", func(t *testing.T) {
		t.Parallel()

		var workspaceName bigcli.String

		os := bigcli.OptionSet{
			bigcli.Option{
				Name:          "Workspace Name",
				Value:         &workspaceName,
				FlagShorthand: "n",
			},
		}

		var err error
		err = os.ParseFlags("--workspace-name", "foo")
		require.NoError(t, err)
		require.EqualValues(t, "foo", workspaceName)

		err = os.ParseFlags("-n", "f")
		require.NoError(t, err)
		require.EqualValues(t, "f", workspaceName)
	})

	t.Run("ExtraFlags", func(t *testing.T) {
		t.Parallel()

		var workspaceName bigcli.String

		os := bigcli.OptionSet{
			bigcli.Option{
				Name:  "Workspace Name",
				Value: &workspaceName,
			},
		}

		err := os.ParseFlags("--some-unknown", "foo")
		require.Error(t, err)
	})
}

func TestOptionSet_ParseEnv(t *testing.T) {
	t.Parallel()

	t.Run("SimpleString", func(t *testing.T) {
		t.Parallel()

		var workspaceName bigcli.String

		os := bigcli.OptionSet{
			bigcli.Option{
				Name:  "Workspace Name",
				Value: &workspaceName,
			},
		}

		err := os.ParseEnv("CODER_", []string{"CODER_WORKSPACE_NAME=foo"})
		require.NoError(t, err)
		require.EqualValues(t, "foo", workspaceName)
	})
}

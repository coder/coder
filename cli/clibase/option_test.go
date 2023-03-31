package clibase_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clibase"
)

func TestOptionSet_ParseFlags(t *testing.T) {
	t.Parallel()

	t.Run("SimpleString", func(t *testing.T) {
		t.Parallel()

		var workspaceName clibase.String

		os := clibase.OptionSet{
			clibase.Option{
				Name:          "Workspace Name",
				Value:         &workspaceName,
				Flag:          "workspace-name",
				FlagShorthand: "n",
			},
		}

		var err error
		err = os.FlagSet().Parse([]string{"--workspace-name", "foo"})
		require.NoError(t, err)
		require.EqualValues(t, "foo", workspaceName)

		err = os.FlagSet().Parse([]string{"-n", "f"})
		require.NoError(t, err)
		require.EqualValues(t, "f", workspaceName)
	})

	t.Run("StringArray", func(t *testing.T) {
		t.Parallel()

		var names clibase.StringArray

		os := clibase.OptionSet{
			clibase.Option{
				Name:          "name",
				Value:         &names,
				Flag:          "name",
				FlagShorthand: "n",
			},
		}

		err := os.SetDefaults()
		require.NoError(t, err)

		err = os.FlagSet().Parse([]string{"--name", "foo", "--name", "bar"})
		require.NoError(t, err)
		require.EqualValues(t, []string{"foo", "bar"}, names)
	})

	t.Run("ExtraFlags", func(t *testing.T) {
		t.Parallel()

		var workspaceName clibase.String

		os := clibase.OptionSet{
			clibase.Option{
				Name:  "Workspace Name",
				Value: &workspaceName,
			},
		}

		err := os.FlagSet().Parse([]string{"--some-unknown", "foo"})
		require.Error(t, err)
	})
}

func TestOptionSet_ParseEnv(t *testing.T) {
	t.Parallel()

	t.Run("SimpleString", func(t *testing.T) {
		t.Parallel()

		var workspaceName clibase.String

		os := clibase.OptionSet{
			clibase.Option{
				Name:  "Workspace Name",
				Value: &workspaceName,
				Env:   "WORKSPACE_NAME",
			},
		}

		err := os.ParseEnv([]clibase.EnvVar{
			{Name: "WORKSPACE_NAME", Value: "foo"},
		})
		require.NoError(t, err)
		require.EqualValues(t, "foo", workspaceName)
	})

	t.Run("EmptyValue", func(t *testing.T) {
		t.Parallel()

		var workspaceName clibase.String

		os := clibase.OptionSet{
			clibase.Option{
				Name:    "Workspace Name",
				Value:   &workspaceName,
				Default: "defname",
				Env:     "WORKSPACE_NAME",
			},
		}

		err := os.SetDefaults()
		require.NoError(t, err)

		err = os.ParseEnv(clibase.ParseEnviron([]string{"CODER_WORKSPACE_NAME="}, "CODER_"))
		require.NoError(t, err)
		require.EqualValues(t, "defname", workspaceName)
	})

	t.Run("StringSlice", func(t *testing.T) {
		t.Parallel()

		var actual clibase.StringArray
		expected := []string{"foo", "bar", "baz"}

		os := clibase.OptionSet{
			clibase.Option{
				Name:  "name",
				Value: &actual,
				Env:   "NAMES",
			},
		}

		err := os.SetDefaults()
		require.NoError(t, err)

		err = os.ParseEnv([]clibase.EnvVar{
			{Name: "NAMES", Value: "foo,bar,baz"},
		})
		require.NoError(t, err)
		require.EqualValues(t, expected, actual)
	})

	t.Run("StructMapStringString", func(t *testing.T) {
		t.Parallel()

		var actual clibase.Struct[map[string]string]
		expected := map[string]string{"foo": "bar", "baz": "zap"}

		os := clibase.OptionSet{
			clibase.Option{
				Name:  "labels",
				Value: &actual,
				Env:   "LABELS",
			},
		}

		err := os.SetDefaults()
		require.NoError(t, err)

		err = os.ParseEnv([]clibase.EnvVar{
			{Name: "LABELS", Value: `{"foo":"bar","baz":"zap"}`},
		})
		require.NoError(t, err)
		require.EqualValues(t, expected, actual.Value)
	})
}

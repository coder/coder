package clibase_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/coder/coder/cli/clibase"
)

func TestOption_ToYAML(t *testing.T) {
	t.Parallel()

	t.Run("RequireKey", func(t *testing.T) {
		t.Parallel()
		var workspaceName clibase.String
		os := clibase.OptionSet{
			clibase.Option{
				Name:    "Workspace Name",
				Value:   &workspaceName,
				Default: "billie",
			},
		}

		node, err := os.ToYAML()
		require.NoError(t, err)
		require.Len(t, node.Content, 0)
	})

	t.Run("SimpleString", func(t *testing.T) {
		t.Parallel()

		var workspaceName clibase.String

		os := clibase.OptionSet{
			clibase.Option{
				Name:        "Workspace Name",
				Value:       &workspaceName,
				Default:     "billie",
				Description: "The workspace's name.",
				Group:       &clibase.Group{Name: "Names"},
				YAML:        "workspaceName",
			},
		}

		err := os.SetDefaults()
		require.NoError(t, err)

		n, err := os.ToYAML()
		require.NoError(t, err)
		// Visually inspect for now.
		byt, err := yaml.Marshal(n)
		require.NoError(t, err)
		t.Logf("Raw YAML:\n%s", string(byt))
	})
}

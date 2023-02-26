package bigcli_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/coder/coder/cli/bigcli"
)

func TestOption_ToYAML(t *testing.T) {
	t.Parallel()

	var workspaceName bigcli.String

	os := bigcli.OptionSet{
		bigcli.Option{
			Name:  "Workspace Name",
			Value: &workspaceName,
			Group: []string{"Names"},
		},
	}

	n, err := os.ToYAML()
	require.NoError(t, err)
	// Visually inspect for now.
	byt, err := yaml.Marshal(n)
	require.NoError(t, err)
	t.Logf("Raw YAML:\n%s", string(byt))
}

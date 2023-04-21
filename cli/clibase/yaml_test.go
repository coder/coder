package clibase_test

import (
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slices"
	"gopkg.in/yaml.v3"

	"github.com/coder/coder/cli/clibase"
)

func TestOptionSet_YAML(t *testing.T) {
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

		node, err := os.MarshalYAML()
		require.NoError(t, err)
		require.Len(t, node.(*yaml.Node).Content, 0)
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
				Group:       &clibase.Group{YAML: "names"},
				YAML:        "workspaceName",
			},
		}

		err := os.SetDefaults()
		require.NoError(t, err)

		n, err := os.MarshalYAML()
		require.NoError(t, err)
		// Visually inspect for now.
		byt, err := yaml.Marshal(n)
		require.NoError(t, err)
		t.Logf("Raw YAML:\n%s", string(byt))
	})
}

func TestOptionSet_YAMLUnknownOptions(t *testing.T) {
	t.Parallel()
	os := clibase.OptionSet{
		{
			Name:        "Workspace Name",
			Default:     "billie",
			Description: "The workspace's name.",
			YAML:        "workspaceName",
			Value:       new(clibase.String),
		},
	}

	const yamlDoc = `something: else`
	err := yaml.Unmarshal([]byte(yamlDoc), &os)
	require.Error(t, err)
	require.Empty(t, os[0].Value.String())

	os[0].YAML = "something"

	err = yaml.Unmarshal([]byte(yamlDoc), &os)
	require.NoError(t, err)

	require.Equal(t, "else", os[0].Value.String())
}

// TestOptionSet_YAMLIsomorphism tests that the YAML representations of an
// OptionSet converts to the same OptionSet when read back in.
func TestOptionSet_YAMLIsomorphism(t *testing.T) {
	t.Parallel()
	// This is used to form a generic.
	//nolint:unused
	type kid struct {
		Name string `yaml:"name"`
		Age  int    `yaml:"age"`
	}

	for _, tc := range []struct {
		name      string
		os        clibase.OptionSet
		zeroValue func() pflag.Value
	}{
		{
			name: "SimpleString",
			os: clibase.OptionSet{
				{
					Name:        "Workspace Name",
					Default:     "billie",
					Description: "The workspace's name.",
					Group:       &clibase.Group{YAML: "names"},
					YAML:        "workspaceName",
				},
			},
			zeroValue: func() pflag.Value {
				return clibase.StringOf(new(string))
			},
		},
		{
			name: "Array",
			os: clibase.OptionSet{
				{
					YAML:    "names",
					Default: "jill,jack,joan",
				},
			},
			zeroValue: func() pflag.Value {
				return clibase.StringArrayOf(&[]string{})
			},
		},
		{
			name: "ComplexObject",
			os: clibase.OptionSet{
				{
					YAML: "kids",
					Default: `- name: jill
  age: 12
- name: jack
  age: 13`,
				},
			},
			zeroValue: func() pflag.Value {
				return &clibase.Struct[[]kid]{}
			},
		},
		{
			name: "DeepGroup",
			os: clibase.OptionSet{
				{
					YAML:    "names",
					Default: "jill,jack,joan",
					Group:   &clibase.Group{YAML: "kids", Parent: &clibase.Group{YAML: "family"}},
				},
			},
			zeroValue: func() pflag.Value {
				return clibase.StringArrayOf(&[]string{})
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Set initial values.
			for i := range tc.os {
				tc.os[i].Value = tc.zeroValue()
			}
			err := tc.os.SetDefaults()
			require.NoError(t, err)

			y, err := tc.os.MarshalYAML()
			require.NoError(t, err)

			toByt, err := yaml.Marshal(y)
			require.NoError(t, err)

			t.Logf("Raw YAML:\n%s", string(toByt))

			var y2 yaml.Node
			err = yaml.Unmarshal(toByt, &y2)
			require.NoError(t, err)

			os2 := slices.Clone(tc.os)
			for i := range os2 {
				os2[i].Value = tc.zeroValue()
				os2[i].ValueSource = clibase.ValueSourceNone
			}

			// os2 values should be zeroed whereas tc.os should be
			// set to defaults.
			// This check makes sure we aren't mixing pointers.
			require.NotEqual(t, tc.os, os2)
			err = os2.UnmarshalYAML(&y2)
			require.NoError(t, err)

			want := tc.os
			for i := range want {
				want[i].ValueSource = clibase.ValueSourceYAML
			}

			require.Equal(t, tc.os, os2)
		})
	}
}

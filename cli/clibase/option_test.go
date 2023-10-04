package clibase_test

import (
	"bytes"
	"encoding/json"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
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

	t.Run("RegexValid", func(t *testing.T) {
		t.Parallel()

		var regexpString clibase.Regexp

		os := clibase.OptionSet{
			clibase.Option{
				Name:  "RegexpString",
				Value: &regexpString,
				Flag:  "regexp-string",
			},
		}

		err := os.FlagSet().Parse([]string{"--regexp-string", "$test^"})
		require.NoError(t, err)
	})

	t.Run("RegexInvalid", func(t *testing.T) {
		t.Parallel()

		var regexpString clibase.Regexp

		os := clibase.OptionSet{
			clibase.Option{
				Name:  "RegexpString",
				Value: &regexpString,
				Flag:  "regexp-string",
			},
		}

		err := os.FlagSet().Parse([]string{"--regexp-string", "(("})
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

	t.Run("Homebrew", func(t *testing.T) {
		t.Parallel()

		var agentToken clibase.String

		os := clibase.OptionSet{
			clibase.Option{
				Name:  "Agent Token",
				Value: &agentToken,
				Env:   "AGENT_TOKEN",
			},
		}

		err := os.ParseEnv([]clibase.EnvVar{
			{Name: "HOMEBREW_AGENT_TOKEN", Value: "foo"},
		})
		require.NoError(t, err)
		require.EqualValues(t, "foo", agentToken)
	})
}

func TestOptionSet_JsonMarshal(t *testing.T) {
	t.Parallel()

	// This unit test ensures if the source optionset is missing the option
	// and cannot determine the type, it will not panic. The unmarshal will
	// succeed with a best effort.
	t.Run("MissingSrcOption", func(t *testing.T) {
		t.Parallel()

		var str clibase.String = "something"
		var arr clibase.StringArray = []string{"foo", "bar"}
		opts := clibase.OptionSet{
			clibase.Option{
				Name:  "StringOpt",
				Value: &str,
			},
			clibase.Option{
				Name:  "ArrayOpt",
				Value: &arr,
			},
		}
		data, err := json.Marshal(opts)
		require.NoError(t, err, "marshal option set")

		tgt := clibase.OptionSet{}
		err = json.Unmarshal(data, &tgt)
		require.NoError(t, err, "unmarshal option set")
		for i := range opts {
			compareOptionsExceptValues(t, opts[i], tgt[i])
			require.Empty(t, tgt[i].Value.String(), "unknown value types are empty")
		}
	})

	t.Run("RegexCase", func(t *testing.T) {
		t.Parallel()

		val := clibase.Regexp(*regexp.MustCompile(".*"))
		opts := clibase.OptionSet{
			clibase.Option{
				Name:    "Regex",
				Value:   &val,
				Default: ".*",
			},
		}
		data, err := json.Marshal(opts)
		require.NoError(t, err, "marshal option set")

		var foundVal clibase.Regexp
		newOpts := clibase.OptionSet{
			clibase.Option{
				Name:  "Regex",
				Value: &foundVal,
			},
		}
		err = json.Unmarshal(data, &newOpts)
		require.NoError(t, err, "unmarshal option set")

		require.EqualValues(t, opts[0].Value.String(), newOpts[0].Value.String())
	})

	t.Run("AllValues", func(t *testing.T) {
		t.Parallel()

		vals := coderdtest.DeploymentValues(t)
		opts := vals.Options()
		sources := []clibase.ValueSource{
			clibase.ValueSourceNone,
			clibase.ValueSourceFlag,
			clibase.ValueSourceEnv,
			clibase.ValueSourceYAML,
			clibase.ValueSourceDefault,
		}
		for i := range opts {
			opts[i].ValueSource = sources[i%len(sources)]
		}

		data, err := json.Marshal(opts)
		require.NoError(t, err, "marshal option set")

		newOpts := (&codersdk.DeploymentValues{}).Options()
		err = json.Unmarshal(data, &newOpts)
		require.NoError(t, err, "unmarshal option set")

		for i := range opts {
			exp := opts[i]
			found := newOpts[i]

			compareOptionsExceptValues(t, exp, found)
			compareValues(t, exp, found)
		}

		thirdOpts := (&codersdk.DeploymentValues{}).Options()
		data, err = json.Marshal(newOpts)
		require.NoError(t, err, "marshal option set")

		err = json.Unmarshal(data, &thirdOpts)
		require.NoError(t, err, "unmarshal option set")
		// Compare to the original opts again
		for i := range opts {
			exp := opts[i]
			found := thirdOpts[i]

			compareOptionsExceptValues(t, exp, found)
			compareValues(t, exp, found)
		}
	})
}

func compareOptionsExceptValues(t *testing.T, exp, found clibase.Option) {
	t.Helper()

	require.Equalf(t, exp.Name, found.Name, "option name %q", exp.Name)
	require.Equalf(t, exp.Description, found.Description, "option description %q", exp.Name)
	require.Equalf(t, exp.Required, found.Required, "option required %q", exp.Name)
	require.Equalf(t, exp.Flag, found.Flag, "option flag %q", exp.Name)
	require.Equalf(t, exp.FlagShorthand, found.FlagShorthand, "option flag shorthand %q", exp.Name)
	require.Equalf(t, exp.Env, found.Env, "option env %q", exp.Name)
	require.Equalf(t, exp.YAML, found.YAML, "option yaml %q", exp.Name)
	require.Equalf(t, exp.Default, found.Default, "option default %q", exp.Name)
	require.Equalf(t, exp.ValueSource, found.ValueSource, "option value source %q", exp.Name)
	require.Equalf(t, exp.Hidden, found.Hidden, "option hidden %q", exp.Name)
	require.Equalf(t, exp.Annotations, found.Annotations, "option annotations %q", exp.Name)
	require.Equalf(t, exp.Group, found.Group, "option group %q", exp.Name)
	// UseInstead is the same comparison problem, just check the length
	require.Equalf(t, len(exp.UseInstead), len(found.UseInstead), "option use instead %q", exp.Name)
}

func compareValues(t *testing.T, exp, found clibase.Option) {
	t.Helper()

	if (exp.Value == nil || found.Value == nil) || (exp.Value.String() != found.Value.String() && found.Value.String() == "") {
		// If the string values are different, this can be a "nil" issue.
		// So only run this case if the found string is the empty string.
		// We use MarshalYAML for struct strings, and it will return an
		// empty string '""' for nil slices/maps/etc.
		// So use json to compare.

		expJSON, err := json.Marshal(exp.Value)
		require.NoError(t, err, "marshal")
		foundJSON, err := json.Marshal(found.Value)
		require.NoError(t, err, "marshal")

		expJSON = normalizeJSON(expJSON)
		foundJSON = normalizeJSON(foundJSON)
		assert.Equalf(t, string(expJSON), string(foundJSON), "option value %q", exp.Name)
	} else {
		assert.Equal(t,
			exp.Value.String(),
			found.Value.String(),
			"option value %q", exp.Name)
	}
}

// normalizeJSON handles the fact that an empty map/slice is not the same
// as a nil empty/slice. For our purposes, they are the same.
func normalizeJSON(data []byte) []byte {
	if bytes.Equal(data, []byte("[]")) || bytes.Equal(data, []byte("{}")) {
		return []byte("null")
	}
	return data
}

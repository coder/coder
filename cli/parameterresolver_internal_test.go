package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func TestParameterResolverSkipsCopiedSensitiveParameters(t *testing.T) {
	t.Parallel()

	resolver := new(ParameterResolver).WithSourceWorkspaceParameters([]codersdk.WorkspaceBuildParameter{
		{Name: "sensitive", Value: "secret"},
		{Name: "redacted", Value: codersdk.RedactedValue},
		{Name: "ordinary", Value: "visible"},
	})
	definitions := []codersdk.TemplateVersionParameter{
		{Name: "sensitive", Sensitive: true},
		{Name: "redacted"},
		{Name: "ordinary"},
	}

	resolved := resolver.resolveWithSourceBuildParametersInParameters(nil, definitions)
	require.Equal(t, []codersdk.WorkspaceBuildParameter{{Name: "ordinary", Value: "visible"}}, resolved)
}

func TestParameterResolverRedactsSensitiveDefaultOutput(t *testing.T) {
	t.Parallel()

	const secret = "default-secret"
	var stdout bytes.Buffer
	inv := (&serpent.Invocation{Stdout: &stdout}).WithContext(t.Context())
	resolver := new(ParameterResolver).WithUseParameterDefaults(true)

	resolved, err := resolver.Resolve(inv, WorkspaceCreate, []codersdk.TemplateVersionParameter{
		{Name: "token", DefaultValue: secret, Sensitive: true},
	})
	require.NoError(t, err)
	require.Equal(t, []codersdk.WorkspaceBuildParameter{{Name: "token", Value: secret}}, resolved)
	assert.NotContains(t, stdout.String(), secret)
	assert.Contains(t, stdout.String(), codersdk.RedactedValue)
}

func TestIsValidTemplateParameterOption(t *testing.T) {
	t.Parallel()

	options := []codersdk.TemplateVersionParameterOption{
		{Name: "Vim", Value: "vim"},
		{Name: "Emacs", Value: "emacs"},
		{Name: "VS Code", Value: "vscode"},
	}

	t.Run("SingleSelectValid", func(t *testing.T) {
		t.Parallel()
		bp := codersdk.WorkspaceBuildParameter{Name: "editor", Value: "vim"}
		tvp := codersdk.TemplateVersionParameter{
			Name:    "editor",
			Type:    "string",
			Options: options,
		}
		assert.True(t, isValidTemplateParameterOption(bp, tvp))
	})

	t.Run("SingleSelectInvalid", func(t *testing.T) {
		t.Parallel()
		bp := codersdk.WorkspaceBuildParameter{Name: "editor", Value: "notepad"}
		tvp := codersdk.TemplateVersionParameter{
			Name:    "editor",
			Type:    "string",
			Options: options,
		}
		assert.False(t, isValidTemplateParameterOption(bp, tvp))
	})

	t.Run("MultiSelectAllValid", func(t *testing.T) {
		t.Parallel()
		bp := codersdk.WorkspaceBuildParameter{Name: "editors", Value: `["vim","emacs"]`}
		tvp := codersdk.TemplateVersionParameter{
			Name:    "editors",
			Type:    "list(string)",
			Options: options,
		}
		assert.True(t, isValidTemplateParameterOption(bp, tvp))
	})

	t.Run("MultiSelectOneInvalid", func(t *testing.T) {
		t.Parallel()
		bp := codersdk.WorkspaceBuildParameter{Name: "editors", Value: `["vim","notepad"]`}
		tvp := codersdk.TemplateVersionParameter{
			Name:    "editors",
			Type:    "list(string)",
			Options: options,
		}
		assert.False(t, isValidTemplateParameterOption(bp, tvp))
	})

	t.Run("MultiSelectEmptyArray", func(t *testing.T) {
		t.Parallel()
		bp := codersdk.WorkspaceBuildParameter{Name: "editors", Value: `[]`}
		tvp := codersdk.TemplateVersionParameter{
			Name:    "editors",
			Type:    "list(string)",
			Options: options,
		}
		assert.True(t, isValidTemplateParameterOption(bp, tvp))
	})

	t.Run("MultiSelectInvalidJSON", func(t *testing.T) {
		t.Parallel()
		bp := codersdk.WorkspaceBuildParameter{Name: "editors", Value: `not-json`}
		tvp := codersdk.TemplateVersionParameter{
			Name:    "editors",
			Type:    "list(string)",
			Options: options,
		}
		assert.False(t, isValidTemplateParameterOption(bp, tvp))
	})
}

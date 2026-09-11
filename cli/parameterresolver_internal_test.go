package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/v2/codersdk"
)

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

func TestWorkspaceBuildParametersForDryRun(t *testing.T) {
	t.Parallel()

	buildParameters := []codersdk.WorkspaceBuildParameter{
		{Name: "mutable", Value: "new"},
		{Name: "immutable_present", Value: "resolved"},
	}
	lastBuildParameters := []codersdk.WorkspaceBuildParameter{
		{Name: "mutable", Value: "old"},
		{Name: "immutable", Value: "repository"},
		{Name: "immutable_present", Value: "previous"},
		{Name: "ephemeral", Value: "old"},
		{Name: "removed", Value: "old"},
	}
	templateVersionParameters := []codersdk.TemplateVersionParameter{
		{Name: "mutable", Mutable: true},
		{Name: "immutable", Mutable: false, Options: []codersdk.TemplateVersionParameterOption{{Value: "different"}}},
		{Name: "immutable_present", Mutable: false},
		{Name: "ephemeral", Mutable: true, Ephemeral: true},
	}

	dryRunParameters := workspaceBuildParametersForDryRun(
		buildParameters,
		lastBuildParameters,
		templateVersionParameters,
	)

	assert.Equal(t, []codersdk.WorkspaceBuildParameter{
		{Name: "mutable", Value: "new"},
		{Name: "immutable_present", Value: "resolved"},
		{Name: "immutable", Value: "repository"},
	}, dryRunParameters)
	assert.Equal(t, []codersdk.WorkspaceBuildParameter{
		{Name: "mutable", Value: "new"},
		{Name: "immutable_present", Value: "resolved"},
	}, buildParameters, "must not mutate the workspace build parameters")
}

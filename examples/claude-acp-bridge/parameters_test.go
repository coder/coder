package nativeagents_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/dynamicparameters"
	"github.com/coder/preview"
	"github.com/coder/preview/types"
)

func modelParameter(t *testing.T, harness, model string) types.Parameter {
	t.Helper()
	inputs := map[string]string{"harness": harness}
	if model != "" {
		inputs["native_model"] = model
	}
	output, diagnostics := preview.Preview(t.Context(), preview.Input{
		ParameterValues: inputs,
	}, os.DirFS("template"))
	require.False(t, diagnostics.HasErrors(), "%s", diagnostics.Error())
	require.NotNil(t, output)
	for _, parameter := range output.Parameters {
		if parameter.Name == "native_model" {
			return parameter
		}
	}
	t.Fatal("model parameter not found")
	return types.Parameter{}
}

func TestTrustProjectDefault(t *testing.T) {
	t.Parallel()
	for _, selected := range []string{"", "false", "true"} {
		t.Run("selection="+selected, func(t *testing.T) {
			t.Parallel()
			inputs := map[string]string{}
			if selected != "" {
				inputs["trust_project"] = selected
			}
			output, diagnostics := preview.Preview(t.Context(), preview.Input{ParameterValues: inputs}, os.DirFS("template"))
			require.False(t, diagnostics.HasErrors(), "%s", diagnostics.Error())
			for _, parameter := range output.Parameters {
				if parameter.Name != "trust_project" {
					continue
				}
				require.Empty(t, parameter.Diagnostics)
				require.Equal(t, "true", parameter.DefaultValue.AsString())
				expected := selected
				if expected == "" {
					expected = "true"
				}
				require.Equal(t, expected, parameter.Value.AsString())
				return
			}
			t.Fatal("trust_project parameter not found")
		})
	}
}

type templateRenderer struct{}

func (templateRenderer) Render(ctx context.Context, _ uuid.UUID, values map[string]string) (*preview.Output, hcl.Diagnostics) {
	return preview.Preview(ctx, preview.Input{ParameterValues: values}, os.DirFS("template"))
}

func (templateRenderer) Close() {}

func TestModelUpgrade(t *testing.T) {
	t.Parallel()
	for _, harness := range []string{"claude", "codex", "opencode"} {
		t.Run(harness, func(t *testing.T) {
			t.Parallel()
			for _, model := range []string{"haiku", "sonnet", "opus"} {
				t.Run(model, func(t *testing.T) {
					t.Parallel()
					previous := []database.WorkspaceBuildParameter{
						{Name: "harness", Value: harness},
						{Name: "model", Value: model},
						{Name: "prompt", Value: "Retain this original task"},
						{Name: "repository", Value: ""},
						{Name: "trust_project", Value: "true"},
					}
					values, err := dynamicparameters.ResolveParameters(t.Context(), uuid.New(), templateRenderer{}, false, database.WorkspaceTransitionStart, previous, nil, nil)
					require.NoError(t, err)
					require.Equal(t, harness, values["harness"])
					require.Equal(t, "Retain this original task", values["prompt"])
					require.Equal(t, "true", values["trust_project"])
					require.NotContains(t, values, "model")
					require.Equal(t, modelParameter(t, harness, "").DefaultValue.AsString(), values["native_model"])
				})
			}
		})
	}
}

func TestModelOptionsByHarness(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		harness string
		model   string
	}{
		{harness: "claude", model: "haiku"},
		{harness: "codex", model: "gpt-6.1-sol"},
		{harness: "opencode", model: "anthropic/claude-haiku-5-5"},
	} {
		t.Run(test.harness, func(t *testing.T) {
			t.Parallel()
			parameter := modelParameter(t, test.harness, "")
			require.Empty(t, parameter.Diagnostics)
			require.Equal(t, "Model", parameter.DisplayName)
			require.Equal(t, "dropdown", string(parameter.FormType))
			require.False(t, parameter.Mutable)
			require.Equal(t, test.model, parameter.DefaultValue.AsString())
			require.Equal(t, test.model, parameter.Value.AsString())
			require.GreaterOrEqual(t, len(parameter.Options), 4)
			seen := make(map[string]bool)
			for _, option := range parameter.Options {
				value := option.Value.AsString()
				require.NotEmpty(t, option.Name)
				require.False(t, seen[value], "duplicate model %s", value)
				seen[value] = true
				switch test.harness {
				case "claude":
					require.False(t, strings.Contains(value, "/"))
					require.False(t, strings.HasPrefix(value, "gpt-"))
				case "codex":
					require.False(t, strings.Contains(value, "/"))
					require.False(t, strings.HasPrefix(value, "claude-"))
				case "opencode":
					require.True(t, strings.HasPrefix(value, "anthropic/claude-") || strings.HasPrefix(value, "openai/"), "%s", value)
				}
				selected := modelParameter(t, test.harness, value)
				require.Empty(t, selected.Diagnostics, "%s", value)
				require.Equal(t, value, selected.Value.AsString())
			}
		})
	}
}

func TestModelOptionsOpenCodeIncludesBothProviders(t *testing.T) {
	t.Parallel()
	openCode := modelParameter(t, "opencode", "")
	values := make(map[string]bool)
	for _, option := range openCode.Options {
		values[option.Value.AsString()] = true
	}
	for harness, provider := range map[string]string{"claude": "anthropic", "codex": "openai"} {
		for _, option := range modelParameter(t, harness, "").Options {
			value := option.Value.AsString()
			if harness == "claude" && !strings.HasPrefix(value, "claude-") {
				continue // Claude's native aliases aren't OpenCode model IDs.
			}
			require.True(t, values[provider+"/"+value], "missing %s model %s", provider, value)
		}
	}
}

func TestModelRejectsSelectionFromAnotherHarness(t *testing.T) {
	t.Parallel()
	for harness, model := range map[string]string{
		"claude":   "gpt-6.1-sol",
		"codex":    "haiku",
		"opencode": "gpt-6.1-sol",
	} {
		t.Run(harness, func(t *testing.T) {
			t.Parallel()
			parameter := modelParameter(t, harness, model)
			require.NotEmpty(t, parameter.Diagnostics)
		})
	}
}

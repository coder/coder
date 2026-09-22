package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func TestDisplayAppliedPresetRedactsSensitiveParameters(t *testing.T) {
	t.Parallel()

	const secret = "preset-secret"
	var stdout bytes.Buffer
	inv := &serpent.Invocation{Stdout: &stdout}
	preset := &codersdk.Preset{Name: "secure"}
	parameters := []codersdk.WorkspaceBuildParameter{
		{Name: "token", Value: secret},
		{Name: "region", Value: "us-east"},
	}
	definitions := []codersdk.TemplateVersionParameter{
		{Name: "token", Sensitive: true},
		{Name: "region"},
	}

	displayAppliedPreset(inv, preset, parameters, definitions)

	assert.NotContains(t, stdout.String(), secret)
	assert.Contains(t, stdout.String(), "token: '"+codersdk.RedactedValue+"'")
	assert.Contains(t, stdout.String(), "region: 'us-east'")
}

func TestTemplatePresetsToRowsRedactsSensitiveParameters(t *testing.T) {
	t.Parallel()

	const secret = "preset-secret"
	preset := codersdk.Preset{
		Name: "secure",
		Parameters: []codersdk.PresetParameter{
			{Name: "token", Value: secret},
			{Name: "region", Value: "us-east"},
		},
	}
	definitions := []codersdk.TemplateVersionParameter{
		{Name: "token", Sensitive: true},
		{Name: "region"},
	}

	rows := templatePresetsToRows(definitions, preset)
	require.Len(t, rows, 1)
	assert.NotContains(t, rows[0].Parameters, secret)
	assert.Contains(t, rows[0].Parameters, "token="+codersdk.RedactedValue)
	assert.Equal(t, codersdk.RedactedValue, rows[0].TemplatePreset.Parameters[0].Value)
	assert.Equal(t, secret, preset.Parameters[0].Value)
}

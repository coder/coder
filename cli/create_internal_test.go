package cli

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestPresetSelectOptions(t *testing.T) {
	t.Parallel()

	eu := codersdk.Preset{ID: uuid.New(), Name: "eu", Description: "Deploys to EU"}
	us := codersdk.Preset{ID: uuid.New(), Name: "us"}
	namedNone := codersdk.Preset{ID: uuid.New(), Name: "None"}
	namedNoneWithDescription := codersdk.Preset{ID: uuid.New(), Name: "None", Description: "Minimal"}

	tests := []struct {
		name        string
		presets     []codersdk.Preset
		wantOptions []string
		wantNone    string
		wantPresets map[string]uuid.UUID
	}{
		{
			name:        "NoneFirst",
			presets:     []codersdk.Preset{eu, us},
			wantOptions: []string{"None", "eu: Deploys to EU", "us"},
			wantNone:    "None",
			wantPresets: map[string]uuid.UUID{"eu: Deploys to EU": eu.ID, "us": us.ID},
		},
		{
			// A preset whose label is "None" must stay selectable, so the
			// option for applying no preset gets a distinct label.
			name:        "PresetNamedNone",
			presets:     []codersdk.Preset{namedNone, us},
			wantOptions: []string{"None (no preset)", "None", "us"},
			wantNone:    "None (no preset)",
			wantPresets: map[string]uuid.UUID{"None": namedNone.ID, "us": us.ID},
		},
		{
			name:        "PresetNamedNoneWithDescription",
			presets:     []codersdk.Preset{namedNoneWithDescription},
			wantOptions: []string{"None", "None: Minimal"},
			wantNone:    "None",
			wantPresets: map[string]uuid.UUID{"None: Minimal": namedNoneWithDescription.ID},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			options, noneOption, presetMap := presetSelectOptions(tt.presets)
			require.Equal(t, tt.wantOptions, options)
			require.Equal(t, tt.wantNone, noneOption)
			require.NotContains(t, presetMap, noneOption)
			require.Len(t, presetMap, len(tt.wantPresets))
			for option, id := range tt.wantPresets {
				require.Contains(t, presetMap, option)
				require.Equal(t, id, presetMap[option].ID)
			}
		})
	}
}

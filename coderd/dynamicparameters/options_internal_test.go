package dynamicparameters

import (
	"testing"

	"github.com/stretchr/testify/require"

	previewtypes "github.com/coder/preview/types"
)

func TestIsValidParameterOption(t *testing.T) {
	t.Parallel()

	options := []*previewtypes.ParameterOption{
		{Name: "Blue", Value: previewtypes.StringLiteral("blue")},
		{Name: "Green", Value: previewtypes.StringLiteral("green")},
	}

	tests := []struct {
		name      string
		paramType previewtypes.ParameterType
		options   []*previewtypes.ParameterOption
		value     string
		expect    bool
	}{
		{
			name:      "no options accepts anything",
			paramType: previewtypes.ParameterTypeString,
			value:     "anything",
			expect:    true,
		},
		{
			name:      "value is an option",
			paramType: previewtypes.ParameterTypeString,
			options:   options,
			value:     "blue",
			expect:    true,
		},
		{
			name:      "value was removed from the options",
			paramType: previewtypes.ParameterTypeString,
			options:   options,
			value:     "red",
			expect:    false,
		},
		{
			name:      "empty value is not an option",
			paramType: previewtypes.ParameterTypeString,
			options:   options,
			value:     "",
			expect:    false,
		},
		{
			name:      "every multi select entry is an option",
			paramType: previewtypes.ParameterTypeListString,
			options:   options,
			value:     `["blue","green"]`,
			expect:    true,
		},
		{
			name:      "one multi select entry was removed",
			paramType: previewtypes.ParameterTypeListString,
			options:   options,
			value:     `["blue","red"]`,
			expect:    false,
		},
		{
			name:      "empty multi select selects nothing invalid",
			paramType: previewtypes.ParameterTypeListString,
			options:   options,
			value:     `[]`,
			expect:    true,
		},
		{
			name:      "malformed multi select value",
			paramType: previewtypes.ParameterTypeListString,
			options:   options,
			value:     "blue",
			expect:    false,
		},
		{
			// Without a complete option set there is nothing to judge against,
			// so the value is left alone.
			name:      "unresolved option",
			paramType: previewtypes.ParameterTypeString,
			options: []*previewtypes.ParameterOption{
				{Name: "Unknown", Value: previewtypes.HCLString{}},
			},
			value:  "red",
			expect: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			parameter := previewtypes.Parameter{
				ParameterData: previewtypes.ParameterData{
					Name:    "color",
					Type:    tc.paramType,
					Mutable: true,
					Options: tc.options,
				},
			}
			require.Equal(t, tc.expect, isValidParameterOption(parameter, tc.value))
		})
	}
}

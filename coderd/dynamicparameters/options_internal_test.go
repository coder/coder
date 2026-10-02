package dynamicparameters

import (
	"testing"

	"github.com/stretchr/testify/require"

	previewtypes "github.com/coder/preview/types"
	"github.com/coder/terraform-provider-coder/v2/provider"
)

func TestIsValidParameterOption(t *testing.T) {
	t.Parallel()

	options := []*previewtypes.ParameterOption{
		{Name: "Blue", Value: previewtypes.StringLiteral("blue")},
		{Name: "Green", Value: previewtypes.StringLiteral("green")},
	}

	// A list(string) parameter that is not a multi select takes whole lists as
	// its option values, so the value is matched against them in one piece.
	listOptions := []*previewtypes.ParameterOption{
		{Name: "Cool", Value: previewtypes.StringLiteral(`["blue","green"]`)},
		{Name: "Warm", Value: previewtypes.StringLiteral(`["red","orange"]`)},
	}

	tests := []struct {
		name      string
		paramType previewtypes.ParameterType
		formType  provider.ParameterFormType
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
			formType:  provider.ParameterFormTypeMultiSelect,
			options:   options,
			value:     `["blue","green"]`,
			expect:    true,
		},
		{
			name:      "one multi select entry was removed",
			paramType: previewtypes.ParameterTypeListString,
			formType:  provider.ParameterFormTypeMultiSelect,
			options:   options,
			value:     `["blue","red"]`,
			expect:    false,
		},
		{
			name:      "empty multi select selects nothing invalid",
			paramType: previewtypes.ParameterTypeListString,
			formType:  provider.ParameterFormTypeMultiSelect,
			options:   options,
			value:     `[]`,
			expect:    true,
		},
		{
			name:      "malformed multi select value",
			paramType: previewtypes.ParameterTypeListString,
			formType:  provider.ParameterFormTypeMultiSelect,
			options:   options,
			value:     "blue",
			expect:    false,
		},
		{
			name:      "list option is matched whole",
			paramType: previewtypes.ParameterTypeListString,
			formType:  provider.ParameterFormTypeRadio,
			options:   listOptions,
			value:     `["blue","green"]`,
			expect:    true,
		},
		{
			name:      "list option was removed",
			paramType: previewtypes.ParameterTypeListString,
			formType:  provider.ParameterFormTypeRadio,
			options:   listOptions,
			value:     `["black","white"]`,
			expect:    false,
		},
		{
			// list(string) with options defaults to radio, not multi select.
			name:      "list option is matched whole without a form type",
			paramType: previewtypes.ParameterTypeListString,
			options:   listOptions,
			value:     `["blue","green"]`,
			expect:    true,
		},
		{
			// Without a complete option set there is nothing to judge against,
			name:      "unresolved option",
			paramType: previewtypes.ParameterTypeString,
			options: []*previewtypes.ParameterOption{
				{Name: "Unknown", Value: previewtypes.HCLString{}},
			},
			value:  "red",
			expect: true,
		},
		{
			// A form type the provider rejects leaves no option set to judge against.
			name:      "unsupported form type",
			paramType: previewtypes.ParameterTypeString,
			formType:  provider.ParameterFormTypeSlider,
			options:   options,
			value:     "red",
			expect:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			parameter := previewtypes.Parameter{
				ParameterData: previewtypes.ParameterData{
					Name:     "color",
					Type:     tc.paramType,
					FormType: tc.formType,
					Mutable:  true,
					Options:  tc.options,
				},
			}
			require.Equal(t, tc.expect, isValidParameterOption(parameter, tc.value))
		})
	}
}

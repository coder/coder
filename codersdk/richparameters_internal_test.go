package codersdk

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/terraform-provider-coder/v2/provider"
)

func Test_inOptionSet(t *testing.T) {
	t.Parallel()

	options := func(vals ...string) []TemplateVersionParameterOption {
		opts := make([]TemplateVersionParameterOption, 0, len(vals))
		for _, val := range vals {
			opts = append(opts, TemplateVersionParameterOption{
				Name:  val,
				Value: val,
			})
		}
		return opts
	}

	tests := []struct {
		name  string
		param TemplateVersionParameter
		value string
		want  bool
	}{
		// The function should never be called with 0 options, but if it is,
		// it should always return false.
		{
			name: "empty",
			want: false,
		},
		{
			name: "no-options",
			param: TemplateVersionParameter{
				Options: make([]TemplateVersionParameterOption, 0),
			},
		},
		{
			name: "no-options-multi",
			param: TemplateVersionParameter{
				Type:     provider.OptionTypeListString,
				FormType: string(provider.ParameterFormTypeMultiSelect),
				Options:  make([]TemplateVersionParameterOption, 0),
			},
			want: false,
		},
		{
			name: "no-options-list(string)",
			param: TemplateVersionParameter{
				Type:     provider.OptionTypeListString,
				FormType: "",
				Options:  make([]TemplateVersionParameterOption, 0),
			},
			want: false,
		},
		{
			name: "list(string)-no-form",
			param: TemplateVersionParameter{
				Type:     provider.OptionTypeListString,
				FormType: "",
				Options:  options("red", "green", "blue"),
			},
			want:  false,
			value: `["red", "blue", "green"]`,
		},
		// now for some reasonable values
		{
			name: "list(string)-multi",
			param: TemplateVersionParameter{
				Type:     provider.OptionTypeListString,
				FormType: string(provider.ParameterFormTypeMultiSelect),
				Options:  options("red", "green", "blue"),
			},
			want:  true,
			value: `["red", "blue", "green"]`,
		},
		{
			name: "string with json",
			param: TemplateVersionParameter{
				Type:    provider.OptionTypeString,
				Options: options(`["red","blue","green"]`, `["red","orange"]`),
			},
			want:  true,
			value: `["red","blue","green"]`,
		},
		{
			name: "string",
			param: TemplateVersionParameter{
				Type:    provider.OptionTypeString,
				Options: options("red", "green", "blue"),
			},
			want:  true,
			value: "red",
		},
		// False values
		{
			name: "list(string)-multi",
			param: TemplateVersionParameter{
				Type:     provider.OptionTypeListString,
				FormType: string(provider.ParameterFormTypeMultiSelect),
				Options:  options("red", "green", "blue"),
			},
			want:  false,
			value: `["red", "blue", "purple"]`,
		},
		{
			name: "string with json",
			param: TemplateVersionParameter{
				Type:    provider.OptionTypeString,
				Options: options(`["red","blue"]`, `["red","orange"]`),
			},
			want:  false,
			value: `["red","blue","green"]`,
		},
		{
			name: "string",
			param: TemplateVersionParameter{
				Type:    provider.OptionTypeString,
				Options: options("red", "green", "blue"),
			},
			want:  false,
			value: "purple",
		},
		{
			name: "list(string)-multi-scalar-value",
			param: TemplateVersionParameter{
				Type:     provider.OptionTypeListString,
				FormType: string(provider.ParameterFormTypeMultiSelect),
				Options:  options("red", "green", "blue"),
			},
			want:  false,
			value: "green",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := inOptionSet(tt.param, tt.value)
			require.Equal(t, tt.want, got)
		})
	}
}

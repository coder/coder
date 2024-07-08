package codersdk_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
)

func TestParameterResolver_ValidateResolve_New(t *testing.T) {
	t.Parallel()
	uut := codersdk.ParameterResolver{}
	p := codersdk.TemplateVersionParameter{
		Name: "n",
		Type: "number",
	}
	v, err := uut.ValidateResolve(p, &codersdk.WorkspaceBuildParameter{
		Name:  "n",
		Value: "1",
	})
	require.NoError(t, err)
	require.Equal(t, "1", v)
}

func TestParameterResolver_ValidateResolve_Default(t *testing.T) {
	t.Parallel()
	uut := codersdk.ParameterResolver{}
	p := codersdk.TemplateVersionParameter{
		Name:         "n",
		Type:         "number",
		DefaultValue: "5",
	}
	v, err := uut.ValidateResolve(p, nil)
	require.NoError(t, err)
	require.Equal(t, "5", v)
}

func TestParameterResolver_ValidateResolve_MissingRequired(t *testing.T) {
	t.Parallel()
	uut := codersdk.ParameterResolver{}
	p := codersdk.TemplateVersionParameter{
		Name:     "n",
		Type:     "number",
		Required: true,
	}
	v, err := uut.ValidateResolve(p, nil)
	require.Error(t, err)
	require.Equal(t, "", v)
}

func TestParameterResolver_ValidateResolve_PrevRequired(t *testing.T) {
	t.Parallel()
	uut := codersdk.ParameterResolver{
		Rich: []codersdk.WorkspaceBuildParameter{{Name: "n", Value: "5"}},
	}
	p := codersdk.TemplateVersionParameter{
		Name:     "n",
		Type:     "number",
		Required: true,
	}
	v, err := uut.ValidateResolve(p, nil)
	require.NoError(t, err)
	require.Equal(t, "5", v)
}

func TestParameterResolver_ValidateResolve_PrevInvalid(t *testing.T) {
	t.Parallel()
	uut := codersdk.ParameterResolver{
		Rich: []codersdk.WorkspaceBuildParameter{{Name: "n", Value: "11"}},
	}
	p := codersdk.TemplateVersionParameter{
		Name:          "n",
		Type:          "number",
		ValidationMax: ptr.Ref(int32(10)),
		ValidationMin: ptr.Ref(int32(1)),
	}
	v, err := uut.ValidateResolve(p, nil)
	require.Error(t, err)
	require.Equal(t, "", v)
}

func TestParameterResolver_ValidateResolve_DefaultInvalid(t *testing.T) {
	// this one arises from an error on the template itself, where the default
	// value doesn't pass validation.  But, it's good to catch early and error out
	// rather than send invalid data to the provisioner
	t.Parallel()
	uut := codersdk.ParameterResolver{}
	p := codersdk.TemplateVersionParameter{
		Name:          "n",
		Type:          "number",
		ValidationMax: ptr.Ref(int32(10)),
		ValidationMin: ptr.Ref(int32(1)),
		DefaultValue:  "11",
	}
	v, err := uut.ValidateResolve(p, nil)
	require.Error(t, err)
	require.Equal(t, "", v)
}

func TestParameterResolver_ValidateResolve_NewOverridesOld(t *testing.T) {
	t.Parallel()
	uut := codersdk.ParameterResolver{
		Rich: []codersdk.WorkspaceBuildParameter{{Name: "n", Value: "5"}},
	}
	p := codersdk.TemplateVersionParameter{
		Name:     "n",
		Type:     "number",
		Required: true,
		Mutable:  true,
	}
	v, err := uut.ValidateResolve(p, &codersdk.WorkspaceBuildParameter{
		Name:  "n",
		Value: "6",
	})
	require.NoError(t, err)
	require.Equal(t, "6", v)
}

func TestParameterResolver_ValidateResolve_Immutable(t *testing.T) {
	t.Parallel()
	uut := codersdk.ParameterResolver{
		Rich: []codersdk.WorkspaceBuildParameter{{Name: "n", Value: "5"}},
	}
	p := codersdk.TemplateVersionParameter{
		Name:     "n",
		Type:     "number",
		Required: true,
		Mutable:  false,
	}
	v, err := uut.ValidateResolve(p, &codersdk.WorkspaceBuildParameter{
		Name:  "n",
		Value: "6",
	})
	require.Error(t, err)
	require.Equal(t, "", v)
}

func TestRichParameterValidation(t *testing.T) {
	t.Parallel()

	const (
		stringParameterName  = "string_parameter"
		stringParameterValue = "abc"

		numberParameterName  = "number_parameter"
		numberParameterValue = "7"

		boolParameterName  = "bool_parameter"
		boolParameterValue = "true"

		listOfStringsParameterName  = "list_of_strings_parameter"
		listOfStringsParameterValue = `["a","b","c"]`
	)

	initialBuildParameters := []codersdk.WorkspaceBuildParameter{
		{Name: stringParameterName, Value: stringParameterValue},
		{Name: numberParameterName, Value: numberParameterValue},
		{Name: boolParameterName, Value: boolParameterValue},
		{Name: listOfStringsParameterName, Value: listOfStringsParameterValue},
	}

	t.Run("NoValidation", func(t *testing.T) {
		t.Parallel()

		p := codersdk.TemplateVersionParameter{
			Name: numberParameterName, Type: "number", Mutable: true,
		}

		uut := codersdk.ParameterResolver{
			Rich: initialBuildParameters,
		}
		v, err := uut.ValidateResolve(p, &codersdk.WorkspaceBuildParameter{
			Name: numberParameterName, Value: "42",
		})
		require.NoError(t, err)
		require.Equal(t, v, "42")
	})

	t.Run("Validation", func(t *testing.T) {
		t.Parallel()

		numberRichParameters := []codersdk.TemplateVersionParameter{
			{Name: stringParameterName, Type: "string", Mutable: true},
			{Name: numberParameterName, Type: "number", Mutable: true, ValidationMin: ptr.Ref(int32(3)), ValidationMax: ptr.Ref(int32(10))},
			{Name: boolParameterName, Type: "bool", Mutable: true},
		}

		numberRichParametersMinOnly := []codersdk.TemplateVersionParameter{
			{Name: stringParameterName, Type: "string", Mutable: true},
			{Name: numberParameterName, Type: "number", Mutable: true, ValidationMin: ptr.Ref(int32(5))},
			{Name: boolParameterName, Type: "bool", Mutable: true},
		}

		numberRichParametersMaxOnly := []codersdk.TemplateVersionParameter{
			{Name: stringParameterName, Type: "string", Mutable: true},
			{Name: numberParameterName, Type: "number", Mutable: true, ValidationMax: ptr.Ref(int32(5))},
			{Name: boolParameterName, Type: "bool", Mutable: true},
		}

		monotonicIncreasingNumberRichParameters := []codersdk.TemplateVersionParameter{
			{Name: stringParameterName, Type: "string", Mutable: true},
			{Name: numberParameterName, Type: "number", Mutable: true, ValidationMin: ptr.Ref(int32(3)), ValidationMax: ptr.Ref(int32(100)), ValidationMonotonic: codersdk.MonotonicOrderIncreasing},
			{Name: boolParameterName, Type: "bool", Mutable: true},
		}

		monotonicDecreasingNumberRichParameters := []codersdk.TemplateVersionParameter{
			{Name: stringParameterName, Type: "string", Mutable: true},
			{Name: numberParameterName, Type: "number", Mutable: true, ValidationMin: ptr.Ref(int32(3)), ValidationMax: ptr.Ref(int32(100)), ValidationMonotonic: codersdk.MonotonicOrderDecreasing},
			{Name: boolParameterName, Type: "bool", Mutable: true},
		}

		stringRichParameters := []codersdk.TemplateVersionParameter{
			{Name: stringParameterName, Type: "string", Mutable: true},
			{Name: numberParameterName, Type: "number", Mutable: true},
			{Name: boolParameterName, Type: "bool", Mutable: true},
		}

		boolRichParameters := []codersdk.TemplateVersionParameter{
			{Name: stringParameterName, Type: "string", Mutable: true},
			{Name: numberParameterName, Type: "number", Mutable: true},
			{Name: boolParameterName, Type: "bool", Mutable: true},
		}

		regexRichParameters := []codersdk.TemplateVersionParameter{
			{Name: stringParameterName, Type: "string", Mutable: true, ValidationRegex: "^[a-z]+$", ValidationError: "this is error"},
			{Name: numberParameterName, Type: "number", Mutable: true},
			{Name: boolParameterName, Type: "bool", Mutable: true},
		}

		listOfStringsRichParameters := []codersdk.TemplateVersionParameter{
			{Name: listOfStringsParameterName, Type: "list(string)", Mutable: true},
		}

		tests := []struct {
			parameterName  string
			value          string
			valid          bool
			richParameters []codersdk.TemplateVersionParameter
		}{
			{numberParameterName, "2", false, numberRichParameters},
			{numberParameterName, "3", true, numberRichParameters},
			{numberParameterName, "10", true, numberRichParameters},
			{numberParameterName, "11", false, numberRichParameters},

			{numberParameterName, "4", false, numberRichParametersMinOnly},
			{numberParameterName, "5", true, numberRichParametersMinOnly},
			{numberParameterName, "6", true, numberRichParametersMinOnly},

			{numberParameterName, "4", true, numberRichParametersMaxOnly},
			{numberParameterName, "5", true, numberRichParametersMaxOnly},
			{numberParameterName, "6", false, numberRichParametersMaxOnly},

			{numberParameterName, "6", false, monotonicIncreasingNumberRichParameters},
			{numberParameterName, "7", true, monotonicIncreasingNumberRichParameters},
			{numberParameterName, "8", true, monotonicIncreasingNumberRichParameters},
			{numberParameterName, "11", true, monotonicIncreasingNumberRichParameters},
			{numberParameterName, "53", true, monotonicIncreasingNumberRichParameters},

			{numberParameterName, "6", true, monotonicDecreasingNumberRichParameters},
			{numberParameterName, "7", true, monotonicDecreasingNumberRichParameters},
			{numberParameterName, "8", false, monotonicDecreasingNumberRichParameters},
			{numberParameterName, "11", false, monotonicDecreasingNumberRichParameters},
			{numberParameterName, "53", false, monotonicDecreasingNumberRichParameters},

			{stringParameterName, "", true, stringRichParameters},
			{stringParameterName, "foobar", true, stringRichParameters},

			{stringParameterName, "abcd", true, regexRichParameters},
			{stringParameterName, "abcd1", false, regexRichParameters},

			{boolParameterName, "true", true, boolRichParameters},
			{boolParameterName, "false", true, boolRichParameters},
			{boolParameterName, "cat", false, boolRichParameters},

			{listOfStringsParameterName, `[]`, true, listOfStringsRichParameters},
			{listOfStringsParameterName, `["aa"]`, true, listOfStringsRichParameters},
			{listOfStringsParameterName, `["aa]`, false, listOfStringsRichParameters},
			{listOfStringsParameterName, ``, false, listOfStringsRichParameters},
		}

		for _, tc := range tests {
			tc := tc
			t.Run(tc.parameterName+"-"+tc.value, func(t *testing.T) {
				t.Parallel()

				uut := codersdk.ParameterResolver{
					Rich: initialBuildParameters,
				}

				for _, p := range tc.richParameters {
					if p.Name != tc.parameterName {
						continue
					}
					v, err := uut.ValidateResolve(p, &codersdk.WorkspaceBuildParameter{
						Name:  tc.parameterName,
						Value: tc.value,
					})
					if tc.valid {
						require.NoError(t, err)
						require.Equal(t, tc.value, v)
					} else {
						require.Error(t, err)
					}
				}
			})
		}
	})
}

func TestParameterResolver_ValidateResolve_EmptyString_Monotonic(t *testing.T) {
	t.Parallel()
	uut := codersdk.ParameterResolver{
		Rich: []codersdk.WorkspaceBuildParameter{{Name: "n", Value: ""}},
	}
	p := codersdk.TemplateVersionParameter{
		Name:                "n",
		Type:                "number",
		Mutable:             true,
		DefaultValue:        "0",
		ValidationMonotonic: codersdk.MonotonicOrderIncreasing,
	}
	v, err := uut.ValidateResolve(p, &codersdk.WorkspaceBuildParameter{
		Name:  "n",
		Value: "1",
	})
	require.NoError(t, err)
	require.Equal(t, "1", v)
}

func TestParameterResolver_ValidateResolve_Ephemeral_OverridePrevious(t *testing.T) {
	t.Parallel()
	uut := codersdk.ParameterResolver{
		Rich: []codersdk.WorkspaceBuildParameter{{Name: "n", Value: "5"}},
	}
	p := codersdk.TemplateVersionParameter{
		Name:         "n",
		Type:         "number",
		Mutable:      true,
		DefaultValue: "4",
		Ephemeral:    true,
	}
	v, err := uut.ValidateResolve(p, &codersdk.WorkspaceBuildParameter{
		Name:  "n",
		Value: "6",
	})
	require.NoError(t, err)
	require.Equal(t, "6", v)
}

func TestParameterResolver_ValidateResolve_Ephemeral_FirstTime(t *testing.T) {
	t.Parallel()
	uut := codersdk.ParameterResolver{}
	p := codersdk.TemplateVersionParameter{
		Name:         "n",
		Type:         "number",
		Mutable:      true,
		DefaultValue: "5",
		Ephemeral:    true,
	}
	v, err := uut.ValidateResolve(p, &codersdk.WorkspaceBuildParameter{
		Name:  "n",
		Value: "6",
	})
	require.NoError(t, err)
	require.Equal(t, "6", v)
}

func TestParameterResolver_ValidateResolve_Ephemeral_UseDefault(t *testing.T) {
	t.Parallel()
	uut := codersdk.ParameterResolver{}
	p := codersdk.TemplateVersionParameter{
		Name:         "n",
		Type:         "number",
		Mutable:      true,
		DefaultValue: "5",
		Ephemeral:    true,
	}
	v, err := uut.ValidateResolve(p, nil)
	require.NoError(t, err)
	require.Equal(t, "5", v)
}

func TestParameterResolver_ValidateResolve_Ephemeral_UseEmptyDefault(t *testing.T) {
	t.Parallel()
	uut := codersdk.ParameterResolver{}
	p := codersdk.TemplateVersionParameter{
		Name:         "n",
		Type:         "number",
		Mutable:      true,
		DefaultValue: "",
		Ephemeral:    true,
	}
	v, err := uut.ValidateResolve(p, nil)
	require.NoError(t, err)
	require.Equal(t, "", v)
}

func TestParameterResolver_ValidateResolve_Number_CustomError(t *testing.T) {
	t.Parallel()
	uut := codersdk.ParameterResolver{}
	p := codersdk.TemplateVersionParameter{
		Name:         "n",
		Type:         "number",
		Mutable:      true,
		DefaultValue: "5",

		ValidationMin:   ptr.Ref(int32(4)),
		ValidationMax:   ptr.Ref(int32(6)),
		ValidationError: "These are values for testing purposes: {min}, {max}, and {value}.",
	}
	_, err := uut.ValidateResolve(p, &codersdk.WorkspaceBuildParameter{
		Name:  "n",
		Value: "8",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "These are values for testing purposes: 4, 6, and 8.")
}

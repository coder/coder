package codersdk_test

import (
	"github.com/coder/coder/codersdk"
	"github.com/stretchr/testify/require"
	"testing"
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
		ValidationMax: 10,
		ValidationMin: 1,
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
		ValidationMax: 10,
		ValidationMin: 1,
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

func TestParameterResolver_ValidateResolve_Legacy(t *testing.T) {
	t.Parallel()
	uut := codersdk.ParameterResolver{
		Legacy: []codersdk.Parameter{
			{Name: "l", SourceValue: "5"},
			{Name: "n", SourceValue: "6"},
		},
	}
	p := codersdk.TemplateVersionParameter{
		Name:               "n",
		Type:               "number",
		Required:           true,
		LegacyVariableName: "l",
	}
	v, err := uut.ValidateResolve(p, nil)
	require.NoError(t, err)
	require.Equal(t, "5", v)
}

func TestParameterResolver_ValidateResolve_PreferRichOverLegacy(t *testing.T) {
	t.Parallel()
	uut := codersdk.ParameterResolver{
		Rich: []codersdk.WorkspaceBuildParameter{{Name: "n", Value: "7"}},
		Legacy: []codersdk.Parameter{
			{Name: "l", SourceValue: "5"},
			{Name: "n", SourceValue: "6"},
		},
	}
	p := codersdk.TemplateVersionParameter{
		Name:               "n",
		Type:               "number",
		Required:           true,
		LegacyVariableName: "l",
	}
	v, err := uut.ValidateResolve(p, nil)
	require.NoError(t, err)
	require.Equal(t, "7", v)
}

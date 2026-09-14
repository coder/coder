package proto

import (
	"golang.org/x/xerrors"

	"github.com/coder/terraform-provider-coder/v2/provider"
)

func ProviderFormType(ft ParameterFormType) (provider.ParameterFormType, error) {
	switch ft {
	case ParameterFormType_DEFAULT:
		return provider.ParameterFormTypeDefault, nil
	case ParameterFormType_FORM_ERROR:
		return provider.ParameterFormTypeError, nil
	case ParameterFormType_RADIO:
		return provider.ParameterFormTypeRadio, nil
	case ParameterFormType_DROPDOWN:
		return provider.ParameterFormTypeDropdown, nil
	case ParameterFormType_INPUT:
		return provider.ParameterFormTypeInput, nil
	case ParameterFormType_TEXTAREA:
		return provider.ParameterFormTypeTextArea, nil
	case ParameterFormType_SLIDER:
		return provider.ParameterFormTypeSlider, nil
	case ParameterFormType_CHECKBOX:
		return provider.ParameterFormTypeCheckbox, nil
	case ParameterFormType_SWITCH:
		return provider.ParameterFormTypeSwitch, nil
	case ParameterFormType_TAGSELECT:
		return provider.ParameterFormTypeTagSelect, nil
	case ParameterFormType_MULTISELECT:
		return provider.ParameterFormTypeMultiSelect, nil
	}
	return provider.ParameterFormTypeDefault, xerrors.Errorf("unsupported form type: %s", ft)
}

func FormType(ft provider.ParameterFormType) (ParameterFormType, error) {
	switch ft {
	case provider.ParameterFormTypeDefault:
		return ParameterFormType_DEFAULT, nil
	case provider.ParameterFormTypeError:
		return ParameterFormType_FORM_ERROR, nil
	case provider.ParameterFormTypeRadio:
		return ParameterFormType_RADIO, nil
	case provider.ParameterFormTypeDropdown:
		return ParameterFormType_DROPDOWN, nil
	case provider.ParameterFormTypeInput:
		return ParameterFormType_INPUT, nil
	case provider.ParameterFormTypeTextArea:
		return ParameterFormType_TEXTAREA, nil
	case provider.ParameterFormTypeSlider:
		return ParameterFormType_SLIDER, nil
	case provider.ParameterFormTypeCheckbox:
		return ParameterFormType_CHECKBOX, nil
	case provider.ParameterFormTypeSwitch:
		return ParameterFormType_SWITCH, nil
	case provider.ParameterFormTypeTagSelect:
		return ParameterFormType_TAGSELECT, nil
	case provider.ParameterFormTypeMultiSelect:
		return ParameterFormType_MULTISELECT, nil
	}
	return ParameterFormType_DEFAULT, xerrors.Errorf("unsupported form type: %s", ft)
}

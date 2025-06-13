package codersdk

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/codersdk/wsjson"
	"github.com/coder/websocket"
)

type ParameterFormType string

const (
	ParameterFormTypeDefault     ParameterFormType = ""
	ParameterFormTypeRadio       ParameterFormType = "radio"
	ParameterFormTypeSlider      ParameterFormType = "slider"
	ParameterFormTypeInput       ParameterFormType = "input"
	ParameterFormTypeDropdown    ParameterFormType = "dropdown"
	ParameterFormTypeCheckbox    ParameterFormType = "checkbox"
	ParameterFormTypeSwitch      ParameterFormType = "switch"
	ParameterFormTypeMultiSelect ParameterFormType = "multi-select"
	ParameterFormTypeTagSelect   ParameterFormType = "tag-select"
	ParameterFormTypeTextArea    ParameterFormType = "textarea"
	ParameterFormTypeError       ParameterFormType = "error"
)

type OptionType string

const (
	OptionTypeString     OptionType = "string"
	OptionTypeNumber     OptionType = "number"
	OptionTypeBoolean    OptionType = "bool"
	OptionTypeListString OptionType = "list(string)"
)

type DiagnosticSeverityString string

const (
	DiagnosticSeverityError   DiagnosticSeverityString = "error"
	DiagnosticSeverityWarning DiagnosticSeverityString = "warning"
)

// FriendlyDiagnostic == previewtypes.FriendlyDiagnostic
// Copied to avoid import deps
type FriendlyDiagnostic struct {
	Severity DiagnosticSeverityString `json:"severity"`
	Summary  string                   `json:"summary"`
	Detail   string                   `json:"detail"`

	Extra DiagnosticExtra `json:"extra"`
}

type DiagnosticExtra struct {
	Code string `json:"code"`
}

// NullHCLString == `previewtypes.NullHCLString`.
type NullHCLString struct {
	Value string `json:"value"`
	Valid bool   `json:"valid"`
}

type PreviewParameter struct {
	PreviewParameterData
	Value       NullHCLString        `json:"value"`
	Diagnostics []FriendlyDiagnostic `json:"diagnostics"`
}

type PreviewParameterData struct {
	Name         string                       `json:"name"`
	DisplayName  string                       `json:"display_name"`
	Description  string                       `json:"description"`
	Type         OptionType                   `json:"type"`
	FormType     ParameterFormType            `json:"form_type"`
	Styling      PreviewParameterStyling      `json:"styling"`
	Mutable      bool                         `json:"mutable"`
	DefaultValue NullHCLString                `json:"default_value"`
	Icon         string                       `json:"icon"`
	Options      []PreviewParameterOption     `json:"options"`
	Validations  []PreviewParameterValidation `json:"validations"`
	Required     bool                         `json:"required"`
	// legacy_variable_name was removed (= 14)
	Order     int64 `json:"order"`
	Ephemeral bool  `json:"ephemeral"`
}

type PreviewParameterStyling struct {
	Placeholder *string `json:"placeholder,omitempty"`
	Disabled    *bool   `json:"disabled,omitempty"`
	Label       *string `json:"label,omitempty"`
}

type PreviewParameterOption struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Value       NullHCLString `json:"value"`
	Icon        string        `json:"icon"`
}

type PreviewParameterValidation struct {
	Error string `json:"validation_error"`

	// All validation attributes are optional.
	Regex     *string `json:"validation_regex"`
	Min       *int64  `json:"validation_min"`
	Max       *int64  `json:"validation_max"`
	Monotonic *string `json:"validation_monotonic"`
}

type DynamicParametersRequest struct {
	// ID identifies the request. The response contains the same
	// ID so that the client can match it to the request.
	ID     int               `json:"id"`
	Inputs map[string]string `json:"inputs"`
	// OwnerID if uuid.Nil, it defaults to `codersdk.Me`
	OwnerID uuid.UUID `json:"owner_id,omitempty" format:"uuid"`
}

type DynamicParametersResponse struct {
	ID          int                  `json:"id"`
	Diagnostics []FriendlyDiagnostic `json:"diagnostics"`
	Parameters  []PreviewParameter   `json:"parameters"`
	// TODO: Workspace tags
}

func (c *Client) TemplateVersionDynamicParameters(ctx context.Context, version uuid.UUID) (*wsjson.Stream[DynamicParametersResponse, DynamicParametersRequest], error) {
	conn, err := c.Dial(ctx, fmt.Sprintf("/api/v2/templateversions/%s/dynamic-parameters", version), nil)
	if err != nil {
		return nil, err
	}
	return wsjson.NewStream[DynamicParametersResponse, DynamicParametersRequest](conn, websocket.MessageText, websocket.MessageText, c.Logger()), nil
}

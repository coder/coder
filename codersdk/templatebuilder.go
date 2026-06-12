package codersdk

import "encoding/json"

// TemplateBuilderVariableType enumerates the variable types
// supported by template builder module manifests.
type TemplateBuilderVariableType string

const (
	TemplateBuilderVariableTypeString TemplateBuilderVariableType = "string"
	TemplateBuilderVariableTypeNumber TemplateBuilderVariableType = "number"
	TemplateBuilderVariableTypeBool   TemplateBuilderVariableType = "bool"
)

type TemplateBuilderModuleVariable struct {
	Name        string                      `json:"name"`
	Type        TemplateBuilderVariableType `json:"type"`
	Description string                      `json:"description"`
	Default     json.RawMessage             `json:"default,omitempty"`
	Required    bool                        `json:"required"`
	Sensitive   bool                        `json:"sensitive"`
	Computed    bool                        `json:"computed"`
}

// TemplateBuilderModule is the API response type returned by
// GET /api/v2/templatebuilder/modules. The Version field is
// populated from the catalog manifest's PinnedVersion at serving time.
type TemplateBuilderModule struct {
	ID            string                          `json:"id"`
	DisplayName   string                          `json:"display_name"`
	Description   string                          `json:"description"`
	Icon          string                          `json:"icon"`
	Category      string                          `json:"category"`
	Version       string                          `json:"version"`
	CompatibleOS  []string                        `json:"compatible_os"`
	ConflictsWith []string                        `json:"conflicts_with"`
	Variables     []TemplateBuilderModuleVariable `json:"variables"`
}

type TemplateBuilderModulesResponse struct {
	Modules []TemplateBuilderModule `json:"modules"`
}

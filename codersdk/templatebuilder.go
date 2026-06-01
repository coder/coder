package codersdk

// TemplateBuilderVariableType represents the type of a template builder variable.
type TemplateBuilderVariableType string

const (
	TemplateBuilderVariableTypeString TemplateBuilderVariableType = "string"
	TemplateBuilderVariableTypeNumber TemplateBuilderVariableType = "number"
	TemplateBuilderVariableTypeBool   TemplateBuilderVariableType = "bool"
)

// TemplateBuilderModuleVariable represents a variable within a template builder module.
type TemplateBuilderModuleVariable struct {
	Name           string                      `json:"name"`
	Type           TemplateBuilderVariableType `json:"type"`
	Description    string                      `json:"description"`
	Default        *string                     `json:"default,omitempty"`
	Required       bool                        `json:"required"`
	Sensitive      bool                        `json:"sensitive"`
	BuilderManaged bool                        `json:"builder_managed"`
}

// TemplateBuilderModule is the API response type returned by
// GET /api/v2/templatebuilder/modules. The Version field is
// populated from the catalog's PinnedVersion at serving time.
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

// TemplateBuilderModulesResponse is the response body for listing
// template builder modules.
type TemplateBuilderModulesResponse struct {
	Modules []TemplateBuilderModule `json:"modules"`
}

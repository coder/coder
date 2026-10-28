package codersdk

import "github.com/google/uuid"

// FoundationPlanPreview is advisory metadata, never output values or a raw plan.
// Missing or unavailable previews must not be interpreted as an empty change set.
type FoundationPlanPreview struct {
	Available          bool                          `json:"available"`
	OutputChanges      []FoundationOutputChange      `json:"output_changes"`
	ConsumerReferences []FoundationConsumerReference `json:"consumer_references"`
	Advisory           string                        `json:"advisory"`
}

// FoundationOutputChange describes a root output without disclosing its value.
type FoundationOutputChange struct {
	Name      string   `json:"name"`
	Actions   []string `json:"actions"`
	Unknown   bool     `json:"unknown"`
	Sensitive bool     `json:"sensitive"`
}

// FoundationConsumerReference reports changed outputs referenced by a version.
// Unresolved includes dynamic references and incomplete or unavailable analysis.
type FoundationConsumerReference struct {
	TemplateVersionID uuid.UUID `json:"template_version_id" format:"uuid"`
	OutputNames       []string  `json:"output_names"`
	Unresolved        bool      `json:"unresolved"`
}

//nolint:revive // Package name is used for clarity within notifications package hierarchy
package types

type TemplateAction struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

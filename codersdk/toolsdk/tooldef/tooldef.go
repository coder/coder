// Package tooldef defines the provider-neutral description of a tool that
// can be exposed to language models and MCP clients.
package tooldef

// Tool describes a tool's name, purpose, and input parameters.
type Tool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Schema      Schema `json:"parameters"`
}

// Schema is the JSON Schema object describing a tool's input parameters.
type Schema struct {
	Required   []string       `json:"required"`
	Properties map[string]any `json:"properties"`
}

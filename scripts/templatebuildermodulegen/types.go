package main

import "encoding/json"

// ModuleManifest is the on-disk module.json schema.
type ModuleManifest struct {
	ID            string           `json:"id"`
	DisplayName   string           `json:"display_name"`
	Description   string           `json:"description"`
	Icon          string           `json:"icon"`
	Category      string           `json:"category"`
	Tags          []string         `json:"tags"`
	CompatibleOS  []string         `json:"compatible_os"`
	ConflictsWith []string         `json:"conflicts_with"`
	Namespace     string           `json:"namespace"`
	PinnedVersion string           `json:"pinned_version"`
	Variables     []ModuleVariable `json:"variables"`
}

// ModuleVariable is a variable declaration within a module manifest.
type ModuleVariable struct {
	Name        string          `json:"name"`
	Type        string          `json:"type"`
	Description string          `json:"description"`
	Default     json.RawMessage `json:"default,omitempty"`
	Required    bool            `json:"required"`
	Sensitive   bool            `json:"sensitive"`
	Computed    bool            `json:"computed"`
}

// registryModule is the JSON shape returned by GET /api/modules/{id}.
type registryModule struct {
	ID          string             `json:"id"`
	Slug        string             `json:"slug"`
	DisplayName string             `json:"displayName"`
	Description string             `json:"description"`
	IconURL     string             `json:"iconUrl"`
	Tags        []string           `json:"tags"`
	Variables   []registryVariable `json:"variables"`
	Namespace   string             `json:"contributorNamespace"`
}

// registryVariable is a variable as returned by the registry API.
// The Default field is a raw JSON value because the API returns typed
// defaults (string, bool, number, null, array, object).
type registryVariable struct {
	Name        string      `json:"name"`
	Type        string      `json:"type"`
	Description string      `json:"description"`
	Default     interface{} `json:"default"`
	Required    bool        `json:"required"`
	Sensitive   bool        `json:"sensitive"`
}

// terraformVersionsResponse wraps the Terraform protocol versions endpoint.
type terraformVersionsResponse struct {
	Modules []struct {
		Versions []struct {
			Version string `json:"version"`
		} `json:"versions"`
	} `json:"modules"`
}

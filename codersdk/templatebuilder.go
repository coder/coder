package codersdk

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"

	"github.com/google/uuid"
)

// TemplateBuilderVariableType enumerates the variable types
// supported by template builder module manifests.
type TemplateBuilderVariableType string

const (
	TemplateBuilderVariableTypeString TemplateBuilderVariableType = "string"
	TemplateBuilderVariableTypeNumber TemplateBuilderVariableType = "number"
	TemplateBuilderVariableTypeBool   TemplateBuilderVariableType = "bool"
)

// TemplateBuilderModuleVariable describes a single input variable declared by a
// base template or module manifest.
type TemplateBuilderModuleVariable struct {
	// Name is the Terraform variable name values are keyed by.
	Name string `json:"name"`
	// Type constrains the accepted value (string, number, or bool).
	Type TemplateBuilderVariableType `json:"type"`
	// Description is human-facing help text shown in the builder.
	Description string `json:"description"`
	// Default is applied when no value is supplied; omitted when there is none.
	Default json.RawMessage `json:"default,omitempty"`
	// Required reports whether a value must be supplied to compose.
	Required bool `json:"required"`
	// Sensitive hides the value in the UI and excludes it from logs.
	Sensitive bool `json:"sensitive"`
}

// TemplateBuilderModule is the API response type returned by
// GET /api/v2/templatebuilder/modules. The Version field is
// populated from the catalog manifest's PinnedVersion at serving time.
type TemplateBuilderModule struct {
	// ID uniquely identifies the module in the catalog and compose requests.
	ID string `json:"id"`
	// DisplayName is the human-facing module name.
	DisplayName string `json:"display_name"`
	// Description summarizes what the module does.
	Description string `json:"description"`
	// Icon is a URL or built-in icon path.
	Icon string `json:"icon"`
	// Category groups related modules in the builder.
	Category string `json:"category"`
	// Version is the pinned module version from the catalog manifest.
	Version string `json:"version"`
	// CompatibleOS lists the base operating systems the module supports.
	CompatibleOS []string `json:"compatible_os"`
	// ConflictsWith lists module IDs that cannot be selected alongside this one.
	ConflictsWith []string `json:"conflicts_with"`
	// Variables are the module's configurable inputs.
	Variables []TemplateBuilderModuleVariable `json:"variables"`
}

// TemplateBuilderModulesResponse is the response body for listing template builder modules.
type TemplateBuilderModulesResponse struct {
	// Modules are the modules available for the requested base.
	Modules []TemplateBuilderModule `json:"modules"`
}

// TemplateBuilderBase is the API response type for a base template
// returned by GET /api/v2/templatebuilder/bases.
type TemplateBuilderBase struct {
	// ID uniquely identifies the base in compose requests.
	ID string `json:"id"`
	// Name is the human-facing base template name.
	Name string `json:"name"`
	// Description summarizes the infrastructure the base provisions.
	Description string `json:"description"`
	// Icon is a URL or built-in icon path.
	Icon string `json:"icon"`
	// OS is the operating system the base provisions.
	OS string `json:"os"`
	// Variables are the base template's configurable inputs.
	Variables []TemplateBuilderModuleVariable `json:"variables"`
	// Prerequisites describes setup required before using the base, if any.
	Prerequisites string `json:"prerequisites"`
	// Agents are the coder_agents the base declares for modules to target.
	Agents []TemplateBuilderBaseAgent `json:"agents"`
}

// TemplateBuilderBaseAgent is a coder_agent a base template declares. Modules
// composed onto the base target one of these by Name.
type TemplateBuilderBaseAgent struct {
	// Name is the coder_agent resource name modules target.
	Name string `json:"name"`
	// DisplayName is the human-facing agent name; may be empty.
	DisplayName string `json:"display_name"`
	// Default reports whether modules attach to this agent when they do not
	// name one.
	Default bool `json:"default"`
}

// TemplateBuilderBasesResponse is the response body for listing template builder bases.
type TemplateBuilderBasesResponse struct {
	// Bases are the available base templates.
	Bases []TemplateBuilderBase `json:"bases"`
}

// TemplateBuilderBases returns the list of base templates available
// in the template builder.
func (c *Client) TemplateBuilderBases(ctx context.Context) (TemplateBuilderBasesResponse, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/templatebuilder/bases", nil)
	if err != nil {
		return TemplateBuilderBasesResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return TemplateBuilderBasesResponse{}, ReadBodyAsError(res)
	}
	var resp TemplateBuilderBasesResponse
	return resp, ReadBodyAsJSON(res, &resp)
}

// TemplateBuilderModules returns the list of modules available for a given
// base template. If base is empty, all modules are returned.
func (c *Client) TemplateBuilderModules(ctx context.Context, base string) (TemplateBuilderModulesResponse, error) {
	path := "/api/v2/templatebuilder/modules"
	if base != "" {
		q := url.Values{"base": {base}}
		path += "?" + q.Encode()
	}
	res, err := c.Request(ctx, http.MethodGet, path, nil)
	if err != nil {
		return TemplateBuilderModulesResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return TemplateBuilderModulesResponse{}, ReadBodyAsError(res)
	}
	var resp TemplateBuilderModulesResponse
	return resp, ReadBodyAsJSON(res, &resp)
}

// TemplateBuilderComposeRequest is the request body for
// POST /api/v2/templatebuilder/compose.
type TemplateBuilderComposeRequest struct {
	// BaseTemplateID selects the base template to compose onto.
	BaseTemplateID string `json:"base_template_id"`
	// BaseVariableValues sets the base's input variables by name.
	BaseVariableValues map[string]string `json:"base_variable_values,omitempty"`
	// Modules are the modules to compose onto the base.
	Modules []TemplateBuilderComposeModule `json:"modules"`
}

// TemplateBuilderComposeModule identifies a module and its variable
// values for the compose request.
type TemplateBuilderComposeModule struct {
	// ID selects a catalog module to compose.
	ID string `json:"id"`
	// AgentName targets a base coder_agent by name. Empty uses the base default.
	AgentName string `json:"agent_name,omitempty"`
	// Variables sets the module's input variables by name.
	Variables map[string]string `json:"variables,omitempty"`
}

// TemplateBuilderCompose renders a base template with the selected
// modules and returns the resulting tar archive bytes.
func (c *Client) TemplateBuilderCompose(ctx context.Context, req TemplateBuilderComposeRequest) ([]byte, error) {
	res, err := c.Request(ctx, http.MethodPost, "/api/v2/templatebuilder/compose", req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	return io.ReadAll(res.Body)
}

// TemplateBuilderCreateTemplateRequest is the request body for
// POST /api/v2/templatebuilder/compose/template.
type TemplateBuilderCreateTemplateRequest struct {
	// BaseTemplateID selects the base template to compose onto.
	BaseTemplateID string `json:"base_template_id"`
	// BaseVariableValues sets the base's input variables by name.
	BaseVariableValues map[string]string `json:"base_variable_values,omitempty"`
	// Modules are the modules to compose onto the base.
	Modules []TemplateBuilderComposeModule `json:"modules"`
	// OrganizationID owns the created template.
	OrganizationID uuid.UUID `json:"organization_id" format:"uuid" validate:"required"`
	// Name is the template's unique slug.
	Name string `json:"name" validate:"required,template_name"`
	// DisplayName is the human-facing template name.
	DisplayName string `json:"display_name,omitempty" validate:"template_display_name"`
	// Description is shown on the template page.
	Description string `json:"description,omitempty" validate:"lt=128"`
	// Icon is a URL or built-in icon path.
	Icon string `json:"icon,omitempty"`
	// ProvisionerTags route the import job to matching provisioners.
	ProvisionerTags map[string]string `json:"provisioner_tags,omitempty"`
}

// TemplateBuilderCreateTemplateResponse is the response body for
// POST /api/v2/templatebuilder/compose/template.
type TemplateBuilderCreateTemplateResponse struct {
	// Template is the newly created template.
	Template Template `json:"template"`
}

// TemplateBuilderSessionEventType enumerates the event types for
// template builder session telemetry.
type TemplateBuilderSessionEventType string

const (
	TemplateBuilderSessionEventWizardEntry       TemplateBuilderSessionEventType = "wizard_entry"
	TemplateBuilderSessionEventComposeCompletion TemplateBuilderSessionEventType = "compose_completion"
)

// TemplateBuilderSessionRequest is the request body for
// POST /api/v2/templatebuilder/sessions.
type TemplateBuilderSessionRequest struct {
	// SessionID correlates events within one builder session.
	SessionID uuid.UUID `json:"session_id" format:"uuid" validate:"required"`
	// EventType is the telemetry event being reported.
	EventType TemplateBuilderSessionEventType `json:"event_type" validate:"required,oneof=wizard_entry compose_completion"`
	// BaseTemplateID is the selected base, when known.
	BaseTemplateID string `json:"base_template_id,omitempty"`
	// ModuleIDs are the selected modules, when known.
	ModuleIDs []string `json:"module_ids,omitempty"`
	// DurationSeconds is the elapsed wizard time for the event.
	DurationSeconds float64 `json:"duration_seconds,omitempty"`
	// Success reports whether the composition succeeded.
	Success bool `json:"success,omitempty"`
}

// TemplateBuilderSession reports a template builder session event for
// telemetry purposes.
func (c *Client) TemplateBuilderSession(ctx context.Context, req TemplateBuilderSessionRequest) error {
	res, err := c.Request(ctx, http.MethodPost, "/api/v2/templatebuilder/sessions", req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

// TemplateBuilderCreateTemplate composes a template from a base and modules,
// validates it via a provisioner import job, and creates the template.
func (c *Client) TemplateBuilderCreateTemplate(ctx context.Context, req TemplateBuilderCreateTemplateRequest) (TemplateBuilderCreateTemplateResponse, error) {
	res, err := c.Request(ctx, http.MethodPost, "/api/v2/templatebuilder/compose/template", req)
	if err != nil {
		return TemplateBuilderCreateTemplateResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return TemplateBuilderCreateTemplateResponse{}, ReadBodyAsError(res)
	}
	var resp TemplateBuilderCreateTemplateResponse
	return resp, ReadBodyAsJSON(res, &resp)
}

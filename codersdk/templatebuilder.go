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

type TemplateBuilderModuleVariable struct {
	Name        string                      `json:"name"`
	Type        TemplateBuilderVariableType `json:"type"`
	Description string                      `json:"description"`
	Default     json.RawMessage             `json:"default,omitempty"`
	Required    bool                        `json:"required"`
	Sensitive   bool                        `json:"sensitive"`
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

// TemplateBuilderModulesResponse is the response body for listing template builder modules.
type TemplateBuilderModulesResponse struct {
	Modules []TemplateBuilderModule `json:"modules"`
}

// TemplateBuilderBase is the API response type for a base template
// returned by GET /api/v2/templatebuilder/bases.
type TemplateBuilderBase struct {
	ID            string                          `json:"id"`
	Name          string                          `json:"name"`
	Description   string                          `json:"description"`
	Icon          string                          `json:"icon"`
	OS            string                          `json:"os"`
	Variables     []TemplateBuilderModuleVariable `json:"variables"`
	Prerequisites string                          `json:"prerequisites"`
}

// TemplateBuilderBasesResponse is the response body for listing template builder bases.
type TemplateBuilderBasesResponse struct {
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
	return resp, json.NewDecoder(res.Body).Decode(&resp)
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
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// TemplateBuilderComposeRequest is the request body for
// POST /api/v2/templatebuilder/compose.
type TemplateBuilderComposeRequest struct {
	BaseTemplateID     string                         `json:"base_template_id"`
	BaseVariableValues map[string]string              `json:"base_variable_values,omitempty"`
	Modules            []TemplateBuilderComposeModule `json:"modules"`
}

// TemplateBuilderComposeModule identifies a module and its variable
// values for the compose request.
type TemplateBuilderComposeModule struct {
	ID        string            `json:"id"`
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
	BaseTemplateID     string                         `json:"base_template_id"`
	BaseVariableValues map[string]string              `json:"base_variable_values,omitempty"`
	Modules            []TemplateBuilderComposeModule `json:"modules"`
	OrganizationID     uuid.UUID                      `json:"organization_id" format:"uuid" validate:"required"`
	Name               string                         `json:"name" validate:"required,template_name"`
	DisplayName        string                         `json:"display_name,omitempty" validate:"template_display_name"`
	Description        string                         `json:"description,omitempty" validate:"lt=128"`
	Icon               string                         `json:"icon,omitempty"`
	ProvisionerTags    map[string]string              `json:"provisioner_tags,omitempty"`
}

// TemplateBuilderCreateTemplateResponse is the response body for
// POST /api/v2/templatebuilder/compose/template.
type TemplateBuilderCreateTemplateResponse struct {
	Template Template `json:"template"`
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
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

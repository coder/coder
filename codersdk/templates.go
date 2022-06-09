package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

// Template is the JSON representation of a Coder template. This type matches the
// database object for now, but is abstracted for ease of change later on.
type Template struct {
	ID                         uuid.UUID       `json:"id"`
	CreatedAt                  time.Time       `json:"created_at"`
	UpdatedAt                  time.Time       `json:"updated_at"`
	OrganizationID             uuid.UUID       `json:"organization_id"`
	Name                       string          `json:"name"`
	Provisioner                ProvisionerType `json:"provisioner"`
	ActiveVersionID            uuid.UUID       `json:"active_version_id"`
	WorkspaceOwnerCount        uint32          `json:"workspace_owner_count"`
	Description                string          `json:"description"`
	MaxTTLMillis               int64           `json:"max_ttl_ms"`
	MinAutostartIntervalMillis int64           `json:"min_autostart_interval_ms"`
}

type UpdateActiveTemplateVersion struct {
	ID uuid.UUID `json:"id" validate:"required"`
}

type UpdateTemplateMeta struct {
	Description                string `json:"description,omitempty"`
	MaxTTLMillis               int64  `json:"max_ttl_ms,omitempty"`
	MinAutostartIntervalMillis int64  `json:"min_autostart_interval_ms,omitempty"`
}

// Template returns a single template.
func (c *Client) Template(ctx context.Context, template uuid.UUID) (Template, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/templates/%s", template), nil)
	if err != nil {
		return Template{}, nil
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Template{}, readBodyAsError(res)
	}
	var resp Template
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

func (c *Client) DeleteTemplate(ctx context.Context, template uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodDelete, fmt.Sprintf("/api/v2/templates/%s", template), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return readBodyAsError(res)
	}
	return nil
}

func (c *Client) UpdateTemplateMeta(ctx context.Context, templateID uuid.UUID, req UpdateTemplateMeta) (Template, error) {
	res, err := c.Request(ctx, http.MethodPatch, fmt.Sprintf("/api/v2/templates/%s", templateID), req)
	if err != nil {
		return Template{}, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotModified {
		return Template{}, xerrors.New("template metadata not modified")
	}
	if res.StatusCode != http.StatusOK {
		return Template{}, readBodyAsError(res)
	}
	var updated Template
	return updated, json.NewDecoder(res.Body).Decode(&updated)
}

// UpdateActiveTemplateVersion updates the active template version to the ID provided.
// The template version must be attached to the template.
func (c *Client) UpdateActiveTemplateVersion(ctx context.Context, template uuid.UUID, req UpdateActiveTemplateVersion) error {
	res, err := c.Request(ctx, http.MethodPatch, fmt.Sprintf("/api/v2/templates/%s/versions", template), req)
	if err != nil {
		return nil
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return readBodyAsError(res)
	}
	return nil
}

// TemplateVersionsByTemplateRequest defines the request parameters for
// TemplateVersionsByTemplate.
type TemplateVersionsByTemplateRequest struct {
	TemplateID uuid.UUID `json:"template_id" validate:"required"`
	Pagination
}

// TemplateVersionsByTemplate lists versions associated with a template.
func (c *Client) TemplateVersionsByTemplate(ctx context.Context, req TemplateVersionsByTemplateRequest) ([]TemplateVersion, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/templates/%s/versions", req.TemplateID), nil, req.Pagination.asRequestOption())
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}
	var templateVersion []TemplateVersion
	return templateVersion, json.NewDecoder(res.Body).Decode(&templateVersion)
}

// TemplateVersionByName returns a template version by it's friendly name.
// This is used for path-based routing. Like: /templates/example/versions/helloworld
func (c *Client) TemplateVersionByName(ctx context.Context, template uuid.UUID, name string) (TemplateVersion, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/templates/%s/versions/%s", template, name), nil)
	if err != nil {
		return TemplateVersion{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return TemplateVersion{}, readBodyAsError(res)
	}
	var templateVersion TemplateVersion
	return templateVersion, json.NewDecoder(res.Body).Decode(&templateVersion)
}

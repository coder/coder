package codersdk

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// TemplateResources is the independently provisioned Terraform root of a template.
// Outputs contain only non-sensitive values. Raw Terraform state is never exposed.
type TemplateResources struct {
	TemplateID        uuid.UUID                  `json:"template_id"`
	Status            string                     `json:"status" enums:"unconfigured,pending,applied,failed,unknown,destroyed"`
	CurrentRevisionID *uuid.UUID                 `json:"current_revision_id"`
	AppliedRevisionID *uuid.UUID                 `json:"applied_revision_id"`
	Revisions         []TemplateResourceRevision `json:"revisions"`
	Runs              []TemplateResourceRun      `json:"runs"`
	Outputs           map[string]interface{}     `json:"outputs"`
}

// TemplateResourceRevision is an immutable source archive independent of workspace versions.
type TemplateResourceRevision struct {
	ID         uuid.UUID `json:"id"`
	TemplateID uuid.UUID `json:"template_id"`
	CreatedAt  time.Time `json:"created_at" format:"date-time"`
	// FileID identifies the complete, normalized TAR archive.
	FileID uuid.UUID `json:"file_id" format:"uuid"`
	// Source is the main.tf preview, or empty when that file is absent.
	Source string `json:"source"`
}

// TemplateResourceRun records one plan, apply, or destroy using a native provisioner job.
type TemplateResourceRun struct {
	ID         uuid.UUID      `json:"id"`
	RevisionID uuid.UUID      `json:"revision_id"`
	Action     string         `json:"action" enums:"plan,apply,destroy"`
	CreatedAt  time.Time      `json:"created_at" format:"date-time"`
	Job        ProvisionerJob `json:"job"`
}

// CreateTemplateResourceRevisionRequest provides either a complete uploaded
// archive or inline main.tf source. FileID and nonempty Source are mutually exclusive.
type CreateTemplateResourceRevisionRequest struct {
	FileID uuid.UUID `json:"file_id,omitempty" format:"uuid"`
	Source string    `json:"source,omitempty"`
}

// CreateTemplateResourceRunRequest selects an immutable revision and operation.
type CreateTemplateResourceRunRequest struct {
	RevisionID uuid.UUID `json:"revision_id"`
	Action     string    `json:"action" enums:"plan,apply,destroy"`
}

// TemplateResources fetches shared resources and their history.
func (c *Client) TemplateResources(ctx context.Context, templateID uuid.UUID) (TemplateResources, error) {
	var result TemplateResources
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/templates/%s/resources", templateID), nil)
	if err != nil {
		return result, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return result, ReadBodyAsError(res)
	}
	return result, ReadBodyAsJSON(res, &result)
}

// CreateTemplateResourceRevision saves a new immutable revision.
func (c *Client) CreateTemplateResourceRevision(ctx context.Context, templateID uuid.UUID, request CreateTemplateResourceRevisionRequest) (TemplateResourceRevision, error) {
	var result TemplateResourceRevision
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/templates/%s/resources/versions", templateID), request)
	if err != nil {
		return result, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return result, ReadBodyAsError(res)
	}
	return result, ReadBodyAsJSON(res, &result)
}

// CreateTemplateResourceRun enqueues a native Terraform job.
func (c *Client) CreateTemplateResourceRun(ctx context.Context, templateID uuid.UUID, request CreateTemplateResourceRunRequest) (TemplateResourceRun, error) {
	var result TemplateResourceRun
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/templates/%s/resources/runs", templateID), request)
	if err != nil {
		return result, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return result, ReadBodyAsError(res)
	}
	return result, ReadBodyAsJSON(res, &result)
}

// CancelTemplateResourceRun requests cancellation. Acquired runs retain their
// reservation until the worker or reaper establishes the infrastructure outcome.
func (c *Client) CancelTemplateResourceRun(ctx context.Context, templateID, runID uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodPatch, fmt.Sprintf("/api/v2/templates/%s/resources/runs/%s/cancel", templateID, runID), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ReadBodyAsError(res)
	}
	return nil
}

// TemplateResourceRunLogs fetches persisted logs after the supplied log ID.
func (c *Client) TemplateResourceRunLogs(ctx context.Context, templateID, runID uuid.UUID, after int64) ([]ProvisionerJobLog, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/templates/%s/resources/runs/%s/logs?after=%d", templateID, runID, after), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var result []ProvisionerJobLog
	return result, ReadBodyAsJSON(res, &result)
}

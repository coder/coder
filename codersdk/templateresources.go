package codersdk

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// Foundation is an independently provisioned, organization-scoped Terraform deployment.
// Outputs contain only non-sensitive values. Raw Terraform state is never exposed.
type Foundation struct {
	ID                uuid.UUID         `json:"id"`
	OrganizationID    uuid.UUID         `json:"organization_id"`
	Name              string            `json:"name"`
	ProvisionerTags   map[string]string `json:"provisioner_tags"`
	TemplateID        *uuid.UUID        `json:"template_id"`
	Status            string            `json:"status" enums:"unconfigured,pending,applied,failed,unknown,destroyed"`
	CurrentRevisionID *uuid.UUID        `json:"current_revision_id"`
	AppliedRevisionID *uuid.UUID        `json:"applied_revision_id"`
	// CheckpointRevisionID identifies the configuration used for retained state,
	// including a completed partial apply. Destroy uses this revision.
	CheckpointRevisionID *uuid.UUID                          `json:"checkpoint_revision_id"`
	Revisions            []TemplateResourceRevision          `json:"revisions"`
	Runs                 []TemplateResourceRun               `json:"runs"`
	Outputs              map[string]interface{}              `json:"outputs"`
	OutputMetadata       map[string]FoundationOutputMetadata `json:"output_metadata"`
	OutputSnapshotID     string                              `json:"output_snapshot_id"`
	// Resources is the managed inventory from retained Terraform state, not a
	// live infrastructure refresh. Unavailable state yields an empty inventory.
	Resources []FoundationResource `json:"resources"`
	// ResourcesAvailable distinguishes unreadable state from a known empty inventory.
	ResourcesAvailable bool `json:"resources_available"`
	// PreventDestroyWithWorkspaces blocks Destroy while any consumers remain.
	PreventDestroyWithWorkspaces bool                `json:"prevent_destroy_with_workspaces"`
	Consumers                    FoundationConsumers `json:"consumers"`
}

// FoundationConsumers counts all non-deleted consumers, including hidden workspaces.
// Running and stopped require a successful latest build; other states are unknown.
// Counts include workspaces with historical snapshots of this Foundation.
type FoundationConsumers struct {
	Total   int64 `json:"total"`
	Running int64 `json:"running"`
	Stopped int64 `json:"stopped"`
	Unknown int64 `json:"unknown"`
}

// FoundationOutputMetadata describes an output without exposing its value.
type FoundationOutputMetadata struct {
	Type      interface{} `json:"type,omitempty"`
	Sensitive bool        `json:"sensitive"`
}

// FoundationResource describes a managed resource without its state attributes.
type FoundationResource struct {
	Address   string `json:"address"`
	Type      string `json:"type"`
	Name      string `json:"name"`
	Instances int    `json:"instances"`
}

// CreateFoundationRequest configures a Foundation without provisioning infrastructure.
type CreateFoundationRequest struct {
	Name            string            `json:"name" validate:"required,template_name"`
	ProvisionerTags map[string]string `json:"provisioner_tags"`
	// PreventDestroyWithWorkspaces defaults to true when omitted.
	PreventDestroyWithWorkspaces *bool `json:"prevent_destroy_with_workspaces,omitempty"`
}

// UpdateFoundationRequest replaces a Foundation's settings, not its infrastructure.
type UpdateFoundationRequest struct {
	Name            string            `json:"name" validate:"required,template_name"`
	ProvisionerTags map[string]string `json:"provisioner_tags"`
	// PreventDestroyWithWorkspaces is preserved when omitted.
	PreventDestroyWithWorkspaces *bool `json:"prevent_destroy_with_workspaces,omitempty"`
}

// SetTemplateFoundationRequest associates an existing Foundation with a template.
type SetTemplateFoundationRequest struct {
	FoundationID uuid.UUID `json:"foundation_id" format:"uuid"`
}

// TemplateResourceRevision is an immutable Foundation source archive.
type TemplateResourceRevision struct {
	ID           uuid.UUID `json:"id"`
	FoundationID uuid.UUID `json:"foundation_id"`
	CreatedAt    time.Time `json:"created_at" format:"date-time"`
	// FileID identifies the complete, normalized TAR archive.
	FileID uuid.UUID `json:"file_id" format:"uuid"`
	// Source is the main.tf preview, or empty when that file is absent.
	Source string `json:"source"`
}

// TemplateResourceRun records one plan, apply, or destroy using a native provisioner job.
type TemplateResourceRun struct {
	ID         uuid.UUID              `json:"id"`
	RevisionID uuid.UUID              `json:"revision_id"`
	Action     string                 `json:"action" enums:"plan,apply,destroy"`
	CreatedAt  time.Time              `json:"created_at" format:"date-time"`
	Job        ProvisionerJob         `json:"job"`
	Preview    *FoundationPlanPreview `json:"preview,omitempty"`
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

func foundationPath(orgID, foundationID uuid.UUID) string {
	return fmt.Sprintf("/api/v2/organizations/%s/foundations/%s", orgID, foundationID)
}

func (c *Client) foundationRequest(ctx context.Context, method, path string, request interface{}, status int, result interface{}) error {
	res, err := c.Request(ctx, method, path, request)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != status {
		return ReadBodyAsError(res)
	}
	if result == nil {
		return nil
	}
	return ReadBodyAsJSON(res, result)
}

// Foundations lists accessible Foundations in an organization.
func (c *Client) Foundations(ctx context.Context, orgID uuid.UUID) ([]Foundation, error) {
	var result []Foundation
	err := c.foundationRequest(ctx, http.MethodGet, fmt.Sprintf("/api/v2/organizations/%s/foundations", orgID), nil, http.StatusOK, &result)
	return result, err
}

// CreateFoundation creates an unbound Foundation without provisioning infrastructure.
func (c *Client) CreateFoundation(ctx context.Context, orgID uuid.UUID, request CreateFoundationRequest) (Foundation, error) {
	var result Foundation
	err := c.foundationRequest(ctx, http.MethodPost, fmt.Sprintf("/api/v2/organizations/%s/foundations", orgID), request, http.StatusCreated, &result)
	return result, err
}

// Foundation fetches a Foundation and its history.
func (c *Client) Foundation(ctx context.Context, orgID, foundationID uuid.UUID) (Foundation, error) {
	var result Foundation
	err := c.foundationRequest(ctx, http.MethodGet, foundationPath(orgID, foundationID), nil, http.StatusOK, &result)
	return result, err
}

// UpdateFoundation replaces settings. Existing queued jobs retain their routing.
func (c *Client) UpdateFoundation(ctx context.Context, orgID, foundationID uuid.UUID, request UpdateFoundationRequest) (Foundation, error) {
	var result Foundation
	err := c.foundationRequest(ctx, http.MethodPatch, foundationPath(orgID, foundationID), request, http.StatusOK, &result)
	return result, err
}

// DeleteFoundation removes a Foundation after infrastructure cleanup.
func (c *Client) DeleteFoundation(ctx context.Context, orgID, foundationID uuid.UUID) error {
	return c.foundationRequest(ctx, http.MethodDelete, foundationPath(orgID, foundationID), nil, http.StatusOK, nil)
}

// TemplateFoundation returns the associated Foundation, or nil when unbound.
func (c *Client) TemplateFoundation(ctx context.Context, templateID uuid.UUID) (*Foundation, error) {
	var result *Foundation
	err := c.foundationRequest(ctx, http.MethodGet, fmt.Sprintf("/api/v2/templates/%s/foundation", templateID), nil, http.StatusOK, &result)
	return result, err
}

// SetTemplateFoundation associates a Foundation with a template in the same organization.
func (c *Client) SetTemplateFoundation(ctx context.Context, templateID uuid.UUID, request SetTemplateFoundationRequest) (Foundation, error) {
	var result Foundation
	err := c.foundationRequest(ctx, http.MethodPut, fmt.Sprintf("/api/v2/templates/%s/foundation", templateID), request, http.StatusOK, &result)
	return result, err
}

// CreateFoundationRevision saves a new immutable revision.
func (c *Client) CreateFoundationRevision(ctx context.Context, orgID, foundationID uuid.UUID, request CreateTemplateResourceRevisionRequest) (TemplateResourceRevision, error) {
	var result TemplateResourceRevision
	err := c.foundationRequest(ctx, http.MethodPost, foundationPath(orgID, foundationID)+"/versions", request, http.StatusCreated, &result)
	return result, err
}

// CreateFoundationRun enqueues a native Terraform job.
func (c *Client) CreateFoundationRun(ctx context.Context, orgID, foundationID uuid.UUID, request CreateTemplateResourceRunRequest) (TemplateResourceRun, error) {
	var result TemplateResourceRun
	err := c.foundationRequest(ctx, http.MethodPost, foundationPath(orgID, foundationID)+"/runs", request, http.StatusCreated, &result)
	return result, err
}

// CancelFoundationRun requests cancellation, retaining acquired mutation reservations.
func (c *Client) CancelFoundationRun(ctx context.Context, orgID, foundationID, runID uuid.UUID) error {
	return c.foundationRequest(ctx, http.MethodPatch, foundationPath(orgID, foundationID)+fmt.Sprintf("/runs/%s/cancel", runID), nil, http.StatusOK, nil)
}

// FoundationRunLogs fetches persisted logs after the supplied log ID.
func (c *Client) FoundationRunLogs(ctx context.Context, orgID, foundationID, runID uuid.UUID, after int64) ([]ProvisionerJobLog, error) {
	var result []ProvisionerJobLog
	err := c.foundationRequest(ctx, http.MethodGet, foundationPath(orgID, foundationID)+fmt.Sprintf("/runs/%s/logs?after=%d", runID, after), nil, http.StatusOK, &result)
	return result, err
}

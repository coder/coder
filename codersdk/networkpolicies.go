package codersdk

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/google/uuid"
)

// NetworkPolicy is an organization-scoped, independently provisioned policy.
type NetworkPolicy struct {
	ID                  uuid.UUID               `json:"id" format:"uuid"`
	OrganizationID      uuid.UUID               `json:"organization_id" format:"uuid"`
	Name                string                  `json:"name"`
	Description         string                  `json:"description"`
	Generation          int64                   `json:"generation"`
	CreatedAt           time.Time               `json:"created_at" format:"date-time"`
	UpdatedAt           time.Time               `json:"updated_at" format:"date-time"`
	DeletionRequestedAt *time.Time              `json:"deletion_requested_at" format:"date-time"`
	DeletedAt           *time.Time              `json:"deleted_at" format:"date-time"`
	Revisions           []NetworkPolicyRevision `json:"revisions"`
	TemplateIDs         []uuid.UUID             `json:"template_ids"`
	Targets             []NetworkPolicyTarget   `json:"targets"`
	Runs                []NetworkPolicyRun      `json:"runs"`
}

// NetworkPolicyRevision is an immutable HCL and trusted module snapshot.
type NetworkPolicyRevision struct {
	ID        uuid.UUID `json:"id" format:"uuid"`
	PolicyID  uuid.UUID `json:"policy_id" format:"uuid"`
	HCL       string    `json:"hcl"`
	CreatedAt time.Time `json:"created_at" format:"date-time"`
	CreatedBy uuid.UUID `json:"created_by" format:"uuid"`
}

// NetworkPolicyTarget identifies explicit infrastructure and provisioner routing.
type NetworkPolicyTarget struct {
	ID                uuid.UUID         `json:"id" format:"uuid"`
	PolicyID          uuid.UUID         `json:"policy_id" format:"uuid"`
	Name              string            `json:"name"`
	Cluster           string            `json:"cluster"`
	Context           string            `json:"context"`
	Namespace         string            `json:"namespace"`
	ProvisionerTags   map[string]string `json:"provisioner_tags"`
	DesiredRevisionID *uuid.UUID        `json:"desired_revision_id" format:"uuid"`
	AppliedRevisionID *uuid.UUID        `json:"applied_revision_id" format:"uuid"`
	Status            string            `json:"status"`
	Drifted           bool              `json:"drifted"`
	LastJobID         *uuid.UUID        `json:"last_job_id" format:"uuid"`
	LastError         string            `json:"last_error"`
	DestroyedAt       *time.Time        `json:"destroyed_at" format:"date-time"`
	CreatedAt         time.Time         `json:"created_at" format:"date-time"`
	UpdatedAt         time.Time         `json:"updated_at" format:"date-time"`
}

// NetworkPolicyOperation identifies an apply, drift plan, or destroy job.
type NetworkPolicyOperation string

const (
	NetworkPolicyOperationApply   NetworkPolicyOperation = "apply"
	NetworkPolicyOperationPlan    NetworkPolicyOperation = "plan"
	NetworkPolicyOperationDestroy NetworkPolicyOperation = "destroy"
)

// NetworkPolicyRun records a real provisioner job, not an optimistic apply result.
type NetworkPolicyRun struct {
	// TemplateIDs is the immutable desired attachment snapshot for this run.
	TemplateIDs []uuid.UUID            `json:"template_ids,omitempty"`
	ID          uuid.UUID              `json:"id" format:"uuid"`
	PolicyID    uuid.UUID              `json:"policy_id" format:"uuid"`
	TargetID    uuid.UUID              `json:"target_id" format:"uuid"`
	RevisionID  uuid.UUID              `json:"revision_id" format:"uuid"`
	JobID       uuid.UUID              `json:"job_id" format:"uuid"`
	Operation   NetworkPolicyOperation `json:"operation"`
	Status      string                 `json:"status"`
	Error       string                 `json:"error"`
	// Plan is a bounded text preview of persisted output for completed plan runs.
	// Use NetworkPolicyRunLogs for the complete output.
	Plan        string     `json:"plan"`
	CreatedAt   time.Time  `json:"created_at" format:"date-time"`
	CompletedAt *time.Time `json:"completed_at" format:"date-time"`
}

// CreateNetworkPolicyRequest creates a policy and its first revision.
type CreateNetworkPolicyRequest struct {
	Name        string      `json:"name" validate:"required"`
	Description string      `json:"description"`
	HCL         string      `json:"hcl" validate:"required"`
	TemplateIDs []uuid.UUID `json:"template_ids"`
}

// UpdateNetworkPolicyRequest changes policy metadata with optimistic concurrency.
type UpdateNetworkPolicyRequest struct {
	Name               string `json:"name" validate:"required"`
	Description        string `json:"description"`
	ExpectedGeneration int64  `json:"expected_generation"`
}

// DeleteNetworkPolicyRequest starts or retries asynchronous infrastructure deletion.
type DeleteNetworkPolicyRequest struct {
	ExpectedGeneration int64 `json:"expected_generation"`
}

// CreateNetworkPolicyRevisionRequest snapshots HCL and the configured module.
type CreateNetworkPolicyRevisionRequest struct {
	HCL                string `json:"hcl" validate:"required"`
	ExpectedGeneration int64  `json:"expected_generation"`
}

// SetNetworkPolicyTemplatesRequest replaces the desired attachment set and,
// when published, queues apply jobs on every target without rebuilding workspaces.
type SetNetworkPolicyTemplatesRequest struct {
	TemplateIDs        []uuid.UUID `json:"template_ids"`
	ExpectedGeneration int64       `json:"expected_generation"`
}

// CreateNetworkPolicyTargetRequest registers an explicit execution target.
type CreateNetworkPolicyTargetRequest struct {
	Name               string            `json:"name"`
	Cluster            string            `json:"cluster" validate:"required"`
	Context            string            `json:"context" validate:"required"`
	Namespace          string            `json:"namespace" validate:"required"`
	ProvisionerTags    map[string]string `json:"provisioner_tags"`
	ExpectedGeneration int64             `json:"expected_generation"`
}

// PublishNetworkPolicyRequest queues operations for explicit targets.
type PublishNetworkPolicyRequest struct {
	RevisionID         uuid.UUID              `json:"revision_id" format:"uuid"`
	TargetIDs          []uuid.UUID            `json:"target_ids"`
	Operation          NetworkPolicyOperation `json:"operation" enums:"apply,plan"`
	ExpectedGeneration int64                  `json:"expected_generation"`
}

func networkPoliciesPath(organization uuid.UUID) string {
	return fmt.Sprintf("/api/experimental/organizations/%s/network-policies", organization)
}

// NetworkPolicies lists policies in an organization.
func (c *Client) NetworkPolicies(ctx context.Context, organization uuid.UUID) ([]NetworkPolicy, error) {
	return networkPolicyRequest[[]NetworkPolicy](ctx, c, http.MethodGet, networkPoliciesPath(organization), nil, http.StatusOK)
}

// NetworkPolicyTemplates lists only templates the caller can administer.
func (c *Client) NetworkPolicyTemplates(ctx context.Context, organization uuid.UUID) ([]Template, error) {
	return networkPolicyRequest[[]Template](ctx, c, http.MethodGet, networkPoliciesPath(organization)+"/templates", nil, http.StatusOK)
}

// CreateNetworkPolicy creates a policy with an immutable initial revision.
func (c *Client) CreateNetworkPolicy(ctx context.Context, organization uuid.UUID, req CreateNetworkPolicyRequest) (NetworkPolicy, error) {
	return networkPolicyRequest[NetworkPolicy](ctx, c, http.MethodPost, networkPoliciesPath(organization), req, http.StatusCreated)
}

// NetworkPolicy retrieves revisions, attachments, targets, and run reports.
func (c *Client) NetworkPolicy(ctx context.Context, organization, policy uuid.UUID) (NetworkPolicy, error) {
	return networkPolicyRequest[NetworkPolicy](ctx, c, http.MethodGet, networkPoliciesPath(organization)+"/"+policy.String(), nil, http.StatusOK)
}

// UpdateNetworkPolicy updates policy metadata.
func (c *Client) UpdateNetworkPolicy(ctx context.Context, organization, policy uuid.UUID, req UpdateNetworkPolicyRequest) (NetworkPolicy, error) {
	return networkPolicyRequest[NetworkPolicy](ctx, c, http.MethodPatch, networkPoliciesPath(organization)+"/"+policy.String(), req, http.StatusOK)
}

// DeleteNetworkPolicy queues destruction and returns the retained policy record.
// DeletedAt is set only after every target is proven destroyed.
func (c *Client) DeleteNetworkPolicy(ctx context.Context, organization, policy uuid.UUID, req DeleteNetworkPolicyRequest) (NetworkPolicy, error) {
	return networkPolicyRequest[NetworkPolicy](ctx, c, http.MethodDelete, networkPoliciesPath(organization)+"/"+policy.String(), req, http.StatusAccepted, http.StatusOK)
}

// CreateNetworkPolicyRevision validates and saves an immutable revision.
func (c *Client) CreateNetworkPolicyRevision(ctx context.Context, organization, policy uuid.UUID, req CreateNetworkPolicyRevisionRequest) (NetworkPolicyRevision, error) {
	return networkPolicyRequest[NetworkPolicyRevision](ctx, c, http.MethodPost, networkPoliciesPath(organization)+"/"+policy.String()+"/revisions", req, http.StatusCreated)
}

// SetNetworkPolicyTemplates replaces a policy's attachments.
func (c *Client) SetNetworkPolicyTemplates(ctx context.Context, organization, policy uuid.UUID, req SetNetworkPolicyTemplatesRequest) (NetworkPolicy, error) {
	return networkPolicyRequest[NetworkPolicy](ctx, c, http.MethodPut, networkPoliciesPath(organization)+"/"+policy.String()+"/templates", req, http.StatusOK)
}

// CreateNetworkPolicyTarget registers an explicit target, without provisioning it.
func (c *Client) CreateNetworkPolicyTarget(ctx context.Context, organization, policy uuid.UUID, req CreateNetworkPolicyTargetRequest) (NetworkPolicyTarget, error) {
	return networkPolicyRequest[NetworkPolicyTarget](ctx, c, http.MethodPost, networkPoliciesPath(organization)+"/"+policy.String()+"/targets", req, http.StatusCreated)
}

// PublishNetworkPolicy queues provisioner jobs. Apply success is asynchronous.
func (c *Client) PublishNetworkPolicy(ctx context.Context, organization, policy uuid.UUID, req PublishNetworkPolicyRequest) ([]NetworkPolicyRun, error) {
	return networkPolicyRequest[[]NetworkPolicyRun](ctx, c, http.MethodPost, networkPoliciesPath(organization)+"/"+policy.String()+"/publications", req, http.StatusAccepted)
}

// NetworkPolicyRunLogs returns persistent provisioner output, including plans.
func (c *Client) NetworkPolicyRunLogs(ctx context.Context, organization, policy, run uuid.UUID, after int64) ([]ProvisionerJobLog, error) {
	path := fmt.Sprintf("%s/%s/runs/%s/logs?after=%d", networkPoliciesPath(organization), policy, run, after)
	return networkPolicyRequest[[]ProvisionerJobLog](ctx, c, http.MethodGet, path, nil, http.StatusOK)
}

func networkPolicyRequest[T any](ctx context.Context, c *Client, method, path string, body any, statuses ...int) (T, error) {
	var result T
	res, err := c.Request(ctx, method, path, body)
	if err != nil {
		return result, err
	}
	defer res.Body.Close()
	if !slices.Contains(statuses, res.StatusCode) {
		return result, ReadBodyAsError(res)
	}
	return result, ReadBodyAsJSON(res, &result)
}

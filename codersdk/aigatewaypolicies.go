package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/google/uuid"
)

// AIGatewayPolicyNameRegex mirrors the CHECK constraint on
// ai_gateway_policies.name: lowercase alphanumeric with hyphen separators.
var AIGatewayPolicyNameRegex = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// AIGatewayPolicyKind identifies a policy's role, which determines the Rego
// entrypoint rule it binds and the host applier that consumes its result.
type AIGatewayPolicyKind string

const (
	AIGatewayPolicyKindClassify  AIGatewayPolicyKind = "classify"
	AIGatewayPolicyKindRoute     AIGatewayPolicyKind = "route"
	AIGatewayPolicyKindDecide    AIGatewayPolicyKind = "decide"
	AIGatewayPolicyKindTransform AIGatewayPolicyKind = "transform"
)

// AIGatewayHook is the point in the request lifecycle where a policy runs.
type AIGatewayHook string

const (
	AIGatewayHookPreAuth AIGatewayHook = "pre_auth"
	AIGatewayHookPreReq  AIGatewayHook = "pre_req"
	AIGatewayHookPreTool AIGatewayHook = "pre_tool"
)

// AIGatewayFailMode controls behaviour when a policy cannot produce a result.
type AIGatewayFailMode string

const (
	AIGatewayFailModeOpen   AIGatewayFailMode = "fail_open"
	AIGatewayFailModeClosed AIGatewayFailMode = "fail_closed"
)

// AIGatewayPolicy is a reusable, versioned Rego policy.
type AIGatewayPolicy struct {
	ID              uuid.UUID                `json:"id" format:"uuid"`
	Name            string                   `json:"name"`
	DisplayName     string                   `json:"display_name"`
	Kind            AIGatewayPolicyKind      `json:"kind"`
	ActiveVersionID *uuid.UUID               `json:"active_version_id,omitempty" format:"uuid"`
	CreatedAt       time.Time                `json:"created_at" format:"date-time"`
	UpdatedAt       time.Time                `json:"updated_at" format:"date-time"`
	Versions        []AIGatewayPolicyVersion `json:"versions,omitempty"`
}

// AIGatewayPolicyVersion is an immutable snapshot of a policy's Rego text and
// its frozen schema bindings.
type AIGatewayPolicyVersion struct {
	ID                  uuid.UUID  `json:"id" format:"uuid"`
	PolicyID            uuid.UUID  `json:"policy_id" format:"uuid"`
	VersionNumber       int32      `json:"version_number"`
	Rego                string     `json:"rego"`
	InputSchemaVersion  int32      `json:"input_schema_version"`
	OutputSchemaVersion int32      `json:"output_schema_version"`
	Description         string     `json:"description"`
	CreatedAt           time.Time  `json:"created_at" format:"date-time"`
	CreatedBy           *uuid.UUID `json:"created_by,omitempty" format:"uuid"`
}

// CreateAIGatewayPolicyRequest creates a policy and its first version.
type CreateAIGatewayPolicyRequest struct {
	Name        string              `json:"name"`
	DisplayName string              `json:"display_name,omitempty"`
	Kind        AIGatewayPolicyKind `json:"kind"`
	Rego        string              `json:"rego"`
	Description string              `json:"description,omitempty"`
}

func (req CreateAIGatewayPolicyRequest) Validate() []ValidationError {
	var v []ValidationError
	if req.Name == "" {
		v = append(v, ValidationError{Field: "name", Detail: "name is required"})
	} else if !AIGatewayPolicyNameRegex.MatchString(req.Name) {
		v = append(v, ValidationError{Field: "name", Detail: fmt.Sprintf("name must match %s", AIGatewayPolicyNameRegex)})
	}
	v = append(v, validateAIGatewayPolicyKind(req.Kind)...)
	if req.Rego == "" {
		v = append(v, ValidationError{Field: "rego", Detail: "rego is required"})
	}
	return v
}

// CreateAIGatewayPolicyVersionRequest mints a new immutable version.
type CreateAIGatewayPolicyVersionRequest struct {
	Rego        string `json:"rego"`
	Description string `json:"description,omitempty"`
	// Activate sets the new version as the policy's active version.
	Activate bool `json:"activate"`
}

func (req CreateAIGatewayPolicyVersionRequest) Validate() []ValidationError {
	var v []ValidationError
	if req.Rego == "" {
		v = append(v, ValidationError{Field: "rego", Detail: "rego is required"})
	}
	return v
}

// UpdateAIGatewayPolicyRequest partially updates a policy parent. At least one
// field must be set.
type UpdateAIGatewayPolicyRequest struct {
	DisplayName     *string    `json:"display_name,omitempty"`
	ActiveVersionID *uuid.UUID `json:"active_version_id,omitempty" format:"uuid"`
}

func (req UpdateAIGatewayPolicyRequest) IsEmpty() bool {
	return req.DisplayName == nil && req.ActiveVersionID == nil
}

func validateAIGatewayPolicyKind(kind AIGatewayPolicyKind) []ValidationError {
	switch kind {
	case AIGatewayPolicyKindClassify, AIGatewayPolicyKindRoute,
		AIGatewayPolicyKindDecide, AIGatewayPolicyKindTransform:
		return nil
	case "":
		return []ValidationError{{Field: "kind", Detail: "kind is required"}}
	default:
		return []ValidationError{{Field: "kind", Detail: fmt.Sprintf("unsupported kind %q", kind)}}
	}
}

// AIGatewayPolicies lists all (non-deleted) policies.
func (c *Client) AIGatewayPolicies(ctx context.Context) ([]AIGatewayPolicy, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/aibridge/policies", nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var out []AIGatewayPolicy
	return out, json.NewDecoder(res.Body).Decode(&out)
}

// AIGatewayPolicy fetches a single policy (with its versions) by ID.
func (c *Client) AIGatewayPolicy(ctx context.Context, id uuid.UUID) (AIGatewayPolicy, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/aibridge/policies/%s", id), nil)
	if err != nil {
		return AIGatewayPolicy{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return AIGatewayPolicy{}, ReadBodyAsError(res)
	}
	var out AIGatewayPolicy
	return out, json.NewDecoder(res.Body).Decode(&out)
}

// CreateAIGatewayPolicy creates a policy and its first (active) version.
func (c *Client) CreateAIGatewayPolicy(ctx context.Context, req CreateAIGatewayPolicyRequest) (AIGatewayPolicy, error) {
	res, err := c.Request(ctx, http.MethodPost, "/api/v2/aibridge/policies", req)
	if err != nil {
		return AIGatewayPolicy{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return AIGatewayPolicy{}, ReadBodyAsError(res)
	}
	var out AIGatewayPolicy
	return out, json.NewDecoder(res.Body).Decode(&out)
}

// CreateAIGatewayPolicyVersion mints a new version of a policy.
func (c *Client) CreateAIGatewayPolicyVersion(ctx context.Context, id uuid.UUID, req CreateAIGatewayPolicyVersionRequest) (AIGatewayPolicyVersion, error) {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/aibridge/policies/%s/versions", id), req)
	if err != nil {
		return AIGatewayPolicyVersion{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return AIGatewayPolicyVersion{}, ReadBodyAsError(res)
	}
	var out AIGatewayPolicyVersion
	return out, json.NewDecoder(res.Body).Decode(&out)
}

// UpdateAIGatewayPolicy partially updates a policy.
func (c *Client) UpdateAIGatewayPolicy(ctx context.Context, id uuid.UUID, req UpdateAIGatewayPolicyRequest) (AIGatewayPolicy, error) {
	res, err := c.Request(ctx, http.MethodPatch, fmt.Sprintf("/api/v2/aibridge/policies/%s", id), req)
	if err != nil {
		return AIGatewayPolicy{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return AIGatewayPolicy{}, ReadBodyAsError(res)
	}
	var out AIGatewayPolicy
	return out, json.NewDecoder(res.Body).Decode(&out)
}

// DeleteAIGatewayPolicy soft-deletes a policy. It fails if the policy is still
// referenced by an active pipeline.
func (c *Client) DeleteAIGatewayPolicy(ctx context.Context, id uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodDelete, fmt.Sprintf("/api/v2/aibridge/policies/%s", id), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

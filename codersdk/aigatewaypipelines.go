package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// AIGatewayPipeline attaches at most one policy pipeline to a provider.
type AIGatewayPipeline struct {
	ID              uuid.UUID                 `json:"id" format:"uuid"`
	ProviderID      uuid.UUID                 `json:"provider_id" format:"uuid"`
	Enabled         bool                      `json:"enabled"`
	ActiveVersionID *uuid.UUID                `json:"active_version_id,omitempty" format:"uuid"`
	CreatedAt       time.Time                 `json:"created_at" format:"date-time"`
	UpdatedAt       time.Time                 `json:"updated_at" format:"date-time"`
	ActiveVersion   *AIGatewayPipelineVersion `json:"active_version,omitempty"`
	// LatestVersionID / LatestVersionNumber identify the pipeline's tip (most
	// recent) version. Under the explicit two-stage rollout, activating a policy
	// or guardrail mints a new pipeline version on the tip without promoting it,
	// so the tip can be ahead of the active (live) version. When LatestVersionID
	// differs from ActiveVersionID the pipeline has unpromoted changes (drift):
	// the operator can promote the tip to take them live.
	LatestVersionID     *uuid.UUID `json:"latest_version_id,omitempty" format:"uuid"`
	LatestVersionNumber int32      `json:"latest_version_number"`
	// LatestVersion is the tip version with its full membership (policies and
	// guardrails). Editing a pipeline must base the new version on the tip, not
	// the active version, so staged changes accumulate as one linear draft
	// lineage; basing an edit on the active version would silently drop members
	// added in an unpromoted draft.
	LatestVersion *AIGatewayPipelineVersion `json:"latest_version,omitempty"`
}

// HasUnpromotedChanges reports whether the pipeline's tip version is ahead of
// its active (live) version, i.e. minted-but-unpromoted drift exists.
func (p AIGatewayPipeline) HasUnpromotedChanges() bool {
	return p.LatestVersionID != nil &&
		(p.ActiveVersionID == nil || *p.LatestVersionID != *p.ActiveVersionID)
}

// AIGatewayPipelineVersion is an immutable composition snapshot.
type AIGatewayPipelineVersion struct {
	ID            uuid.UUID                    `json:"id" format:"uuid"`
	PipelineID    uuid.UUID                    `json:"pipeline_id" format:"uuid"`
	VersionNumber int32                        `json:"version_number"`
	CreatedAt     time.Time                    `json:"created_at" format:"date-time"`
	Policies      []AIGatewayPipelinePolicy    `json:"policies"`
	Guardrails    []AIGatewayPipelineGuardrail `json:"guardrails"`
}

// AIGatewayPipelineGuardrail is one guardrail member of a pipeline version: a
// pinned guardrail version bound to a hook with a mode and fail mode.
type AIGatewayPipelineGuardrail struct {
	GuardrailVersionID uuid.UUID              `json:"guardrail_version_id" format:"uuid"`
	Hook               AIGatewayHook          `json:"hook"`
	Mode               AIGatewayGuardrailMode `json:"mode"`
	FailMode           AIGatewayFailMode      `json:"fail_mode"`
	NetworkTimeoutMS   int32                  `json:"network_timeout_ms"`
	Enabled            bool                   `json:"enabled"`
}

// AIGatewayPipelineGuardrailRequest pins a guardrail version into a pipeline
// version at a hook. NetworkTimeoutMS defaults to 2000 and Enabled to true when
// omitted.
type AIGatewayPipelineGuardrailRequest struct {
	GuardrailVersionID uuid.UUID              `json:"guardrail_version_id" format:"uuid"`
	Hook               AIGatewayHook          `json:"hook"`
	Mode               AIGatewayGuardrailMode `json:"mode"`
	FailMode           AIGatewayFailMode      `json:"fail_mode"`
	NetworkTimeoutMS   *int32                 `json:"network_timeout_ms,omitempty"`
	Enabled            *bool                  `json:"enabled,omitempty"`
}

// AIGatewayPipelinePolicy is one member of a pipeline version: a pinned policy
// version bound to a hook with a fail mode.
type AIGatewayPipelinePolicy struct {
	PolicyVersionID uuid.UUID           `json:"policy_version_id" format:"uuid"`
	Hook            AIGatewayHook       `json:"hook"`
	Kind            AIGatewayPolicyKind `json:"kind"`
	FailMode        AIGatewayFailMode   `json:"fail_mode"`
	// Enabled disables this policy within this pipeline without disabling it
	// globally. Disabled members are excluded from the runtime snapshot.
	Enabled bool `json:"enabled"`
}

// AIGatewayPipelinePolicyRequest pins a policy version into a pipeline version
// at a hook. Kind is derived server-side from the policy version. Enabled
// defaults to true when omitted.
type AIGatewayPipelinePolicyRequest struct {
	PolicyVersionID uuid.UUID         `json:"policy_version_id" format:"uuid"`
	Hook            AIGatewayHook     `json:"hook"`
	FailMode        AIGatewayFailMode `json:"fail_mode"`
	Enabled         *bool             `json:"enabled,omitempty"`
}

// CreateAIGatewayPipelineRequest creates a pipeline for a provider and its
// first version.
type CreateAIGatewayPipelineRequest struct {
	ProviderID uuid.UUID                           `json:"provider_id" format:"uuid"`
	Enabled    bool                                `json:"enabled"`
	Policies   []AIGatewayPipelinePolicyRequest    `json:"policies"`
	Guardrails []AIGatewayPipelineGuardrailRequest `json:"guardrails,omitempty"`
}

func (req CreateAIGatewayPipelineRequest) Validate() []ValidationError {
	var v []ValidationError
	if req.ProviderID == uuid.Nil {
		v = append(v, ValidationError{Field: "provider_id", Detail: "provider_id is required"})
	}
	v = append(v, validateAIGatewayPipelinePolicies(req.Policies)...)
	v = append(v, validateAIGatewayPipelineGuardrails(req.Guardrails)...)
	return v
}

// CreateAIGatewayPipelineVersionRequest mints a new pipeline version.
type CreateAIGatewayPipelineVersionRequest struct {
	Policies   []AIGatewayPipelinePolicyRequest    `json:"policies"`
	Guardrails []AIGatewayPipelineGuardrailRequest `json:"guardrails,omitempty"`
	Activate   bool                                `json:"activate"`
}

func (req CreateAIGatewayPipelineVersionRequest) Validate() []ValidationError {
	v := validateAIGatewayPipelinePolicies(req.Policies)
	return append(v, validateAIGatewayPipelineGuardrails(req.Guardrails)...)
}

// UpdateAIGatewayPipelineRequest partially updates a pipeline parent.
type UpdateAIGatewayPipelineRequest struct {
	Enabled         *bool      `json:"enabled,omitempty"`
	ActiveVersionID *uuid.UUID `json:"active_version_id,omitempty" format:"uuid"`
}

func (req UpdateAIGatewayPipelineRequest) IsEmpty() bool {
	return req.Enabled == nil && req.ActiveVersionID == nil
}

// UpdateAIGatewayPipelineMemberRequest enables or disables a single member
// (policy or guardrail) of a pipeline's live (active) version in place. Unlike a
// composition edit, this does not mint a new pipeline version: enable/disable is
// a live pause control that takes effect immediately. Exactly one of
// PolicyVersionID or GuardrailVersionID must be set, identifying the member
// within the active version together with Hook.
type UpdateAIGatewayPipelineMemberRequest struct {
	PolicyVersionID    *uuid.UUID    `json:"policy_version_id,omitempty" format:"uuid"`
	GuardrailVersionID *uuid.UUID    `json:"guardrail_version_id,omitempty" format:"uuid"`
	Hook               AIGatewayHook `json:"hook"`
	Enabled            bool          `json:"enabled"`
}

func (req UpdateAIGatewayPipelineMemberRequest) Validate() []ValidationError {
	var v []ValidationError
	switch {
	case req.PolicyVersionID == nil && req.GuardrailVersionID == nil:
		v = append(v, ValidationError{Field: "policy_version_id", Detail: "one of policy_version_id or guardrail_version_id is required"})
	case req.PolicyVersionID != nil && req.GuardrailVersionID != nil:
		v = append(v, ValidationError{Field: "policy_version_id", Detail: "only one of policy_version_id or guardrail_version_id may be set"})
	}
	switch req.Hook {
	case AIGatewayHookPreAuth, AIGatewayHookPreReq, AIGatewayHookPreTool:
	default:
		v = append(v, ValidationError{Field: "hook", Detail: fmt.Sprintf("unsupported hook %q", req.Hook)})
	}
	return v
}

func validateAIGatewayPipelinePolicies(policies []AIGatewayPipelinePolicyRequest) []ValidationError {
	var v []ValidationError
	for i, p := range policies {
		if p.PolicyVersionID == uuid.Nil {
			v = append(v, ValidationError{Field: fmt.Sprintf("policies[%d].policy_version_id", i), Detail: "policy_version_id is required"})
		}
		switch p.Hook {
		case AIGatewayHookPreAuth, AIGatewayHookPreReq, AIGatewayHookPreTool:
		default:
			v = append(v, ValidationError{Field: fmt.Sprintf("policies[%d].hook", i), Detail: fmt.Sprintf("unsupported hook %q", p.Hook)})
		}
		switch p.FailMode {
		case AIGatewayFailModeOpen, AIGatewayFailModeClosed:
		case "":
			v = append(v, ValidationError{Field: fmt.Sprintf("policies[%d].fail_mode", i), Detail: "fail_mode is required"})
		default:
			v = append(v, ValidationError{Field: fmt.Sprintf("policies[%d].fail_mode", i), Detail: fmt.Sprintf("unsupported fail_mode %q", p.FailMode)})
		}
	}
	return v
}

func validateAIGatewayPipelineGuardrails(guardrails []AIGatewayPipelineGuardrailRequest) []ValidationError {
	var v []ValidationError
	for i, g := range guardrails {
		if g.GuardrailVersionID == uuid.Nil {
			v = append(v, ValidationError{Field: fmt.Sprintf("guardrails[%d].guardrail_version_id", i), Detail: "guardrail_version_id is required"})
		}
		// v1 wires only pre-req guardrails into the runtime.
		if g.Hook != AIGatewayHookPreReq {
			v = append(v, ValidationError{Field: fmt.Sprintf("guardrails[%d].hook", i), Detail: fmt.Sprintf("unsupported hook %q (only pre_req is supported)", g.Hook)})
		}
		switch g.Mode {
		case AIGatewayGuardrailModeAdvisory, AIGatewayGuardrailModeEnforcing:
		case "":
			v = append(v, ValidationError{Field: fmt.Sprintf("guardrails[%d].mode", i), Detail: "mode is required"})
		default:
			v = append(v, ValidationError{Field: fmt.Sprintf("guardrails[%d].mode", i), Detail: fmt.Sprintf("unsupported mode %q", g.Mode)})
		}
		switch g.FailMode {
		case AIGatewayFailModeOpen, AIGatewayFailModeClosed:
		case "":
			v = append(v, ValidationError{Field: fmt.Sprintf("guardrails[%d].fail_mode", i), Detail: "fail_mode is required"})
		default:
			v = append(v, ValidationError{Field: fmt.Sprintf("guardrails[%d].fail_mode", i), Detail: fmt.Sprintf("unsupported fail_mode %q", g.FailMode)})
		}
		if g.NetworkTimeoutMS != nil && *g.NetworkTimeoutMS <= 0 {
			v = append(v, ValidationError{Field: fmt.Sprintf("guardrails[%d].network_timeout_ms", i), Detail: "network_timeout_ms must be positive"})
		}
	}
	return v
}

// AIGatewayPipelines lists all (non-deleted) pipelines.
func (c *Client) AIGatewayPipelines(ctx context.Context) ([]AIGatewayPipeline, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/aibridge/pipelines", nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var out []AIGatewayPipeline
	return out, json.NewDecoder(res.Body).Decode(&out)
}

// AIGatewayPipeline fetches a single pipeline (with its active version) by ID.
func (c *Client) AIGatewayPipeline(ctx context.Context, id uuid.UUID) (AIGatewayPipeline, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/aibridge/pipelines/%s", id), nil)
	if err != nil {
		return AIGatewayPipeline{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return AIGatewayPipeline{}, ReadBodyAsError(res)
	}
	var out AIGatewayPipeline
	return out, json.NewDecoder(res.Body).Decode(&out)
}

// AIGatewayPipelineVersions lists a pipeline's versions, newest first. Used to
// surface pipeline version history and to choose a minted-but-unpromoted
// version to promote.
func (c *Client) AIGatewayPipelineVersions(ctx context.Context, id uuid.UUID) ([]AIGatewayPipelineVersion, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/aibridge/pipelines/%s/versions", id), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var out []AIGatewayPipelineVersion
	return out, json.NewDecoder(res.Body).Decode(&out)
}

// CreateAIGatewayPipeline creates a pipeline and its first (active) version.
func (c *Client) CreateAIGatewayPipeline(ctx context.Context, req CreateAIGatewayPipelineRequest) (AIGatewayPipeline, error) {
	res, err := c.Request(ctx, http.MethodPost, "/api/v2/aibridge/pipelines", req)
	if err != nil {
		return AIGatewayPipeline{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return AIGatewayPipeline{}, ReadBodyAsError(res)
	}
	var out AIGatewayPipeline
	return out, json.NewDecoder(res.Body).Decode(&out)
}

// CreateAIGatewayPipelineVersion mints a new version of a pipeline.
func (c *Client) CreateAIGatewayPipelineVersion(ctx context.Context, id uuid.UUID, req CreateAIGatewayPipelineVersionRequest) (AIGatewayPipelineVersion, error) {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/aibridge/pipelines/%s/versions", id), req)
	if err != nil {
		return AIGatewayPipelineVersion{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return AIGatewayPipelineVersion{}, ReadBodyAsError(res)
	}
	var out AIGatewayPipelineVersion
	return out, json.NewDecoder(res.Body).Decode(&out)
}

// UpdateAIGatewayPipeline partially updates a pipeline.
func (c *Client) UpdateAIGatewayPipeline(ctx context.Context, id uuid.UUID, req UpdateAIGatewayPipelineRequest) (AIGatewayPipeline, error) {
	res, err := c.Request(ctx, http.MethodPatch, fmt.Sprintf("/api/v2/aibridge/pipelines/%s", id), req)
	if err != nil {
		return AIGatewayPipeline{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return AIGatewayPipeline{}, ReadBodyAsError(res)
	}
	var out AIGatewayPipeline
	return out, json.NewDecoder(res.Body).Decode(&out)
}

// UpdateAIGatewayPipelineMember enables or disables a single member of a
// pipeline's active version in place, without minting a new version.
func (c *Client) UpdateAIGatewayPipelineMember(ctx context.Context, id uuid.UUID, req UpdateAIGatewayPipelineMemberRequest) (AIGatewayPipeline, error) {
	res, err := c.Request(ctx, http.MethodPatch, fmt.Sprintf("/api/v2/aibridge/pipelines/%s/members", id), req)
	if err != nil {
		return AIGatewayPipeline{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return AIGatewayPipeline{}, ReadBodyAsError(res)
	}
	var out AIGatewayPipeline
	return out, json.NewDecoder(res.Body).Decode(&out)
}

// DeleteAIGatewayPipeline soft-deletes a pipeline.
func (c *Client) DeleteAIGatewayPipeline(ctx context.Context, id uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodDelete, fmt.Sprintf("/api/v2/aibridge/pipelines/%s", id), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

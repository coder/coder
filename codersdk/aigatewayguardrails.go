package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// AIGatewayGuardrail is a reusable, versioned networked guardrail.
type AIGatewayGuardrail struct {
	ID              uuid.UUID                   `json:"id" format:"uuid"`
	Name            string                      `json:"name"`
	DisplayName     string                      `json:"display_name"`
	AdapterType     string                      `json:"adapter_type"`
	ActiveVersionID *uuid.UUID                  `json:"active_version_id,omitempty" format:"uuid"`
	Enabled         bool                        `json:"enabled"`
	CreatedAt       time.Time                   `json:"created_at" format:"date-time"`
	UpdatedAt       time.Time                   `json:"updated_at" format:"date-time"`
	Versions        []AIGatewayGuardrailVersion `json:"versions,omitempty"`
}

// AIGatewayGuardrailVersion is an immutable snapshot of a guardrail's adapter
// config. The credential is write-only and never serialized back; HasCredential
// reports whether a secret is stored.
type AIGatewayGuardrailVersion struct {
	ID            uuid.UUID       `json:"id" format:"uuid"`
	GuardrailID   uuid.UUID       `json:"guardrail_id" format:"uuid"`
	VersionNumber int32           `json:"version_number"`
	Config        json.RawMessage `json:"config"`
	HasCredential bool            `json:"has_credential"`
	Description   string          `json:"description"`
	CreatedAt     time.Time       `json:"created_at" format:"date-time"`
	CreatedBy     *uuid.UUID      `json:"created_by,omitempty" format:"uuid"`
}

// CreateAIGatewayGuardrailRequest creates a guardrail and its first version.
type CreateAIGatewayGuardrailRequest struct {
	Name        string          `json:"name"`
	DisplayName string          `json:"display_name,omitempty"`
	AdapterType string          `json:"adapter_type"`
	Config      json.RawMessage `json:"config"`
	// Credential is the secret (e.g. an API key) stored encrypted; write-only.
	Credential  string `json:"credential,omitempty"`
	Description string `json:"description,omitempty"`
}

func (req CreateAIGatewayGuardrailRequest) Validate() []ValidationError {
	var v []ValidationError
	if req.Name == "" {
		v = append(v, ValidationError{Field: "name", Detail: "name is required"})
	} else if !AIGatewayPolicyNameRegex.MatchString(req.Name) {
		v = append(v, ValidationError{Field: "name", Detail: fmt.Sprintf("name must match %s", AIGatewayPolicyNameRegex)})
	}
	if req.AdapterType == "" {
		v = append(v, ValidationError{Field: "adapter_type", Detail: "adapter_type is required"})
	}
	if len(req.Config) == 0 {
		v = append(v, ValidationError{Field: "config", Detail: "config is required"})
	}
	return v
}

// CreateAIGatewayGuardrailVersionRequest mints a new immutable version.
type CreateAIGatewayGuardrailVersionRequest struct {
	Config      json.RawMessage `json:"config"`
	Credential  string          `json:"credential,omitempty"`
	Description string          `json:"description,omitempty"`
	// Activate sets the new version as the guardrail's active version.
	// Activation propagates to referencing pipelines by minting (not promoting)
	// new pipeline versions on their tip; live posture is unchanged until
	// promotion. Defaults false.
	Activate bool `json:"activate"`
	// Promote, only meaningful with Activate, additionally activates the minted
	// pipeline versions so the change goes live immediately. Defaults false.
	Promote bool `json:"promote,omitempty"`
}

func (req CreateAIGatewayGuardrailVersionRequest) Validate() []ValidationError {
	var v []ValidationError
	if len(req.Config) == 0 {
		v = append(v, ValidationError{Field: "config", Detail: "config is required"})
	}
	return v
}

// UpdateAIGatewayGuardrailRequest partially updates a guardrail parent. At least
// one field must be set.
type UpdateAIGatewayGuardrailRequest struct {
	DisplayName     *string    `json:"display_name,omitempty"`
	Enabled         *bool      `json:"enabled,omitempty"`
	ActiveVersionID *uuid.UUID `json:"active_version_id,omitempty" format:"uuid"`
	// Promote, only meaningful with ActiveVersionID, additionally activates the
	// pipeline versions minted by propagation so the activation goes live
	// immediately. Defaults false.
	Promote bool `json:"promote,omitempty"`
}

func (req UpdateAIGatewayGuardrailRequest) IsEmpty() bool {
	return req.DisplayName == nil && req.Enabled == nil && req.ActiveVersionID == nil
}

// AIGatewayGuardrails lists all (non-deleted) guardrails.
func (c *Client) AIGatewayGuardrails(ctx context.Context) ([]AIGatewayGuardrail, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/aibridge/guardrails", nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var out []AIGatewayGuardrail
	return out, json.NewDecoder(res.Body).Decode(&out)
}

// AIGatewayGuardrail fetches a single guardrail (with its versions) by ID.
func (c *Client) AIGatewayGuardrail(ctx context.Context, id uuid.UUID) (AIGatewayGuardrail, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/aibridge/guardrails/%s", id), nil)
	if err != nil {
		return AIGatewayGuardrail{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return AIGatewayGuardrail{}, ReadBodyAsError(res)
	}
	var out AIGatewayGuardrail
	return out, json.NewDecoder(res.Body).Decode(&out)
}

// CreateAIGatewayGuardrail creates a guardrail and its first (active) version.
func (c *Client) CreateAIGatewayGuardrail(ctx context.Context, req CreateAIGatewayGuardrailRequest) (AIGatewayGuardrail, error) {
	res, err := c.Request(ctx, http.MethodPost, "/api/v2/aibridge/guardrails", req)
	if err != nil {
		return AIGatewayGuardrail{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return AIGatewayGuardrail{}, ReadBodyAsError(res)
	}
	var out AIGatewayGuardrail
	return out, json.NewDecoder(res.Body).Decode(&out)
}

// CreateAIGatewayGuardrailVersion mints a new version of a guardrail.
func (c *Client) CreateAIGatewayGuardrailVersion(ctx context.Context, id uuid.UUID, req CreateAIGatewayGuardrailVersionRequest) (AIGatewayGuardrailVersion, error) {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/aibridge/guardrails/%s/versions", id), req)
	if err != nil {
		return AIGatewayGuardrailVersion{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return AIGatewayGuardrailVersion{}, ReadBodyAsError(res)
	}
	var out AIGatewayGuardrailVersion
	return out, json.NewDecoder(res.Body).Decode(&out)
}

// UpdateAIGatewayGuardrail partially updates a guardrail.
func (c *Client) UpdateAIGatewayGuardrail(ctx context.Context, id uuid.UUID, req UpdateAIGatewayGuardrailRequest) (AIGatewayGuardrail, error) {
	res, err := c.Request(ctx, http.MethodPatch, fmt.Sprintf("/api/v2/aibridge/guardrails/%s", id), req)
	if err != nil {
		return AIGatewayGuardrail{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return AIGatewayGuardrail{}, ReadBodyAsError(res)
	}
	var out AIGatewayGuardrail
	return out, json.NewDecoder(res.Body).Decode(&out)
}

// DeleteAIGatewayGuardrail soft-deletes a guardrail. It fails if the guardrail
// is still referenced by an active pipeline.
func (c *Client) DeleteAIGatewayGuardrail(ctx context.Context, id uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodDelete, fmt.Sprintf("/api/v2/aibridge/guardrails/%s", id), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

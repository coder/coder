package codersdk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

// AIProviderNameRegex mirrors the CHECK constraint on ai_providers.name.
// Provider names are lowercase alphanumeric with hyphen separators so
// they are safe in URLs.
var AIProviderNameRegex = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// AIProviderType identifies the protocol Coder uses to communicate
// with an upstream AI provider.
type AIProviderType string

const (
	AIProviderTypeOpenAI    AIProviderType = "openai"
	AIProviderTypeAnthropic AIProviderType = "anthropic"
	// AIProviderTypeAzure, AIProviderTypeGoogle, AIProviderTypeOpenAICompat,
	// AIProviderTypeOpenrouter, and AIProviderTypeVercel route through
	// aibridge's OpenAI client today because chatd configures these
	// providers against their OpenAI-compatible endpoints. Native
	// gateway-side support arrives later without an enum change.
	AIProviderTypeAzure        AIProviderType = "azure"
	AIProviderTypeGoogle       AIProviderType = "google"
	AIProviderTypeOpenAICompat AIProviderType = "openai-compat"
	AIProviderTypeOpenrouter   AIProviderType = "openrouter"
	AIProviderTypeVercel       AIProviderType = "vercel"
	// AIProviderTypeBedrock routes through aibridge's Anthropic client
	// using the Bedrock discriminator in Settings; native support is
	// future work.
	AIProviderTypeBedrock AIProviderType = "bedrock"
	// AIProviderTypeCopilot routes through aibridge's Copilot client,
	// which uses request-time GitHub OAuth tokens rather than pre-shared
	// API keys.
	AIProviderTypeCopilot AIProviderType = "copilot"
)

// AgentsUnsupportedProviderType is an AIProviderType the Coder Agents harness
// cannot use. Declaring these as an enum exposes the generated
// AgentsUnsupportedProviderTypes list to the frontend, which labels these
// providers without a per-provider field on the AIProvider response.
type AgentsUnsupportedProviderType string

const (
	// AgentsUnsupportedProviderTypeCopilot is GitHub Copilot: it authenticates
	// with a per-request token only an official Copilot client can mint, which
	// the server-side harness is not.
	AgentsUnsupportedProviderTypeCopilot AgentsUnsupportedProviderType = AgentsUnsupportedProviderType(AIProviderTypeCopilot)
)

// IsAgentsUnsupportedProviderType reports whether the Coder Agents harness
// cannot use the provider type. It is the single source of truth, shared by
// the chatd catalog predicate and, via the generated
// AgentsUnsupportedProviderTypes list, the frontend.
func IsAgentsUnsupportedProviderType(t AIProviderType) bool {
	switch AgentsUnsupportedProviderType(t) {
	case AgentsUnsupportedProviderTypeCopilot:
		return true
	default:
		return false
	}
}

// AIProviderSettings is the discriminated container for type-specific
// provider settings stored in ai_providers.settings. Providers that
// need no type-specific configuration (current OpenAI and standard
// Anthropic flows) leave every field nil; the wire form for those
// providers is JSON null.
//
// On the wire, settings serialize as a JSON object that always carries
// _type and _version discriminator keys alongside the type-specific
// fields. The custom (Un)MarshalJSON implementations on this type
// handle the routing automatically; callers should never marshal the
// concrete settings struct directly.
type AIProviderSettings struct {
	// Bedrock, when set, indicates this provider authenticates against
	// AWS Bedrock instead of api.anthropic.com. Only meaningful for
	// AIProviderTypeAnthropic.
	Bedrock *AIProviderBedrockSettings `json:"-"`
}

// IsZero reports whether the settings carry no type-specific data.
func (s AIProviderSettings) IsZero() bool {
	return s.Bedrock == nil
}

// MarshalJSON emits the discriminated wire form. Empty settings encode
// as JSON null so the column round-trips cleanly through SQL NULL.
func (s AIProviderSettings) MarshalJSON() ([]byte, error) {
	switch {
	case s.Bedrock != nil:
		return marshalSettings(*s.Bedrock)
	default:
		return []byte("null"), nil
	}
}

// UnmarshalJSON inspects the _type discriminator and routes to the
// concrete settings struct that matches it.
func (s *AIProviderSettings) UnmarshalJSON(data []byte) error {
	*s = AIProviderSettings{}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}
	var header aiProviderSettingsHeader
	if err := json.Unmarshal(data, &header); err != nil {
		return xerrors.Errorf("decode settings header: %w", err)
	}
	if header.Type == "" {
		return xerrors.New("settings missing _type discriminator")
	}
	switch header.Type {
	case AIProviderSettingsTypeBedrock:
		// TODO: handle multiple versions; this will be implemented
		// once needed.
		if header.Version != AIProviderBedrockSettingsVersion {
			return xerrors.Errorf("unsupported %q settings version %d (expected %d)",
				header.Type, header.Version, AIProviderBedrockSettingsVersion)
		}
		var b AIProviderBedrockSettings
		if err := json.Unmarshal(data, &b); err != nil {
			return xerrors.Errorf("decode bedrock settings: %w", err)
		}
		s.Bedrock = &b
		return nil
	default:
		return xerrors.Errorf("unknown settings type %q", header.Type)
	}
}

// aiProviderSettingsHeader is the discriminator-only view of an
// encoded settings blob.
type aiProviderSettingsHeader struct {
	Type    string `json:"_type"`
	Version int    `json:"_version"`
}

// settingsTyped is implemented by concrete settings structs so that
// marshalSettings can inject the discriminator without type-asserting
// against every variant.
type settingsTyped interface {
	settingsType() string
	settingsVersion() int
}

// marshalSettings encodes a concrete settings struct and merges the
// _type and _version discriminator keys at the top level of the
// resulting JSON object.
func marshalSettings(s settingsTyped) ([]byte, error) {
	raw, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	if m == nil {
		m = make(map[string]json.RawMessage)
	}
	typeRaw, err := json.Marshal(s.settingsType())
	if err != nil {
		return nil, err
	}
	versRaw, err := json.Marshal(s.settingsVersion())
	if err != nil {
		return nil, err
	}
	m["_type"] = typeRaw
	m["_version"] = versRaw
	return json.Marshal(m)
}

// AIProvider represents an AI provider configuration row as returned
// by the API. Each APIKey entry carries the row's ID so callers can
// reference it in an UpdateAIProviderRequest; the plaintext value is
// never echoed back (see AIProviderKey.Masked). Secret fields on
// Settings are never included in responses.
type AIProvider struct {
	ID          uuid.UUID          `json:"id" format:"uuid"`
	Type        AIProviderType     `json:"type"`
	Name        string             `json:"name"`
	DisplayName string             `json:"display_name"`
	Icon        string             `json:"icon"`
	Enabled     bool               `json:"enabled"`
	BaseURL     string             `json:"base_url"`
	APIKeys     []AIProviderKey    `json:"api_keys"`
	Settings    AIProviderSettings `json:"settings"`
	CreatedAt   time.Time          `json:"created_at" format:"date-time"`
	UpdatedAt   time.Time          `json:"updated_at" format:"date-time"`
}

// AIProviderKey is a single API key registered on a provider. The
// plaintext is never returned; Masked is a one-way rendering safe for
// display (see aibridge utils MaskSecret). ID lets clients reference
// the row in an UpdateAIProviderRequest without re-sending plaintext.
type AIProviderKey struct {
	ID        uuid.UUID `json:"id" format:"uuid"`
	Masked    string    `json:"masked"`
	CreatedAt time.Time `json:"created_at" format:"date-time"`
}

// CreateAIProviderRequest is the payload for creating a new AI
// provider. Name and Type are required. APIKeys carries the plaintext
// keys for OpenAI/Anthropic providers; Bedrock and Copilot providers
// must omit APIKeys (Bedrock authenticates via Settings, Copilot via
// request-time GitHub OAuth tokens).
type CreateAIProviderRequest struct {
	Type        AIProviderType     `json:"type"`
	Name        string             `json:"name"`
	DisplayName string             `json:"display_name,omitempty"`
	Icon        string             `json:"icon,omitempty"`
	Enabled     bool               `json:"enabled"`
	BaseURL     string             `json:"base_url"`
	APIKeys     []string           `json:"api_keys,omitempty"`
	Settings    AIProviderSettings `json:"settings,omitzero"`
}

// Validate returns the field-level validation errors for a create
// request. An empty slice indicates the request is valid.
func (req CreateAIProviderRequest) Validate() []ValidationError {
	var validations []ValidationError
	switch req.Type {
	case AIProviderTypeOpenAI,
		AIProviderTypeAnthropic,
		AIProviderTypeAzure,
		AIProviderTypeBedrock,
		AIProviderTypeCopilot,
		AIProviderTypeGoogle,
		AIProviderTypeOpenAICompat,
		AIProviderTypeOpenrouter,
		AIProviderTypeVercel:
	case "":
		validations = append(validations, ValidationError{Field: "type", Detail: "type is required"})
	default:
		validations = append(validations, ValidationError{
			Field:  "type",
			Detail: fmt.Sprintf("unsupported provider type %q", req.Type),
		})
	}
	validations = append(validations, validateAIProviderName(req.Name)...)
	validations = append(validations, validateRequiredAIProviderBaseURL(req.BaseURL)...)
	validations = append(validations, validateAIProviderAPIKeys(req.APIKeys)...)
	if req.Settings.Bedrock != nil &&
		req.Type != AIProviderTypeAnthropic &&
		req.Type != AIProviderTypeBedrock {
		validations = append(validations, ValidationError{
			Field:  "settings",
			Detail: "bedrock settings are only valid for type=anthropic or type=bedrock",
		})
	}
	if req.Type == AIProviderTypeBedrock && (req.Settings.Bedrock == nil || !req.Settings.Bedrock.IsConfigured()) {
		validations = append(validations, ValidationError{
			Field:  "settings",
			Detail: "type=bedrock requires bedrock settings",
		})
	}
	if req.Type == AIProviderTypeBedrock && len(req.APIKeys) > 0 {
		validations = append(validations, ValidationError{
			Field:  "api_keys",
			Detail: "type=bedrock does not accept api_keys",
		})
	}
	if req.Settings.Bedrock != nil {
		validations = append(validations, validateAIProviderRoleARN(req.Settings.Bedrock.RoleARN)...)
		if req.Settings.Bedrock.ExternalID != "" {
			validations = append(validations, ValidationError{
				Field:  "settings.external_id",
				Detail: "external_id is server-generated and cannot be set",
			})
		}
	}
	if req.Type == AIProviderTypeCopilot && len(req.APIKeys) > 0 {
		validations = append(validations, ValidationError{
			Field:  "api_keys",
			Detail: "type=copilot does not accept api_keys",
		})
	}
	return validations
}

// UpdateAIProviderRequest is the payload for partially updating an
// AI provider. At least one field must be non-nil. Pointer fields
// distinguish "not sent" (nil) from "set to empty/zero" (a pointer
// to the zero value). When APIKeys is non-nil, the supplied list
// describes the post-patch state of the key set; see
// AIProviderKeyMutation for the per-entry semantics. An empty slice
// clears all keys.
type UpdateAIProviderRequest struct {
	DisplayName *string                  `json:"display_name,omitempty"`
	Icon        *string                  `json:"icon,omitempty"`
	Enabled     *bool                    `json:"enabled,omitempty"`
	BaseURL     *string                  `json:"base_url,omitempty"`
	APIKeys     *[]AIProviderKeyMutation `json:"api_keys,omitempty"`
	Settings    *AIProviderSettings      `json:"settings,omitempty"`
}

// AIProviderKeyMutation describes the intended state of a single key
// in an UpdateAIProviderRequest. Exactly one of ID or APIKey must be
// set:
//
//   - ID set, APIKey nil: keep this existing key (matched by ID).
//   - ID nil, APIKey set: insert this new plaintext as a new key.
//
// Any existing key whose ID is absent from the request is deleted.
type AIProviderKeyMutation struct {
	ID     *uuid.UUID `json:"id,omitempty" format:"uuid"`
	APIKey *string    `json:"api_key,omitempty"`
}

// Validate returns the field-level validation errors for an update
// request. An empty slice indicates the request is valid. Callers
// should reject empty patches with IsEmpty before invoking Validate.
func (req UpdateAIProviderRequest) Validate() []ValidationError {
	var validations []ValidationError
	if req.BaseURL != nil {
		validations = append(validations, validateRequiredAIProviderBaseURL(*req.BaseURL)...)
	}
	if req.APIKeys != nil {
		validations = append(validations, validateAIProviderKeyMutations(*req.APIKeys)...)
	}
	if req.Settings != nil && req.Settings.Bedrock != nil {
		validations = append(validations, validateAIProviderRoleARN(req.Settings.Bedrock.RoleARN)...)
	}
	return validations
}

// IsEmpty reports whether the patch carries no fields.
func (req UpdateAIProviderRequest) IsEmpty() bool {
	return req.DisplayName == nil && req.Icon == nil && req.Enabled == nil && req.BaseURL == nil && req.APIKeys == nil && req.Settings == nil
}

func validateAIProviderName(name string) []ValidationError {
	var validations []ValidationError
	switch {
	case name == "":
		validations = append(validations, ValidationError{Field: "name", Detail: "name is required"})
	case !AIProviderNameRegex.MatchString(name):
		validations = append(validations, ValidationError{
			Field:  "name",
			Detail: fmt.Sprintf("name must match %s (lowercase alphanumeric, hyphens between words)", AIProviderNameRegex),
		})
	}
	return validations
}

func validateAIProviderRoleARN(roleARN string) []ValidationError {
	if roleARN == "" {
		return nil
	}
	const exampleRoleARN = "arn:aws:iam::123456789012:role/BedrockRole"
	invalid := func(detail string) []ValidationError {
		return []ValidationError{{Field: "settings.role_arn", Detail: detail}}
	}
	parsed, err := arn.Parse(roleARN)
	if err != nil {
		return invalid(fmt.Sprintf("role_arn %q is not a valid ARN, e.g. %s", roleARN, exampleRoleARN))
	}
	if parsed.Service != "iam" {
		return invalid(fmt.Sprintf("role_arn must be an IAM ARN, but resolved to service %q, e.g. %s", parsed.Service, exampleRoleARN))
	}
	if !strings.HasPrefix(parsed.Resource, "role/") {
		return invalid(fmt.Sprintf("role_arn must reference an IAM role, but resolved to resource %q, e.g. %s", parsed.Resource, exampleRoleARN))
	}
	return nil
}

func validateRequiredAIProviderBaseURL(raw string) []ValidationError {
	if raw == "" {
		return []ValidationError{{Field: "base_url", Detail: "base_url is required"}}
	}
	return validateAIProviderBaseURL(raw)
}

func validateAIProviderBaseURL(raw string) []ValidationError {
	var validations []ValidationError
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		validations = append(validations, ValidationError{
			Field:  "base_url",
			Detail: "base_url must be an absolute URL (e.g. https://api.example.com/)",
		})
		return validations
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		validations = append(validations, ValidationError{
			Field:  "base_url",
			Detail: fmt.Sprintf("base_url scheme must be http or https, got %q", parsed.Scheme),
		})
	}
	return validations
}

// validateAIProviderAPIKeys checks that each supplied key is non-empty
// and free of leading/trailing whitespace. An empty slice itself is
// permitted: on create it means "no keys yet"; on update it means
// "clear all keys". Keys are stored verbatim; surrounding whitespace
// would silently corrupt the credential, so callers must trim before
// sending.
func validateAIProviderAPIKeys(keys []string) []ValidationError {
	var validations []ValidationError
	for i, key := range keys {
		switch {
		case key == "":
			validations = append(validations, ValidationError{
				Field:  fmt.Sprintf("api_keys[%d]", i),
				Detail: "api_keys entries must not be empty",
			})
		case strings.TrimSpace(key) != key:
			validations = append(validations, ValidationError{
				Field:  fmt.Sprintf("api_keys[%d]", i),
				Detail: "api_keys entries must not contain leading or trailing whitespace",
			})
		}
	}
	return validations
}

// validateAIProviderKeyMutations checks each entry has exactly one of
// ID or APIKey set, that plaintexts are non-empty after trimming, and
// that no ID is referenced twice in the same request. An empty slice
// itself is permitted (it clears all keys).
func validateAIProviderKeyMutations(muts []AIProviderKeyMutation) []ValidationError {
	var validations []ValidationError
	seen := make(map[uuid.UUID]int, len(muts))
	for i, m := range muts {
		hasID := m.ID != nil
		hasKey := m.APIKey != nil
		switch {
		case hasID == hasKey:
			validations = append(validations, ValidationError{
				Field:  fmt.Sprintf("api_keys[%d]", i),
				Detail: "exactly one of id or api_key must be set",
			})
		case hasKey && *m.APIKey == "":
			validations = append(validations, ValidationError{
				Field:  fmt.Sprintf("api_keys[%d].api_key", i),
				Detail: "api_key must not be empty",
			})
		case hasKey && strings.TrimSpace(*m.APIKey) != *m.APIKey:
			validations = append(validations, ValidationError{
				Field:  fmt.Sprintf("api_keys[%d].api_key", i),
				Detail: "api_key must not contain leading or trailing whitespace",
			})
		}
		if hasID && !hasKey {
			if prev, ok := seen[*m.ID]; ok {
				validations = append(validations, ValidationError{
					Field:  fmt.Sprintf("api_keys[%d].id", i),
					Detail: fmt.Sprintf("id %s already referenced at api_keys[%d]", *m.ID, prev),
				})
			} else {
				seen[*m.ID] = i
			}
		}
	}
	return validations
}

// AIProviders lists all (non-deleted) AI providers.
func (c *Client) AIProviders(ctx context.Context) ([]AIProvider, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/ai/providers", nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var providers []AIProvider
	return providers, json.NewDecoder(res.Body).Decode(&providers)
}

// AIProvider fetches a single AI provider by ID or name.
func (c *Client) AIProvider(ctx context.Context, idOrName string) (AIProvider, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/ai/providers/%s", idOrName), nil)
	if err != nil {
		return AIProvider{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return AIProvider{}, ReadBodyAsError(res)
	}
	var provider AIProvider
	return provider, json.NewDecoder(res.Body).Decode(&provider)
}

// CreateAIProvider creates a new AI provider.
func (c *Client) CreateAIProvider(ctx context.Context, req CreateAIProviderRequest) (AIProvider, error) {
	res, err := c.Request(ctx, http.MethodPost, "/api/v2/ai/providers", req)
	if err != nil {
		return AIProvider{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return AIProvider{}, ReadBodyAsError(res)
	}
	var provider AIProvider
	return provider, json.NewDecoder(res.Body).Decode(&provider)
}

// UpdateAIProvider partially updates an AI provider identified by
// ID or name.
func (c *Client) UpdateAIProvider(ctx context.Context, idOrName string, req UpdateAIProviderRequest) (AIProvider, error) {
	res, err := c.Request(ctx, http.MethodPatch, fmt.Sprintf("/api/v2/ai/providers/%s", idOrName), req)
	if err != nil {
		return AIProvider{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return AIProvider{}, ReadBodyAsError(res)
	}
	var provider AIProvider
	return provider, json.NewDecoder(res.Body).Decode(&provider)
}

// DeleteAIProvider soft-deletes an AI provider identified by ID or
// name. The row is preserved for audit/FK history.
func (c *Client) DeleteAIProvider(ctx context.Context, idOrName string) error {
	res, err := c.Request(ctx, http.MethodDelete, fmt.Sprintf("/api/v2/ai/providers/%s", idOrName), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

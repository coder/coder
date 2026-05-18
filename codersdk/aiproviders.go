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
)

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
// by the API. API keys are stored in a separate ai_provider_keys
// table and managed via the keys sub-endpoints; secret fields on
// Settings are never included in responses.
type AIProvider struct {
	ID          uuid.UUID          `json:"id" format:"uuid"`
	Type        AIProviderType     `json:"type"`
	Name        string             `json:"name"`
	DisplayName string             `json:"display_name"`
	Enabled     bool               `json:"enabled"`
	BaseURL     string             `json:"base_url"`
	Settings    AIProviderSettings `json:"settings"`
	CreatedAt   time.Time          `json:"created_at" format:"date-time"`
	UpdatedAt   time.Time          `json:"updated_at" format:"date-time"`
}

// CreateAIProviderRequest is the payload for creating a new AI
// provider. Name, Type, and BaseURL are required. API keys for
// OpenAI/Anthropic providers are added via the keys sub-endpoint
// after the provider is created; Bedrock providers carry their
// credentials in Settings and do not use the keys sub-endpoint.
type CreateAIProviderRequest struct {
	Type        AIProviderType     `json:"type"`
	Name        string             `json:"name"`
	DisplayName string             `json:"display_name,omitempty"`
	Enabled     bool               `json:"enabled"`
	BaseURL     string             `json:"base_url"`
	Settings    AIProviderSettings `json:"settings,omitzero"`
}

// Validate returns the field-level validation errors for a create
// request. An empty slice indicates the request is valid.
func (req CreateAIProviderRequest) Validate() []ValidationError {
	var validations []ValidationError
	switch req.Type {
	case AIProviderTypeOpenAI, AIProviderTypeAnthropic:
	case "":
		validations = append(validations, ValidationError{Field: "type", Detail: "type is required"})
	default:
		validations = append(validations, ValidationError{
			Field:  "type",
			Detail: fmt.Sprintf("unsupported provider type %q; expected one of: openai, anthropic", req.Type),
		})
	}
	validations = append(validations, validateAIProviderName(req.Name)...)
	if req.BaseURL == "" {
		validations = append(validations, ValidationError{Field: "base_url", Detail: "base_url is required"})
	} else {
		validations = append(validations, validateAIProviderBaseURL(req.BaseURL)...)
	}
	return validations
}

// UpdateAIProviderRequest is the payload for partially updating an
// AI provider. At least one field must be non-nil. Pointer fields
// distinguish "not sent" (nil) from "set to empty/zero" (a pointer
// to the zero value).
type UpdateAIProviderRequest struct {
	DisplayName *string             `json:"display_name,omitempty"`
	Enabled     *bool               `json:"enabled,omitempty"`
	BaseURL     *string             `json:"base_url,omitempty"`
	Settings    *AIProviderSettings `json:"settings,omitempty"`
}

// Validate returns the field-level validation errors for an update
// request. An empty slice indicates the request is valid. Callers
// should reject empty patches with IsEmpty before invoking Validate.
func (req UpdateAIProviderRequest) Validate() []ValidationError {
	if req.BaseURL == nil {
		return nil
	}
	if *req.BaseURL == "" {
		return []ValidationError{{Field: "base_url", Detail: "base_url cannot be empty"}}
	}
	return validateAIProviderBaseURL(*req.BaseURL)
}

// IsEmpty reports whether the patch carries no fields.
func (req UpdateAIProviderRequest) IsEmpty() bool {
	return req.DisplayName == nil && req.Enabled == nil && req.BaseURL == nil && req.Settings == nil
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

// AIProviderKey represents a single API key registered against an
// AI provider, as returned by the API. The plaintext APIKey is
// write-only and never included in responses.
type AIProviderKey struct {
	ID         uuid.UUID `json:"id" format:"uuid"`
	ProviderID uuid.UUID `json:"provider_id" format:"uuid"`
	CreatedAt  time.Time `json:"created_at" format:"date-time"`
	UpdatedAt  time.Time `json:"updated_at" format:"date-time"`
}

// CreateAIProviderKeyRequest is the payload for adding an API key to
// an AI provider. Only meaningful for openai and anthropic providers;
// Bedrock providers reject this call because they use the access
// credentials stored in Settings.
type CreateAIProviderKeyRequest struct {
	APIKey string `json:"api_key"`
}

// Validate returns the field-level validation errors for a key create
// request. An empty slice indicates the request is valid.
func (req CreateAIProviderKeyRequest) Validate() []ValidationError {
	var validations []ValidationError
	if strings.TrimSpace(req.APIKey) == "" {
		validations = append(validations, ValidationError{Field: "api_key", Detail: "api_key is required"})
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

// CreateAIProviderKey registers a new API key against an AI
// provider identified by ID or name.
func (c *Client) CreateAIProviderKey(ctx context.Context, idOrName string, req CreateAIProviderKeyRequest) (AIProviderKey, error) {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/ai/providers/%s/keys", idOrName), req)
	if err != nil {
		return AIProviderKey{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return AIProviderKey{}, ReadBodyAsError(res)
	}
	var key AIProviderKey
	return key, json.NewDecoder(res.Body).Decode(&key)
}

// DeleteAIProviderKey removes a single API key from an AI provider.
func (c *Client) DeleteAIProviderKey(ctx context.Context, idOrName string, keyID uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodDelete, fmt.Sprintf("/api/v2/ai/providers/%s/keys/%s", idOrName, keyID), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

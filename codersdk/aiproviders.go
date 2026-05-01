package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// AIProviderType identifies the protocol Coder uses to communicate
// with an upstream AI provider.
type AIProviderType string

const (
	AIProviderTypeOpenAI    AIProviderType = "openai"
	AIProviderTypeAnthropic AIProviderType = "anthropic"
)

// AIProviderSettings holds type-specific provider settings that do
// not fit the generic API key + base URL pattern. Fields are only
// meaningful for specific provider types.
type AIProviderSettings struct {
	// BedrockRegion is the AWS region used to construct the Bedrock
	// endpoint URL when BaseURL is not set on the parent provider.
	// Only meaningful when Type is AIProviderTypeAnthropic.
	BedrockRegion string `json:"bedrock_region,omitempty"`
	// BedrockModel is the AWS Bedrock model identifier used for
	// primary requests. Only meaningful when Type is
	// AIProviderTypeAnthropic.
	BedrockModel string `json:"bedrock_model,omitempty"`
	// BedrockSmallFastModel is the AWS Bedrock model identifier used
	// for background tasks (e.g. Claude Code's haiku-class model).
	// Only meaningful when Type is AIProviderTypeAnthropic.
	BedrockSmallFastModel string `json:"bedrock_small_fast_model,omitempty"`
}

// AIProvider represents an AI Bridge provider configuration row as
// returned by the API. APIKey and BedrockAccessKeySecret are write-
// only and never included in responses.
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
// provider. Name, Type, and BaseURL are required.
type CreateAIProviderRequest struct {
	Type        AIProviderType     `json:"type"`
	Name        string             `json:"name"`
	DisplayName string             `json:"display_name,omitempty"`
	Enabled     bool               `json:"enabled"`
	BaseURL     string             `json:"base_url"`
	APIKey      string             `json:"api_key,omitempty"`
	Settings    AIProviderSettings `json:"settings,omitempty"`
	// BedrockAccessKeySecret is the AWS secret access key paired with
	// APIKey (used as the access key) when configuring an Anthropic
	// provider that targets AWS Bedrock. Write-only.
	BedrockAccessKeySecret string `json:"bedrock_access_key_secret,omitempty"`
}

// UpdateAIProviderRequest is the payload for partially updating an
// AI provider. At least one field must be non-nil. Pointer fields
// distinguish "not sent" (nil) from "set to empty/zero" (a pointer
// to the zero value).
type UpdateAIProviderRequest struct {
	DisplayName            *string             `json:"display_name,omitempty"`
	Enabled                *bool               `json:"enabled,omitempty"`
	BaseURL                *string             `json:"base_url,omitempty"`
	APIKey                 *string             `json:"api_key,omitempty"`
	Settings               *AIProviderSettings `json:"settings,omitempty"`
	BedrockAccessKeySecret *string             `json:"bedrock_access_key_secret,omitempty"`
}

// AIProviders lists all (non-deleted) AI providers.
func (c *Client) AIProviders(ctx context.Context) ([]AIProvider, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/aibridge/providers", nil)
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
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/aibridge/providers/%s", idOrName), nil)
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
	res, err := c.Request(ctx, http.MethodPost, "/api/v2/aibridge/providers", req)
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
	res, err := c.Request(ctx, http.MethodPatch, fmt.Sprintf("/api/v2/aibridge/providers/%s", idOrName), req)
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
	res, err := c.Request(ctx, http.MethodDelete, fmt.Sprintf("/api/v2/aibridge/providers/%s", idOrName), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

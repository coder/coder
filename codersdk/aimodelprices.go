package codersdk

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// AIModelPrice is a per-model token price used by AI Gateway to compute the
// cost of an interception.
//
// Prices are integer micro-units per million tokens, so 1000000 is $1.00 per
// million tokens. A nil price means the price is not known, which the cost
// calculation treats the same as zero. Distinguish that from an explicit 0,
// which declares the model free of charge.
type AIModelPrice struct {
	Provider        string             `json:"provider"`
	Model           string             `json:"model"`
	InputPrice      *int64             `json:"input_price"`
	OutputPrice     *int64             `json:"output_price"`
	CacheReadPrice  *int64             `json:"cache_read_price"`
	CacheWritePrice *int64             `json:"cache_write_price"`
	Source          AIModelPriceSource `json:"source"`
	CreatedAt       time.Time          `json:"created_at" format:"date-time"`
	UpdatedAt       time.Time          `json:"updated_at" format:"date-time"`
}

// AIModelPriceSource is where a model price came from.
type AIModelPriceSource string

const (
	// AIModelPriceSourceDefault is a price from the embedded price book.
	AIModelPriceSourceDefault AIModelPriceSource = "default"
	// AIModelPriceSourceCustom is a price set through the API.
	AIModelPriceSourceCustom AIModelPriceSource = "custom"
)

// AIModelPriceSourceFilter selects which prices a listing reports. It is
// distinct from AIModelPriceSource because no stored price is "all".
//
// @typescript-ignore AIModelPriceSourceFilter
type AIModelPriceSourceFilter string

const (
	AIModelPriceSourceFilterDefault = AIModelPriceSourceFilter(AIModelPriceSourceDefault)
	AIModelPriceSourceFilterCustom  = AIModelPriceSourceFilter(AIModelPriceSourceCustom)
	// AIModelPriceSourceFilterAll reports every price a model holds, so a model
	// carrying both appears twice.
	AIModelPriceSourceFilterAll AIModelPriceSourceFilter = "all"
)

// MaxAIModelPricesBytes bounds an upsert request body.
const MaxAIModelPricesBytes = 1 << 20 // 1 MiB

// UpsertAIModelPricesRequest sets prices for the listed models. Models absent
// from the request are left untouched.
type UpsertAIModelPricesRequest struct {
	Prices []AIModelPriceUpsert `json:"prices"`
}

// AIModelPriceUpsert is one model's prices in an upsert request. It carries
// only the writable fields of AIModelPrice.
type AIModelPriceUpsert struct {
	Provider        string `json:"provider"`
	Model           string `json:"model"`
	InputPrice      *int64 `json:"input_price"`
	OutputPrice     *int64 `json:"output_price"`
	CacheReadPrice  *int64 `json:"cache_read_price"`
	CacheWritePrice *int64 `json:"cache_write_price"`
}

// AIModelPricesFilter narrows the listed model prices. An empty field does not
// filter on that attribute.
//
// @typescript-ignore AIModelPricesFilter
type AIModelPricesFilter struct {
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
	// Source narrows to prices from one source. A model with both reports only
	// its custom price unless this is set.
	Source AIModelPriceSourceFilter `json:"source,omitempty"`
}

func (f AIModelPricesFilter) asRequestOption() RequestOption {
	return func(r *http.Request) {
		query := r.URL.Query()
		if f.Provider != "" {
			query.Set("provider", f.Provider)
		}
		if f.Model != "" {
			query.Set("model", f.Model)
		}
		if f.Source != "" {
			query.Set("source", string(f.Source))
		}
		r.URL.RawQuery = query.Encode()
	}
}

// ListAIModelPrices returns the AI model prices matching the filter.
func (c *ExperimentalClient) ListAIModelPrices(ctx context.Context, filter AIModelPricesFilter) ([]AIModelPrice, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/experimental/ai/model-prices", nil, filter.asRequestOption())
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}

	var prices []AIModelPrice
	return prices, ReadBodyAsJSON(res, &prices)
}

// UpsertAIModelPrices sets prices for the models in req. The request is
// rejected in full if any model fails validation.
func (c *ExperimentalClient) UpsertAIModelPrices(ctx context.Context, req UpsertAIModelPricesRequest) error {
	res, err := c.Request(ctx, http.MethodPost, "/api/experimental/ai/model-prices", req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

// AIModelPriceScope is what a model price applies to, as seen from an
// organization. In lookup order, most specific first.
type AIModelPriceScope string

const (
	// AIModelPriceScopeOrganizationProvider is an organization's price for one
	// configured provider.
	AIModelPriceScopeOrganizationProvider AIModelPriceScope = "organization_provider"
	// AIModelPriceScopeSite is a site-wide price for a provider type, from the
	// price book or set through the site-wide API.
	AIModelPriceScopeSite AIModelPriceScope = "site"
)

// OrganizationAIModelPrice is a model price that applies to an organization:
// one of its own prices or a site-wide price. Prices follow the units of
// AIModelPrice.
type OrganizationAIModelPrice struct {
	Scope AIModelPriceScope `json:"scope"`
	// OrganizationID is set for the organization's own prices.
	OrganizationID *uuid.UUID `json:"organization_id,omitempty" format:"uuid"`
	// ProviderID and ProviderName are set for prices for one configured
	// provider.
	ProviderID   *uuid.UUID `json:"provider_id,omitempty" format:"uuid"`
	ProviderName string     `json:"provider_name,omitempty"`
	// Provider is the provider type the price applies to.
	Provider        string `json:"provider"`
	Model           string `json:"model"`
	InputPrice      *int64 `json:"input_price"`
	OutputPrice     *int64 `json:"output_price"`
	CacheReadPrice  *int64 `json:"cache_read_price"`
	CacheWritePrice *int64 `json:"cache_write_price"`
	// Source is custom for the organization's own prices.
	Source    AIModelPriceSource `json:"source"`
	CreatedAt time.Time          `json:"created_at" format:"date-time"`
	UpdatedAt time.Time          `json:"updated_at" format:"date-time"`
}

// UpsertOrganizationAIModelPriceRequest sets an organization's price for one
// model on one configured provider. Every price key must be present. Use null
// for a price that is not known.
type UpsertOrganizationAIModelPriceRequest struct {
	ProviderID      uuid.UUID `json:"provider_id" format:"uuid"`
	Model           string    `json:"model"`
	InputPrice      *int64    `json:"input_price"`
	OutputPrice     *int64    `json:"output_price"`
	CacheReadPrice  *int64    `json:"cache_read_price"`
	CacheWritePrice *int64    `json:"cache_write_price"`
}

// OrganizationAIModelPriceKey identifies one of an organization's prices: a
// configured provider and a model.
//
// @typescript-ignore OrganizationAIModelPriceKey
type OrganizationAIModelPriceKey struct {
	ProviderID uuid.UUID `json:"provider_id"`
	Model      string    `json:"model"`
}

func (k OrganizationAIModelPriceKey) asRequestOption() RequestOption {
	return func(r *http.Request) {
		query := r.URL.Query()
		if k.ProviderID != uuid.Nil {
			query.Set("provider_id", k.ProviderID.String())
		}
		query.Set("model", k.Model)
		r.URL.RawQuery = query.Encode()
	}
}

// ListOrganizationAIModelPrices returns the model prices that apply to the
// organization and match the filter. By default a custom site-wide price
// hides the price book price it overrides, and each organization price for a
// configured provider is listed alongside the site-wide price for its type.
// AIModelPriceSourceFilterAll reports every row instead.
func (c *ExperimentalClient) ListOrganizationAIModelPrices(ctx context.Context, organization uuid.UUID, filter AIModelPricesFilter) ([]OrganizationAIModelPrice, error) {
	res, err := c.Request(ctx, http.MethodGet,
		fmt.Sprintf("/api/experimental/organizations/%s/ai/model-prices", organization.String()),
		nil,
		filter.asRequestOption(),
	)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}

	var prices []OrganizationAIModelPrice
	return prices, ReadBodyAsJSON(res, &prices)
}

// UpsertOrganizationAIModelPrice sets the organization's price for one model
// on one configured provider.
func (c *ExperimentalClient) UpsertOrganizationAIModelPrice(ctx context.Context, organization uuid.UUID, req UpsertOrganizationAIModelPriceRequest) error {
	res, err := c.Request(ctx, http.MethodPost,
		fmt.Sprintf("/api/experimental/organizations/%s/ai/model-prices", organization.String()),
		req,
	)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

// DeleteOrganizationAIModelPrice deletes one of the organization's prices.
// Site-wide prices cannot be deleted through an organization.
func (c *ExperimentalClient) DeleteOrganizationAIModelPrice(ctx context.Context, organization uuid.UUID, key OrganizationAIModelPriceKey) error {
	res, err := c.Request(ctx, http.MethodDelete,
		fmt.Sprintf("/api/experimental/organizations/%s/ai/model-prices", organization.String()),
		nil,
		key.asRequestOption(),
	)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

package codersdk

import (
	"context"
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
	// Provider is the provider type the model is priced for.
	Provider string `json:"provider"`
	// ProviderID is the configured provider a custom price applies to. It is
	// nil for a price that applies to every provider of the Provider type. A
	// provider-specific price takes precedence over a provider-type price for
	// the same model.
	ProviderID      *uuid.UUID         `json:"provider_id" format:"uuid"`
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
	// Provider is the provider type the model is priced for. It may be omitted
	// when ProviderID is set, and must match that provider's type otherwise.
	Provider string `json:"provider,omitempty"`
	// ProviderID prices the model for one configured provider rather than for
	// every provider of the Provider type. It allows pricing models served by
	// generic provider types such as openai-compat.
	ProviderID      *uuid.UUID `json:"provider_id,omitempty" format:"uuid"`
	Model           string     `json:"model"`
	InputPrice      *int64     `json:"input_price"`
	OutputPrice     *int64     `json:"output_price"`
	CacheReadPrice  *int64     `json:"cache_read_price"`
	CacheWritePrice *int64     `json:"cache_write_price"`
}

// AIModelPricesFilter narrows the listed model prices. An empty field does not
// filter on that attribute.
//
// @typescript-ignore AIModelPricesFilter
type AIModelPricesFilter struct {
	Provider string `json:"provider,omitempty"`
	// ProviderID narrows to the prices set for one configured provider.
	ProviderID uuid.UUID `json:"provider_id,omitempty" format:"uuid"`
	Model      string    `json:"model,omitempty"`
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
		if f.ProviderID != uuid.Nil {
			query.Set("provider_id", f.ProviderID.String())
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

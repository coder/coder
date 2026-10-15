package codersdk

import (
	"context"
	"net/http"
	"time"
)

// AIModelPrice is a per-model token price used by AI Gateway to compute the
// cost of an interception.
//
// Prices are integer micro-units per million tokens, so 1000000 is $1.00 per
// million tokens. A nil price means the price is not known, which the cost
// calculation treats the same as zero. Distinguish that from an explicit 0,
// which declares the model free of charge.
type AIModelPrice struct {
	Provider        string `json:"provider"`
	Model           string `json:"model"`
	InputPrice      *int64 `json:"input_price"`
	OutputPrice     *int64 `json:"output_price"`
	CacheReadPrice  *int64 `json:"cache_read_price"`
	CacheWritePrice *int64 `json:"cache_write_price"`
	// CacheWrite1hPrice is the price of a cache write with a 1-hour TTL. Nil
	// when a 1-hour write costs the cache write price.
	CacheWrite1hPrice *int64 `json:"cache_write_1h_price"`
	// Tiers are long-context prices, sorted by ThresholdTokens.
	Tiers     []AIModelPriceTier `json:"tiers"`
	Source    AIModelPriceSource `json:"source"`
	CreatedAt time.Time          `json:"created_at" format:"date-time"`
	UpdatedAt time.Time          `json:"updated_at" format:"date-time"`
}

// AIModelPriceTier holds the prices for a request whose prompt is over
// ThresholdTokens tokens. Every token of such a request uses the prices of the
// highest tier it is over. A nil price uses the base price of the model, and a
// nil CacheWrite1hPrice uses the CacheWritePrice of the tier.
type AIModelPriceTier struct {
	ThresholdTokens   int64  `json:"threshold_tokens"`
	InputPrice        *int64 `json:"input_price"`
	OutputPrice       *int64 `json:"output_price"`
	CacheReadPrice    *int64 `json:"cache_read_price"`
	CacheWritePrice   *int64 `json:"cache_write_price"`
	CacheWrite1hPrice *int64 `json:"cache_write_1h_price"`
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
// only the writable fields of AIModelPrice. CacheWrite1hPrice and Tiers are
// optional; leaving them out clears them.
type AIModelPriceUpsert struct {
	Provider          string             `json:"provider"`
	Model             string             `json:"model"`
	InputPrice        *int64             `json:"input_price"`
	OutputPrice       *int64             `json:"output_price"`
	CacheReadPrice    *int64             `json:"cache_read_price"`
	CacheWritePrice   *int64             `json:"cache_write_price"`
	CacheWrite1hPrice *int64             `json:"cache_write_1h_price"`
	Tiers             []AIModelPriceTier `json:"tiers,omitempty"`
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

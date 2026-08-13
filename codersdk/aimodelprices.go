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
	Provider        string    `json:"provider"`
	Model           string    `json:"model"`
	InputPrice      *int64    `json:"input_price"`
	OutputPrice     *int64    `json:"output_price"`
	CacheReadPrice  *int64    `json:"cache_read_price"`
	CacheWritePrice *int64    `json:"cache_write_price"`
	CreatedAt       time.Time `json:"created_at" format:"date-time"`
	UpdatedAt       time.Time `json:"updated_at" format:"date-time"`
}

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

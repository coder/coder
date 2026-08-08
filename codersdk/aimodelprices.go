package codersdk

import (
	"context"
	"net/http"
	"time"
)

// AIModelPrice is a per-model token price used by AI Gateway to compute the
// cost of an interception.
//
// Prices are integer micro-units per million tokens, so 10000000 is $10.00 per
// million tokens. A nil price means the price is not known, which the cost
// calculation treats the same as zero. Distinguish that from an explicit 0,
// which declares the model free.
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

// ListAIModelPricesOptions narrows what ListAIModelPrices returns.
type ListAIModelPricesOptions struct {
	// IncludeDefaults adds the models Coder prices out of the box, which are
	// otherwise omitted because they outnumber operator-set prices by orders
	// of magnitude.
	IncludeDefaults bool
}

// ListAIModelPrices returns the AI model prices set for this deployment.
func (c *ExperimentalClient) ListAIModelPrices(ctx context.Context, opts ListAIModelPricesOptions) ([]AIModelPrice, error) {
	path := "/api/experimental/ai/model-prices"
	if opts.IncludeDefaults {
		path += "?all=true"
	}
	res, err := c.Request(ctx, http.MethodGet, path, nil)
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
	res, err := c.Request(ctx, http.MethodPut, "/api/experimental/ai/model-prices", req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

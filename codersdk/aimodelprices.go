package codersdk

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// AIModelPrice is a single model pricing row for the AI Gateway.
// Prices are in micro-units (1 unit = 1,000,000) per million tokens.
// A nil price means "unknown"; an explicit zero means "free".
// EXPERIMENTAL: this type is experimental and is subject to change.
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

// ListAIModelPrices returns all configured model prices.
// EXPERIMENTAL: this API is subject to change.
func (c *ExperimentalClient) ListAIModelPrices(ctx context.Context) ([]AIModelPrice, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/experimental/ai/model-prices", nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}

	var prices []AIModelPrice
	return prices, json.NewDecoder(res.Body).Decode(&prices)
}

// PutAIModelPrices upserts a batch of model prices.
// EXPERIMENTAL: this API is subject to change.
func (c *ExperimentalClient) PutAIModelPrices(ctx context.Context, prices []AIModelPrice) error {
	res, err := c.Request(ctx, http.MethodPut, "/api/experimental/ai/model-prices", prices)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

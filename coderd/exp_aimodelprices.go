package coderd

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
// @Summary List AI model prices
// @ID list-ai-model-prices
// @Security CoderSessionToken
// @Tags Experimental
// @Produce json
// @Success 200 {array} codersdk.AIModelPrice
// @Router /api/experimental/ai/model-prices [get]
// @Description Experimental: this endpoint is subject to change.
func (api *API) listAIModelPrices(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	prices, err := api.Database.GetAIModelPrices(ctx)
	if err != nil {
		if dbauthz.IsNotAuthorizedError(err) {
			httpapi.Forbidden(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to list AI model prices.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, dbAIModelPricesToSDK(prices))
}

// dbAIModelPricesToSDK converts database AI model price rows to the codersdk
// wire format.
func dbAIModelPricesToSDK(prices []database.AIModelPrice) []codersdk.AIModelPrice {
	resp := make([]codersdk.AIModelPrice, 0, len(prices))
	for _, p := range prices {
		resp = append(resp, codersdk.AIModelPrice{
			Provider:        p.Provider,
			Model:           p.Model,
			InputPrice:      db2sdk.NullInt64Ptr(p.InputPrice),
			OutputPrice:     db2sdk.NullInt64Ptr(p.OutputPrice),
			CacheReadPrice:  db2sdk.NullInt64Ptr(p.CacheReadPrice),
			CacheWritePrice: db2sdk.NullInt64Ptr(p.CacheWritePrice),
			CreatedAt:       p.CreatedAt,
			UpdatedAt:       p.UpdatedAt,
		})
	}
	return resp
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
// @Summary Upsert AI model prices
// @ID put-ai-model-prices
// @Security CoderSessionToken
// @Tags Experimental
// @Accept json
// @Produce json
// @Success 200 {array} codersdk.AIModelPrice
// @Router /api/experimental/ai/model-prices [put]
// @Description Experimental: this endpoint is subject to change.
func (api *API) putAIModelPrices(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to read request body.",
		})
		return
	}

	// Unmarshal into typed slice for validation only; the raw bytes are
	// passed to the database so jsonb_array_elements sees the original field names.
	var prices []codersdk.AIModelPrice
	if err := json.Unmarshal(rawBody, &prices); err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid request body.",
			Detail:  err.Error(),
		})
		return
	}

	if len(prices) == 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "At least one model price must be provided.",
		})
		return
	}

	for _, p := range prices {
		if p.InputPrice != nil && *p.InputPrice < 0 {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Input price must not be negative.",
			})
			return
		}
		if p.OutputPrice != nil && *p.OutputPrice < 0 {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Output price must not be negative.",
			})
			return
		}
		if p.CacheReadPrice != nil && *p.CacheReadPrice < 0 {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Cache read price must not be negative.",
			})
			return
		}
		if p.CacheWritePrice != nil && *p.CacheWritePrice < 0 {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Cache write price must not be negative.",
			})
			return
		}
	}

	err = api.Database.UpsertAIModelPrices(ctx, json.RawMessage(rawBody))
	if err != nil {
		if dbauthz.IsNotAuthorizedError(err) {
			httpapi.Forbidden(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to upsert AI model prices.",
			Detail:  err.Error(),
		})
		return
	}

	// Return the updated list so callers get timestamps back.
	updated, err := api.Database.GetAIModelPrices(ctx)
	if err != nil {
		if dbauthz.IsNotAuthorizedError(err) {
			httpapi.Forbidden(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to list AI model prices after upsert.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, dbAIModelPricesToSDK(updated))
}

package coderd

import (
	"encoding/json"
	"fmt"
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
// @Tags AI Gateway
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
// @ID upsert-ai-model-prices
// @Security CoderSessionToken
// @Tags AI Gateway
// @Accept json
// @Param request body codersdk.AIModelPrice true "Model prices"
// @Success 204
// @Router /api/experimental/ai/model-prices [put]
// @Description Experimental: this endpoint is subject to change.
func (api *API) putAIModelPrices(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	r.Body = http.MaxBytesReader(rw, r.Body, 1<<20)
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

	seen := make(map[string]bool, len(prices))
	for i, p := range prices {
		if p.Provider == "" {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: fmt.Sprintf("Element %d: provider must not be empty.", i),
			})
			return
		}
		if p.Model == "" {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: fmt.Sprintf("Element %d: model must not be empty.", i),
			})
			return
		}
		key := p.Provider + "\x00" + p.Model
		if seen[key] {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: fmt.Sprintf("Duplicate entry for provider %q model %q.", p.Provider, p.Model),
			})
			return
		}
		seen[key] = true
		for _, pc := range []struct {
			name  string
			value *int64
		}{
			{"input price", p.InputPrice},
			{"output price", p.OutputPrice},
			{"cache read price", p.CacheReadPrice},
			{"cache write price", p.CacheWritePrice},
		} {
			if pc.value != nil && *pc.value < 0 {
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: fmt.Sprintf("Element %d (%s/%s): %s must not be negative.", i, p.Provider, p.Model, pc.name),
				})
				return
			}
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

	rw.WriteHeader(http.StatusNoContent)
}

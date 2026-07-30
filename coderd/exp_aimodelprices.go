package coderd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/audit"
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

	// Fetch current state before upsert to determine adds vs changes.
	current, err := api.Database.GetAIModelPrices(ctx)
	if err != nil {
		if dbauthz.IsNotAuthorizedError(err) {
			httpapi.Forbidden(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to read current AI model prices.",
			Detail:  err.Error(),
		})
		return
	}

	curMap := make(map[string]database.AIModelPrice, len(current))
	for _, c := range current {
		curMap[c.Provider+"\x00"+c.Model] = c
	}

	// Categorize the incoming prices.
	var additions, changes []codersdk.AIModelPrice
	addProviderSet := make(map[string]bool)
	changeProviderSet := make(map[string]bool)
	for _, p := range prices {
		key := p.Provider + "\x00" + p.Model
		if _, ok := curMap[key]; ok {
			changes = append(changes, p)
			changeProviderSet[p.Provider] = true
		} else {
			additions = append(additions, p)
			addProviderSet[p.Provider] = true
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

	// Emit audit log entries. We use BackgroundAudit (which commits
	// immediately) rather than the deferred InitRequest pattern because a
	// single PUT can produce up to two entries: one for added rows and one
	// for changed rows.
	if len(additions) > 0 {
		entries := make([]aiModelPriceAuditEntry, 0, len(additions))
		for _, p := range additions {
			entries = append(entries, aiModelPriceAuditEntry{
				Provider:           p.Provider,
				Model:              p.Model,
				NewInputPrice:      p.InputPrice,
				NewOutputPrice:     p.OutputPrice,
				NewCacheReadPrice:  p.CacheReadPrice,
				NewCacheWritePrice: p.CacheWritePrice,
			})
		}
		emitAIModelPriceAudit(ctx, api, database.AuditActionCreate,
			database.AuditableAIModelPriceUpdate{
				ID:        uuid.New(),
				Providers: strings.Join(sortedKeys(addProviderSet), ", "),
				Count:     len(additions),
			}, entries)
	}

	if len(changes) > 0 {
		entries := make([]aiModelPriceAuditEntry, 0, len(changes))
		for _, p := range changes {
			old := curMap[p.Provider+"\x00"+p.Model]
			entries = append(entries, aiModelPriceAuditEntry{
				Provider:           p.Provider,
				Model:              p.Model,
				OldInputPrice:      db2sdk.NullInt64Ptr(old.InputPrice),
				OldOutputPrice:     db2sdk.NullInt64Ptr(old.OutputPrice),
				OldCacheReadPrice:  db2sdk.NullInt64Ptr(old.CacheReadPrice),
				OldCacheWritePrice: db2sdk.NullInt64Ptr(old.CacheWritePrice),
				NewInputPrice:      p.InputPrice,
				NewOutputPrice:     p.OutputPrice,
				NewCacheReadPrice:  p.CacheReadPrice,
				NewCacheWritePrice: p.CacheWritePrice,
			})
		}
		emitAIModelPriceAudit(ctx, api, database.AuditActionWrite,
			database.AuditableAIModelPriceUpdate{
				ID:        uuid.New(),
				Providers: strings.Join(sortedKeys(changeProviderSet), ", "),
				Count:     len(changes),
			}, entries)
	}
}

// aiModelPriceAuditAdditionalFields carries the full detail of which rows
// were added or changed in a batch upsert. It is marshaled into the audit
// log's AdditionalFields column.
type aiModelPriceAuditAdditionalFields struct {
	Entries []aiModelPriceAuditEntry `json:"entries"`
}

type aiModelPriceAuditEntry struct {
	Provider           string `json:"provider"`
	Model              string `json:"model"`
	OldInputPrice      *int64 `json:"old_input_price,omitempty"`
	OldOutputPrice     *int64 `json:"old_output_price,omitempty"`
	OldCacheReadPrice  *int64 `json:"old_cache_read_price,omitempty"`
	OldCacheWritePrice *int64 `json:"old_cache_write_price,omitempty"`
	NewInputPrice      *int64 `json:"new_input_price,omitempty"`
	NewOutputPrice     *int64 `json:"new_output_price,omitempty"`
	NewCacheReadPrice  *int64 `json:"new_cache_read_price,omitempty"`
	NewCacheWritePrice *int64 `json:"new_cache_write_price,omitempty"`
}

// emitAIModelPriceAudit emits a single audit log entry for a batch of added
// or changed AI model prices.
func emitAIModelPriceAudit(ctx context.Context, api *API, action database.AuditAction, summary database.AuditableAIModelPriceUpdate, entries []aiModelPriceAuditEntry) {
	addFields, err := json.Marshal(aiModelPriceAuditAdditionalFields{Entries: entries})
	if err != nil {
		api.Logger.Warn(ctx, "marshal ai model price audit additional fields", slog.Error(err))
		addFields = json.RawMessage("{}")
	}
	audit.BackgroundAudit(ctx, &audit.BackgroundAuditParams[database.AuditableAIModelPriceUpdate]{
		Audit:            *api.Auditor.Load(),
		Log:              api.Logger,
		Action:           action,
		New:              summary,
		Old:              database.AuditableAIModelPriceUpdate{},
		AdditionalFields: addFields,
	})
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

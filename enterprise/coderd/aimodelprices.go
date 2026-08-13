package coderd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/aibridge/prices"
	"github.com/coder/coder/v2/coderd/aibridge/prices/providers"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
)

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
// @Summary List AI model prices
// @ID list-ai-model-prices
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param provider query string false "Only return prices for this provider"
// @Param model query string false "Only return prices for this model"
// @Success 200 {array} codersdk.AIModelPrice
// @Router /api/experimental/ai/model-prices [get]
// @x-apidocgen {"skip": true}
func (api *API) listAIModelPrices(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	dbPrices, err := api.Database.GetAIModelPrices(ctx, database.GetAIModelPricesParams{
		Provider: r.URL.Query().Get("provider"),
		Model:    r.URL.Query().Get("model"),
	})
	if dbauthz.IsNotAuthorizedError(err) {
		httpapi.Forbidden(rw)
		return
	}
	if err != nil {
		api.Logger.Error(ctx, "list ai model prices", slog.Error(err))
		httpapi.InternalServerError(rw, err)
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.AIModelPrices(dbPrices))
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
// @Summary Upsert AI model prices
// @ID upsert-ai-model-prices
// @Security CoderSessionToken
// @Accept json
// @Tags Enterprise
// @Param request body codersdk.UpsertAIModelPricesRequest true "Prices to set"
// @Success 204
// @Router /api/experimental/ai/model-prices [post]
// @x-apidocgen {"skip": true}
func (api *API) upsertAIModelPrices(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	r.Body = http.MaxBytesReader(rw, r.Body, codersdk.MaxAIModelPricesBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			httpapi.Write(ctx, rw, http.StatusRequestEntityTooLarge, codersdk.Response{
				Message: "Request body too large.",
				Detail:  err.Error(),
			})
			return
		}
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to read request body.",
			Detail:  err.Error(),
		})
		return
	}

	// Decoding a price into *int64 loses whether its key was there at all, so
	// decode the entries as raw keys too. An absent key is not the same as an
	// explicit null.
	var rawReq struct {
		Prices []map[string]json.RawMessage `json:"prices"`
	}
	if err := json.Unmarshal(body, &rawReq); err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Request body must be valid JSON.",
			Detail:  err.Error(),
		})
		return
	}
	var req codersdk.UpsertAIModelPricesRequest
	if err := json.Unmarshal(body, &req); err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Request body must be valid JSON.",
			Detail:  err.Error(),
		})
		return
	}

	// Validate the whole request before writing anything, so a single bad
	// entry cannot leave the table half-updated.
	validations := validateAIModelPrices(req.Prices, rawReq.Prices)
	if len(validations) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid AI model prices.",
			Validations: validations,
		})
		return
	}

	// The batch upsert reads the rows as a JSON array, matching the embedded
	// price seed's wire format.
	seed, err := json.Marshal(req.Prices)
	if err != nil {
		api.Logger.Error(ctx, "marshal ai model prices", slog.Error(err))
		httpapi.InternalServerError(rw, err)
		return
	}
	err = api.Database.UpsertAIModelPrices(ctx, seed)
	if dbauthz.IsNotAuthorizedError(err) {
		httpapi.Forbidden(rw)
		return
	}
	if err != nil {
		api.Logger.Error(ctx, "upsert ai model prices", slog.Error(err),
			slog.F("user_id", httpmw.APIKey(r).UserID),
			slog.F("count", len(req.Prices)),
		)
		httpapi.InternalServerError(rw, err)
		return
	}

	// Model prices feed cost reporting and budget enforcement, so record who
	// changed how many.
	// TODO(ssncferreira): replace with audit logging once ai_model_price is an
	// auditable resource (AIGOV-590).
	api.Logger.Info(ctx, "ai model prices updated",
		slog.F("user_id", httpmw.APIKey(r).UserID),
		slog.F("count", len(req.Prices)),
	)

	rw.WriteHeader(http.StatusNoContent)
}

// modelKey identifies a priced model.
type modelKey struct {
	provider string
	model    string
}

// validateAIModelPrices reports every problem with the requested prices: a
// supported provider, a model Coder's price book does not already cover, all
// four price keys, non-negative prices with at least one set, and no repeated
// model.
func validateAIModelPrices(requested []codersdk.AIModelPriceUpsert, raw []map[string]json.RawMessage) []codersdk.ValidationError {
	if len(requested) == 0 {
		return []codersdk.ValidationError{{
			Field:  "prices",
			Detail: "At least one model price is required.",
		}}
	}

	supportedProviders := strings.Join(providers.SupportedStrings(), ", ")
	seen := make(map[modelKey]struct{}, len(requested))
	var validations []codersdk.ValidationError

	for i, price := range requested {
		field := fmt.Sprintf("prices[%d]", i)

		// Provider and model identify the row, so report them first.
		switch {
		case price.Provider == "":
			validations = append(validations, codersdk.ValidationError{
				Field:  field + ".provider",
				Detail: fmt.Sprintf("Provider is required. Supported providers: %s.", supportedProviders),
			})
		case !slices.Contains(providers.Supported, database.AIProviderType(price.Provider)):
			validations = append(validations, codersdk.ValidationError{
				Field:  field + ".provider",
				Detail: fmt.Sprintf("Provider %q is not supported. Supported providers: %s.", price.Provider, supportedProviders),
			})
		}
		if price.Model == "" {
			validations = append(validations, codersdk.ValidationError{
				Field:  field + ".model",
				Detail: "Model is required.",
			})
		}
		// The price book is re-applied on every server start, so a price set for
		// a model it covers would not survive a restart.
		// TODO(ssncferreira): drop this once custom pricing is supported
		// (AIGOV-589).
		if prices.IsDefaultPriced(price.Provider, price.Model) {
			validations = append(validations, codersdk.ValidationError{
				Field:  field,
				Detail: fmt.Sprintf("%s/%s is priced by Coder's default price book. Overriding a default price is not supported.", price.Provider, price.Model),
			})
		}

		named := []struct {
			name  string
			value *int64
		}{
			{"input_price", price.InputPrice},
			{"output_price", price.OutputPrice},
			{"cache_read_price", price.CacheReadPrice},
			{"cache_write_price", price.CacheWritePrice},
		}
		// raw and requested are decoded from the same body, so this index
		// always exists. Bounds-check it anyway rather than risk a panic.
		var rawEntry map[string]json.RawMessage
		if i < len(raw) {
			rawEntry = raw[i]
		}

		var present, populated int
		for _, p := range named {
			if _, ok := rawEntry[p.name]; ok {
				present++
			}
			if p.value == nil {
				continue
			}
			populated++
			if *p.value < 0 {
				validations = append(validations, codersdk.ValidationError{
					Field:  field + "." + p.name,
					Detail: "Price must not be negative.",
				})
			}
		}

		// An entry sets all four columns, so an absent key would clear that
		// price rather than leave it alone.
		if present > 0 && present < len(named) {
			for _, p := range named {
				if _, ok := rawEntry[p.name]; ok {
					continue
				}
				validations = append(validations, codersdk.ValidationError{
					Field:  field + "." + p.name,
					Detail: "Price is required. Use 'null' for a price that is not known.",
				})
			}
		}

		// An all-null entry creates a row that computes zero cost, so the model
		// stops counting as unpriced without being priced. The price book skips
		// such models rather than seeding a row for them.
		if populated == 0 {
			validations = append(validations, codersdk.ValidationError{
				Field:  field,
				Detail: "At least one price must be set. Use 0 to declare a model free of charge.",
			})
		}

		key := modelKey{provider: price.Provider, model: price.Model}
		if _, duplicate := seen[key]; duplicate {
			validations = append(validations, codersdk.ValidationError{
				Field:  field,
				Detail: fmt.Sprintf("%s/%s appears more than once.", price.Provider, price.Model),
			})
		}
		seen[key] = struct{}{}
	}

	return validations
}

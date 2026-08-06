package coderd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/aibridge/prices"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary List AI model prices
// @ID list-ai-model-prices
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param all query bool false "Include models priced by Coder's default price book"
// @Success 200 {array} codersdk.AIModelPrice
// @Router /ai/model-prices [get]
// @x-apidocgen {"skip": true}
func (api *API) listAIModelPrices(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Coder's own price book is hundreds of models, which drowns out the
	// handful an operator has set. Callers opt in to the full table.
	includeDefaults := r.URL.Query().Get("all") == "true"

	dbPrices, err := api.Database.GetAIModelPrices(ctx)
	if dbauthz.IsNotAuthorizedError(err) {
		httpapi.Forbidden(rw)
		return
	}
	if err != nil {
		api.Logger.Error(ctx, "list ai model prices", slog.Error(err))
		httpapi.InternalServerError(rw, err)
		return
	}
	if !includeDefaults {
		dbPrices = slices.DeleteFunc(dbPrices, func(price database.AIModelPrice) bool {
			return prices.IsDefaultPriced(price.Provider, price.Model)
		})
	}

	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.AIModelPrices(dbPrices))
}

// @Summary Upsert AI model prices
// @ID upsert-ai-model-prices
// @Security CoderSessionToken
// @Accept json
// @Tags Enterprise
// @Param request body codersdk.UpsertAIModelPricesRequest true "Prices to set"
// @Success 204
// @Router /ai/model-prices [put]
// @x-apidocgen {"skip": true}
func (api *API) upsertAIModelPrices(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req codersdk.UpsertAIModelPricesRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	// Validate the whole request before writing anything, so a single bad
	// entry cannot leave the table half-updated, and report every problem at
	// once so a large payload can be fixed in one pass.
	if validations := validateAIModelPrices(req.Prices); len(validations) > 0 {
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
		api.Logger.Error(ctx, "upsert ai model prices", slog.Error(err))
		httpapi.InternalServerError(rw, err)
		return
	}

	// Model prices feed cost reporting and budget enforcement, so record who
	// changed what. Replaced by audit logging once ai_model_price becomes an
	// auditable resource.
	models := make([]string, 0, len(req.Prices))
	for _, price := range req.Prices {
		models = append(models, price.Provider+"/"+price.Model)
	}
	api.Logger.Info(ctx, "ai model prices updated",
		slog.F("user_id", httpmw.APIKey(r).UserID),
		slog.F("count", len(req.Prices)),
		slog.F("models", models),
	)

	rw.WriteHeader(http.StatusNoContent)
}

// validateAIModelPrices reports every problem with the requested prices.
func validateAIModelPrices(requested []codersdk.AIModelPriceUpsert) []codersdk.ValidationError {
	if len(requested) == 0 {
		return []codersdk.ValidationError{{
			Field:  "prices",
			Detail: "At least one model price is required.",
		}}
	}

	supported := prices.SupportedProviders()
	seen := make(map[string]struct{}, len(requested))
	var validations []codersdk.ValidationError

	for i, price := range requested {
		field := fmt.Sprintf("prices[%d]", i)

		if !slices.Contains(supported, database.AIProviderType(price.Provider)) {
			validations = append(validations, codersdk.ValidationError{
				Field:  field + ".provider",
				Detail: fmt.Sprintf("Provider %q is not supported. Supported providers: %s.", price.Provider, joinProviders(supported)),
			})
		}
		if price.Model == "" {
			validations = append(validations, codersdk.ValidationError{
				Field:  field + ".model",
				Detail: "Model is required.",
			})
		}
		if prices.IsDefaultPriced(price.Provider, price.Model) {
			validations = append(validations, codersdk.ValidationError{
				Field:  field,
				Detail: fmt.Sprintf("%s/%s is priced by Coder's default price book. Overriding a default price is not supported yet.", price.Provider, price.Model),
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
		populated := 0
		for _, p := range named {
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
		// An entry with no prices at all is almost always a missing field
		// rather than an intent to declare the model free, which is better
		// expressed as an explicit zero.
		if populated == 0 {
			validations = append(validations, codersdk.ValidationError{
				Field:  field,
				Detail: "At least one price is required. Use an explicit 0 to declare a model free.",
			})
		}

		key := price.Provider + "/" + price.Model
		if _, duplicate := seen[key]; duplicate {
			validations = append(validations, codersdk.ValidationError{
				Field:  field,
				Detail: fmt.Sprintf("%s appears more than once.", key),
			})
		}
		seen[key] = struct{}{}
	}

	return validations
}

func joinProviders(providers []database.AIProviderType) string {
	names := make([]string, 0, len(providers))
	for _, provider := range providers {
		names = append(names, string(provider))
	}
	return strings.Join(names, ", ")
}

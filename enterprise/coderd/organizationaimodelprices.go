package coderd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
)

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
// @Summary List organization AI model prices
// @ID list-organization-ai-model-prices
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param organization path string true "Organization ID" format(uuid)
// @Param provider query string false "Only return prices for this provider type"
// @Param model query string false "Only return prices for this model"
// @Param source query string false "Only return prices from this source, or all to return every price that applies" Enums(default,custom,all)
// @Success 200 {array} codersdk.OrganizationAIModelPrice
// @Router /api/experimental/organizations/{organization}/ai/model-prices [get]
// @x-apidocgen {"skip": true}
func (api *API) listOrganizationAIModelPrices(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org := httpmw.OrganizationParam(r)

	// Organization prices exist for openai-compat providers, so any provider
	// type can be listed.
	provider := r.URL.Query().Get("provider")
	if provider != "" && !database.AIProviderType(provider).Valid() {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid AI model price provider.",
			Validations: []codersdk.ValidationError{{
				Field:  "provider",
				Detail: fmt.Sprintf("Provider %q is not a known provider type.", provider),
			}},
		})
		return
	}

	source := r.URL.Query().Get("source")
	if source != "" && !slices.Contains(aiModelPriceSources, source) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid AI model price source.",
			Validations: []codersdk.ValidationError{{
				Field:  "source",
				Detail: fmt.Sprintf("Source %q is not supported. Supported sources: %s.", source, strings.Join(aiModelPriceSources, ", ")),
			}},
		})
		return
	}

	dbPrices, err := api.Database.GetOrganizationAIModelPrices(ctx, database.GetOrganizationAIModelPricesParams{
		OrganizationID: org.ID,
		Provider:       provider,
		Model:          r.URL.Query().Get("model"),
		Source:         source,
	})
	if dbauthz.IsNotAuthorizedError(err) {
		httpapi.Forbidden(rw)
		return
	}
	if err != nil {
		api.Logger.Error(ctx, "list organization ai model prices", slog.Error(err),
			slog.F("organization_id", org.ID))
		httpapi.InternalServerError(rw, err)
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.OrganizationAIModelPrices(dbPrices))
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
// @Summary Upsert organization AI model prices
// @ID upsert-organization-ai-model-prices
// @Security CoderSessionToken
// @Accept json
// @Tags Enterprise
// @Param organization path string true "Organization ID" format(uuid)
// @Param request body codersdk.UpsertOrganizationAIModelPricesRequest true "Prices to set"
// @Success 204
// @Router /api/experimental/organizations/{organization}/ai/model-prices [post]
// @x-apidocgen {"skip": true}
func (api *API) upsertOrganizationAIModelPrices(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org := httpmw.OrganizationParam(r)

	// Validation looks up providers with system access, so authorize first
	// to keep unauthorized callers from probing provider IDs.
	if !api.Authorize(r, policy.ActionUpdate, rbac.ResourceAiModelPrice.InOrg(org.ID)) {
		httpapi.Forbidden(rw)
		return
	}

	var req codersdk.UpsertOrganizationAIModelPricesRequest
	rawPrices, ok := readAIModelPricesBody(rw, r, &req)
	if !ok {
		return
	}

	// Validate the whole request before writing anything, so a single bad
	// entry cannot leave the table half-updated.
	//nolint:gocritic // Org admins may lack provider read; the update was authorized above.
	validations, err := validateOrganizationAIModelPrices(dbauthz.AsSystemRestricted(ctx), api.Database, req.Prices, rawPrices)
	if err != nil {
		api.Logger.Error(ctx, "validate organization ai model prices", slog.Error(err),
			slog.F("organization_id", org.ID))
		httpapi.InternalServerError(rw, err)
		return
	}
	if len(validations) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid AI model prices.",
			Validations: validations,
		})
		return
	}

	// The batch upsert reads the rows as a JSON array.
	batch, err := json.Marshal(req.Prices)
	if err != nil {
		httpapi.InternalServerError(rw, xerrors.Errorf("marshal provider prices: %w", err))
		return
	}
	err = api.Database.UpsertOrganizationAIProviderModelPrices(ctx, database.UpsertOrganizationAIProviderModelPricesParams{
		OrganizationID: org.ID,
		Prices:         batch,
	})
	if dbauthz.IsNotAuthorizedError(err) {
		httpapi.Forbidden(rw)
		return
	}
	if err != nil {
		api.Logger.Error(ctx, "upsert organization ai model prices", slog.Error(err),
			slog.F("organization_id", org.ID),
			slog.F("user_id", httpmw.APIKey(r).UserID),
			slog.F("count", len(req.Prices)),
		)
		httpapi.InternalServerError(rw, err)
		return
	}

	// TODO(ssncferreira): replace with audit logging once ai_model_price is an
	// auditable resource (AIGOV-590).
	api.Logger.Info(ctx, "organization ai model prices updated",
		slog.F("organization_id", org.ID),
		slog.F("user_id", httpmw.APIKey(r).UserID),
		slog.F("count", len(req.Prices)),
	)

	rw.WriteHeader(http.StatusNoContent)
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
// @Summary Delete organization AI model price
// @ID delete-organization-ai-model-price
// @Security CoderSessionToken
// @Tags Enterprise
// @Param organization path string true "Organization ID" format(uuid)
// @Param provider_id query string true "Configured provider of the price to delete" format(uuid)
// @Param model query string true "Model of the price to delete"
// @Success 204
// @Router /api/experimental/organizations/{organization}/ai/model-prices [delete]
// @x-apidocgen {"skip": true}
func (api *API) deleteOrganizationAIModelPrice(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org := httpmw.OrganizationParam(r)

	// Model names can contain slashes, so the key is read from the query
	// rather than the path.
	query := r.URL.Query()
	parser := httpapi.NewQueryParamParser().RequiredNotEmpty("provider_id", "model")
	model := parser.String(query, "", "model")
	providerID := parser.UUID(query, uuid.Nil, "provider_id")
	parser.ErrorExcessParams(query)
	if len(parser.Errors) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid AI model price key.",
			Validations: parser.Errors,
		})
		return
	}

	deleted, err := api.Database.DeleteOrganizationAIProviderModelPrice(ctx, database.DeleteOrganizationAIProviderModelPriceParams{
		OrganizationID: org.ID,
		ProviderID:     providerID,
		Model:          model,
	})
	if dbauthz.IsNotAuthorizedError(err) {
		httpapi.Forbidden(rw)
		return
	}
	if err != nil {
		api.Logger.Error(ctx, "delete organization ai model price", slog.Error(err),
			slog.F("organization_id", org.ID))
		httpapi.InternalServerError(rw, err)
		return
	}
	if deleted == 0 {
		httpapi.ResourceNotFound(rw)
		return
	}

	// TODO(ssncferreira): replace with audit logging once ai_model_price is an
	// auditable resource (AIGOV-590).
	api.Logger.Info(ctx, "organization ai model price deleted",
		slog.F("organization_id", org.ID),
		slog.F("user_id", httpmw.APIKey(r).UserID),
		slog.F("provider_id", providerID),
		slog.F("model", model),
	)

	rw.WriteHeader(http.StatusNoContent)
}

// organizationAIModelPriceKey identifies one of an organization's prices.
type organizationAIModelPriceKey struct {
	providerID uuid.UUID
	model      string
}

// validateOrganizationAIModelPrices reports every problem with the requested
// organization prices: a live provider_id, a model, valid prices, and no
// repeated key. It returns an error only when a provider lookup fails.
func validateOrganizationAIModelPrices(ctx context.Context, db database.Store, requested []codersdk.OrganizationAIModelPriceUpsert, raw []map[string]json.RawMessage) ([]codersdk.ValidationError, error) {
	if len(requested) == 0 {
		return []codersdk.ValidationError{{
			Field:  "prices",
			Detail: "At least one model price is required.",
		}}, nil
	}

	// Whether each looked up provider ID matches a live provider.
	live := make(map[uuid.UUID]bool)
	seen := make(map[organizationAIModelPriceKey]struct{}, len(requested))
	var validations []codersdk.ValidationError

	for i, price := range requested {
		field := fmt.Sprintf("prices[%d]", i)

		// The provider and model identify the row, so report them first.
		if price.ProviderID == uuid.Nil {
			validations = append(validations, codersdk.ValidationError{
				Field:  field + ".provider_id",
				Detail: "Provider ID is required.",
			})
		} else {
			exists, ok := live[price.ProviderID]
			if !ok {
				_, err := db.GetAIProviderByID(ctx, price.ProviderID)
				switch {
				case errors.Is(err, sql.ErrNoRows):
				case err != nil:
					return nil, xerrors.Errorf("get provider %s: %w", price.ProviderID, err)
				default:
					exists = true
				}
				live[price.ProviderID] = exists
			}
			if !exists {
				validations = append(validations, codersdk.ValidationError{
					Field:  field + ".provider_id",
					Detail: fmt.Sprintf("Provider %s does not exist.", price.ProviderID),
				})
			}
		}
		if price.Model == "" {
			validations = append(validations, codersdk.ValidationError{
				Field:  field + ".model",
				Detail: "Model is required.",
			})
		}
		validations = append(validations, validateAIModelPriceValues(field, rawEntryAt(raw, i),
			price.InputPrice, price.OutputPrice, price.CacheReadPrice, price.CacheWritePrice)...)

		key := organizationAIModelPriceKey{providerID: price.ProviderID, model: price.Model}
		if _, duplicate := seen[key]; duplicate {
			validations = append(validations, codersdk.ValidationError{
				Field:  field,
				Detail: fmt.Sprintf("%s/%s appears more than once.", price.ProviderID, price.Model),
			})
		}
		seen[key] = struct{}{}
	}

	return validations, nil
}

package coderd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
// @Summary Upsert organization AI model price
// @ID upsert-organization-ai-model-price
// @Security CoderSessionToken
// @Accept json
// @Tags Enterprise
// @Param organization path string true "Organization ID" format(uuid)
// @Param request body codersdk.UpsertOrganizationAIModelPriceRequest true "Price to set"
// @Success 204
// @Router /api/experimental/organizations/{organization}/ai/model-prices [post]
// @x-apidocgen {"skip": true}
func (api *API) upsertOrganizationAIModelPrice(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org := httpmw.OrganizationParam(r)

	// Validation looks up providers with system access, so authorize first
	// to keep unauthorized callers from probing provider IDs.
	if !api.Authorize(r, policy.ActionUpdate, rbac.ResourceAiModelPrice.InOrg(org.ID)) {
		httpapi.Forbidden(rw)
		return
	}

	r.Body = http.MaxBytesReader(rw, r.Body, codersdk.MaxAIModelPricesBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAIModelPricesBodyReadError(ctx, rw, err)
		return
	}
	var req codersdk.UpsertOrganizationAIModelPriceRequest
	keys, err := decodeOrganizationAIModelPriceBody(body, &req)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Request body must be valid JSON.",
			Detail:  err.Error(),
		})
		return
	}

	//nolint:gocritic // Org admins may lack provider read; the update was authorized above.
	validations, err := validateOrganizationAIModelPrice(dbauthz.AsSystemRestricted(ctx), api.Database, req, keys)
	if err != nil {
		api.Logger.Error(ctx, "validate organization ai model price", slog.Error(err),
			slog.F("organization_id", org.ID))
		httpapi.InternalServerError(rw, err)
		return
	}
	if len(validations) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid AI model price.",
			Validations: validations,
		})
		return
	}

	err = api.Database.UpsertOrganizationAIProviderModelPrice(ctx, database.UpsertOrganizationAIProviderModelPriceParams{
		OrganizationID:  org.ID,
		ProviderID:      req.ProviderID,
		Model:           req.Model,
		InputPrice:      nullInt64(req.InputPrice),
		OutputPrice:     nullInt64(req.OutputPrice),
		CacheReadPrice:  nullInt64(req.CacheReadPrice),
		CacheWritePrice: nullInt64(req.CacheWritePrice),
	})
	if dbauthz.IsNotAuthorizedError(err) {
		httpapi.Forbidden(rw)
		return
	}
	if err != nil {
		api.Logger.Error(ctx, "upsert organization ai model price", slog.Error(err),
			slog.F("organization_id", org.ID),
			slog.F("user_id", httpmw.APIKey(r).UserID),
			slog.F("provider_id", req.ProviderID),
			slog.F("model", req.Model),
		)
		httpapi.InternalServerError(rw, err)
		return
	}

	// TODO(ssncferreira): replace with audit logging once ai_model_price is an
	// auditable resource (AIGOV-590).
	api.Logger.Info(ctx, "organization ai model price updated",
		slog.F("organization_id", org.ID),
		slog.F("user_id", httpmw.APIKey(r).UserID),
		slog.F("provider_id", req.ProviderID),
		slog.F("model", req.Model),
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

// decodeOrganizationAIModelPriceBody decodes an organization upsert body into
// req, and returns the keys present in it.
func decodeOrganizationAIModelPriceBody(body []byte, req *codersdk.UpsertOrganizationAIModelPriceRequest) (aiModelPriceKeys, error) {
	if err := json.Unmarshal(body, req); err != nil {
		return nil, xerrors.Errorf("decode price: %w", err)
	}
	var keys aiModelPriceKeys
	if err := json.Unmarshal(body, &keys); err != nil {
		return nil, xerrors.Errorf("decode price keys: %w", err)
	}
	return keys, nil
}

// validateOrganizationAIModelPrice reports every problem with the requested
// organization price: a live provider_id, a model, and valid prices. It
// returns an error only when the provider lookup fails.
func validateOrganizationAIModelPrice(ctx context.Context, db database.Store, req codersdk.UpsertOrganizationAIModelPriceRequest, keys aiModelPriceKeys) ([]codersdk.ValidationError, error) {
	var validations []codersdk.ValidationError

	// The provider and model identify the row, so report them first.
	if req.ProviderID == uuid.Nil {
		validations = append(validations, codersdk.ValidationError{
			Field:  "provider_id",
			Detail: "Provider ID is required.",
		})
	} else {
		_, err := db.GetAIProviderByID(ctx, req.ProviderID)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			validations = append(validations, codersdk.ValidationError{
				Field:  "provider_id",
				Detail: fmt.Sprintf("Provider %s does not exist.", req.ProviderID),
			})
		case err != nil:
			return nil, xerrors.Errorf("get provider %s: %w", req.ProviderID, err)
		}
	}
	if req.Model == "" {
		validations = append(validations, codersdk.ValidationError{
			Field:  "model",
			Detail: "Model is required.",
		})
	}
	validations = append(validations, validateAIModelPriceValues("", keys,
		req.InputPrice, req.OutputPrice, req.CacheReadPrice, req.CacheWritePrice)...)

	return validations, nil
}

// nullInt64 converts an optional price to its column value.
func nullInt64(v *int64) sql.NullInt64 {
	if v == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *v, Valid: true}
}

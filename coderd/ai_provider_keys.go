package coderd

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Create an AI provider key
// @ID create-an-ai-provider-key
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags AI Providers
// @Param idOrName path string true "Provider ID or name"
// @Param request body codersdk.CreateAIProviderKeyRequest true "Create AI provider key request"
// @Success 201 {object} codersdk.AIProviderKey
// @Router /api/v2/ai/providers/{idOrName}/keys [post]
func (api *API) aiProviderKeysCreate(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.AIProviderKey](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionCreate,
		})
	)
	defer commitAudit()

	provider, err := lookupAIProvider(ctx, api.Database, chi.URLParam(r, "idOrName"))
	if err != nil {
		writeAIProviderLookupError(ctx, api.Logger, rw, err)
		return
	}

	var req codersdk.CreateAIProviderKeyRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	if validations := req.Validate(); len(validations) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid AI provider key request.",
			Validations: validations,
		})
		return
	}

	// Bedrock providers authenticate via the settings blob, not via a
	// bearer key, so registering a key would be silently unused.
	if isBedrockProvider(provider) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Bedrock providers do not accept API keys; configure access credentials via the provider settings.",
		})
		return
	}

	now := dbtime.Now()
	row, err := api.Database.InsertAIProviderKey(ctx, database.InsertAIProviderKeyParams{
		ID:         uuid.New(),
		ProviderID: provider.ID,
		APIKey:     req.APIKey,
		// ApiKeyKeyID is set by the dbcrypt wrapper.
		ApiKeyKeyID: sql.NullString{},
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		if dbauthz.IsNotAuthorizedError(err) {
			api.Logger.Error(ctx, "create AI provider key", slog.F("provider_id", provider.ID), slog.Error(err))
			httpapi.Forbidden(rw)
			return
		}
		api.Logger.Error(ctx, "create AI provider key", slog.F("provider_id", provider.ID), slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error creating AI provider key.",
			Detail:  err.Error(),
		})
		return
	}
	aReq.New = row

	httpapi.Write(ctx, rw, http.StatusCreated, db2sdk.AIProviderKey(row))
}

// @Summary Delete an AI provider key
// @ID delete-an-ai-provider-key
// @Security CoderSessionToken
// @Tags AI Providers
// @Param idOrName path string true "Provider ID or name"
// @Param keyID path string true "Key ID" format(uuid)
// @Success 204
// @Router /api/v2/ai/providers/{idOrName}/keys/{keyID} [delete]
func (api *API) aiProviderKeysDelete(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.AIProviderKey](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionDelete,
		})
	)
	defer commitAudit()

	provider, err := lookupAIProvider(ctx, api.Database, chi.URLParam(r, "idOrName"))
	if err != nil {
		writeAIProviderLookupError(ctx, api.Logger, rw, err)
		return
	}

	keyID, err := uuid.Parse(chi.URLParam(r, "keyID"))
	if err != nil {
		httpapi.ResourceNotFound(rw)
		return
	}

	existing, err := api.Database.GetAIProviderKeyByID(ctx, keyID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.ResourceNotFound(rw)
			return
		}
		if dbauthz.IsNotAuthorizedError(err) {
			api.Logger.Error(ctx, "fetch AI provider key", slog.F("key_id", keyID), slog.Error(err))
			httpapi.Forbidden(rw)
			return
		}
		api.Logger.Error(ctx, "fetch AI provider key", slog.F("key_id", keyID), slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching AI provider key.",
			Detail:  err.Error(),
		})
		return
	}
	if existing.ProviderID != provider.ID {
		// Don't leak the existence of a key under a different provider.
		httpapi.ResourceNotFound(rw)
		return
	}
	aReq.Old = existing

	if err := api.Database.DeleteAIProviderKey(ctx, keyID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			rw.WriteHeader(http.StatusNoContent)
			return
		}
		if dbauthz.IsNotAuthorizedError(err) {
			api.Logger.Error(ctx, "delete AI provider key", slog.F("key_id", keyID), slog.Error(err))
			httpapi.Forbidden(rw)
			return
		}
		api.Logger.Error(ctx, "delete AI provider key", slog.F("key_id", keyID), slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error deleting AI provider key.",
			Detail:  err.Error(),
		})
		return
	}

	rw.WriteHeader(http.StatusNoContent)
}

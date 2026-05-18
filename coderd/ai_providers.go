package coderd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
)

// aiProvidersHandler registers the CRUD HTTP routes for runtime AI
// provider configuration at /api/v2/ai/providers.
func aiProvidersHandler(api *API, middlewares ...func(http.Handler) http.Handler) func(r chi.Router) {
	return func(r chi.Router) {
		r.Use(middlewares...)
		r.Get("/", api.aiProvidersList)
		r.Post("/", api.aiProvidersCreate)
		r.Route("/{idOrName}", func(r chi.Router) {
			r.Get("/", api.aiProvidersGet)
			r.Patch("/", api.aiProvidersUpdate)
			r.Delete("/", api.aiProvidersDelete)
		})
	}
}

// @Summary List AI providers
// @ID list-ai-providers
// @Security CoderSessionToken
// @Produce json
// @Tags AI Providers
// @Success 200 {array} codersdk.AIProvider
// @Router /api/v2/ai/providers [get]
func (api *API) aiProvidersList(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	rows, err := api.Database.GetAIProviders(ctx, database.GetAIProvidersParams{
		IncludeDisabled: true,
	})
	if dbauthz.IsNotAuthorizedError(err) {
		api.Logger.Error(ctx, "list AI providers", slog.Error(err))
		httpapi.Forbidden(rw)
		return
	}
	if err != nil {
		api.Logger.Error(ctx, "list AI providers", slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error listing AI providers.",
			Detail:  err.Error(),
		})
		return
	}

	keysByProvider, err := loadAIProviderKeysByProvider(ctx, api.Database)
	if err != nil {
		api.Logger.Error(ctx, "list AI provider keys", slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error loading AI provider keys.",
			Detail:  err.Error(),
		})
		return
	}

	out := make([]codersdk.AIProvider, 0, len(rows))
	for _, row := range rows {
		sdk, err := db2sdk.AIProvider(row, keysByProvider[row.ID])
		if err != nil {
			api.Logger.Error(ctx, "convert AI provider", slog.F("provider_id", row.ID), slog.Error(err))
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error converting AI provider.",
				Detail:  err.Error(),
			})
			return
		}
		out = append(out, sdk)
	}
	httpapi.Write(ctx, rw, http.StatusOK, out)
}

// @Summary Get an AI provider
// @ID get-an-ai-provider
// @Security CoderSessionToken
// @Produce json
// @Tags AI Providers
// @Param idOrName path string true "Provider ID or name"
// @Success 200 {object} codersdk.AIProvider
// @Router /api/v2/ai/providers/{idOrName} [get]
func (api *API) aiProvidersGet(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	row, err := lookupAIProvider(ctx, api.Database, chi.URLParam(r, "idOrName"))
	if err != nil {
		writeAIProviderLookupError(ctx, api.Logger, rw, err)
		return
	}

	keys, err := api.Database.GetAIProviderKeysByProviderID(ctx, row.ID)
	if err != nil {
		api.Logger.Error(ctx, "fetch AI provider keys", slog.F("provider_id", row.ID), slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error loading AI provider keys.",
			Detail:  err.Error(),
		})
		return
	}

	sdk, err := db2sdk.AIProvider(row, keys)
	if err != nil {
		api.Logger.Error(ctx, "convert AI provider", slog.F("provider_id", row.ID), slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error converting AI provider.",
			Detail:  err.Error(),
		})
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, sdk)
}

// @Summary Create an AI provider
// @ID create-an-ai-provider
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags AI Providers
// @Param request body codersdk.CreateAIProviderRequest true "Create AI provider request"
// @Success 201 {object} codersdk.AIProvider
// @Router /api/v2/ai/providers [post]
func (api *API) aiProvidersCreate(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.AIProvider](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionCreate,
		})
	)
	defer commitAudit()

	var req codersdk.CreateAIProviderRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	if validations := req.Validate(); len(validations) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid AI provider request.",
			Validations: validations,
		})
		return
	}

	// Bedrock providers authenticate via the settings blob, not via a
	// bearer key, so registering an api_keys list against them would
	// be silently unused.
	if req.Settings.Bedrock != nil && len(req.APIKeys) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Bedrock providers do not accept api_keys; configure access credentials via settings.",
		})
		return
	}

	settings, err := encodeAIProviderSettings(req.Settings)
	if err != nil {
		api.Logger.Error(ctx, "encode AI provider settings", slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error encoding settings.",
			Detail:  err.Error(),
		})
		return
	}

	var (
		row  database.AIProvider
		keys []database.AIProviderKey
	)
	err = api.Database.InTx(func(tx database.Store) error {
		var txErr error
		row, txErr = tx.InsertAIProvider(ctx, database.InsertAIProviderParams{
			ID:          uuid.New(),
			Type:        database.AIProviderType(req.Type),
			Name:        req.Name,
			DisplayName: sql.NullString{String: req.DisplayName, Valid: req.DisplayName != ""},
			Enabled:     req.Enabled,
			BaseUrl:     req.BaseURL,
			Settings:    settings,
			// SettingsKeyID is set by the dbcrypt wrapper.
			SettingsKeyID: sql.NullString{},
		})
		if txErr != nil {
			return txErr
		}

		keys, txErr = insertAIProviderKeys(ctx, tx, row.ID, req.APIKeys)
		return txErr
	}, &database.TxOptions{TxIdentifier: "create_ai_provider"})
	if err != nil {
		if database.IsUniqueViolation(err) {
			api.Logger.Warn(ctx, "create AI provider: duplicate name", slog.F("name", req.Name), slog.Error(err))
			httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
				Message: fmt.Sprintf("AI provider %q already exists.", req.Name),
				Detail:  err.Error(),
			})
			return
		}
		if dbauthz.IsNotAuthorizedError(err) {
			api.Logger.Error(ctx, "create AI provider", slog.Error(err))
			httpapi.Forbidden(rw)
			return
		}
		api.Logger.Error(ctx, "create AI provider", slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error creating AI provider.",
			Detail:  err.Error(),
		})
		return
	}
	aReq.New = row

	sdk, err := db2sdk.AIProvider(row, keys)
	if err != nil {
		api.Logger.Error(ctx, "convert AI provider", slog.F("provider_id", row.ID), slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error converting AI provider.",
			Detail:  err.Error(),
		})
		return
	}
	httpapi.Write(ctx, rw, http.StatusCreated, sdk)
}

// @Summary Update an AI provider
// @ID update-an-ai-provider
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags AI Providers
// @Param idOrName path string true "Provider ID or name"
// @Param request body codersdk.UpdateAIProviderRequest true "Update AI provider request"
// @Success 200 {object} codersdk.AIProvider
// @Router /api/v2/ai/providers/{idOrName} [patch]
func (api *API) aiProvidersUpdate(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.AIProvider](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionWrite,
		})
	)
	defer commitAudit()

	var req codersdk.UpdateAIProviderRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	if req.IsEmpty() {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "At least one field must be provided.",
		})
		return
	}
	if validations := req.Validate(); len(validations) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid AI provider request.",
			Validations: validations,
		})
		return
	}

	idOrName := chi.URLParam(r, "idOrName")

	var (
		updated database.AIProvider
		keys    []database.AIProviderKey
	)
	err := api.Database.InTx(func(tx database.Store) error {
		old, err := lookupAIProvider(ctx, tx, idOrName)
		if err != nil {
			return err
		}
		aReq.Old = old

		// Decode the existing settings to merge with the patch. The dbcrypt
		// wrapper has already decrypted the blob for us.
		existing, err := db2sdk.AIProviderSettings(old.Settings)
		if err != nil {
			return xerrors.Errorf("decode existing settings: %w", err)
		}
		if req.Settings != nil {
			existing = mergeAIProviderSettings(existing, *req.Settings)
		}
		settings, err := encodeAIProviderSettings(existing)
		if err != nil {
			return xerrors.Errorf("encode settings: %w", err)
		}

		// Reject keys against Bedrock providers (whether the existing
		// row is Bedrock or the patch would make it so).
		if req.APIKeys != nil && existing.Bedrock != nil && len(*req.APIKeys) > 0 {
			return errBedrockRejectsAPIKeys
		}

		displayName := old.DisplayName
		if req.DisplayName != nil {
			// Empty string clears the column.
			displayName = sql.NullString{String: *req.DisplayName, Valid: *req.DisplayName != ""}
		}
		params := database.UpdateAIProviderParams{
			ID:          old.ID,
			DisplayName: displayName,
			Enabled:     ptr.NilToDefault(req.Enabled, old.Enabled),
			BaseUrl:     ptr.NilToDefault(req.BaseURL, old.BaseUrl),
			Settings:    settings,
			// SettingsKeyID is set by the dbcrypt wrapper.
			SettingsKeyID: sql.NullString{},
		}

		updated, err = tx.UpdateAIProvider(ctx, params)
		if err != nil {
			return xerrors.Errorf("update ai provider: %w", err)
		}
		aReq.New = updated

		if req.APIKeys != nil {
			keys, err = replaceAIProviderKeys(ctx, tx, updated.ID, *req.APIKeys)
			if err != nil {
				return xerrors.Errorf("replace ai provider keys: %w", err)
			}
			return nil
		}

		keys, err = tx.GetAIProviderKeysByProviderID(ctx, updated.ID)
		if err != nil {
			return xerrors.Errorf("load ai provider keys: %w", err)
		}
		return nil
	}, &database.TxOptions{TxIdentifier: "update_ai_provider"})
	if errors.Is(err, errBedrockRejectsAPIKeys) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Bedrock providers do not accept api_keys; configure access credentials via settings.",
		})
		return
	}
	if err != nil {
		writeAIProviderLookupError(ctx, api.Logger, rw, err)
		return
	}

	sdk, err := db2sdk.AIProvider(updated, keys)
	if err != nil {
		api.Logger.Error(ctx, "convert AI provider", slog.F("provider_id", updated.ID), slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error converting AI provider.",
			Detail:  err.Error(),
		})
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, sdk)
}

// @Summary Delete an AI provider
// @ID delete-an-ai-provider
// @Security CoderSessionToken
// @Tags AI Providers
// @Param idOrName path string true "Provider ID or name"
// @Success 204
// @Router /api/v2/ai/providers/{idOrName} [delete]
func (api *API) aiProvidersDelete(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.AIProvider](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionDelete,
		})
	)
	defer commitAudit()

	idOrName := chi.URLParam(r, "idOrName")

	row, err := lookupAIProvider(ctx, api.Database, idOrName)
	if err != nil {
		writeAIProviderLookupError(ctx, api.Logger, rw, err)
		return
	}
	aReq.Old = row

	if err := api.Database.DeleteAIProviderByID(ctx, row.ID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Already gone; treat as success for idempotency.
			rw.WriteHeader(http.StatusNoContent)
			return
		}
		if dbauthz.IsNotAuthorizedError(err) {
			api.Logger.Error(ctx, "delete AI provider", slog.F("provider_id", row.ID), slog.Error(err))
			httpapi.Forbidden(rw)
			return
		}
		api.Logger.Error(ctx, "delete AI provider", slog.F("provider_id", row.ID), slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error deleting AI provider.",
			Detail:  err.Error(),
		})
		return
	}

	rw.WriteHeader(http.StatusNoContent)
}

// errBedrockRejectsAPIKeys is the sentinel returned from inside the
// update transaction when a caller attempts to attach api_keys to a
// Bedrock-typed provider; the outer handler translates it into a 400.
var errBedrockRejectsAPIKeys = xerrors.New("bedrock providers do not accept api_keys")

// lookupAIProvider resolves a UUID-or-name path parameter against a Store.
// Soft-deleted providers are not returned; lookup by name searches active
// rows only.
func lookupAIProvider(ctx context.Context, store database.Store, idOrName string) (database.AIProvider, error) {
	if id, err := uuid.Parse(idOrName); err == nil {
		row, err := store.GetAIProviderByID(ctx, id)
		if err != nil {
			return database.AIProvider{}, err
		}
		return row, nil
	}
	if !codersdk.AIProviderNameRegex.MatchString(idOrName) {
		// The regex check protects against accidental/malicious lookups
		// against rows that should be impossible to insert.
		return database.AIProvider{}, sql.ErrNoRows
	}
	return store.GetAIProviderByName(ctx, idOrName)
}

// writeAIProviderLookupError translates lookup errors into the right HTTP
// status code.
func writeAIProviderLookupError(ctx context.Context, logger slog.Logger, rw http.ResponseWriter, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if dbauthz.IsNotAuthorizedError(err) {
		logger.Error(ctx, "lookup AI provider", slog.Error(err))
		httpapi.Forbidden(rw)
		return
	}
	logger.Error(ctx, "lookup AI provider", slog.Error(err))
	httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
		Message: "Internal error fetching AI provider.",
		Detail:  err.Error(),
	})
}

// loadAIProviderKeysByProvider fetches every (non-deleted-provider) key
// in one query and buckets the rows by ProviderID, so a list handler
// can avoid an N+1 fetch.
func loadAIProviderKeysByProvider(ctx context.Context, store database.Store) (map[uuid.UUID][]database.AIProviderKey, error) {
	rows, err := store.GetAIProviderKeys(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID][]database.AIProviderKey, len(rows))
	for _, row := range rows {
		out[row.ProviderID] = append(out[row.ProviderID], row)
	}
	return out, nil
}

// insertAIProviderKeys writes a fresh set of key rows for a provider
// inside a transaction. It returns the inserted rows in insertion
// order so callers can render them in a response.
func insertAIProviderKeys(ctx context.Context, tx database.Store, providerID uuid.UUID, plaintexts []string) ([]database.AIProviderKey, error) {
	out := make([]database.AIProviderKey, 0, len(plaintexts))
	now := dbtime.Now()
	for _, key := range plaintexts {
		row, err := tx.InsertAIProviderKey(ctx, database.InsertAIProviderKeyParams{
			ID:         uuid.New(),
			ProviderID: providerID,
			APIKey:     key,
			// ApiKeyKeyID is set by the dbcrypt wrapper.
			ApiKeyKeyID: sql.NullString{},
			CreatedAt:   now,
			UpdatedAt:   now,
		})
		if err != nil {
			return nil, xerrors.Errorf("insert ai provider key: %w", err)
		}
		out = append(out, row)
	}
	return out, nil
}

// replaceAIProviderKeys atomically swaps the api_keys for a provider:
// it deletes every existing key and inserts the supplied plaintext
// list in the same transaction.
func replaceAIProviderKeys(ctx context.Context, tx database.Store, providerID uuid.UUID, plaintexts []string) ([]database.AIProviderKey, error) {
	existing, err := tx.GetAIProviderKeysByProviderID(ctx, providerID)
	if err != nil {
		return nil, xerrors.Errorf("load existing ai provider keys: %w", err)
	}
	for _, k := range existing {
		if err := tx.DeleteAIProviderKey(ctx, k.ID); err != nil {
			return nil, xerrors.Errorf("delete ai provider key %s: %w", k.ID, err)
		}
	}
	return insertAIProviderKeys(ctx, tx, providerID, plaintexts)
}

// encodeAIProviderSettings serializes a settings value into the
// discriminated JSON form stored in ai_providers.settings. Empty
// settings return an invalid sql.NullString so the row stores SQL NULL
// and skips dbcrypt encryption entirely.
func encodeAIProviderSettings(s codersdk.AIProviderSettings) (sql.NullString, error) {
	if s.IsZero() {
		return sql.NullString{}, nil
	}
	out, err := json.Marshal(s)
	if err != nil {
		return sql.NullString{}, err
	}
	return sql.NullString{String: string(out), Valid: true}, nil
}

// mergeAIProviderSettings overlays a patch onto an existing settings
// value. Write-only fields (Bedrock AccessKey and AccessKeySecret) are
// preserved when the patch leaves them blank so callers can rotate
// non-secret fields without resending the secret.
func mergeAIProviderSettings(existing, patch codersdk.AIProviderSettings) codersdk.AIProviderSettings {
	if patch.Bedrock == nil {
		// Patch carries no type-specific data; treat as a clear.
		return codersdk.AIProviderSettings{}
	}
	merged := *patch.Bedrock
	if existing.Bedrock != nil {
		if merged.AccessKey == "" {
			merged.AccessKey = existing.Bedrock.AccessKey
		}
		if merged.AccessKeySecret == "" {
			merged.AccessKeySecret = existing.Bedrock.AccessKeySecret
		}
	}
	return codersdk.AIProviderSettings{Bedrock: &merged}
}

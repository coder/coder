package coderd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

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
	"github.com/coder/coder/v2/codersdk"
)

// aiProviderNameRegex mirrors the CHECK constraint on ai_providers.name.
// Provider names are lowercase alphanumeric with hyphen separators so they
// are safe in URLs.
var aiProviderNameRegex = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// The on-disk shape of ai_providers.settings is the same discriminated
// JSON form that codersdk.AIProviderSettings serializes to: an object
// carrying _type and _version discriminator keys alongside the
// type-specific fields. The dbcrypt wrapper encrypts the marshaled
// bytes opaquely; this package round-trips through the codersdk type
// directly rather than maintaining a duplicate struct.

// aiProvidersHandler registers the CRUD HTTP routes for runtime AI
// provider configuration at /api/v2/ai/providers, including the keys
// sub-resource at /api/v2/ai/providers/{idOrName}/keys.
func aiProvidersHandler(api *API, middlewares ...func(http.Handler) http.Handler) func(r chi.Router) {
	return func(r chi.Router) {
		r.Use(middlewares...)
		r.Get("/", api.aiProvidersList)
		r.Post("/", api.aiProvidersCreate)
		r.Route("/{idOrName}", func(r chi.Router) {
			r.Get("/", api.aiProvidersGet)
			r.Patch("/", api.aiProvidersUpdate)
			r.Delete("/", api.aiProvidersDelete)
			r.Route("/keys", func(r chi.Router) {
				r.Post("/", api.aiProviderKeysCreate)
				r.Delete("/{keyID}", api.aiProviderKeysDelete)
			})
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

	out := make([]codersdk.AIProvider, 0, len(rows))
	for _, row := range rows {
		sdk, err := db2sdk.AIProvider(row)
		if err != nil {
			api.Logger.Error(ctx, "convert AI provider", slog.F("id", row.ID), slog.Error(err))
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

	sdk, err := db2sdk.AIProvider(row)
	if err != nil {
		api.Logger.Error(ctx, "convert AI provider", slog.F("id", row.ID), slog.Error(err))
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
		auditor           = api.AGPL.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.AIProvider](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.AGPL.Logger,
			Request: r,
			Action:  database.AuditActionCreate,
		})
	)
	defer commitAudit()

	var req codersdk.CreateAIProviderRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	if validations := validateCreateAIProviderRequest(req); len(validations) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid AI provider request.",
			Validations: validations,
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

	row, err := api.Database.InsertAIProvider(ctx, database.InsertAIProviderParams{
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

	sdk, err := db2sdk.AIProvider(row)
	if err != nil {
		api.Logger.Error(ctx, "convert AI provider", slog.F("id", row.ID), slog.Error(err))
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
		auditor           = api.AGPL.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.AIProvider](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.AGPL.Logger,
			Request: r,
			Action:  database.AuditActionWrite,
		})
	)
	defer commitAudit()

	var req codersdk.UpdateAIProviderRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	if req.DisplayName == nil && req.Enabled == nil && req.BaseURL == nil && req.Settings == nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "At least one field must be provided.",
		})
		return
	}
	if validations := validateUpdateAIProviderRequest(req); len(validations) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid AI provider request.",
			Validations: validations,
		})
		return
	}

	idOrName := chi.URLParam(r, "idOrName")

	var updated database.AIProvider
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

		params := database.UpdateAIProviderParams{
			ID:            old.ID,
			DisplayName:   nullStrDeref(req.DisplayName, old.DisplayName),
			Enabled:       boolDeref(req.Enabled, old.Enabled),
			BaseUrl:       strDeref(req.BaseURL, old.BaseUrl),
			Settings:      settings,
			SettingsKeyID: sql.NullString{},
		}

		updated, err = tx.UpdateAIProvider(ctx, params)
		if err != nil {
			return xerrors.Errorf("update ai provider: %w", err)
		}
		aReq.New = updated
		return nil
	}, nil)
	if err != nil {
		writeAIProviderLookupError(ctx, api.Logger, rw, err)
		return
	}

	sdk, err := db2sdk.AIProvider(updated)
	if err != nil {
		api.Logger.Error(ctx, "convert AI provider", slog.F("id", updated.ID), slog.Error(err))
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
		auditor           = api.AGPL.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.AIProvider](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.AGPL.Logger,
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
			api.Logger.Error(ctx, "delete AI provider", slog.F("id", row.ID), slog.Error(err))
			httpapi.Forbidden(rw)
			return
		}
		api.Logger.Error(ctx, "delete AI provider", slog.F("id", row.ID), slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error deleting AI provider.",
			Detail:  err.Error(),
		})
		return
	}

	rw.WriteHeader(http.StatusNoContent)
}

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
		auditor           = api.AGPL.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.AIProviderKey](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.AGPL.Logger,
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

	if strings.TrimSpace(req.APIKey) == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid AI provider key request.",
			Validations: []codersdk.ValidationError{
				{Field: "api_key", Detail: "api_key is required"},
			},
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
		ID:          uuid.New(),
		ProviderID:  provider.ID,
		APIKey:      req.APIKey,
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

	httpapi.Write(ctx, rw, http.StatusCreated, dbAIProviderKeyToSDK(row))
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
		auditor           = api.AGPL.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.AIProviderKey](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.AGPL.Logger,
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
	if !aiProviderNameRegex.MatchString(idOrName) {
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

// isBedrockProvider returns true when the row's settings contain a
// Bedrock discriminator. Malformed JSON is treated as "not Bedrock"
// because a row that cannot be decoded cannot be served either way.
func isBedrockProvider(row database.AIProvider) bool {
	s, err := db2sdk.AIProviderSettings(row.Settings)
	if err != nil {
		return false
	}
	return s.Bedrock != nil
}

// validateCreateAIProviderRequest returns the field-level validation errors
// for a create request. An empty slice indicates the request is valid.
func validateCreateAIProviderRequest(req codersdk.CreateAIProviderRequest) []codersdk.ValidationError {
	var validations []codersdk.ValidationError
	switch req.Type {
	case codersdk.AIProviderTypeOpenAI, codersdk.AIProviderTypeAnthropic:
	case "":
		validations = append(validations, codersdk.ValidationError{Field: "type", Detail: "type is required"})
	default:
		validations = append(validations, codersdk.ValidationError{
			Field:  "type",
			Detail: fmt.Sprintf("unsupported provider type %q; expected one of: openai, anthropic", req.Type),
		})
	}
	if errs := validateAIProviderName(req.Name); len(errs) > 0 {
		validations = append(validations, errs...)
	}
	if req.BaseURL == "" {
		validations = append(validations, codersdk.ValidationError{Field: "base_url", Detail: "base_url is required"})
	} else if errs := validateAIProviderBaseURL(req.BaseURL); len(errs) > 0 {
		validations = append(validations, errs...)
	}
	return validations
}

// validateUpdateAIProviderRequest validates only the fields that were
// supplied in the request.
func validateUpdateAIProviderRequest(req codersdk.UpdateAIProviderRequest) []codersdk.ValidationError {
	var validations []codersdk.ValidationError
	if req.BaseURL != nil {
		if *req.BaseURL == "" {
			validations = append(validations, codersdk.ValidationError{Field: "base_url", Detail: "base_url cannot be empty"})
		} else if errs := validateAIProviderBaseURL(*req.BaseURL); len(errs) > 0 {
			validations = append(validations, errs...)
		}
	}
	return validations
}

func validateAIProviderName(name string) []codersdk.ValidationError {
	var validations []codersdk.ValidationError
	switch {
	case name == "":
		validations = append(validations, codersdk.ValidationError{Field: "name", Detail: "name is required"})
	case !aiProviderNameRegex.MatchString(name):
		validations = append(validations, codersdk.ValidationError{
			Field:  "name",
			Detail: "name must match ^[a-z0-9]+(-[a-z0-9]+)*$ (lowercase alphanumeric, hyphens between words)",
		})
	}
	return validations
}

func validateAIProviderBaseURL(raw string) []codersdk.ValidationError {
	var validations []codersdk.ValidationError
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		validations = append(validations, codersdk.ValidationError{
			Field:  "base_url",
			Detail: "base_url must be an absolute URL (e.g. https://api.example.com/)",
		})
		return validations
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		validations = append(validations, codersdk.ValidationError{
			Field:  "base_url",
			Detail: fmt.Sprintf("base_url scheme must be http or https, got %q", parsed.Scheme),
		})
	}
	return validations
}

// dbAIProviderKeyToSDK converts an ai_provider_keys row into the codersdk
// shape. The plaintext api_key is intentionally not included.
func dbAIProviderKeyToSDK(row database.AIProviderKey) codersdk.AIProviderKey {
	return codersdk.AIProviderKey{
		ID:         row.ID,
		ProviderID: row.ProviderID,
		CreatedAt:  row.CreatedAt,
		UpdatedAt:  row.UpdatedAt,
	}
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
	if patch.Bedrock != nil {
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
	// Patch carries no type-specific data; treat as a clear.
	return codersdk.AIProviderSettings{}
}

func strDeref(p *string, fallback string) string {
	if p == nil {
		return fallback
	}
	return *p
}

// nullStrDeref returns fallback unchanged when p is nil; otherwise it
// returns a sql.NullString built from *p, treating an empty string as
// SQL NULL so callers can clear the column by sending "".
func nullStrDeref(p *string, fallback sql.NullString) sql.NullString {
	if p == nil {
		return fallback
	}
	return sql.NullString{String: *p, Valid: *p != ""}
}

func boolDeref(p *bool, fallback bool) bool {
	if p == nil {
		return fallback
	}
	return *p
}

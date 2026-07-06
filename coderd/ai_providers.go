package coderd

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	aibridgeutils "github.com/coder/coder/v2/aibridge/utils"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	coderpubsub "github.com/coder/coder/v2/coderd/pubsub"
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
		writeAIProviderError(ctx, api.Logger, rw, err, "lookup AI provider", "Internal error fetching AI provider.")
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

	// Generate the server-owned external ID when the provider assumes a role.
	ensureBedrockExternalID(&req.Settings)

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

	auditAIProviderKeyChanges(ctx, r, *auditor, api.Logger, aiProviderKeyChanges{Added: keys})
	api.publishAIProvidersChanged(ctx)

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
	// keyOpsAudit attaches per-key add/remove/keep counts to the audit
	// entry. Keys live in a separate table, so a key-only PATCH would
	// otherwise produce an empty diff and hide rotation from the log.
	keyOpsAudit := &aiProviderKeyOpsAudit{}
	var (
		ctx               = r.Context()
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.AIProvider](rw, &audit.RequestParams{
			Audit:            *auditor,
			Log:              api.Logger,
			Request:          r,
			Action:           database.AuditActionWrite,
			AdditionalFields: keyOpsAudit,
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
		updated    database.AIProvider
		keys       []database.AIProviderKey
		keyChanges aiProviderKeyChanges
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
			if err := validateBedrockExternalIDUnchanged(existing, *req.Settings); err != nil {
				return err
			}
			existing = mergeAIProviderSettings(existing, *req.Settings)
		}
		// Bedrock settings are only meaningful for anthropic- or
		// bedrock-typed providers; rejecting the mismatch keeps a
		// misconfiguration from sitting silently in the encrypted
		// blob.
		if existing.Bedrock != nil &&
			old.Type != database.AIProviderTypeAnthropic &&
			old.Type != database.AIProviderTypeBedrock {
			return errAIProviderBedrockTypeMismatch
		}
		// Generate the server-owned external ID when the provider assumes a role
		// and lacks one.
		ensureBedrockExternalID(&existing)
		settings, err := encodeAIProviderSettings(existing)
		if err != nil {
			return xerrors.Errorf("encode settings: %w", err)
		}

		// Reject keys against Bedrock providers (whether the existing
		// row is Bedrock or the patch would make it so).
		if req.APIKeys != nil && existing.Bedrock != nil && len(*req.APIKeys) > 0 {
			return errBedrockRejectsAPIKeys
		}

		if req.APIKeys != nil && old.Type == database.AIProviderTypeCopilot && len(*req.APIKeys) > 0 {
			return errCopilotRejectsAPIKeys
		}

		displayName := old.DisplayName
		if req.DisplayName != nil {
			// Empty string clears the column.
			displayName = sql.NullString{String: *req.DisplayName, Valid: *req.DisplayName != ""}
		}
		params := database.UpdateAIProviderParams{
			ID:          old.ID,
			Type:        old.Type,
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
			var ops aiProviderKeyOpsAudit
			keys, ops, keyChanges, err = applyAIProviderKeyOps(ctx, tx, updated.ID, *req.APIKeys)
			if err != nil {
				return err
			}
			*keyOpsAudit = ops
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
	if errors.Is(err, errCopilotRejectsAPIKeys) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Copilot providers do not accept api_keys; they authenticate via request-time GitHub OAuth tokens.",
		})
		return
	}
	if errors.Is(err, errAIProviderBedrockTypeMismatch) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Bedrock settings are only valid for type=anthropic or type=bedrock.",
		})
		return
	}
	if errors.Is(err, errAIProviderExternalIDReadOnly) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "The Bedrock external ID is server-generated and cannot be changed.",
		})
		return
	}
	if errors.Is(err, errAIProviderKeyUnknown) {
		// Use the sentinel directly so the response message does not
		// leak the "execute transaction:" wrapper xerrors added on the
		// way out of InTx.
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: errAIProviderKeyUnknown.Error(),
			Detail:  err.Error(),
		})
		return
	}
	if err != nil {
		writeAIProviderError(ctx, api.Logger, rw, err, "update AI provider", "Internal error updating AI provider.")
		return
	}

	auditAIProviderKeyChanges(ctx, r, *auditor, api.Logger, keyChanges)
	api.publishAIProvidersChanged(ctx)

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

	err := api.Database.InTx(func(tx database.Store) error {
		row, err := lookupAIProvider(ctx, tx, idOrName)
		if err != nil {
			return err
		}
		aReq.Old = row

		// Soft-delete UPDATE; :exec, so re-deletion is a silent no-op.
		if err := tx.DeleteAIProviderByID(ctx, row.ID); err != nil {
			return xerrors.Errorf("delete ai provider: %w", err)
		}
		return nil
	}, &database.TxOptions{TxIdentifier: "delete_ai_provider"})
	if err != nil {
		writeAIProviderError(ctx, api.Logger, rw, err, "delete AI provider", "Internal error deleting AI provider.")
		return
	}

	api.publishAIProvidersChanged(ctx)

	rw.WriteHeader(http.StatusNoContent)
}

// publishAIProvidersChanged notifies subscribers (aibridged,
// aibridgeproxyd, chatd) that the live provider set changed and they
// should refetch from the database. Pubsub failures are logged but not
// propagated: subscribers refresh authoritatively from the DB, so a
// dropped notification only delays convergence.
func (api *API) publishAIProvidersChanged(ctx context.Context) {
	if api.Pubsub == nil {
		return
	}
	if err := api.Pubsub.Publish(coderpubsub.AIProvidersChangedChannel, nil); err != nil {
		api.Logger.Warn(ctx, "publish ai providers changed event", slog.Error(err))
	}
}

// errBedrockRejectsAPIKeys is the sentinel returned from inside the
// update transaction when a caller attempts to attach api_keys to a
// Bedrock-typed provider; the outer handler translates it into a 400.
var errBedrockRejectsAPIKeys = xerrors.New("bedrock providers do not accept api_keys")

// errCopilotRejectsAPIKeys is the sentinel returned from inside the
// update transaction when a caller attempts to attach api_keys to a
// Copilot-typed provider; the outer handler translates it into a 400.
// Copilot authenticates via request-time GitHub OAuth tokens.
var errCopilotRejectsAPIKeys = xerrors.New("copilot providers do not accept api_keys")

// errAIProviderBedrockTypeMismatch is the sentinel returned from
// inside the update transaction when the post-merge settings carry a
// Bedrock block but the provider is not anthropic- or bedrock-typed;
// the outer handler translates it into a 400.
var errAIProviderBedrockTypeMismatch = xerrors.New("bedrock settings are only valid for type=anthropic or type=bedrock")

// errAIProviderExternalIDReadOnly is the sentinel returned from inside
// the update transaction when a patch tries to change the server-owned
// Bedrock external ID; the outer handler translates it into a 400. A
// patch may echo the stored value but not set a different one.
var errAIProviderExternalIDReadOnly = xerrors.New("external_id is server-generated and cannot be changed")

// errAIProviderInvalidName is returned from lookupAIProvider when the
// idOrName parameter is neither a UUID nor a syntactically-valid name.
// The handler translates this into a 400 so an integrator gets a hint
// about the path shape instead of a misleading 404.
var errAIProviderInvalidName = xerrors.New("invalid provider id or name")

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
		// Bail before hitting the DB: the regex matches the CHECK
		// constraint on ai_providers.name, so a non-matching string
		// could not have been inserted.
		return database.AIProvider{}, errAIProviderInvalidName
	}
	return store.GetAIProviderByName(ctx, idOrName)
}

// writeAIProviderError translates an error from the AI provider
// lookup/update/delete paths into the right HTTP status code. logMsg
// labels the log line for operator debugging, and userMsg is the
// internal-error response message shown to the API consumer when no
// more specific branch fires.
func writeAIProviderError(ctx context.Context, logger slog.Logger, rw http.ResponseWriter, err error, logMsg, userMsg string) {
	if errors.Is(err, errAIProviderInvalidName) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Invalid provider id or name: must be a UUID or match %s.", codersdk.AIProviderNameRegex),
		})
		return
	}
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if dbauthz.IsNotAuthorizedError(err) {
		logger.Error(ctx, logMsg, slog.Error(err))
		httpapi.Forbidden(rw)
		return
	}
	logger.Error(ctx, logMsg, slog.Error(err))
	httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
		Message: userMsg,
		Detail:  err.Error(),
	})
}

// loadAIProviderKeysByProvider fetches keys for every live provider in
// one query and buckets the rows by ProviderID, so the list handler
// can avoid an N+1 fetch. Soft-deleted providers' keys are excluded
// by the query.
func loadAIProviderKeysByProvider(ctx context.Context, store database.Store) (map[uuid.UUID][]database.AIProviderKey, error) {
	rows, err := store.GetAIProviderKeys(ctx, false)
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

// aiProviderKeyOpsAudit is serialized into the audit entry's
// additional_fields. Surfacing the per-key ID and masked secret for
// adds and removes gives operators a precise record of which keys
// rotated on a PATCH whose top-level diff would otherwise look empty.
// Kept is a count: a steady-state rotation commonly retains many keys,
// and per-entry detail there is noise.
type aiProviderKeyOpsAudit struct {
	Added   []aiProviderKeyOp `json:"added"`
	Removed []aiProviderKeyOp `json:"removed"`
	Kept    int               `json:"kept"`
}

// aiProviderKeyOp identifies a single key affected by a PATCH. Masked
// is the one-way rendering produced by aibridgeutils.MaskSecret, so
// plaintext never lands in the audit log.
type aiProviderKeyOp struct {
	ID     uuid.UUID `json:"id"`
	Masked string    `json:"masked"`
}

// aiProviderKeyChanges captures the rows added and removed by
// applyAIProviderKeyOps so the caller can emit one audit entry per
// affected key after the transaction commits.
type aiProviderKeyChanges struct {
	Added   []database.AIProviderKey
	Removed []database.AIProviderKey
}

// auditAIProviderKeyChanges emits one audit entry per added or removed
// key, attributed to the actor on the HTTP request. Per-key entries
// keep key rotation visible in the audit log because the parent
// AIProvider audit diff is empty for key-only PATCHes (keys live in a
// separate table).
//
// APIKey is replaced with the masked rendering before the row reaches
// the audit pipeline so plaintext keys never land in the diff or any
// audit backend, independent of the api_key column's audit policy.
func auditAIProviderKeyChanges(ctx context.Context, r *http.Request, auditor audit.Auditor, log slog.Logger, changes aiProviderKeyChanges) {
	if len(changes.Added) == 0 && len(changes.Removed) == 0 {
		return
	}
	key, ok := httpmw.APIKeyOptional(r)
	if !ok {
		return
	}
	requestID, _ := httpmw.RequestIDOptional(r)
	emit := func(action database.AuditAction, before, after database.AIProviderKey) {
		before.APIKey = aibridgeutils.MaskSecret(before.APIKey)
		after.APIKey = aibridgeutils.MaskSecret(after.APIKey)
		audit.BackgroundAudit(ctx, &audit.BackgroundAuditParams[database.AIProviderKey]{
			Audit:     auditor,
			Log:       log,
			UserID:    key.UserID,
			RequestID: requestID,
			Status:    http.StatusOK,
			IP:        r.RemoteAddr,
			UserAgent: r.UserAgent(),
			Action:    action,
			Old:       before,
			New:       after,
		})
	}
	for _, k := range changes.Removed {
		emit(database.AuditActionDelete, k, database.AIProviderKey{})
	}
	for _, k := range changes.Added {
		emit(database.AuditActionCreate, database.AIProviderKey{}, k)
	}
}

// applyAIProviderKeyOps reconciles a provider's keys against the
// supplied mutation list inside a transaction: kept-by-ID rows stay,
// rows whose ID is absent from the list are deleted, and entries
// carrying a plaintext APIKey are inserted as new rows. Caller is
// responsible for prior validation (XOR per entry, no duplicate IDs).
// IDs that do not belong to this provider return errAIProviderKeyUnknown.
func applyAIProviderKeyOps(ctx context.Context, tx database.Store, providerID uuid.UUID, muts []codersdk.AIProviderKeyMutation) ([]database.AIProviderKey, aiProviderKeyOpsAudit, aiProviderKeyChanges, error) {
	var (
		ops     aiProviderKeyOpsAudit
		changes aiProviderKeyChanges
	)
	existing, err := tx.GetAIProviderKeysByProviderID(ctx, providerID)
	if err != nil {
		return nil, ops, changes, xerrors.Errorf("load existing ai provider keys: %w", err)
	}
	existingByID := make(map[uuid.UUID]struct{}, len(existing))
	for _, k := range existing {
		existingByID[k.ID] = struct{}{}
	}

	keep := make(map[uuid.UUID]struct{}, len(muts))
	var inserts []string
	for _, m := range muts {
		switch {
		case m.ID != nil:
			if _, ok := existingByID[*m.ID]; !ok {
				return nil, ops, changes, xerrors.Errorf("%w: %s", errAIProviderKeyUnknown, *m.ID)
			}
			keep[*m.ID] = struct{}{}
		case m.APIKey != nil:
			inserts = append(inserts, *m.APIKey)
		}
	}

	for _, k := range existing {
		if _, ok := keep[k.ID]; ok {
			continue
		}
		if err := tx.DeleteAIProviderKey(ctx, k.ID); err != nil {
			return nil, ops, changes, xerrors.Errorf("delete ai provider key %s: %w", k.ID, err)
		}
		ops.Removed = append(ops.Removed, aiProviderKeyOp{ID: k.ID, Masked: aibridgeutils.MaskSecret(k.APIKey)})
		changes.Removed = append(changes.Removed, k)
	}

	added, err := insertAIProviderKeys(ctx, tx, providerID, inserts)
	if err != nil {
		return nil, ops, changes, err
	}
	for _, k := range added {
		ops.Added = append(ops.Added, aiProviderKeyOp{ID: k.ID, Masked: aibridgeutils.MaskSecret(k.APIKey)})
	}
	changes.Added = append(changes.Added, added...)
	ops.Kept = len(keep)

	out, err := tx.GetAIProviderKeysByProviderID(ctx, providerID)
	if err != nil {
		return nil, ops, changes, xerrors.Errorf("reload ai provider keys: %w", err)
	}
	return out, ops, changes, nil
}

// errAIProviderKeyUnknown is the sentinel returned by
// applyAIProviderKeyOps when a mutation references an ID that does not
// belong to the provider being patched; the outer handler translates it
// into a 400.
var errAIProviderKeyUnknown = xerrors.New("api_keys references an unknown id for this provider")

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
// value. Write-only fields (Bedrock AccessKey and AccessKeySecret) use
// pointers so the patch can distinguish "omitted, keep existing" (nil)
// from "explicitly clear" (pointer to empty string) - e.g. when an
// admin migrates from static AWS credentials to IAM role-based auth
// in a single PATCH.
func mergeAIProviderSettings(existing, patch codersdk.AIProviderSettings) codersdk.AIProviderSettings {
	if patch.Bedrock == nil {
		// Patch carries no type-specific data; treat as a clear.
		return codersdk.AIProviderSettings{}
	}
	merged := *patch.Bedrock
	if existing.Bedrock != nil {
		if merged.AccessKey == nil {
			merged.AccessKey = existing.Bedrock.AccessKey
		}
		if merged.AccessKeySecret == nil {
			merged.AccessKeySecret = existing.Bedrock.AccessKeySecret
		}
		// The external ID is server-owned and stable: carry the stored value
		// forward so a patch can't change it. A patch that sets a different
		// value is rejected upstream.
		merged.ExternalID = existing.Bedrock.ExternalID
	}
	return codersdk.AIProviderSettings{Bedrock: &merged}
}

// validateBedrockExternalIDUnchanged rejects a patch that sets a Bedrock
// external ID different from the stored one. A patch may echo the stored
// value (read-modify-write resends it) but not change it; the value is
// server-owned.
func validateBedrockExternalIDUnchanged(existing, patch codersdk.AIProviderSettings) error {
	stored := ""
	if existing.Bedrock != nil {
		stored = existing.Bedrock.ExternalID
	}

	provided := ""
	if patch.Bedrock != nil {
		provided = patch.Bedrock.ExternalID
	}

	if provided != "" && provided != stored {
		return errAIProviderExternalIDReadOnly
	}
	return nil
}

// ensureBedrockExternalID assigns a server-owned STS external ID when the
// Bedrock provider assumes a role and none is set yet.
func ensureBedrockExternalID(s *codersdk.AIProviderSettings) {
	if s.Bedrock != nil && s.Bedrock.RoleARN != "" && s.Bedrock.ExternalID == "" {
		s.Bedrock.ExternalID = rand.Text()
	}
}

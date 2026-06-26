package coderd

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/aibridge/keys"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

// nameFormatDetail is the human-readable description of valid key names.
const nameFormatDetail = "Must be 64 characters or fewer, lowercase letters, numbers, and non-consecutive hyphens, cannot start or end with a hyphen."

// @Summary Create AI Gateway key
// @ID create-ai-gateway-key
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Enterprise
// @Param request body codersdk.CreateAIGatewayKeyRequest true "Create AI Gateway key request"
// @Success 201 {object} codersdk.CreateAIGatewayKeyResponse
// @Router /api/v2/ai-gateway/keys [post]
func (api *API) postAIGatewayKey(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		auditor           = api.AGPL.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.AIGatewayKey](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionCreate,
		})
	)
	defer commitAudit()

	var req codersdk.CreateAIGatewayKeyRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	row, secret, err := api.generateAndInsertKey(ctx, req.Name)
	if err != nil {
		writeKeyInsertError(ctx, rw, err)
		return
	}

	aReq.New = database.AIGatewayKey{
		ID:           row.ID,
		Name:         row.Name,
		SecretPrefix: row.SecretPrefix,
		CreatedAt:    row.CreatedAt,
	}

	httpapi.Write(ctx, rw, http.StatusCreated, codersdk.CreateAIGatewayKeyResponse{
		ID:        row.ID,
		Name:      row.Name,
		KeyPrefix: row.SecretPrefix,
		CreatedAt: row.CreatedAt,
		Key:       secret,
	})
}

// generateAndInsertKey creates fresh key material and attempts an insert.
func (api *API) generateAndInsertKey(ctx context.Context, name string) (database.InsertAIGatewayKeyRow, string, error) {
	params, key, err := keys.New(name)
	if err != nil {
		return database.InsertAIGatewayKeyRow{}, "", err
	}
	row, err := api.Database.InsertAIGatewayKey(ctx, params)
	if err != nil {
		return database.InsertAIGatewayKeyRow{}, "", err
	}
	return row, key, nil
}

// writeKeyInsertError maps insert errors to HTTP responses.
func writeKeyInsertError(ctx context.Context, rw http.ResponseWriter, err error) {
	switch {
	case httpapi.IsUnauthorizedError(err):
		httpapi.Forbidden(rw)
	case database.IsCheckViolation(err, database.CheckAIGatewayKeysNameCheck):
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid key name.",
			Validations: []codersdk.ValidationError{
				{Field: "name", Detail: nameFormatDetail},
			},
		})
	case database.IsUniqueViolation(err, database.UniqueAIGatewayKeysNameIndex):
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Key name must be unique.",
			Validations: []codersdk.ValidationError{
				{Field: "name", Detail: "A key with this name already exists."},
			},
		})
	default:
		// Secret collisions (hashed_secret or secret_prefix unique
		// violations, should not happen in practice) and other unexpected errors
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to create key. Please retry.",
		})
	}
}

// @Summary List AI Gateway keys
// @ID list-ai-gateway-keys
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Success 200 {array} codersdk.AIGatewayKey
// @Router /api/v2/ai-gateway/keys [get]
func (api *API) aiGatewayKeys(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	rows, err := api.Database.ListAIGatewayKeys(ctx)
	if httpapi.IsUnauthorizedError(err) {
		httpapi.Forbidden(rw)
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to list keys.",
		})
		return
	}

	out := make([]codersdk.AIGatewayKey, 0, len(rows))
	for _, row := range rows {
		out = append(out, convertAIGatewayKey(row))
	}

	httpapi.Write(ctx, rw, http.StatusOK, out)
}

// @Summary Delete AI Gateway key
// @ID delete-ai-gateway-key
// @Security CoderSessionToken
// @Tags Enterprise
// @Param key path string true "Key ID" format(uuid)
// @Success 204
// @Router /api/v2/ai-gateway/keys/{key} [delete]
func (api *API) deleteAIGatewayKey(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		auditor           = api.AGPL.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.AIGatewayKey](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionDelete,
		})
	)
	defer commitAudit()

	id, err := uuid.Parse(chi.URLParam(r, "key"))
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid key ID.",
			Detail:  err.Error(),
		})
		return
	}

	deleted, err := api.Database.DeleteAIGatewayKey(ctx, id)
	if err != nil {
		if httpapi.IsUnauthorizedError(err) {
			httpapi.Forbidden(rw)
			return
		}
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to delete key.",
		})
		return
	}

	aReq.Old = database.AIGatewayKey{
		ID:           deleted.ID,
		Name:         deleted.Name,
		SecretPrefix: deleted.SecretPrefix,
		CreatedAt:    deleted.CreatedAt,
		LastUsedAt:   deleted.LastUsedAt,
	}

	rw.WriteHeader(http.StatusNoContent)
}

func convertAIGatewayKey(row database.ListAIGatewayKeysRow) codersdk.AIGatewayKey {
	var lastUsed *time.Time
	if row.LastUsedAt.Valid {
		t := row.LastUsedAt.Time
		lastUsed = &t
	}
	return codersdk.AIGatewayKey{
		ID:         row.ID,
		Name:       row.Name,
		KeyPrefix:  row.SecretPrefix,
		CreatedAt:  row.CreatedAt,
		LastUsedAt: lastUsed,
	}
}

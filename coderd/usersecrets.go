package coderd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
)

const (
	// These names are raised by the enforce_user_secrets_per_user_limits
	// trigger with USING CONSTRAINT. They are not table CHECK
	// constraints, so dbgen does not emit them in check_constraint.go.
	userSecretsCountLimitConstraint      database.CheckConstraint = "user_secrets_per_user_count_limit"
	userSecretsTotalBytesLimitConstraint database.CheckConstraint = "user_secrets_per_user_total_bytes_limit"
	userSecretsEnvBytesLimitConstraint   database.CheckConstraint = "user_secrets_per_user_env_bytes_limit"
)

// errUserSecretInjectionTargetRequired signals that a PATCH would leave an
// enabled secret with both env_name and file_path empty. It is returned
// from the patchUserSecret transaction so the handler can map it to a 400.
// Creates enforce the same invariant in
// codersdk.ValidateCreateUserSecretRequest.
var errUserSecretInjectionTargetRequired = xerrors.New("enabled user secret must have at least one of env_name or file_path set")

var (
	errUserSecretFilePathDisabled  = xerrors.New("file-based user secrets are disabled")
	errUserSecretEnvTargetRequired = xerrors.New("enabled user secret requires env_name")
)

const (
	//nolint:gosec // User-facing validation text.
	userSecretFilePathDisabledDetail = "File path delivery for user secrets is disabled by the deployment administrator. Clear file_path or use env_name instead."
	//nolint:gosec // User-facing validation text.
	userSecretEnvTargetRequiredDetail = "File path delivery for user secrets is disabled by the deployment administrator, so an enabled secret must have an env_name. Add env_name, or set enabled to false."
)

// userSecretFilePathBlocked reports whether the deployment forbids file path
// delivery for user secrets.
func (api *API) userSecretFilePathBlocked() bool {
	return api.DeploymentValues != nil && api.DeploymentValues.DisableUserSecretFilePath.Value()
}

//nolint:revive // blocked is deployment configuration, not caller control coupling.
func userSecretCreateValidationErrors(req codersdk.CreateUserSecretRequest, blocked bool) []codersdk.ValidationError {
	validations := codersdk.ValidateCreateUserSecretRequest(req)
	if !blocked {
		return validations
	}

	if req.FilePath != "" {
		validations = append(validations, codersdk.ValidationError{
			Field:  codersdk.UserSecretFilePathField,
			Detail: userSecretFilePathDisabledDetail,
		})
	}
	if (req.Enabled != nil && !*req.Enabled) || req.EnvName != "" {
		return validations
	}

	for i := range validations {
		if validations[i].Field == codersdk.UserSecretEnvNameField && validations[i].Detail == codersdk.UserSecretInjectionTargetRequiredDetail {
			validations[i].Detail = userSecretEnvTargetRequiredDetail
		}
	}
	return validations
}

func prefixUserSecretValidationErrors(index int, validations []codersdk.ValidationError) []codersdk.ValidationError {
	if index < 0 {
		return validations
	}
	prefixed := make([]codersdk.ValidationError, 0, len(validations))
	for _, v := range validations {
		prefixed = append(prefixed, codersdk.ValidationError{
			Field:  fmt.Sprintf("secrets[%d].%s", index, v.Field),
			Detail: v.Detail,
		})
	}
	return prefixed
}

// Existing enabled file-only rows may be edited or disabled, but a PATCH
// cannot add or change a path or enter a new enabled state without env_name.
func userSecretFilePathPolicyError(old database.UserSecret, req codersdk.UpdateUserSecretRequest) error {
	if req.FilePath != nil && *req.FilePath != "" && *req.FilePath != old.FilePath {
		return errUserSecretFilePathDisabled
	}

	postEnvName := old.EnvName
	if req.EnvName != nil {
		postEnvName = *req.EnvName
	}
	postEnabled := old.Enabled
	if req.Enabled != nil {
		postEnabled = *req.Enabled
	}
	if postEnabled && postEnvName == "" && req.FilePath != nil && *req.FilePath == "" {
		return errUserSecretEnvTargetRequired
	}

	predatesPolicy := old.Enabled && old.EnvName == ""
	if postEnabled && postEnvName == "" && !predatesPolicy {
		return errUserSecretEnvTargetRequired
	}
	return nil
}

// @Summary Create a new user secret
// @ID create-a-new-user-secret
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Secrets
// @Param user path string true "User ID, username, or me"
// @Param request body codersdk.CreateUserSecretRequest true "Create secret request"
// @Success 201 {object} codersdk.UserSecret
// @Failure 400 {object} codersdk.Response
// @Failure 409 {object} codersdk.Response
// @Router /api/v2/users/{user}/secrets [post]
func (api *API) postUserSecret(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		user              = httpmw.UserParam(r)
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.UserSecret](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionCreate,
		})
	)
	defer commitAudit()

	var req codersdk.CreateUserSecretRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	if validations := userSecretCreateValidationErrors(req, api.userSecretFilePathBlocked()); len(validations) > 0 {
		writeUserSecretValidationErrors(ctx, rw, http.StatusBadRequest, validations)
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	secret, err := api.Database.CreateUserSecret(ctx, database.CreateUserSecretParams{
		ID:          uuid.New(),
		UserID:      user.ID,
		Name:        req.Name,
		Description: req.Description,
		Value:       req.Value,
		ValueKeyID:  sql.NullString{},
		EnvName:     req.EnvName,
		FilePath:    req.FilePath,
		Enabled:     enabled,
	})
	if err != nil {
		if validations := userSecretConflictValidationErrors(err); len(validations) > 0 {
			writeUserSecretValidationErrors(ctx, rw, http.StatusConflict, validations)
			return
		}
		if validations := userSecretInjectionTargetValidationErrors(err); len(validations) > 0 {
			writeUserSecretValidationErrors(ctx, rw, http.StatusBadRequest, validations)
			return
		}
		if resp, ok := userSecretLimitResponse(err); ok {
			httpapi.Write(ctx, rw, http.StatusBadRequest, resp)
			return
		}
		if httpapi.IsUnauthorizedError(err) {
			httpapi.Forbidden(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error creating secret.",
			Detail:  err.Error(),
		})
		return
	}
	aReq.New = secret

	httpapi.Write(ctx, rw, http.StatusCreated, db2sdk.UserSecretFromFull(secret))
}

// @Summary Import user secrets from a file
// @ID import-user-secrets-from-a-file
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Secrets
// @Param user path string true "User ID, username, or me"
// @Param request body codersdk.ImportUserSecretsRequest true "Import secrets request"
// @Success 201 {array} codersdk.UserSecret
// @Failure 400 {object} codersdk.Response
// @Failure 409 {object} codersdk.Response
// @Failure 413 {object} codersdk.Response "Request body exceeds 8 MiB"
// @Router /api/v2/users/{user}/secrets/batch [post]
func (api *API) postUserSecretsBatch(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := httpmw.UserParam(r)

	// Worst-case JSON escaping can inflate a max-size file several-fold, so 8x
	// gives comfortable headroom. This exceeds
	// httpapi.DefaultMaxRequestBodyBytes, so it must be passed to ReadLimit
	// rather than wrapping r.Body here.
	var req codersdk.ImportUserSecretsRequest
	if !httpapi.ReadLimit(ctx, rw, r, 8*codersdk.MaxSecretsFileBytes, &req) {
		return
	}

	reqs, err := codersdk.ParseSecretsFile(req.Format, req.Content)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to parse secrets file.",
			Detail:  err.Error(),
		})
		return
	}

	// Validate every entry and accumulate all errors so the caller can
	// fix the whole file in one round-trip. Each field is prefixed with
	// the entry index, e.g. "secrets[2].env_name".
	filePathBlocked := api.userSecretFilePathBlocked()
	var validations []codersdk.ValidationError
	for i, sreq := range reqs {
		entryValidations := userSecretCreateValidationErrors(sreq, filePathBlocked)
		validations = append(validations, prefixUserSecretValidationErrors(i, entryValidations)...)
	}
	if len(validations) > 0 {
		writeUserSecretValidationErrors(ctx, rw, http.StatusBadRequest, validations)
		return
	}

	// Insert atomically. The per-user-limit trigger fires per row, and
	// any unique or limit violation aborts the whole transaction, so a
	// failed import creates nothing. failedIndex records which entry
	// failed so the error can be attributed to it after the rollback.
	var created []database.UserSecret
	failedIndex := -1
	err = api.Database.InTx(func(tx database.Store) error {
		for i, sreq := range reqs {
			enabled := true
			if sreq.Enabled != nil {
				enabled = *sreq.Enabled
			}
			s, txErr := tx.CreateUserSecret(ctx, database.CreateUserSecretParams{
				ID:          uuid.New(),
				UserID:      user.ID,
				Name:        sreq.Name,
				Description: sreq.Description,
				Value:       sreq.Value,
				ValueKeyID:  sql.NullString{},
				EnvName:     sreq.EnvName,
				FilePath:    sreq.FilePath,
				Enabled:     enabled,
			})
			if txErr != nil {
				failedIndex = i
				return txErr
			}
			created = append(created, s)
		}
		return nil
	}, nil)
	if err != nil {
		index := failedIndex

		if conflicts := userSecretConflictValidationErrors(err); len(conflicts) > 0 {
			writeUserSecretValidationErrors(ctx, rw, http.StatusConflict, prefixUserSecretValidationErrors(index, conflicts))
			return
		}
		if validations := userSecretInjectionTargetValidationErrors(err); len(validations) > 0 {
			writeUserSecretValidationErrors(ctx, rw, http.StatusBadRequest, prefixUserSecretValidationErrors(index, validations))
			return
		}
		if resp, ok := userSecretLimitResponse(err); ok {
			if index >= 0 {
				resp.Detail = fmt.Sprintf("Entry secrets[%d] (%q): %s", index, reqs[index].Name, resp.Detail)
			}
			httpapi.Write(ctx, rw, http.StatusBadRequest, resp)
			return
		}
		if httpapi.IsUnauthorizedError(err) {
			httpapi.Forbidden(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error importing secrets.",
			Detail:  err.Error(),
		})
		return
	}

	// Emit audit logs only after the transaction commits so a rolled-back
	// batch produces zero logs. One create log is emitted per secret
	// because database.UserSecret is registered as auditable.
	auditor := api.Auditor.Load()
	requestID := httpmw.RequestID(r)
	auditCtx := context.WithoutCancel(ctx)
	for _, secret := range created {
		audit.BackgroundAudit(auditCtx, &audit.BackgroundAuditParams[database.UserSecret]{
			Audit:     *auditor,
			Log:       api.Logger,
			UserID:    user.ID,
			RequestID: requestID,
			Status:    http.StatusCreated,
			IP:        r.RemoteAddr,
			UserAgent: r.UserAgent(),
			Action:    database.AuditActionCreate,
			New:       secret,
			Old:       database.UserSecret{},
		})
	}

	out := make([]codersdk.UserSecret, 0, len(created))
	for _, secret := range created {
		out = append(out, db2sdk.UserSecretFromFull(secret))
	}
	httpapi.Write(ctx, rw, http.StatusCreated, out)
}

// @Summary List user secrets
// @ID list-user-secrets
// @Security CoderSessionToken
// @Produce json
// @Tags Secrets
// @Param user path string true "User ID, username, or me"
// @Success 200 {array} codersdk.UserSecret
// @Router /api/v2/users/{user}/secrets [get]
func (api *API) getUserSecrets(rw http.ResponseWriter, r *http.Request) { //nolint:revive // Method name matches route.
	ctx := r.Context()
	user := httpmw.UserParam(r)

	secrets, err := api.Database.ListUserSecrets(ctx, user.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error listing secrets.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.UserSecrets(secrets))
}

// @Summary Get a user secret by name
// @ID get-a-user-secret-by-name
// @Security CoderSessionToken
// @Produce json
// @Tags Secrets
// @Param user path string true "User ID, username, or me"
// @Param name path string true "Secret name"
// @Success 200 {object} codersdk.UserSecret
// @Router /api/v2/users/{user}/secrets/{name} [get]
func (api *API) getUserSecret(rw http.ResponseWriter, r *http.Request) { //nolint:revive // Method name matches route.
	ctx := r.Context()
	user := httpmw.UserParam(r)
	name := chi.URLParam(r, codersdk.UserSecretNameField)

	secret, err := api.Database.GetUserSecretByUserIDAndName(ctx, database.GetUserSecretByUserIDAndNameParams{
		UserID: user.ID,
		Name:   name,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching secret.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.UserSecretFromFull(secret))
}

// @Summary Update a user secret
// @ID update-a-user-secret
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Secrets
// @Param user path string true "User ID, username, or me"
// @Param name path string true "Secret name"
// @Param request body codersdk.UpdateUserSecretRequest true "Update secret request"
// @Success 200 {object} codersdk.UserSecret
// @Failure 400 {object} codersdk.Response
// @Failure 409 {object} codersdk.Response
// @Router /api/v2/users/{user}/secrets/{name} [patch]
func (api *API) patchUserSecret(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		user              = httpmw.UserParam(r)
		name              = chi.URLParam(r, codersdk.UserSecretNameField)
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.UserSecret](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionWrite,
		})
	)
	defer commitAudit()

	var req codersdk.UpdateUserSecretRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	if req.Value == nil && req.Description == nil && req.EnvName == nil && req.FilePath == nil && req.Enabled == nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "At least one field must be provided.",
		})
		return
	}
	if validations := updateUserSecretValidationErrors(req); len(validations) > 0 {
		writeUserSecretValidationErrors(ctx, rw, http.StatusBadRequest, validations)
		return
	}

	params := database.UpdateUserSecretByUserIDAndNameParams{
		UserID:            user.ID,
		Name:              name,
		UpdateValue:       req.Value != nil,
		Value:             "",
		ValueKeyID:        sql.NullString{},
		UpdateDescription: req.Description != nil,
		Description:       "",
		UpdateEnvName:     req.EnvName != nil,
		EnvName:           "",
		UpdateFilePath:    req.FilePath != nil,
		FilePath:          "",
		UpdateEnabled:     req.Enabled != nil,
		Enabled:           false,
	}
	if req.Value != nil {
		params.Value = *req.Value
	}
	if req.Description != nil {
		params.Description = *req.Description
	}
	if req.EnvName != nil {
		params.EnvName = *req.EnvName
	}
	if req.FilePath != nil {
		params.FilePath = *req.FilePath
	}
	if req.Enabled != nil {
		params.Enabled = *req.Enabled
	}

	// Lock before computing post-state so concurrent PATCHes serialize.
	var secret database.UserSecret
	filePathBlocked := api.userSecretFilePathBlocked()
	err := api.Database.InTx(func(tx database.Store) error {
		old, err := tx.GetUserSecretByUserIDAndNameForUpdate(ctx, database.GetUserSecretByUserIDAndNameForUpdateParams{
			UserID: user.ID,
			Name:   name,
		})
		if err != nil {
			return xerrors.Errorf("fetch user secret: %w", err)
		}
		aReq.Old = old

		// Reject patches that would leave an enabled secret with both
		// env_name and file_path empty. Evaluated against the post-update
		// state so atomic env<->file swaps still succeed, and so targets
		// can be cleared when the same PATCH also disables the secret.
		postEnvName := old.EnvName
		if req.EnvName != nil {
			postEnvName = *req.EnvName
		}
		postFilePath := old.FilePath
		if req.FilePath != nil {
			postFilePath = *req.FilePath
		}
		postEnabled := old.Enabled
		if req.Enabled != nil {
			postEnabled = *req.Enabled
		}
		if filePathBlocked {
			if err := userSecretFilePathPolicyError(old, req); err != nil {
				return err
			}
		}

		if postEnabled && postEnvName == "" && postFilePath == "" {
			return errUserSecretInjectionTargetRequired
		}

		updated, err := tx.UpdateUserSecretByUserIDAndName(ctx, params)
		if err != nil {
			return xerrors.Errorf("update user secret: %w", err)
		}
		secret = updated
		aReq.New = updated
		return nil
	}, nil)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.ResourceNotFound(rw)
			return
		}
		var field, detail string
		switch {
		case errors.Is(err, errUserSecretInjectionTargetRequired):
			field, detail = codersdk.UserSecretEnvNameField, codersdk.UserSecretInjectionTargetRequiredDetail
		case errors.Is(err, errUserSecretFilePathDisabled):
			field, detail = codersdk.UserSecretFilePathField, userSecretFilePathDisabledDetail
		case errors.Is(err, errUserSecretEnvTargetRequired):
			field, detail = codersdk.UserSecretEnvNameField, userSecretEnvTargetRequiredDetail
		}
		if field != "" {
			writeUserSecretValidationErrors(ctx, rw, http.StatusBadRequest, []codersdk.ValidationError{{
				Field:  field,
				Detail: detail,
			}})
			return
		}
		if validations := userSecretInjectionTargetValidationErrors(err); len(validations) > 0 {
			writeUserSecretValidationErrors(ctx, rw, http.StatusBadRequest, validations)
			return
		}
		if validations := userSecretConflictValidationErrors(err); len(validations) > 0 {
			writeUserSecretValidationErrors(ctx, rw, http.StatusConflict, validations)
			return
		}
		if resp, ok := userSecretLimitResponse(err); ok {
			httpapi.Write(ctx, rw, http.StatusBadRequest, resp)
			return
		}
		if httpapi.IsUnauthorizedError(err) {
			httpapi.Forbidden(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error updating secret.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.UserSecretFromFull(secret))
}

// @Summary Delete a user secret
// @ID delete-a-user-secret
// @Security CoderSessionToken
// @Tags Secrets
// @Param user path string true "User ID, username, or me"
// @Param name path string true "Secret name"
// @Success 204
// @Router /api/v2/users/{user}/secrets/{name} [delete]
func (api *API) deleteUserSecret(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		user              = httpmw.UserParam(r)
		name              = chi.URLParam(r, codersdk.UserSecretNameField)
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.UserSecret](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionDelete,
		})
	)
	defer commitAudit()

	deleted, err := api.Database.DeleteUserSecretByUserIDAndName(ctx, database.DeleteUserSecretByUserIDAndNameParams{
		UserID: user.ID,
		Name:   name,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error deleting secret.",
			Detail:  err.Error(),
		})
		return
	}
	aReq.Old = deleted

	rw.WriteHeader(http.StatusNoContent)
}

func writeUserSecretValidationErrors(ctx context.Context, rw http.ResponseWriter, status int, validations []codersdk.ValidationError) {
	httpapi.Write(ctx, rw, status, codersdk.Response{
		Message:     "Validation failed.",
		Validations: validations,
	})
}

func updateUserSecretValidationErrors(req codersdk.UpdateUserSecretRequest) []codersdk.ValidationError {
	var validations []codersdk.ValidationError
	if req.Value != nil {
		validations = appendUserSecretValidationError(validations, codersdk.UserSecretValueField, codersdk.UserSecretValueValid(*req.Value))
	}
	if req.EnvName != nil {
		validations = appendUserSecretValidationError(validations, codersdk.UserSecretEnvNameField, codersdk.UserSecretEnvNameValid(*req.EnvName))
	}
	if req.FilePath != nil {
		validations = appendUserSecretValidationError(validations, codersdk.UserSecretFilePathField, codersdk.UserSecretFilePathValid(*req.FilePath))
	}
	return validations
}

func appendUserSecretValidationError(validations []codersdk.ValidationError, field string, err error) []codersdk.ValidationError {
	if err == nil {
		return validations
	}
	return append(validations, codersdk.ValidationError{
		Field:  field,
		Detail: err.Error(),
	})
}

// userSecretLimitResponse maps a per-user-limits trigger violation
// (raised by enforce_user_secrets_per_user_limits) to a 400. Returns
// ok=false if err is not such a violation. See
// codersdk.MaxUserSecretsPerUserCount for the rationale behind the caps.
func userSecretLimitResponse(err error) (codersdk.Response, bool) {
	switch {
	case database.IsCheckViolation(err, userSecretsCountLimitConstraint):
		return codersdk.Response{
			Message: "User secrets limit reached.",
			Detail: fmt.Sprintf(
				"Each user can have at most %d secrets.",
				codersdk.MaxUserSecretsPerUserCount,
			),
		}, true
	case database.IsCheckViolation(err, userSecretsTotalBytesLimitConstraint):
		return codersdk.Response{
			Message: "User secrets value-bytes limit reached.",
			Detail: fmt.Sprintf(
				"Stored bytes of your secret values exceed the per-user "+
					"budget (%d bytes after encryption, if applicable). "+
					"Reduce the size or number of your secrets.",
				codersdk.MaxUserSecretsTotalValueBytes,
			),
		}, true
	case database.IsCheckViolation(err, userSecretsEnvBytesLimitConstraint):
		return codersdk.Response{
			Message: "Environment-injected user secrets bytes limit reached.",
			Detail: fmt.Sprintf(
				"Stored bytes of env-injected secret values exceed the "+
					"per-user budget (%d bytes after encryption, if applicable). "+
					"Reduce the size or number of env-injected secrets. If your deployment permits file path delivery, clear env_name and use file_path instead.",
				codersdk.MaxUserSecretValueBytes,
			),
		}, true
	}
	return codersdk.Response{}, false
}

// userSecretInjectionTargetValidationErrors maps the
// user_secrets_enabled_requires_target CHECK violation to a field-level
// validation error. The database constraint is the race-safe source of
// truth for the injection-target invariant: concurrent PATCHes can each
// clear a different target and pass the handler's own post-state check,
// so the constraint is what ultimately rejects an enabled target-less row.
func userSecretInjectionTargetValidationErrors(err error) []codersdk.ValidationError {
	if database.IsCheckViolation(err, database.CheckUserSecretsEnabledRequiresTarget) {
		return []codersdk.ValidationError{{
			Field:  codersdk.UserSecretEnvNameField,
			Detail: codersdk.UserSecretInjectionTargetRequiredDetail,
		}}
	}
	return nil
}

func userSecretConflictValidationErrors(err error) []codersdk.ValidationError {
	switch {
	case database.IsUniqueViolation(err, database.UniqueUserSecretsUserNameIndex):
		return []codersdk.ValidationError{{
			Field:  codersdk.UserSecretNameField,
			Detail: "name already in use",
		}}
	case database.IsUniqueViolation(err, database.UniqueUserSecretsUserEnvNameIndex):
		return []codersdk.ValidationError{{
			Field:  codersdk.UserSecretEnvNameField,
			Detail: "environment variable already in use",
		}}
	case database.IsUniqueViolation(err, database.UniqueUserSecretsUserFilePathIndex):
		return []codersdk.ValidationError{{
			Field:  codersdk.UserSecretFilePathField,
			Detail: "file path already in use",
		}}
	default:
		return nil
	}
}

package coderd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"slices"

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

// @Summary Create a new user secret
// @ID create-a-new-user-secret
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Secrets
// @Param user path string true "User ID, username, or me"
// @Param request body codersdk.CreateUserSecretRequest true "Create secret request"
// @Success 201 {object} codersdk.UserSecret
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

	if validations := codersdk.ValidateCreateUserSecretRequest(req); len(validations) > 0 {
		writeUserSecretValidationErrors(ctx, rw, http.StatusBadRequest, validations)
		return
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
	})
	if err != nil {
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
// @Failure 413 {object} codersdk.Response
// @Router /api/v2/users/{user}/secrets/batch [post]
func (api *API) postUserSecretsBatch(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := httpmw.UserParam(r)

	// Cap body size before reading; worst-case JSON escaping can inflate
	// a max-size file several-fold, so 8x gives comfortable headroom.
	r.Body = http.MaxBytesReader(rw, r.Body, 8*codersdk.MaxSecretsFileBytes)
	var req codersdk.ImportUserSecretsRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	entries, err := codersdk.ParseSecretsFileEntries(req.Format, req.Content)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to parse secrets file.",
			Detail:  err.Error(),
		})
		return
	}

	// Validate every entry and accumulate all errors so the caller can
	// fix the whole file in one round-trip. Each field is prefixed with
	// the entry index, e.g. "secrets[2].env_name", and each detail names
	// the source key (and line, where the format has one) so the caller
	// does not have to count entries to locate the problem.
	var validations []codersdk.ValidationError
	for i, entry := range entries {
		for _, v := range codersdk.ValidateCreateUserSecretRequest(entry.Request) {
			validations = append(validations, codersdk.ValidationError{
				Field:  fmt.Sprintf("secrets[%d].%s", i, v.Field),
				Detail: fmt.Sprintf("Secret %s: %s", userSecretEntryLabel(entry), v.Detail),
			})
		}
	}
	if len(validations) > 0 {
		writeUserSecretValidationErrors(ctx, rw, http.StatusBadRequest, validations)
		return
	}

	// Enumerate every entry that collides with an already-stored secret
	// before opening the transaction, so re-importing a file that collides
	// on ten keys reports all ten at once instead of one per round-trip.
	// The unique indexes stay the authority: two concurrent imports can
	// both pass this check, so the insert loop below still maps a
	// uniqueness violation to the same 409.
	//
	// ListUserSecrets returns metadata only, with no value column, so these
	// rows can serve a secret-count pre-flight but not the per-user byte
	// budgets, which the trigger computes from sum(octet_length(value)).
	// Conflicts also short-circuit here, so a file that both collides and
	// exceeds a cap reports only the conflict.
	existing, err := api.Database.ListUserSecrets(ctx, user.ID)
	if err != nil {
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
	if conflicts := userSecretImportConflicts(entries, existing); len(conflicts) > 0 {
		writeUserSecretImportConflictErrors(ctx, rw, conflicts)
		return
	}

	// Reject a file that cannot fit before opening the transaction, so the
	// caller learns their headroom instead of getting a trigger error pinned to
	// whichever entry happened to reach the cap. Running after the conflict
	// check means every entry here is new, so len(existing)+len(entries) is the
	// resulting count. It also keeps a colliding file reporting its duplicates:
	// re-importing keys the user already has needs no keys removed.
	//
	// The trigger stays the authority. Concurrent imports can both pass this
	// check, and it deliberately covers only the count, not the two byte
	// budgets described above.
	if overflow := len(existing) + len(entries) - codersdk.MaxUserSecretsPerUserCount; overflow > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "User secrets limit reached.",
			Detail: fmt.Sprintf(
				"You have %d of %d secrets and this file contains %d, so remove at least %d.",
				len(existing), codersdk.MaxUserSecretsPerUserCount, len(entries), overflow,
			),
		})
		return
	}

	// Insert atomically. The per-user-limit trigger fires per row, and
	// any unique or limit violation aborts the whole transaction, so a
	// failed import creates nothing. failedIndex records which entry
	// failed so the error can be attributed to it after the rollback.
	var created []database.UserSecret
	failedIndex := -1
	err = api.Database.InTx(func(tx database.Store) error {
		for i, entry := range entries {
			sreq := entry.Request
			s, txErr := tx.CreateUserSecret(ctx, database.CreateUserSecretParams{
				ID:          uuid.New(),
				UserID:      user.ID,
				Name:        sreq.Name,
				Description: sreq.Description,
				Value:       sreq.Value,
				ValueKeyID:  sql.NullString{},
				EnvName:     sreq.EnvName,
				FilePath:    sreq.FilePath,
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
			// A conflict reaching the unique indexes lost a race with a
			// concurrent write, so attribute it to the entry that hit it.
			if index >= 0 {
				for i := range conflicts {
					conflicts[i].Detail = fmt.Sprintf("Secret %s: %s", userSecretEntryLabel(entries[index]), conflicts[i].Detail)
					conflicts[i].Field = fmt.Sprintf("secrets[%d].%s", index, conflicts[i].Field)
				}
			}
			writeUserSecretImportConflictErrors(ctx, rw, conflicts)
			return
		}
		if resp, ok := userSecretLimitResponse(err); ok {
			if index >= 0 {
				resp.Detail = fmt.Sprintf("Entry secrets[%d] (%s): %s", index, userSecretEntryLabel(entries[index]), resp.Detail)
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

	if req.Value == nil && req.Description == nil && req.EnvName == nil && req.FilePath == nil {
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

	// Pre-read the secret inside a transaction so the audit diff has both an
	// "old" and "new" snapshot.
	//
	// Under read committed isolation, a concurrent writer between our SELECT
	// and our UPDATE can cause the audit diff to attribute changes to us that
	// we did not make. We accept this race to match other audit log diffs
	// (templates, workspaces, chats, etc). In practice this should be unlikely
	// to hit since a user can only modify their own secrets.
	var secret database.UserSecret
	err := api.Database.InTx(func(tx database.Store) error {
		old, err := tx.GetUserSecretByUserIDAndName(ctx, database.GetUserSecretByUserIDAndNameParams{
			UserID: user.ID,
			Name:   name,
		})
		if err != nil {
			return xerrors.Errorf("fetch user secret: %w", err)
		}
		aReq.Old = old

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
		if validations := userSecretConflictValidationErrors(err); len(validations) > 0 {
			writeUserSecretValidationErrors(ctx, rw, http.StatusConflict, validations)
			return
		}
		if resp, ok := userSecretLimitResponse(err); ok {
			httpapi.Write(ctx, rw, http.StatusBadRequest, resp)
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

// maxUserSecretLabelNameRunes bounds how much of an offending key is echoed in
// a per-entry error detail. Names may exceed MaxUserSecretNameLength (that is
// itself a validation failure), and an oversized key produces both a name and a
// value error, so echoing keys in full lets a 1 MiB import return a
// multi-megabyte response.
const maxUserSecretLabelNameRunes = 64

// userSecretEntryLabel names an imported entry by its parsed key and, when the
// file format tracks one, its line. Formats without line information (JSON)
// omit the line rather than reporting a placeholder. Only the parsed key is
// echoed, never Request.Value. Malformed input can cause the parser to read
// value material as a key, so the key is truncated.
func userSecretEntryLabel(entry codersdk.ParsedSecret) string {
	name := entry.Request.Name
	if r := []rune(name); len(r) > maxUserSecretLabelNameRunes {
		name = string(r[:maxUserSecretLabelNameRunes]) + "..."
	}
	if entry.Line > 0 {
		return fmt.Sprintf("%q on line %d", name, entry.Line)
	}
	return fmt.Sprintf("%q", name)
}

func writeUserSecretValidationErrors(ctx context.Context, rw http.ResponseWriter, status int, validations []codersdk.ValidationError) {
	httpapi.Write(ctx, rw, status, codersdk.Response{
		Message:     "Validation failed.",
		Validations: validations,
	})
}

// writeUserSecretImportConflictErrors reports an import that collides with
// secrets the user already has. Import uses a conflict-specific title because
// the response lists entries from a batch; single-create and update keep the
// "Validation failed." title their clients already match on.
func writeUserSecretImportConflictErrors(ctx context.Context, rw http.ResponseWriter, conflicts []codersdk.ValidationError) {
	httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
		Message:     "Some secrets already exist.",
		Validations: conflicts,
	})
}

// userSecretImportConflicts reports the entries that collide with the user's
// existing secrets, across all three uniqueness dimensions.
//
// Dimensions that resolve to the same stored secret are reported once. A
// wholesale re-import collides on both the name and the env_name of one stored
// row, and naming that row twice adds nothing. Dimensions that resolve to
// different stored rows are independent problems, so all of them are reported:
// otherwise clearing one collision only reveals the next on a later request,
// which is the round-trip cost this enumeration exists to remove.
//
// The env_name and file_path unique indexes are partial and exclude the empty
// string, so empty values never collide. See migration 000357.
func userSecretImportConflicts(entries []codersdk.ParsedSecret, existing []database.ListUserSecretsRow) []codersdk.ValidationError {
	names := make(map[string]uuid.UUID, len(existing))
	envNames := make(map[string]uuid.UUID, len(existing))
	filePaths := make(map[string]uuid.UUID, len(existing))
	for _, secret := range existing {
		names[secret.Name] = secret.ID
		if secret.EnvName != "" {
			envNames[secret.EnvName] = secret.ID
		}
		if secret.FilePath != "" {
			filePaths[secret.FilePath] = secret.ID
		}
	}

	var conflicts []codersdk.ValidationError
	for i, entry := range entries {
		req := entry.Request
		dimensions := []struct {
			field string
			value string
			owner map[string]uuid.UUID
		}{
			{codersdk.UserSecretNameField, req.Name, names},
			{codersdk.UserSecretEnvNameField, req.EnvName, envNames},
			{codersdk.UserSecretFilePathField, req.FilePath, filePaths},
		}

		var reported []uuid.UUID
		for _, dimension := range dimensions {
			if dimension.value == "" {
				continue
			}
			id, ok := dimension.owner[dimension.value]
			if !ok || slices.Contains(reported, id) {
				continue
			}
			reported = append(reported, id)
			conflicts = append(conflicts, codersdk.ValidationError{
				Field:  fmt.Sprintf("secrets[%d].%s", i, dimension.field),
				Detail: fmt.Sprintf("Secret %s: %s", userSecretEntryLabel(entry), userSecretConflictDetail(dimension.field)),
			})
		}
	}
	return conflicts
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
					"Clear env_name on large secrets or use file_path instead.",
				codersdk.MaxUserSecretValueBytes,
			),
		}, true
	}
	return codersdk.Response{}, false
}

// userSecretConflictDetail is the single wording for a conflict on each
// uniqueness dimension, keyed by the field it is reported under.
func userSecretConflictDetail(field string) string {
	switch field {
	case codersdk.UserSecretNameField:
		return "Name is already in use."
	case codersdk.UserSecretEnvNameField:
		return "Environment variable name is already in use."
	case codersdk.UserSecretFilePathField:
		return "File path is already in use."
	default:
		return "Already in use."
	}
}

func userSecretConflictValidationErrors(err error) []codersdk.ValidationError {
	var field string
	switch {
	case database.IsUniqueViolation(err, database.UniqueUserSecretsUserNameIndex):
		field = codersdk.UserSecretNameField
	case database.IsUniqueViolation(err, database.UniqueUserSecretsUserEnvNameIndex):
		field = codersdk.UserSecretEnvNameField
	case database.IsUniqueViolation(err, database.UniqueUserSecretsUserFilePathIndex):
		field = codersdk.UserSecretFilePathField
	default:
		return nil
	}
	return []codersdk.ValidationError{{
		Field:  field,
		Detail: userSecretConflictDetail(field),
	}}
}

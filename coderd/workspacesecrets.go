package coderd

import (
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpapi/httperror"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/workspacesecrets"
	"github.com/coder/coder/v2/codersdk"
)

// validateWorkspaceSecretInputs validates the secrets supplied on a
// workspace or workspace build request. It applies the same per-field rules
// and file path delivery policy as user secrets. It also rejects entries in
// one request that share a name, env_name, or file_path, which would
// otherwise fail on the build's unique indexes inside the build transaction.
func (api *API) validateWorkspaceSecretInputs(secrets []codersdk.WorkspaceSecretInput) error {
	if len(secrets) == 0 {
		return nil
	}

	blocked := api.userSecretFilePathBlocked()
	seenNames := make(map[string]struct{}, len(secrets))
	seenEnvNames := make(map[string]struct{}, len(secrets))
	seenFilePaths := make(map[string]struct{}, len(secrets))
	var validations []codersdk.ValidationError
	for i, secret := range secrets {
		fieldErrs := codersdk.ValidateWorkspaceSecretInput(secret)
		if blocked && !secret.IsRemoval() && secret.FilePath != "" {
			fieldErrs = append(fieldErrs, codersdk.ValidationError{
				Field:  codersdk.UserSecretFilePathField,
				Detail: userSecretFilePathDisabledDetail,
			})
		}
		if _, dup := seenNames[secret.Name]; dup {
			fieldErrs = append(fieldErrs, codersdk.ValidationError{
				Field:  codersdk.UserSecretNameField,
				Detail: fmt.Sprintf("Secret %q is listed more than once.", secret.Name),
			})
		}
		seenNames[secret.Name] = struct{}{}
		if !secret.IsRemoval() {
			if _, dup := seenEnvNames[secret.EnvName]; dup && secret.EnvName != "" {
				fieldErrs = append(fieldErrs, codersdk.ValidationError{
					Field:  codersdk.UserSecretEnvNameField,
					Detail: fmt.Sprintf("env_name %q is used by more than one secret in this request.", secret.EnvName),
				})
			}
			seenEnvNames[secret.EnvName] = struct{}{}
			if _, dup := seenFilePaths[secret.FilePath]; dup && secret.FilePath != "" {
				fieldErrs = append(fieldErrs, codersdk.ValidationError{
					Field:  codersdk.UserSecretFilePathField,
					Detail: fmt.Sprintf("file_path %q is used by more than one secret in this request.", secret.FilePath),
				})
			}
			seenFilePaths[secret.FilePath] = struct{}{}
		}
		validations = append(validations, prefixUserSecretValidationErrors(i, fieldErrs)...)
	}
	if len(validations) == 0 {
		return nil
	}
	return httperror.NewResponseError(http.StatusBadRequest, codersdk.Response{
		Message:     "Invalid workspace secrets.",
		Validations: validations,
	})
}

// @Summary Get workspace build secrets
// @Description Lists the metadata of the secrets linked to a workspace build,
// @Description including secrets whose values a later build cleared. Values
// @Description are never returned. env_replaces and file_replaces name the
// @Description workspace owner's user secrets that each live secret displaces,
// @Description and are omitted when the caller cannot read those user secrets.
// @ID get-workspace-build-secrets
// @Security CoderSessionToken
// @Produce json
// @Tags Builds
// @Param workspacebuild path string true "Workspace build ID" format(uuid)
// @Success 200 {array} codersdk.WorkspaceSecret
// @Router /api/v2/workspacebuilds/{workspacebuild}/secrets [get]
func (api *API) workspaceBuildSecrets(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx       = r.Context()
		build     = httpmw.WorkspaceBuildParam(r)
		workspace = httpmw.WorkspaceParam(r)
	)

	rows, err := api.Database.GetWorkspaceSecretsHistory(ctx, database.GetWorkspaceSecretsHistoryParams{
		WorkspaceID:      workspace.ID,
		WorkspaceBuildID: build.ID,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error listing workspace build secrets.",
			Detail:  err.Error(),
		})
		return
	}
	secrets := make([]codersdk.WorkspaceSecret, len(rows))
	for i, row := range rows {
		secrets[i] = db2sdk.WorkspaceBuildSecret(row)
	}

	userSecrets, err := api.Database.ListUserSecrets(ctx, workspace.OwnerID)
	switch {
	case dbauthz.IsNotAuthorizedError(err):
		// The caller cannot see the owner's user secrets, so report no
		// replacements rather than their IDs.
	case err != nil:
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error listing the workspace owner's secrets.",
			Detail:  err.Error(),
		})
		return
	default:
		api.applySecretReplacements(secrets, userSecrets)
	}

	httpapi.Write(ctx, rw, http.StatusOK, secrets)
}

// applySecretReplacements records on each live build secret which of the
// workspace owner's user secrets it displaces, using the same rules as the
// agent manifest. Cleared build secrets are not delivered, so they replace
// nothing.
func (api *API) applySecretReplacements(secrets []codersdk.WorkspaceSecret, userSecrets []database.ListUserSecretsRow) {
	policy := workspacesecrets.FilePathAllowed
	if api.userSecretFilePathBlocked() {
		policy = workspacesecrets.FilePathBlocked
	}
	user := make([]workspacesecrets.Secret, 0, len(userSecrets))
	for _, s := range userSecrets {
		user = append(user, workspacesecrets.Secret{
			ID:       s.ID,
			EnvName:  s.EnvName,
			FilePath: s.FilePath,
			Enabled:  s.Enabled,
		})
	}
	build := make([]workspacesecrets.Secret, 0, len(secrets))
	byID := make(map[uuid.UUID]*codersdk.WorkspaceSecret, len(secrets))
	for i, s := range secrets {
		if s.ClearedAt != nil {
			continue
		}
		byID[s.ID] = &secrets[i]
		build = append(build, workspacesecrets.Secret{
			ID:       s.ID,
			EnvName:  s.EnvName,
			FilePath: s.FilePath,
		})
	}
	resolvedUser, _ := workspacesecrets.Resolve(user, build, policy)
	for _, r := range resolvedUser {
		if s, ok := byID[r.EnvReplacedBy.UUID]; r.EnvReplacedBy.Valid && ok {
			s.EnvReplaces = &r.ID
		}
		if s, ok := byID[r.FileReplacedBy.UUID]; r.FileReplacedBy.Valid && ok {
			s.FileReplaces = &r.ID
		}
	}
}

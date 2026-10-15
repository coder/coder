package coderd

import (
	"fmt"
	"net/http"

	"github.com/coder/coder/v2/coderd/httpapi/httperror"
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

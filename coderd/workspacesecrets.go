package coderd

import (
	"fmt"
	"net/http"

	"github.com/coder/coder/v2/coderd/httpapi/httperror"
	"github.com/coder/coder/v2/codersdk"
)

// validateWorkspaceSecretInputs validates the secrets supplied on a
// workspace or workspace build request. It applies the same per-field rules
// and file path delivery policy as user secrets, and rejects duplicate names
// within one request because the builder applies inputs in order and a
// duplicate would silently win.
func (api *API) validateWorkspaceSecretInputs(secrets []codersdk.WorkspaceSecretInput) error {
	if len(secrets) == 0 {
		return nil
	}

	blocked := api.userSecretFilePathBlocked()
	seen := make(map[string]struct{}, len(secrets))
	var validations []codersdk.ValidationError
	for i, secret := range secrets {
		fieldErrs := codersdk.ValidateWorkspaceSecretInput(secret)
		if blocked && !secret.Remove() && secret.FilePath != "" {
			fieldErrs = append(fieldErrs, codersdk.ValidationError{
				Field:  codersdk.UserSecretFilePathField,
				Detail: userSecretFilePathDisabledDetail,
			})
		}
		if _, dup := seen[secret.Name]; dup {
			fieldErrs = append(fieldErrs, codersdk.ValidationError{
				Field:  codersdk.UserSecretNameField,
				Detail: fmt.Sprintf("Secret %q is listed more than once.", secret.Name),
			})
		}
		seen[secret.Name] = struct{}{}
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

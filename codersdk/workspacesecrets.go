package codersdk

// WorkspaceSecretInput sets or removes a workspace secret as part of a
// workspace or workspace build request.
//
// Workspace secrets are delivered to the workspace only through the agent
// manifest (as an environment variable, a file, or both). They are never
// passed to the provisioner, so they do not appear in workspace build
// parameters or Terraform state, and they cannot be read back through the
// API. Every build holds its own copy of the secrets it receives.
// Non-ephemeral secrets carry forward to later builds until a request
// replaces them by name, removes them with a null value, or sets another
// secret on their env_name or file_path. A secret applies as a whole, so
// taking over either of its targets drops it entirely. Ephemeral secrets
// are delivered to that build only.
type WorkspaceSecretInput struct {
	Name string `json:"name"`
	// Value is the plaintext secret. A null value removes the secret. An
	// empty string sets an empty secret.
	Value *string `json:"value"`
	// EnvName is the environment variable to inject the secret as. Empty
	// means no env injection. Required when file_path is empty and value is
	// set.
	EnvName string `json:"env_name,omitempty"`
	// FilePath is the path to write the secret to inside the workspace.
	// Empty means no file is written. Deployments may disable file path
	// delivery.
	FilePath string `json:"file_path,omitempty"`
	// Ephemeral secrets are delivered to this build only and are not
	// carried forward to the next build.
	Ephemeral bool `json:"ephemeral,omitempty"`
}

// IsRemoval reports whether the input removes the secret instead of setting
// it.
func (s WorkspaceSecretInput) IsRemoval() bool {
	return s.Value == nil
}

// ValidateWorkspaceSecretInput validates a single workspace secret input.
// It reuses the user secret rules for name, value, and delivery targets.
func ValidateWorkspaceSecretInput(in WorkspaceSecretInput) []ValidationError {
	var validations []ValidationError
	if err := UserSecretNameValid(in.Name); err != nil {
		validations = append(validations, ValidationError{Field: UserSecretNameField, Detail: err.Error()})
	}
	if in.IsRemoval() {
		// An omitted value also decodes as null, so reject removals that
		// look like a set request with a missing value.
		if in.EnvName != "" || in.FilePath != "" || in.Ephemeral {
			validations = append(validations, ValidationError{
				Field:  UserSecretValueField,
				Detail: WorkspaceSecretRemovalWithFieldsDetail,
			})
		}
		return validations
	}
	if err := UserSecretValueValid(*in.Value); err != nil {
		validations = append(validations, ValidationError{Field: UserSecretValueField, Detail: err.Error()})
	}
	if err := UserSecretEnvNameValid(in.EnvName); err != nil {
		validations = append(validations, ValidationError{Field: UserSecretEnvNameField, Detail: err.Error()})
	}
	if err := UserSecretFilePathValid(in.FilePath); err != nil {
		validations = append(validations, ValidationError{Field: UserSecretFilePathField, Detail: err.Error()})
	}
	if in.EnvName == "" && in.FilePath == "" {
		validations = append(validations, ValidationError{
			Field:  UserSecretEnvNameField,
			Detail: WorkspaceSecretInjectionTargetRequiredDetail,
		})
	}
	return validations
}

// WorkspaceSecretRemovalWithFieldsDetail explains that a removal carries
// only a name.
const WorkspaceSecretRemovalWithFieldsDetail = "A null value removes the secret, so env_name, file_path, and ephemeral must not be set. Set value to set the secret." //nolint:gosec // G101: message text, not a hardcoded credential.

// WorkspaceSecretInjectionTargetRequiredDetail explains that a workspace
// secret has no disabled state, so every set request needs a delivery
// target.
const WorkspaceSecretInjectionTargetRequiredDetail = "A workspace secret must have at least one of env_name or file_path set." //nolint:gosec // G101: message text, not a hardcoded credential.

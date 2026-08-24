package coderd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

func TestUserSecretCreatePolicyValidationErrors(t *testing.T) {
	t.Parallel()

	ptr := func(b bool) *bool { return &b }

	tests := []struct {
		name            string
		disableFilePath bool
		req             codersdk.CreateUserSecretRequest
		wantFields      []string
	}{
		{
			name:            "PolicyOffAllowsFilePath",
			disableFilePath: false,
			req:             codersdk.CreateUserSecretRequest{FilePath: "/tmp/x"},
		},
		{
			name:            "PolicyOnAllowsEnvOnly",
			disableFilePath: true,
			req:             codersdk.CreateUserSecretRequest{EnvName: "X"},
		},
		{
			name:            "PolicyOnRejectsFilePath",
			disableFilePath: true,
			req:             codersdk.CreateUserSecretRequest{EnvName: "X", FilePath: "/tmp/x"},
			wantFields:      []string{codersdk.UserSecretFilePathField},
		},
		{
			name:            "PolicyOnRejectsFilePathOnDisabledSecret",
			disableFilePath: true,
			req:             codersdk.CreateUserSecretRequest{FilePath: "/tmp/x", Enabled: ptr(false)},
			wantFields:      []string{codersdk.UserSecretFilePathField},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			policy := userSecretFilePathAllowed
			if tt.disableFilePath {
				policy = userSecretFilePathBlocked
			}
			got := userSecretCreatePolicyValidationErrors(tt.req, policy)
			fields := make([]string, 0, len(got))
			for _, v := range got {
				fields = append(fields, v.Field)
				assert.NotEmpty(t, v.Detail)
			}
			assert.Equal(t, tt.wantFields, nilIfEmpty(fields))
		})
	}
}

func TestUserSecretCreateValidationErrors(t *testing.T) {
	t.Parallel()

	t.Run("PolicyOnRequiresEnvTarget", func(t *testing.T) {
		t.Parallel()

		validations := userSecretCreateValidationErrors(codersdk.CreateUserSecretRequest{
			Name:  "targetless",
			Value: "value",
		}, userSecretFilePathBlocked)
		require.Len(t, validations, 1)
		assert.Equal(t, codersdk.UserSecretEnvNameField, validations[0].Field)
		assert.Equal(t, userSecretEnvTargetRequiredDetail, validations[0].Detail)
	})

	t.Run("PolicyOnDisabledTargetlessAllowed", func(t *testing.T) {
		t.Parallel()

		disabled := false
		validations := userSecretCreateValidationErrors(codersdk.CreateUserSecretRequest{
			Name:    "targetless-disabled",
			Value:   "value",
			Enabled: &disabled,
		}, userSecretFilePathBlocked)
		require.Empty(t, validations)
	})

	t.Run("PolicyOffKeepsSharedMessage", func(t *testing.T) {
		t.Parallel()

		validations := userSecretCreateValidationErrors(codersdk.CreateUserSecretRequest{
			Name:  "targetless",
			Value: "value",
		}, userSecretFilePathAllowed)
		require.Len(t, validations, 1)
		assert.Equal(t, codersdk.UserSecretInjectionTargetRequiredDetail, validations[0].Detail)
	})
}

// The batch endpoint attributes every entry error to its index, so policy
// errors have to carry the same secrets[i].<field> shape as the shared
// per-entry validator.
func TestPrefixUserSecretValidationErrors(t *testing.T) {
	t.Parallel()

	got := prefixUserSecretValidationErrors(2, []codersdk.ValidationError{
		{Field: codersdk.UserSecretFilePathField, Detail: "nope"},
		{Field: codersdk.UserSecretEnvNameField, Detail: "also nope"},
	})
	require.Len(t, got, 2)
	assert.Equal(t, "secrets[2].file_path", got[0].Field)
	assert.Equal(t, "nope", got[0].Detail)
	assert.Equal(t, "secrets[2].env_name", got[1].Field)
}

func TestUserSecretFilePathPolicyError(t *testing.T) {
	t.Parallel()

	strPtr := func(s string) *string { return &s }
	boolPtr := func(b bool) *bool { return &b }

	legacyFileOnly := database.UserSecret{EnvName: "", FilePath: "/tmp/legacy", Enabled: true}
	bothTargets := database.UserSecret{EnvName: "ENV", FilePath: "/tmp/legacy", Enabled: true}
	disabledFileOnly := database.UserSecret{EnvName: "", FilePath: "/tmp/legacy", Enabled: false}
	envOnly := database.UserSecret{EnvName: "ENV", Enabled: true}

	tests := []struct {
		name    string
		old     database.UserSecret
		req     codersdk.UpdateUserSecretRequest
		wantErr error
	}{
		{
			name: "UnrelatedEditOnLegacyRow",
			old:  legacyFileOnly,
			req:  codersdk.UpdateUserSecretRequest{Description: strPtr("new")},
		},
		{
			name: "ResubmittingSamePath",
			old:  legacyFileOnly,
			req:  codersdk.UpdateUserSecretRequest{FilePath: strPtr("/tmp/legacy")},
		},
		{
			name: "DisablingLegacyRow",
			old:  legacyFileOnly,
			req:  codersdk.UpdateUserSecretRequest{Enabled: boolPtr(false)},
		},
		{
			name: "ClearingPathAndDisabling",
			old:  legacyFileOnly,
			req:  codersdk.UpdateUserSecretRequest{FilePath: strPtr(""), Enabled: boolPtr(false)},
		},
		{
			name: "MigratingToEnv",
			old:  legacyFileOnly,
			req:  codersdk.UpdateUserSecretRequest{EnvName: strPtr("ENV"), FilePath: strPtr("")},
		},
		{
			name:    "AddingPath",
			old:     envOnly,
			req:     codersdk.UpdateUserSecretRequest{FilePath: strPtr("/tmp/new")},
			wantErr: errUserSecretFilePathDisabled,
		},
		{
			name:    "ChangingPath",
			old:     legacyFileOnly,
			req:     codersdk.UpdateUserSecretRequest{FilePath: strPtr("/tmp/other")},
			wantErr: errUserSecretFilePathDisabled,
		},
		{
			name:    "ClearingEnvLeavesOnlyPath",
			old:     bothTargets,
			req:     codersdk.UpdateUserSecretRequest{EnvName: strPtr("")},
			wantErr: errUserSecretEnvTargetRequired,
		},
		{
			name:    "ReEnablingFileOnlyRow",
			old:     disabledFileOnly,
			req:     codersdk.UpdateUserSecretRequest{Enabled: boolPtr(true)},
			wantErr: errUserSecretEnvTargetRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := userSecretFilePathPolicyError(tt.old, tt.req)
			if tt.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func nilIfEmpty(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	return s
}

package coderd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

func boolPtr(b bool) *bool    { return &b }
func strPtr(s string) *string { return &s }

func TestUserSecretCreateValidationErrors(t *testing.T) {
	t.Parallel()

	req := func(envName, filePath string, enabled *bool) codersdk.CreateUserSecretRequest {
		return codersdk.CreateUserSecretRequest{
			Name: "n", Value: "v", EnvName: envName, FilePath: filePath, Enabled: enabled,
		}
	}
	const (
		envField  = codersdk.UserSecretEnvNameField
		pathField = codersdk.UserSecretFilePathField
	)

	tests := []struct {
		name       string
		policy     userSecretFilePathPolicy
		req        codersdk.CreateUserSecretRequest
		wantField  string
		wantDetail string
	}{
		{name: "PolicyOffAllowsFilePath", policy: userSecretFilePathAllowed, req: req("X", "/tmp/x", nil)},
		{name: "PolicyOnAllowsEnvOnly", policy: userSecretFilePathBlocked, req: req("X", "", nil)},
		{name: "PolicyOnAllowsDisabledTargetless", policy: userSecretFilePathBlocked, req: req("", "", boolPtr(false))},
		{name: "PolicyOnRejectsFilePath", policy: userSecretFilePathBlocked, req: req("X", "/tmp/x", nil), wantField: pathField, wantDetail: userSecretFilePathDisabledDetail},
		{name: "PolicyOnRejectsFilePathOnDisabled", policy: userSecretFilePathBlocked, req: req("", "/tmp/x", boolPtr(false)), wantField: pathField, wantDetail: userSecretFilePathDisabledDetail},
		{name: "PolicyOnRequiresEnvTarget", policy: userSecretFilePathBlocked, req: req("", "", nil), wantField: envField, wantDetail: userSecretEnvTargetRequiredDetail},
		{name: "PolicyOffKeepsSharedMessage", policy: userSecretFilePathAllowed, req: req("", "", nil), wantField: envField, wantDetail: codersdk.UserSecretInjectionTargetRequiredDetail},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := userSecretCreateValidationErrors(tt.req, tt.policy)
			if tt.wantField == "" {
				require.Empty(t, got)
				return
			}
			require.Len(t, got, 1)
			assert.Equal(t, tt.wantField, got[0].Field)
			assert.Equal(t, tt.wantDetail, got[0].Detail)
		})
	}
}

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

	var (
		legacyFileOnly   = database.UserSecret{FilePath: "/tmp/legacy", Enabled: true}
		bothTargets      = database.UserSecret{EnvName: "ENV", FilePath: "/tmp/legacy", Enabled: true}
		disabledFileOnly = database.UserSecret{FilePath: "/tmp/legacy"}
		envOnly          = database.UserSecret{EnvName: "ENV", Enabled: true}
	)

	tests := []struct {
		name    string
		old     database.UserSecret
		req     codersdk.UpdateUserSecretRequest
		wantErr error
	}{
		{name: "UnrelatedEdit", old: legacyFileOnly, req: codersdk.UpdateUserSecretRequest{Description: strPtr("new")}},
		{name: "ResubmitSamePath", old: legacyFileOnly, req: codersdk.UpdateUserSecretRequest{FilePath: strPtr("/tmp/legacy")}},
		{name: "Disable", old: legacyFileOnly, req: codersdk.UpdateUserSecretRequest{Enabled: boolPtr(false)}},
		{name: "ClearOnlyPath", old: legacyFileOnly, req: codersdk.UpdateUserSecretRequest{FilePath: strPtr("")}, wantErr: errUserSecretEnvTargetRequired},
		{name: "ClearPathAndDisable", old: legacyFileOnly, req: codersdk.UpdateUserSecretRequest{FilePath: strPtr(""), Enabled: boolPtr(false)}},
		{name: "MigrateToEnv", old: legacyFileOnly, req: codersdk.UpdateUserSecretRequest{EnvName: strPtr("ENV"), FilePath: strPtr("")}},
		{name: "CleanupPathWithEnvTarget", old: bothTargets, req: codersdk.UpdateUserSecretRequest{FilePath: strPtr("")}},
		{name: "AddPath", old: envOnly, req: codersdk.UpdateUserSecretRequest{FilePath: strPtr("/tmp/new")}, wantErr: errUserSecretFilePathDisabled},
		{name: "ChangePath", old: legacyFileOnly, req: codersdk.UpdateUserSecretRequest{FilePath: strPtr("/tmp/other")}, wantErr: errUserSecretFilePathDisabled},
		{name: "ClearEnvLeavesOnlyPath", old: bothTargets, req: codersdk.UpdateUserSecretRequest{EnvName: strPtr("")}, wantErr: errUserSecretEnvTargetRequired},
		{name: "ReEnableFileOnly", old: disabledFileOnly, req: codersdk.UpdateUserSecretRequest{Enabled: boolPtr(true)}, wantErr: errUserSecretEnvTargetRequired},
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

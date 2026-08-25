package coderd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
)

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
		blocked    bool
		req        codersdk.CreateUserSecretRequest
		wantField  string
		wantDetail string
	}{
		{name: "PolicyOffAllowsFilePath", blocked: false, req: req("X", "/tmp/x", nil)},
		{name: "PolicyOnAllowsEnvOnly", blocked: true, req: req("X", "", nil)},
		{name: "PolicyOnAllowsDisabledTargetless", blocked: true, req: req("", "", ptr.Ref(false))},
		{name: "PolicyOnRejectsFilePath", blocked: true, req: req("X", "/tmp/x", nil), wantField: pathField, wantDetail: userSecretFilePathDisabledDetail},
		{name: "PolicyOnRejectsFilePathOnDisabled", blocked: true, req: req("", "/tmp/x", ptr.Ref(false)), wantField: pathField, wantDetail: userSecretFilePathDisabledDetail},
		{name: "PolicyOnRequiresEnvTarget", blocked: true, req: req("", "", nil), wantField: envField, wantDetail: userSecretEnvTargetRequiredDetail},
		{name: "PolicyOffKeepsSharedMessage", blocked: false, req: req("", "", nil), wantField: envField, wantDetail: codersdk.UserSecretInjectionTargetRequiredDetail},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := userSecretCreateValidationErrors(tt.req, tt.blocked)
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

func TestPrefixUserSecretValidationErrorsNegativeIndex(t *testing.T) {
	t.Parallel()

	// The batch handler passes failedIndex=-1 when no entry can be blamed.
	in := []codersdk.ValidationError{{Field: codersdk.UserSecretEnvNameField, Detail: "nope"}}
	got := prefixUserSecretValidationErrors(-1, in)
	require.Len(t, got, 1)
	assert.Equal(t, codersdk.UserSecretEnvNameField, got[0].Field)
	assert.Equal(t, "nope", got[0].Detail)
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
		{name: "UnrelatedEdit", old: legacyFileOnly, req: codersdk.UpdateUserSecretRequest{Description: ptr.Ref("new")}},
		{name: "ResubmitSamePath", old: legacyFileOnly, req: codersdk.UpdateUserSecretRequest{FilePath: ptr.Ref("/tmp/legacy")}},
		{name: "Disable", old: legacyFileOnly, req: codersdk.UpdateUserSecretRequest{Enabled: ptr.Ref(false)}},
		{name: "ClearOnlyPath", old: legacyFileOnly, req: codersdk.UpdateUserSecretRequest{FilePath: ptr.Ref("")}, wantErr: errUserSecretEnvTargetRequired},
		{name: "ClearPathAndDisable", old: legacyFileOnly, req: codersdk.UpdateUserSecretRequest{FilePath: ptr.Ref(""), Enabled: ptr.Ref(false)}},
		{name: "MigrateToEnv", old: legacyFileOnly, req: codersdk.UpdateUserSecretRequest{EnvName: ptr.Ref("ENV"), FilePath: ptr.Ref("")}},
		{name: "CleanupPathWithEnvTarget", old: bothTargets, req: codersdk.UpdateUserSecretRequest{FilePath: ptr.Ref("")}},
		{name: "AddPath", old: envOnly, req: codersdk.UpdateUserSecretRequest{FilePath: ptr.Ref("/tmp/new")}, wantErr: errUserSecretFilePathDisabled},
		{name: "ChangePath", old: legacyFileOnly, req: codersdk.UpdateUserSecretRequest{FilePath: ptr.Ref("/tmp/other")}, wantErr: errUserSecretFilePathDisabled},
		{name: "ClearEnvLeavesOnlyPath", old: bothTargets, req: codersdk.UpdateUserSecretRequest{EnvName: ptr.Ref("")}, wantErr: errUserSecretEnvTargetRequired},
		{name: "ReEnableFileOnly", old: disabledFileOnly, req: codersdk.UpdateUserSecretRequest{Enabled: ptr.Ref(true)}, wantErr: errUserSecretEnvTargetRequired},
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

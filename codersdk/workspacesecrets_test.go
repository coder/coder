package codersdk_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
)

func TestValidateWorkspaceSecretInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   codersdk.WorkspaceSecretInput
		want []codersdk.ValidationError
	}{
		{
			name: "Valid",
			in: codersdk.WorkspaceSecretInput{
				Name:     "api-key",
				Value:    ptr.Ref("secret"),
				EnvName:  "API_KEY",
				FilePath: "~/.api-key",
			},
		},
		{
			name: "RemoveNeedsOnlyName",
			in: codersdk.WorkspaceSecretInput{
				Name: "api-key",
			},
		},
		{
			// Only a null value removes; an empty string is a value.
			name: "EmptyValueNeedsTarget",
			in: codersdk.WorkspaceSecretInput{
				Name:  "api-key",
				Value: ptr.Ref(""),
			},
			want: []codersdk.ValidationError{{
				Field:  "env_name",
				Detail: codersdk.WorkspaceSecretInjectionTargetRequiredDetail,
			}},
		},
		{
			name: "RemoveInvalidName",
			in: codersdk.WorkspaceSecretInput{
				Name: "bad/name",
			},
			want: []codersdk.ValidationError{{
				Field:  "name",
				Detail: "Name must not contain /, ?, or #.",
			}},
		},
		{
			name: "MissingInjectionTarget",
			in: codersdk.WorkspaceSecretInput{
				Name:  "api-key",
				Value: ptr.Ref("secret"),
			},
			want: []codersdk.ValidationError{{
				Field:  "env_name",
				Detail: codersdk.WorkspaceSecretInjectionTargetRequiredDetail,
			}},
		},
		{
			name: "MultiInvalid",
			in: codersdk.WorkspaceSecretInput{
				Value:    ptr.Ref("secret"),
				EnvName:  "1TOKEN",
				FilePath: "relative/path",
			},
			want: []codersdk.ValidationError{
				{Field: "name", Detail: "Name is required."},
				{Field: "env_name", Detail: "must start with a letter or underscore, followed by letters, digits, or underscores"},
				{Field: "file_path", Detail: "file path must start with ~/ or /"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := codersdk.ValidateWorkspaceSecretInput(tt.in)
			assert.Equal(t, tt.want, got)
		})
	}
}

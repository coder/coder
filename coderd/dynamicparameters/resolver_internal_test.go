package dynamicparameters

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestFormatSecretRequirementDetail(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		req  codersdk.SecretRequirementStatus
		want string
	}{
		{
			name: "Env",
			req: codersdk.SecretRequirementStatus{
				Env:         "GITHUB_TOKEN",
				HelpMessage: "Add a GitHub PAT",
			},
			want: "env GITHUB_TOKEN: Add a GitHub PAT",
		},
		{
			name: "EnvNoHelpMessage",
			req: codersdk.SecretRequirementStatus{
				Env: "GITHUB_TOKEN",
			},
			want: "env GITHUB_TOKEN",
		},
		{
			name: "File",
			req: codersdk.SecretRequirementStatus{
				File:        "~/.ssh/id_rsa",
				HelpMessage: "Add an SSH key",
			},
			want: "file ~/.ssh/id_rsa: Add an SSH key",
		},
		{
			name: "FileNoHelpMessage",
			req: codersdk.SecretRequirementStatus{
				File: "~/.ssh/id_rsa",
			},
			want: "file ~/.ssh/id_rsa",
		},
		{
			name: "MalformedEmpty",
			req:  codersdk.SecretRequirementStatus{},
			want: "malformed secret requirement",
		},
		{
			name: "MalformedBothEnvAndFile",
			req: codersdk.SecretRequirementStatus{
				Env:  "GITHUB_TOKEN",
				File: "~/.ssh/id_rsa",
			},
			want: "malformed secret requirement",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, formatSecretRequirementDetail(tt.req))
		})
	}
}

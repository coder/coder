package dynamicparameters

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestFormatMissingSecrets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		reqs []codersdk.SecretRequirementStatus
		want string
	}{
		{
			name: "Env",
			reqs: []codersdk.SecretRequirementStatus{{
				Env:         "GITHUB_TOKEN",
				HelpMessage: "Add a GitHub PAT",
			}},
			want: "env GITHUB_TOKEN: Add a GitHub PAT",
		},
		{
			name: "File",
			reqs: []codersdk.SecretRequirementStatus{{
				File: "~/.ssh/id_rsa",
			}},
			want: "file ~/.ssh/id_rsa",
		},
		{
			name: "Multiple",
			reqs: []codersdk.SecretRequirementStatus{
				{
					Env: "GITHUB_TOKEN",
				},
				{
					File:        "~/.ssh/id_rsa",
					HelpMessage: "Add an SSH key",
				},
			},
			want: "env GITHUB_TOKEN\nfile ~/.ssh/id_rsa: Add an SSH key",
		},
		{
			name: "MalformedEmpty",
			reqs: []codersdk.SecretRequirementStatus{{}},
			want: "malformed secret requirement",
		},
		{
			name: "MalformedBothEnvAndFile",
			reqs: []codersdk.SecretRequirementStatus{{
				Env:  "GITHUB_TOKEN",
				File: "~/.ssh/id_rsa",
			}},
			want: "malformed secret requirement",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, formatMissingSecrets(tt.reqs))
		})
	}
}

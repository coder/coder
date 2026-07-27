package coderd

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

// TestUserSecretImportConflicts exercises userSecretImportConflicts directly.
// The endpoint cannot reach every branch: ParseSecretsFileEntries never sets
// FilePath, so the file_path dimension is unreachable over HTTP, and a single
// entry colliding on all three dimensions of one stored row cannot be built
// from a parsed file either.
func TestUserSecretImportConflicts(t *testing.T) {
	t.Parallel()

	rowOne, rowTwo, rowThree := uuid.New(), uuid.New(), uuid.New()

	cases := []struct {
		name     string
		entries  []codersdk.ParsedSecret
		existing []database.ListUserSecretsRow
		want     []codersdk.ValidationError
	}{
		{
			// Each dimension is owned by a different stored row, so clearing
			// any one of them leaves the others colliding. All three are
			// reported, in field order.
			name: "ThreeDistinctRows",
			entries: []codersdk.ParsedSecret{{
				Request: codersdk.CreateUserSecretRequest{
					Name:     "SHARED",
					Value:    "v",
					EnvName:  "SHARED_ENV",
					FilePath: "/tmp/shared",
				},
				Line: 1,
			}},
			existing: []database.ListUserSecretsRow{
				{ID: rowOne, Name: "SHARED"},
				{ID: rowTwo, Name: "holder-env", EnvName: "SHARED_ENV"},
				{ID: rowThree, Name: "holder-file", FilePath: "/tmp/shared"},
			},
			want: []codersdk.ValidationError{
				{
					Field:  "secrets[0].name",
					Detail: `Secret "SHARED" on line 1: Name is already in use.`,
				},
				{
					Field:  "secrets[0].env_name",
					Detail: `Secret "SHARED" on line 1: Environment variable name is already in use.`,
				},
				{
					Field:  "secrets[0].file_path",
					Detail: `Secret "SHARED" on line 1: File path is already in use.`,
				},
			},
		},
		{
			// One stored row owns all three dimensions. Naming it once is
			// enough, so the entry yields a single conflict on the first
			// dimension.
			name: "OneRowOwnsAllDimensions",
			entries: []codersdk.ParsedSecret{{
				Request: codersdk.CreateUserSecretRequest{
					Name:     "SHARED",
					Value:    "v",
					EnvName:  "SHARED",
					FilePath: "/tmp/shared",
				},
				Line: 1,
			}},
			existing: []database.ListUserSecretsRow{
				{ID: rowOne, Name: "SHARED", EnvName: "SHARED", FilePath: "/tmp/shared"},
			},
			want: []codersdk.ValidationError{{
				Field:  "secrets[0].name",
				Detail: `Secret "SHARED" on line 1: Name is already in use.`,
			}},
		},
		{
			name: "EnvNameOnly",
			entries: []codersdk.ParsedSecret{{
				Request: codersdk.CreateUserSecretRequest{
					Name:    "FRESH",
					Value:   "v",
					EnvName: "SHARED_ENV",
				},
				Line: 4,
			}},
			existing: []database.ListUserSecretsRow{
				{ID: rowOne, Name: "holder", EnvName: "SHARED_ENV"},
			},
			want: []codersdk.ValidationError{{
				Field:  "secrets[0].env_name",
				Detail: `Secret "FRESH" on line 4: Environment variable name is already in use.`,
			}},
		},
		{
			// The env_name and file_path indexes are partial, so a stored row
			// with empty values does not make an entry's empty values collide.
			name: "EmptyDimensionsNeverCollide",
			entries: []codersdk.ParsedSecret{{
				Request: codersdk.CreateUserSecretRequest{Name: "FRESH", Value: "v"},
				Line:    1,
			}},
			existing: []database.ListUserSecretsRow{
				{ID: rowOne, Name: "holder"},
			},
			want: nil,
		},
		{
			name:     "NoExistingSecrets",
			entries:  []codersdk.ParsedSecret{{Request: codersdk.CreateUserSecretRequest{Name: "FRESH", Value: "v"}, Line: 1}},
			existing: nil,
			want:     nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := userSecretImportConflicts(tc.entries, tc.existing)
			assert.Equal(t, tc.want, got)
			for _, conflict := range got {
				require.NotEmpty(t, conflict.Detail)
			}
		})
	}
}

// TestUserSecretConflictDetail pins the wording every endpoint shares and
// verifies an unrecognized field still produces a usable detail rather than an
// empty string.
func TestUserSecretConflictDetail(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "Name is already in use.", userSecretConflictDetail(codersdk.UserSecretNameField))
	assert.Equal(t, "Environment variable name is already in use.", userSecretConflictDetail(codersdk.UserSecretEnvNameField))
	assert.Equal(t, "File path is already in use.", userSecretConflictDetail(codersdk.UserSecretFilePathField))
	assert.Equal(t, "Already in use.", userSecretConflictDetail("some_new_dimension"))
}

package tfstate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/go-version"
	"github.com/stretchr/testify/require"
)

// testMaxVersion is newer than the Terraform that stamped the state files
// below.
var testMaxVersion = version.Must(version.NewVersion("1.17.0"))

func TestReadEmptyState(t *testing.T) {
	t.Parallel()

	t.Run("NoResources", func(t *testing.T) {
		t.Parallel()
		modules, err := parseStateFile([]byte(`{"version":4,"terraform_version":"1.16.2","resources":[]}`), testMaxVersion)
		require.NoError(t, err)
		require.Empty(t, modules)
	})

	t.Run("MissingFile", func(t *testing.T) {
		t.Parallel()
		modules, err := Read(filepath.Join(t.TempDir(), "terraform.tfstate"), testMaxVersion)
		require.NoError(t, err)
		require.Empty(t, modules)
	})
}

// TestReadRejects covers state files that Read leaves to `terraform show`.
func TestReadRejects(t *testing.T) {
	t.Parallel()

	withResource := func(resource string) string {
		return `{"version":4,"terraform_version":"1.16.2","resources":[` + resource + `]}`
	}
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{"NotJSON", `{"version":4,`, "decode state file"},
		{"OldFormat", `{"version":3,"modules":[]}`, "unsupported state file version 3"},
		{"NewFormat", `{"version":5,"terraform_version":"2.0.0"}`, "unsupported state file version 5"},
		{"NoTerraformVersion", `{"version":4,"resources":[]}`, "parse terraform version"},
		{"UnknownMode", withResource(`{"mode":"ephemeral","type":"a","name":"b","instances":[]}`), "unsupported mode"},
		{"BoolIndexKey", withResource(`{"mode":"managed","type":"a","name":"b","instances":[{"index_key":true,"attributes":{}}]}`), "unsupported index key"},
		{"FlatmapAttributes", withResource(`{"mode":"managed","type":"a","name":"b","instances":[{"attributes_flat":{"id":"1"}}]}`), "legacy flatmap"},
		{"InvalidModule", withResource(`{"module":"module.a.b","mode":"managed","type":"a","name":"b","instances":[{"attributes":{}}]}`), "parse module address"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "terraform.tfstate")
			require.NoError(t, os.WriteFile(path, []byte(test.content), 0o600))
			_, err := Read(path, testMaxVersion)
			require.ErrorContains(t, err, test.wantErr)
		})
	}

	t.Run("Unreadable", func(t *testing.T) {
		t.Parallel()
		_, err := Read(t.TempDir(), testMaxVersion)
		require.ErrorContains(t, err, "read state file")
	})
}

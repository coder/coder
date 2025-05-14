package files_test

import (
	"io/fs"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/files"
)

func TestOverlayFS(t *testing.T) {
	t.Parallel()

	a := afero.NewMemMapFs()
	afero.WriteFile(a, "main.tf", []byte("terraform {}"), 0o644)
	afero.WriteFile(a, ".terraform/modules/example_module/main.tf", []byte("inaccessible"), 0o644)
	afero.WriteFile(a, ".terraform/modules/other_module/main.tf", []byte("inaccessible"), 0o644)
	b := afero.NewMemMapFs()
	afero.WriteFile(b, ".terraform/modules/modules.json", []byte("{}"), 0o644)
	afero.WriteFile(b, ".terraform/modules/example_module/main.tf", []byte("terraform {}"), 0o644)

	it := files.NewOverlayFS(afero.NewIOFS(a), []files.Overlay{{
		Path: ".terraform/modules",
		FS:   afero.NewIOFS(b),
	}})

	content, err := fs.ReadFile(it, "main.tf")
	require.NoError(t, err)
	require.Equal(t, "terraform {}", string(content))

	_, err = fs.ReadFile(it, ".terraform/modules/other_module/main.tf")
	require.Error(t, err)

	content, err = fs.ReadFile(it, ".terraform/modules/modules.json")
	require.NoError(t, err)
	require.Equal(t, "{}", string(content))

	content, err = fs.ReadFile(it, ".terraform/modules/example_module/main.tf")
	require.NoError(t, err)
	require.Equal(t, "terraform {}", string(content))
}

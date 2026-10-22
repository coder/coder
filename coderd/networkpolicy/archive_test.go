package networkpolicy_test

import (
	"archive/tar"
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"testing/fstest"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/networkpolicy"
	"github.com/coder/coder/v2/provisionersdk/tfpath"
	"github.com/coder/coder/v2/testutil"
)

func TestArchive(t *testing.T) {
	t.Parallel()
	module := fstest.MapFS{
		"main.tf":                       &fstest.MapFile{Data: []byte("# trusted module\n"), Mode: 0o777},
		"variables.tf":                  &fstest.MapFile{Data: []byte("# trusted variables\n")},
		".terraform.lock.hcl":           &fstest.MapFile{Data: []byte("# pinned providers\n")},
		"terraform.tfstate":             &fstest.MapFile{Data: []byte("secret state")},
		"credentials":                   &fstest.MapFile{Data: []byte("secret credential")},
		"tests/fixture.tf":              &fstest.MapFile{Data: []byte("# fixture\n")},
		".terraform/modules/main.tf":    &fstest.MapFile{Data: []byte("# downloaded module\n")},
		"nested/module/main.tf":         &fstest.MapFile{Data: []byte("# nested module\n")},
		".terraform/providers/provider": &fstest.MapFile{Data: []byte("provider binary")},
	}
	hclSource := `module "coder_network_policy" {
 source = "./module"
 allowed_https_hosts = ["github.com"]
 direct_p2p_enabled = false
}`
	first, err := networkpolicy.Archive(hclSource, module)
	require.NoError(t, err)
	second, err := networkpolicy.Archive(hclSource, module)
	require.NoError(t, err)
	require.Equal(t, first, second, "revision packages must be deterministic")

	// Use the same extraction path as Terraform Init, without precreating module/.
	layout := tfpath.Session(t.TempDir(), "policy")
	require.NoError(t, layout.ExtractArchive(t.Context(), testutil.Logger(t), afero.NewOsFs(), first, nil))

	files := map[string]string{}
	var entries []string
	reader := tar.NewReader(bytes.NewReader(first))
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		entries = append(entries, header.Name)
		if header.Typeflag == tar.TypeDir {
			require.Equal(t, "module/", header.Name)
			require.EqualValues(t, 0o700, header.Mode)
			require.Zero(t, header.Size)
			continue
		}
		require.Equal(t, byte(tar.TypeReg), header.Typeflag)
		require.EqualValues(t, 0o600, header.Mode)
		data, err := io.ReadAll(reader)
		require.NoError(t, err)
		files[header.Name] = string(data)
	}
	require.Equal(t, []string{"main.tf", "coder.tf", ".terraform.lock.hcl", "module/", "module/main.tf", "module/variables.tf"}, entries)
	require.Len(t, files, 5)
	require.Equal(t, "# trusted module\n", files["module/main.tf"])
	require.Equal(t, "# pinned providers\n", files[".terraform.lock.hcl"])
	require.Contains(t, files["main.tf"], "var.template_ids")
	require.Contains(t, files["main.tf"], "var.policy_id")
	require.Contains(t, files["main.tf"], "var.namespace")
	require.Contains(t, files["coder.tf"], "var.kubeconfig_context")
	for name, source := range files {
		_, diags := hclsyntax.ParseConfig([]byte(source), name, hcl.InitialPos)
		require.False(t, diags.HasErrors(), diags.Error())
		path := filepath.Join(layout.WorkDirectory(), filepath.FromSlash(name))
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, source, string(data))
		info, err := os.Stat(path)
		require.NoError(t, err)
		if runtime.GOOS != "windows" {
			require.Equal(t, fs.FileMode(0o600), info.Mode().Perm())
		}
	}
	var extracted []string
	require.NoError(t, filepath.WalkDir(layout.WorkDirectory(), func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == layout.WorkDirectory() {
			return nil
		}
		relative, err := filepath.Rel(layout.WorkDirectory(), path)
		require.NoError(t, err)
		name := filepath.ToSlash(relative)
		if entry.IsDir() {
			info, err := entry.Info()
			require.NoError(t, err)
			if runtime.GOOS != "windows" {
				require.Equal(t, fs.FileMode(0o700), info.Mode().Perm())
			}
			name += "/"
		}
		extracted = append(extracted, name)
		return nil
	}))
	require.ElementsMatch(t, entries, extracted, "excluded files and directories must not be extracted")
}

func TestArchiveRejectsUnsafeModule(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		module fstest.MapFS
	}{
		{"empty", fstest.MapFS{}},
		{"symlink", fstest.MapFS{"main.tf": &fstest.MapFile{Mode: fs.ModeSymlink, Data: []byte("/etc/passwd")}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := networkpolicy.Archive(`module "coder_network_policy" { source = "./module" }`, tc.module)
			require.Error(t, err)
		})
	}
}

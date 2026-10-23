package coderd

import (
	"archive/tar"
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type templateResourceTestEntry struct {
	name    string
	content string
	kind    byte
}

func templateResourceTestArchive(t *testing.T, entries ...templateResourceTestEntry) []byte {
	t.Helper()
	var data bytes.Buffer
	writer := tar.NewWriter(&data)
	for _, entry := range entries {
		header := &tar.Header{Name: entry.name, Mode: 0o755, Typeflag: entry.kind, Size: int64(len(entry.content))}
		if entry.kind == tar.TypeSymlink || entry.kind == tar.TypeLink {
			header.Linkname = "root.tf"
		}
		require.NoError(t, writer.WriteHeader(header))
		_, err := writer.Write([]byte(entry.content))
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	return data.Bytes()
}

func TestNormalizeTemplateResourceArchive(t *testing.T) {
	t.Parallel()
	root := templateResourceTestEntry{"root.tf", `module "local" { source = "./modules/local" }`, tar.TypeReg}
	for _, tc := range []struct {
		name    string
		entries []templateResourceTestEntry
		wantErr string
	}{
		{"no_main", []templateResourceTestEntry{root, {"./modules/local/main.tf", `output "value" { value = "hello" }`, tar.TypeReg}, {"scripts/run.sh", "#!/bin/sh\necho hello", tar.TypeReg}}, ""},
		{"json_root", []templateResourceTestEntry{{"resources.tf.json", `{"resource":{"terraform_data":{"example":{"input":"hello"}}}}`, tar.TypeReg}}, ""},
		{"dependency_lock", []templateResourceTestEntry{root, {".terraform.lock.hcl", "# dependency selections", tar.TypeReg}, {"modules/local/main.tf", "# module source", tar.TypeReg}}, ""},
		{"local_state", []templateResourceTestEntry{root, {"terraform.tfstate", `{ "version": 4, "resources": [] }`, tar.TypeReg}}, "reserved Terraform runtime path"},
		{"local_state_backup", []templateResourceTestEntry{root, {"./terraform.tfstate.backup", "{}", tar.TypeReg}}, "reserved Terraform runtime path"},
		{"runtime_directory_empty", []templateResourceTestEntry{root, {"./.terraform/", "", tar.TypeDir}}, "reserved Terraform runtime path"},
		{"runtime_directory_implicit", []templateResourceTestEntry{root, {"./.terraform/terraform.tfstate", "{}", tar.TypeReg}}, "reserved Terraform runtime path"},
		{"runtime_workspace_selection", []templateResourceTestEntry{root, {".terraform/environment", "foreign", tar.TypeReg}}, "reserved Terraform runtime path"},
		{"workspace_state_directory_empty", []templateResourceTestEntry{root, {"terraform.tfstate.d/", "", tar.TypeDir}}, "reserved Terraform runtime path"},
		{"workspace_state_directory_implicit", []templateResourceTestEntry{root, {"./terraform.tfstate.d/foreign/terraform.tfstate", "{}", tar.TypeReg}}, "reserved Terraform runtime path"},
		{"runtime_path_file", []templateResourceTestEntry{root, {".terraform", "", tar.TypeReg}}, "reserved Terraform runtime path"},
		{"second_file_backend", []templateResourceTestEntry{root, {"backend.tf", "terraform {\n backend \"s3\" {}\n}", tar.TypeReg}}, "state is managed by Coder"},
		{"json_backend", []templateResourceTestEntry{root, {"backend.tf.json", `{"terraform":{"backend":{"s3":{"bucket":"example"}}}}`, tar.TypeReg}}, "state is managed by Coder"},
		{"json_cloud", []templateResourceTestEntry{{"root.tf.json", `{"terraform":{"cloud":{"organization":"example"}}}`, tar.TypeReg}}, "state is managed by Coder"},
		{"nested_backend", []templateResourceTestEntry{root, {"modules/local/backend.tf", "terraform {\n backend \"local\" {}\n}", tar.TypeReg}}, "state is managed by Coder"},
		{"malformed_hcl", []templateResourceTestEntry{{"root.tf", "resource {", tar.TypeReg}}, "Invalid Terraform configuration"},
		{"malformed_json", []templateResourceTestEntry{{"root.tf.json", "{", tar.TypeReg}}, "Invalid Terraform configuration"},
		{"no_root", []templateResourceTestEntry{{"modules/local/main.tf", "# module", tar.TypeReg}}, "root .tf"},
		{"ignored_root", []templateResourceTestEntry{{".hidden.tf", "# ignored by Terraform", tar.TypeReg}}, "root .tf"},
		{"json_backend_array", []templateResourceTestEntry{{"root.tf.json", `{"terraform":[{"backend":{"s3":{}}}]}`, tar.TypeReg}}, "state is managed by Coder"},
		{"traversal", []templateResourceTestEntry{root, {"modules/../escape", "", tar.TypeReg}}, "unsafe path"},
		{"absolute", []templateResourceTestEntry{root, {"/escape", "", tar.TypeReg}}, "unsafe path"},
		{"windows", []templateResourceTestEntry{root, {`C:\escape`, "", tar.TypeReg}}, "unsafe path"},
		{"duplicate", []templateResourceTestEntry{root, {"./root.tf", "", tar.TypeReg}}, "duplicate path"},
		{"duplicate_directory", []templateResourceTestEntry{root, {"modules/", "", tar.TypeDir}, {"./modules", "", tar.TypeDir}}, "duplicate path"},
		{"parent_is_file", []templateResourceTestEntry{root, {"scripts/run", "", tar.TypeReg}, {"scripts", "", tar.TypeReg}}, "parent directory"},
		{"symlink", []templateResourceTestEntry{root, {"link", "", tar.TypeSymlink}}, "unsupported entry"},
		{"hardlink", []templateResourceTestEntry{root, {"link", "", tar.TypeLink}}, "unsupported entry"},
		{"fifo", []templateResourceTestEntry{root, {"pipe", "", tar.TypeFifo}}, "unsupported entry"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			data := templateResourceTestArchive(t, tc.entries...)
			normalized, preview, err := normalizeTemplateResourceArchive(data, HTTPFileMaxBytes)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Empty(t, preview)
			again, _, err := normalizeTemplateResourceArchive(normalized, HTTPFileMaxBytes)
			require.NoError(t, err)
			require.Equal(t, normalized, again, "normalization is deterministic")
			reader := tar.NewReader(bytes.NewReader(normalized))
			var names []string
			for {
				header, err := reader.Next()
				if errors.Is(err, io.EOF) {
					break
				}
				require.NoError(t, err)
				if header.Typeflag == tar.TypeDir {
					require.EqualValues(t, 0o700, header.Mode)
					continue
				}
				names = append(names, header.Name)
				require.EqualValues(t, 0o711, header.Mode, "retain executable bits, not ownership or write permissions")
			}
			require.Len(t, names, len(tc.entries))
		})
	}
}

func TestTemplateResourceArchiveLimits(t *testing.T) {
	t.Parallel()
	data := templateResourceTestArchive(t, templateResourceTestEntry{"main.tf", "# preview", tar.TypeReg})
	_, preview, err := normalizeTemplateResourceArchive(data, len(data))
	require.NoError(t, err)
	require.Equal(t, "# preview", preview)
	_, _, err = normalizeTemplateResourceArchive(data, len(data)-1)
	require.ErrorContains(t, err, "exceeds maximum size")
	// The frontend TAR writer can omit footer blocks at buffer boundaries.
	// Match Go's reader tolerance, then emit a complete normalized archive.
	withoutFooter, _, err := normalizeTemplateResourceArchive(data[:len(data)-1024], HTTPFileMaxBytes)
	require.NoError(t, err)
	_, _, err = normalizeTemplateResourceArchive(withoutFooter, HTTPFileMaxBytes)
	require.NoError(t, err)
	_, _, err = normalizeTemplateResourceArchive(data[:515], HTTPFileMaxBytes)
	require.Error(t, err, "truncated entry")
	_, _, err = normalizeTemplateResourceArchive([]byte("not a tar"), HTTPFileMaxBytes)
	require.Error(t, err)
	_, _, err = normalizeTemplateResourceArchive(append(data, 'x'), HTTPFileMaxBytes)
	require.ErrorContains(t, err, "unexpected data")
	var oversized bytes.Buffer
	writer := tar.NewWriter(&oversized)
	require.NoError(t, writer.WriteHeader(&tar.Header{Name: "main.tf", Mode: 0o600, Size: templateResourceFileMaxBytes + 1}))
	_, _, err = normalizeTemplateResourceArchive(oversized.Bytes(), HTTPFileMaxBytes)
	require.ErrorContains(t, err, "excessive size", "reject declared size before allocating or reading contents")
	longPath := templateResourceTestArchive(t, templateResourceTestEntry{strings.Repeat("a", 4097), "", tar.TypeReg})
	_, _, err = normalizeTemplateResourceArchive(longPath, HTTPFileMaxBytes)
	require.ErrorContains(t, err, "path exceeds")
	withImplicitDirectories := templateResourceTestArchive(t,
		templateResourceTestEntry{"main.tf", "# root", tar.TypeReg},
		templateResourceTestEntry{"a/b/c/script.sh", "echo hello", tar.TypeReg},
	)
	_, _, err = normalizeTemplateResourceArchive(withImplicitDirectories, len(withImplicitDirectories))
	require.ErrorContains(t, err, "normalized archive exceeds", "added directory metadata is also bounded")
}

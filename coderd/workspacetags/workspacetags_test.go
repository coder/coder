package workspacetags_test

import (
	"archive/tar"
	"bytes"
	"testing"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/workspacetags"
	"github.com/coder/coder/v2/testutil"

	"github.com/stretchr/testify/require"
)

func Test_Validate(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name        string
		files       map[string]string
		expectTags  map[string]string
		expectError string
	}{
		{
			name:        "empty",
			files:       map[string]string{},
			expectTags:  map[string]string{},
			expectError: "",
		},
		{
			name: "single text file",
			files: map[string]string{
				"file.txt": `
		hello world`,
			},
			expectTags:  map[string]string{},
			expectError: "",
		},
		{
			name: "main.tf with no workspace_tags",
			files: map[string]string{
				"main.tf": `
		provider "foo" {}
		resource "foo_bar" "baz" {}`,
			},
			expectTags:  map[string]string{},
			expectError: "",
		},
		{
			name: "main.tf with empty workspace tags",
			files: map[string]string{
				"main.tf": `
		provider "foo" {}
		resource "foo_bar" "baz" {}
		data "coder_workspace_tags" "tags" {}`,
			},
			expectTags:  map[string]string{},
			expectError: "",
		},
		{
			name: "main.tf with valid workspace tags",
			files: map[string]string{
				"main.tf": `
					provider "foo" {}
					resource "foo_bar" "baz" {}
					variable "region" {
						type    = string
						default = "us"
					}
					data "coder_parameter" "az" {
					  name = "az"
						type = "string"
						default = "a"
					}
					data "coder_workspace_tags" "tags" {
						tags = {
							"platform" = "kubernetes",
							"cluster"  = "${"devel"}${"opers"}"
							"region"   = var.region
							"az"       = data.coder_parameter.az.value
						}
					}`,
			},
			expectTags:  map[string]string{"platform": "kubernetes", "cluster": "developers", "region": "us", "az": "a"},
			expectError: "",
		},
		{
			name: "main.tf with missing parameter default value for workspace tags",
			files: map[string]string{
				"main.tf": `
					provider "foo" {}
					resource "foo_bar" "baz" {}
					variable "region" {
						type    = string
						default = "us"
					}
					data "coder_parameter" "az" {
					  name = "az"
						type = "string"
					}
					data "coder_workspace_tags" "tags" {
						tags = {
							"platform" = "kubernetes",
							"cluster"  = "${"devel"}${"opers"}"
							"region"   = var.region
							"az"       = data.coder_parameter.az.value
						}
					}`,
			},
			expectError: `"default" attribute is required by coder_parameter`,
		},
		{
			name: "main.tf with disallowed data source for workspace tags",
			files: map[string]string{
				"main.tf": `
					provider "foo" {}
					resource "foo_bar" "baz" {}
					variable "region" {
						type    = string
						default = "us"
					}
					data "coder_parameter" "az" {
					  name = "az"
						type = "string"
						default = "a"
					}
					data "local_file" "hostname" {
						filename = "/etc/hostname"
					}
					data "coder_workspace_tags" "tags" {
						tags = {
							"platform" = "kubernetes",
							"cluster"  = "${"devel"}${"opers"}"
							"region"   = var.region
							"az"       = data.coder_parameter.az.value
							"hostname" = data.local_file.hostname.content
						}
					}`,
			},
			expectTags:  nil,
			expectError: "only the \"coder_parameter\" data source is supported in workspace tags",
		},
	} {
		t.Run(tc.name+"/tar", func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)
			tar := createTar(t, tc.files)
			logger := slogtest.Make(t, nil)
			tags, err := workspacetags.Validate(ctx, logger, tar, "application/x-tar")
			if tc.expectError != "" {
				require.Contains(t, err.Error(), tc.expectError)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expectTags, tags)
			}
		})
		t.Run(tc.name+"/zip", func(t *testing.T) {
			t.Parallel()
			t.Skip("TODO: convert zip to tar")
			ctx := testutil.Context(t, testutil.WaitShort)
			zip := createZip(t, tc.files)
			logger := slogtest.Make(t, nil)
			tags, err := workspacetags.Validate(ctx, logger, zip, "application/zip")
			if tc.expectError != "" {
				require.Contains(t, err.Error(), tc.expectError)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expectTags, tags)
			}
		})
	}
}

func createTar(t *testing.T, files map[string]string) []byte {
	var buffer bytes.Buffer
	writer := tar.NewWriter(&buffer)
	for path, content := range files {
		err := writer.WriteHeader(&tar.Header{
			Name: path,
			Size: int64(len(content)),
			Uid:  65534, // nobody
			Gid:  65534, // nogroup
			Mode: 0666,  // -rw-rw-rw-
		})
		require.NoError(t, err)

		_, err = writer.Write([]byte(content))
		require.NoError(t, err)
	}

	err := writer.Flush()
	require.NoError(t, err)
	return buffer.Bytes()
}

func createZip(t *testing.T, files map[string]string) []byte {
	ta := createTar(t, files)
	tr := tar.NewReader(bytes.NewReader(ta))
	za, err := coderd.CreateZipFromTar(tr)
	require.NoError(t, err)
	return za
}

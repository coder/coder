package tfparse_test

import (
	"archive/tar"
	"bytes"
	"testing"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/archive"
	"github.com/coder/coder/v2/provisioner/terraform/tfparse"
	"github.com/coder/coder/v2/testutil"

	"github.com/stretchr/testify/require"
)

func Test_WorkspaceTagDefaultsFromFile(t *testing.T) {
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
					resource "foo_bar" "baz" {}
					variable "region" {
						type    = string
						default = "us"
					}
					data "coder_parameter" "az" {
						name = "az"
						type = "string"
						default = "a"
					}`,
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
					variable "region" {
						type    = string
						default = "us"
					}
					data "coder_parameter" "az" {
						name = "az"
						type = "string"
						default = "a"
					}
					data "coder_workspace_tags" "tags" {}`,
			},
			expectTags:  map[string]string{},
			expectError: `"tags" attribute is required by coder_workspace_tags`,
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
			name: "main.tf with multiple valid workspace tags",
			files: map[string]string{
				"main.tf": `
					provider "foo" {}
					resource "foo_bar" "baz" {}
					variable "region" {
						type    = string
						default = "us"
					}
					variable "region2" {
						type    = string
						default = "eu"
					}
					data "coder_parameter" "az" {
					  name = "az"
						type = "string"
						default = "a"
					}
					data "coder_parameter" "az2" {
					  name = "az2"
						type = "string"
						default = "b"
					}
					data "coder_workspace_tags" "tags" {
						tags = {
							"platform" = "kubernetes",
							"cluster"  = "${"devel"}${"opers"}"
							"region"   = var.region
							"az"       = data.coder_parameter.az.value
						}
					}
					data "coder_workspace_tags" "more_tags" {
						tags = {
							"foo" = "bar"
						}
					}`,
			},
			expectTags:  map[string]string{"platform": "kubernetes", "cluster": "developers", "region": "us", "az": "a", "foo": "bar"},
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
			expectError: `provisioner tag "az" evaluated to an empty value, please set a default value`,
		},
		{
			name: "main.tf with missing parameter default value outside workspace tags",
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
					data "coder_parameter" "notaz" {
						name = "notaz"
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
			expectTags:  map[string]string{"platform": "kubernetes", "cluster": "developers", "region": "us", "az": "a"},
			expectError: ``,
		},
		{
			name: "main.tf with missing variable default value outside workspace tags",
			files: map[string]string{
				"main.tf": `
					provider "foo" {}
					resource "foo_bar" "baz" {}
					variable "region" {
						type    = string
						default = "us"
					}
					variable "notregion" {
						type = string
					}
					data "coder_parameter" "az" {
						name = "az"
						type = "string"
						default = "a"
					}
					data "coder_workspace_tags" "tags" {
						tags = {
							"platform"  = "kubernetes",
							"cluster"   = "${"devel"}${"opers"}"
							"region"    = var.region
							"az"        = data.coder_parameter.az.value
						}
					}`,
			},
			expectTags:  map[string]string{"platform": "kubernetes", "cluster": "developers", "region": "us", "az": "a"},
			expectError: ``,
		},
		{
			name: "main.tf with disallowed data source for workspace tags",
			files: map[string]string{
				"main.tf": `
					provider "foo" {}
					resource "foo_bar" "baz" {
						name = "foobar"
					}
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
							"platform"  = "kubernetes",
							"cluster"   = "${"devel"}${"opers"}"
							"region"    = var.region
							"az"        = data.coder_parameter.az.value
							"hostname"  = data.local_file.hostname.content
						}
					}`,
			},
			expectTags:  nil,
			expectError: `invalid workspace tag value "data.local_file.hostname.content": only the "coder_parameter" data source is supported here`,
		},
		{
			name: "main.tf with disallowed resource for workspace tags",
			files: map[string]string{
				"main.tf": `
					provider "foo" {}
					resource "foo_bar" "baz" {
						name = "foobar"
					}
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
							"platform"  = "kubernetes",
							"cluster"   = "${"devel"}${"opers"}"
							"region"    = var.region
							"az"        = data.coder_parameter.az.value
							"foobarbaz" = foo_bar.baz.name
						}
					}`,
			},
			expectTags: nil,
			// TODO: this error isn't great, but it has the desired effect.
			expectError: `There is no variable named "foo_bar"`,
		},
	} {
		tc := tc
		t.Run(tc.name+"/tar", func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)
			tar := createTar(t, tc.files)
			logger := slogtest.Make(t, nil)
			tags, err := tfparse.WorkspaceTagDefaultsFromFile(ctx, logger, tar, "application/x-tar")
			if tc.expectError != "" {
				require.NotNil(t, err)
				require.Contains(t, err.Error(), tc.expectError)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expectTags, tags)
			}
		})
		t.Run(tc.name+"/zip", func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)
			zip := createZip(t, tc.files)
			logger := slogtest.Make(t, nil)
			tags, err := tfparse.WorkspaceTagDefaultsFromFile(ctx, logger, zip, "application/zip")
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
			Mode: 0o666, // -rw-rw-rw-
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
	za, err := archive.CreateZipFromTar(tr, int64(len(ta)))
	require.NoError(t, err)
	return za
}

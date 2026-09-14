package dynamicparameters

import (
	"archive/zip"
	"bytes"
	"testing"

	"github.com/spf13/afero"
	"github.com/spf13/afero/zipfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	archivefs "github.com/coder/coder/v2/archive/fs"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/preview"
)

func Test_DynamicWorkspaceTagDefaultsFromFile(t *testing.T) {
	t.Parallel()

	const (
		unknownTag       = "Tag value is not known"
		invalidValueType = "Tag value is not valid"
	)

	for _, tc := range []struct {
		name               string
		files              map[string]string
		expectTags         map[string]string
		expectedFailedTags map[string]string
		expectedError      string
	}{
		{
			name: "single text file",
			files: map[string]string{
				"file.txt": `
					hello world`,
			},
			expectTags: map[string]string{},
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
					data "coder_parameter" "unrelated" {
						name    = "unrelated"
						type    = "list(string)"
						default = jsonencode(["a", "b"])
					}
					data "coder_parameter" "az" {
						name = "az"
						type = "string"
						default = "a"
					}`,
			},
			expectTags: map[string]string{},
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
					data "coder_parameter" "unrelated" {
						name    = "unrelated"
						type    = "list(string)"
						default = jsonencode(["a", "b"])
					}
					data "coder_parameter" "az" {
						name = "az"
						type = "string"
						default = "a"
					}
					data "coder_workspace_tags" "tags" {}`,
			},
			expectTags: map[string]string{},
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
					variable "unrelated" {
						type = bool
					}
					data "coder_parameter" "unrelated" {
						name    = "unrelated"
						type    = "list(string)"
						default = jsonencode(["a", "b"])
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
			expectTags: map[string]string{"platform": "kubernetes", "cluster": "developers", "region": "us", "az": "a"},
		},
		{
			name: "main.tf with parameter that has default value from dynamic value",
			files: map[string]string{
				"main.tf": `
					provider "foo" {}
					resource "foo_bar" "baz" {}
					variable "region" {
						type    = string
						default = "us"
					}
					variable "az" {
						type    = string
						default = "${""}${"a"}"
					}
					data "coder_parameter" "unrelated" {
						name    = "unrelated"
						type    = "list(string)"
						default = jsonencode(["a", "b"])
					}
					data "coder_parameter" "az" {
						name = "az"
						type = "string"
						default = var.az
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
			expectTags: map[string]string{"platform": "kubernetes", "cluster": "developers", "region": "us", "az": "a"},
		},
		{
			name: "main.tf with parameter that has default value from another parameter",
			files: map[string]string{
				"main.tf": `
					provider "foo" {}
					resource "foo_bar" "baz" {}
					variable "region" {
						type    = string
						default = "us"
					}
					data "coder_parameter" "unrelated" {
						name    = "unrelated"
						type    = "list(string)"
						default = jsonencode(["a", "b"])
					}
					data "coder_parameter" "az" {
						name = "az"
						type    = string
						default = "${""}${"a"}"
					}
					data "coder_parameter" "az2" {
					  	name = "az2"
						type = "string"
						default = data.coder_parameter.az.value
					}
					data "coder_workspace_tags" "tags" {
						tags = {
							"platform" = "kubernetes",
							"cluster"  = "${"devel"}${"opers"}"
							"region"   = var.region
							"az"       = data.coder_parameter.az2.value
						}
					}`,
			},
			expectTags: map[string]string{
				"platform": "kubernetes",
				"cluster":  "developers",
				"region":   "us",
				"az":       "a",
			},
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
					data "coder_parameter" "unrelated" {
						name    = "unrelated"
						type    = "list(string)"
						default = jsonencode(["a", "b"])
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
			expectTags: map[string]string{"platform": "kubernetes", "cluster": "developers", "region": "us", "az": "a", "foo": "bar"},
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
					data "coder_parameter" "unrelated" {
						name    = "unrelated"
						type    = "list(string)"
						default = jsonencode(["a", "b"])
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
			expectTags: map[string]string{"cluster": "developers", "platform": "kubernetes", "region": "us"},
			expectedFailedTags: map[string]string{
				"az": "Tag value is not known, it likely refers to a variable that is not set or has no default.",
			},
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
					data "coder_parameter" "unrelated" {
						name    = "unrelated"
						type    = "list(string)"
						default = jsonencode(["a", "b"])
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
			expectTags: map[string]string{"platform": "kubernetes", "cluster": "developers", "region": "us", "az": "a"},
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
					data "coder_parameter" "unrelated" {
						name    = "unrelated"
						type    = "list(string)"
						default = jsonencode(["a", "b"])
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
			expectTags: map[string]string{"platform": "kubernetes", "cluster": "developers", "region": "us", "az": "a"},
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
					data "coder_parameter" "unrelated" {
						name    = "unrelated"
						type    = "list(string)"
						default = jsonencode(["a", "b"])
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
			expectTags: map[string]string{
				"platform": "kubernetes",
				"cluster":  "developers",
				"region":   "us",
				"az":       "a",
			},
			expectedFailedTags: map[string]string{
				"hostname": unknownTag,
			},
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
					data "coder_parameter" "unrelated" {
						name    = "unrelated"
						type    = "list(string)"
						default = jsonencode(["a", "b"])
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
			expectTags: map[string]string{
				"platform":  "kubernetes",
				"cluster":   "developers",
				"region":    "us",
				"az":        "a",
				"foobarbaz": "foobar",
			},
		},
		{
			name: "main.tf with allowed functions in workspace tags",
			files: map[string]string{
				"main.tf": `
					provider "foo" {}
					resource "foo_bar" "baz" {
						name = "foobar"
					}
					locals {
						some_path = pathexpand("file.txt")
					}
					variable "region" {
						type    = string
						default = "us"
					}
					data "coder_parameter" "unrelated" {
						name    = "unrelated"
						type    = "list(string)"
						default = jsonencode(["a", "b"])
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
							"region"    = try(split(".", var.region)[1], "placeholder")
							"az"        = try(split(".", data.coder_parameter.az.value)[1], "placeholder")
						}
					}`,
			},
			expectTags: map[string]string{"platform": "kubernetes", "cluster": "developers", "region": "placeholder", "az": "placeholder"},
		},
		{
			// Trying to use '~' in a path expand is not allowed, as there is
			// no concept of home directory in preview.
			name: "main.tf with disallowed functions in workspace tags",
			files: map[string]string{
				"main.tf": `
					provider "foo" {}
					resource "foo_bar" "baz" {
						name = "foobar"
					}
					locals {
						some_path = pathexpand("file.txt")
					}
					variable "region" {
						type    = string
						default = "region.us"
					}
					data "coder_parameter" "unrelated" {
						name    = "unrelated"
						type    = "list(string)"
						default = jsonencode(["a", "b"])
					}
					data "coder_parameter" "az" {
						name = "az"
						type = "string"
						default = "az.a"
					}
					data "coder_workspace_tags" "tags" {
						tags = {
							"platform"  = "kubernetes",
							"cluster"   = "${"devel"}${"opers"}"
							"region"    = try(split(".", var.region)[1], "placeholder")
							"az"        = try(split(".", data.coder_parameter.az.value)[1], "placeholder")
							"some_path" = pathexpand("~/file.txt")
						}
					}`,
			},
			expectTags: map[string]string{
				"platform": "kubernetes",
				"cluster":  "developers",
				"region":   "us",
				"az":       "a",
			},
			expectedFailedTags: map[string]string{
				"some_path": unknownTag,
			},
		},
		{
			name: "supported types",
			files: map[string]string{
				"main.tf": `
					variable "stringvar" {
						type    = string
						default = "a"
					}
					variable "numvar" {
						type    = number
						default = 1
					}
					variable "boolvar" {
						type    = bool
						default = true
					}
					variable "listvar" {
						type    = list(string)
						default = ["a"]
					}
					variable "mapvar" {
						type    = map(string)
						default = {"a": "b"}
					}
					data "coder_parameter" "stringparam" {
						name    = "stringparam"
						type    = "string"
						default = "a"
					}
					data "coder_parameter" "numparam" {
						name    = "numparam"
						type    = "number"
						default = 1
					}
					data "coder_parameter" "boolparam" {
						name    = "boolparam"
						type    = "bool"
						default = true
					}
					data "coder_parameter" "listparam" {
						name    = "listparam"
						type    = "list(string)"
						default = "[\"a\", \"b\"]"
					}
					data "coder_workspace_tags" "tags" {
						tags = {
							"stringvar"   = var.stringvar
							"numvar"      = var.numvar
							"boolvar"     = var.boolvar
							"listvar"     = var.listvar
							"mapvar"      = var.mapvar
							"stringparam" = data.coder_parameter.stringparam.value
							"numparam"    = data.coder_parameter.numparam.value
							"boolparam"   = data.coder_parameter.boolparam.value
							"listparam"   = data.coder_parameter.listparam.value
						}
					}`,
			},
			expectTags: map[string]string{
				"stringvar":   "a",
				"numvar":      "1",
				"boolvar":     "true",
				"stringparam": "a",
				"numparam":    "1",
				"boolparam":   "true",
				"listparam":   `["a", "b"]`, // OK because params are cast to strings
			},
			expectedFailedTags: map[string]string{
				"listvar": invalidValueType,
				"mapvar":  invalidValueType,
			},
		},
		{
			name: "overlapping var name",
			files: map[string]string{
				`main.tf`: `
				variable "a" {
					type = string
					default = "1"
				}
				variable "unused" {
					type = map(string)
					default = {"a" : "b"}
				}
				variable "ab" {
					description = "This is a variable of type string"
					type        = string
					default     = "ab"
				}
				data "coder_workspace_tags" "tags" {
					tags = {
						"foo": "bar",
						"a": var.a,
					}
				}`,
			},
			expectTags: map[string]string{"foo": "bar", "a": "1"},
		},
	} {
		t.Run(tc.name+"/tar", func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)
			tarData := testutil.CreateTar(t, tc.files)

			output, diags := preview.Preview(ctx, preview.Input{}, archivefs.FromTarReader(bytes.NewBuffer(tarData)))
			if tc.expectedError != "" {
				require.True(t, diags.HasErrors())
				require.Contains(t, diags.Error(), tc.expectedError)
				return
			}
			require.False(t, diags.HasErrors(), diags.Error())

			tags := output.WorkspaceTags
			tagMap := tags.Tags()
			failedTags := tags.UnusableTags()
			assert.Equal(t, tc.expectTags, tagMap, "expected tags to match, must always provide something")
			for _, tag := range failedTags {
				verr := failedTagDiagnostic(tag)
				expectedErr, ok := tc.expectedFailedTags[tag.KeyString()]
				require.Truef(t, ok, "assertion for failed tag required: %s, %s", tag.KeyString(), verr.Error())
				assert.Contains(t, verr.Error(), expectedErr, tag.KeyString())
			}
		})

		t.Run(tc.name+"/zip", func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)
			zipData := testutil.CreateZip(t, tc.files)

			// get the zip fs
			r, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
			require.NoError(t, err)

			output, diags := preview.Preview(ctx, preview.Input{}, afero.NewIOFS(zipfs.New(r)))
			if tc.expectedError != "" {
				require.True(t, diags.HasErrors())
				require.Contains(t, diags.Error(), tc.expectedError)
				return
			}
			require.False(t, diags.HasErrors(), diags.Error())

			tags := output.WorkspaceTags
			tagMap := tags.Tags()
			failedTags := tags.UnusableTags()
			assert.Equal(t, tc.expectTags, tagMap, "expected tags to match, must always provide something")
			for _, tag := range failedTags {
				verr := failedTagDiagnostic(tag)
				expectedErr, ok := tc.expectedFailedTags[tag.KeyString()]
				assert.Truef(t, ok, "assertion for failed tag required: %s, %s", tag.KeyString(), verr.Error())
				assert.Contains(t, verr.Error(), expectedErr)
			}
		})
	}
}

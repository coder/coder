package templatebuilder

import (
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

func TestParseBasesFromFS(t *testing.T) {
	t.Parallel()

	t.Run("ValidManifest", func(t *testing.T) {
		t.Parallel()

		fsys := fstest.MapFS{
			"bases/docker/base.json": &fstest.MapFile{
				Data: []byte(`{
					"id": "docker",
					"display_name": "Docker",
					"os": "linux",
					"default_context": {
						"container_image": "codercom/enterprise-base:ubuntu"
					}
				}`),
			},
			"bases/docker/main.tf.tmpl": &fstest.MapFile{
				Data: []byte(`image = "{{ .ContainerImage }}"`),
			},
			"bases/docker/README.md": &fstest.MapFile{
				Data: []byte("# Docker\n"),
			},
		}

		bases, err := parseBasesFromFS(fsys)
		require.NoError(t, err)
		require.Len(t, bases, 1)

		b := bases["docker"]
		require.NotNil(t, b)
		require.Equal(t, "docker", b.Manifest.ID)
		require.Equal(t, "Docker", b.Manifest.DisplayName)
		require.Equal(t, "linux", b.Manifest.OS)
		require.Equal(t, "codercom/enterprise-base:ubuntu", b.Manifest.DefaultContext.ContainerImage)
		require.Contains(t, b.Templates, "main.tf.tmpl")
	})

	t.Run("MultipleBases", func(t *testing.T) {
		t.Parallel()

		fsys := fstest.MapFS{
			"bases/alpha/base.json": &fstest.MapFile{
				Data: []byte(`{"id": "alpha", "os": "linux"}`),
			},
			"bases/alpha/main.tf.tmpl": &fstest.MapFile{
				Data: []byte(`resource "alpha" {}`),
			},
			"bases/alpha/README.md": &fstest.MapFile{
				Data: []byte("# Alpha\n"),
			},
			"bases/beta/base.json": &fstest.MapFile{
				Data: []byte(`{"id": "beta", "os": "linux"}`),
			},
			"bases/beta/main.tf.tmpl": &fstest.MapFile{
				Data: []byte(`resource "beta" {}`),
			},
			"bases/beta/README.md": &fstest.MapFile{
				Data: []byte("# Beta\n"),
			},
		}

		bases, err := parseBasesFromFS(fsys)
		require.NoError(t, err)
		require.Len(t, bases, 2)
		require.NotNil(t, bases["alpha"])
		require.NotNil(t, bases["beta"])
	})

	t.Run("EmptyCatalog", func(t *testing.T) {
		t.Parallel()

		fsys := fstest.MapFS{
			"bases/.keep": &fstest.MapFile{Data: []byte{}},
		}

		bases, err := parseBasesFromFS(fsys)
		require.NoError(t, err)
		require.Empty(t, bases)
	})

	t.Run("PreParsesTemplates", func(t *testing.T) {
		t.Parallel()

		fsys := fstest.MapFS{
			"bases/mybase/base.json": &fstest.MapFile{
				Data: []byte(`{"id": "mybase", "os": "linux"}`),
			},
			"bases/mybase/main.tf.tmpl": &fstest.MapFile{
				Data: []byte(`image = "{{ .ContainerImage }}"`),
			},
			// .tftpl files are Terraform templatefile() inputs, not Go templates.
			"bases/mybase/cloud-init/config.yaml.tftpl": &fstest.MapFile{
				Data: []byte(`${some_terraform_var}`),
			},
			"bases/mybase/README.md": &fstest.MapFile{
				Data: []byte("# My Base\n"),
			},
		}

		bases, err := parseBasesFromFS(fsys)
		require.NoError(t, err)

		b := bases["mybase"]
		require.NotNil(t, b)
		require.Contains(t, b.Templates, "main.tf.tmpl")
		// .tftpl files should not be pre-parsed as Go templates.
		require.NotContains(t, b.Templates, "cloud-init/config.yaml.tftpl")
	})

	t.Run("RejectsMissingReadme", func(t *testing.T) {
		t.Parallel()

		fsys := fstest.MapFS{
			"bases/bad/base.json": &fstest.MapFile{
				Data: []byte(`{"id": "bad", "os": "linux"}`),
			},
			"bases/bad/main.tf.tmpl": &fstest.MapFile{
				Data: []byte(`resource {}`),
			},
		}

		_, err := parseBasesFromFS(fsys)
		require.ErrorContains(t, err, "read README.md for base")
	})

	t.Run("RejectsDirWithoutManifest", func(t *testing.T) {
		t.Parallel()

		fsys := fstest.MapFS{
			"bases/nobase/readme.txt": &fstest.MapFile{Data: []byte("hi")},
		}

		_, err := parseBasesFromFS(fsys)
		require.ErrorContains(t, err, "read nobase/base.json")
	})

	t.Run("RejectsEmptyID", func(t *testing.T) {
		t.Parallel()

		fsys := fstest.MapFS{
			"bases/bad/base.json": &fstest.MapFile{
				Data: []byte(`{"id": "", "os": "linux"}`),
			},
		}

		_, err := parseBasesFromFS(fsys)
		require.ErrorContains(t, err, "empty id")
	})

	t.Run("RejectsDuplicateID", func(t *testing.T) {
		t.Parallel()

		fsys := fstest.MapFS{
			"bases/a/base.json": &fstest.MapFile{
				Data: []byte(`{"id": "dupe", "os": "linux"}`),
			},
			"bases/a/README.md": &fstest.MapFile{
				Data: []byte("# A\n"),
			},
			"bases/b/base.json": &fstest.MapFile{
				Data: []byte(`{"id": "dupe", "os": "linux"}`),
			},
			"bases/b/README.md": &fstest.MapFile{
				Data: []byte("# B\n"),
			},
		}

		_, err := parseBasesFromFS(fsys)
		require.ErrorContains(t, err, "duplicate base id")
	})

	t.Run("RejectsUnknownOS", func(t *testing.T) {
		t.Parallel()

		fsys := fstest.MapFS{
			"bases/bad/base.json": &fstest.MapFile{
				Data: []byte(`{"id": "bad", "os": "beos"}`),
			},
		}

		_, err := parseBasesFromFS(fsys)
		require.ErrorContains(t, err, `unknown os "beos"`)
	})

	t.Run("RejectsUnknownField", func(t *testing.T) {
		t.Parallel()

		fsys := fstest.MapFS{
			"bases/bad/base.json": &fstest.MapFile{
				Data: []byte(`{"id": "bad", "os": "linux", "dispaly_name": "typo"}`),
			},
		}

		_, err := parseBasesFromFS(fsys)
		require.ErrorContains(t, err, "decode")
	})

	t.Run("RejectsInvalidJSON", func(t *testing.T) {
		t.Parallel()

		fsys := fstest.MapFS{
			"bases/bad/base.json": &fstest.MapFile{
				Data: []byte(`{not json`),
			},
		}

		_, err := parseBasesFromFS(fsys)
		require.ErrorContains(t, err, "decode")
	})

	t.Run("RejectsInvalidTemplate", func(t *testing.T) {
		t.Parallel()

		fsys := fstest.MapFS{
			"bases/bad/base.json": &fstest.MapFile{
				Data: []byte(`{"id": "bad", "os": "linux"}`),
			},
			"bases/bad/main.tf.tmpl": &fstest.MapFile{
				Data: []byte(`{{ .Broken`),
			},
		}

		_, err := parseBasesFromFS(fsys)
		require.ErrorContains(t, err, "parse templates")
	})

	t.Run("AllowsEmptyOS", func(t *testing.T) {
		t.Parallel()

		fsys := fstest.MapFS{
			"bases/nospec/base.json": &fstest.MapFile{
				Data: []byte(`{"id": "nospec"}`),
			},
			"bases/nospec/README.md": &fstest.MapFile{
				Data: []byte("# No Spec\n"),
			},
		}

		bases, err := parseBasesFromFS(fsys)
		require.NoError(t, err)
		require.Equal(t, "", bases["nospec"].Manifest.OS)
	})

	t.Run("AcceptsWindowsOS", func(t *testing.T) {
		t.Parallel()

		fsys := fstest.MapFS{
			"bases/winbox/base.json": &fstest.MapFile{
				Data: []byte(`{"id": "winbox", "os": "windows"}`),
			},
			"bases/winbox/main.tf.tmpl": &fstest.MapFile{
				Data: []byte(`resource "coder_agent" "main" {}`),
			},
			"bases/winbox/README.md": &fstest.MapFile{
				Data: []byte("# Windows\n"),
			},
		}

		bases, err := parseBasesFromFS(fsys)
		require.NoError(t, err)
		require.Equal(t, "windows", bases["winbox"].Manifest.OS)
		require.Equal(t, BaseOSWindows, validBaseOS[bases["winbox"].Manifest.OS])
	})
}

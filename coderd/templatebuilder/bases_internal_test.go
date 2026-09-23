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

	t.Run("ParsesAgents", func(t *testing.T) {
		t.Parallel()

		fsys := fstest.MapFS{
			"bases/multi/base.json": &fstest.MapFile{
				Data: []byte(`{"id": "multi", "os": "linux", "agents": [{"name": "main"}, {"name": "gpu", "default": true}]}`),
			},
			"bases/multi/main.tf.tmpl": &fstest.MapFile{
				Data: []byte(`resource "coder_agent" "main" {}
resource "coder_agent" "gpu" {}`),
			},
			"bases/multi/README.md": &fstest.MapFile{
				Data: []byte("# Multi\n"),
			},
		}

		bases, err := parseBasesFromFS(fsys)
		require.NoError(t, err)
		agents := bases["multi"].Manifest.Agents
		require.Equal(t, []BaseAgent{{Name: "main"}, {Name: "gpu", Default: true}}, agents)
	})

	rejects := []struct {
		name    string
		fsys    fstest.MapFS
		wantErr string
	}{
		{
			name: "RejectsMissingReadme",
			fsys: fstest.MapFS{
				"bases/bad/base.json": &fstest.MapFile{
					Data: []byte(`{"id": "bad", "os": "linux"}`),
				},
				"bases/bad/main.tf.tmpl": &fstest.MapFile{
					Data: []byte(`resource {}`),
				},
			},
			wantErr: "read README.md for base",
		},
		{
			name: "RejectsDirWithoutManifest",
			fsys: fstest.MapFS{
				"bases/nobase/readme.txt": &fstest.MapFile{Data: []byte("hi")},
			},
			wantErr: "read nobase/base.json",
		},
		{
			name: "RejectsEmptyID",
			fsys: fstest.MapFS{
				"bases/bad/base.json": &fstest.MapFile{
					Data: []byte(`{"id": "", "os": "linux"}`),
				},
			},
			wantErr: "empty id",
		},
		{
			name: "RejectsDuplicateID",
			fsys: fstest.MapFS{
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
			},
			wantErr: "duplicate base id",
		},
		{
			name: "RejectsUnknownOS",
			fsys: fstest.MapFS{
				"bases/bad/base.json": &fstest.MapFile{
					Data: []byte(`{"id": "bad", "os": "beos"}`),
				},
			},
			wantErr: `unknown os "beos"`,
		},
		{
			name: "RejectsUnknownField",
			fsys: fstest.MapFS{
				"bases/bad/base.json": &fstest.MapFile{
					Data: []byte(`{"id": "bad", "os": "linux", "dispaly_name": "typo"}`),
				},
			},
			wantErr: "decode",
		},
		{
			name: "RejectsInvalidJSON",
			fsys: fstest.MapFS{
				"bases/bad/base.json": &fstest.MapFile{
					Data: []byte(`{not json`),
				},
			},
			wantErr: "decode",
		},
		{
			name: "RejectsInvalidTemplate",
			fsys: fstest.MapFS{
				"bases/bad/base.json": &fstest.MapFile{
					Data: []byte(`{"id": "bad", "os": "linux"}`),
				},
				"bases/bad/main.tf.tmpl": &fstest.MapFile{
					Data: []byte(`{{ .Broken`),
				},
			},
			wantErr: "parse templates",
		},
		{
			name: "RejectsEmptyAgentName",
			fsys: fstest.MapFS{
				"bases/bad/base.json": &fstest.MapFile{
					Data: []byte(`{"id": "bad", "os": "linux", "agents": [{"name": ""}]}`),
				},
			},
			wantErr: "empty name",
		},
		{
			name: "RejectsDuplicateAgentName",
			fsys: fstest.MapFS{
				"bases/bad/base.json": &fstest.MapFile{
					Data: []byte(`{"id": "bad", "os": "linux", "agents": [{"name": "main"}, {"name": "main"}]}`),
				},
			},
			wantErr: "duplicate agent name",
		},
		{
			name: "RejectsMultipleDefaultAgents",
			fsys: fstest.MapFS{
				"bases/bad/base.json": &fstest.MapFile{
					Data: []byte(`{"id": "bad", "os": "linux", "agents": [{"name": "a", "default": true}, {"name": "b", "default": true}]}`),
				},
			},
			wantErr: "exactly one required",
		},
		{
			name: "RejectsMultiAgentWithoutDefault",
			fsys: fstest.MapFS{
				"bases/bad/base.json": &fstest.MapFile{
					Data: []byte(`{"id": "bad", "os": "linux", "agents": [{"name": "a"}, {"name": "b"}]}`),
				},
			},
			wantErr: "exactly one required",
		},
	}
	for _, tc := range rejects {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := parseBasesFromFS(tc.fsys)
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

func TestDefaultAgentName(t *testing.T) {
	t.Parallel()

	require.Equal(t, "", defaultAgentName(nil))
	require.Equal(t, "main", defaultAgentName([]BaseAgent{{Name: "main"}, {Name: "gpu"}}),
		"first agent is the default when none is marked")
	require.Equal(t, "gpu", defaultAgentName([]BaseAgent{{Name: "main"}, {Name: "gpu", Default: true}}),
		"the marked agent is the default")
}

package templatebuilder_test

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/templatebuilder"
)

var updateGolden = flag.Bool("update", false, "update golden files")

// testRenderContext builds a BaseRenderContext from defaults and fills
// in deterministic test values for any required variables that have no
// default. This keeps the all-bases test loops working without silently
// producing broken HCL.
func testRenderContext(exampleID string) templatebuilder.BaseRenderContext {
	rc := templatebuilder.DefaultBaseRenderContext(exampleID)
	if rc.Variables == nil {
		rc.Variables = make(map[string]string)
	}
	for _, v := range templatebuilder.BaseVariables(exampleID) {
		if v.Computed || v.Sensitive {
			continue
		}
		if _, ok := rc.Variables[v.Name]; !ok {
			rc.Variables[v.Name] = fmt.Sprintf("%q", "test-"+v.Name)
		}
	}
	return rc
}

func TestRenderBaseTemplate(t *testing.T) {
	t.Parallel()

	t.Run("UnknownBase", func(t *testing.T) {
		t.Parallel()
		_, err := templatebuilder.RenderBaseTemplate("nonexistent", "main.tf.tmpl", templatebuilder.BaseRenderContext{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "unknown base template")
	})

	t.Run("InvalidPath", func(t *testing.T) {
		t.Parallel()
		_, err := templatebuilder.RenderBaseTemplate("docker", "nonexistent.tf.tmpl", templatebuilder.BaseRenderContext{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "not found")
	})

	imageOpts := []templatebuilder.ImageOption{
		{Name: "Ubuntu", Value: "codercom/enterprise-base:ubuntu"},
		{Name: "Custom", Value: "custom/image:latest"},
	}

	t.Run("DockerWithImageOptions", func(t *testing.T) {
		t.Parallel()

		renderCtx := templatebuilder.BaseRenderContext{
			ContainerImage: "custom/image:latest",
			ImageOptions:   imageOpts,
		}
		out, err := templatebuilder.RenderBaseTemplate("docker", "main.tf.tmpl", renderCtx)
		require.NoError(t, err)
		rendered := string(out)
		require.Contains(t, rendered, `data.coder_parameter.container_image.value`)
		require.Contains(t, rendered, `name  = "Ubuntu"`)
		require.Contains(t, rendered, `name  = "Custom"`)
		require.Contains(t, rendered, `coder_parameter`)
	})

	t.Run("KubernetesWithImageOptions", func(t *testing.T) {
		t.Parallel()

		renderCtx := templatebuilder.BaseRenderContext{
			ContainerImage: "custom/image:latest",
			ImageOptions:   imageOpts,
			Variables: map[string]string{
				"namespace":      `"test-ns"`,
				"use_kubeconfig": "false",
			},
		}
		out, err := templatebuilder.RenderBaseTemplate("kubernetes", "main.tf.tmpl", renderCtx)
		require.NoError(t, err)
		rendered := string(out)
		require.Contains(t, rendered, `data.coder_parameter.container_image.value`)
		require.Contains(t, rendered, `name  = "Ubuntu"`)
		require.Contains(t, rendered, `coder_parameter`)
	})

	// MissingKeyErrors is tested via RenderModuleTemplate since base templates
	// are pre-parsed from the embedded catalog and cannot use ad-hoc filesystems.
}

func TestRenderModuleTemplate(t *testing.T) {
	t.Parallel()

	t.Run("InvalidPath", func(t *testing.T) {
		t.Parallel()
		fsys := fstest.MapFS{}
		_, err := templatebuilder.RenderModuleTemplate(fsys, "missing.tf.tmpl", templatebuilder.ModuleRenderContext{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "read template")
	})

	t.Run("RendersAllFields", func(t *testing.T) {
		t.Parallel()
		fsys := fstest.MapFS{
			"test.tf.tmpl": &fstest.MapFile{
				Data: []byte(`module "test" {
  source   = "{{ .RegistryBase }}/coder/test/coder"
  version  = "{{ .PinnedVersion }}"
  agent_id = coder_agent.{{ .AgentResourceName }}.id
  port = {{ .Variables.port }}
}
`),
			},
		}
		ctx := templatebuilder.ModuleRenderContext{
			RegistryBase:      "https://registry.coder.com",
			PinnedVersion:     "1.5.0",
			AgentResourceName: "main",
			Variables:         map[string]string{"port": "8080"},
		}
		out, err := templatebuilder.RenderModuleTemplate(fsys, "test.tf.tmpl", ctx)
		require.NoError(t, err)
		rendered := string(out)
		require.Contains(t, rendered, `"https://registry.coder.com/coder/test/coder"`)
		require.Contains(t, rendered, `"1.5.0"`)
		require.Contains(t, rendered, `coder_agent.main.id`)
		require.Contains(t, rendered, `port = 8080`)
	})

	t.Run("NilVariablesDoesNotPanic", func(t *testing.T) {
		t.Parallel()
		fsys := fstest.MapFS{
			"test.tf.tmpl": &fstest.MapFile{
				Data: []byte(`module "test" {
  source = "{{ .RegistryBase }}"
}
`),
			},
		}
		out, err := templatebuilder.RenderModuleTemplate(fsys, "test.tf.tmpl", templatebuilder.ModuleRenderContext{
			RegistryBase: "https://registry.coder.com",
		})
		require.NoError(t, err)
		require.Contains(t, string(out), "https://registry.coder.com")
	})

	t.Run("MissingKeyErrors", func(t *testing.T) {
		t.Parallel()
		fsys := fstest.MapFS{
			"test.tf.tmpl": &fstest.MapFile{
				Data: []byte(`{{ .Variables.missing_key }}`),
			},
		}
		_, err := templatebuilder.RenderModuleTemplate(fsys, "test.tf.tmpl", templatebuilder.ModuleRenderContext{
			Variables: map[string]string{"other": "value"},
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "execute template")
	})

	t.Run("ParseError", func(t *testing.T) {
		t.Parallel()
		fsys := fstest.MapFS{
			"bad.tf.tmpl": &fstest.MapFile{
				Data: []byte(`{{ .Invalid {{ syntax`),
			},
		}
		_, err := templatebuilder.RenderModuleTemplate(fsys, "bad.tf.tmpl", templatebuilder.ModuleRenderContext{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "parse template")
	})
	t.Run("RealModuleTemplate", func(t *testing.T) {
		t.Parallel()
		modules, err := templatebuilder.LoadModules()
		require.NoError(t, err)

		var csMod templatebuilder.ModuleManifest
		for _, m := range modules {
			if m.ID == "code-server" {
				csMod = m
				break
			}
		}
		require.NotEmpty(t, csMod.ID, "code-server module must exist")

		fsys, err := templatebuilder.ModuleTemplateFS(csMod.ID)
		require.NoError(t, err)

		vars := make(map[string]string)
		for _, v := range csMod.Variables {
			if !v.Computed && !v.Sensitive {
				vars[v.Name] = `"test-value"`
			}
		}

		ctx := templatebuilder.ModuleRenderContext{
			RegistryBase:      "https://registry.coder.com",
			PinnedVersion:     csMod.PinnedVersion,
			AgentResourceName: "main",
			Variables:         vars,
		}
		out, err := templatebuilder.RenderModuleTemplate(fsys, csMod.ID+".tf.tmpl", ctx)
		require.NoError(t, err)
		rendered := string(out)
		require.Contains(t, rendered, `module "code-server"`)
		require.Contains(t, rendered, `coder_agent.main.id`)
		require.Contains(t, rendered, csMod.PinnedVersion)
	})
}

func TestExtractAgentResourceName(t *testing.T) {
	t.Parallel()

	t.Run("DockerBase", func(t *testing.T) {
		t.Parallel()
		rendered, err := templatebuilder.RenderBaseTemplate("docker", "main.tf.tmpl", templatebuilder.DefaultBaseRenderContext("docker"))
		require.NoError(t, err)

		name, err := templatebuilder.ExtractAgentResourceName(rendered)
		require.NoError(t, err)
		require.Equal(t, "main", name)
	})

	t.Run("AWSLinuxBase", func(t *testing.T) {
		t.Parallel()
		rendered, err := templatebuilder.RenderBaseTemplate("aws-linux", "main.tf.tmpl", templatebuilder.DefaultBaseRenderContext("aws-linux"))
		require.NoError(t, err)

		name, err := templatebuilder.ExtractAgentResourceName(rendered)
		require.NoError(t, err)
		require.Equal(t, "dev[0]", name, "counted agent should include index")
	})

	t.Run("CountedAgent", func(t *testing.T) {
		t.Parallel()
		hcl := []byte(`resource "coder_agent" "myagent" {
  count = data.coder_workspace.me.start_count
  arch  = "amd64"
}`)
		name, err := templatebuilder.ExtractAgentResourceName(hcl)
		require.NoError(t, err)
		require.Equal(t, "myagent[0]", name)
	})

	t.Run("UncountedAgent", func(t *testing.T) {
		t.Parallel()
		hcl := []byte(`resource "coder_agent" "main" {
  arch = "amd64"
}`)
		name, err := templatebuilder.ExtractAgentResourceName(hcl)
		require.NoError(t, err)
		require.Equal(t, "main", name)
	})

	t.Run("NoAgent", func(t *testing.T) {
		t.Parallel()
		_, err := templatebuilder.ExtractAgentResourceName([]byte(`resource "docker_container" "workspace" {}`))
		require.Error(t, err)
		require.Contains(t, err.Error(), "no coder_agent")
	})

	t.Run("MultipleAgents", func(t *testing.T) {
		t.Parallel()
		hcl := []byte(`
resource "coder_agent" "first" {}
resource "coder_agent" "second" {}
`)
		_, err := templatebuilder.ExtractAgentResourceName(hcl)
		require.Error(t, err)
		require.Contains(t, err.Error(), "expected exactly one")
		require.Contains(t, err.Error(), "found 2")
	})

	t.Run("NilInput", func(t *testing.T) {
		t.Parallel()
		_, err := templatebuilder.ExtractAgentResourceName(nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "no coder_agent")
	})

	t.Run("EmptyInput", func(t *testing.T) {
		t.Parallel()
		_, err := templatebuilder.ExtractAgentResourceName([]byte{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "no coder_agent")
	})
}

func TestModuleTemplateFS(t *testing.T) {
	t.Parallel()

	t.Run("ValidModule", func(t *testing.T) {
		t.Parallel()
		fsys, err := templatebuilder.ModuleTemplateFS("code-server")
		require.NoError(t, err)
		require.NotNil(t, fsys)
	})

	t.Run("UnknownModule", func(t *testing.T) {
		t.Parallel()
		_, err := templatebuilder.ModuleTemplateFS("nonexistent-module")
		require.Error(t, err)
		require.Contains(t, err.Error(), "not found in embedded catalog")
	})
}

func TestAllBasesRenderAndExtractAgent(t *testing.T) {
	t.Parallel()

	for _, id := range templatebuilder.BaseTemplateIDs() {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			renderCtx := testRenderContext(id)
			rendered, err := templatebuilder.RenderBaseTemplate(id, "main.tf.tmpl", renderCtx)
			require.NoError(t, err, "base %q should render without error", id)
			require.NotEmpty(t, rendered)

			name, err := templatebuilder.ExtractAgentResourceName(rendered)
			require.NoError(t, err, "base %q should have exactly one coder_agent", id)
			require.NotEmpty(t, name)
		})
	}
}

func TestBaseTemplateSnapshot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		exampleID string
	}{
		{exampleID: "docker"},
		{exampleID: "kubernetes"},
		{exampleID: "aws-linux"},
		{exampleID: "aws-windows"},
		{exampleID: "azure-linux"},
		{exampleID: "digitalocean-linux"},
		{exampleID: "gcp-linux"},
		{exampleID: "gcp-windows"},
		{exampleID: "scratch"},
	}

	for _, tc := range tests {
		t.Run(tc.exampleID, func(t *testing.T) {
			t.Parallel()

			renderCtx := testRenderContext(tc.exampleID)
			rendered, err := templatebuilder.RenderBaseTemplate(tc.exampleID, "main.tf.tmpl", renderCtx)
			require.NoError(t, err)
			require.NotEmpty(t, rendered)

			goldenPath := filepath.Join("testdata", tc.exampleID+".tf.golden")

			if *updateGolden {
				err := os.MkdirAll("testdata", 0o755)
				require.NoError(t, err)
				err = os.WriteFile(goldenPath, rendered, 0o600)
				require.NoError(t, err)
				return
			}

			expected, err := os.ReadFile(goldenPath)
			require.NoError(t, err, "golden file %s not found; run with -update to create", goldenPath)
			require.Equal(t, string(expected), string(rendered),
				"rendered output for %s does not match golden file; run with -update to regenerate", tc.exampleID)
		})
	}
}

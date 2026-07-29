package templatebuilder_test

import (
	"archive/tar"
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/templatebuilder"
)

func TestCompose(t *testing.T) {
	t.Parallel()

	t.Run("BaseOnly", func(t *testing.T) {
		t.Parallel()
		result, err := templatebuilder.Compose(templatebuilder.ComposeRequest{
			BaseTemplateID: "docker",
			RegistryURL:    "https://registry.coder.com",
		})
		require.NoError(t, err)
		require.NotEmpty(t, result.MainTF)
		require.Contains(t, string(result.MainTF), `resource "coder_agent" "main"`)
		require.Empty(t, result.ModulesTF)
		require.NotEmpty(t, result.Readme, "compose should include base README")
	})

	t.Run("BaseWithModuleAndVariableOverride", func(t *testing.T) {
		t.Parallel()
		result, err := templatebuilder.Compose(templatebuilder.ComposeRequest{
			BaseTemplateID: "docker",
			RegistryURL:    "https://registry.coder.com",
			Modules: []templatebuilder.ComposeModule{
				{
					ID: "code-server",
					Variables: map[string]string{
						"port": "9999",
					},
				},
			},
		})
		require.NoError(t, err)
		require.NotEmpty(t, result.MainTF)
		require.NotEmpty(t, result.ModulesTF)

		modules := string(result.ModulesTF)
		require.Contains(t, modules, `module "code-server"`)
		require.Contains(t, modules, `coder_agent.main.id`)
		require.Contains(t, modules, `registry.coder.com`)
		require.Regexp(t, `port\s+=\s+9999`, modules)
	})

	t.Run("AWSLinuxAgentName", func(t *testing.T) {
		t.Parallel()
		result, err := templatebuilder.Compose(templatebuilder.ComposeRequest{
			BaseTemplateID: "aws-linux",
			RegistryURL:    "https://registry.coder.com",
			Modules: []templatebuilder.ComposeModule{
				{ID: "git-commit-signing"},
			},
		})
		require.NoError(t, err)
		require.Contains(t, string(result.ModulesTF), `coder_agent.dev[0].id`)
	})

	t.Run("AWSLinuxExtraFiles", func(t *testing.T) {
		t.Parallel()
		result, err := templatebuilder.Compose(templatebuilder.ComposeRequest{
			BaseTemplateID: "aws-linux",
			RegistryURL:    "https://registry.coder.com",
		})
		require.NoError(t, err)
		require.NotNil(t, result.ExtraFiles, "aws-linux should have extra files")
		require.Contains(t, result.ExtraFiles, "cloud-init/cloud-config.yaml.tftpl")
		require.Contains(t, result.ExtraFiles, "cloud-init/userdata.sh.tftpl")
	})

	t.Run("GCPLinuxBaseWithProjectID", func(t *testing.T) {
		t.Parallel()
		result, err := templatebuilder.Compose(templatebuilder.ComposeRequest{
			BaseTemplateID:     "gcp-linux",
			BaseVariableValues: map[string]string{"project_id": "my-gcp-project"},
			RegistryURL:        "https://registry.coder.com",
		})
		require.NoError(t, err)
		require.NotEmpty(t, result.MainTF)
		mainTF := string(result.MainTF)
		require.Contains(t, mainTF, `resource "coder_agent" "main"`)
		require.Contains(t, mainTF, `default     = "my-gcp-project"`)
		require.Contains(t, mainTF, `project = var.project_id`)
	})

	t.Run("GCPWindowsBase", func(t *testing.T) {
		t.Parallel()
		result, err := templatebuilder.Compose(templatebuilder.ComposeRequest{
			BaseTemplateID:     "gcp-windows",
			BaseVariableValues: map[string]string{"project_id": "my-gcp-project"},
			RegistryURL:        "https://registry.coder.com",
		})
		require.NoError(t, err)
		require.NotEmpty(t, result.MainTF)
		mainTF := string(result.MainTF)
		require.Contains(t, mainTF, `resource "coder_agent" "main"`)
		require.Contains(t, mainTF, `default     = "my-gcp-project"`)
		require.Contains(t, mainTF, `project = var.project_id`)
	})

	t.Run("GCPMissingRequiredBaseVariable", func(t *testing.T) {
		t.Parallel()
		_, err := templatebuilder.Compose(templatebuilder.ComposeRequest{
			BaseTemplateID: "gcp-linux",
			RegistryURL:    "https://registry.coder.com",
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), `variable "project_id" is required`)
	})

	t.Run("AzureLinuxExtraFiles", func(t *testing.T) {
		t.Parallel()
		result, err := templatebuilder.Compose(templatebuilder.ComposeRequest{
			BaseTemplateID: "azure-linux",
			RegistryURL:    "https://registry.coder.com",
		})
		require.NoError(t, err)
		require.Contains(t, result.ExtraFiles, "cloud-init/cloud-config.yaml.tftpl")
	})

	t.Run("SensitiveVariable", func(t *testing.T) {
		t.Parallel()
		result, err := templatebuilder.Compose(templatebuilder.ComposeRequest{
			BaseTemplateID: "docker",
			RegistryURL:    "https://registry.coder.com",
			Modules: []templatebuilder.ComposeModule{
				{ID: "claude-code"},
			},
		})
		require.NoError(t, err)
		modules := string(result.ModulesTF)
		// claude-code has a sensitive variable (claude_code_oauth_token)
		// that renders as a top-level variable block + var. reference.
		require.Contains(t, modules, `variable "claude_code_oauth_token"`)
		require.Contains(t, modules, `sensitive   = true`)
		require.Contains(t, modules, `var.claude_code_oauth_token`)
	})

	t.Run("MultipleModulesWithRequiredVariable", func(t *testing.T) {
		t.Parallel()
		result, err := templatebuilder.Compose(templatebuilder.ComposeRequest{
			BaseTemplateID: "docker",
			RegistryURL:    "https://registry.coder.com",
			Modules: []templatebuilder.ComposeModule{
				{ID: "code-server"},
				{
					ID: "git-clone",
					Variables: map[string]string{
						"url": "https://github.com/coder/coder",
					},
				},
			},
		})
		require.NoError(t, err)
		modules := string(result.ModulesTF)
		require.Contains(t, modules, `module "code-server"`)
		require.Contains(t, modules, `module "git-clone"`)
		require.Contains(t, modules, `"https://github.com/coder/coder"`)
	})

	t.Run("CustomRegistryURL", func(t *testing.T) {
		t.Parallel()
		result, err := templatebuilder.Compose(templatebuilder.ComposeRequest{
			BaseTemplateID: "docker",
			RegistryURL:    "https://registry.internal.corp",
			Modules: []templatebuilder.ComposeModule{
				{ID: "code-server"},
			},
		})
		require.NoError(t, err)
		require.Contains(t, string(result.ModulesTF), `registry.internal.corp`)
	})

	t.Run("DuplicateModuleError", func(t *testing.T) {
		t.Parallel()
		_, err := templatebuilder.Compose(templatebuilder.ComposeRequest{
			BaseTemplateID: "docker",
			RegistryURL:    "https://registry.coder.com",
			Modules: []templatebuilder.ComposeModule{
				{ID: "code-server"},
				{ID: "code-server"},
			},
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), `duplicate module "code-server"`)
	})

	t.Run("ConflictingModuleError", func(t *testing.T) {
		t.Parallel()
		_, err := templatebuilder.Compose(templatebuilder.ComposeRequest{
			BaseTemplateID: "docker",
			RegistryURL:    "https://registry.coder.com",
			Modules: []templatebuilder.ComposeModule{
				{ID: "code-server"},
				{ID: "vscode-web"},
			},
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "conflicts with")
	})

	t.Run("DockerNoExtraFiles", func(t *testing.T) {
		t.Parallel()
		result, err := templatebuilder.Compose(templatebuilder.ComposeRequest{
			BaseTemplateID: "docker",
			RegistryURL:    "https://registry.coder.com",
		})
		require.NoError(t, err)
		require.Empty(t, result.ExtraFiles, "docker should have no extra files")
	})

	t.Run("UnknownBase", func(t *testing.T) {
		t.Parallel()
		_, err := templatebuilder.Compose(templatebuilder.ComposeRequest{
			BaseTemplateID: "nonexistent",
			RegistryURL:    "https://registry.coder.com",
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "unknown base template")
	})

	t.Run("UnknownModule", func(t *testing.T) {
		t.Parallel()
		_, err := templatebuilder.Compose(templatebuilder.ComposeRequest{
			BaseTemplateID: "docker",
			RegistryURL:    "https://registry.coder.com",
			Modules: []templatebuilder.ComposeModule{
				{ID: "nonexistent-module"},
			},
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), `unknown module "nonexistent-module"`)
	})

	t.Run("UnknownVariableKeyRejected", func(t *testing.T) {
		t.Parallel()
		_, err := templatebuilder.Compose(templatebuilder.ComposeRequest{
			BaseTemplateID: "docker",
			RegistryURL:    "https://registry.coder.com",
			Modules: []templatebuilder.ComposeModule{
				{
					ID: "code-server",
					Variables: map[string]string{
						"nonexistent_var": "value",
					},
				},
			},
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), `module "code-server"`)
		require.Contains(t, err.Error(), `unknown variable "nonexistent_var"`)
	})

	t.Run("InvalidVariableValueRejected", func(t *testing.T) {
		t.Parallel()
		_, err := templatebuilder.Compose(templatebuilder.ComposeRequest{
			BaseTemplateID: "docker",
			RegistryURL:    "https://registry.coder.com",
			Modules: []templatebuilder.ComposeModule{
				{
					ID: "code-server",
					Variables: map[string]string{
						"port": "not-a-number",
					},
				},
			},
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), `module "code-server"`)
		require.Contains(t, err.Error(), `variable "port"`)
	})

	t.Run("HCLInjectionRejected", func(t *testing.T) {
		t.Parallel()
		_, err := templatebuilder.Compose(templatebuilder.ComposeRequest{
			BaseTemplateID: "docker",
			RegistryURL:    "https://registry.coder.com",
			Modules: []templatebuilder.ComposeModule{
				{
					ID: "code-server",
					Variables: map[string]string{
						"folder": "${var.evil}",
					},
				},
			},
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "interpolation")
	})

	t.Run("DockerDefaultContainerImage", func(t *testing.T) {
		t.Parallel()
		result, err := templatebuilder.Compose(templatebuilder.ComposeRequest{
			BaseTemplateID: "docker",
			RegistryURL:    "https://registry.coder.com",
		})
		require.NoError(t, err)
		require.Contains(t, string(result.MainTF), `"codercom/example-base:ubuntu"`)
	})

	t.Run("DockerCustomContainerImage", func(t *testing.T) {
		t.Parallel()
		result, err := templatebuilder.Compose(templatebuilder.ComposeRequest{
			BaseTemplateID: "docker",
			RegistryURL:    "https://registry.coder.com",
			BaseVariableValues: map[string]string{
				"container_image": "myregistry/myimage:v2",
			},
		})
		require.NoError(t, err)
		mainTF := string(result.MainTF)
		require.Contains(t, mainTF, `"myregistry/myimage:v2"`)
		require.NotContains(t, mainTF, `codercom/example-base:ubuntu`)
	})

	t.Run("KubernetesDefaultContainerImage", func(t *testing.T) {
		t.Parallel()
		result, err := templatebuilder.Compose(templatebuilder.ComposeRequest{
			BaseTemplateID: "kubernetes",
			RegistryURL:    "https://registry.coder.com",
			BaseVariableValues: map[string]string{
				"namespace": "default",
			},
		})
		require.NoError(t, err)
		require.Contains(t, string(result.MainTF), `"codercom/example-base:ubuntu"`)
	})

	t.Run("KubernetesCustomContainerImage", func(t *testing.T) {
		t.Parallel()
		result, err := templatebuilder.Compose(templatebuilder.ComposeRequest{
			BaseTemplateID: "kubernetes",
			RegistryURL:    "https://registry.coder.com",
			BaseVariableValues: map[string]string{
				"namespace":       "default",
				"container_image": "custom/workspace:latest",
			},
		})
		require.NoError(t, err)
		mainTF := string(result.MainTF)
		require.Contains(t, mainTF, `"custom/workspace:latest"`)
		require.NotContains(t, mainTF, `codercom/example-base:ubuntu`)
	})

	t.Run("MissingRequiredVariable", func(t *testing.T) {
		t.Parallel()
		// git-clone has a required "url" variable with no default.
		// Omitting it should cause a render error from missingkey=error.
		_, err := templatebuilder.Compose(templatebuilder.ComposeRequest{
			BaseTemplateID: "docker",
			RegistryURL:    "https://registry.coder.com",
			Modules: []templatebuilder.ComposeModule{
				{ID: "git-clone"},
			},
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), `variable "url"`)
		require.Contains(t, err.Error(), "is required")
	})
}

func TestBundleTar(t *testing.T) {
	t.Parallel()

	t.Run("NilResult", func(t *testing.T) {
		t.Parallel()
		_, err := templatebuilder.BundleTar(nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "nil")
	})

	t.Run("MainOnly", func(t *testing.T) {
		t.Parallel()
		result := &templatebuilder.ComposeResult{
			MainTF: []byte("resource {}"),
		}
		data, err := templatebuilder.BundleTar(result)
		require.NoError(t, err)

		files := extractTar(t, data)
		require.Contains(t, files, "main.tf")
		require.NotContains(t, files, "modules.tf")
		require.NotContains(t, files, "README.md")
		require.Equal(t, "resource {}", files["main.tf"])
	})

	t.Run("MainAndModules", func(t *testing.T) {
		t.Parallel()
		result := &templatebuilder.ComposeResult{
			MainTF:    []byte("resource {}"),
			ModulesTF: []byte("module {}"),
		}
		data, err := templatebuilder.BundleTar(result)
		require.NoError(t, err)

		files := extractTar(t, data)
		require.Contains(t, files, "main.tf")
		require.Contains(t, files, "modules.tf")
		require.NotContains(t, files, "README.md")
		require.Equal(t, "resource {}", files["main.tf"])
		require.Equal(t, "module {}", files["modules.tf"])
	})

	t.Run("IncludesReadme", func(t *testing.T) {
		t.Parallel()
		result := &templatebuilder.ComposeResult{
			MainTF: []byte("resource {}"),
			Readme: []byte("# My Template\n"),
		}
		data, err := templatebuilder.BundleTar(result)
		require.NoError(t, err)

		files := extractTar(t, data)
		require.Contains(t, files, "main.tf")
		require.Contains(t, files, "README.md")
		require.Equal(t, "# My Template\n", files["README.md"])
	})

	t.Run("RoundTrip", func(t *testing.T) {
		t.Parallel()
		result, err := templatebuilder.Compose(templatebuilder.ComposeRequest{
			BaseTemplateID: "docker",
			RegistryURL:    "https://registry.coder.com",
			Modules: []templatebuilder.ComposeModule{
				{ID: "code-server"},
			},
		})
		require.NoError(t, err)

		data, err := templatebuilder.BundleTar(result)
		require.NoError(t, err)

		files := extractTar(t, data)
		require.Equal(t, string(result.MainTF), files["main.tf"])
		require.Equal(t, string(result.ModulesTF), files["modules.tf"])
		require.Equal(t, string(result.Readme), files["README.md"])
	})

	t.Run("ExtraFilesInTar", func(t *testing.T) {
		t.Parallel()
		result := &templatebuilder.ComposeResult{
			MainTF: []byte("resource {}"),
			ExtraFiles: map[string][]byte{
				"cloud-init/config.yaml.tftpl": []byte("cloud config"),
				"cloud-init/userdata.sh.tftpl": []byte("userdata"),
			},
		}
		data, err := templatebuilder.BundleTar(result)
		require.NoError(t, err)

		files := extractTar(t, data)
		require.Contains(t, files, "cloud-init/", "directory entry should be present for subdirectories")
		require.Contains(t, files, "main.tf")
		require.Contains(t, files, "cloud-init/config.yaml.tftpl")
		require.Contains(t, files, "cloud-init/userdata.sh.tftpl")
		require.Equal(t, "cloud config", files["cloud-init/config.yaml.tftpl"])
		require.Equal(t, "userdata", files["cloud-init/userdata.sh.tftpl"])
	})

	t.Run("NestedStaticFileDirEntries", func(t *testing.T) {
		t.Parallel()
		result := &templatebuilder.ComposeResult{
			MainTF: []byte("resource {}"),
			ExtraFiles: map[string][]byte{
				"a/b/c/deep.txt": []byte("deep"),
				"top.txt":        []byte("top"),
			},
		}
		data, err := templatebuilder.BundleTar(result)
		require.NoError(t, err)

		files := extractTar(t, data)
		require.Contains(t, files, "a/", "top-level parent dir entry")
		require.Contains(t, files, "a/b/", "intermediate parent dir entry")
		require.Contains(t, files, "a/b/c/", "leaf parent dir entry")
		require.Contains(t, files, "a/b/c/deep.txt")
		require.Contains(t, files, "top.txt")
		// top.txt is at root, so no extra directory entry needed.
	})

	t.Run("AWSLinuxRoundTrip", func(t *testing.T) {
		t.Parallel()
		result, err := templatebuilder.Compose(templatebuilder.ComposeRequest{
			BaseTemplateID: "aws-linux",
			RegistryURL:    "https://registry.coder.com",
		})
		require.NoError(t, err)

		data, err := templatebuilder.BundleTar(result)
		require.NoError(t, err)

		files := extractTar(t, data)
		require.Contains(t, files, "main.tf")
		require.Contains(t, files, "README.md")
		require.Contains(t, files, "cloud-init/cloud-config.yaml.tftpl")
		require.Contains(t, files, "cloud-init/userdata.sh.tftpl")
	})

	t.Run("ReproducibleArchive", func(t *testing.T) {
		t.Parallel()
		result := &templatebuilder.ComposeResult{
			MainTF:    []byte("resource {}"),
			ModulesTF: []byte("module {}"),
		}
		data1, err := templatebuilder.BundleTar(result)
		require.NoError(t, err)
		data2, err := templatebuilder.BundleTar(result)
		require.NoError(t, err)
		require.Equal(t, data1, data2, "identical inputs should produce identical archives")
	})
}

// extractTar reads a tar archive and returns a map of filename to content.
func extractTar(t *testing.T, data []byte) map[string]string {
	t.Helper()
	tr := tar.NewReader(bytes.NewReader(data))
	files := make(map[string]string)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		body, err := io.ReadAll(tr)
		require.NoError(t, err)
		files[hdr.Name] = string(body)
	}
	return files
}

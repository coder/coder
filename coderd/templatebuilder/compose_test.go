package templatebuilder_test

import (
	"archive/tar"
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os/exec"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/templatebuilder"
	"github.com/coder/coder/v2/codersdk"
)

func TestCompose(t *testing.T) {
	t.Parallel()

	t.Run("BaseOnly", func(t *testing.T) {
		t.Parallel()
		result, err := templatebuilder.Compose(templatebuilder.ComposeRequest{
			BaseTemplateID: "docker",
			RegistryURL:    "registry.coder.com",
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
			RegistryURL:    "registry.coder.com",
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
			RegistryURL:    "registry.coder.com",
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
			RegistryURL:    "registry.coder.com",
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
			RegistryURL:        "registry.coder.com",
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
			RegistryURL:        "registry.coder.com",
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
			RegistryURL:    "registry.coder.com",
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), `variable "project_id" is required`)
	})

	t.Run("AzureLinuxExtraFiles", func(t *testing.T) {
		t.Parallel()
		result, err := templatebuilder.Compose(templatebuilder.ComposeRequest{
			BaseTemplateID: "azure-linux",
			RegistryURL:    "registry.coder.com",
		})
		require.NoError(t, err)
		require.Contains(t, result.ExtraFiles, "cloud-init/cloud-config.yaml.tftpl")
	})

	t.Run("SensitiveVariable", func(t *testing.T) {
		t.Parallel()
		result, err := templatebuilder.Compose(templatebuilder.ComposeRequest{
			BaseTemplateID: "docker",
			RegistryURL:    "registry.coder.com",
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
			RegistryURL:    "registry.coder.com",
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
			RegistryURL:    "registry.coder.com",
			Modules: []templatebuilder.ComposeModule{
				{ID: "code-server"},
				{ID: "code-server"},
			},
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), `duplicate module "code-server"`)
	})

	t.Run("BaseIncludedModuleCollisionError", func(t *testing.T) {
		t.Parallel()
		// The quickstart base already declares module "git-clone"; selecting
		// the catalog git-clone module in the wizard would render a duplicate
		// module block, so compose must reject it.
		_, err := templatebuilder.Compose(templatebuilder.ComposeRequest{
			BaseTemplateID: "quickstart",
			RegistryURL:    "registry.coder.com",
			Modules: []templatebuilder.ComposeModule{
				{ID: "git-clone"},
			},
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), `module "git-clone" is already included by this base template`)
	})

	t.Run("BaseAllowsNonIncludedModule", func(t *testing.T) {
		t.Parallel()
		// Quickstart only includes git-clone; other catalog modules such as
		// code-server compose normally on top of it.
		result, err := templatebuilder.Compose(templatebuilder.ComposeRequest{
			BaseTemplateID: "quickstart",
			RegistryURL:    "registry.coder.com",
			Modules: []templatebuilder.ComposeModule{
				{ID: "code-server"},
			},
		})
		require.NoError(t, err)
		require.Contains(t, string(result.ModulesTF), `module "code-server"`)
	})

	t.Run("ConflictingModuleError", func(t *testing.T) {
		t.Parallel()
		_, err := templatebuilder.Compose(templatebuilder.ComposeRequest{
			BaseTemplateID: "docker",
			RegistryURL:    "registry.coder.com",
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
			RegistryURL:    "registry.coder.com",
		})
		require.NoError(t, err)
		require.Empty(t, result.ExtraFiles, "docker should have no extra files")
	})

	t.Run("UnknownBase", func(t *testing.T) {
		t.Parallel()
		_, err := templatebuilder.Compose(templatebuilder.ComposeRequest{
			BaseTemplateID: "nonexistent",
			RegistryURL:    "registry.coder.com",
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "unknown base template")
	})

	t.Run("UnknownModule", func(t *testing.T) {
		t.Parallel()
		_, err := templatebuilder.Compose(templatebuilder.ComposeRequest{
			BaseTemplateID: "docker",
			RegistryURL:    "registry.coder.com",
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
			RegistryURL:    "registry.coder.com",
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
			RegistryURL:    "registry.coder.com",
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
			RegistryURL:    "registry.coder.com",
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
			RegistryURL:    "registry.coder.com",
		})
		require.NoError(t, err)
		require.Contains(t, string(result.MainTF), `"codercom/example-base:ubuntu"`)
	})

	t.Run("DockerCustomContainerImage", func(t *testing.T) {
		t.Parallel()
		result, err := templatebuilder.Compose(templatebuilder.ComposeRequest{
			BaseTemplateID: "docker",
			RegistryURL:    "registry.coder.com",
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
			RegistryURL:    "registry.coder.com",
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
			RegistryURL:    "registry.coder.com",
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
			RegistryURL:    "registry.coder.com",
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
			RegistryURL:    "registry.coder.com",
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
			RegistryURL:    "registry.coder.com",
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

// optionValuePattern matches a quoted `value = "..."` assignment. In the
// quickstart base such a quoted literal only appears inside the languages
// selector's option {} blocks: Docker labels assign `value = data....` and
// presets assign `languages = jsonencode(...)`, neither of which is a quoted
// `value = "..."`.
var optionValuePattern = regexp.MustCompile(`(?m)^\s*value\s*=\s*"([^"]+)"`)

// hasLanguageDispatchPattern matches the `if has_language <name>` call sites in
// the quickstart language-install script (not the has_language definition).
var hasLanguageDispatchPattern = regexp.MustCompile(`(?m)^\s*if has_language (\S+?);`)

// TestQuickstartLanguageSelectorMatchesInstallScript enforces that the
// quickstart "languages" selector options and the language-install script's
// has_language dispatch branches describe the same set of languages. They are
// two hand-maintained lists with nothing else binding them: if one gains or
// loses a language without the other, a selected language would silently
// install nothing (or a branch would be dead). This test fails on that drift.
func TestQuickstartLanguageSelectorMatchesInstallScript(t *testing.T) {
	t.Parallel()

	mainTF, err := templatebuilder.RenderBaseTemplate(
		"quickstart", "main.tf.tmpl", templatebuilder.DefaultBaseRenderContext("quickstart"))
	require.NoError(t, err)

	var selectorValues []string
	for _, m := range optionValuePattern.FindAllSubmatch(mainTF, -1) {
		selectorValues = append(selectorValues, string(m[1]))
	}
	require.NotEmpty(t, selectorValues,
		"expected the quickstart languages selector to declare options")

	fsys, err := templatebuilder.BaseTemplateFS("quickstart")
	require.NoError(t, err)
	script, err := fs.ReadFile(fsys, "install-languages.sh.tftpl")
	require.NoError(t, err)

	var dispatchNames []string
	for _, m := range hasLanguageDispatchPattern.FindAllSubmatch(script, -1) {
		dispatchNames = append(dispatchNames, string(m[1]))
	}
	require.NotEmpty(t, dispatchNames,
		"expected the install script to dispatch on has_language")

	require.ElementsMatch(t, selectorValues, dispatchNames,
		"quickstart languages selector options must match the install script's has_language branches")
}

// TestQuickstartToolchainsPersistUnderHome guards the fix for the every-start
// reinstall: Go, Node.js, and Java must install under $HOME (the persistent home
// volume) instead of into the ephemeral container, so a workspace restart reuses
// them rather than re-fetching them from the network. It asserts the system-level
// install mechanisms this change removed do not creep back in, and that each of
// the three toolchains still puts its bin directory under $HOME on PATH, exposed
// both through the coder_agent env (which reaches every agent-spawned session)
// and a ~/.profile fallback. Python and C/C++ come from the base image and Rust
// already persists under ~/.cargo, so they are out of scope here.
func TestQuickstartToolchainsPersistUnderHome(t *testing.T) {
	t.Parallel()

	fsys, err := templatebuilder.BaseTemplateFS("quickstart")
	require.NoError(t, err)
	raw, err := fs.ReadFile(fsys, "install-languages.sh.tftpl")
	require.NoError(t, err)
	script := string(raw)

	// System-level installs that landed in the ephemeral container and so
	// re-downloaded on every start. None of these may return.
	for _, ephemeral := range []string{
		"/usr/local/go",                 // Go unpacked into the system prefix
		"deb.nodesource.com",            // Node.js installed via apt
		"apt-get install -y -qq nodejs", // Node.js installed via apt
		"openjdk",                       // Java installed via apt
	} {
		require.NotContains(t, script, ephemeral,
			"Go/Node.js/Java must persist under $HOME, not reinstall into the ephemeral container")
	}

	// Each network toolchain must put its bin directory under the persistent home
	// volume on PATH, so it survives a restart.
	for _, persisted := range []string{
		"$HOME/.local/go/bin",   // Go
		"$HOME/.local/node/bin", // Node.js
		"$HOME/.local/java/bin", // Java
	} {
		require.Contains(t, script, persisted,
			"expected the toolchain to install under the persistent home volume")
	}

	// PATH must reach every session, including the non-login, non-interactive
	// shells coder ssh and remote IDEs use. The durable mechanism is the
	// coder_agent env, which the agent applies to every session it spawns
	// regardless of login or interactivity; ~/.profile (login shells only) is a
	// fallback for shells the agent does not spawn. Assert the agent env carries
	// each toolchain bin dir on PATH.
	rc := testRenderContext("quickstart")
	mainTF, err := templatebuilder.RenderBaseTemplate("quickstart", "main.tf.tmpl", rc)
	require.NoError(t, err)
	require.Regexp(t, `PATH\s*=\s*"\$PATH:`, string(mainTF),
		"coder_agent env must prepend the existing PATH")
	for _, binDir := range []string{
		"$HOME/.local/go/bin",
		"$HOME/go/bin",
		"$HOME/.local/node/bin",
		"$HOME/.local/java/bin",
	} {
		require.Contains(t, string(mainTF), binDir,
			"coder_agent env PATH must include %s so every agent session sees the toolchain", binDir)
	}

	// Keep a ~/.profile fallback for login shells the agent does not spawn, and
	// do not rely on ~/.bashrc, which returns early in non-interactive shells.
	require.Contains(t, script, "$HOME/.profile",
		"the install script should persist PATH to ~/.profile as a login-shell fallback")
	require.NotContains(t, script, ".bashrc",
		"~/.bashrc returns early in non-interactive shells; use ~/.profile plus the agent env")
}

// TestQuickstartInstallScriptRendersValidBash renders the quickstart language
// install template the way Terraform's templatefile() does, then syntax-checks
// the result with `bash -n` and lints it with `shellcheck`. The .sh.tftpl
// extension hides this base from lint/shellcheck and fmt/shfmt (both match only
// *.sh), so without this test the script has no mechanical coverage: a template
// edit that renders to broken shell, or a quoting/expansion defect `bash -n`
// cannot see, would ship silently. This guards the whole file, including the
// PATH and atomic-install logic other tests only assert on as text.
func TestQuickstartInstallScriptRendersValidBash(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("bash is not available on Windows runners")
	}
	bashPath, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not found in PATH")
	}

	fsys, err := templatebuilder.BaseTemplateFS("quickstart")
	require.NoError(t, err)
	raw, err := fs.ReadFile(fsys, "install-languages.sh.tftpl")
	require.NoError(t, err)

	// Emulate Terraform templatefile(): the only interpolation token in this
	// file is ${LANGUAGES}, and every shell brace expansion is escaped as
	// $${...}, which Terraform unescapes to ${...}. Substitute all selectable
	// languages so every install branch is present in the checked script.
	rendered := strings.ReplaceAll(string(raw), "${LANGUAGES}", "python,nodejs,go,rust,java,cpp")
	rendered = strings.ReplaceAll(rendered, "$${", "${")

	cmd := exec.Command(bashPath, "-n")
	cmd.Stdin = strings.NewReader(rendered)
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "rendered install script failed `bash -n`:\n%s", out)

	// shellcheck catches quoting and expansion defects that are valid bash
	// syntax (so `bash -n` passes) but wrong at runtime, e.g. an unquoted
	// expansion that word-splits. Skip when the linter is absent so the test
	// stays hermetic on runners without it.
	shellcheckPath, err := exec.LookPath("shellcheck")
	if err != nil {
		t.Skip("shellcheck not found in PATH")
	}
	scCmd := exec.Command(shellcheckPath, "-s", "bash", "-")
	scCmd.Stdin = strings.NewReader(rendered)
	scOut, err := scCmd.CombinedOutput()
	require.NoErrorf(t, err, "rendered install script failed shellcheck:\n%s", scOut)
}

// stripHCLCommentLines drops whole-line "#" comments so a registry host left in
// a module source is caught while the "# Module docs (public registry):
// registry.coder.com/modules/..." links, which legitimately name the public
// registry, are excluded by construction.
func stripHCLCommentLines(s string) string {
	var b strings.Builder
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		_, _ = b.WriteString(line)
		_ = b.WriteByte('\n')
	}
	return b.String()
}

func TestRenderBaseHonorsRegistryMirror(t *testing.T) {
	t.Parallel()

	const mirror = "mirror.internal.example"

	// resolvesThroughMirror reports whether a module source resolves through the
	// configured registry: the mirror host, or a hostless local path (./ or ../).
	resolvesThroughMirror := func(source string) bool {
		return strings.HasPrefix(source, mirror+"/") ||
			strings.HasPrefix(source, "./") ||
			strings.HasPrefix(source, "../")
	}

	// Positive anchor: quickstart embeds git-clone, so the mirror must appear in
	// its rendered source. This keeps the per-base sweep below from passing
	// vacuously on a base with no embedded module.
	rc := testRenderContext("quickstart")
	rc.RegistryBase = mirror
	rendered, err := templatebuilder.RenderBaseTemplate("quickstart", "main.tf.tmpl", rc)
	require.NoError(t, err)
	require.Contains(t, string(rendered), mirror+"/coder/git-clone/coder",
		"quickstart must render its embedded module source against the configured registry")

	// Every base must honor the mirror: every rendered module block's `source`
	// must resolve through it. Reading each source from the parsed HCL (not a
	// substring scan) makes a hostless, third-party, or default-registry source
	// fail while never mistaking a comment or heredoc body for a module source.
	// Sorting BaseTemplateIDs() pins the otherwise map-ordered subtests.
	ids := templatebuilder.BaseTemplateIDs()
	slices.Sort(ids)
	for _, id := range ids {
		t.Run(id, func(t *testing.T) {
			t.Parallel()

			rc := testRenderContext(id)
			rc.RegistryBase = mirror
			rendered, err := templatebuilder.RenderBaseTemplate(id, "main.tf.tmpl", rc)
			require.NoError(t, err)
			// Guard against a vacuous sweep: a parse failure or a non-static source
			// makes ExtractModuleSources drop entries, so require one extracted
			// source per module block before asserting on them.
			sources := templatebuilder.ExtractModuleSources(rendered)
			require.Len(t, sources, len(templatebuilder.ExtractModuleNames(rendered)),
				"base %q: every module block must have a static, extractable source", id)
			for _, source := range sources {
				require.True(t, resolvesThroughMirror(source),
					"base %q module source %q must resolve through the configured registry %q, not a hardcoded host or namespace",
					id, source, mirror)
			}
		})
	}
}

func TestComposeThreadsRegistryURLToBase(t *testing.T) {
	t.Parallel()

	const mirror = "mirror.internal.example"

	// Compose threads ComposeRequest.RegistryURL into base rendering, so the
	// quickstart base's embedded git-clone module resolves through the mirror.
	res, err := templatebuilder.Compose(templatebuilder.ComposeRequest{
		BaseTemplateID: "quickstart",
		RegistryURL:    mirror,
	})
	require.NoError(t, err)
	require.Contains(t, string(res.MainTF), mirror+"/coder/git-clone/coder")
	require.NotContains(t, string(res.MainTF),
		codersdk.DefaultTemplateBuilderRegistryURL+"/coder/git-clone/coder")

	// With RegistryURL unset, the base falls back to the canonical default
	// instead of rendering a registry-less source.
	resDefault, err := templatebuilder.Compose(templatebuilder.ComposeRequest{
		BaseTemplateID: "quickstart",
	})
	require.NoError(t, err)
	require.Contains(t, string(resDefault.MainTF),
		codersdk.DefaultTemplateBuilderRegistryURL+"/coder/git-clone/coder")
}

// TestComposeNormalizesRegistryURL covers the boundary normalization Compose
// applies to ComposeRequest.RegistryURL: scheme and trailing slash stripped,
// empty defaults on the module path, and a corrupting value rejected.
func TestComposeNormalizesRegistryURL(t *testing.T) {
	t.Parallel()

	t.Run("StripsSchemeAndTrailingSlash", func(t *testing.T) {
		t.Parallel()
		res, err := templatebuilder.Compose(templatebuilder.ComposeRequest{
			BaseTemplateID: "quickstart",
			RegistryURL:    "https://mirror.internal.example/",
		})
		require.NoError(t, err)
		mainTF := string(res.MainTF)
		require.Contains(t, mainTF, "mirror.internal.example/coder/git-clone/coder")
		require.NotContains(t, mainTF, "https://mirror.internal.example")
		require.NotContains(t, mainTF, "mirror.internal.example//coder")
	})

	t.Run("EmptyDefaultsOnModulePath", func(t *testing.T) {
		t.Parallel()
		// CODER_TEMPLATE_BUILDER_REGISTRY_URL= (present but empty) reaches Compose
		// as ""; the module path must default too, not render a registry-less source.
		res, err := templatebuilder.Compose(templatebuilder.ComposeRequest{
			BaseTemplateID: "docker",
			Modules:        []templatebuilder.ComposeModule{{ID: "code-server"}},
		})
		require.NoError(t, err)
		require.NotEmpty(t, res.ModulesTF)
		require.Contains(t, string(res.ModulesTF),
			codersdk.DefaultTemplateBuilderRegistryURL+"/coder/code-server/coder")
		require.NotContains(t, string(res.ModulesTF), `"/coder/code-server/coder"`)
	})

	t.Run("RejectsQuote", func(t *testing.T) {
		t.Parallel()
		_, err := templatebuilder.Compose(templatebuilder.ComposeRequest{
			BaseTemplateID: "quickstart",
			RegistryURL:    `mirror.example.com/"`,
		})
		require.ErrorContains(t, err, "bare host")
	})

	t.Run("StripsUppercaseScheme", func(t *testing.T) {
		t.Parallel()
		// A scheme is case-insensitive by RFC, so an uppercase scheme must be
		// stripped, not left to render into the source.
		res, err := templatebuilder.Compose(templatebuilder.ComposeRequest{
			BaseTemplateID: "quickstart",
			RegistryURL:    "HTTPS://mirror.internal.example",
		})
		require.NoError(t, err)
		mainTF := string(res.MainTF)
		require.Contains(t, mainTF, "mirror.internal.example/coder/git-clone/coder")
		require.NotContains(t, mainTF, "HTTPS://")
	})

	t.Run("RejectsCredentials", func(t *testing.T) {
		t.Parallel()
		// Userinfo must be rejected, not silently stripped: it would otherwise
		// render the deployment's mirror credential into the returned main.tf.
		_, err := templatebuilder.Compose(templatebuilder.ComposeRequest{
			BaseTemplateID: "quickstart",
			RegistryURL:    "https://user:s3cr3t-token@mirror.example.com",
		})
		require.ErrorContains(t, err, "bare host")
		// The rejection error must not echo the credential it rejected.
		require.NotContains(t, err.Error(), "s3cr3t-token")
	})

	t.Run("RejectsInterpolation", func(t *testing.T) {
		t.Parallel()
		_, err := templatebuilder.Compose(templatebuilder.ComposeRequest{
			BaseTemplateID: "quickstart",
			RegistryURL:    "mirror.example.com/${var.evil}",
		})
		require.ErrorContains(t, err, "bare host")
	})

	t.Run("RejectsBackslash", func(t *testing.T) {
		t.Parallel()
		_, err := templatebuilder.Compose(templatebuilder.ComposeRequest{
			BaseTemplateID: "quickstart",
			RegistryURL:    `mirror.example.com\x`,
		})
		require.ErrorContains(t, err, "bare host")
	})

	t.Run("RejectsDoubledScheme", func(t *testing.T) {
		t.Parallel()
		_, err := templatebuilder.Compose(templatebuilder.ComposeRequest{
			BaseTemplateID: "quickstart",
			RegistryURL:    "https://https://mirror.example.com",
		})
		require.ErrorContains(t, err, "bare host")
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

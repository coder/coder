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
	"github.com/coder/coder/v2/testutil"
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

// TestQuickstartToolchainsPersistUnderHome guards the every-start reinstall fix:
// Go, Node.js, and Java must install under $HOME (the persistent home volume)
// and reach every session, so a restart reuses them instead of re-fetching from
// the network. The assertions encode those invariants rather than a blocklist of
// yesterday's strings, so reintroducing the regression another way still fails.
func TestQuickstartToolchainsPersistUnderHome(t *testing.T) {
	t.Parallel()

	fsys, err := templatebuilder.BaseTemplateFS("quickstart")
	require.NoError(t, err)
	raw, err := fs.ReadFile(fsys, "install-languages.sh.tftpl")
	require.NoError(t, err)
	script := string(raw)

	// The persistent root must be under $HOME, so moving it off the home volume
	// (e.g. LOCAL_PREFIX="/opt/toolchains") fails here instead of slipping past a
	// blocklist that only named the old system paths.
	require.Contains(t, script, `LOCAL_PREFIX="$HOME/.local"`,
		"network toolchains must root under $HOME so they persist across restarts")

	// Each network toolchain must define its dir under $LOCAL_PREFIX and install
	// into it via install_tarball. Asserting the install_tarball call (not the
	// absence of apt) fails a swap to `apt-get install` even with flags the old
	// blocklist never enumerated.
	for _, tc := range []struct {
		name   string
		dirDef string
	}{
		{"NODE_DIR", `NODE_DIR="$LOCAL_PREFIX/node"`},
		{"GO_DIR", `GO_DIR="$LOCAL_PREFIX/go"`},
		{"JAVA_DIR", `JAVA_DIR="$LOCAL_PREFIX/java"`},
	} {
		require.Contains(t, script, tc.dirDef,
			"%s must be defined under $LOCAL_PREFIX", tc.name)
		require.Regexp(t, `install_tarball\s+"[^"]+"\s+"\$`+tc.name+`"`, script,
			"%s toolchain must install via install_tarball into $%s", tc.name, tc.name)
	}

	// PATH must reach every session, including the non-login, non-interactive
	// shells coder ssh and remote IDEs use. The durable mechanism is the
	// coder_agent env (see main.tf). Parse the PATH value and assert each bin dir
	// is a real ":"-separated element, so dropping one while mentioning it in a
	// nearby comment (which a substring match would accept) fails.
	rc := testRenderContext("quickstart")
	mainTF, err := templatebuilder.RenderBaseTemplate("quickstart", "main.tf.tmpl", rc)
	require.NoError(t, err)
	pathElems := strings.Split(extractAgentEnvPATH(t, string(mainTF)), ":")
	require.Equal(t, "$PATH", pathElems[0], "coder_agent env PATH must prepend the existing $PATH")
	for _, binDir := range []string{
		"$HOME/.local/bin",      // pip install --user
		"$HOME/.cargo/bin",      // rustup
		"$HOME/.local/go/bin",   // Go
		"$HOME/go/bin",          // go install targets
		"$HOME/.local/node/bin", // Node.js
		"$HOME/.local/java/bin", // Java
	} {
		require.Contains(t, pathElems, binDir,
			"coder_agent env PATH must include %s as a path element", binDir)
	}

	// JAVA_HOME goes in the same durable env, conditional on Java so it never
	// points at a missing dir, with a ~/.profile export as the login-shell
	// fallback.
	require.Regexp(t, `contains\(local\.languages, "java"\)\s*\?\s*\{[^}]*JAVA_HOME\s*=\s*"\$HOME/\.local/java"`, string(mainTF),
		"JAVA_HOME must be set in the agent env, conditional on Java being selected")
	require.Contains(t, script, `export JAVA_HOME=$HOME/.local/java`,
		"the install script should also persist JAVA_HOME to ~/.profile for login shells")

	// The install script keeps a ~/.profile fallback and must not rely on
	// ~/.bashrc, which returns early in non-interactive shells.
	require.Contains(t, script, `PROFILE="$HOME/.profile"`,
		"the install script should persist to ~/.profile as a login-shell fallback")
	require.NotContains(t, script, ".bashrc",
		"~/.bashrc returns early in non-interactive shells; use ~/.profile plus the agent env")
}

// extractAgentEnvPATH returns the value of the PATH assignment in the rendered
// coder_agent env block.
func extractAgentEnvPATH(t *testing.T, mainTF string) string {
	t.Helper()
	m := regexp.MustCompile(`(?m)^\s*PATH\s*=\s*"([^"]*)"`).FindStringSubmatch(mainTF)
	require.Len(t, m, 2, "expected a PATH assignment in the rendered coder_agent env")
	return m[1]
}

// unescapedInterpolationPattern matches a Terraform ${name} interpolation that
// is not escaped as $${name}; the char before ${ is captured and must not be $.
var unescapedInterpolationPattern = regexp.MustCompile(`(^|[^$])\$\{([A-Za-z0-9_]+)\}`)

// TestQuickstartInstallScriptRendersValidBash gives the quickstart install
// template the mechanical coverage its .sh.tftpl extension hides from
// lint/shellcheck and fmt/shfmt (both match only *.sh): it verifies the
// Terraform escaping, then renders the script and checks it with `bash -n` and
// shellcheck, guarding the whole file including the PATH and atomic-install
// logic other tests only assert on as text.
func TestQuickstartInstallScriptRendersValidBash(t *testing.T) {
	t.Parallel()

	fsys, err := templatebuilder.BaseTemplateFS("quickstart")
	require.NoError(t, err)
	raw, err := fs.ReadFile(fsys, "install-languages.sh.tftpl")
	require.NoError(t, err)

	// Verify the escaping on the raw template rather than emulating it: the only
	// interpolation this file passes to templatefile() is ${LANGUAGES}, so every
	// other ${...} must be escaped as $${...}. An unescaped ${FOO} renders fine
	// but fails templatefile() at every workspace build, which bash -n cannot see.
	for _, m := range unescapedInterpolationPattern.FindAllStringSubmatch(string(raw), -1) {
		require.Equalf(t, "LANGUAGES", m[2],
			"unescaped Terraform interpolation ${%s}; write shell brace expansions as $${%s}", m[2], m[2])
	}

	if runtime.GOOS == "windows" {
		t.Skip("bash and shellcheck are not available on Windows runners")
	}

	// Emulate Terraform templatefile(): substitute ${LANGUAGES} with every
	// selectable language so all install branches are present, then unescape
	// $${ to ${.
	rendered := strings.ReplaceAll(string(raw), "${LANGUAGES}", "python,nodejs,go,rust,java,cpp")
	rendered = strings.ReplaceAll(rendered, "$${", "${")

	t.Run("bash -n", func(t *testing.T) {
		t.Parallel()
		bashPath, err := exec.LookPath("bash")
		if err != nil {
			t.Skip("bash not found in PATH")
		}
		cmd := exec.CommandContext(t.Context(), bashPath, "-n")
		cmd.Stdin = strings.NewReader(rendered)
		out, err := cmd.CombinedOutput()
		require.NoErrorf(t, err, "rendered install script failed `bash -n`:\n%s", out)
	})

	t.Run("shellcheck", func(t *testing.T) {
		t.Parallel()
		shellcheckPath, err := exec.LookPath("shellcheck")
		if err != nil {
			// shellcheck is pre-installed on the Linux CI runners (the lint job's
			// lint/shellcheck relies on the same ambient binary), so require it
			// there to keep this coverage from silently vanishing. It is not
			// guaranteed on the macOS runner image, so skip rather than fail when
			// it is absent off Linux.
			if testutil.InCI() && runtime.GOOS == "linux" {
				t.Fatal("shellcheck must be installed on Linux CI so this coverage cannot silently vanish")
			}
			t.Skip("shellcheck not found in PATH")
		}
		cmd := exec.CommandContext(t.Context(), shellcheckPath, "-s", "bash", "-")
		cmd.Stdin = strings.NewReader(rendered)
		out, err := cmd.CombinedOutput()
		if err == nil {
			return
		}
		// Distinguish a lint finding (exit 1) from a tool or launch failure (any
		// other exit code), so a broken shellcheck invocation is not misreported
		// as a defect in the script.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			t.Fatalf("rendered install script failed shellcheck:\n%s", out)
		}
		t.Fatalf("could not run shellcheck (%v):\n%s", err, out)
	})
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

package terraform

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/provisionersdk/tfpath"
)

const testLockFile = `# This file is maintained automatically by "terraform init".
# Manual edits may be lost in future updates.

provider "registry.terraform.io/coder/coder" {
  version     = "2.18.0"
  constraints = "2.18.0"
  hashes = [
    "h1:abc=",
    "zh:def",
  ]
}

provider "registry.terraform.io/kreuzwerker/docker" {
  version = "4.6.0"
  hashes = [
    "h1:ghi=",
  ]
}
`

func Test_parseLockFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, ".terraform.lock.hcl")
	require.NoError(t, os.WriteFile(path, []byte(testLockFile), 0o600))

	providers, err := parseLockFile(path)
	require.NoError(t, err)
	got := make([]string, 0, len(providers))
	for _, p := range providers {
		got = append(got, p.Source+"@"+p.Version)
	}
	require.Equal(t, []string{
		"registry.terraform.io/coder/coder@2.18.0",
		"registry.terraform.io/kreuzwerker/docker@4.6.0",
	}, got)

	entry := func(source, version, hashes string) string {
		return fmt.Sprintf("provider %q {\n  version = %q\n  hashes = [%s]\n}\n", source, version, hashes)
	}
	const h1 = `"h1:abc="`
	for name, content := range map[string]string{
		"SourceTraversal":  entry("registry.terraform.io/../x", "1.0.0", h1),
		"SourceTooShort":   entry("coder/coder", "1.0.0", h1),
		"SourceTooLong":    entry("registry.terraform.io/a/b/c", "1.0.0", h1),
		"EmptyVersion":     entry("registry.terraform.io/coder/coder", "", h1),
		"VersionSeparator": entry("registry.terraform.io/coder/coder", "1.0/x", h1),
		"VersionTraversal": entry("registry.terraform.io/coder/coder", "..", h1),
		"NoHashes":         "provider \"registry.terraform.io/coder/coder\" {\n  version = \"1.0.0\"\n}\n",
		"OnlyZipHashes":    entry("registry.terraform.io/coder/coder", "1.0.0", `"zh:abc"`),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			bad := filepath.Join(t.TempDir(), ".terraform.lock.hcl")
			require.NoError(t, os.WriteFile(bad, []byte(content), 0o600))
			_, err := parseLockFile(bad)
			require.Error(t, err)
		})
	}
}

// fakeCache creates a plugin cache directory holding the given providers in
// the layout Terraform uses.
func fakeCache(t *testing.T, providers ...lockedProvider) string {
	t.Helper()
	cache := t.TempDir()
	for _, p := range providers {
		dir := providerPackageDir(cache, p)
		require.NoError(t, os.MkdirAll(dir, 0o755))
		//nolint:gosec // G306: a fake provider binary needs the executable bit, like a real one.
		require.NoError(t, os.WriteFile(filepath.Join(dir, "terraform-provider-x_v"+p.Version), []byte("#!/bin/sh\n"), 0o755))
	}
	return cache
}

func Test_linkCachedProviders(t *testing.T) {
	t.Parallel()

	coder := lockedProvider{Source: "registry.terraform.io/coder/coder", Version: "2.18.0"}
	docker := lockedProvider{Source: "registry.terraform.io/kreuzwerker/docker", Version: "4.6.0"}

	t.Run("AllCached", func(t *testing.T) {
		t.Parallel()
		cache := fakeCache(t, coder, docker)
		files := tfpath.Layout(t.TempDir())

		reason, err := linkCachedProviders(files, cache, []lockedProvider{coder, docker})
		require.NoError(t, err)
		require.Empty(t, reason)

		for _, p := range []lockedProvider{coder, docker} {
			target, err := os.Readlink(providerPackageDir(files.ProvidersDirectory(), p))
			require.NoError(t, err)
			require.Equal(t, providerPackageDir(cache, p), target)
		}
	})

	// The template's source archive can put a package where the link goes.
	t.Run("PackageInPlace", func(t *testing.T) {
		t.Parallel()
		cache := fakeCache(t, coder)
		files := tfpath.Layout(t.TempDir())
		planted := filepath.Join(providerPackageDir(files.ProvidersDirectory(), coder), "terraform-provider-coder")
		require.NoError(t, os.MkdirAll(filepath.Dir(planted), 0o755))
		require.NoError(t, os.WriteFile(planted, nil, 0o600))

		reason, err := linkCachedProviders(files, cache, []lockedProvider{coder})
		require.NoError(t, err)
		require.Contains(t, reason, "already holds provider registry.terraform.io/coder/coder@2.18.0")
		require.FileExists(t, planted)
	})

	t.Run("MissingProvider", func(t *testing.T) {
		t.Parallel()
		cache := fakeCache(t, coder)
		files := tfpath.Layout(t.TempDir())

		reason, err := linkCachedProviders(files, cache, []lockedProvider{coder, docker})
		require.NoError(t, err)
		require.Contains(t, reason, "registry.terraform.io/kreuzwerker/docker@4.6.0 is not in the plugin cache")
		_, err = os.Stat(files.TerraformMetadataDir())
		require.ErrorIs(t, err, os.ErrNotExist, "nothing should be linked when a provider is missing")
	})

	t.Run("EmptyProviderDir", func(t *testing.T) {
		t.Parallel()
		cache := t.TempDir()
		require.NoError(t, os.MkdirAll(providerPackageDir(cache, coder), 0o755))
		files := tfpath.Layout(t.TempDir())

		reason, err := linkCachedProviders(files, cache, []lockedProvider{coder})
		require.NoError(t, err)
		require.NotEmpty(t, reason, "a provider directory without a binary is not usable")
	})

	t.Run("NoCache", func(t *testing.T) {
		t.Parallel()
		reason, err := linkCachedProviders(tfpath.Layout(t.TempDir()), "", []lockedProvider{coder})
		require.NoError(t, err)
		require.NotEmpty(t, reason)
	})
}

func Test_missingModules(t *testing.T) {
	t.Parallel()

	t.Run("NoModuleCalls", func(t *testing.T) {
		t.Parallel()
		files := tfpath.Layout(t.TempDir())
		require.NoError(t, os.WriteFile(filepath.Join(files.WorkDirectory(), "main.tf"), []byte(`resource "null_resource" "a" {}`), 0o600))
		reason, err := missingModules(files)
		require.NoError(t, err)
		require.Empty(t, reason)
	})

	t.Run("ModuleCallWithoutManifest", func(t *testing.T) {
		t.Parallel()
		files := tfpath.Layout(t.TempDir())
		require.NoError(t, os.WriteFile(filepath.Join(files.WorkDirectory(), "main.tf"), []byte(`module "m" { source = "./m" }`), 0o600))
		reason, err := missingModules(files)
		require.NoError(t, err)
		require.Contains(t, reason, "no saved module manifest")
	})

	withManifest := func(t *testing.T) tfpath.Layout {
		t.Helper()
		files := tfpath.Layout(t.TempDir())
		require.NoError(t, os.WriteFile(filepath.Join(files.WorkDirectory(), "main.tf"),
			[]byte(`module "local" { source = "./local" }
module "remote" { source = "example.com/x/y/z" }`), 0o600))
		require.NoError(t, os.MkdirAll(files.ModulesDirectory(), 0o755))
		require.NoError(t, os.WriteFile(files.ModulesFilePath(), []byte(`{"Modules":[
			{"Key":"","Source":"","Dir":"."},
			{"Key":"local","Source":"./local","Dir":"local"},
			{"Key":"remote","Source":"example.com/x/y/z","Dir":".terraform/modules/remote"}
		]}`), 0o600))
		require.NoError(t, os.MkdirAll(filepath.Join(files.WorkDirectory(), "local"), 0o755))
		return files
	}

	t.Run("ManifestWithAllModules", func(t *testing.T) {
		t.Parallel()
		files := withManifest(t)
		require.NoError(t, os.MkdirAll(filepath.Join(files.ModulesDirectory(), "remote"), 0o755))
		reason, err := missingModules(files)
		require.NoError(t, err)
		require.Empty(t, reason)
	})

	// The module archive keeps the manifest when it leaves out modules over
	// its size limit, so the manifest alone does not prove they are there.
	t.Run("ManifestWithMissingModule", func(t *testing.T) {
		t.Parallel()
		files := withManifest(t)
		reason, err := missingModules(files)
		require.NoError(t, err)
		require.Contains(t, reason, "module remote is not in the working directory")
	})
}

func Test_manualInit(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("init is only skipped on Linux, where the plugin cache is used")
	}
	logger := slogtest.Make(t, nil)
	coder := lockedProvider{Source: "registry.terraform.io/coder/coder", Version: "2.18.0"}
	docker := lockedProvider{Source: "registry.terraform.io/kreuzwerker/docker", Version: "4.6.0"}

	newWorkDir := func(t *testing.T) tfpath.Layout {
		files := tfpath.Layout(t.TempDir())
		require.NoError(t, os.WriteFile(filepath.Join(files.WorkDirectory(), "main.tf"), []byte(`resource "coder_agent" "a" {}`), 0o600))
		require.NoError(t, os.WriteFile(files.TerraformLockFile(), []byte(testLockFile), 0o600))
		return files
	}

	t.Run("Skips", func(t *testing.T) {
		t.Parallel()
		files := newWorkDir(t)
		require.True(t, manualInit(t.Context(), logger, files, fakeCache(t, coder, docker), nil))
		require.DirExists(t, files.ProvidersDirectory())
	})

	// Only `terraform init` can set up a backend or HCP Terraform.
	for name, settings := range map[string]string{"Backend": `backend "local" {}`, "Cloud": `cloud {}`} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			files := newWorkDir(t)
			config := "terraform {\n" + settings + "\n}\n"
			require.NoError(t, os.WriteFile(filepath.Join(files.WorkDirectory(), "backend.tf"), []byte(config), 0o600))
			require.False(t, manualInit(t.Context(), logger, files, fakeCache(t, coder, docker), nil))
			require.NoDirExists(t, files.ProvidersDirectory())
		})
	}

	// With TF_DATA_DIR, Terraform doesn't look for providers in .terraform.
	t.Run("CustomizedInit", func(t *testing.T) {
		t.Parallel()
		files := newWorkDir(t)
		env := []string{"PATH=/usr/bin", "TF_DATA_DIR=.tfdata"}
		require.False(t, manualInit(t.Context(), logger, files, fakeCache(t, coder, docker), env))
		require.NoDirExists(t, files.ProvidersDirectory())
	})
}

// Test_initDiagnosticsSink logs a plan's output the way the executor does.
func Test_initDiagnosticsSink(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		summary  string
		detail   string
		wantHeld bool
	}{
		{"PluginsNotInstalled", "Required plugins are not installed", `Run "terraform init".`, true},
		// Terraform appends advice to this summary.
		{"BackendNotInitialized", `Backend initialization required, please run "terraform init"`, "Reason: Initial configuration of the requested backend.", true},
		{"UnrelatedError", "Unsupported attribute", `This object has no argument named "x".`, false},
		// A diagnostic's detail can quote the template.
		{"TemplateTextInDetail", "Resource precondition failed", "Not ready.\nError: Module not installed", false},
		// Only the state needs this provider, and the init that Init runs
		// never sees the state.
		{"StateOnlyProvider", "Missing required provider", "This state requires provider registry.terraform.io/hashicorp/null.", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			diagnostic, err := json.Marshal(map[string]any{
				"@level": "error", "@message": "Error: " + test.summary, "type": "diagnostic",
				"diagnostic": map[string]string{"severity": "error", "summary": test.summary, "detail": test.detail},
			})
			require.NoError(t, err)
			diagnosticLines := append([]string{"Error: " + test.summary}, strings.Split(test.detail, "\n")...)

			inner := &mockLogger{}
			sink := &initDiagnosticsSink{logSink: inner}
			e := &executor{logger: slogtest.Make(t, nil), timings: newTimingAggregator(database.ProvisionerJobTimingStagePlan)}
			writer, done := e.provisionLogWriter(sink)
			_, err = writer.Write(append(diagnostic, '\n'))
			require.NoError(t, err)
			require.NoError(t, writer.Close())
			<-done
			// Error lines outside a diagnostic, such as Terraform's stderr,
			// are never held.
			sink.ProvisionLog(proto.LogLevel_ERROR, "stderr")

			forwarded := func() []string {
				lines := make([]string, 0, len(inner.logs))
				for _, log := range inner.logs {
					require.Equal(t, proto.LogLevel_ERROR, log.Level)
					lines = append(lines, log.Output)
				}
				return lines
			}
			if !test.wantHeld {
				require.Empty(t, sink.heldLines())
				require.Equal(t, append(diagnosticLines, "stderr"), forwarded())
				return
			}
			require.Equal(t, diagnosticLines, sink.heldLines())
			require.Equal(t, []string{"stderr"}, forwarded())

			sink.release()
			sink.release()
			require.Equal(t, append([]string{"stderr"}, diagnosticLines...), forwarded(), "held lines are forwarded once")
		})
	}
}

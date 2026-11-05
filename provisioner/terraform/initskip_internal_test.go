package terraform

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
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

	// Sources and versions become path components when providers are
	// linked, so anything that could escape the cache is rejected.
	for name, entry := range map[string]string{
		"SourceTraversal":  `provider "registry.terraform.io/../x" { version = "1.0.0" }`,
		"SourceTooShort":   `provider "coder/coder" { version = "1.0.0" }`,
		"SourceTooLong":    `provider "registry.terraform.io/a/b/c" { version = "1.0.0" }`,
		"EmptyVersion":     `provider "registry.terraform.io/coder/coder" { version = "" }`,
		"VersionSeparator": `provider "registry.terraform.io/coder/coder" { version = "1.0/../../x" }`,
		"VersionTraversal": `provider "registry.terraform.io/coder/coder" { version = ".." }`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			bad := filepath.Join(t.TempDir(), ".terraform.lock.hcl")
			require.NoError(t, os.WriteFile(bad, []byte(entry), 0o600))
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

		linked, err := linkCachedProviders(files, cache, []lockedProvider{coder, docker})
		require.NoError(t, err)
		require.True(t, linked)

		for _, p := range []lockedProvider{coder, docker} {
			target, err := os.Readlink(providerPackageDir(files.ProvidersDirectory(), p))
			require.NoError(t, err)
			require.Equal(t, providerPackageDir(cache, p), target)
		}

		// An existing provider entry is left in place rather than treated as
		// an error. Plan checks whatever ends up linked against the lock
		// file's hashes.
		linked, err = linkCachedProviders(files, cache, []lockedProvider{coder, docker})
		require.NoError(t, err)
		require.True(t, linked)
	})

	t.Run("MissingProvider", func(t *testing.T) {
		t.Parallel()
		cache := fakeCache(t, coder)
		files := tfpath.Layout(t.TempDir())

		linked, err := linkCachedProviders(files, cache, []lockedProvider{coder, docker})
		require.NoError(t, err)
		require.False(t, linked)
		_, err = os.Stat(files.TerraformMetadataDir())
		require.ErrorIs(t, err, os.ErrNotExist, "nothing should be linked when a provider is missing")
	})

	t.Run("EmptyProviderDir", func(t *testing.T) {
		t.Parallel()
		cache := t.TempDir()
		require.NoError(t, os.MkdirAll(providerPackageDir(cache, coder), 0o755))
		files := tfpath.Layout(t.TempDir())

		linked, err := linkCachedProviders(files, cache, []lockedProvider{coder})
		require.NoError(t, err)
		require.False(t, linked, "a provider directory without a binary is not usable")
	})

	t.Run("NoCache", func(t *testing.T) {
		t.Parallel()
		linked, err := linkCachedProviders(tfpath.Layout(t.TempDir()), "", []lockedProvider{coder})
		require.NoError(t, err)
		require.False(t, linked)
	})
}

func Test_modulesReady(t *testing.T) {
	t.Parallel()

	t.Run("NoModuleCalls", func(t *testing.T) {
		t.Parallel()
		files := tfpath.Layout(t.TempDir())
		require.NoError(t, os.WriteFile(filepath.Join(files.WorkDirectory(), "main.tf"), []byte(`resource "null_resource" "a" {}`), 0o600))
		ready, err := modulesReady(files)
		require.NoError(t, err)
		require.True(t, ready)
	})

	t.Run("ModuleCallWithoutManifest", func(t *testing.T) {
		t.Parallel()
		files := tfpath.Layout(t.TempDir())
		require.NoError(t, os.WriteFile(filepath.Join(files.WorkDirectory(), "main.tf"), []byte(`module "m" { source = "./m" }`), 0o600))
		ready, err := modulesReady(files)
		require.NoError(t, err)
		require.False(t, ready)
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
		ready, err := modulesReady(files)
		require.NoError(t, err)
		require.True(t, ready)
	})

	// The module archive keeps the manifest when it leaves out modules over
	// its size limit, so the manifest alone does not prove they are there.
	t.Run("ManifestWithMissingModule", func(t *testing.T) {
		t.Parallel()
		files := withManifest(t)
		ready, err := modulesReady(files)
		require.NoError(t, err)
		require.False(t, ready)
	})
}

func Test_prepareWithoutInit(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("init is only skipped on Linux, where the plugin cache is used")
	}
	logger := slogtest.Make(t, nil)
	coder := lockedProvider{Source: "registry.terraform.io/coder/coder", Version: "2.18.0"}
	docker := lockedProvider{Source: "registry.terraform.io/kreuzwerker/docker", Version: "4.6.0"}

	newWorkDir := func(t *testing.T, withLock bool) tfpath.Layout {
		files := tfpath.Layout(t.TempDir())
		require.NoError(t, os.WriteFile(filepath.Join(files.WorkDirectory(), "main.tf"), []byte(`resource "coder_agent" "a" {}`), 0o600))
		if withLock {
			require.NoError(t, os.WriteFile(files.TerraformLockFile(), []byte(testLockFile), 0o600))
		}
		return files
	}

	t.Run("Skips", func(t *testing.T) {
		t.Parallel()
		files := newWorkDir(t, true)
		require.True(t, prepareWithoutInit(context.Background(), logger, files, fakeCache(t, coder, docker)))
		_, err := os.Stat(files.ProvidersDirectory())
		require.NoError(t, err)
	})

	t.Run("NoLockFile", func(t *testing.T) {
		t.Parallel()
		files := newWorkDir(t, false)
		require.False(t, prepareWithoutInit(context.Background(), logger, files, fakeCache(t, coder, docker)))
	})

	t.Run("ProviderNotCached", func(t *testing.T) {
		t.Parallel()
		files := newWorkDir(t, true)
		require.False(t, prepareWithoutInit(context.Background(), logger, files, fakeCache(t, coder)))
	})

	t.Run("NoCachePath", func(t *testing.T) {
		t.Parallel()
		files := newWorkDir(t, true)
		require.False(t, prepareWithoutInit(context.Background(), logger, files, ""))
	})
}

func Test_errorCapturingSink(t *testing.T) {
	t.Parallel()

	t.Run("NeedsInit", func(t *testing.T) {
		t.Parallel()
		// Diagnostics as Terraform reports them.
		for line, want := range map[string]bool{
			"Error: Required plugins are not installed":                           true,
			"Error: Inconsistent dependency lock file":                            true,
			"Error: Module not installed":                                         true,
			"Error: Module source has changed":                                    true,
			`Error: Backend initialization required, please run "terraform init"`: true,
			"the cached package for registry.terraform.io/coder/coder 2.18.0 (in .terraform/providers) does not match any of the checksums recorded in the dependency lock file": true,
			"Error: Unsupported attribute": false,
			"Error: Invalid reference":     false,
		} {
			sink := &errorCapturingSink{logSink: &mockLogger{}}
			sink.ProvisionLog(proto.LogLevel_ERROR, line)
			require.Equal(t, want, sink.needsInit(), line)
		}
	})

	t.Run("OnlyErrorsCount", func(t *testing.T) {
		t.Parallel()
		inner := &mockLogger{}
		sink := &errorCapturingSink{logSink: inner}
		sink.ProvisionLog(proto.LogLevel_INFO, "Required plugins are not installed")
		require.False(t, sink.needsInit())
		sink.ProvisionLog(proto.LogLevel_ERROR, "Error: Unsupported attribute")
		require.Len(t, inner.logs, 2, "every line is forwarded")
	})

	t.Run("HoldErrors", func(t *testing.T) {
		t.Parallel()
		inner := &mockLogger{}
		sink := &errorCapturingSink{logSink: inner, holdErrors: true}
		sink.ProvisionLog(proto.LogLevel_INFO, "Planning...")
		sink.ProvisionLog(proto.LogLevel_ERROR, "Error: Unsupported attribute")
		require.Len(t, inner.logs, 1, "error lines are held back")
		require.Equal(t, []string{"Error: Unsupported attribute"}, sink.capturedErrors())

		sink.flushHeldErrors()
		require.Len(t, inner.logs, 2)
		require.Equal(t, proto.LogLevel_ERROR, inner.logs[1].Level)
		require.Equal(t, "Error: Unsupported attribute", inner.logs[1].Output)

		sink.flushHeldErrors()
		require.Len(t, inner.logs, 2, "held errors are forwarded once")
	})
}

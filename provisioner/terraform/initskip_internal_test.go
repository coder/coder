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
	require.Equal(t, []lockedProvider{
		{Source: "registry.terraform.io/coder/coder", Version: "2.18.0", Remain: providers[0].Remain},
		{Source: "registry.terraform.io/kreuzwerker/docker", Version: "4.6.0", Remain: providers[1].Remain},
	}, providers)

	t.Run("RejectsPathTraversal", func(t *testing.T) {
		t.Parallel()
		bad := filepath.Join(t.TempDir(), ".terraform.lock.hcl")
		require.NoError(t, os.WriteFile(bad, []byte(`provider "registry.terraform.io/../x" {
  version = "1.0.0"
}`), 0o600))
		_, err := parseLockFile(bad)
		require.Error(t, err)
	})
}

// fakeCache creates a plugin cache directory holding the given providers in
// the layout Terraform uses.
func fakeCache(t *testing.T, providers ...lockedProvider) string {
	t.Helper()
	cache := t.TempDir()
	for _, p := range providers {
		dir := cachedProviderDir(cache, p)
		require.NoError(t, os.MkdirAll(dir, 0o755))
		// The plugin binary must be executable, like a real provider.
		//nolint:gosec
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
			link := filepath.Join(files.TerraformMetadataDir(), "providers", filepath.FromSlash(p.Source), p.Version, runtime.GOOS+"_"+runtime.GOARCH)
			target, err := os.Readlink(link)
			require.NoError(t, err)
			require.Equal(t, cachedProviderDir(cache, p), target)
		}

		// Linking again must be a no-op rather than an error, so a retried
		// Init in the same working directory keeps working.
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
		require.NoError(t, os.MkdirAll(cachedProviderDir(cache, coder), 0o755))
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

	t.Run("ModuleCallWithManifest", func(t *testing.T) {
		t.Parallel()
		files := tfpath.Layout(t.TempDir())
		require.NoError(t, os.WriteFile(filepath.Join(files.WorkDirectory(), "main.tf"), []byte(`module "m" { source = "./m" }`), 0o600))
		require.NoError(t, os.MkdirAll(files.ModulesDirectory(), 0o755))
		require.NoError(t, os.WriteFile(files.ModulesFilePath(), []byte(`{"Modules":[]}`), 0o600))
		ready, err := modulesReady(files)
		require.NoError(t, err)
		require.True(t, ready)
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
		_, err := os.Stat(filepath.Join(files.TerraformMetadataDir(), "providers"))
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

type recordingSink struct {
	lines []string
}

func (r *recordingSink) ProvisionLog(_ proto.LogLevel, output string) {
	r.lines = append(r.lines, output)
}

func Test_errorCapturingSink(t *testing.T) {
	t.Parallel()

	inner := &recordingSink{}
	sink := &errorCapturingSink{logSink: inner}
	sink.ProvisionLog(proto.LogLevel_INFO, "Required plugins are not installed")
	require.False(t, sink.needsInit(), "only error level output counts")

	sink.ProvisionLog(proto.LogLevel_ERROR, "Error: Unsupported attribute")
	require.False(t, sink.needsInit())

	sink.ProvisionLog(proto.LogLevel_ERROR, "Error: Inconsistent dependency lock file")
	require.True(t, sink.needsInit())
	require.Len(t, inner.lines, 3, "every line is forwarded")
}

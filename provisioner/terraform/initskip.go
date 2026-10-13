package terraform

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/terraform-config-inspect/tfconfig"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/provisionersdk/tfpath"
)

// lockedProvider is one provider entry from a .terraform.lock.hcl file.
type lockedProvider struct {
	// Source is the fully qualified provider address, for example
	// registry.terraform.io/coder/coder.
	Source  string `hcl:"source,label"`
	Version string `hcl:"version"`
	// Remain absorbs the constraints and hashes attributes, which we
	// don't need.
	Remain hcl.Body `hcl:",remain"`
}

type lockFile struct {
	Providers []lockedProvider `hcl:"provider,block"`
	Remain    hcl.Body         `hcl:",remain"`
}

// parseLockFile returns the providers pinned by a Terraform dependency lock
// file.
func parseLockFile(path string) ([]lockedProvider, error) {
	parser := hclparse.NewParser()
	file, diags := parser.ParseHCLFile(path)
	if diags.HasErrors() {
		return nil, xerrors.Errorf("parse %s: %w", path, diags)
	}
	var lock lockFile
	diags = gohcl.DecodeBody(file.Body, nil, &lock)
	if diags.HasErrors() {
		return nil, xerrors.Errorf("decode %s: %w", path, diags)
	}
	for _, p := range lock.Providers {
		if strings.Count(p.Source, "/") != 2 || strings.Contains(p.Source, "..") || p.Version == "" || strings.Contains(p.Version, "/") || strings.Contains(p.Version, "..") {
			return nil, xerrors.Errorf("unexpected provider entry %q version %q in %s", p.Source, p.Version, path)
		}
	}
	return lock.Providers, nil
}

// cachedProviderDir returns the directory inside the plugin cache that
// Terraform uses for a provider package, which mirrors the layout of the
// .terraform/providers directory that terraform init would create.
func cachedProviderDir(cachePath string, p lockedProvider) string {
	return filepath.Join(cachePath, filepath.FromSlash(p.Source), p.Version, runtime.GOOS+"_"+runtime.GOARCH)
}

// linkCachedProviders reproduces the part of `terraform init` that matters
// for a workspace build when nothing needs to be downloaded: it links every
// provider pinned by the lock file from the plugin cache into
// .terraform/providers. It reports false without touching the working
// directory when any pinned provider is missing from the cache, in which
// case the caller must run a real init.
//
// This mirrors Terraform's own behavior with TF_PLUGIN_CACHE_DIR, which
// creates an absolute symlink from the working directory into the cache.
func linkCachedProviders(files tfpath.Layout, cachePath string, providers []lockedProvider) (bool, error) {
	if cachePath == "" || len(providers) == 0 {
		return false, nil
	}
	for _, p := range providers {
		dir := cachedProviderDir(cachePath, p)
		entries, err := os.ReadDir(dir)
		if err != nil {
			return false, nil
		}
		hasBinary := false
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), "terraform-provider-") {
				hasBinary = true
				break
			}
		}
		if !hasBinary {
			return false, nil
		}
	}
	for _, p := range providers {
		src := cachedProviderDir(cachePath, p)
		dst := filepath.Join(files.TerraformMetadataDir(), "providers", filepath.FromSlash(p.Source), p.Version, runtime.GOOS+"_"+runtime.GOARCH)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return false, xerrors.Errorf("create provider directory: %w", err)
		}
		if err := os.Symlink(src, dst); err != nil && !errors.Is(err, os.ErrExist) {
			return false, xerrors.Errorf("link provider %s: %w", p.Source, err)
		}
	}
	return true, nil
}

// modulesReady reports whether the working directory already contains
// everything `terraform init` would have installed for modules: either the
// module manifest from a previous init is present, or the configuration
// declares no module calls at all.
func modulesReady(files tfpath.Layout) (bool, error) {
	if _, err := os.Stat(files.ModulesFilePath()); err == nil {
		return true, nil
	}
	module, diags := tfconfig.LoadModule(files.WorkDirectory())
	if diags.HasErrors() {
		return false, xerrors.Errorf("load module: %w", diags.Err())
	}
	return len(module.ModuleCalls) == 0, nil
}

// prepareWithoutInit tries to make the working directory ready for plan and
// apply without running `terraform init`. That command costs a few hundred
// milliseconds even when it has nothing to download, because it contacts the
// registry for every pinned provider and re-verifies package checksums. When
// the dependency lock file pins every provider to a version that is already
// in the plugin cache, and modules are already present, linking the cached
// packages is equivalent.
//
// It returns true when init was skipped. Any problem is reported as false so
// the caller falls back to a real init.
func prepareWithoutInit(ctx context.Context, logger slog.Logger, files tfpath.Layout, cachePath string) bool {
	// Terraform only reliably uses the plugin cache on Linux, and the
	// cache layout we link from is platform specific.
	if cachePath == "" || runtime.GOOS != "linux" {
		return false
	}
	lockPath := files.TerraformLockFile()
	if _, err := os.Stat(lockPath); err != nil {
		return false
	}
	providers, err := parseLockFile(lockPath)
	if err != nil {
		logger.Debug(ctx, "cannot skip terraform init: unreadable lock file", slog.Error(err))
		return false
	}
	ready, err := modulesReady(files)
	if err != nil {
		logger.Debug(ctx, "cannot skip terraform init: module check failed", slog.Error(err))
		return false
	}
	if !ready {
		logger.Debug(ctx, "cannot skip terraform init: modules are not installed")
		return false
	}
	linked, err := linkCachedProviders(files, cachePath, providers)
	if err != nil {
		logger.Warn(ctx, "cannot skip terraform init: linking cached providers failed", slog.Error(err))
		// Remove any partial provider tree so init starts from a clean slate.
		_ = os.RemoveAll(filepath.Join(files.TerraformMetadataDir(), "providers"))
		return false
	}
	if !linked {
		logger.Debug(ctx, "cannot skip terraform init: a pinned provider is not cached")
		return false
	}
	names := make([]string, 0, len(providers))
	for _, p := range providers {
		names = append(names, fmt.Sprintf("%s@%s", p.Source, p.Version))
	}
	logger.Debug(ctx, "skipped terraform init: reused cached providers", slog.F("providers", names))
	return true
}

// errorCapturingSink forwards provision logs and keeps the error level ones
// so the caller can inspect why a command failed. Terraform reports missing
// providers or modules as diagnostics such as "Required plugins are not
// installed", which is how Plan decides that skipping init was wrong.
type errorCapturingSink struct {
	logSink
	mu     sync.Mutex
	errors strings.Builder
}

func (s *errorCapturingSink) ProvisionLog(level proto.LogLevel, output string) {
	if level == proto.LogLevel_ERROR {
		s.mu.Lock()
		_, _ = s.errors.WriteString(output)
		_ = s.errors.WriteByte('\n')
		s.mu.Unlock()
	}
	s.logSink.ProvisionLog(level, output)
}

// initErrorMarkers are fragments of the Terraform diagnostics that mean the
// working directory is missing something `terraform init` would install.
var initErrorMarkers = []string{
	"plugins are not installed",
	"cached in .terraform/providers",
	"dependency lock file",
	"module not installed",
	"module source has changed",
	"initialization required",
	"terraform init",
}

// needsInit reports whether any captured error indicates that the working
// directory needs `terraform init`.
func (s *errorCapturingSink) needsInit() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := strings.ToLower(s.errors.String())
	for _, marker := range initErrorMarkers {
		if strings.Contains(out, marker) {
			return true
		}
	}
	return false
}

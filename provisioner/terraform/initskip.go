package terraform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
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
		if err := validateLockedProvider(p); err != nil {
			return nil, xerrors.Errorf("%s: %w", path, err)
		}
	}
	return lock.Providers, nil
}

// validateLockedProvider rejects lock file entries whose source or version
// could escape the plugin cache or .terraform/providers, because both become
// path components when providers are linked.
func validateLockedProvider(p lockedProvider) error {
	switch {
	case strings.Count(p.Source, "/") != 2, strings.Contains(p.Source, ".."):
		return xerrors.Errorf("unexpected provider source %q", p.Source)
	case p.Version == "", strings.Contains(p.Version, "/"), strings.Contains(p.Version, ".."):
		return xerrors.Errorf("unexpected version %q for provider %q", p.Version, p.Source)
	}
	return nil
}

// providerPackageDir returns the directory of a provider package below root,
// in the layout shared by the plugin cache and the .terraform/providers
// directory that terraform init creates.
func providerPackageDir(root string, p lockedProvider) string {
	return filepath.Join(root, filepath.FromSlash(p.Source), p.Version, terraformPlatform())
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
		entries, err := os.ReadDir(providerPackageDir(cachePath, p))
		if err != nil {
			return false, nil
		}
		hasBinary := slices.ContainsFunc(entries, func(entry os.DirEntry) bool {
			return strings.HasPrefix(entry.Name(), "terraform-provider-")
		})
		if !hasBinary {
			return false, nil
		}
	}
	for _, p := range providers {
		src := providerPackageDir(cachePath, p)
		dst := providerPackageDir(files.ProvidersDirectory(), p)
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
// module manifest from a previous init is present along with every module it
// lists, or the configuration declares no module calls at all.
func modulesReady(files tfpath.Layout) (bool, error) {
	content, err := os.ReadFile(files.ModulesFilePath())
	switch {
	case err == nil:
		var manifest modulesFile
		if err := json.Unmarshal(content, &manifest); err != nil {
			return false, xerrors.Errorf("parse module manifest: %w", err)
		}
		// The module archive keeps the manifest even when it left out
		// modules that exceeded its size limit.
		for _, m := range manifest.Modules {
			if m.Key == "" {
				// The root module.
				continue
			}
			if _, err := os.Stat(filepath.Join(files.WorkDirectory(), filepath.FromSlash(m.Dir))); err != nil {
				return false, nil
			}
		}
		return true, nil
	case !errors.Is(err, fs.ErrNotExist):
		return false, xerrors.Errorf("read module manifest: %w", err)
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
// packages is equivalent for plan and apply, which still check the linked
// packages against the lock file's hashes. What is skipped is init's check
// that the configured provider sources still offer the pinned versions.
//
// The caller must only use this with a lock file that a real init produced
// on this platform, so it holds hashes this platform can verify.
//
// It returns true when init was skipped. Any problem is reported as false so
// the caller falls back to a real init.
func prepareWithoutInit(ctx context.Context, logger slog.Logger, files tfpath.Layout, cachePath string) bool {
	// Terraform only fills the plugin cache when basicEnv hands it one.
	if !pluginCacheEnabled(cachePath) {
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
		_ = os.RemoveAll(files.ProvidersDirectory())
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
	// holdErrors keeps error lines back instead of forwarding them, until
	// flushHeldErrors, so the caller can drop errors it recovers from.
	holdErrors bool

	mu     sync.Mutex
	errors []string
}

func (s *errorCapturingSink) ProvisionLog(level proto.LogLevel, output string) {
	if level == proto.LogLevel_ERROR {
		s.mu.Lock()
		s.errors = append(s.errors, output)
		s.mu.Unlock()
		if s.holdErrors {
			return
		}
	}
	s.logSink.ProvisionLog(level, output)
}

// flushHeldErrors forwards the error lines that holdErrors kept back. Call it
// only after the command has finished writing logs.
func (s *errorCapturingSink) flushHeldErrors() {
	if !s.holdErrors {
		return
	}
	s.holdErrors = false
	for _, line := range s.capturedErrors() {
		s.logSink.ProvisionLog(proto.LogLevel_ERROR, line)
	}
}

func (s *errorCapturingSink) capturedErrors() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.errors)
}

// initErrorMarkers are fragments of the Terraform diagnostics that mean the
// working directory is missing something `terraform init` would install. The
// generic "terraform init" covers diagnostics that tell the user to run it.
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
	out := strings.ToLower(strings.Join(s.capturedErrors(), "\n"))
	return slices.ContainsFunc(initErrorMarkers, func(marker string) bool {
		return strings.Contains(out, marker)
	})
}

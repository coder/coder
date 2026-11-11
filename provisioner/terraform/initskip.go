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
	tfjson "github.com/hashicorp/terraform-json"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/provisionersdk/tfpath"
)

// lockedProvider is one provider entry from a .terraform.lock.hcl file.
type lockedProvider struct {
	// Source is the fully qualified provider address, for example
	// registry.terraform.io/coder/coder.
	Source  string   `hcl:"source,label"`
	Version string   `hcl:"version"`
	Hashes  []string `hcl:"hashes,optional"`
	// Remain absorbs the constraints attribute, which we don't need.
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

// validateLockedProvider rejects lock file entries that linking can't use
// safely: a source or version that could escape the plugin cache or
// .terraform/providers, where both become path components, or no h1 hash.
// Plan loads a linked package unverified when its entry has no hashes, and
// can't check a zh hash against an unpacked package.
func validateLockedProvider(p lockedProvider) error {
	switch {
	case strings.Count(p.Source, "/") != 2, strings.Contains(p.Source, ".."):
		return xerrors.Errorf("unexpected provider source %q", p.Source)
	case p.Version == "", strings.Contains(p.Version, "/"), strings.Contains(p.Version, ".."):
		return xerrors.Errorf("unexpected version %q for provider %q", p.Version, p.Source)
	case !slices.ContainsFunc(p.Hashes, func(hash string) bool { return strings.HasPrefix(hash, "h1:") }):
		return xerrors.Errorf("no h1 hash for provider %q", p.Source)
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
// directory when any pinned provider is missing from the cache, or when the
// working directory already holds something else in a provider's place, in
// which case the caller must run a real init.
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
		// The template's source archive could have put a package there.
		target, err := os.Readlink(providerPackageDir(files.ProvidersDirectory(), p))
		switch {
		case errors.Is(err, fs.ErrNotExist):
		case err != nil, target != providerPackageDir(cachePath, p):
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

// initCustomizingEnv lists the Terraform environment variables that change
// where `terraform init` installs providers or how it runs. Linking cached
// providers into .terraform can't honor them.
var initCustomizingEnv = []string{"TF_DATA_DIR", "TF_CLI_ARGS", "TF_CLI_ARGS_init"}

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
// the caller falls back to a real init. env is the environment Terraform
// runs with.
func prepareWithoutInit(ctx context.Context, logger slog.Logger, files tfpath.Layout, cachePath string, env []string) bool {
	// Terraform only fills the plugin cache when basicEnv hands it one.
	if !pluginCacheEnabled(cachePath) {
		return false
	}
	for _, name := range initCustomizingEnv {
		if safeEnvironValue(env, name) != "" {
			logger.Debug(ctx, "cannot skip terraform init: the environment customizes it", slog.F("variable", name))
			return false
		}
	}
	providers, err := parseLockFile(files.TerraformLockFile())
	if err != nil {
		logger.Debug(ctx, "cannot skip terraform init: unusable lock file", slog.Error(err))
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
		logger.Debug(ctx, "cannot skip terraform init: a pinned provider is not cached, or the working directory already holds one")
		return false
	}
	names := make([]string, 0, len(providers))
	for _, p := range providers {
		names = append(names, fmt.Sprintf("%s@%s", p.Source, p.Version))
	}
	logger.Debug(ctx, "skipped terraform init: reused cached providers", slog.F("providers", names))
	return true
}

// needsInitSummaries start the summaries of the Terraform diagnostics that
// mean the working directory is missing something `terraform init` installs.
//
// "Missing required provider" is left out on purpose. Plan reports it for a
// provider that only the workspace's state needs. The init that Init runs
// never sees that state, so falling back for it would let a build pass or
// fail depending on whether init was skipped.
var needsInitSummaries = []string{
	"Required plugins are not installed",
	"Inconsistent dependency lock file",
	"Module not installed",
	"Module source has changed",
	"Module version requirements have changed",
	"Backend initialization required",
}

// initDiagnosticsSink forwards a plan's provision logs, except for the
// diagnostics that mean the working directory needs `terraform init`. It
// holds those back, so Plan can drop them when a real init fixes the plan
// and release them when it doesn't. Only the diagnostic's summary decides:
// its detail can quote the template, such as a precondition's error message.
type initDiagnosticsSink struct {
	logSink

	mu   sync.Mutex
	held []string
}

var _ diagnosticSink = (*initDiagnosticsSink)(nil)

func (s *initDiagnosticsSink) diagnosticLog(diag *tfjson.Diagnostic) logSink {
	needsInit := slices.ContainsFunc(needsInitSummaries, func(prefix string) bool {
		return strings.HasPrefix(diag.Summary, prefix)
	})
	if !needsInit {
		return s.logSink
	}
	return heldLogSink{s}
}

// heldLogSink holds back the lines logged to it.
type heldLogSink struct {
	sink *initDiagnosticsSink
}

func (h heldLogSink) ProvisionLog(_ proto.LogLevel, output string) {
	h.sink.mu.Lock()
	defer h.sink.mu.Unlock()
	h.sink.held = append(h.sink.held, output)
}

// heldErrors returns the lines of the diagnostics held back so far. Any line
// means the plan reported that the working directory needs init.
func (s *initDiagnosticsSink) heldErrors() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.held)
}

// release forwards the held lines. The diagnostics in needsInitSummaries are
// all errors.
func (s *initDiagnosticsSink) release() {
	s.mu.Lock()
	held := s.held
	s.held = nil
	s.mu.Unlock()
	for _, line := range held {
		s.ProvisionLog(proto.LogLevel_ERROR, line)
	}
}

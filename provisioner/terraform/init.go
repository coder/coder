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

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/terraform-config-inspect/tfconfig"
	tfjson "github.com/hashicorp/terraform-json"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/provisionersdk/tfpath"
)

// lockedProvider is a provider entry in a .terraform.lock.hcl file.
type lockedProvider struct {
	// Source is the full provider address, such as
	// registry.terraform.io/coder/coder.
	Source  string   `hcl:"source,label"`
	Version string   `hcl:"version"`
	Hashes  []string `hcl:"hashes,optional"`
	// Remain absorbs the attributes we don't read.
	Remain hcl.Body `hcl:",remain"`
}

type lockFile struct {
	Providers []lockedProvider `hcl:"provider,block"`
	Remain    hcl.Body         `hcl:",remain"`
}

// parseLockFile returns the providers a dependency lock file pins.
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

// validateLockedProvider rejects entries that manualInit can't link safely.
func validateLockedProvider(p lockedProvider) error {
	switch {
	case strings.Count(p.Source, "/") != 2, strings.Contains(p.Source, ".."):
		// The source becomes a path, so it must be exactly
		// hostname/namespace/type.
		return xerrors.Errorf("unexpected provider source %q", p.Source)
	case p.Version == "", strings.Contains(p.Version, "/"), strings.Contains(p.Version, ".."):
		// The version becomes a path component.
		return xerrors.Errorf("unexpected version %q for provider %q", p.Version, p.Source)
	case !slices.ContainsFunc(p.Hashes, func(hash string) bool { return strings.HasPrefix(hash, "h1:") }):
		// Without an h1 hash, plan can't verify the linked package.
		return xerrors.Errorf("no h1 hash for provider %q", p.Source)
	}
	return nil
}

// providerPackageDir returns where p's package lives under root. The plugin
// cache and .terraform/providers share this layout.
func providerPackageDir(root string, p lockedProvider) string {
	return filepath.Join(root, filepath.FromSlash(p.Source), p.Version, terraformPlatform())
}

// linkCachedProviders symlinks each provider from the plugin cache into
// .terraform/providers, as `terraform init` does on a cache hit. If a
// provider can't be linked, it links nothing and returns the reason.
func linkCachedProviders(files tfpath.Layout, cachePath string, providers []lockedProvider) (reason string, err error) {
	if cachePath == "" {
		return "there is no plugin cache", nil
	}
	if len(providers) == 0 {
		return "the lock file pins no providers", nil
	}
	for _, p := range providers {
		entries, err := os.ReadDir(providerPackageDir(cachePath, p))
		hasBinary := slices.ContainsFunc(entries, func(entry os.DirEntry) bool {
			return strings.HasPrefix(entry.Name(), "terraform-provider-")
		})
		if err != nil || !hasBinary {
			return fmt.Sprintf("provider %s@%s is not in the plugin cache", p.Source, p.Version), nil
		}
		// The template's source archive could have put something there.
		if _, err := os.Lstat(providerPackageDir(files.ProvidersDirectory(), p)); !errors.Is(err, fs.ErrNotExist) {
			return fmt.Sprintf("the working directory already holds provider %s@%s", p.Source, p.Version), nil
		}
	}
	for _, p := range providers {
		src := providerPackageDir(cachePath, p)
		dst := providerPackageDir(files.ProvidersDirectory(), p)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return "", xerrors.Errorf("create provider directory: %w", err)
		}
		if err := os.Symlink(src, dst); err != nil {
			return "", xerrors.Errorf("link provider %s: %w", p.Source, err)
		}
	}
	return "", nil
}

// missingModules returns why the working directory lacks a module that
// `terraform init` would install, or "" if it lacks none.
func missingModules(files tfpath.Layout) (reason string, err error) {
	content, err := os.ReadFile(files.ModulesFilePath())
	switch {
	case err == nil:
		var manifest modulesFile
		if err := json.Unmarshal(content, &manifest); err != nil {
			return "", xerrors.Errorf("parse module manifest: %w", err)
		}
		// The manifest also lists modules the archive left out for size.
		for _, m := range manifest.Modules {
			if m.Key == "" {
				// The root module.
				continue
			}
			if _, err := os.Stat(filepath.Join(files.WorkDirectory(), filepath.FromSlash(m.Dir))); err != nil {
				return fmt.Sprintf("module %s is not in the working directory", m.Key), nil
			}
		}
		return "", nil
	case !errors.Is(err, fs.ErrNotExist):
		return "", xerrors.Errorf("read module manifest: %w", err)
	}
	// Without a manifest, the template must not call any module.
	module, diags := tfconfig.LoadModule(files.WorkDirectory())
	if diags.HasErrors() {
		return "", xerrors.Errorf("load module: %w", diags.Err())
	}
	if len(module.ModuleCalls) > 0 {
		return "the template calls modules but has no saved module manifest", nil
	}
	return "", nil
}

// These schemas find the blocks of a terraform block that only `terraform
// init` can set up.
var (
	terraformBlockSchema = &hcl.BodySchema{Blocks: []hcl.BlockHeaderSchema{{Type: "terraform"}}}
	backendSchema        = &hcl.BodySchema{Blocks: []hcl.BlockHeaderSchema{
		{Type: "backend", LabelNames: []string{"type"}},
		{Type: "cloud"},
	}}
)

// declaresBackend reports whether the root module in dir configures a
// backend or HCP Terraform.
func declaresBackend(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, xerrors.Errorf("read working directory: %w", err)
	}
	parser := hclparse.NewParser()
	for _, entry := range entries {
		var (
			file  *hcl.File
			diags hcl.Diagnostics
			path  = filepath.Join(dir, entry.Name())
		)
		switch {
		case entry.IsDir():
			continue
		case strings.HasSuffix(entry.Name(), ".tf"):
			file, diags = parser.ParseHCLFile(path)
		case strings.HasSuffix(entry.Name(), ".tf.json"):
			file, diags = parser.ParseJSONFile(path)
		default:
			continue
		}
		if diags.HasErrors() {
			return false, xerrors.Errorf("parse %s: %w", entry.Name(), diags)
		}
		root, _, diags := file.Body.PartialContent(terraformBlockSchema)
		if diags.HasErrors() {
			return false, xerrors.Errorf("read %s: %w", entry.Name(), diags)
		}
		for _, block := range root.Blocks {
			settings, _, diags := block.Body.PartialContent(backendSchema)
			if diags.HasErrors() {
				return false, xerrors.Errorf("read %s: %w", entry.Name(), diags)
			}
			if len(settings.Blocks) > 0 {
				return true, nil
			}
		}
	}
	return false, nil
}

// initCustomizingEnv are the variables that change where or how `terraform
// init` installs providers. manualInit can't honor them.
var initCustomizingEnv = []string{"TF_DATA_DIR", "TF_CLI_ARGS", "TF_CLI_ARGS_init"}

// manualInit links the cached providers into the working directory itself,
// instead of running `terraform init`. This is only for speed: with every
// provider cached, init still spends 0.3s to 1.4s calling the registry and
// hashing the packages. It reports false if a real init is needed.
//
// The lock file must come from a real init on this platform, so that plan
// can verify the linked packages against its hashes. env is the environment
// Terraform runs with.
func manualInit(ctx context.Context, logger slog.Logger, files tfpath.Layout, cachePath string, env []string) bool {
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
	backend, err := declaresBackend(files.WorkDirectory())
	if err != nil {
		logger.Debug(ctx, "cannot skip terraform init: backend check failed", slog.Error(err))
		return false
	}
	if backend {
		logger.Debug(ctx, "cannot skip terraform init: the template configures a backend")
		return false
	}
	reason, err := missingModules(files)
	if err != nil {
		logger.Debug(ctx, "cannot skip terraform init: module check failed", slog.Error(err))
		return false
	}
	if reason != "" {
		logger.Debug(ctx, "cannot skip terraform init", slog.F("reason", reason))
		return false
	}
	reason, err = linkCachedProviders(files, cachePath, providers)
	if err != nil {
		logger.Warn(ctx, "cannot skip terraform init: linking cached providers failed", slog.Error(err))
		// Leave nothing half-linked for the real init.
		_ = os.RemoveAll(files.ProvidersDirectory())
		return false
	}
	if reason != "" {
		logger.Debug(ctx, "cannot skip terraform init", slog.F("reason", reason))
		return false
	}
	names := make([]string, 0, len(providers))
	for _, p := range providers {
		names = append(names, fmt.Sprintf("%s@%s", p.Source, p.Version))
	}
	logger.Debug(ctx, "skipped terraform init: reused cached providers", slog.F("providers", names))
	return true
}

// planAfterManualInit plans in a working directory that manualInit prepared.
// If Terraform reports that the directory needs `terraform init`, it runs
// init and plans once more.
func (s *server) planAfterManualInit(
	ctx, killCtx context.Context, e *executor, sess *provisionersdk.Session, sessState *sessionState,
	plan func(logSink) (*proto.PlanComplete, *tfjson.Plan, error),
) (*proto.PlanComplete, *tfjson.Plan, error) {
	logs := &initDiagnosticsSink{logSink: sess}
	resp, parsed, err := plan(logs)
	held := logs.heldLines()
	if err == nil || len(held) == 0 || ctx.Err() != nil {
		logs.release()
		return resp, parsed, err
	}

	s.logger.Warn(ctx, "plan failed after skipping terraform init, retrying with a full init",
		slog.Error(err), slog.F("plan_errors", held))
	sess.ProvisionLog(proto.LogLevel_WARN, "The cached providers and modules were not enough for this build, running terraform init")
	sessState.initSkipped = false
	// This init counts toward the plan stage: job stats can't interleave
	// stages.
	if err := s.initWorkingDirectory(ctx, killCtx, e, sess, sessState); err != nil {
		// The plan won't run again, so log why it failed.
		logs.release()
		return nil, nil, xerrors.Errorf("initialize terraform: %w", err)
	}
	return plan(sess)
}

// needsInitSummaryPrefixes are how the summaries of Terraform's "run
// terraform init" errors begin. "Missing required provider" is left out: plan
// also reports it for a provider that only the state needs, and init runs
// without the state.
var needsInitSummaryPrefixes = []string{
	"Required plugins are not installed",
	"Inconsistent dependency lock file",
	"Module not installed",
	"Module source has changed",
	"Module version requirements have changed",
	"Backend initialization required",
}

// initDiagnosticsSink is the log sink for the plan that follows manualInit.
// It holds back the errors in needsInitSummaryPrefixes, so that a build which
// recovers by running init doesn't log them. Only the summary is matched,
// because the detail can quote the template.
type initDiagnosticsSink struct {
	logSink
	// held is written while the plan logs and read after it returns.
	held []string
}

var _ diagnosticSink = (*initDiagnosticsSink)(nil)

func (s *initDiagnosticsSink) sinkForDiagnostic(diag *tfjson.Diagnostic) logSink {
	for _, prefix := range needsInitSummaryPrefixes {
		if strings.HasPrefix(diag.Summary, prefix) {
			return heldLogSink{s}
		}
	}
	return s.logSink
}

// heldLogSink keeps the lines logged to it in sink.held.
type heldLogSink struct {
	sink *initDiagnosticsSink
}

func (h heldLogSink) ProvisionLog(_ proto.LogLevel, output string) {
	h.sink.held = append(h.sink.held, output)
}

// heldLines returns the held lines. Any line means plan asked for init.
func (s *initDiagnosticsSink) heldLines() []string {
	return s.held
}

// release logs the held lines.
func (s *initDiagnosticsSink) release() {
	for _, line := range s.held {
		s.ProvisionLog(proto.LogLevel_ERROR, line)
	}
	s.held = nil
}

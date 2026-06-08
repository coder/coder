package agentcontext

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// Default caps. Copied from the RFC. The Manager exposes
// overrides via Options.
const (
	// DefaultMaxResourceBytes is the per-resource payload cap.
	// Resources whose payload exceeds this size are emitted
	// with Status == StatusOversize and an empty Payload.
	DefaultMaxResourceBytes = 64 * 1024
	// DefaultMaxSnapshotBytes is the aggregate payload cap.
	// Resources past this cap are emitted with Status ==
	// StatusExcluded.
	DefaultMaxSnapshotBytes = 2 * 1024 * 1024
	// DefaultMaxResources is the resource count cap. Resources
	// past this cap are emitted with Status == StatusExcluded.
	DefaultMaxResources = 500
	// DefaultMaxScanDepth bounds how deep the recursive walk
	// descends from each scan root. The default avoids runaway
	// scans in node_modules / vendor / .git trees while still
	// covering realistic monorepo layouts.
	DefaultMaxScanDepth = 8
)

// File-name conventions recognized by the v1 resolver.
var (
	// instructionFileNames are picked up from any scan root.
	// Matching is case-insensitive on the basename.
	instructionFileNames = []string{
		"AGENTS.md",
		"CLAUDE.md",
		".cursorrules",
	}
	// mcpConfigFileName is recognized at any depth under a
	// scan root.
	mcpConfigFileName = ".mcp.json"
	// skillMetaFileName is the file inside a skill directory
	// that carries the skill front-matter.
	skillMetaFileName = "SKILL.md"
)

// skipDirNames are directory basenames that the recursive walk
// never descends into. The list mirrors what most language
// tool-chains treat as opaque.
var skipDirNames = map[string]struct{}{
	".git":         {},
	".hg":          {},
	".svn":         {},
	"node_modules": {},
	"vendor":       {},
	"target":       {},
	"dist":         {},
	"build":        {},
	".venv":        {},
	"__pycache__":  {},
}

// recognizedInstructionFile reports whether name is one of the
// instruction-file conventions, case-insensitively.
func recognizedInstructionFile(name string) bool {
	for _, candidate := range instructionFileNames {
		if strings.EqualFold(name, candidate) {
			return true
		}
	}
	return false
}

// Resolver walks one or more scan roots and produces a snapshot
// of every recognized resource it finds. The Resolver is
// stateless; the Manager owns the scan-root list and orchestrates
// successive resolves.
type Resolver struct {
	// MaxResourceBytes caps the per-resource payload size. Use
	// DefaultMaxResourceBytes if zero.
	MaxResourceBytes uint64
	// MaxSnapshotBytes caps the aggregate payload size. Use
	// DefaultMaxSnapshotBytes if zero.
	MaxSnapshotBytes uint64
	// MaxResources caps the resource count. Use
	// DefaultMaxResources if zero.
	MaxResources int
	// MaxDepth caps the directory walk depth. Use
	// DefaultMaxScanDepth if zero.
	MaxDepth int
	// MCP, when non-nil, is consulted after the filesystem
	// pass and contributes any KindMCPServer resources for
	// live MCP servers.
	MCP MCPProvider
}

// ScanRoot describes a single directory or file the resolver
// should examine.
type ScanRoot struct {
	// Path is the absolute path. Symlinks should already be
	// resolved.
	Path string
	// UserSource is the canonical source path the user
	// declared, when this root came from a user-added Source.
	// Empty for built-in roots.
	UserSource string
}

// Resolve walks the supplied scan roots and returns a Snapshot.
// The version and schemaVersion fields are stamped by the
// caller; Resolve fills everything else. Resolve is the
// non-cancellable convenience wrapper around ResolveContext
// using context.Background.
func (r *Resolver) Resolve(roots []ScanRoot) Snapshot {
	return r.ResolveContext(context.Background(), roots)
}

// ResolveContext is the cancellable variant of Resolve. The
// context is checked between scan roots so callers can bail out
// of a long pass without waiting for the current root's walk to
// finish. Cancellation never partially populates the returned
// Snapshot: a canceled context returns an empty Snapshot with
// SnapshotError set to the context error.
func (r *Resolver) ResolveContext(ctx context.Context, roots []ScanRoot) Snapshot {
	res := r.normalize()
	resources, snapErrs := res.walk(ctx, roots)
	if err := ctx.Err(); err != nil {
		return Snapshot{SnapshotError: err.Error()}
	}
	resources, totalBytes := res.applyCaps(resources)

	// Append MCP server resources after the filesystem caps
	// are applied so a runaway MCP server cannot crowd out
	// instruction files.
	if r.MCP != nil {
		mcp := r.MCP.MCPResources()
		startIdx := len(resources)
		resources = append(resources, mcp...)
		// MCP resources may push the aggregate over the
		// count or byte cap. Apply both, picking up
		// where applyCaps left off.
		resources, snapErrs = res.applyMCPCaps(resources, startIdx, totalBytes, snapErrs)
	}

	// Deterministic order by ID for stable IDs and hashes.
	slices.SortFunc(resources, func(a, b Resource) int {
		return strings.Compare(a.ID, b.ID)
	})

	var payloadBytes uint64
	for _, r := range resources {
		payloadBytes += uint64(len(r.Payload))
	}

	hash := ComputeAggregateHash(resources)

	snap := Snapshot{
		Resources:     resources,
		AggregateHash: hash,
		PayloadBytes:  payloadBytes,
	}
	if len(snapErrs) > 0 {
		// Pick the most severe single error. Today every
		// snapshot-level problem is "warning equivalent" so
		// the first one wins; the design reserves the field
		// for a singular message.
		snap.SnapshotError = snapErrs[0]
	}
	return snap
}

func (r *Resolver) normalize() *Resolver {
	out := *r
	if out.MaxResourceBytes == 0 {
		out.MaxResourceBytes = DefaultMaxResourceBytes
	}
	if out.MaxSnapshotBytes == 0 {
		out.MaxSnapshotBytes = DefaultMaxSnapshotBytes
	}
	if out.MaxResources == 0 {
		out.MaxResources = DefaultMaxResources
	}
	if out.MaxDepth == 0 {
		out.MaxDepth = DefaultMaxScanDepth
	}
	return &out
}

// walk traverses every scan root and produces an unordered
// resource list. Aggregate caps are applied separately. The ctx
// is checked between roots so callers can bail out promptly.
func (r *Resolver) walk(ctx context.Context, roots []ScanRoot) (resources []Resource, snapErrs []string) {
	// Dedup roots by canonical path. The first occurrence
	// wins so user-added roots that overlap with a built-in
	// root attribute resources to the built-in.
	seenRoot := make(map[string]struct{}, len(roots))
	dedup := make([]ScanRoot, 0, len(roots))
	for _, root := range roots {
		if root.Path == "" {
			continue
		}
		if _, ok := seenRoot[root.Path]; ok {
			continue
		}
		seenRoot[root.Path] = struct{}{}
		dedup = append(dedup, root)
	}

	// Deduplicate resources across roots by ID. Without this,
	// a built-in root and a user root that both cover the
	// same project tree would double-count AGENTS.md.
	seenID := make(map[string]struct{})

	for _, root := range dedup {
		if err := ctx.Err(); err != nil {
			return nil, []string{err.Error()}
		}
		info, err := os.Stat(root.Path)
		if err != nil {
			// Missing roots silently fall through. The user
			// either added a path that does not exist yet or
			// removed it later. The watcher will surface
			// re-creation as a change event.
			continue
		}
		if !info.IsDir() {
			// Single-file roots are classified directly.
			if res, ok := r.classifyFile(root.Path, root.Path, info, root.UserSource); ok {
				if _, dup := seenID[res.ID]; !dup {
					seenID[res.ID] = struct{}{}
					resources = append(resources, res)
				}
			}
			continue
		}
		walkErr := r.walkDir(ctx, root, &resources, seenID)
		if walkErr != nil {
			snapErrs = append(snapErrs, fmt.Sprintf("walk %q: %s", root.Path, walkErr))
		}
	}
	return resources, snapErrs
}

// walkDir performs the recursive descent for a single scan
// directory. It honors r.MaxDepth and skipDirNames. The ctx is
// checked inside the WalkDir callback so cancellation
// terminates the walk even mid-root.
func (r *Resolver) walkDir(ctx context.Context, root ScanRoot, out *[]Resource, seenID map[string]struct{}) error {
	rootDepth := strings.Count(filepath.Clean(root.Path), string(os.PathSeparator))
	maxDepth := rootDepth + r.MaxDepth

	return filepath.WalkDir(root.Path, func(path string, d fs.DirEntry, err error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err != nil {
			// Surface the error as Unreadable when we can
			// associate it with a single recognized file;
			// otherwise let the walk continue.
			if d != nil && !d.IsDir() {
				kind, recognized := kindFromFilename(d.Name())
				if recognized {
					res := Resource{
						ID:         resourceID(kind, path),
						Kind:       kind,
						Source:     path,
						SizeBytes:  0,
						Status:     StatusUnreadable,
						Error:      err.Error(),
						SourcePath: root.UserSource,
					}
					if _, dup := seenID[res.ID]; !dup {
						seenID[res.ID] = struct{}{}
						*out = append(*out, res)
					}
				}
			}
			if errors.Is(err, fs.ErrPermission) {
				// Permission errors on a directory: skip the
				// subtree but continue walking siblings.
				if d != nil && d.IsDir() {
					return fs.SkipDir
				}
			}
			return nil
		}

		if d.IsDir() {
			if strings.Count(path, string(os.PathSeparator)) > maxDepth {
				return fs.SkipDir
			}
			if _, skip := skipDirNames[d.Name()]; skip && path != root.Path {
				return fs.SkipDir
			}
			// If we are entering a "skills container"
			// directory (".agents/skills", "~/.coder/skills",
			// "plugins/<plugin>/skills"), eagerly emit skill
			// resources for its immediate subdirectories.
			if isSkillsContainer(path) {
				r.emitSkillsFromContainer(path, root, out, seenID)
			}
			return nil
		}

		// Regular file.
		info, statErr := d.Info()
		if statErr != nil {
			return nil
		}
		if res, ok := r.classifyFile(root.Path, path, info, root.UserSource); ok {
			if _, dup := seenID[res.ID]; dup {
				return nil
			}
			seenID[res.ID] = struct{}{}
			*out = append(*out, res)
		}
		return nil
	})
}

// kindFromFilename maps a file basename to its ResourceKind.
// recognized=false when the name matches no convention.
func kindFromFilename(name string) (kind ResourceKind, recognized bool) {
	switch {
	case recognizedInstructionFile(name):
		return KindInstructionFile, true
	case name == mcpConfigFileName:
		return KindMCPConfig, true
	case name == skillMetaFileName:
		return KindSkill, true
	default:
		return 0, false
	}
}

// resolveReadTarget produces the path and FileInfo that should
// be used to read the resource. When the input is not a
// symlink the original path and info are returned unchanged.
// When it is a symlink the target is resolved and validated
// against scanRoot so a malicious AGENTS.md ->
// ~/.ssh/id_rsa cannot exfiltrate files outside the
// contributing scan root.
//
// codex follows symlinks unconditionally because it trusts the
// local user's filesystem. Coder workspaces may execute
// templates and repositories that the agent operator did not
// author, so the resolver follows symlinks only within the
// scan-root boundary. Symlinks whose targets escape the
// boundary are emitted as StatusInvalid; broken symlinks and
// non-regular targets are emitted as StatusUnreadable.
func resolveReadTarget(path string, info fs.FileInfo, scanRoot string) (readPath string, readInfo fs.FileInfo, ok bool, status ResourceStatus, errMsg string) {
	if info.Mode()&fs.ModeSymlink == 0 {
		return path, info, true, StatusOK, ""
	}
	target, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", nil, false, StatusUnreadable, fmt.Sprintf("symlink resolve: %v", err)
	}
	rootClean := filepath.Clean(scanRoot)
	if !pathHasPrefix(target, rootClean) {
		return "", nil, false, StatusInvalid, fmt.Sprintf("symlink target %q escapes scan root %q", target, scanRoot)
	}
	tgtInfo, err := os.Stat(target)
	if err != nil {
		return "", nil, false, StatusUnreadable, err.Error()
	}
	if !tgtInfo.Mode().IsRegular() {
		return "", nil, false, StatusInvalid, fmt.Sprintf("symlink target %q is not a regular file", target)
	}
	return target, tgtInfo, true, StatusOK, ""
}

// classifyFile inspects a single file path and produces a
// Resource when the basename matches a recognized convention.
func (r *Resolver) classifyFile(scanRoot, path string, info fs.FileInfo, userSource string) (Resource, bool) {
	name := info.Name()
	switch {
	case recognizedInstructionFile(name):
		return r.readInstructionFile(scanRoot, path, info, userSource), true
	case name == mcpConfigFileName:
		return r.readMCPConfig(scanRoot, path, info, userSource), true
	case name == skillMetaFileName:
		// SKILL.md outside a skills container is still a
		// valid skill if its parent directory name matches
		// the front-matter name. emitSkillsFromContainer
		// already handles the common case; here we cover
		// "user adds a single SKILL.md file as a source".
		res, ok := r.readSkillMeta(scanRoot, path, info, userSource)
		return res, ok
	default:
		return Resource{}, false
	}
}

// readInstructionFile reads an instruction file and produces a
// KindInstructionFile resource. The file is read into memory
// with the per-resource cap applied.
//
// The bytes are returned verbatim. The legacy code path in
// agentcontextconfig/api.go strips HTML comments and invisible
// Unicode before serving instruction-file contents to chat; the
// equivalent sanitization for this pipeline lives in the
// follow-up chatd integration that consumes Snapshot.Resources.
// Until that lands, downstream consumers that render these
// payloads must sanitize themselves.
func (r *Resolver) readInstructionFile(scanRoot, path string, info fs.FileInfo, userSource string) Resource {
	res := r.readFileResource(KindInstructionFile, scanRoot, path, info, userSource)
	if res.Status == StatusOK {
		res.Description = firstLine(string(res.Payload))
	}
	return res
}

// readMCPConfig reads a .mcp.json file and produces a
// KindMCPConfig resource carrying only path metadata and a
// content hash.
//
// .mcp.json fragments frequently embed secret-bearing fields
// (Env tokens, Authorization headers). The resolver hashes the
// file for change detection but intentionally does not ship
// the bytes; the live MCP server's tool list arrives via the
// MCPProvider as a KindMCPServer resource, which is what
// downstream consumers actually need.
func (r *Resolver) readMCPConfig(scanRoot, path string, info fs.FileInfo, userSource string) Resource {
	res := Resource{
		ID:         resourceID(KindMCPConfig, path),
		Kind:       KindMCPConfig,
		Source:     path,
		SizeBytes:  safeUint64(info.Size()),
		SourcePath: userSource,
	}
	readPath, readInfo, ok, status, errMsg := resolveReadTarget(path, info, scanRoot)
	if !ok {
		res.Status = status
		res.Error = errMsg
		return res
	}
	res.SizeBytes = safeUint64(readInfo.Size())
	if safeUint64(readInfo.Size()) > r.MaxResourceBytes {
		res.Status = StatusOversize
		res.Error = fmt.Sprintf("file size %d exceeds per-resource cap of %d bytes", readInfo.Size(), r.MaxResourceBytes)
		if data, err := readFileCapped(readPath, safeInt64(r.MaxResourceBytes)); err == nil {
			res.ContentHash = sha256.Sum256(data)
		}
		return res
	}
	data, err := os.ReadFile(readPath)
	if err != nil {
		res.Status = StatusUnreadable
		res.Error = err.Error()
		return res
	}
	res.ContentHash = sha256.Sum256(data)
	return res
}

// readFileResource is the shared plumbing for kinds whose only
// difference is the enum stamped on the Resource: build the
// Resource header, enforce the per-resource size cap, read the
// file, hash it, attach the bytes. Callers add kind-specific
// post-processing (e.g. firstLine for instruction files) by
// inspecting Status==StatusOK.
func (r *Resolver) readFileResource(kind ResourceKind, scanRoot, path string, info fs.FileInfo, userSource string) Resource {
	res := Resource{
		ID:         resourceID(kind, path),
		Kind:       kind,
		Source:     path,
		SizeBytes:  safeUint64(info.Size()),
		SourcePath: userSource,
	}
	readPath, readInfo, ok, status, errMsg := resolveReadTarget(path, info, scanRoot)
	if !ok {
		res.Status = status
		res.Error = errMsg
		return res
	}
	res.SizeBytes = safeUint64(readInfo.Size())
	if safeUint64(readInfo.Size()) > r.MaxResourceBytes {
		res.Status = StatusOversize
		res.Error = fmt.Sprintf("file size %d exceeds per-resource cap of %d bytes", readInfo.Size(), r.MaxResourceBytes)
		// Still hash the (capped) content so a fix is
		// detectable.
		if data, err := readFileCapped(readPath, safeInt64(r.MaxResourceBytes)); err == nil {
			res.ContentHash = sha256.Sum256(data)
		}
		return res
	}
	data, err := os.ReadFile(readPath)
	if err != nil {
		res.Status = StatusUnreadable
		res.Error = err.Error()
		return res
	}
	res.Payload = data
	res.ContentHash = sha256.Sum256(data)
	return res
}

// readSkillMeta reads a SKILL.md file, parses its front-matter,
// and emits a KindSkill resource. The name encoded in the
// front-matter must match the parent directory's basename to
// be considered valid; otherwise Status is StatusInvalid.
func (r *Resolver) readSkillMeta(scanRoot, path string, info fs.FileInfo, userSource string) (Resource, bool) {
	parent := filepath.Base(filepath.Dir(path))
	res := Resource{
		ID:         resourceID(KindSkill, filepath.Dir(path)),
		Kind:       KindSkill,
		Source:     filepath.Dir(path),
		SizeBytes:  safeUint64(info.Size()),
		SourcePath: userSource,
	}
	readPath, readInfo, ok, status, errMsg := resolveReadTarget(path, info, scanRoot)
	if !ok {
		res.Status = status
		res.Error = errMsg
		return res, true
	}
	res.SizeBytes = safeUint64(readInfo.Size())
	if safeUint64(readInfo.Size()) > r.MaxResourceBytes {
		res.Status = StatusOversize
		res.Error = fmt.Sprintf("file size %d exceeds per-resource cap of %d bytes", readInfo.Size(), r.MaxResourceBytes)
		// Hash the (capped) prefix so an edit that keeps
		// the file oversize still shifts the aggregate
		// hash and triggers a re-broadcast. Mirrors the
		// behavior in readFileResource.
		if data, err := readFileCapped(readPath, safeInt64(r.MaxResourceBytes)); err == nil {
			res.ContentHash = sha256.Sum256(data)
		}
		return res, true
	}
	data, err := os.ReadFile(readPath)
	if err != nil {
		res.Status = StatusUnreadable
		res.Error = err.Error()
		return res, true
	}
	res.ContentHash = sha256.Sum256(data)
	name, description, _, err := workspacesdk.ParseSkillFrontmatter(string(data))
	if err != nil {
		res.Status = StatusInvalid
		res.Error = err.Error()
		return res, true
	}
	if name != parent {
		res.Status = StatusInvalid
		res.Error = fmt.Sprintf("front-matter name %q does not match directory %q", name, parent)
		return res, true
	}
	if !workspacesdk.SkillNamePattern.MatchString(name) {
		res.Status = StatusInvalid
		res.Error = fmt.Sprintf("skill name %q is not kebab-case", name)
		return res, true
	}
	res.Description = description
	res.Name = name
	res.Payload = data
	return res, true
}

// emitSkillsFromContainer scans the immediate children of a
// recognized skills-container directory and emits one Skill
// resource per subdirectory whose SKILL.md parses cleanly.
func (r *Resolver) emitSkillsFromContainer(container string, root ScanRoot, out *[]Resource, seenID map[string]struct{}) {
	entries, err := os.ReadDir(container)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		meta := filepath.Join(container, e.Name(), skillMetaFileName)
		// Lstat (not Stat) so a symlinked SKILL.md is
		// detected and routed through resolveReadTarget,
		// which enforces the scan-root boundary.
		info, err := os.Lstat(meta)
		if err != nil {
			continue
		}
		res, ok := r.readSkillMeta(root.Path, meta, info, root.UserSource)
		if !ok {
			continue
		}
		if _, dup := seenID[res.ID]; dup {
			continue
		}
		seenID[res.ID] = struct{}{}
		*out = append(*out, res)
	}
}

// applyCaps enforces the resource-count cap and aggregate
// payload cap. Resources past either cap have their Status set
// to StatusExcluded and their Payload cleared. The returned
// byte total is the sum of surviving payloads, so callers that
// append additional resources (e.g. MCP server tool lists) can
// apply the same byte cap to the appended slice.
func (r *Resolver) applyCaps(resources []Resource) ([]Resource, uint64) {
	// Stable sort by (Kind asc, Source asc) so excluded
	// resources are deterministic.
	slices.SortStableFunc(resources, func(a, b Resource) int {
		if a.Kind != b.Kind {
			return int(a.Kind) - int(b.Kind)
		}
		return strings.Compare(a.Source, b.Source)
	})

	var total uint64
	for i := range resources {
		if i >= r.MaxResources {
			resources[i] = excluded(resources[i],
				fmt.Sprintf("dropped to fit %d-resource snapshot count cap", r.MaxResources))
			continue
		}
		if resources[i].Status != StatusOK {
			continue
		}
		size := uint64(len(resources[i].Payload))
		if total+size > r.MaxSnapshotBytes {
			resources[i] = excluded(resources[i],
				fmt.Sprintf("dropped to fit %d-byte aggregate cap", r.MaxSnapshotBytes))
			continue
		}
		total += size
	}
	return resources, total
}

// applyMCPCaps enforces both the count cap and the remaining
// aggregate byte cap on MCP resources appended after
// applyCaps. startIdx is the first index of the appended tail.
// priorBytes is the sum of payload bytes already committed by
// the filesystem pass; MCP resources whose payloads would push
// the running total past MaxSnapshotBytes are stamped
// StatusExcluded. Without this guard a provider returning one
// large KindMCPServer payload would exceed the aggregate cap
// with StatusOK, breaking the contract in
// DefaultMaxSnapshotBytes.
func (r *Resolver) applyMCPCaps(resources []Resource, startIdx int, priorBytes uint64, snapErrs []string) ([]Resource, []string) {
	total := priorBytes
	countCapHit := false
	byteCapHit := false
	for i := startIdx; i < len(resources); i++ {
		if i >= r.MaxResources {
			resources[i] = excluded(resources[i],
				fmt.Sprintf("dropped to fit %d-resource snapshot count cap", r.MaxResources))
			countCapHit = true
			continue
		}
		if resources[i].Status != StatusOK {
			continue
		}
		size := uint64(len(resources[i].Payload))
		if total+size > r.MaxSnapshotBytes {
			resources[i] = excluded(resources[i],
				fmt.Sprintf("dropped to fit %d-byte aggregate cap", r.MaxSnapshotBytes))
			byteCapHit = true
			continue
		}
		total += size
	}
	if countCapHit {
		snapErrs = append(snapErrs, fmt.Sprintf("snapshot exceeds %d-resource count cap", r.MaxResources))
	}
	if byteCapHit {
		snapErrs = append(snapErrs, fmt.Sprintf("snapshot exceeds %d-byte aggregate cap", r.MaxSnapshotBytes))
	}
	return resources, snapErrs
}

// excluded mutates and returns the supplied resource with the
// StatusExcluded outcome.
func excluded(r Resource, reason string) Resource {
	r.Status = StatusExcluded
	r.Error = reason
	r.Payload = nil
	return r
}

// isSkillsContainer reports whether dir is a recognized skills
// container directory whose immediate children carry SKILL.md
// files. Both bare "skills" and nested "<parent>/skills"
// directories qualify (e.g. ".agents/skills",
// "plugins/foo/skills").
func isSkillsContainer(dir string) bool {
	return filepath.Base(dir) == "skills"
}

// resourceID builds a stable resource ID. Kind plus canonical
// source path is enough; sources never collide across kinds for
// v1 because each kind owns a distinct file-name pattern.
func resourceID(kind ResourceKind, source string) string {
	return kind.String() + ":" + source
}

// readFileCapped reads up to maxBytes from path. It returns the
// truncated payload on success.
func readFileCapped(path string, maxBytes int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, maxBytes))
}

// firstLine returns the first non-empty trimmed line of s, used
// as a short description fallback.
func firstLine(s string) string {
	for line := range strings.SplitSeq(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Strip leading markdown heading markers for prettier
		// descriptions.
		return strings.TrimSpace(headingPrefixRegex.ReplaceAllString(line, ""))
	}
	return ""
}

var headingPrefixRegex = regexp.MustCompile(`^#+\s*`)

// safeUint64 converts a non-negative int64 to uint64. Negative
// inputs are clamped to 0, which is safe for the size-tracking
// fields that use it; a negative os.FileInfo size is pathological
// and never indicates real content.
func safeUint64(n int64) uint64 {
	if n < 0 {
		return 0
	}
	return uint64(n)
}

// safeInt64 converts a uint64 to int64, clamping to math.MaxInt64
// when the input would overflow. The caps configured on the
// resolver never approach 2^63 bytes, so the clamp only guards
// against pathological caller input.
func safeInt64(n uint64) int64 {
	if n > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(n)
}

// ResourceKind describes the category of a resolved context
// resource. The values mirror the proto ContextResource.Kind
// enum reserved in the RFC; future kinds (PLUGIN, HOOK,
// SUBAGENT, COMMAND) are defined here so callers can switch
// exhaustively, but no v1 resolver emits them.
type ResourceKind int

const (
	KindUnspecified ResourceKind = iota
	// KindInstructionFile covers AGENTS.md, CLAUDE.md,
	// .cursorrules, and similar plain-text rule files that
	// inject content into the model prompt.
	KindInstructionFile
	// KindSkill is a directory containing SKILL.md and any
	// supporting files. Only the meta file is read at
	// resolve time; bodies are fetched on demand.
	KindSkill
	// KindMCPConfig is a .mcp.json fragment declaring one or
	// more MCP servers.
	KindMCPConfig
	// KindMCPServer is a live MCP server's resolved tool list,
	// populated by an MCPProvider after the server has been
	// connected.
	KindMCPServer
	// KindPlugin is reserved for Claude Code plugin manifests.
	// Not emitted by v1.
	KindPlugin
	// KindHook is reserved for plugin hooks. Not emitted by v1.
	KindHook
	// KindSubagent is reserved for plugin-declared subagents.
	// Not emitted by v1.
	KindSubagent
	// KindCommand is reserved for plugin slash commands.
	// Not emitted by v1.
	KindCommand
)

// String returns the lower-snake-case name used in IDs and
// metrics. Unknown values stringify to "unknown".
func (k ResourceKind) String() string {
	switch k {
	case KindInstructionFile:
		return "instruction_file"
	case KindSkill:
		return "skill"
	case KindMCPConfig:
		return "mcp_config"
	case KindMCPServer:
		return "mcp_server"
	case KindPlugin:
		return "plugin"
	case KindHook:
		return "hook"
	case KindSubagent:
		return "subagent"
	case KindCommand:
		return "command"
	default:
		return "unknown"
	}
}

// ResourceStatus describes whether a resource was successfully
// read and whether its payload survived the per-resource and
// aggregate caps.
//
// Note: these iota ordinals do NOT match the proto
// ContextResource.Status ordinals one-to-one. The proto enum
// reserves 0 for STATUS_UNSPECIFIED and shifts every value by
// one, so the conversion in resourceStatusToProto cannot be
// replaced with a direct int cast. ResourceKind, by contrast,
// does align with its proto counterpart.
type ResourceStatus int

const (
	// StatusOK indicates the payload was populated.
	StatusOK ResourceStatus = iota
	// StatusOversize indicates the resource exceeded the
	// per-resource size cap; payload is omitted.
	StatusOversize
	// StatusUnreadable indicates an IO error reading the
	// resource (permission denied, broken symlink, etc.).
	StatusUnreadable
	// StatusInvalid indicates the resource was structurally
	// malformed (bad JSON, missing front-matter, etc.).
	StatusInvalid
	// StatusExcluded indicates the resource was dropped to fit
	// the aggregate snapshot or count cap.
	StatusExcluded
)

// String returns the lower-snake-case name used in IDs and
// metrics. Unknown values stringify to "unknown".
func (s ResourceStatus) String() string {
	switch s {
	case StatusOK:
		return "ok"
	case StatusOversize:
		return "oversize"
	case StatusUnreadable:
		return "unreadable"
	case StatusInvalid:
		return "invalid"
	case StatusExcluded:
		return "excluded"
	default:
		return "unknown"
	}
}

// Resource is what the resolver emits for each recognized file
// or live server it discovers under a scan root. The struct is
// intentionally flat; the typed wire mapping happens in
// drpc.go where Kind selects the proto oneof variant.
type Resource struct {
	// ID is stable across pushes for the same logical
	// resource. The current scheme is "<kind>:<source>". It is
	// used for in-snapshot dedup and as part of the aggregate
	// hash; it is not transmitted on the wire.
	ID string
	// Kind classifies the resource. Drives which proto oneof
	// variant the DRPC adapter sets.
	Kind ResourceKind
	// Source is the file path or MCP server name.
	Source string
	// ContentHash is sha256 over the resource's original
	// bytes (or transport-encoded server tool list).
	ContentHash [32]byte
	// Payload is the full bytes when Status == StatusOK; the
	// per-resource and aggregate caps may leave it empty.
	// Unused for KindMCPServer (Tools is used instead).
	Payload []byte
	// SizeBytes is the original payload size, populated
	// regardless of Status.
	SizeBytes uint64
	// Status records OK or a reason the payload is absent.
	Status ResourceStatus
	// Error is populated whenever Status != StatusOK; may
	// also carry a non-fatal warning when Status == StatusOK.
	Error string
	// Name is the resource's own short identifier. Currently
	// populated for KindSkill (from front-matter) and
	// KindMCPServer (server name); empty for other kinds.
	Name string
	// Description is a short human-readable summary (skill
	// front-matter description, MCP server description,
	// instruction-file first line). Shipped on the wire only
	// for kinds whose body type carries a description field.
	Description string
	// SourcePath is the user-declared source that contributed
	// the resource; empty for built-in scan roots.
	SourcePath string
	// Tools is populated for KindMCPServer with the live
	// server's tool list; empty otherwise.
	Tools []MCPTool
}

// MCPTool mirrors the wire MCPTool message. InputSchema is the
// JSON-Schema-shaped object the MCP server reported for the
// tool's arguments.
type MCPTool struct {
	Name        string
	Description string
	InputSchema map[string]any
}

// Snapshot is the immutable bundle of resources produced by a
// single resolver pass.
type Snapshot struct {
	// Version is monotonically increasing per Manager
	// instance; resets when the agent process restarts.
	Version uint64
	// SchemaVersion is bumped if the resource shape on the
	// wire changes.
	SchemaVersion uint64
	// AggregateHash is sha256 over a canonical encoding of
	// (ID, Kind, Source, ContentHash, Status) for every
	// resource. Identical inputs always produce identical
	// hashes; see ComputeAggregateHash.
	AggregateHash [32]byte
	// Resources is sorted by ID for deterministic encoding.
	Resources []Resource
	// PayloadBytes is the sum of len(Resource.Payload) across
	// emitted resources after caps were applied.
	PayloadBytes uint64
	// SnapshotError carries a single snapshot-level error
	// string when present (count cap exceeded, watcher
	// degraded, ENOSPC, etc.). Empty when healthy.
	SnapshotError string
}

// ComputeAggregateHash produces the deterministic snapshot
// aggregate hash for the supplied resources. The caller does
// not need to pre-sort; the function sorts a copy of the slice
// to keep its inputs side-effect free.
//
// The encoding is a Netstring-style stream. Each string field
// is written as the decimal-ASCII length, the literal ':', and
// the raw UTF-8 bytes. ContentHash is written as 32 raw bytes
// without a length prefix because it is a fixed-size SHA-256
// digest. Resources are separated by a single NUL byte. The
// scheme is internal to the agent and coderd, but it is stable
// across platforms because every field has an unambiguous
// length.
func ComputeAggregateHash(resources []Resource) [32]byte {
	indexed := make([]Resource, len(resources))
	copy(indexed, resources)
	slices.SortFunc(indexed, func(a, b Resource) int {
		return strings.Compare(a.ID, b.ID)
	})

	h := sha256.New()
	for _, r := range indexed {
		writeLengthPrefixed(h, r.ID)
		writeLengthPrefixed(h, r.Kind.String())
		writeLengthPrefixed(h, r.Source)
		_, _ = h.Write(r.ContentHash[:])
		writeLengthPrefixed(h, r.Status.String())
		_, _ = h.Write([]byte{0})
	}
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

// writeLengthPrefixed writes a decimal-ASCII length prefix, a
// literal ':' separator, and the raw bytes of s. This matches
// the Netstring framing used by ComputeAggregateHash.
func writeLengthPrefixed(h interface{ Write([]byte) (int, error) }, s string) {
	_, _ = h.Write([]byte(strconv.Itoa(len(s))))
	_, _ = h.Write([]byte{':'})
	_, _ = h.Write([]byte(s))
}

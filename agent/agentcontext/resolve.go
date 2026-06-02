package agentcontext

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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
// caller; Resolve fills everything else.
func (r *Resolver) Resolve(roots []ScanRoot) Snapshot {
	res := r.normalize()
	resources, snapErrs := res.walk(roots)
	resources = res.applyCaps(resources)

	// Append MCP server resources after the filesystem caps
	// are applied so a runaway MCP server cannot crowd out
	// instruction files.
	if r.MCP != nil {
		mcp := r.MCP.MCPResources()
		resources = append(resources, mcp...)
		// MCP resources may push the aggregate over the cap.
		// Re-apply count and size limits to MCP entries only.
		resources, snapErrs = res.applyMCPCaps(resources, snapErrs)
	}

	// Deterministic order by ID for stable IDs and hashes.
	sort.Slice(resources, func(i, j int) bool {
		return resources[i].ID < resources[j].ID
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
// resource list. Aggregate caps are applied separately.
func (r *Resolver) walk(roots []ScanRoot) (resources []Resource, snapErrs []string) {
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
			if res, ok := r.classifyFile(root.Path, info, root.UserSource); ok {
				if _, dup := seenID[res.ID]; !dup {
					seenID[res.ID] = struct{}{}
					resources = append(resources, res)
				}
			}
			continue
		}
		walkErr := r.walkDir(root, &resources, seenID)
		if walkErr != nil {
			snapErrs = append(snapErrs, fmt.Sprintf("walk %q: %s", root.Path, walkErr))
		}
	}
	return resources, snapErrs
}

// walkDir performs the recursive descent for a single scan
// directory. It honors r.MaxDepth and skipDirNames.
func (r *Resolver) walkDir(root ScanRoot, out *[]Resource, seenID map[string]struct{}) error {
	rootDepth := strings.Count(filepath.Clean(root.Path), string(os.PathSeparator))
	maxDepth := rootDepth + r.MaxDepth

	return filepath.WalkDir(root.Path, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Surface the error as Unreadable when we can
			// associate it with a single recognized file;
			// otherwise let the walk continue.
			if d != nil && !d.IsDir() {
				if recognizedInstructionFile(d.Name()) ||
					d.Name() == mcpConfigFileName ||
					d.Name() == skillMetaFileName {
					res := Resource{
						ID:         resourceID(KindInstructionFile, path),
						Kind:       KindInstructionFile,
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
		if res, ok := r.classifyFile(path, info, root.UserSource); ok {
			if _, dup := seenID[res.ID]; dup {
				return nil
			}
			seenID[res.ID] = struct{}{}
			*out = append(*out, res)
		}
		return nil
	})
}

// classifyFile inspects a single file path and produces a
// Resource when the basename matches a recognized convention.
func (r *Resolver) classifyFile(path string, info fs.FileInfo, userSource string) (Resource, bool) {
	name := info.Name()
	switch {
	case recognizedInstructionFile(name):
		return r.readInstructionFile(path, info, userSource), true
	case name == mcpConfigFileName:
		return r.readMCPConfig(path, info, userSource), true
	case name == skillMetaFileName:
		// SKILL.md outside a skills container is still a
		// valid skill if its parent directory name matches
		// the front-matter name. emitSkillsFromContainer
		// already handles the common case; here we cover
		// "user adds a single SKILL.md file as a source".
		res, ok := r.readSkillMeta(path, info, userSource)
		return res, ok
	default:
		return Resource{}, false
	}
}

// readInstructionFile reads an instruction file and produces a
// KindInstructionFile resource. The file is read into memory
// with the per-resource cap applied.
func (r *Resolver) readInstructionFile(path string, info fs.FileInfo, userSource string) Resource {
	res := Resource{
		ID:         resourceID(KindInstructionFile, path),
		Kind:       KindInstructionFile,
		Source:     path,
		SizeBytes:  safeUint64(info.Size()),
		SourcePath: userSource,
	}
	if safeUint64(info.Size()) > r.MaxResourceBytes {
		res.Status = StatusOversize
		res.Error = fmt.Sprintf("file size %d exceeds per-resource cap of %d bytes", info.Size(), r.MaxResourceBytes)
		// Still hash the (capped) content so a fix is
		// detectable.
		if data, err := readFileCapped(path, safeInt64(r.MaxResourceBytes)); err == nil {
			res.ContentHash = sha256.Sum256(data)
		}
		return res
	}
	data, err := os.ReadFile(path)
	if err != nil {
		res.Status = StatusUnreadable
		res.Error = err.Error()
		return res
	}
	res.Payload = data
	res.ContentHash = sha256.Sum256(data)
	// Description is just the first non-empty line.
	res.Description = firstLine(string(data))
	return res
}

// readMCPConfig reads a .mcp.json file and produces a
// KindMCPConfig resource. Parsing is left to consumers; the
// resolver only enforces JSON shape lightly via size and Unix
// newline conversion. Future work: detect malformed JSON and
// surface StatusInvalid.
func (r *Resolver) readMCPConfig(path string, info fs.FileInfo, userSource string) Resource {
	res := Resource{
		ID:         resourceID(KindMCPConfig, path),
		Kind:       KindMCPConfig,
		Source:     path,
		SizeBytes:  safeUint64(info.Size()),
		SourcePath: userSource,
	}
	if safeUint64(info.Size()) > r.MaxResourceBytes {
		res.Status = StatusOversize
		res.Error = fmt.Sprintf("file size %d exceeds per-resource cap of %d bytes", info.Size(), r.MaxResourceBytes)
		if data, err := readFileCapped(path, safeInt64(r.MaxResourceBytes)); err == nil {
			res.ContentHash = sha256.Sum256(data)
		}
		return res
	}
	data, err := os.ReadFile(path)
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
func (r *Resolver) readSkillMeta(path string, info fs.FileInfo, userSource string) (Resource, bool) {
	parent := filepath.Base(filepath.Dir(path))
	res := Resource{
		ID:         resourceID(KindSkill, filepath.Dir(path)),
		Kind:       KindSkill,
		Source:     filepath.Dir(path),
		SizeBytes:  safeUint64(info.Size()),
		SourcePath: userSource,
	}
	if safeUint64(info.Size()) > r.MaxResourceBytes {
		res.Status = StatusOversize
		res.Error = fmt.Sprintf("file size %d exceeds per-resource cap of %d bytes", info.Size(), r.MaxResourceBytes)
		return res, true
	}
	data, err := os.ReadFile(path)
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
		info, err := os.Stat(meta)
		if err != nil {
			continue
		}
		res, ok := r.readSkillMeta(meta, info, root.UserSource)
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
// to StatusExcluded and their Payload cleared.
func (r *Resolver) applyCaps(resources []Resource) []Resource {
	// Stable sort by (Kind asc, Source asc) so excluded
	// resources are deterministic.
	sort.SliceStable(resources, func(i, j int) bool {
		if resources[i].Kind != resources[j].Kind {
			return resources[i].Kind < resources[j].Kind
		}
		return resources[i].Source < resources[j].Source
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
	return resources
}

// applyMCPCaps re-applies the count cap after MCP resources are
// appended. MCP payloads are typically small JSON descriptors,
// so we treat the aggregate budget as already consumed by the
// filesystem pass.
func (r *Resolver) applyMCPCaps(resources []Resource, snapErrs []string) ([]Resource, []string) {
	if len(resources) <= r.MaxResources {
		return resources, snapErrs
	}
	for i := r.MaxResources; i < len(resources); i++ {
		resources[i] = excluded(resources[i],
			fmt.Sprintf("dropped to fit %d-resource snapshot count cap", r.MaxResources))
	}
	snapErrs = append(snapErrs, fmt.Sprintf("snapshot exceeds %d-resource count cap", r.MaxResources))
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
	for _, line := range strings.Split(s, "\n") {
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

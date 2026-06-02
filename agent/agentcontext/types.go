package agentcontext

import (
	"crypto/sha256"
	"sort"
	"strconv"
)

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

// Source is a user-declared scan root added to the agent's
// in-memory list via the HTTP API or boot-time env seeding.
// Identity is the canonical absolute path.
type Source struct {
	// Path is the canonical absolute path (symlinks resolved,
	// ~ expanded). Empty means the zero value.
	Path string
}

// Resource is what the resolver emits for each recognized file
// or live server it discovers under a scan root.
type Resource struct {
	// ID is stable across pushes for the same logical
	// resource. The current scheme is "<kind>:<source>".
	ID string
	// Kind classifies the resource for snapshot consumers.
	Kind ResourceKind
	// Source is the file path or MCP server name.
	Source string
	// ContentHash is sha256 over the resource's original
	// bytes (or transport-encoded server tool list).
	ContentHash [32]byte
	// Payload is the full bytes when Status == StatusOK; the
	// per-resource and aggregate caps may leave it empty.
	Payload []byte
	// SizeBytes is the original payload size, populated
	// regardless of Status.
	SizeBytes uint64
	// Status records OK or a reason the payload is absent.
	Status ResourceStatus
	// Error is populated whenever Status != StatusOK; may
	// also carry a non-fatal warning when Status == StatusOK.
	Error string
	// Description is a short human-readable summary (skill
	// front-matter description, MCP server description, etc.).
	Description string
	// SourcePath is the user-declared source that contributed
	// the resource; empty for built-in scan roots.
	SourcePath string
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
// The encoding is a newline-delimited stream of fields. The
// resource boundary is a single NUL byte. The boundary scheme
// is internal to the agent and coderd, but it is stable across
// platforms because every field is encoded as either a UTF-8
// string with a length prefix or a fixed-width integer.
func ComputeAggregateHash(resources []Resource) [32]byte {
	indexed := make([]Resource, len(resources))
	copy(indexed, resources)
	sort.Slice(indexed, func(i, j int) bool {
		return indexed[i].ID < indexed[j].ID
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

// writeLengthPrefixed writes a uvarint length prefix followed
// by the raw bytes of s.
func writeLengthPrefixed(h interface{ Write([]byte) (int, error) }, s string) {
	_, _ = h.Write([]byte(strconv.Itoa(len(s))))
	_, _ = h.Write([]byte{':'})
	_, _ = h.Write([]byte(s))
}

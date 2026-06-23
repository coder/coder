package agentcontext_test

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentcontext"
	"github.com/coder/coder/v2/testutil"
)

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

func mustWriteSkill(t *testing.T, dir, name, description string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, name), 0o755))
	mustWriteFile(t, filepath.Join(dir, name, "SKILL.md"),
		"---\nname: "+name+"\ndescription: "+description+"\n---\nSkill body for "+name)
}

func findResource(t *testing.T, resources []agentcontext.Resource, kind agentcontext.ResourceKind, source string) agentcontext.Resource {
	t.Helper()
	for _, r := range resources {
		if r.Kind == kind && r.Source == source {
			return r
		}
	}
	t.Fatalf("resource not found: kind=%s source=%s", kind, source)
	return agentcontext.Resource{}
}

func TestResolver_ProjectAGENTSFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "AGENTS.md"), "# Project rules\n\nDo the thing.")

	r := &agentcontext.Resolver{}
	snap := r.Resolve([]agentcontext.ScanRoot{{Path: dir}})

	require.Len(t, snap.Resources, 1)
	got := snap.Resources[0]
	require.Equal(t, agentcontext.KindInstructionFile, got.Kind)
	require.Equal(t, agentcontext.StatusOK, got.Status)
	require.Equal(t, filepath.Join(dir, "AGENTS.md"), got.Source)
	require.Contains(t, string(got.Payload), "Do the thing.")
	require.Equal(t, "Project rules", got.Description)
	require.NotEqual(t, [32]byte{}, got.ContentHash)
}

// TestResolver_InstructionNamesAreCaseSensitive verifies the
// resolver matches instruction filenames exactly, mirroring
// codex. A lower-case agents.md (for example a generated API
// reference doc) must not be mistaken for an instruction file.
func TestResolver_InstructionNamesAreCaseSensitive(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "agents.md"), "lower\n")
	mustWriteFile(t, filepath.Join(dir, "CLAUDE.md"), "claude\n")

	r := &agentcontext.Resolver{}
	snap := r.Resolve([]agentcontext.ScanRoot{{Path: dir}})

	require.Len(t, snap.Resources, 1)
	require.Equal(t, filepath.Join(dir, "CLAUDE.md"), snap.Resources[0].Source)
}

func TestResolver_SkillsContainerEmitsEachSubdir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustWriteSkill(t, filepath.Join(dir, ".agents", "skills"), "make-coffee", "Coffee skill")
	mustWriteSkill(t, filepath.Join(dir, ".agents", "skills"), "fold-laundry", "Laundry skill")

	r := &agentcontext.Resolver{}
	snap := r.Resolve([]agentcontext.ScanRoot{{Path: dir}})

	var kinds []string
	for _, res := range snap.Resources {
		kinds = append(kinds, res.Kind.String()+":"+filepath.Base(res.Source))
	}
	require.ElementsMatch(t, []string{
		"skill:make-coffee",
		"skill:fold-laundry",
	}, kinds)
}

func TestResolver_SkillNameMismatchInvalid(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, ".agents", "skills", "make-coffee")
	require.NoError(t, os.MkdirAll(skillsDir, 0o755))
	mustWriteFile(t, filepath.Join(skillsDir, "SKILL.md"),
		"---\nname: drink-tea\ndescription: oops\n---\nBody")

	r := &agentcontext.Resolver{}
	snap := r.Resolve([]agentcontext.ScanRoot{{Path: dir}})

	require.Len(t, snap.Resources, 1)
	got := snap.Resources[0]
	require.Equal(t, agentcontext.KindSkill, got.Kind)
	require.Equal(t, agentcontext.StatusInvalid, got.Status)
	require.Contains(t, got.Error, "does not match directory")
}

// TestResolver_SkillNameNonKebabInvalid exercises the kebab-case
// validation branch in readSkillMeta. The skill name matches the
// parent directory (so the mismatch check passes) but contains
// characters that SkillNamePattern rejects. Without this test
// the kebab branch could be deleted and the suite would still
// pass.
func TestResolver_SkillNameNonKebabInvalid(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	skillDir := filepath.Join(dir, ".agents", "skills", "Make_Coffee")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	mustWriteFile(t, filepath.Join(skillDir, "SKILL.md"),
		"---\nname: Make_Coffee\ndescription: oops\n---\nBody")

	r := &agentcontext.Resolver{}
	snap := r.Resolve([]agentcontext.ScanRoot{{Path: dir}})

	require.Len(t, snap.Resources, 1)
	got := snap.Resources[0]
	require.Equal(t, agentcontext.KindSkill, got.Kind)
	require.Equal(t, agentcontext.StatusInvalid, got.Status)
	require.Contains(t, got.Error, "kebab-case")
}

func TestResolver_MCPConfigEmitted(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	contents := `{"mcpServers": {"github": {"env": {"GITHUB_TOKEN": "secret-token"}}}}`
	mustWriteFile(t, filepath.Join(dir, ".mcp.json"), contents)

	r := &agentcontext.Resolver{}
	snap := r.Resolve([]agentcontext.ScanRoot{{Path: dir}})

	require.Len(t, snap.Resources, 1)
	got := snap.Resources[0]
	require.Equal(t, agentcontext.KindMCPConfig, got.Kind)
	require.Equal(t, agentcontext.StatusOK, got.Status)
	// The .mcp.json payload is intentionally not shipped:
	// the file can contain secret-bearing Env/Headers values.
	// Only the path + ContentHash are exposed, so consumers
	// can detect changes without ever seeing the bytes.
	require.Empty(t, got.Payload, "readMCPConfig must not include the file payload")
	require.NotEqual(t, [32]byte{}, got.ContentHash, "readMCPConfig must populate ContentHash for change detection")
	require.Equal(t, uint64(len(contents)), got.SizeBytes)
}

// TestResolver_SymlinkInsideScanRootAllowed exercises the
// monorepo case where a top-level AGENTS.md is symlinked to
// shared content elsewhere inside the same workspace tree. The
// target lives under the scan root, so the resolver follows the
// symlink, emits the target bytes, and attributes the resource
// to the resolved target path.
func TestResolver_SymlinkInsideScanRootAllowed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require admin privileges on Windows runners")
	}
	t.Parallel()
	dir := testutil.TempDirResolved(t)
	target := filepath.Join(dir, "docs", "AGENTS.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o755))
	mustWriteFile(t, target, "shared monorepo guidance")
	link := filepath.Join(dir, "AGENTS.md")
	require.NoError(t, os.Symlink(target, link))

	r := &agentcontext.Resolver{}
	snap := r.Resolve([]agentcontext.ScanRoot{{Path: dir}})

	// The nested target is not independently recognized (only the
	// top-level symlink is), so exactly one resource is emitted,
	// carrying the target bytes and attributed to the target.
	require.Len(t, snap.Resources, 1)
	got := snap.Resources[0]
	require.Equal(t, agentcontext.StatusOK, got.Status)
	require.Equal(t, target, got.Source)
	require.Equal(t, "shared monorepo guidance", string(got.Payload))
}

// TestResolver_SymlinkedInstructionFilesDeduplicated reproduces
// the common repo layout where CLAUDE.md and .cursorrules are
// symlinks to a single AGENTS.md. All three resolve to the same
// file, so the resolver must emit one instruction resource
// attributed to the real AGENTS.md rather than three copies of
// identical content.
func TestResolver_SymlinkedInstructionFilesDeduplicated(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require admin privileges on Windows runners")
	}
	t.Parallel()
	dir := testutil.TempDirResolved(t)
	agents := filepath.Join(dir, "AGENTS.md")
	mustWriteFile(t, agents, "the one true guidance")
	require.NoError(t, os.Symlink(agents, filepath.Join(dir, "CLAUDE.md")))
	require.NoError(t, os.Symlink(agents, filepath.Join(dir, ".cursorrules")))

	r := &agentcontext.Resolver{}
	snap := r.Resolve([]agentcontext.ScanRoot{{Path: dir}})

	require.Len(t, snap.Resources, 1)
	got := snap.Resources[0]
	require.Equal(t, agentcontext.KindInstructionFile, got.Kind)
	require.Equal(t, agents, got.Source)
	require.Equal(t, "the one true guidance", string(got.Payload))
}

// TestResolver_InstructionFilesOnlyAtScanRoot verifies the
// resolver does not descend into subdirectories to collect
// nested instruction files, mirroring codex. A nested
// site/AGENTS.md is ignored while the top-level one is kept.
func TestResolver_InstructionFilesOnlyAtScanRoot(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "AGENTS.md"), "root")
	mustWriteFile(t, filepath.Join(dir, "site", "AGENTS.md"), "nested")

	r := &agentcontext.Resolver{}
	snap := r.Resolve([]agentcontext.ScanRoot{{Path: dir}})

	require.Len(t, snap.Resources, 1)
	require.Equal(t, filepath.Join(dir, "AGENTS.md"), snap.Resources[0].Source)
	require.Equal(t, "root", string(snap.Resources[0].Payload))
}

// TestResolver_SymlinkOutsideScanRootRejected guards the
// security boundary. A malicious workspace cannot ship a
// snapshot containing ~/.ssh/id_rsa or /etc/passwd by placing a
// symlink with that target at AGENTS.md, .mcp.json, or
// SKILL.md inside the scan root.
func TestResolver_SymlinkOutsideScanRootRejected(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require admin privileges on Windows runners")
	}
	t.Parallel()
	dir := t.TempDir()
	secretDir := t.TempDir()
	secret := filepath.Join(secretDir, "id_rsa")
	mustWriteFile(t, secret, "-----BEGIN OPENSSH PRIVATE KEY-----")
	link := filepath.Join(dir, "AGENTS.md")
	require.NoError(t, os.Symlink(secret, link))

	r := &agentcontext.Resolver{}
	snap := r.Resolve([]agentcontext.ScanRoot{{Path: dir}})

	require.Len(t, snap.Resources, 1)
	got := snap.Resources[0]
	require.Equal(t, agentcontext.StatusInvalid, got.Status)
	require.Empty(t, got.Payload, "escaping symlink target must not be shipped")
	require.Contains(t, got.Error, "escapes scan root")
}

// TestResolver_BrokenSymlink emits Unreadable for a dangling
// link rather than crashing the walk.
func TestResolver_BrokenSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require admin privileges on Windows runners")
	}
	t.Parallel()
	dir := t.TempDir()
	link := filepath.Join(dir, "AGENTS.md")
	require.NoError(t, os.Symlink(filepath.Join(dir, "does-not-exist"), link))

	r := &agentcontext.Resolver{}
	snap := r.Resolve([]agentcontext.ScanRoot{{Path: dir}})

	require.Len(t, snap.Resources, 1)
	require.Equal(t, agentcontext.StatusUnreadable, snap.Resources[0].Status)
}

func TestResolver_OversizeInstructionFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// Write a file larger than the per-resource cap.
	big := make([]byte, 200)
	for i := range big {
		big[i] = 'a'
	}
	mustWriteFile(t, filepath.Join(dir, "AGENTS.md"), string(big))

	r := &agentcontext.Resolver{MaxResourceBytes: 100}
	snap := r.Resolve([]agentcontext.ScanRoot{{Path: dir}})

	require.Len(t, snap.Resources, 1)
	got := snap.Resources[0]
	require.Equal(t, agentcontext.StatusOversize, got.Status)
	require.Empty(t, got.Payload)
	require.Equal(t, uint64(200), got.SizeBytes)
	// Hash over capped slice is still populated so callers
	// can detect "still oversize but content changed".
	require.NotEqual(t, [32]byte{}, got.ContentHash)
}

func TestResolver_AggregateCapExcludes(t *testing.T) {
	t.Parallel()
	// Instruction files are only read at a scan root's top level,
	// so each contributing file lives at its own scan root.
	dirRoot := t.TempDir()
	dirA := t.TempDir()
	dirB := t.TempDir()
	mustWriteFile(t, filepath.Join(dirRoot, "AGENTS.md"), "small")
	mustWriteFile(t, filepath.Join(dirA, "AGENTS.md"), "AAAA")
	mustWriteFile(t, filepath.Join(dirB, "AGENTS.md"), "BBBB")

	// Aggregate cap of 9 bytes lets two of the three (5+4) bytes
	// through but excludes the third regardless of order.
	r := &agentcontext.Resolver{MaxSnapshotBytes: 9}
	snap := r.Resolve([]agentcontext.ScanRoot{
		{Path: dirRoot},
		{Path: dirA},
		{Path: dirB},
	})

	var excluded int
	for _, res := range snap.Resources {
		if res.Status == agentcontext.StatusExcluded {
			excluded++
		}
	}
	require.Equal(t, 1, excluded)
}

func TestResolver_CountCapExcludes(t *testing.T) {
	t.Parallel()
	// Instruction files are only read at a scan root's top level,
	// so spread the five files across five scan roots.
	roots := make([]agentcontext.ScanRoot, 0, 5)
	for i := 0; i < 5; i++ {
		d := t.TempDir()
		mustWriteFile(t, filepath.Join(d, "AGENTS.md"), "x")
		roots = append(roots, agentcontext.ScanRoot{Path: d})
	}

	r := &agentcontext.Resolver{MaxResources: 3}
	snap := r.Resolve(roots)

	require.Len(t, snap.Resources, 5)
	var excluded int
	for _, res := range snap.Resources {
		if res.Status == agentcontext.StatusExcluded {
			excluded++
		}
	}
	require.Equal(t, 2, excluded)
}

// TestResolver_MCPConfigOnlyAtScanRoot verifies that .mcp.json is
// recognized only at a scan root's top level. A nested config is
// ignored because the resolver no longer walks the tree.
func TestResolver_MCPConfigOnlyAtScanRoot(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, ".mcp.json"), `{"mcpServers": {}}`)
	mustWriteFile(t, filepath.Join(dir, "sub", ".mcp.json"), `{"mcpServers": {}}`)

	r := &agentcontext.Resolver{}
	snap := r.Resolve([]agentcontext.ScanRoot{{Path: dir}})

	require.Len(t, snap.Resources, 1)
	require.Equal(t, agentcontext.KindMCPConfig, snap.Resources[0].Kind)
	require.Equal(t, filepath.Join(dir, ".mcp.json"), snap.Resources[0].Source)
}

// TestResolver_SkillsOnlyFromFixedContainers verifies skills are
// discovered from the fixed container locations (skills,
// .agents/skills, .claude/skills, .codex/skills) and never from an
// arbitrary skills/ directory nested elsewhere in the tree.
func TestResolver_SkillsOnlyFromFixedContainers(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustWriteSkill(t, filepath.Join(dir, "skills"), "water-plants", "p")
	mustWriteSkill(t, filepath.Join(dir, ".agents", "skills"), "make-coffee", "c")
	mustWriteSkill(t, filepath.Join(dir, ".claude", "skills"), "fold-laundry", "l")
	mustWriteSkill(t, filepath.Join(dir, ".codex", "skills"), "walk-dog", "d")
	// A skills/ directory buried under an arbitrary path is not a
	// fixed container location and must be ignored.
	mustWriteSkill(t, filepath.Join(dir, "pkg", "skills"), "buried", "b")

	r := &agentcontext.Resolver{}
	snap := r.Resolve([]agentcontext.ScanRoot{{Path: dir}})

	var names []string
	for _, res := range snap.Resources {
		require.Equal(t, agentcontext.KindSkill, res.Kind)
		names = append(names, filepath.Base(res.Source))
	}
	require.ElementsMatch(t,
		[]string{"water-plants", "make-coffee", "fold-laundry", "walk-dog"}, names)
}

func TestResolver_UserSourceAttribution(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "AGENTS.md"), "user-added")

	r := &agentcontext.Resolver{}
	snap := r.Resolve([]agentcontext.ScanRoot{{Path: dir, UserSource: dir}})

	require.Len(t, snap.Resources, 1)
	require.Equal(t, dir, snap.Resources[0].SourcePath)
}

func TestResolver_MissingRootSilentlyIgnored(t *testing.T) {
	t.Parallel()
	r := &agentcontext.Resolver{}
	snap := r.Resolve([]agentcontext.ScanRoot{{Path: "/nonexistent/path"}})
	require.Empty(t, snap.Resources)
	require.Empty(t, snap.SnapshotError)
}

func TestResolver_SingleFileRootClassified(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")
	mustWriteFile(t, path, "x")

	r := &agentcontext.Resolver{}
	snap := r.Resolve([]agentcontext.ScanRoot{{Path: path}})

	require.Len(t, snap.Resources, 1)
	require.Equal(t, agentcontext.KindInstructionFile, snap.Resources[0].Kind)
}

func TestResolver_DuplicateRootsDeduplicated(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "AGENTS.md"), "x")

	r := &agentcontext.Resolver{}
	snap := r.Resolve([]agentcontext.ScanRoot{
		{Path: dir},
		{Path: dir},
		{Path: dir},
	})
	require.Len(t, snap.Resources, 1)
}

func TestResolver_MCPResources(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	mcpRes := agentcontext.Resource{
		ID:          "mcp_server:github",
		Kind:        agentcontext.KindMCPServer,
		Source:      "github",
		Status:      agentcontext.StatusOK,
		Payload:     []byte("tool-list-json"),
		ContentHash: sha256.Sum256([]byte("tool-list-json")),
		Description: "GitHub MCP server",
	}
	r := &agentcontext.Resolver{
		MCPResources: func() []agentcontext.Resource { return []agentcontext.Resource{mcpRes} },
	}

	snap := r.Resolve([]agentcontext.ScanRoot{{Path: dir}})
	got := findResource(t, snap.Resources, agentcontext.KindMCPServer, "github")
	require.Equal(t, agentcontext.StatusOK, got.Status)
	require.Equal(t, "GitHub MCP server", got.Description)
}

// TestResolver_MCPResourcesRespectAggregateByteCap guards the
// contract that a single oversized MCP payload cannot blow past
// MaxSnapshotBytes with StatusOK.
func TestResolver_MCPResourcesRespectAggregateByteCap(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	big := make([]byte, 1024)
	for i := range big {
		big[i] = 'x'
	}
	mcpRes := agentcontext.Resource{
		ID:          "mcp_server:big",
		Kind:        agentcontext.KindMCPServer,
		Source:      "big",
		Status:      agentcontext.StatusOK,
		Payload:     big,
		ContentHash: sha256.Sum256(big),
	}
	r := &agentcontext.Resolver{
		MaxSnapshotBytes: 512,
		MCPResources:     func() []agentcontext.Resource { return []agentcontext.Resource{mcpRes} },
	}

	snap := r.Resolve([]agentcontext.ScanRoot{{Path: dir}})
	got := findResource(t, snap.Resources, agentcontext.KindMCPServer, "big")
	require.Equal(t, agentcontext.StatusExcluded, got.Status,
		"MCP payload exceeding MaxSnapshotBytes must be excluded")
	require.Empty(t, got.Payload)
	require.NotEmpty(t, snap.SnapshotError, "snapshot must surface the cap breach")
}

// TestResolver_MCPExcludedFromAggregateHash verifies that MCP resources
// (config and live servers) are carried in the snapshot but excluded
// from the drift/aggregate hash, so an MCP server connecting (or its
// tools changing) does not flip already-hydrated chats to dirty.
func TestResolver_MCPExcludedFromAggregateHash(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// An instruction file provides drift-relevant pinned content.
	mustWriteFile(t, filepath.Join(dir, "AGENTS.md"), "workspace rules")

	base := (&agentcontext.Resolver{}).Resolve([]agentcontext.ScanRoot{{Path: dir}})

	mcpRes := agentcontext.Resource{
		ID:          "mcp_server:github",
		Kind:        agentcontext.KindMCPServer,
		Source:      "github",
		Name:        "github",
		Status:      agentcontext.StatusOK,
		ContentHash: sha256.Sum256([]byte("tool-list")),
		Tools:       []agentcontext.MCPTool{{Name: "search"}},
	}
	withMCP := (&agentcontext.Resolver{
		MCPResources: func() []agentcontext.Resource { return []agentcontext.Resource{mcpRes} },
	}).Resolve([]agentcontext.ScanRoot{{Path: dir}})

	// The MCP server resource is present in the snapshot...
	got := findResource(t, withMCP.Resources, agentcontext.KindMCPServer, "github")
	require.Len(t, got.Tools, 1)
	// ...but does not change the drift/aggregate hash.
	require.Equal(t, base.AggregateHash, withMCP.AggregateHash,
		"MCP resources must not participate in the drift hash")
}

// TestResolver_UnreadableInstructionFile verifies the
// permission-denied walk path produces a StatusUnreadable
// resource classified with the correct kind, matching the
// classification the resolver would emit on a successful read.
func TestResolver_UnreadableInstructionFile(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("file mode 0o000 does not deny reads on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses file mode permissions")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")
	mustWriteFile(t, path, "hello")
	require.NoError(t, os.Chmod(path, 0o000))
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })

	r := &agentcontext.Resolver{}
	snap := r.Resolve([]agentcontext.ScanRoot{{Path: dir}})

	require.Len(t, snap.Resources, 1)
	got := snap.Resources[0]
	require.Equal(t, agentcontext.KindInstructionFile, got.Kind)
	require.Equal(t, agentcontext.StatusUnreadable, got.Status)
	require.NotEmpty(t, got.Error)
}

// TestResolver_UnreadableMCPConfig confirms the walk-error path
// uses the file's real kind, not a hardcoded fallback. Without
// this, a permission flip on .mcp.json would produce a phantom
// resource ID swap when the permission is later restored.
func TestResolver_UnreadableMCPConfig(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("file mode 0o000 does not deny reads on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses file mode permissions")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, ".mcp.json")
	mustWriteFile(t, path, `{"mcpServers": {}}`)
	require.NoError(t, os.Chmod(path, 0o000))
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })

	r := &agentcontext.Resolver{}
	snap := r.Resolve([]agentcontext.ScanRoot{{Path: dir}})

	require.Len(t, snap.Resources, 1)
	got := snap.Resources[0]
	require.Equal(t, agentcontext.KindMCPConfig, got.Kind)
	require.Equal(t, agentcontext.StatusUnreadable, got.Status)
	require.NotEmpty(t, got.Error)
}

func TestResourceKindString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		kind agentcontext.ResourceKind
		want string
	}{
		{agentcontext.KindUnspecified, "unknown"},
		{agentcontext.KindInstructionFile, "instruction_file"},
		{agentcontext.KindSkill, "skill"},
		{agentcontext.KindMCPConfig, "mcp_config"},
		{agentcontext.KindMCPServer, "mcp_server"},
		{agentcontext.KindPlugin, "plugin"},
		{agentcontext.KindHook, "hook"},
		{agentcontext.KindSubagent, "subagent"},
		{agentcontext.KindCommand, "command"},
		{agentcontext.ResourceKind(999), "unknown"},
	}
	for _, tt := range tests {
		require.Equal(t, tt.want, tt.kind.String())
	}
}

func TestResourceStatusString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		status agentcontext.ResourceStatus
		want   string
	}{
		{agentcontext.StatusOK, "ok"},
		{agentcontext.StatusOversize, "oversize"},
		{agentcontext.StatusUnreadable, "unreadable"},
		{agentcontext.StatusInvalid, "invalid"},
		{agentcontext.StatusExcluded, "excluded"},
		{agentcontext.ResourceStatus(999), "unknown"},
	}
	for _, tt := range tests {
		require.Equal(t, tt.want, tt.status.String())
	}
}

func TestComputeAggregateHash_DeterministicAcrossOrder(t *testing.T) {
	t.Parallel()
	a := agentcontext.Resource{
		ID:     "instruction_file:/a/AGENTS.md",
		Kind:   agentcontext.KindInstructionFile,
		Source: "/a/AGENTS.md",
		Status: agentcontext.StatusOK,
	}
	b := agentcontext.Resource{
		ID:     "instruction_file:/b/AGENTS.md",
		Kind:   agentcontext.KindInstructionFile,
		Source: "/b/AGENTS.md",
		Status: agentcontext.StatusOK,
	}
	got1 := agentcontext.ComputeAggregateHash([]agentcontext.Resource{a, b})
	got2 := agentcontext.ComputeAggregateHash([]agentcontext.Resource{b, a})
	require.Equal(t, got1, got2)
}

func TestComputeAggregateHash_ChangesOnContent(t *testing.T) {
	t.Parallel()
	base := agentcontext.Resource{
		ID:     "instruction_file:/a/AGENTS.md",
		Kind:   agentcontext.KindInstructionFile,
		Source: "/a/AGENTS.md",
		Status: agentcontext.StatusOK,
	}
	hash1 := agentcontext.ComputeAggregateHash([]agentcontext.Resource{base})

	withContent := base
	withContent.ContentHash = [32]byte{0x01}
	hash2 := agentcontext.ComputeAggregateHash([]agentcontext.Resource{withContent})
	require.NotEqual(t, hash1, hash2)

	withStatus := base
	withStatus.Status = agentcontext.StatusOversize
	hash3 := agentcontext.ComputeAggregateHash([]agentcontext.Resource{withStatus})
	require.NotEqual(t, hash1, hash3)
}

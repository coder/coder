package agentcontext_test

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentcontext"
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

func TestResolver_CaseInsensitiveInstructionNames(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "agents.md"), "lower\n")
	mustWriteFile(t, filepath.Join(dir, "CLAUDE.md"), "claude\n")

	r := &agentcontext.Resolver{}
	snap := r.Resolve([]agentcontext.ScanRoot{{Path: dir}})

	require.Len(t, snap.Resources, 2)
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
	mustWriteFile(t, filepath.Join(dir, ".mcp.json"), `{"mcpServers": {}}`)

	r := &agentcontext.Resolver{}
	snap := r.Resolve([]agentcontext.ScanRoot{{Path: dir}})

	require.Len(t, snap.Resources, 1)
	require.Equal(t, agentcontext.KindMCPConfig, snap.Resources[0].Kind)
	require.Equal(t, agentcontext.StatusOK, snap.Resources[0].Status)
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
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "AGENTS.md"), "small")
	subA := filepath.Join(dir, "a")
	subB := filepath.Join(dir, "b")
	mustWriteFile(t, filepath.Join(subA, "AGENTS.md"), "AAAA")
	mustWriteFile(t, filepath.Join(subB, "AGENTS.md"), "BBBB")

	// Aggregate cap of 9 bytes lets the first two through but
	// excludes the third regardless of which order they
	// appear.
	r := &agentcontext.Resolver{MaxSnapshotBytes: 9}
	snap := r.Resolve([]agentcontext.ScanRoot{{Path: dir}})

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
	dir := t.TempDir()
	for i := 0; i < 5; i++ {
		sub := filepath.Join(dir, "dir", string('a'+rune(i)))
		mustWriteFile(t, filepath.Join(sub, "AGENTS.md"), "x")
	}

	r := &agentcontext.Resolver{MaxResources: 3}
	snap := r.Resolve([]agentcontext.ScanRoot{{Path: dir}})

	require.Len(t, snap.Resources, 5)
	var excluded int
	for _, res := range snap.Resources {
		if res.Status == agentcontext.StatusExcluded {
			excluded++
		}
	}
	require.Equal(t, 2, excluded)
}

func TestResolver_SkipsVendorAndNodeModules(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "AGENTS.md"), "root")
	mustWriteFile(t, filepath.Join(dir, "node_modules", "deep", "AGENTS.md"), "should not appear")
	mustWriteFile(t, filepath.Join(dir, "vendor", "AGENTS.md"), "should not appear either")

	r := &agentcontext.Resolver{}
	snap := r.Resolve([]agentcontext.ScanRoot{{Path: dir}})

	require.Len(t, snap.Resources, 1)
	require.Equal(t, filepath.Join(dir, "AGENTS.md"), snap.Resources[0].Source)
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

func TestResolver_MCPProviderResources(t *testing.T) {
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
		MCP: &fakeMCPProvider{resources: []agentcontext.Resource{mcpRes}},
	}

	snap := r.Resolve([]agentcontext.ScanRoot{{Path: dir}})
	got := findResource(t, snap.Resources, agentcontext.KindMCPServer, "github")
	require.Equal(t, agentcontext.StatusOK, got.Status)
	require.Equal(t, "GitHub MCP server", got.Description)
}

type fakeMCPProvider struct {
	resources []agentcontext.Resource
}

func (f *fakeMCPProvider) MCPResources() []agentcontext.Resource {
	return f.resources
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

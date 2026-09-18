package chatd

import (
	"context"
	"fmt"
	"path"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/encoding/protojson"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	"github.com/coder/coder/v2/testutil"
)

func TestTouchedPaths(t *testing.T) {
	t.Parallel()

	call := func(id, name, input string) fantasy.ToolCallContent {
		return fantasy.ToolCallContent{ToolCallID: id, ToolName: name, Input: input}
	}
	result := func(id string) fantasy.Content {
		return fantasy.ToolResultContent{ToolCallID: id}
	}

	calls := []fantasy.ToolCallContent{
		call("read", "read_file", `{"path":"/home/coder/project/site/src/App.tsx"}`),
		call("write", "write_file", `{"path":" /home/coder/project/docs/../README.md ","content":"x"}`),
		call("edit", "edit_files", `{"files":[{"path":"/tmp/repo/a.go","edits":[]},{"path":"relative/b.go","edits":[]}]}`),
		call("exec", "execute", `{"command":"ls","workdir":"/tmp/repo/pkg"}`),
		call("exec-default", "execute", `{"command":"ls"}`),
		call("denied", "read_file", `{"path":"/never/ran.txt"}`),
		call("other", "process_list", `{}`),
		call("bad", "read_file", `{not json`),
	}
	results := []fantasy.Content{
		result("read"), result("write"), result("edit"), result("exec"), result("exec-default"), result("other"), result("bad"),
	}

	files, dirs := touchedPaths(calls, results)
	require.Equal(t, []string{
		"/home/coder/project/site/src/App.tsx",
		"/home/coder/project/README.md",
		"/tmp/repo/a.go",
	}, files, "paths are trimmed like the file tools do and cleaned; relative paths and calls without a result are ignored")
	require.Equal(t, []string{"/tmp/repo/pkg"}, dirs, "only an explicit workdir counts")

	// A Windows agent reports drive-rooted paths with backslashes; they
	// are normalized to forward slashes so the same logic applies.
	files, dirs = touchedPaths([]fantasy.ToolCallContent{
		call("win-read", "read_file", `{"path":"C:\\repo\\site\\App.tsx"}`),
		call("win-exec", "execute", `{"command":"dir","workdir":"C:\\repo\\pkg"}`),
		call("win-rel", "read_file", `{"path":"repo\\App.tsx"}`),
		call("unc-read", "read_file", `{"path":"\\\\server\\share\\repo\\App.tsx"}`),
		call("unc-exec", "execute", `{"command":"dir","workdir":"//server/share/repo"}`),
	}, []fantasy.Content{result("win-read"), result("win-exec"), result("win-rel"), result("unc-read"), result("unc-exec")})
	require.Equal(t, []string{"C:/repo/site/App.tsx"}, files, "a UNC path is not probed")
	require.Equal(t, []string{"C:/repo/pkg"}, dirs)

	// On POSIX a backslash is an ordinary character in a name and stays one.
	files, _ = touchedPaths([]fantasy.ToolCallContent{
		call("posix-read", "read_file", `{"path":"/repo/dir\\name/file.go"}`),
	}, []fantasy.Content{result("posix-read")})
	require.Equal(t, []string{"/repo/dir\\name/file.go"}, files)
}

func TestCandidateInstructionDirs(t *testing.T) {
	t.Parallel()

	const workingDir = "/home/coder/project"

	t.Run("NestedWalksUpToWorkingDir", func(t *testing.T) {
		t.Parallel()
		got := candidateInstructionDirs([]string{"/home/coder/project/site/src/App.tsx"}, nil, workingDir)
		require.Equal(t, []string{"/home/coder/project/site", "/home/coder/project/site/src"}, got)
	})

	t.Run("WorkingDirAndAncestorsExcluded", func(t *testing.T) {
		t.Parallel()
		got := candidateInstructionDirs([]string{"/home/coder/project/AGENTS.md", "/home/coder/notes.txt", "/etc/hosts"}, nil, workingDir)
		require.Equal(t, []string{"/etc"}, got)
	})

	t.Run("WindowsDriveRoot", func(t *testing.T) {
		t.Parallel()
		got := candidateInstructionDirs([]string{"C:/repo/site/src/App.tsx", "D:/other/x/y.txt"}, nil, "C:\\repo")
		require.Equal(t, []string{"D:/other", "C:/repo/site", "D:/other/x", "C:/repo/site/src"}, got, "the walk stops at the drive root and below the working directory")

		// Windows file systems are case-insensitive, so a differently cased
		// drive or directory is still inside the working directory, and two
		// spellings of one directory are one candidate.
		got = candidateInstructionDirs([]string{"c:/Repo/site/src/App.tsx", "C:/repo/SITE/a.ts"}, nil, "C:\\repo")
		require.Equal(t, []string{"c:/Repo/site", "c:/Repo/site/src"}, got)
	})

	t.Run("OutsideWorkingDirStopsBelowRoot", func(t *testing.T) {
		t.Parallel()
		got := candidateInstructionDirs(nil, []string{"/tmp/repo/pkg"}, workingDir)
		require.Equal(t, []string{"/tmp", "/tmp/repo", "/tmp/repo/pkg"}, got)
	})

	t.Run("DedupesAndOrdersShallowFirst", func(t *testing.T) {
		t.Parallel()
		got := candidateInstructionDirs(
			[]string{"/home/coder/project/site/a.ts", "/home/coder/project/site/b.ts", "/home/coder/project/cli/main.go"},
			[]string{"/home/coder/project/site"},
			workingDir,
		)
		require.Equal(t, []string{"/home/coder/project/cli", "/home/coder/project/site"}, got)
	})

	t.Run("DeepPathKeepsEveryAncestor", func(t *testing.T) {
		t.Parallel()
		deep := workingDir + "/site"
		for range 40 {
			deep += "/d"
		}
		got := candidateInstructionDirs([]string{deep + "/f"}, nil, workingDir)
		require.Len(t, got, 41)
		require.Equal(t, workingDir+"/site", got[0], "the highest nested scope survives, and sorts first for the request cap")
	})

	t.Run("NoRequestCap", func(t *testing.T) {
		t.Parallel()
		var dirs []string
		for i := range 50 {
			dirs = append(dirs, "/tmp/"+string(rune('a'+i%26))+string(rune('a'+i/26)))
		}
		got := candidateInstructionDirs(nil, dirs, workingDir)
		require.Len(t, got, 51, "candidates are batched at request time, not truncated here")
		require.Equal(t, "/tmp", got[0], "the shared parent sorts first")
	})
}

func TestSelectInstructionProbes(t *testing.T) {
	t.Parallel()

	// Writing a rule file or running a command in a directory makes it
	// stale: it is probed again even when it already holds a pinned file
	// or was recently found empty.
	stale := staleInstructionDirs(
		[]string{"/repo/site/CLAUDE.md", "/repo/site/src/App.tsx"},
		[]string{"/repo/pkg"},
	)
	require.Equal(t, map[string]struct{}{"/repo/site": {}, "/repo/pkg": {}}, stale)
	// Windows spells the recognized names in any case; POSIX does not.
	require.Equal(t, map[string]struct{}{"c:/repo/site": {}}, staleInstructionDirs([]string{"C:/repo/site/agents.md", "/repo/docs/agents.md"}, nil))

	pinned := map[string]struct{}{"/repo/site": {}, "/repo/docs": {}}
	negative := func(dir string) bool { return dir == "/repo/pkg" || dir == "/repo/cmd" }
	got := selectInstructionProbes(
		[]string{"/repo/site", "/repo/site/src", "/repo/docs", "/repo/pkg", "/repo/cmd", "/repo/lib"},
		pinned, stale, negative,
	)
	require.Equal(t, []string{"/repo/site", "/repo/site/src", "/repo/pkg", "/repo/lib"}, got)

	// A pinned Windows directory matches however the tool spelled it.
	got = selectInstructionProbes([]string{"c:/Repo/site", "C:/repo/docs"}, map[string]struct{}{"c:/repo/site": {}}, nil, func(string) bool { return false })
	require.Equal(t, []string{"C:/repo/docs"}, got)
}

func TestRemovedDiscoveredSources(t *testing.T) {
	t.Parallel()

	rows := []database.ChatContextResource{
		{Source: "/repo/site/AGENTS.md", Discovered: true},
		{Source: "/repo/site/CLAUDE.md", Discovered: true},
		{Source: "/repo/pkg/AGENTS.md", Discovered: true},
		{Source: "/repo/lib/AGENTS.md", Discovered: true},
		{Source: "/repo/docs/AGENTS.md", Discovered: true},
		{Source: "/repo/AGENTS.md", Discovered: false},
		{Source: "C:\\repo\\win\\AGENTS.md", Discovered: true},
	}
	stale := staleInstructionDirs([]string{"/repo/site/CLAUDE.md", "/repo/AGENTS.md"}, []string{"/repo/pkg", "/repo/lib", "c:/repo/win"})
	probed := []string{"/repo/site", "/repo/pkg", "/repo/docs", "C:/repo/win"}
	returned := map[string]struct{}{"/repo/site/CLAUDE.md": {}}

	// site: AGENTS.md vanished, CLAUDE.md was returned. pkg: stale and probed,
	// nothing returned. lib: stale but its batch failed, so it is kept. docs:
	// probed but not stale, so a missing answer is not evidence. The root
	// row is a snapshot copy, never touched. The Windows row matches its
	// directory case-insensitively.
	require.Equal(t, []string{"/repo/site/AGENTS.md", "/repo/pkg/AGENTS.md", "C:\\repo\\win\\AGENTS.md"}, removedDiscoveredSources(rows, stale, probed, returned))
}

// TestReconcileDiscoveredInstructionFilesKeepsSpelling checks that a file
// the chat already pins under another spelling of a Windows path updates
// that row instead of adding a second one, and that a vanished file in a
// stale directory is removed.
func TestReconcileDiscoveredInstructionFilesKeepsSpelling(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	chatID := uuid.New()
	rows := []database.ChatContextResource{
		{Source: "C:\\repo\\site\\AGENTS.md", BodyKind: database.WorkspaceAgentContextBodyKindInstructionFile, Discovered: true},
		{Source: "C:\\repo\\site\\CLAUDE.md", BodyKind: database.WorkspaceAgentContextBodyKindInstructionFile, Discovered: true},
	}
	resolved := []workspacesdk.ContextInstructionFile{{
		Directory: "c:/Repo/site", Source: "c:/Repo/site/AGENTS.md", Content: "rules", ContentHash: "ab", SizeBytes: 5, Status: "ok",
	}}
	db.EXPECT().UpsertChatContextDiscoveredResource(gomock.Any(), gomock.Cond(func(arg database.UpsertChatContextDiscoveredResourceParams) bool {
		return arg.ChatID == chatID && arg.Source == "C:\\repo\\site\\AGENTS.md"
	})).Return(nil)
	db.EXPECT().DeleteChatContextDiscoveredResource(gomock.Any(), database.DeleteChatContextDiscoveredResourceParams{ChatID: chatID, Source: "C:\\repo\\site\\CLAUDE.md"}).Return(nil)

	stale := map[string]struct{}{"c:/repo/site": {}}
	result, err := reconcileDiscoveredInstructionFiles(context.Background(), db, chatID, rows, resolved, stale, []string{"c:/Repo/site"}, discoveryBudget{})
	require.NoError(t, err)
	require.Equal(t, 1, result.pinned)
	require.Equal(t, 1, result.removed)
}

// TestUnchangedDiscoveredRows checks that a rediscovery applied after its
// probe skips every source a step pinned in between: a captured row whose
// content changed, one that vanished, and a file the step discovered first.
func TestUnchangedDiscoveredRows(t *testing.T) {
	t.Parallel()

	row := func(source string, hash byte, discovered bool) database.ChatContextResource {
		return database.ChatContextResource{Source: source, ContentHash: []byte{hash}, Discovered: discovered, BodyKind: database.WorkspaceAgentContextBodyKindInstructionFile, Status: database.WorkspaceAgentContextResourceStatusOk, SizeBytes: int64(hash) * 10}
	}
	file := func(source string) workspacesdk.ContextInstructionFile {
		return workspacesdk.ContextInstructionFile{Directory: path.Dir(source), Source: source, Status: "ok"}
	}
	// The excluded capture keeps the hash of the content it could not hold,
	// so a step that later pins the content changes status and body only.
	excluded := row("/repo/site/pkg/AGENTS.md", 4, true)
	excluded.Status = database.WorkspaceAgentContextResourceStatusExcluded
	captured := []database.ChatContextResource{
		row("/repo/site/AGENTS.md", 1, true),
		row("/repo/site/CLAUDE.md", 1, true),
		row("/repo/docs/AGENTS.md", 1, true),
		excluded,
	}
	current := []database.ChatContextResource{
		row("/repo/AGENTS.md", 9, false),
		row("/repo/site/AGENTS.md", 1, true),
		row("/repo/site/CLAUDE.md", 2, true),
		row("/repo/site/.cursorrules", 3, true),
		row("/repo/site/pkg/AGENTS.md", 4, true),
	}
	resolved := []workspacesdk.ContextInstructionFile{
		file("/repo/site/AGENTS.md"),
		file("/repo/site/CLAUDE.md"),
		file("/repo/site/.cursorrules"),
		file("/repo/docs/AGENTS.md"),
		file("/repo/lib/AGENTS.md"),
		file("/repo/site/pkg/AGENTS.md"),
	}

	rows, files, reserved := unchangedDiscoveredRows(captured, current, resolved)
	sources := func(rows []database.ChatContextResource) []string {
		out := make([]string, 0, len(rows))
		for _, r := range rows {
			out = append(out, r.Source)
		}
		return out
	}
	require.Equal(t, []string{"/repo/AGENTS.md", "/repo/site/AGENTS.md"}, sources(rows),
		"snapshot rows and the unchanged discovered row stay; the rewritten, the step-discovered, and the excluded-then-pinned rows are off limits")
	require.Equal(t, []string{"/repo/site/AGENTS.md", "/repo/lib/AGENTS.md"}, func() []string {
		out := make([]string, 0, len(files))
		for _, f := range files {
			out = append(out, f.Source)
		}
		return out
	}(), "only an unchanged row or a source nobody holds may be written; a rewritten row, a step-discovered file, and a vanished capture are skipped")
	require.Equal(t, discoveryBudget{bytes: 90, files: 3}, reserved,
		"the rows a step owns still count toward the chat's caps, so the rediscovery cannot hand their share out again")
}

// TestResolveInstructionDirsBatches checks that a probe larger than one
// request is split at the agent limit and that a failed batch keeps the
// answers and the coverage of the batches before it.
func TestResolveInstructionDirsBatches(t *testing.T) {
	t.Parallel()

	dirs := make([]string, 0, 40)
	for i := range 40 {
		dirs = append(dirs, "/tmp/"+string(rune('a'+i%26))+string(rune('a'+i/26)))
	}
	found := workspacesdk.ContextInstructionFile{Directory: dirs[0], Source: dirs[0] + "/AGENTS.md", Status: "ok"}

	ctrl := gomock.NewController(t)
	conn := agentconnmock.NewMockAgentConn(ctrl)
	conn.EXPECT().ResolveContextInstructions(gomock.Any(), workspacesdk.ResolveContextInstructionsRequest{Directories: dirs[:32]}).
		Return(workspacesdk.ResolveContextInstructionsResponse{Files: []workspacesdk.ContextInstructionFile{found}}, nil)
	conn.EXPECT().ResolveContextInstructions(gomock.Any(), workspacesdk.ResolveContextInstructionsRequest{Directories: dirs[32:]}).
		Return(workspacesdk.ResolveContextInstructionsResponse{}, xerrors.New("agent gone"))

	files, probed := resolveInstructionDirs(context.Background(), testutil.Logger(t), conn, dirs)
	require.Equal(t, []workspacesdk.ContextInstructionFile{found}, files)
	require.Equal(t, dirs[:32], probed, "only the directories the agent answered for count as probed")
}

func TestInstructionProbeCache(t *testing.T) {
	t.Parallel()

	var cache instructionProbeCache
	agentID := uuid.New()
	now := time.Now()

	require.False(t, cache.negative(now, agentID, "/tmp"))
	cache.markNegative(now, agentID, []string{"/tmp", "/tmp/repo"})
	require.True(t, cache.negative(now, agentID, "/tmp"))
	require.False(t, cache.negative(now, uuid.New(), "/tmp"), "negatives are per agent")
	require.False(t, cache.negative(now.Add(instructionProbeTTL), agentID, "/tmp"), "entries expire")

	cache.forget(agentID, []string{"/tmp/repo"})
	require.False(t, cache.negative(now, agentID, "/tmp/repo"))
	require.True(t, cache.negative(now, agentID, "/tmp"))

	cache.markNegative(now, agentID, []string{"C:/Repo/site"})
	require.True(t, cache.negative(now, agentID, "c:/repo/site"), "Windows directories match case-insensitively")
	cache.forget(agentID, []string{"c:/REPO/site"})
	require.False(t, cache.negative(now, agentID, "C:/Repo/site"))

	// Filling the cache past its bound resets it instead of growing.
	many := make([]string, 0, maxInstructionProbeEntries)
	for i := range maxInstructionProbeEntries {
		many = append(many, "/bulk/"+uuid.NewString()+string(rune('a'+i%26)))
	}
	cache.markNegative(now, agentID, many)
	require.LessOrEqual(t, len(cache.entries), maxInstructionProbeEntries)
	require.False(t, cache.negative(now, agentID, "/tmp"))
}

// TestReconcileDiscoveredInstructionFilesBudget checks the per-chat cap on
// readable discovered content: a re-read of a pinned file swaps its bytes
// rather than adding to them, and a file the remaining budget cannot hold
// is pinned as excluded, without content, instead of being dropped.
func TestReconcileDiscoveredInstructionFilesBudget(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	chatID := uuid.New()
	const held = maxDiscoveredInstructionBytes - 16
	rows := []database.ChatContextResource{{
		Source: "/repo/site/AGENTS.md", BodyKind: database.WorkspaceAgentContextBodyKindInstructionFile,
		Discovered: true, Status: database.WorkspaceAgentContextResourceStatusOk, SizeBytes: held,
	}}
	file := func(source string, size uint64) workspacesdk.ContextInstructionFile {
		return workspacesdk.ContextInstructionFile{Directory: path.Dir(source), Source: source, Content: "rules", ContentHash: "ab", SizeBytes: size, Status: "ok"}
	}
	resolved := []workspacesdk.ContextInstructionFile{
		file("/repo/site/AGENTS.md", held),
		file("/repo/site/CLAUDE.md", 16),
		file("/repo/site/docs/AGENTS.md", 1),
	}
	upserted := make(map[string]database.UpsertChatContextDiscoveredResourceParams)
	db.EXPECT().UpsertChatContextDiscoveredResource(gomock.Any(), gomock.Any()).Times(3).
		DoAndReturn(func(_ context.Context, arg database.UpsertChatContextDiscoveredResourceParams) error {
			upserted[arg.Source] = arg
			return nil
		})

	result, err := reconcileDiscoveredInstructionFiles(context.Background(), db, chatID, rows, resolved, map[string]struct{}{}, []string{"/repo/site", "/repo/site/docs"}, discoveryBudget{})
	require.NoError(t, err)
	require.Equal(t, 3, result.pinned)
	require.Zero(t, result.removed)
	require.Equal(t, database.WorkspaceAgentContextResourceStatusOk, upserted["/repo/site/AGENTS.md"].Status, "a re-read replaces the held bytes")
	require.Equal(t, database.WorkspaceAgentContextResourceStatusOk, upserted["/repo/site/CLAUDE.md"].Status, "the remaining budget holds the second file exactly")
	over := upserted["/repo/site/docs/AGENTS.md"]
	require.Equal(t, database.WorkspaceAgentContextResourceStatusExcluded, over.Status)
	require.Contains(t, over.Error, "cap")
	var body agentproto.InstructionFileBody
	require.NoError(t, protojson.Unmarshal(over.Body, &body))
	require.Empty(t, body.Content, "an excluded file is pinned without its content")
}

// TestReconcileDiscoveredInstructionFilesReplacesUnreadable checks that a
// probe reporting a pinned file as no longer readable overwrites the row
// with that status rather than keeping the old body or deleting the row,
// while a status this server does not know leaves the row alone.
func TestReconcileDiscoveredInstructionFilesReplacesUnreadable(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	chatID := uuid.New()
	rows := []database.ChatContextResource{
		{Source: "/repo/site/AGENTS.md", BodyKind: database.WorkspaceAgentContextBodyKindInstructionFile, Discovered: true, Status: database.WorkspaceAgentContextResourceStatusOk, SizeBytes: 5},
		{Source: "/repo/site/CLAUDE.md", BodyKind: database.WorkspaceAgentContextBodyKindInstructionFile, Discovered: true, Status: database.WorkspaceAgentContextResourceStatusOk, SizeBytes: 5},
	}
	resolved := []workspacesdk.ContextInstructionFile{
		{Directory: "/repo/site", Source: "/repo/site/AGENTS.md", Status: "unreadable", Error: "permission denied"},
		{Directory: "/repo/site", Source: "/repo/site/CLAUDE.md", Status: "from-a-newer-agent"},
	}
	db.EXPECT().UpsertChatContextDiscoveredResource(gomock.Any(), gomock.Cond(func(arg database.UpsertChatContextDiscoveredResourceParams) bool {
		var body agentproto.InstructionFileBody
		return arg.Source == "/repo/site/AGENTS.md" && arg.Status == database.WorkspaceAgentContextResourceStatusUnreadable &&
			arg.Error == "permission denied" && protojson.Unmarshal(arg.Body, &body) == nil && len(body.Content) == 0
	})).Return(nil)

	stale := map[string]struct{}{"/repo/site": {}}
	result, err := reconcileDiscoveredInstructionFiles(context.Background(), db, chatID, rows, resolved, stale, []string{"/repo/site"}, discoveryBudget{})
	require.NoError(t, err)
	require.Equal(t, 1, result.pinned)
	require.Zero(t, result.removed, "a file the probe returned is not treated as vanished, whatever its status")
}

// TestReconcileDiscoveredInstructionFilesReleasesVanishedBudget checks that
// a file renamed within a stale directory takes over the bytes its old name
// held: the vanished row is removed before the replacement is classified, so
// the replacement is pinned readable rather than excluded.
func TestReconcileDiscoveredInstructionFilesReleasesVanishedBudget(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	chatID := uuid.New()
	const held = maxDiscoveredInstructionBytes - 1
	rows := []database.ChatContextResource{{
		Source: "/repo/site/AGENTS.md", BodyKind: database.WorkspaceAgentContextBodyKindInstructionFile,
		Discovered: true, Status: database.WorkspaceAgentContextResourceStatusOk, SizeBytes: held,
	}}
	resolved := []workspacesdk.ContextInstructionFile{{
		Directory: "/repo/site", Source: "/repo/site/CLAUDE.md", Content: "rules", ContentHash: "ab", SizeBytes: held, Status: "ok",
	}}
	gomock.InOrder(
		db.EXPECT().DeleteChatContextDiscoveredResource(gomock.Any(), database.DeleteChatContextDiscoveredResourceParams{ChatID: chatID, Source: "/repo/site/AGENTS.md"}).Return(nil),
		db.EXPECT().UpsertChatContextDiscoveredResource(gomock.Any(), gomock.Cond(func(arg database.UpsertChatContextDiscoveredResourceParams) bool {
			return arg.Source == "/repo/site/CLAUDE.md" && arg.Status == database.WorkspaceAgentContextResourceStatusOk
		})).Return(nil),
	)

	stale := map[string]struct{}{"/repo/site": {}}
	result, err := reconcileDiscoveredInstructionFiles(context.Background(), db, chatID, rows, resolved, stale, []string{"/repo/site"}, discoveryBudget{})
	require.NoError(t, err)
	require.Equal(t, 1, result.pinned)
	require.Equal(t, 1, result.removed)
}

// TestReconcileDiscoveredInstructionFilesRowCap checks that a chat at the
// discovered row cap still refreshes the sources it holds and takes over a
// vanished row's slot, but pins no further new source and reports the
// directories it kept out.
func TestReconcileDiscoveredInstructionFilesRowCap(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	chatID := uuid.New()
	rows := make([]database.ChatContextResource, 0, maxDiscoveredInstructionFiles+1)
	for i := range maxDiscoveredInstructionFiles {
		rows = append(rows, database.ChatContextResource{
			Source: fmt.Sprintf("/repo/pkg%03d/AGENTS.md", i), BodyKind: database.WorkspaceAgentContextBodyKindInstructionFile,
			Discovered: true, Status: database.WorkspaceAgentContextResourceStatusExcluded,
		})
	}
	rows = append(rows, database.ChatContextResource{Source: "/repo/AGENTS.md", BodyKind: database.WorkspaceAgentContextBodyKindInstructionFile})
	file := func(source string) workspacesdk.ContextInstructionFile {
		return workspacesdk.ContextInstructionFile{Directory: path.Dir(source), Source: source, Content: "rules", ContentHash: "ab", SizeBytes: 5, Status: "ok"}
	}
	resolved := []workspacesdk.ContextInstructionFile{
		file("/repo/pkg000/AGENTS.md"),
		file("/repo/pkg000/CLAUDE.md"),
		file("/repo/new/AGENTS.md"),
		file("/repo/new/CLAUDE.md"),
		file("/repo/other/AGENTS.md"),
	}
	upserted := make([]string, 0, 2)
	db.EXPECT().DeleteChatContextDiscoveredResource(gomock.Any(), database.DeleteChatContextDiscoveredResourceParams{ChatID: chatID, Source: "/repo/pkg001/AGENTS.md"}).Return(nil)
	db.EXPECT().UpsertChatContextDiscoveredResource(gomock.Any(), gomock.Any()).Times(2).
		DoAndReturn(func(_ context.Context, arg database.UpsertChatContextDiscoveredResourceParams) error {
			upserted = append(upserted, arg.Source)
			return nil
		})

	stale := map[string]struct{}{"/repo/pkg001": {}}
	result, err := reconcileDiscoveredInstructionFiles(context.Background(), db, chatID, rows, resolved, stale, []string{"/repo/pkg000", "/repo/pkg001", "/repo/new", "/repo/other"}, discoveryBudget{})
	require.NoError(t, err)
	require.Equal(t, 1, result.removed)
	require.Equal(t, 2, result.pinned)
	require.Equal(t, []string{"/repo/pkg000/AGENTS.md", "/repo/pkg000/CLAUDE.md"}, upserted,
		"a held source is re-pinned and the freed slot goes to the first new file; the rest stay out")
	require.Equal(t, []string{"/repo/new", "/repo/other"}, result.capped)
}

func TestDiscoveredInstructionDirsShallowestFirst(t *testing.T) {
	t.Parallel()

	rows := []database.ChatContextResource{
		{Source: "/repo/AGENTS.md"},
		{Source: "/repo/a/deep/AGENTS.md", Discovered: true},
		{Source: "/repo/a/deep/CLAUDE.md", Discovered: true},
		{Source: "/repo/z/AGENTS.md", Discovered: true},
	}
	require.Equal(t, []string{"/repo/z", "/repo/a/deep"}, discoveredInstructionDirs(rows),
		"a refresh probes the directories that govern the most first, whatever the inventory order")
}

// TestReconcileDiscoveredInstructionFilesReleasesBytesOfNonOKReRead checks
// that a pinned file re-read as unreadable gives its bytes back before the
// next file is classified, so the freed budget holds that file.
func TestReconcileDiscoveredInstructionFilesReleasesBytesOfNonOKReRead(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	chatID := uuid.New()
	const held = maxDiscoveredInstructionBytes - 16
	rows := []database.ChatContextResource{{
		Source: "/repo/site/AGENTS.md", BodyKind: database.WorkspaceAgentContextBodyKindInstructionFile,
		Discovered: true, Status: database.WorkspaceAgentContextResourceStatusOk, SizeBytes: held,
	}}
	resolved := []workspacesdk.ContextInstructionFile{
		{Directory: "/repo/site", Source: "/repo/site/AGENTS.md", Status: "unreadable", Error: "permission denied"},
		{Directory: "/repo/site", Source: "/repo/site/CLAUDE.md", Content: "rules", ContentHash: "ab", SizeBytes: 32, Status: "ok"},
	}
	statuses := make(map[string]database.WorkspaceAgentContextResourceStatus)
	db.EXPECT().UpsertChatContextDiscoveredResource(gomock.Any(), gomock.Any()).Times(2).
		DoAndReturn(func(_ context.Context, arg database.UpsertChatContextDiscoveredResourceParams) error {
			statuses[arg.Source] = arg.Status
			return nil
		})

	stale := map[string]struct{}{"/repo/site": {}}
	_, err := reconcileDiscoveredInstructionFiles(context.Background(), db, chatID, rows, resolved, stale, []string{"/repo/site"}, discoveryBudget{})
	require.NoError(t, err)
	require.Equal(t, database.WorkspaceAgentContextResourceStatusUnreadable, statuses["/repo/site/AGENTS.md"])
	require.Equal(t, database.WorkspaceAgentContextResourceStatusOk, statuses["/repo/site/CLAUDE.md"], "the bytes the unreadable file held are free again")
}

// TestReconcileDiscoveredInstructionFilesLeavesSnapshotRows checks that a
// probe of a stale directory returning a file the agent snapshot already
// pins neither writes that row nor charges its bytes to the discovered cap.
func TestReconcileDiscoveredInstructionFilesLeavesSnapshotRows(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	chatID := uuid.New()
	rows := []database.ChatContextResource{
		{Source: "/repo/site/AGENTS.md", BodyKind: database.WorkspaceAgentContextBodyKindInstructionFile, Status: database.WorkspaceAgentContextResourceStatusOk, SizeBytes: 1 << 16},
		{Source: "/repo/site/docs/AGENTS.md", BodyKind: database.WorkspaceAgentContextBodyKindInstructionFile, Discovered: true, Status: database.WorkspaceAgentContextResourceStatusOk, SizeBytes: maxDiscoveredInstructionBytes - 16},
	}
	resolved := []workspacesdk.ContextInstructionFile{
		{Directory: "/repo/site", Source: "/repo/site/AGENTS.md", Content: "rules", ContentHash: "ab", SizeBytes: 1 << 16, Status: "ok"},
		{Directory: "/repo/site", Source: "/repo/site/CLAUDE.md", Content: "rules", ContentHash: "ab", SizeBytes: 16, Status: "ok"},
	}
	db.EXPECT().UpsertChatContextDiscoveredResource(gomock.Any(), gomock.Cond(func(arg database.UpsertChatContextDiscoveredResourceParams) bool {
		return arg.Source == "/repo/site/CLAUDE.md" && arg.Status == database.WorkspaceAgentContextResourceStatusOk
	})).Return(nil)

	stale := map[string]struct{}{"/repo/site": {}}
	result, err := reconcileDiscoveredInstructionFiles(context.Background(), db, chatID, rows, resolved, stale, []string{"/repo/site"}, discoveryBudget{})
	require.NoError(t, err)
	require.Equal(t, 1, result.pinned, "the snapshot's file is not counted as pinned")
}

// TestReconcileDiscoveredInstructionFilesHonorsReservedBudget checks that
// the share of the caps held by rows outside the reconciliation's reach is
// spent before the resolved files are classified.
func TestReconcileDiscoveredInstructionFilesHonorsReservedBudget(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	chatID := uuid.New()
	resolved := []workspacesdk.ContextInstructionFile{
		{Directory: "/repo/site", Source: "/repo/site/AGENTS.md", Content: "rules", ContentHash: "ab", SizeBytes: 32, Status: "ok"},
		{Directory: "/repo/docs", Source: "/repo/docs/AGENTS.md", Content: "rules", ContentHash: "ab", SizeBytes: 1, Status: "ok"},
	}
	db.EXPECT().UpsertChatContextDiscoveredResource(gomock.Any(), gomock.Cond(func(arg database.UpsertChatContextDiscoveredResourceParams) bool {
		return arg.Source == "/repo/site/AGENTS.md" && arg.Status == database.WorkspaceAgentContextResourceStatusExcluded
	})).Return(nil)

	reserved := discoveryBudget{bytes: maxDiscoveredInstructionBytes - 16, files: maxDiscoveredInstructionFiles - 1}
	result, err := reconcileDiscoveredInstructionFiles(context.Background(), db, chatID, nil, resolved, map[string]struct{}{}, []string{"/repo/site", "/repo/docs"}, reserved)
	require.NoError(t, err)
	require.Equal(t, 1, result.pinned, "the reserved bytes leave no room for the first file's content and the reserved rows leave one slot")
	require.Equal(t, []string{"/repo/docs"}, result.capped)
}

// TestInstructionProbeCachePendingCoversTree checks that a command's
// working directory keeps its whole tree pending, since the command may
// write anywhere below it, while a sibling tree is unaffected.
func TestInstructionProbeCachePendingCoversTree(t *testing.T) {
	t.Parallel()

	var cache instructionProbeCache
	agentID := uuid.New()
	now := time.Now()

	cache.markPending(now, agentID, []string{"/repo/site", "C:/Repo/win"})
	require.True(t, cache.isPending(now, agentID, "/repo/site"))
	require.True(t, cache.isPending(now, agentID, "/repo/site/src/components"), "a descendant of the command's directory is pending")
	require.False(t, cache.isPending(now, agentID, "/repo/docs"), "a sibling tree is not")
	require.False(t, cache.isPending(now, agentID, "/repo"), "nor is a parent")
	require.True(t, cache.isPending(now, agentID, "c:/repo/WIN/src"), "Windows trees match case-insensitively")
	require.False(t, cache.isPending(now.Add(pendingProbeTTL), agentID, "/repo/site/src"), "pending entries expire")
}

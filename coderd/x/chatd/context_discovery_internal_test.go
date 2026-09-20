package chatd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

	"cdr.dev/slog/v3/sloggers/slogtest"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
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
	touched := func(operatingSystem string, calls []fantasy.ToolCallContent, results []fantasy.Content) (files, dirs []string) {
		files, dirs = touchedPaths(calls, results)
		return agentTouchedPaths(files, dirs, operatingSystem)
	}

	files, dirs := touchedPaths(calls, results)
	require.Equal(t, []string{
		"/home/coder/project/site/src/App.tsx",
		"/home/coder/project/docs/../README.md",
		"/tmp/repo/a.go",
		"relative/b.go",
	}, files, "paths are trimmed like the file tools do; calls without a result are ignored")
	require.Equal(t, []string{"/tmp/repo/pkg"}, dirs, "only an explicit workdir counts")

	files, dirs = touched("linux", calls, results)
	require.Equal(t, []string{
		"/home/coder/project/site/src/App.tsx",
		"/home/coder/project/README.md",
		"/tmp/repo/a.go",
	}, files, "paths are cleaned and relative paths dropped")
	require.Equal(t, []string{"/tmp/repo/pkg"}, dirs)

	// A Windows agent reports drive-rooted paths with backslashes; they
	// are normalized to forward slashes so the same logic applies.
	winCalls := []fantasy.ToolCallContent{
		call("win-read", "read_file", `{"path":"C:\\repo\\site\\App.tsx"}`),
		call("win-exec", "execute", `{"command":"dir","workdir":"C:\\repo\\pkg"}`),
		call("win-rel", "read_file", `{"path":"repo\\App.tsx"}`),
		call("unc-read", "read_file", `{"path":"\\\\server\\share\\repo\\App.tsx"}`),
		call("unc-exec", "execute", `{"command":"dir","workdir":"//server/share/repo"}`),
		call("win-posix", "read_file", `{"path":"/tmp/App.tsx"}`),
	}
	winResults := []fantasy.Content{result("win-read"), result("win-exec"), result("win-rel"), result("unc-read"), result("unc-exec"), result("win-posix")}
	files, dirs = touched("windows", winCalls, winResults)
	require.Equal(t, []string{"C:/repo/site/App.tsx"}, files, "a UNC path and a POSIX-rooted path are relative to a Windows agent and not probed")
	require.Equal(t, []string{"C:/repo/pkg"}, dirs)
	files, _ = touched("linux", winCalls, winResults)
	require.Equal(t, []string{"/tmp/App.tsx"}, files, "a drive path is relative to a POSIX agent")

	// On POSIX a backslash is an ordinary character in a name and stays
	// one, and a doubled leading slash is the root.
	files, dirs = touched("linux", []fantasy.ToolCallContent{
		call("posix-read", "read_file", `{"path":"/repo/dir\\name/file.go"}`),
		call("posix-double", "read_file", `{"path":"//repo/site/file.go"}`),
		call("posix-double-exec", "execute", `{"command":"ls","workdir":"//repo/pkg"}`),
	}, []fantasy.Content{result("posix-read"), result("posix-double"), result("posix-double-exec")})
	require.Equal(t, []string{"/repo/dir\\name/file.go", "/repo/site/file.go"}, files)
	require.Equal(t, []string{"/repo/pkg"}, dirs)
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

func TestPinnedInstructionDirs(t *testing.T) {
	t.Parallel()

	instruction := func(source string, discovered bool, status database.WorkspaceAgentContextResourceStatus, size int64) database.ChatContextResource {
		return database.ChatContextResource{Source: source, BodyKind: database.WorkspaceAgentContextBodyKindInstructionFile, Discovered: discovered, Status: status, SizeBytes: size}
	}
	const ok, excluded, oversize = database.WorkspaceAgentContextResourceStatusOk, database.WorkspaceAgentContextResourceStatusExcluded, database.WorkspaceAgentContextResourceStatusOversize
	rows := []database.ChatContextResource{
		instruction("/repo/AGENTS.md", false, ok, 10),
		instruction("/repo/site/AGENTS.md", true, ok, maxDiscoveredInstructionBytes-100),
		instruction("/repo/site/CLAUDE.md", true, excluded, 100),
		instruction("/repo/docs/AGENTS.md", true, excluded, 101),
		instruction("/repo/pkg/AGENTS.md", true, oversize, 5),
		instruction("/repo/lib/AGENTS.md", false, excluded, 1),
		{Source: "/repo/cmd/skill", BodyKind: database.WorkspaceAgentContextBodyKindSkill, Discovered: true, Status: ok},
	}

	// site holds a file the cap kept out that fits the 100 free bytes, so
	// it is probed again even though its other file is pinned. docs does
	// not fit yet, pkg is too large for any budget, and lib's exclusion is
	// the snapshot's, not this chat's.
	pinned, freed := pinnedInstructionDirs(rows)
	require.Equal(t, map[string]struct{}{"/repo": {}, "/repo/docs": {}, "/repo/pkg": {}, "/repo/lib": {}}, pinned)
	require.Equal(t, map[string]struct{}{"/repo/site": {}}, freed)

	// Removing the large file frees the budget for docs as well.
	pinned, freed = pinnedInstructionDirs(append(rows[:1:1], rows[2:]...))
	require.Equal(t, map[string]struct{}{"/repo": {}, "/repo/pkg": {}, "/repo/lib": {}}, pinned)
	require.Equal(t, map[string]struct{}{"/repo/site": {}, "/repo/docs": {}}, freed)
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

// The row key is case-sensitive while the agent echoes the request's casing.
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
	chatA, chatB := uuid.New(), uuid.New()
	now := time.Now()

	require.False(t, cache.negative(now, agentID, chatA, "/tmp"))
	cache.markNegative(now, agentID, uuid.Nil, []string{"/tmp", "/tmp/repo"})
	require.True(t, cache.negative(now, agentID, chatA, "/tmp"))
	require.True(t, cache.negative(now, agentID, chatB, "/tmp"), "an empty directory is empty for every chat on the agent")
	require.False(t, cache.negative(now, uuid.New(), chatA, "/tmp"), "negatives are per agent")
	require.False(t, cache.negative(now.Add(instructionProbeTTL), agentID, chatA, "/tmp"), "entries expire")

	cache.forget(agentID, chatA, []string{"/tmp/repo"})
	require.False(t, cache.negative(now, agentID, chatA, "/tmp/repo"))
	require.True(t, cache.negative(now, agentID, chatA, "/tmp"))

	// A directory one chat's row cap kept out is that chat's business only.
	cache.markNegative(now, agentID, chatA, []string{"/tmp/capped"})
	require.True(t, cache.negative(now, agentID, chatA, "/tmp/capped"))
	require.False(t, cache.negative(now, agentID, chatB, "/tmp/capped"), "another chat on the agent still probes the directory")
	cache.forget(agentID, chatA, []string{"/tmp/capped"})
	require.False(t, cache.negative(now, agentID, chatA, "/tmp/capped"))

	cache.markNegative(now, agentID, uuid.Nil, []string{"C:/Repo/site"})
	require.True(t, cache.negative(now, agentID, chatA, "c:/repo/site"), "Windows directories match case-insensitively")
	cache.forget(agentID, chatA, []string{"c:/REPO/site"})
	require.False(t, cache.negative(now, agentID, chatA, "C:/Repo/site"))

	// Filling the cache past its bound resets it instead of growing.
	many := make([]string, 0, maxInstructionProbeEntries)
	for i := range maxInstructionProbeEntries {
		many = append(many, "/bulk/"+uuid.NewString()+string(rune('a'+i%26)))
	}
	cache.markNegative(now, agentID, uuid.Nil, many)
	require.LessOrEqual(t, len(cache.entries), maxInstructionProbeEntries)
	require.False(t, cache.negative(now, agentID, chatA, "/tmp"))
}

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

// A vanished row is removed before the replacement is classified.
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

// discoveryOp is a chat bound to the fixture's agent A, with A's snapshot
// row pinned, and a server to run discovery operations against it over a
// strict mock connection.
type discoveryOp struct {
	fix    rebindFixture
	chat   database.Chat
	agent  database.WorkspaceAgent
	conn   *agentconnmock.MockAgentConn
	server *Server
}

func newDiscoveryOp(t *testing.T) discoveryOp {
	t.Helper()
	fix := newRebindFixture(t)
	chat := dbgen.Chat(t, fix.db, database.Chat{
		OwnerID:           fix.user.ID,
		OrganizationID:    fix.org.ID,
		LastModelConfigID: fix.model.ID,
		WorkspaceID:       uuid.NullUUID{UUID: fix.ws.ID, Valid: true},
		AgentID:           uuid.NullUUID{UUID: fix.agentA, Valid: true},
		Status:            database.ChatStatusWaiting,
	})
	_, err := fix.db.HydrateAgentChatsContext(fix.ctx, database.HydrateAgentChatsContextParams{
		AgentID:       fix.agentA,
		AggregateHash: fix.hashA,
	})
	require.NoError(t, err)
	agent, err := fix.db.GetWorkspaceAgentByID(fix.ctx, fix.agentA)
	require.NoError(t, err)
	agent.Directory = path.Dir(fix.srcA)
	agent.OperatingSystem = "linux"
	return discoveryOp{
		fix:   fix,
		chat:  chat,
		agent: agent,
		conn:  agentconnmock.NewMockAgentConn(gomock.NewController(t)),
		server: &Server{
			db:     fix.db,
			pubsub: pubsub.NewInMemory(),
			logger: slogtest.Make(t, nil),
			clock:  quartz.NewReal(),
		},
	}
}

func (op discoveryOp) captured(t *testing.T) []database.ChatContextResource {
	t.Helper()
	rows, err := op.fix.db.ListChatContextResourcesByChatID(op.fix.ctx, op.chat.ID)
	require.NoError(t, err)
	return rows
}

func (op discoveryOp) rows(t *testing.T) map[string]database.ChatContextResource {
	t.Helper()
	out := make(map[string]database.ChatContextResource)
	for _, row := range op.captured(t) {
		out[row.Source] = row
	}
	return out
}

func (op discoveryOp) discover(files, dirs []string) {
	op.server.discoverInstructionContext(op.fix.ctx, op.conn, op.agent, op.chat, files, dirs)
}

func (op discoveryOp) expectProbe(dirs []string, files ...workspacesdk.ContextInstructionFile) {
	op.conn.EXPECT().ResolveContextInstructions(gomock.Any(), workspacesdk.ResolveContextInstructionsRequest{Directories: dirs}).
		Return(workspacesdk.ResolveContextInstructionsResponse{Files: files}, nil)
}

// discoveryWriteFailStore fails the discovered-row upsert for one source,
// in and out of transactions.
type discoveryWriteFailStore struct {
	database.Store
	failSource string
}

func (s *discoveryWriteFailStore) InTx(fn func(database.Store) error, opts *database.TxOptions) error {
	return s.Store.InTx(func(tx database.Store) error {
		return fn(&discoveryWriteFailStore{Store: tx, failSource: s.failSource})
	}, opts)
}

func (s *discoveryWriteFailStore) UpsertChatContextDiscoveredResource(ctx context.Context, arg database.UpsertChatContextDiscoveredResourceParams) error {
	if arg.Source == s.failSource {
		return xerrors.New("injected discovered row write failure")
	}
	return s.Store.UpsertChatContextDiscoveredResource(ctx, arg)
}

func resolvedInstructionFile(source, content string) workspacesdk.ContextInstructionFile {
	sum := sha256.Sum256([]byte(content))
	return workspacesdk.ContextInstructionFile{
		Directory:   path.Dir(source),
		Source:      source,
		Content:     content,
		ContentHash: hex.EncodeToString(sum[:]),
		SizeBytes:   uint64(len(content)),
		Status:      "ok",
	}
}

func TestApplyDiscoveredInstructionFiles(t *testing.T) {
	t.Parallel()

	const (
		nestedDir    = "/home/coder/workspace/site"
		nestedSource = nestedDir + "/AGENTS.md"
		secondSource = nestedDir + "/CLAUDE.md"
	)
	probed := []string{nestedDir}
	noStale := map[string]struct{}{}

	// A half-pinned directory would be skipped by later touches.
	t.Run("PinsNothingWhenAWriteFails", func(t *testing.T) {
		t.Parallel()
		op := newDiscoveryOp(t)
		captured := op.captured(t)
		op.server.db = &discoveryWriteFailStore{Store: op.fix.db, failSource: secondSource}

		_, err := op.server.applyDiscoveredInstructionFiles(op.fix.ctx, op.chat.ID, op.fix.agentA, captured,
			[]workspacesdk.ContextInstructionFile{resolvedInstructionFile(nestedSource, "site rules"), resolvedInstructionFile(secondSource, "claude rules")}, noStale, probed)
		require.ErrorContains(t, err, "injected")

		rows := op.rows(t)
		require.Len(t, rows, 1, "the failed reconciliation pinned nothing")
		require.False(t, rows[op.fix.srcA].Discovered)
	})

	// The rebind cleared the rows for the new workspace.
	t.Run("DropsTheAnswerAfterARebind", func(t *testing.T) {
		t.Parallel()
		op := newDiscoveryOp(t)
		captured := op.captured(t)
		_, err := op.fix.db.UpdateChatBuildAgentBinding(op.fix.ctx, database.UpdateChatBuildAgentBindingParams{
			ID:      op.chat.ID,
			BuildID: op.chat.BuildID,
			AgentID: uuid.NullUUID{UUID: op.fix.agentB, Valid: true},
		})
		require.NoError(t, err)
		require.NoError(t, op.fix.db.DeleteChatContextResourcesByChatID(op.fix.ctx, op.chat.ID))

		result, err := op.server.applyDiscoveredInstructionFiles(op.fix.ctx, op.chat.ID, op.fix.agentA, captured,
			[]workspacesdk.ContextInstructionFile{resolvedInstructionFile(nestedSource, "site rules")}, noStale, probed)
		require.NoError(t, err)
		require.Zero(t, result.pinned)
		require.Empty(t, op.rows(t), "the previous agent's file is not pinned onto the rebound chat")
	})

	// An agent push made the nested file a snapshot row during the round trip.
	t.Run("LeavesSnapshotRowsToTheAgent", func(t *testing.T) {
		t.Parallel()
		op := newDiscoveryOp(t)
		captured := op.captured(t)
		seedAgentContext(op.fix.ctx, t, op.fix.db, op.fix.agentA, nestedSource, []byte{0x5e},
			database.WorkspaceAgentContextBodyKindInstructionFile, json.RawMessage(`{"instruction_file":{"content":"site rules"}}`))
		require.NoError(t, op.fix.db.InsertAgentContextResourcesIntoChat(op.fix.ctx, database.InsertAgentContextResourcesIntoChatParams{
			ChatID:  op.chat.ID,
			AgentID: op.fix.agentA,
		}))
		nested := resolvedInstructionFile(nestedSource, "site rules")
		nested.SizeBytes = maxDiscoveredInstructionBytes

		result, err := op.server.applyDiscoveredInstructionFiles(op.fix.ctx, op.chat.ID, op.fix.agentA, captured,
			[]workspacesdk.ContextInstructionFile{nested, resolvedInstructionFile(secondSource, "claude rules")}, noStale, probed)
		require.NoError(t, err)
		require.Equal(t, 1, result.pinned)

		rows := op.rows(t)
		require.False(t, rows[nestedSource].Discovered, "the snapshot's row is left to the agent")
		require.True(t, rows[secondSource].Discovered)
		require.Equal(t, database.WorkspaceAgentContextResourceStatusOk, rows[secondSource].Status, "the snapshot-owned file's bytes are not charged to the discovered budget")
	})

	// The other writer read the file after the probe did.
	t.Run("KeepsANewerPin", func(t *testing.T) {
		t.Parallel()
		op := newDiscoveryOp(t)
		captured := op.captured(t)
		newer := resolvedInstructionFile(nestedSource, "site rules v3")
		newerHash, err := hex.DecodeString(newer.ContentHash)
		require.NoError(t, err)
		require.NoError(t, op.fix.db.UpsertChatContextDiscoveredResource(op.fix.ctx, database.UpsertChatContextDiscoveredResourceParams{
			ChatID:      op.chat.ID,
			Source:      nestedSource,
			BodyKind:    database.WorkspaceAgentContextBodyKindInstructionFile,
			Body:        []byte("{}"),
			ContentHash: newerHash,
			SizeBytes:   int64(len("site rules v3")),
			Status:      database.WorkspaceAgentContextResourceStatusOk,
		}))

		_, err = op.server.applyDiscoveredInstructionFiles(op.fix.ctx, op.chat.ID, op.fix.agentA, captured,
			[]workspacesdk.ContextInstructionFile{resolvedInstructionFile(nestedSource, "site rules v2")}, noStale, probed)
		require.NoError(t, err)
		require.Equal(t, newerHash, op.rows(t)[nestedSource].ContentHash, "the newer read stays")
	})
}

func TestDiscoverInstructionContext(t *testing.T) {
	t.Parallel()

	const (
		nestedDir    = "/home/coder/workspace/site"
		nestedSource = nestedDir + "/AGENTS.md"
		secondSource = nestedDir + "/CLAUDE.md"
		touchedFile  = nestedDir + "/src/App.tsx"
	)
	chain := []string{nestedDir, nestedDir + "/src"}

	// The excluded file is larger than the chat's budget, so no later probe
	// could fit it and the second read asks nothing.
	t.Run("PinsFoundFilesOnce", func(t *testing.T) {
		t.Parallel()
		op := newDiscoveryOp(t)
		events := make(chan struct{}, 1)
		cancel, err := op.server.pubsub.Subscribe(coderdpubsub.ChatWatchEventChannel(op.chat.OwnerID), func(context.Context, []byte) {
			select {
			case events <- struct{}{}:
			default:
			}
		})
		require.NoError(t, err)
		defer cancel()
		excluded := resolvedInstructionFile(secondSource, "")
		excluded.SizeBytes, excluded.Status, excluded.Error = maxDiscoveredInstructionBytes+1, "excluded", "response content cap exceeded"
		op.expectProbe(chain, resolvedInstructionFile(nestedSource, "site rules"), excluded)

		op.discover([]string{touchedFile}, nil)
		op.discover([]string{touchedFile}, nil)

		rows := op.rows(t)
		require.Len(t, rows, 3)
		require.False(t, rows[op.fix.srcA].Discovered)
		require.True(t, rows[nestedSource].Discovered)
		require.Equal(t, database.WorkspaceAgentContextResourceStatusExcluded, rows[secondSource].Status, "an excluded file is part of the inventory")
		testutil.RequireReceive(op.fix.ctx, t, events)
	})

	t.Run("SwallowsAFailedProbe", func(t *testing.T) {
		t.Parallel()
		op := newDiscoveryOp(t)
		op.conn.EXPECT().ResolveContextInstructions(gomock.Any(), gomock.Any()).
			Return(workspacesdk.ResolveContextInstructionsResponse{}, xerrors.New("404 not found"))

		op.discover([]string{touchedFile}, nil)

		require.Len(t, op.rows(t), 1, "a failed probe pins nothing")
	})

	t.Run("ReconcilesRemovedFiles", func(t *testing.T) {
		t.Parallel()
		op := newDiscoveryOp(t)
		op.expectProbe(chain, resolvedInstructionFile(nestedSource, "site rules"))
		op.expectProbe([]string{nestedDir}, resolvedInstructionFile(secondSource, "new rules"))

		op.discover([]string{touchedFile}, nil)
		op.discover([]string{secondSource}, nil)

		rows := op.rows(t)
		require.NotContains(t, rows, nestedSource, "the removed file is gone")
		require.True(t, rows[secondSource].Discovered)
	})

	// The write may not have landed when the first probe ran.
	t.Run("ReprobesAStaleDirectory", func(t *testing.T) {
		t.Parallel()
		op := newDiscoveryOp(t)
		op.expectProbe([]string{nestedDir})
		op.expectProbe(chain, resolvedInstructionFile(secondSource, "final rules"))

		op.discover([]string{secondSource}, nil)
		op.discover([]string{touchedFile}, nil)

		require.True(t, op.rows(t)[secondSource].Discovered)
	})

	// The command may still be writing when its result returns.
	t.Run("ReprobesAfterACommand", func(t *testing.T) {
		t.Parallel()
		op := newDiscoveryOp(t)
		op.expectProbe([]string{nestedDir}, resolvedInstructionFile(nestedSource, "site rules"))
		op.expectProbe(chain, resolvedInstructionFile(nestedSource, "site rules"), resolvedInstructionFile(secondSource, "generated rules"))

		op.discover(nil, []string{nestedDir})
		op.discover([]string{touchedFile}, nil)

		rows := op.rows(t)
		require.True(t, rows[nestedSource].Discovered)
		require.True(t, rows[secondSource].Discovered, "the file the command created after its result is pinned by the next read")
	})

	// The excluded row would otherwise keep the directory probed forever.
	t.Run("DropsAVanishedExcludedFile", func(t *testing.T) {
		t.Parallel()
		op := newDiscoveryOp(t)
		require.NoError(t, op.fix.db.UpsertChatContextDiscoveredResource(op.fix.ctx, database.UpsertChatContextDiscoveredResourceParams{
			ChatID:      op.chat.ID,
			Source:      nestedSource,
			BodyKind:    database.WorkspaceAgentContextBodyKindInstructionFile,
			Body:        []byte("{}"),
			ContentHash: []byte("gone"),
			SizeBytes:   12,
			Status:      database.WorkspaceAgentContextResourceStatusExcluded,
			Error:       "discovered instruction content cap reached",
		}))
		op.expectProbe(chain)

		op.discover([]string{touchedFile}, nil)

		require.NotContains(t, op.rows(t), nestedSource, "the vanished file's row is dropped")
	})
}

package chatd

import (
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
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

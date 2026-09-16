package chatd

import (
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
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
		call("write", "write_file", `{"path":"/home/coder/project/docs/../README.md","content":"x"}`),
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
	}, files, "paths are cleaned; relative paths and calls without a result are ignored")
	require.Equal(t, []string{"/tmp/repo/pkg"}, dirs, "only an explicit workdir counts")
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

	t.Run("DepthCap", func(t *testing.T) {
		t.Parallel()
		deep := "/x"
		for range 40 {
			deep += "/d"
		}
		got := candidateInstructionDirs([]string{deep + "/f"}, nil, workingDir)
		require.Len(t, got, maxInstructionAncestorDepth)
	})

	t.Run("RequestCap", func(t *testing.T) {
		t.Parallel()
		var dirs []string
		for i := range 50 {
			dirs = append(dirs, "/tmp/"+string(rune('a'+i%26))+string(rune('a'+i/26)))
		}
		got := candidateInstructionDirs(nil, dirs, workingDir)
		require.Len(t, got, 32)
		require.Equal(t, "/tmp", got[0], "the shared parent sorts first")
	})
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

	// Filling the cache past its bound resets it instead of growing.
	many := make([]string, 0, maxInstructionProbeEntries)
	for i := range maxInstructionProbeEntries {
		many = append(many, "/bulk/"+uuid.NewString()+string(rune('a'+i%26)))
	}
	cache.markNegative(now, agentID, many)
	require.LessOrEqual(t, len(cache.entries), maxInstructionProbeEntries)
	require.False(t, cache.negative(now, agentID, "/tmp"))
}

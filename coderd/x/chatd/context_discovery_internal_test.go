package chatd

import (
	"testing"

	"charm.land/fantasy"
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

package chatd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"path"
	"slices"
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

	t.Run("NoWorkingDirStopsBelowRoot", func(t *testing.T) {
		t.Parallel()
		got := candidateInstructionDirs([]string{"/home/coder/project/site/a.ts"}, nil, "")
		require.Equal(t, []string{"/home", "/home/coder", "/home/coder/project", "/home/coder/project/site"}, got)
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

	// Writing a rule file or running a command in a directory may create a
	// file the agent last reported missing.
	require.Equal(t, []string{"/repo/pkg", "/repo/site"}, rewrittenInstructionDirs(
		[]string{"/repo/site/CLAUDE.md", "/repo/site/src/App.tsx"},
		[]string{"/repo/pkg"},
	))
	// Windows spells the recognized names in any case; POSIX does not.
	require.Equal(t, []string{"C:/repo/site"}, rewrittenInstructionDirs([]string{"C:/repo/site/agents.md", "/repo/docs/agents.md"}, nil))

	pinned := map[string]struct{}{"/repo/site": {}, "/repo/docs": {}}
	negative := func(dir string) bool { return dir == "/repo/pkg" }
	got := selectInstructionProbes([]string{"/repo/site", "/repo/site/src", "/repo/docs", "/repo/pkg", "/repo/lib"}, pinned, negative)
	require.Equal(t, []string{"/repo/site/src", "/repo/lib"}, got)

	// A pinned Windows directory matches however the tool spelled it.
	got = selectInstructionProbes([]string{"c:/Repo/site", "C:/repo/docs"}, map[string]struct{}{"c:/repo/site": {}}, func(string) bool { return false })
	require.Equal(t, []string{"C:/repo/docs"}, got)
}

func TestPinnedInstructionDirs(t *testing.T) {
	t.Parallel()

	rows := []database.ChatContextResource{
		{Source: "/repo/AGENTS.md", BodyKind: database.WorkspaceAgentContextBodyKindInstructionFile},
		{Source: "/repo/site/CLAUDE.md", BodyKind: database.WorkspaceAgentContextBodyKindInstructionFile, Discovered: true, Status: database.WorkspaceAgentContextResourceStatusExcluded},
		{Source: "C:\\repo\\win\\AGENTS.md", BodyKind: database.WorkspaceAgentContextBodyKindInstructionFile, Discovered: true},
		{Source: "/repo/cmd/skill/SKILL.md", BodyKind: database.WorkspaceAgentContextBodyKindSkill},
	}
	require.Equal(t, map[string]struct{}{"/repo": {}, "/repo/site": {}, "c:/repo/win": {}}, pinnedInstructionDirs(rows),
		"an excluded file pins its directory like a readable one; a skill does not")
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

func TestPinInstructionFiles(t *testing.T) {
	t.Parallel()

	const ok = database.WorkspaceAgentContextResourceStatusOk
	instruction := func(source string, discovered bool, size int64) database.ChatContextResource {
		return database.ChatContextResource{Source: source, BodyKind: database.WorkspaceAgentContextBodyKindInstructionFile, Discovered: discovered, Status: ok, SizeBytes: size}
	}
	file := func(source string, size uint64) workspacesdk.ContextInstructionFile {
		return workspacesdk.ContextInstructionFile{Directory: path.Dir(source), Source: source, Content: "rules", ContentHash: "ab", SizeBytes: size, Status: "ok"}
	}
	pin := func(t *testing.T, rows []database.ChatContextResource, resolved ...workspacesdk.ContextInstructionFile) map[string]database.InsertChatContextDiscoveredResourceParams {
		t.Helper()
		db := dbmock.NewMockStore(gomock.NewController(t))
		inserted := make(map[string]database.InsertChatContextDiscoveredResourceParams)
		db.EXPECT().InsertChatContextDiscoveredResource(gomock.Any(), gomock.Any()).AnyTimes().
			DoAndReturn(func(_ context.Context, arg database.InsertChatContextDiscoveredResourceParams) error {
				inserted[arg.Source] = arg
				return nil
			})
		pinned, err := pinInstructionFiles(context.Background(), db, uuid.New(), rows, resolved)
		require.NoError(t, err)
		require.Len(t, inserted, pinned)
		return inserted
	}

	// The row key is case-sensitive while the agent echoes the request's casing.
	t.Run("LeavesHeldSources", func(t *testing.T) {
		t.Parallel()
		inserted := pin(t, []database.ChatContextResource{
			instruction("/repo/site/AGENTS.md", false, 5),
			instruction("C:\\repo\\win\\AGENTS.md", true, 5),
		}, file("/repo/site/AGENTS.md", 5), file("c:/Repo/win/AGENTS.md", 5), file("/repo/site/CLAUDE.md", 5))
		require.Equal(t, []string{"/repo/site/CLAUDE.md"}, slices.Collect(maps.Keys(inserted)))
	})

	t.Run("ExcludesContentPastAByteCap", func(t *testing.T) {
		t.Parallel()
		inserted := pin(t, []database.ChatContextResource{instruction("/repo/site/AGENTS.md", true, maxDiscoveredInstructionBytes-16)},
			file("/repo/site/CLAUDE.md", 16), file("/repo/site/docs/AGENTS.md", 1))
		require.Equal(t, ok, inserted["/repo/site/CLAUDE.md"].Status, "the remaining budget holds the file exactly")
		over := inserted["/repo/site/docs/AGENTS.md"]
		require.Equal(t, database.WorkspaceAgentContextResourceStatusExcluded, over.Status)
		require.Contains(t, over.Error, "cap")
		var body agentproto.InstructionFileBody
		require.NoError(t, protojson.Unmarshal(over.Body, &body))
		require.Empty(t, body.Content, "an excluded file is pinned without its content")

		// Snapshot prompts count against the chat's content cap; MCP tool
		// inventory does not.
		inserted = pin(t, []database.ChatContextResource{
			instruction("/repo/AGENTS.md", false, maxChatContextContentBytes-16),
			{Source: "/repo/.mcp.json", BodyKind: database.WorkspaceAgentContextBodyKindMcpConfig, Status: ok, SizeBytes: 1 << 20},
		}, file("/repo/site/AGENTS.md", 16), file("/repo/site/CLAUDE.md", 1))
		require.Equal(t, ok, inserted["/repo/site/AGENTS.md"].Status)
		require.Equal(t, database.WorkspaceAgentContextResourceStatusExcluded, inserted["/repo/site/CLAUDE.md"].Status)
	})

	t.Run("StopsAtARowCap", func(t *testing.T) {
		t.Parallel()
		discovered := make([]database.ChatContextResource, 0, maxDiscoveredInstructionFiles)
		for i := range maxDiscoveredInstructionFiles - 1 {
			discovered = append(discovered, instruction(fmt.Sprintf("/repo/d%d/AGENTS.md", i), true, 1))
		}
		inserted := pin(t, discovered, file("/repo/a/AGENTS.md", 1), file("/repo/b/AGENTS.md", 1))
		require.Equal(t, []string{"/repo/a/AGENTS.md"}, slices.Collect(maps.Keys(inserted)), "the discovered row cap keeps the first in resolved order")

		snapshot := make([]database.ChatContextResource, 0, maxChatContextResources)
		for i := range maxChatContextResources - 1 {
			snapshot = append(snapshot, instruction(fmt.Sprintf("/repo/s%d/AGENTS.md", i), false, 0))
		}
		inserted = pin(t, snapshot, file("/repo/a/AGENTS.md", 1), file("/repo/b/AGENTS.md", 1))
		require.Equal(t, []string{"/repo/a/AGENTS.md"}, slices.Collect(maps.Keys(inserted)), "so does the chat's row cap")
	})
}

func TestReleaseCapturedRows(t *testing.T) {
	t.Parallel()

	row := func(source string, hash byte, discovered bool) database.ChatContextResource {
		return database.ChatContextResource{Source: source, ContentHash: []byte{hash}, Discovered: discovered, BodyKind: database.WorkspaceAgentContextBodyKindInstructionFile, Status: database.WorkspaceAgentContextResourceStatusOk}
	}
	captured := []database.ChatContextResource{
		row("/repo/site/AGENTS.md", 1, true),
		row("/repo/site/CLAUDE.md", 1, true),
		row("/repo/site/.cursorrules", 1, true),
		row("/repo/lib/AGENTS.md", 1, true),
		row("/repo/docs/AGENTS.md", 1, true),
	}
	current := []database.ChatContextResource{
		row("/repo/AGENTS.md", 9, false),
		row("/repo/docs/AGENTS.md", 1, true),
		row("/repo/lib/AGENTS.md", 1, false),
		row("/repo/site/AGENTS.md", 1, true),
		row("/repo/site/CLAUDE.md", 2, true),
	}
	chatID := uuid.New()
	db := dbmock.NewMockStore(gomock.NewController(t))
	db.EXPECT().DeleteChatContextDiscoveredResource(gomock.Any(), database.DeleteChatContextDiscoveredResourceParams{ChatID: chatID, Source: "/repo/site/AGENTS.md"}).Return(nil)

	rows, kept, err := releaseCapturedRows(context.Background(), db, chatID, current, captured, []string{"/repo/site", "/repo/lib"})
	require.NoError(t, err)
	sources := make([]string, 0, len(rows))
	for _, r := range rows {
		sources = append(sources, r.Source)
	}
	require.Equal(t, []string{"/repo/AGENTS.md", "/repo/docs/AGENTS.md", "/repo/lib/AGENTS.md", "/repo/site/CLAUDE.md"}, sources,
		"only the unchanged row in a probed directory is released")
	require.Equal(t, map[string]struct{}{"/repo/site/CLAUDE.md": {}, "/repo/site/.cursorrules": {}, "/repo/lib/AGENTS.md": {}}, kept,
		"a rewritten, removed, or snapshot-owned row belongs to the newer writer")
}

func TestDiscoveredInstructionDirsShallowestFirst(t *testing.T) {
	t.Parallel()

	rows := []database.ChatContextResource{
		{Source: "/repo/a/deep/AGENTS.md", Discovered: true},
		{Source: "/repo/a/deep/CLAUDE.md", Discovered: true},
		{Source: "/repo/z/AGENTS.md", Discovered: true},
	}
	require.Equal(t, []string{"/repo/z", "/repo/a/deep"}, discoveredInstructionDirs(rows),
		"a refresh probes the directories that govern the most first, whatever the inventory order")
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

// discoveryWriteFailStore fails the discovered-row insert for one source,
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

func (s *discoveryWriteFailStore) InsertChatContextDiscoveredResource(ctx context.Context, arg database.InsertChatContextDiscoveredResourceParams) error {
	if arg.Source == s.failSource {
		return xerrors.New("injected discovered row write failure")
	}
	return s.Store.InsertChatContextDiscoveredResource(ctx, arg)
}

// txStartStore runs onStart once, inside the first transaction and before
// its body.
type txStartStore struct {
	database.Store
	onStart func(tx database.Store)
}

func (s *txStartStore) InTx(fn func(database.Store) error, opts *database.TxOptions) error {
	return s.Store.InTx(func(tx database.Store) error {
		if onStart := s.onStart; onStart != nil {
			s.onStart = nil
			onStart(tx)
		}
		return fn(tx)
	}, opts)
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

	// A half-pinned directory would be skipped by later touches.
	t.Run("PinsNothingWhenAWriteFails", func(t *testing.T) {
		t.Parallel()
		op := newDiscoveryOp(t)
		op.server.db = &discoveryWriteFailStore{Store: op.fix.db, failSource: secondSource}

		_, err := op.server.applyDiscoveredInstructionFiles(op.fix.ctx, op.chat.ID, op.fix.agentA,
			[]workspacesdk.ContextInstructionFile{resolvedInstructionFile(nestedSource, "site rules"), resolvedInstructionFile(secondSource, "claude rules")}, probed, nil)
		require.ErrorContains(t, err, "injected")

		rows := op.rows(t)
		require.Len(t, rows, 1, "the failed apply pinned nothing")
		require.False(t, rows[op.fix.srcA].Discovered)
	})

	// The rebind cleared the rows for the new workspace.
	t.Run("DropsTheAnswerAfterARebind", func(t *testing.T) {
		t.Parallel()
		op := newDiscoveryOp(t)
		_, err := op.fix.db.UpdateChatBuildAgentBinding(op.fix.ctx, database.UpdateChatBuildAgentBindingParams{
			ID:      op.chat.ID,
			BuildID: op.chat.BuildID,
			AgentID: uuid.NullUUID{UUID: op.fix.agentB, Valid: true},
		})
		require.NoError(t, err)
		require.NoError(t, op.fix.db.DeleteChatContextResourcesByChatID(op.fix.ctx, database.DeleteChatContextResourcesByChatIDParams{ChatID: op.chat.ID}))

		pinned, err := op.server.applyDiscoveredInstructionFiles(op.fix.ctx, op.chat.ID, op.fix.agentA,
			[]workspacesdk.ContextInstructionFile{resolvedInstructionFile(nestedSource, "site rules")}, probed, nil)
		require.NoError(t, err)
		require.Zero(t, pinned)
		require.Empty(t, op.rows(t), "the previous agent's file is not pinned onto the rebound chat")
	})

	// An agent push made the nested file a snapshot row during the round trip.
	t.Run("LeavesSnapshotRowsToTheAgent", func(t *testing.T) {
		t.Parallel()
		op := newDiscoveryOp(t)
		seedAgentContext(op.fix.ctx, t, op.fix.db, op.fix.agentA, nestedSource, []byte{0x5e},
			database.WorkspaceAgentContextBodyKindInstructionFile, json.RawMessage(`{"instruction_file":{"content":"site rules"}}`))
		require.NoError(t, op.fix.db.InsertAgentContextResourcesIntoChat(op.fix.ctx, database.InsertAgentContextResourcesIntoChatParams{
			ChatID:  op.chat.ID,
			AgentID: op.fix.agentA,
		}))
		nested := resolvedInstructionFile(nestedSource, "site rules")
		nested.SizeBytes = maxDiscoveredInstructionBytes

		pinned, err := op.server.applyDiscoveredInstructionFiles(op.fix.ctx, op.chat.ID, op.fix.agentA,
			[]workspacesdk.ContextInstructionFile{nested, resolvedInstructionFile(secondSource, "claude rules")}, probed, nil)
		require.NoError(t, err)
		require.Equal(t, 1, pinned)

		rows := op.rows(t)
		require.False(t, rows[nestedSource].Discovered, "the snapshot's row is left to the agent")
		require.True(t, rows[secondSource].Discovered)
		require.Equal(t, database.WorkspaceAgentContextResourceStatusOk, rows[secondSource].Status, "the snapshot-owned file's bytes are not charged to the discovered budget")
	})

	// Two pins that each fit the budget alone must not both land.
	t.Run("AdmitsAgainstAConcurrentPin", func(t *testing.T) {
		t.Parallel()
		op := newDiscoveryOp(t)
		const content = "site rules"
		require.NoError(t, op.fix.db.InsertChatContextDiscoveredResource(op.fix.ctx, database.InsertChatContextDiscoveredResourceParams{
			ChatID:      op.chat.ID,
			Source:      "/home/coder/workspace/big/AGENTS.md",
			BodyKind:    database.WorkspaceAgentContextBodyKindInstructionFile,
			Body:        json.RawMessage(`{}`),
			ContentHash: []byte{0x01},
			SizeBytes:   maxDiscoveredInstructionBytes - int64(len(content)),
			Status:      database.WorkspaceAgentContextResourceStatusOk,
		}))
		concurrent := &Server{db: op.fix.db, logger: op.server.logger}
		op.server.db = &txStartStore{Store: op.fix.db, onStart: func(tx database.Store) {
			// The first statement fixes this transaction's snapshot before
			// the concurrent pin commits.
			_, err := tx.GetChatByID(op.fix.ctx, op.chat.ID)
			require.NoError(t, err)
			pinned, err := concurrent.applyDiscoveredInstructionFiles(op.fix.ctx, op.chat.ID, op.fix.agentA,
				[]workspacesdk.ContextInstructionFile{resolvedInstructionFile(nestedSource, content)}, probed, nil)
			require.NoError(t, err)
			require.Equal(t, 1, pinned)
		}}

		_, err := op.server.applyDiscoveredInstructionFiles(op.fix.ctx, op.chat.ID, op.fix.agentA,
			[]workspacesdk.ContextInstructionFile{resolvedInstructionFile(secondSource, "more rules")}, probed, nil)
		require.NoError(t, err)

		rows := op.rows(t)
		require.Equal(t, database.WorkspaceAgentContextResourceStatusOk, rows[nestedSource].Status)
		require.Equal(t, database.WorkspaceAgentContextResourceStatusExcluded, rows[secondSource].Status, "the later pin is charged for the earlier one's bytes")
	})

	// Only Refresh rewrites what the model has read.
	t.Run("NeverRewritesAPinnedRow", func(t *testing.T) {
		t.Parallel()
		op := newDiscoveryOp(t)
		_, err := op.server.applyDiscoveredInstructionFiles(op.fix.ctx, op.chat.ID, op.fix.agentA,
			[]workspacesdk.ContextInstructionFile{resolvedInstructionFile(nestedSource, "site rules v1")}, probed, nil)
		require.NoError(t, err)
		first := op.rows(t)[nestedSource]

		pinned, err := op.server.applyDiscoveredInstructionFiles(op.fix.ctx, op.chat.ID, op.fix.agentA,
			[]workspacesdk.ContextInstructionFile{resolvedInstructionFile(nestedSource, "site rules v2"), resolvedInstructionFile(secondSource, "claude rules")}, probed, nil)
		require.NoError(t, err)
		require.Equal(t, 1, pinned)
		rows := op.rows(t)
		require.Equal(t, first, rows[nestedSource], "the pinned row keeps the content the model read")
		require.True(t, rows[secondSource].Discovered)
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

	// Pinned and empty directories are not asked again.
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

	// The strict connection fails the test on any probe after the first.
	t.Run("LeavesPinnedDirectoriesToRefresh", func(t *testing.T) {
		t.Parallel()
		op := newDiscoveryOp(t)
		op.expectProbe(chain, resolvedInstructionFile(nestedSource, "site rules"))

		op.discover([]string{touchedFile}, nil)
		pinned := op.rows(t)[nestedSource]
		op.discover([]string{secondSource}, nil)
		op.discover(nil, []string{nestedDir})

		rows := op.rows(t)
		require.Equal(t, pinned, rows[nestedSource])
		require.NotContains(t, rows, secondSource)
	})

	t.Run("ReprobesWhereAToolMayHaveWritten", func(t *testing.T) {
		t.Parallel()
		op := newDiscoveryOp(t)
		op.expectProbe(chain)
		op.expectProbe([]string{nestedDir}, resolvedInstructionFile(secondSource, "final rules"))
		op.expectProbe([]string{nestedDir + "/src"})

		op.discover([]string{touchedFile}, nil)
		op.discover([]string{touchedFile}, nil)
		op.discover([]string{secondSource}, nil)
		op.discover(nil, []string{nestedDir + "/src"})

		require.True(t, op.rows(t)[secondSource].Discovered)
	})

	t.Run("WalksToTheRootWithoutAWorkingDirectory", func(t *testing.T) {
		t.Parallel()
		op := newDiscoveryOp(t)
		op.agent.Directory, op.agent.ExpandedDirectory = "", ""
		op.expectProbe([]string{"/home", "/home/coder", nestedDir, nestedDir + "/src"}, resolvedInstructionFile(nestedSource, "site rules"))

		op.discover([]string{touchedFile}, nil)

		require.True(t, op.rows(t)[nestedSource].Discovered)
	})
}

func TestRefreshChatContextRediscoversInstructionFiles(t *testing.T) {
	t.Parallel()

	const (
		nestedDir    = "/home/coder/workspace/site"
		nestedSource = nestedDir + "/AGENTS.md"
		secondSource = nestedDir + "/CLAUDE.md"
	)
	newRefreshOp := func(t *testing.T, discovered ...workspacesdk.ContextInstructionFile) discoveryOp {
		t.Helper()
		op := newDiscoveryOp(t)
		_, err := op.server.applyDiscoveredInstructionFiles(op.fix.ctx, op.chat.ID, op.fix.agentA, discovered, []string{nestedDir}, nil)
		require.NoError(t, err)
		op.server.agentConnFn = func(context.Context, uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			return op.conn, func() {}, nil
		}
		return op
	}
	refresh := func(t *testing.T, op discoveryOp) database.Chat {
		t.Helper()
		current, err := op.fix.db.GetChatByID(op.fix.ctx, op.chat.ID)
		require.NoError(t, err)
		refreshed, err := op.server.RefreshChatContext(op.fix.ctx, current)
		require.NoError(t, err)
		return refreshed
	}
	hashOf := func(content string) []byte {
		sum := sha256.Sum256([]byte(content))
		return sum[:]
	}

	t.Run("ReReadsDiscoveredRows", func(t *testing.T) {
		t.Parallel()
		op := newRefreshOp(t, resolvedInstructionFile(nestedSource, "site rules v1"), resolvedInstructionFile(secondSource, "claude rules"))
		op.expectProbe([]string{nestedDir}, resolvedInstructionFile(nestedSource, "site rules v2"))

		refresh(t, op)

		rows := op.rows(t)
		require.Len(t, rows, 2, "refresh keeps the discovered file alongside the snapshot copy and drops the vanished one")
		require.False(t, rows[op.fix.srcA].Discovered)
		require.True(t, rows[nestedSource].Discovered)
		require.Equal(t, hashOf("site rules v2"), rows[nestedSource].ContentHash, "refresh re-reads the nested file")
	})

	t.Run("KeepsRowsWhenTheReReadFails", func(t *testing.T) {
		t.Parallel()
		op := newRefreshOp(t, resolvedInstructionFile(nestedSource, "site rules v1"))
		op.conn.EXPECT().ResolveContextInstructions(gomock.Any(), gomock.Any()).
			Return(workspacesdk.ResolveContextInstructionsResponse{}, xerrors.New("agent gone"))

		refreshed := refresh(t, op)

		require.False(t, refreshed.ContextDirtySince.Valid)
		rows := op.rows(t)
		require.Len(t, rows, 2)
		require.True(t, rows[nestedSource].Discovered, "the discovered row survives a failed re-read")
		require.Equal(t, hashOf("site rules v1"), rows[nestedSource].ContentHash)
	})

	// An overlapping refresh found the file gone and a step pinned a new
	// one while this refresh's probe was in flight.
	t.Run("KeepsNewerStateOverAnOlderAnswer", func(t *testing.T) {
		t.Parallel()
		const addedSource = "/home/coder/workspace/docs/AGENTS.md"
		op := newRefreshOp(t, resolvedInstructionFile(nestedSource, "site rules v1"))
		op.conn.EXPECT().ResolveContextInstructions(gomock.Any(), gomock.Any()).
			DoAndReturn(func(context.Context, workspacesdk.ResolveContextInstructionsRequest) (workspacesdk.ResolveContextInstructionsResponse, error) {
				require.NoError(t, op.fix.db.DeleteChatContextDiscoveredResource(op.fix.ctx, database.DeleteChatContextDiscoveredResourceParams{ChatID: op.chat.ID, Source: nestedSource}))
				_, err := op.server.applyDiscoveredInstructionFiles(op.fix.ctx, op.chat.ID, op.fix.agentA,
					[]workspacesdk.ContextInstructionFile{resolvedInstructionFile(addedSource, "docs rules")}, []string{path.Dir(addedSource)}, nil)
				require.NoError(t, err)
				return workspacesdk.ResolveContextInstructionsResponse{Files: []workspacesdk.ContextInstructionFile{resolvedInstructionFile(nestedSource, "site rules v2")}}, nil
			})

		refresh(t, op)

		rows := op.rows(t)
		require.NotContains(t, rows, nestedSource, "the older answer does not bring back a removed row")
		require.True(t, rows[addedSource].Discovered, "the concurrent addition survives")
	})

	// The chat is rebound and its rows cleared while the refresh probe is in flight.
	t.Run("DiscardsTheAnswerAfterARebind", func(t *testing.T) {
		t.Parallel()
		op := newRefreshOp(t, resolvedInstructionFile(nestedSource, "site rules v1"))
		op.conn.EXPECT().ResolveContextInstructions(gomock.Any(), gomock.Any()).
			DoAndReturn(func(context.Context, workspacesdk.ResolveContextInstructionsRequest) (workspacesdk.ResolveContextInstructionsResponse, error) {
				_, err := op.fix.db.UpdateChatBuildAgentBinding(op.fix.ctx, database.UpdateChatBuildAgentBindingParams{
					ID:      op.chat.ID,
					BuildID: op.chat.BuildID,
					AgentID: uuid.NullUUID{UUID: op.fix.agentB, Valid: true},
				})
				require.NoError(t, err)
				require.NoError(t, op.fix.db.DeleteChatContextResourcesByChatID(op.fix.ctx, database.DeleteChatContextResourcesByChatIDParams{ChatID: op.chat.ID}))
				return workspacesdk.ResolveContextInstructionsResponse{Files: []workspacesdk.ContextInstructionFile{
					resolvedInstructionFile(nestedSource, "site rules v2"),
					resolvedInstructionFile(secondSource, "claude rules"),
				}}, nil
			})

		refresh(t, op)

		require.Empty(t, op.rows(t), "the previous agent's files are not pinned onto the rebound chat")
	})
}

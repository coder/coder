package chattool

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
)

func TestValidateMemoryPath(t *testing.T) {
	t.Parallel()

	for _, valid := range []string{
		"/notes.md",
		"/projects/Coder/設計.md",
		"/case/README.md",
	} {
		valid := valid
		t.Run("valid "+valid, func(t *testing.T) {
			t.Parallel()
			require.NoError(t, validateMemoryPath(valid))
		})
	}

	for _, invalid := range []string{
		"",
		"notes.md",
		"/",
		"/notes.MD",
		"/.md",
		"/a/../notes.md",
		"/a/./notes.md",
		"/a//notes.md",
		"/notes.md/",
		"/notes.md\n",
		"/\xff.md",
	} {
		invalid := invalid
		t.Run("invalid "+invalid, func(t *testing.T) {
			t.Parallel()
			require.Error(t, validateMemoryPath(invalid))
		})
	}
}

func TestCompileMemoryGlob(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pattern string
		match   []string
		miss    []string
	}{
		{
			name:    "segment wildcards",
			pattern: "/projects/?oder/*.md",
			match:   []string{"/projects/Coder/a.md", "/projects/coder/.md"},
			miss:    []string{"/projects/Coder/a/b.md", "/projects/Coder/a.MD"},
		},
		{
			name:    "recursive wildcard",
			pattern: "/projects/**/notes.md",
			match:   []string{"/projects/notes.md", "/projects/a/notes.md", "/projects/a/b/notes.md"},
			miss:    []string{"/other/notes.md", "/projects/a/notes.md/child"},
		},
		{
			name:    "escaped metacharacters",
			pattern: `/literal/\*\?.md`,
			match:   []string{"/literal/*?.md"},
			miss:    []string{"/literal/ab.md"},
		},
		{
			name:    "case sensitive",
			pattern: "/Case/*.md",
			match:   []string{"/Case/a.md"},
			miss:    []string{"/case/a.md"},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			expression, err := compileMemoryGlob(test.pattern)
			require.NoError(t, err)
			compiled := regexp.MustCompile(expression)
			for _, path := range test.match {
				require.Truef(t, compiled.MatchString(path), "expected %q to match %q", path, expression)
			}
			for _, path := range test.miss {
				require.Falsef(t, compiled.MatchString(path), "expected %q not to match %q", path, expression)
			}
		})
	}

	for _, invalid := range []string{
		"",
		"relative/*.md",
		"/trailing/",
		"/repeated//*.md",
		"/dot/../*.md",
		"/bad/**x/*.md",
		"/bad/x**/*.md",
		"/bad/escape\\",
		"/bad/\n/*.md",
		"/\xff/*.md",
	} {
		_, err := compileMemoryGlob(invalid)
		require.Errorf(t, err, "expected %q to be invalid", invalid)
	}
}

func TestCompileMemoryDirectoryGlob(t *testing.T) {
	t.Parallel()

	expression, err := compileMemoryDirectoryGlob("/projects/*")
	require.NoError(t, err)
	compiled := regexp.MustCompile(expression)
	require.True(t, compiled.MatchString("/projects/a"))
	require.True(t, compiled.MatchString("/projects/a/nested"))
	require.False(t, compiled.MatchString("/projects"))

	expression, err = compileMemoryDirectoryGlob("/")
	require.NoError(t, err)
	compiled = regexp.MustCompile(expression)
	require.True(t, compiled.MatchString("/"))
	require.True(t, compiled.MatchString("/any/depth"))
}

func TestRenderMemoryHashlines(t *testing.T) {
	t.Parallel()

	require.Equal(t, "1:e3b|", renderMemoryHashlines(""))
	require.Equal(t, "1:2cf|hello\r\n2:e3b|\r\n3:486|world\n", renderMemoryHashlines("hello\r\n\r\nworld\n"))
	require.Equal(t, "ca9", memoryLineHash("a"))
}

func TestApplyMemoryEdits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		edits   []memoryEdit
		want    string
	}{
		{
			name:    "set line",
			content: "a\nb\nc",
			edits:   []memoryEdit{{Op: "set_line", Anchor: "2:3e2", NewText: "x"}},
			want:    "a\nx\nc",
		},
		{
			name:    "replace inclusive range with multiline text",
			content: "a\nb\nc",
			edits: []memoryEdit{{
				Op: "replace_range", StartAnchor: "2:3e2", EndAnchor: "3:2e7", NewText: "x\ny",
			}},
			want: "a\nx\ny",
		},
		{
			name:    "insert before",
			content: "a\nb\nc",
			edits:   []memoryEdit{{Op: "insert_before", Anchor: "2:3e2", NewText: "x"}},
			want:    "a\nx\nb\nc",
		},
		{
			name:    "insert after preserves mixed separator",
			content: "a\r\nb\nc",
			edits:   []memoryEdit{{Op: "insert_after", Anchor: "2:3e2", NewText: "x"}},
			want:    "a\r\nb\nx\r\nc",
		},
		{
			name:    "delete line",
			content: "a\nb\nc",
			edits:   []memoryEdit{{Op: "delete_line", Anchor: "2:3e2"}},
			want:    "a\nc",
		},
		{
			name:    "delete range preserves separator on preceding line",
			content: "a\nb\nc",
			edits: []memoryEdit{{
				Op: "delete_range", StartAnchor: "2:3e2", EndAnchor: "3:2e7",
			}},
			want: "a\n",
		},
		{
			name:    "preserve final newline on replacement",
			content: "a\n",
			edits:   []memoryEdit{{Op: "set_line", Anchor: "1:ca9", NewText: "x"}},
			want:    "x\n",
		},
		{
			name:    "insertions sharing boundary follow request order",
			content: "a\nb",
			edits: []memoryEdit{
				{Op: "insert_before", Anchor: "2:3e2", NewText: "first"},
				{Op: "insert_after", Anchor: "1:ca9", NewText: "second"},
			},
			want: "a\nfirst\nsecond\nb",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := applyMemoryEdits(test.content, test.edits)
			require.NoError(t, err)
			require.Equal(t, test.want, got)
		})
	}
}

func TestApplyMemoryEditsRejectsInvalidBatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		edits []memoryEdit
		err   string
	}{
		{
			name:  "stale hash",
			edits: []memoryEdit{{Op: "set_line", Anchor: "2:000", NewText: "x"}},
			err:   `stale anchor "2:000"; current anchor is "2:3e2"`,
		},
		{
			name:  "missing line",
			edits: []memoryEdit{{Op: "delete_line", Anchor: "9:000"}},
			err:   `stale anchor "9:000"; line no longer exists`,
		},
		{
			name: "overlapping ranges",
			edits: []memoryEdit{
				{Op: "set_line", Anchor: "2:3e2", NewText: "x"},
				{Op: "delete_range", StartAnchor: "1:ca9", EndAnchor: "2:3e2"},
			},
			err: "memory edit ranges overlap",
		},
		{
			name: "insertion inside range",
			edits: []memoryEdit{
				{Op: "delete_range", StartAnchor: "1:ca9", EndAnchor: "2:3e2"},
				{Op: "insert_before", Anchor: "2:3e2", NewText: "x"},
			},
			err: "insertion anchor is inside a replaced or deleted range",
		},
		{
			name:  "reversed range",
			edits: []memoryEdit{{Op: "delete_range", StartAnchor: "2:3e2", EndAnchor: "1:ca9"}},
			err:   "range start must not be after range end",
		},
		{
			name:  "wrong fields",
			edits: []memoryEdit{{Op: "delete_line", Anchor: "1:ca9", NewText: "x"}},
			err:   "delete_line does not accept new_text",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := applyMemoryEdits("a\nb\nc", test.edits)
			require.Empty(t, got)
			require.EqualError(t, err, test.err)
		})
	}
}

func TestMemoryExcerpts(t *testing.T) {
	t.Parallel()

	content := "one\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nnine\nten\neleven\ntwelve"
	headline := "one\n<memory-hit>two</memory-hit>\nthree\n<memory-hit>four</memory-hit>\nfive\nsix\nseven\neight\nnine\n<memory-hit>ten</memory-hit>\neleven\ntwelve"
	excerpts := memoryExcerpts(content, headline)
	require.Len(t, excerpts, 2)
	require.Equal(t, 1, excerpts[0].StartLine)
	require.Equal(t, 5, excerpts[0].EndLine)
	require.Contains(t, excerpts[0].Content, "2:3fc|two")
	require.Equal(t, 9, excerpts[1].StartLine)
	require.Equal(t, 11, excerpts[1].EndLine)

	excerpts = memoryExcerpts(content, content)
	require.Len(t, excerpts, 1)
	require.Equal(t, 1, excerpts[0].StartLine)
	require.Equal(t, 1, excerpts[0].EndLine)
	require.NotContains(t, excerpts[0].Content, "two")
}

func TestMemoryToolSchemas(t *testing.T) {
	t.Parallel()

	tools := append(MemoryReadTools(MemoryToolsOptions{}), MemoryWriteTools(MemoryToolsOptions{})...)
	require.Equal(t, []string{
		"read_memory", "search_memories", "list_memories", "write_memory", "edit_memory",
	}, memoryToolNames(tools))

	wantRequired := map[string][]string{
		"read_memory":     {"path"},
		"write_memory":    {"path", "content"},
		"edit_memory":     {"path", "edits"},
		"search_memories": {"keywords", "paths"},
		"list_memories":   {"directory"},
	}
	for _, tool := range tools {
		info := tool.Info()
		slices.Sort(info.Required)
		want := wantRequired[info.Name]
		slices.Sort(want)
		require.Equal(t, want, info.Required, info.Name)
	}
}

func TestMemoryToolsIntegration(t *testing.T) {
	t.Parallel()

	opts := newMemoryToolTestOptions(t)

	write := writeMemoryTool(opts)
	response := runMemoryTool(t, write, map[string]any{
		"path": "/projects/notes.md", "content": "first\nsecond\n",
	})
	require.False(t, response.IsError)
	var created memoryView
	require.NoError(t, json.Unmarshal([]byte(response.Content), &created))
	require.Equal(t, "/projects/notes.md", created.Path)
	require.Equal(t, "notes.md", created.Name)
	require.Equal(t, "1:a79|first\n2:163|second\n", created.Content)

	response = runMemoryTool(t, write, map[string]any{
		"path": "/projects/notes.md", "content": "overwrite",
	})
	require.True(t, response.IsError)
	require.Contains(t, response.Content, "already exists")

	read := readMemoryTool(opts)
	response = runMemoryTool(t, read, map[string]any{"path": "/projects/notes.md"})
	require.False(t, response.IsError)
	var readBack memoryView
	require.NoError(t, json.Unmarshal([]byte(response.Content), &readBack))
	require.Equal(t, created.Content, readBack.Content)

	edit := editMemoryTool(opts)
	response = runMemoryTool(t, edit, map[string]any{
		"path": "/projects/notes.md",
		"edits": []map[string]any{{
			"op": "set_line", "anchor": "2:163", "new_text": "updated",
		}},
	})
	require.False(t, response.IsError)
	var updated memoryView
	require.NoError(t, json.Unmarshal([]byte(response.Content), &updated))
	require.Equal(t, "1:a79|first\n2:27e|updated\n", updated.Content)

	search := searchMemoriesTool(opts)
	response = runMemoryTool(t, search, map[string]any{
		"keywords": "updated", "paths": []string{"/projects/**"},
	})
	require.False(t, response.IsError)
	var searchShape map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(response.Content), &searchShape))
	require.Equal(t, []string{"matches"}, slices.Sorted(maps.Keys(searchShape)))
	var searchResult struct {
		Matches []memorySearchMatch `json:"matches"`
	}
	require.NoError(t, json.Unmarshal([]byte(response.Content), &searchResult))
	require.Len(t, searchResult.Matches, 1)
	require.Contains(t, searchResult.Matches[0].Excerpts[0].Content, "2:27e|updated")

	list := listMemoriesTool(opts)
	response = runMemoryTool(t, list, map[string]any{"directory": "/projects"})
	require.False(t, response.IsError)
	var listShape map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(response.Content), &listShape))
	require.Equal(t, []string{"memories"}, slices.Sorted(maps.Keys(listShape)))
	var listResult struct {
		Memories []memoryListEntry `json:"memories"`
	}
	require.NoError(t, json.Unmarshal([]byte(response.Content), &listResult))
	require.Len(t, listResult.Memories, 1)
	require.Equal(t, len("first\nupdated\n"), listResult.Memories[0].SizeBytes)
}

func TestMemorySearchKeywordsQuery(t *testing.T) {
	t.Parallel()

	query, err := memorySearchKeywordsQuery("  postgres   postgresql database  ")
	require.NoError(t, err)
	require.Equal(t, "postgres OR postgresql OR database", query)

	query, err = memorySearchKeywordsQuery("needle")
	require.NoError(t, err)
	require.Equal(t, "needle", query)

	_, err = memorySearchKeywordsQuery("   ")
	require.EqualError(t, err, "keywords is required")
}

func TestMemorySearchExcerptsPreserveSourceLines(t *testing.T) {
	t.Parallel()

	opts := newMemoryToolTestOptions(t)
	lines := make([]string, 80)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d", i+1)
	}
	// Stored Markdown may contain the same text used to mark headline hits.
	// These literals must not be mistaken for matches.
	lines[0] = "<memory-hit>literal one</memory-hit>"
	lines[1] = "<memory-hit>literal two</memory-hit>"
	lines[2] = "<memory-hit>literal three</memory-hit>"
	lines[69] = "line 70 needle"

	response := runMemoryTool(t, writeMemoryTool(opts), map[string]any{
		"path": "/late-match.md", "content": strings.Join(lines, "\n"),
	})
	require.False(t, response.IsError)

	response = runMemoryTool(t, searchMemoriesTool(opts), map[string]any{
		"keywords": "needle", "paths": []string{"/late-match.md"},
	})
	require.False(t, response.IsError)
	var result struct {
		Matches []memorySearchMatch `json:"matches"`
	}
	require.NoError(t, json.Unmarshal([]byte(response.Content), &result))
	require.Len(t, result.Matches, 1)
	require.Len(t, result.Matches[0].Excerpts, 1)
	require.Equal(t, 69, result.Matches[0].Excerpts[0].StartLine)
	require.Equal(t, 71, result.Matches[0].Excerpts[0].EndLine)
	require.Contains(t, result.Matches[0].Excerpts[0].Content,
		fmt.Sprintf("70:%s|%s", memoryLineHash(lines[69]), lines[69]))
}

func TestMemorySearchORsSpaceSeparatedKeywords(t *testing.T) {
	t.Parallel()

	opts := newMemoryToolTestOptions(t)
	for _, memory := range []struct {
		path    string
		content string
	}{
		{"/alpha.md", "PostgreSQL keeps durable memory."},
		{"/beta.md", "Templates keep searchable memory."},
		{"/gamma.md", "Unrelated networking notes."},
	} {
		response := runMemoryTool(t, writeMemoryTool(opts), map[string]any{
			"path": memory.path, "content": memory.content,
		})
		require.False(t, response.IsError)
	}

	response := runMemoryTool(t, searchMemoriesTool(opts), map[string]any{
		"keywords": "durable templates", "paths": []string{},
	})
	require.False(t, response.IsError)
	var result struct {
		Matches []memorySearchMatch `json:"matches"`
	}
	require.NoError(t, json.Unmarshal([]byte(response.Content), &result))
	paths := make([]string, 0, len(result.Matches))
	for _, match := range result.Matches {
		paths = append(paths, match.Path)
	}
	require.Equal(t, []string{"/alpha.md", "/beta.md"}, paths)
}

func TestMemoryToolsConcurrentEdits(t *testing.T) {
	t.Parallel()

	opts := newMemoryToolTestOptions(t)
	response := runMemoryTool(t, writeMemoryTool(opts), map[string]any{
		"path": "/concurrent.md", "content": "original",
	})
	require.False(t, response.IsError)

	input, err := json.Marshal(map[string]any{
		"path": "/concurrent.md",
		"edits": []map[string]any{{
			"op": "set_line", "anchor": "1:068", "new_text": "updated",
		}},
	})
	require.NoError(t, err)

	tool := editMemoryTool(opts)
	start := make(chan struct{})
	responses := make(chan fantasy.ToolResponse, 2)
	errors := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			response, runErr := tool.Run(context.Background(), fantasy.ToolCall{
				ID: uuid.NewString(), Name: tool.Info().Name, Input: string(input),
			})
			responses <- response
			errors <- runErr
		}()
	}
	close(start)
	wg.Wait()
	close(responses)
	close(errors)

	for runErr := range errors {
		require.NoError(t, runErr)
	}
	var succeeded, stale int
	for response := range responses {
		if response.IsError {
			require.Contains(t, response.Content, "stale anchor")
			stale++
		} else {
			succeeded++
		}
	}
	require.Equal(t, 1, succeeded)
	require.Equal(t, 1, stale)
}

func memoryToolNames(tools []fantasy.AgentTool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Info().Name)
	}
	return names
}

func runMemoryTool(t *testing.T, tool fantasy.AgentTool, args any) fantasy.ToolResponse {
	t.Helper()
	input, err := json.Marshal(args)
	require.NoError(t, err)
	response, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID: uuid.NewString(), Name: tool.Info().Name, Input: string(input),
	})
	require.NoError(t, err)
	return response
}

func newMemoryToolTestOptions(t *testing.T) MemoryToolsOptions {
	t.Helper()
	db, _ := dbtestutil.NewDB(t)
	now := time.Now()
	userID := uuid.New()
	user, err := db.InsertUser(context.Background(), database.InsertUserParams{
		ID:             userID,
		Email:          userID.String() + "@example.com",
		Username:       "memory-" + userID.String(),
		Name:           "Memory Test",
		HashedPassword: []byte("hashed password"),
		CreatedAt:      now,
		UpdatedAt:      now,
		RBACRoles:      pq.StringArray{},
		LoginType:      database.LoginTypePassword,
		Status:         string(database.UserStatusActive),
	})
	require.NoError(t, err)
	return MemoryToolsOptions{DB: db, UserID: user.ID}
}

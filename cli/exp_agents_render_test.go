package cli //nolint:testpackage // Tests unexported chat TUI render helpers.

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

var ansiRegexp = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func TestExpAgentsRender(t *testing.T) {
	t.Parallel()

	styles := newTUIStyles()

	t.Run("MessagesToBlocks", func(t *testing.T) {
		t.Parallel()

		user, assistant, tool := codersdk.ChatMessageRoleUser, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageRoleTool
		msg := func(role codersdk.ChatMessageRole, parts ...codersdk.ChatMessagePart) codersdk.ChatMessage {
			return codersdk.ChatMessage{Role: role, Content: parts}
		}
		text := func(body string) codersdk.ChatMessagePart {
			return codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: body}
		}
		reasoning := func(body string) codersdk.ChatMessagePart {
			return codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeReasoning, Text: body}
		}
		call := func(name, id, args string) codersdk.ChatMessagePart {
			return codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeToolCall, ToolName: name, ToolCallID: id, Args: rawJSON(args)}
		}
		result := func(name, id, body string, isError bool) codersdk.ChatMessagePart {
			return codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeToolResult, ToolName: name, ToolCallID: id, Result: rawJSON(body), IsError: isError}
		}

		tests := []struct {
			name string
			in   []codersdk.ChatMessage
			want []chatBlock
		}{
			{name: "EmptyMessages", want: []chatBlock{}},
			{name: "UserText", in: []codersdk.ChatMessage{msg(user, text("hello"))}, want: []chatBlock{{kind: blockText, role: user, text: "hello"}}},
			{name: "AssistantText", in: []codersdk.ChatMessage{msg(assistant, text("hi there"))}, want: []chatBlock{{kind: blockText, role: assistant, text: "hi there"}}},
			{name: "ToolCallPart", in: []codersdk.ChatMessage{msg(assistant, call("weather", "call-1", `{"city":"SF"}`))}, want: []chatBlock{{kind: blockToolCall, role: assistant, toolName: "weather", toolID: "call-1", args: `{"city":"SF"}`}}},
			{name: "ToolResultPart", in: []codersdk.ChatMessage{msg(tool, result("weather", "call-1", `{"temp":"68F"}`, true))}, want: []chatBlock{{kind: blockToolResult, role: tool, toolName: "weather", toolID: "call-1", result: `{"temp":"68F"}`, isError: true}}},
			{
				name: "MultipleMessagesInOrder",
				in: []codersdk.ChatMessage{
					msg(user, text("question")),
					msg(assistant, reasoning("thinking"), call("search", "call-3", `{"q":"docs"}`), text("answer")),
				},
				want: []chatBlock{
					{kind: blockText, role: user, text: "question"},
					{kind: blockReasoning, role: assistant, text: "thinking"},
					{kind: blockToolCall, role: assistant, toolName: "search", toolID: "call-3", args: `{"q":"docs"}`},
					{kind: blockText, role: assistant, text: "answer"},
				},
			},
		}

		for _, tt := range tests {
			tt := tt
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				require.Equal(t, tt.want, messagesToBlocks(tt.in))
			})
		}

		t.Run("KeepsToolCallsAndLaterResultsSeparateByToolID", func(t *testing.T) {
			t.Parallel()

			blocks := messagesToBlocks([]codersdk.ChatMessage{
				msg(assistant,
					call("github__get_pull_request", "call-1", `{"owner":"openclaw","repo":"openclaw","pull_number":58036}`),
					call("github__get_pull_request", "call-2", `{"owner":"openclaw","repo":"openclaw","pull_number":58037}`),
				),
				msg(tool,
					result("github__get_pull_request", "call-1", `{"base":{"ref":"main"}}`, false),
					result("github__get_pull_request", "call-2", `{"base":{"ref":"main"}}`, false),
				),
			})

			require.Len(t, blocks, 4)
			require.Equal(t,
				[]chatBlockKind{blockToolCall, blockToolCall, blockToolResult, blockToolResult},
				[]chatBlockKind{blocks[0].kind, blocks[1].kind, blocks[2].kind, blocks[3].kind},
			)
			require.Equal(t, []string{"call-1", "call-2", "call-1", "call-2"}, []string{blocks[0].toolID, blocks[1].toolID, blocks[2].toolID, blocks[3].toolID})
		})
	})

	t.Run("MergeConsecutiveToolBlocks", func(t *testing.T) {
		t.Parallel()

		assistant, tool := codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageRoleTool
		call := func(name, id, args string) chatBlock {
			return chatBlock{kind: blockToolCall, role: assistant, toolName: name, toolID: id, args: args}
		}
		result := func(name, id, body string) chatBlock {
			return chatBlock{kind: blockToolResult, role: tool, toolName: name, toolID: id, result: body}
		}

		for _, tt := range []struct {
			name string
			in   []chatBlock
			want []chatBlock
		}{
			{
				name: "MergesAdjacentEmptyToolIDCallAndResult",
				in:   []chatBlock{call("read_file", "", `{"path":"main.go"}`), result("read_file", "", `{"content":"hello"}`)},
				want: []chatBlock{{kind: blockToolResult, role: tool, toolName: "read_file", toolID: "", args: `{"path":"main.go"}`, result: `{"content":"hello"}`}},
			},
			{
				name: "ExistingToolIDMergeStillWorks",
				in:   []chatBlock{call("read_file", "call-1", `{"path":"main.go"}`), result("read_file", "call-1", `{"content":"hello"}`)},
				want: []chatBlock{{kind: blockToolResult, role: tool, toolName: "read_file", toolID: "call-1", args: `{"path":"main.go"}`, result: `{"content":"hello"}`}},
			},
			{
				name: "MultiplePairs",
				in: []chatBlock{
					call("read_file", "call-1", `{"path":"one.txt"}`),
					result("read_file", "call-1", `{"ok":true}`),
					call("list_dir", "call-2", `{"path":"/tmp"}`),
					result("list_dir", "call-2", `{"entries":[]}`),
				},
				want: []chatBlock{
					{kind: blockToolResult, role: tool, toolName: "read_file", toolID: "call-1", args: `{"path":"one.txt"}`, result: `{"ok":true}`},
					{kind: blockToolResult, role: tool, toolName: "list_dir", toolID: "call-2", args: `{"path":"/tmp"}`, result: `{"entries":[]}`},
				},
			},
		} {
			tt := tt
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				got := mergeConsecutiveToolBlocks(tt.in)
				require.Equal(t, tt.want, got)
			})
		}

		t.Run("NegativeMergeCases", func(t *testing.T) {
			t.Parallel()

			for _, tt := range []struct {
				name string
				in   []chatBlock
				want []chatBlock
			}{
				{
					name: "DifferentToolNames",
					in:   []chatBlock{call("read_file", "", `{"path":"main.go"}`), result("list_dir", "", `{"entries":[]}`)},
					want: []chatBlock{call("read_file", "", `{"path":"main.go"}`), result("list_dir", "", `{"entries":[]}`)},
				},
				{
					name: "NonAdjacentEmptyToolID",
					in:   []chatBlock{call("read_file", "", `{"path":"main.go"}`), {kind: blockText, role: assistant, text: "still thinking"}, result("read_file", "", `{"content":"hello"}`)},
					want: []chatBlock{call("read_file", "", `{"path":"main.go"}`), {kind: blockText, role: assistant, text: "still thinking"}, result("read_file", "", `{"content":"hello"}`)},
				},
				{
					name: "NonAdjacentMatchingToolID",
					in:   []chatBlock{call("read_file", "call-1", `{"path":"main.go"}`), {kind: blockText, role: assistant, text: "still thinking"}, result("read_file", "call-1", `{"content":"hello"}`)},
					want: []chatBlock{call("read_file", "call-1", `{"path":"main.go"}`), {kind: blockText, role: assistant, text: "still thinking"}, result("read_file", "call-1", `{"content":"hello"}`)},
				},
				{
					name: "OrphanedCall",
					in:   []chatBlock{call("read_file", "call-orphan", `{"path":"solo.txt"}`)},
					want: []chatBlock{call("read_file", "call-orphan", `{"path":"solo.txt"}`)},
				},
				{
					name: "OrphanedResult",
					in:   []chatBlock{result("read_file", "call-orphan", `{"content":"hello"}`)},
					want: []chatBlock{result("read_file", "call-orphan", `{"content":"hello"}`)},
				},
			} {
				tt := tt
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()
					got := mergeConsecutiveToolBlocks(tt.in)
					require.Equal(t, tt.want, got)
				})
			}
		})
	})

	t.Run("ToolArgsSummary", func(t *testing.T) {
		t.Parallel()

		for _, tt := range []struct {
			name     string
			toolName string
			args     string
			assert   func(t *testing.T, summary string)
		}{
			{name: "CreateWorkspaceUsesNameField", toolName: "coder_create_workspace", args: `{"name":"my-workspace"}`, assert: func(t *testing.T, summary string) { require.Equal(t, "(my-workspace)", summary) }},
			{name: "CreateWorkspaceUsesWorkspaceNameField", toolName: "coder_create_workspace", args: `{"workspace_name":"my-ws","template":"docker"}`, assert: func(t *testing.T, summary string) { require.Equal(t, "(my-ws)", summary) }},
			{name: "WithUnicodeTruncatesOnRuneBoundary", toolName: "weather", args: strings.Repeat("こんにちは世界", 10), assert: func(t *testing.T, summary string) {
				require.NotEmpty(t, summary)
				require.True(t, utf8.ValidString(summary))
				require.True(t, strings.HasSuffix(summary, "…"))
				require.LessOrEqual(t, len([]rune(summary)), toolSummaryFallbackWidth)
				require.Contains(t, summary, "こんにちは")
			}},
		} {
			tt := tt
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				tt.assert(t, toolArgsSummary(tt.toolName, tt.args))
			})
		}
		require.Equal(t, "(created-ws)", toolResultSummary("coder_create_workspace", "", `{"workspace_name":"created-ws"}`))
	})
	t.Run("RenderToolCall", func(t *testing.T) {
		t.Parallel()

		for _, tt := range []struct {
			name   string
			part   codersdk.ChatMessagePart
			width  int
			assert func(t *testing.T, output string)
		}{
			{name: "ShowsHumanizedToolNameAndContext", part: codersdk.ChatMessagePart{ToolName: "github__get_pull_request", Args: rawJSON(`{"owner":"openclaw","repo":"openclaw","pull_number":58036}`)}, width: 60, assert: func(t *testing.T, output string) {
				require.Contains(t, output, "  ○ get pull request")
				require.Contains(t, output, "(openclaw/openclaw)")
			}},
			{name: "ShowsTruncatedCommandPreview", part: codersdk.ChatMessagePart{ToolName: "coder_execute_command", Args: rawJSON(`{"command":"ls -la /tmp/with/a/very/long/path"}`)}, width: 30, assert: func(t *testing.T, output string) {
				require.Contains(t, output, "○ execute command")
				require.Contains(t, output, `"ls -la`)
				require.Contains(t, output, "…")
			}},
			{name: "ContextCompactionRendersBanner", part: codersdk.ChatMessagePart{ToolName: contextCompactionToolName}, width: 40, assert: func(t *testing.T, output string) {
				require.Contains(t, output, "🗜️  Context compacted")
				require.NotContains(t, output, pendingToolIcon)
			}},
		} {
			tt := tt
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				var output string
				require.NotPanics(t, func() {
					output = plainText(renderToolCallBlock(styles, chatBlock{
						kind:     blockToolCall,
						toolName: tt.part.ToolName,
						args:     compactTranscriptJSON(tt.part.Args),
					}, tt.width))
				})
				tt.assert(t, output)
			})
		}
	})
	t.Run("RenderToolResult", func(t *testing.T) {
		t.Parallel()

		for _, tt := range []struct {
			name   string
			part   codersdk.ChatMessagePart
			width  int
			assert func(t *testing.T, rawOutput, plainOutput string)
		}{
			{name: "SuccessShowsCheckPrefixAndArgsContext", part: codersdk.ChatMessagePart{ToolName: "coder_execute_command", Args: rawJSON(`{"command":"ls -la"}`), Result: rawJSON(`{"ok":true}`)}, width: 40, assert: func(t *testing.T, _, output string) {
				require.Contains(t, output, "✓ execute command")
				require.Contains(t, output, `"ls -la"`)
			}},
			{name: "ErrorShowsErrorStyleAndMessage", part: codersdk.ChatMessagePart{ToolName: "coder_execute_command", Result: rawJSON(`{"error":"command not found"}`), IsError: true}, width: 40, assert: func(t *testing.T, rawOutput, plainOutput string) {
				require.Contains(t, rawOutput, styles.errorText.Render("✗ execute command"))
				require.Contains(t, plainOutput, `"command not found"`)
			}},
			{name: "MergedCreateWorkspaceResultKeepsArgsSummary", part: codersdk.ChatMessagePart{ToolName: "coder_create_workspace", ToolCallID: "call-create-workspace", Args: rawJSON(`{"name":"merged-workspace"}`), Result: rawJSON(`{"workspace_name":"merged-workspace","status":"created"}`)}, width: 60, assert: func(t *testing.T, _, output string) {
				require.Contains(t, output, "✓ create workspace")
				require.Contains(t, output, "(merged-workspace)")
			}},
			{name: "ContextCompactionRendersBanner", part: codersdk.ChatMessagePart{ToolName: contextCompactionToolName}, width: 40, assert: func(t *testing.T, _, output string) {
				require.Contains(t, output, "🗜️  Context compacted")
				require.NotContains(t, output, "✓")
			}},
		} {
			tt := tt
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				var rawOutput string
				require.NotPanics(t, func() {
					rawOutput = renderToolResultBlock(styles, chatBlock{
						kind:     blockToolResult,
						toolName: tt.part.ToolName,
						args:     compactTranscriptJSON(tt.part.Args),
						result:   compactTranscriptJSON(tt.part.Result),
						isError:  tt.part.IsError,
					}, tt.width)
				})
				tt.assert(t, rawOutput, plainText(rawOutput))
			})
		}
	})
	t.Run("RenderCompaction", func(t *testing.T) {
		t.Parallel()

		output := plainText(renderCompaction(styles, 20))
		require.Contains(t, output, "🗜️  Context compacted")
	})
	t.Run("RenderStatusBar", func(t *testing.T) {
		t.Parallel()

		u := func(total, limit int64) *codersdk.ChatMessageUsage {
			return &codersdk.ChatMessageUsage{TotalTokens: int64Ptr(total), ContextLimit: int64Ptr(limit)}
		}

		for _, tt := range []struct {
			name                       string
			status                     codersdk.ChatStatus
			usage                      *codersdk.ChatMessageUsage
			queue                      int
			interrupting, reconnecting bool
			width, maxWidth            int
			wantRaw                    string
			wantPlain, avoidPlain      []string
		}{
			{name: "RunningOmitsUsageWhenNil", status: codersdk.ChatStatusRunning, width: 80, avoidPlain: []string{"tokens:"}},
			{name: "RunningShowsTokenUsage", status: codersdk.ChatStatusRunning, usage: u(50, 100), width: 80, wantPlain: []string{"tokens: 50/100"}},
			{name: "RunningWarnsAndShowsTransientStates", status: codersdk.ChatStatusRunning, usage: u(81, 100), interrupting: true, reconnecting: true, width: 80, wantRaw: styles.warningText.Render("tokens: 81/100"), wantPlain: []string{"interrupting…", "reconnecting…"}},
			{name: "RunningShowsCriticalUsage", status: codersdk.ChatStatusRunning, usage: u(96, 100), width: 80, wantRaw: styles.criticalText.Render("tokens: 96/100")},
			{name: "PendingShowsQueue", status: codersdk.ChatStatusPending, queue: 2, width: 80, wantPlain: []string{"queued: 2"}},
			{name: "NarrowWidthFits", status: codersdk.ChatStatusRunning, usage: u(96, 100), queue: 2, interrupting: true, reconnecting: true, width: 20, maxWidth: 20},
		} {
			tt := tt
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				var output string
				require.NotPanics(t, func() {
					output = renderStatusBar(styles, nil, tt.status, tt.usage, tt.queue, tt.interrupting, tt.reconnecting, tt.width)
				})
				plain := plainText(output)
				require.Contains(t, output, styles.statusColor(tt.status).Render(string(tt.status)))
				if tt.wantRaw != "" {
					require.Contains(t, output, tt.wantRaw)
				}
				for _, want := range tt.wantPlain {
					require.Contains(t, plain, want)
				}
				for _, avoid := range tt.avoidPlain {
					require.NotContains(t, plain, avoid)
				}
				if tt.maxWidth > 0 {
					require.NotEmpty(t, plain)
					require.LessOrEqual(t, lipgloss.Width(plain), tt.maxWidth)
					require.LessOrEqual(t, lipgloss.Width(output), tt.width)
				}
			})
		}
	})
	t.Run("RenderBlock", func(t *testing.T) {
		t.Parallel()

		renderOutput := func(block chatBlock, expanded, plain bool, width int) string {
			output := renderBlock(styles, block, expanded, width)
			if plain {
				return plainText(output)
			}
			return output
		}
		assertOutput := func(t *testing.T, output string, want, avoid []string, lines int, lastLine string) {
			t.Helper()
			for _, s := range want {
				require.Contains(t, output, s)
			}
			for _, s := range avoid {
				require.NotContains(t, output, s)
			}
			if lines > 0 {
				split := strings.Split(output, "\n")
				require.Len(t, split, lines)
				if lastLine != "" {
					require.Equal(t, lastLine, strings.TrimRight(split[len(split)-1], " "))
				}
			}
		}

		for _, tt := range []struct {
			name  string
			block chatBlock
			want  []string
			avoid []string
		}{
			{name: "UserIncludesYouPrefix", block: chatBlock{kind: blockText, role: codersdk.ChatMessageRoleUser, text: "hello"}, want: []string{"You: hello"}},
			{name: "AssistantRendersMarkdown", block: chatBlock{kind: blockText, role: codersdk.ChatMessageRoleAssistant, text: "- first\n- second"}, want: []string{"• first", "• second"}, avoid: []string{"- first"}},
			{name: "ToolRendersDimmed", block: chatBlock{kind: blockText, role: codersdk.ChatMessageRoleTool, text: "tool output"}, want: []string{styles.dimmedText.Render("tool output")}},
		} {
			tt := tt
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				assertOutput(t, renderOutput(tt.block, false, tt.block.role != codersdk.ChatMessageRoleTool, 40), tt.want, tt.avoid, 0, "")
			})
		}

		for _, tt := range []struct {
			name              string
			block             chatBlock
			width             int
			collapsedWant     []string
			collapsedAvoid    []string
			collapsedLines    int
			collapsedLastLine string
			expandedWant      []string
			expandedAvoid     []string
			expandedLines     int
			expandedLastLine  string
		}{
			{
				name:              "Reasoning",
				block:             chatBlock{kind: blockReasoning, role: codersdk.ChatMessageRoleAssistant, text: "line1\nline2\nline3\nline4"},
				width:             40,
				collapsedWant:     []string{"thinking: line1"},
				collapsedLines:    3,
				collapsedLastLine: "line3…",
				expandedWant:      []string{"line4"},
				expandedAvoid:     []string{"line4…"},
				expandedLines:     4,
			},
			{
				name:           "ToolCall",
				block:          chatBlock{kind: blockToolCall, toolName: "read_file", args: `{"path":"very/long/path.txt","recursive":true}`},
				width:          60,
				collapsedWant:  []string{"○ read file", "(very/long/path.txt)"},
				collapsedAvoid: []string{"\n", "args:"},
				expandedWant:   []string{"○ read file", "args:", `{"path":"very/long/path.txt","recursive":true}`, "\n"},
			},
			{
				name:           "ToolResult",
				block:          chatBlock{kind: blockToolResult, toolName: "read_file", args: `{"path":"a.txt"}`, result: `{"path":"a.txt","contents":"hello"}`},
				width:          60,
				collapsedWant:  []string{"✓ read file", "(a.txt)"},
				collapsedAvoid: []string{"\n", "result:"},
				expandedWant:   []string{"✓ read file", "args:", "result:", `{"path":"a.txt","contents":"hello"}`, "\n"},
			},
			{
				name:          "CollapsedToolCallShowsRunCount",
				block:         chatBlock{kind: blockToolCall, toolName: "github__get_pull_request", args: `{"owner":"openclaw","repo":"openclaw"}`, collapsedCount: 3},
				width:         80,
				collapsedWant: []string{"○ get pull request..."},
			},
			{
				name:          "CollapsedToolResultShowsRunCount",
				block:         chatBlock{kind: blockToolResult, toolName: "github__get_pull_request", args: `{"owner":"openclaw","repo":"openclaw"}`, result: `{"ok":true}`, collapsedCount: 10},
				width:         80,
				collapsedWant: []string{"✓ get pull request (x10)"},
			},
		} {
			tt := tt
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				collapsed := renderOutput(tt.block, false, true, tt.width)
				assertOutput(t, collapsed, tt.collapsedWant, tt.collapsedAvoid, tt.collapsedLines, tt.collapsedLastLine)
				if len(tt.expandedWant)+len(tt.expandedAvoid)+tt.expandedLines > 0 || tt.expandedLastLine != "" {
					expanded := renderOutput(tt.block, true, true, tt.width)
					assertOutput(t, expanded, tt.expandedWant, tt.expandedAvoid, tt.expandedLines, tt.expandedLastLine)
				}
			})
		}

		t.Run("CompactionRendersBanner", func(t *testing.T) {
			t.Parallel()

			output := plainText(renderBlock(styles, chatBlock{kind: blockCompaction}, false, 40))
			require.Contains(t, output, "🗜️  Context compacted")
		})
	})

	t.Run("RenderChatBlocks", func(t *testing.T) {
		t.Parallel()

		t.Run("MixedMessagesRenderInOrder", func(t *testing.T) {
			t.Parallel()

			blocks := []chatBlock{
				{kind: blockText, role: codersdk.ChatMessageRoleUser, text: "hello"},
				{kind: blockReasoning, role: codersdk.ChatMessageRoleAssistant, text: "thinking"},
				{kind: blockToolResult, toolName: "read_file", args: `{"path":"a.txt"}`, result: `{"path":"a.txt","contents":"hello"}`},
				{kind: blockText, role: codersdk.ChatMessageRoleAssistant, text: "done"},
			}

			output := plainText(renderChatBlocks(styles, blocks, -1, map[int]bool{}, true, 60))
			require.Contains(t, output, "You: hello")
			require.Contains(t, output, "thinking: thinking")
			require.Contains(t, output, "✓ read file")
			require.Contains(t, output, "done")
			require.Less(t, strings.Index(output, "You: hello"), strings.Index(output, "thinking: thinking"))
			require.Less(t, strings.Index(output, "thinking: thinking"), strings.Index(output, "✓ read file"))
			require.Less(t, strings.Index(output, "✓ read file"), strings.LastIndex(output, "done"))
		})

		t.Run("SelectedBlockUsesLeftBorderIndicator", func(t *testing.T) {
			t.Parallel()

			blocks := []chatBlock{{kind: blockText, role: codersdk.ChatMessageRoleAssistant, text: "assistant reply"}}

			output := plainText(renderChatBlocks(styles, blocks, 0, map[int]bool{}, false, 60))
			require.Contains(t, output, "│   assistant reply")
		})

		t.Run("CollapsesConsecutiveSameNameToolResults", func(t *testing.T) {
			t.Parallel()

			blocks := []chatBlock{
				{kind: blockToolResult, toolName: "github__get_pull_request", args: `{"owner":"openclaw","repo":"openclaw","pull_number":1}`, result: `{"base":{"ref":"main"}}`},
				{kind: blockToolResult, toolName: "github__get_pull_request", args: `{"owner":"openclaw","repo":"openclaw","pull_number":1}`, result: `{"base":{"ref":"main"}}`},
				{kind: blockToolResult, toolName: "github__get_pull_request", args: `{"owner":"openclaw","repo":"openclaw","pull_number":1}`, result: `{"base":{"ref":"main"}}`},
				{kind: blockToolResult, toolName: "create_file", args: `{"path":"main.go"}`, result: `{"ok":true}`},
			}

			output := plainText(renderChatBlocks(styles, blocks, -1, map[int]bool{}, true, 80))
			require.Equal(t, 2, strings.Count(output, "✓"))
			require.Contains(t, output, "get pull request (x3)")
			require.Contains(t, output, "create file")
		})

		t.Run("DoesNotCollapseDifferentToolResults", func(t *testing.T) {
			t.Parallel()

			blocks := []chatBlock{
				{kind: blockToolResult, toolName: "github__get_pull_request", args: `{"owner":"openclaw","repo":"openclaw","pull_number":1}`, result: `{"base":{"ref":"main"}}`},
				{kind: blockToolResult, toolName: "github__get_pull_request", args: `{"owner":"openclaw","repo":"openclaw","pull_number":2}`, result: `{"base":{"ref":"main"}}`},
				{kind: blockToolResult, toolName: "github__get_pull_request", args: `{"owner":"openclaw","repo":"openclaw","pull_number":3}`, result: `{"base":{"ref":"main"}}`},
				{kind: blockToolResult, toolName: "create_file", args: `{"path":"main.go"}`, result: `{"ok":true}`},
			}

			output := plainText(renderChatBlocks(styles, blocks, -1, map[int]bool{}, true, 80))
			require.Equal(t, 4, strings.Count(output, "✓"))
			require.NotContains(t, output, "get pull request (x3)")
			require.Contains(t, output, "create file")
		})

		t.Run("ExpandedToolBlockPreventsCollapse", func(t *testing.T) {
			t.Parallel()

			blocks := []chatBlock{
				{kind: blockToolResult, toolName: "github__get_pull_request", args: `{"owner":"openclaw","repo":"openclaw","pull_number":1}`, result: `{"base":{"ref":"main"}}`},
				{kind: blockToolResult, toolName: "github__get_pull_request", args: `{"owner":"openclaw","repo":"openclaw","pull_number":1}`, result: `{"base":{"ref":"main"}}`},
			}

			output := plainText(renderChatBlocks(styles, blocks, 1, map[int]bool{1: true}, false, 80))
			require.Equal(t, 2, strings.Count(output, "✓"))
			require.NotContains(t, output, "(x2)")
			require.Contains(t, output, "result:")
		})
	})
	t.Run("RenderDiffDrawer", func(t *testing.T) {
		t.Parallel()

		branch := "feature/chat-ui"
		prURL := "https://example.com/pulls/123"
		for _, tt := range []struct {
			name   string
			diff   codersdk.ChatDiffContents
			assert func(t *testing.T, output string)
		}{
			{name: "ShowsMetadataWhenPresent", diff: codersdk.ChatDiffContents{Branch: &branch, PullRequestURL: &prURL}, assert: func(t *testing.T, output string) {
				require.Contains(t, output, "Branch: feature/chat-ui")
				require.Contains(t, output, "PR: https://example.com/pulls/123")
			}},
			{name: "ShowsDiffContent", diff: codersdk.ChatDiffContents{Diff: "diff --git a/a.txt b/a.txt\n--- a/a.txt\n+++ b/a.txt\n@@ -1 +1 @@\n+added line"}, assert: func(t *testing.T, output string) {
				require.Contains(t, output, "1 file changed:")
				require.Contains(t, output, "modified a.txt (+1)")
				require.Contains(t, output, "diff --git a/a.txt b/a.txt")
				require.Contains(t, output, "+added line")
			}},
			{name: "ShowsPlaceholderForEmptyDiff", assert: func(t *testing.T, output string) {
				require.Contains(t, output, "No diff contents.")
				require.Contains(t, output, "No changes detected.")
			}},
			{name: "ShowsFallbackForUnparsableNonEmptyDiff", diff: codersdk.ChatDiffContents{Diff: "Total diff too large to show. Size: 12MB. Showing branch and remote only."}, assert: func(t *testing.T, output string) {
				// When agent/agentgit substitutes a placeholder for
				// an oversized diff, the text is non-empty but not in
				// `diff --git` format. renderChatDiffSummary should
				// report "Changes present but could not be summarized."
				// instead of claiming no changes were detected.
				require.Contains(t, output, "Changes present but could not be summarized.")
				require.NotContains(t, output, "No changes detected.")
			}},
			{name: "FlagsPartiallyUnparsableMultiRepoDiff", diff: codersdk.ChatDiffContents{Diff: "diff --git a/a.txt b/a.txt\n--- a/a.txt\n+++ b/a.txt\n@@ -1 +1 @@\n+added line\nTotal diff too large to show. Size: 12 MiB. Showing branch and remote only."}, assert: func(t *testing.T, output string) {
				// Multi-repo aggregates can legitimately interleave
				// real `diff --git` chunks from small repos with
				// agent/agentgit's oversize placeholder for repos
				// whose UnifiedDiff exceeded maxTotalDiffSize.
				// renderChatDiffSummary must both count the real
				// chunks and flag the omitted oversized repo, so
				// the user is not misled into thinking the files
				// listed are the whole changeset.
				require.Contains(t, output, "1 file changed:")
				require.Contains(t, output, "modified a.txt")
				require.Contains(t, output, "some repositories omitted")
			}},
		} {
			tt := tt
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				var output string
				require.NotPanics(t, func() {
					output = plainText(renderDiffDrawer(styles, tt.diff, renderChatDiffSummary(tt.diff), "", 90, 20))
				})
				tt.assert(t, output)
			})
		}
	})
	t.Run("ParseChatGitChangesFromUnifiedDiff", func(t *testing.T) {
		t.Parallel()

		diff := strings.Join([]string{
			"diff --git a/a.txt b/a.txt",
			"--- a/a.txt",
			"+++ b/a.txt",
			"@@ -1 +1 @@",
			"-old",
			"+new",
			"diff --git a/new.txt b/new.txt",
			"new file mode 100644",
			"--- /dev/null",
			"+++ b/new.txt",
			"@@ -0,0 +1 @@",
			"+hello",
			"diff --git a/old.txt b/old.txt",
			"deleted file mode 100644",
			"--- a/old.txt",
			"+++ /dev/null",
			"@@ -1 +0,0 @@",
			"-bye",
			"diff --git a/old-name.txt b/new-name.txt",
			"similarity index 100%",
			"rename from old-name.txt",
			"rename to new-name.txt",
		}, "\n")

		changes := parseChatGitChangesFromUnifiedDiff(codersdk.ChatDiffContents{Diff: diff})
		require.Len(t, changes, 4)
		require.Equal(t, "a.txt", changes[0].FilePath)
		require.Equal(t, "modified", changes[0].ChangeType)
		require.NotNil(t, changes[0].DiffSummary)
		require.Equal(t, "+1 -1", *changes[0].DiffSummary)
		require.Equal(t, "new.txt", changes[1].FilePath)
		require.Equal(t, "added", changes[1].ChangeType)
		require.NotNil(t, changes[1].DiffSummary)
		require.Equal(t, "+1", *changes[1].DiffSummary)
		require.Equal(t, "old.txt", changes[2].FilePath)
		require.Equal(t, "deleted", changes[2].ChangeType)
		require.NotNil(t, changes[2].DiffSummary)
		require.Equal(t, "-1", *changes[2].DiffSummary)
		require.Equal(t, "new-name.txt", changes[3].FilePath)
		require.Equal(t, "renamed", changes[3].ChangeType)
		require.NotNil(t, changes[3].OldPath)
		require.Equal(t, "old-name.txt", *changes[3].OldPath)
		require.Nil(t, changes[3].DiffSummary)
	})

	t.Run("ParseChatGitChangesFromUnifiedDiffPathsWithSpaces", func(t *testing.T) {
		t.Parallel()

		// Git does not quote paths that only contain spaces, so the
		// `diff --git` header is ambiguous without help from the body.
		// Verify that modifications, binary or mode-only diffs, and
		// renames all resolve to the correct paths and change types.
		diff := strings.Join([]string{
			"diff --git a/foo bar.txt b/foo bar.txt",
			"--- a/foo bar.txt",
			"+++ b/foo bar.txt",
			"@@ -1 +1 @@",
			"-old",
			"+new",
			"diff --git a/foo bar.bin b/foo bar.bin",
			"index 0f49c4a..9100462 100644",
			"Binary files a/foo bar.bin and b/foo bar.bin differ",
			"diff --git a/new empty.txt b/new empty.txt",
			"new file mode 100644",
			"index 0000000..e69de29",
			"diff --git a/old name.txt b/new name.txt",
			"similarity index 100%",
			"rename from old name.txt",
			"rename to new name.txt",
		}, "\n")

		changes := parseChatGitChangesFromUnifiedDiff(codersdk.ChatDiffContents{Diff: diff})
		require.Len(t, changes, 4)

		// The buggy parser used to split the unquoted header on any
		// whitespace, producing truncated paths and marking simple edits
		// as renames. Verify that each change now reports the full path
		// and the correct change type.
		require.Equal(t, "foo bar.txt", changes[0].FilePath)
		require.Equal(t, "modified", changes[0].ChangeType)

		require.Equal(t, "foo bar.bin", changes[1].FilePath)
		require.Equal(t, "modified", changes[1].ChangeType)

		require.Equal(t, "new empty.txt", changes[2].FilePath)
		require.Equal(t, "added", changes[2].ChangeType)

		require.Equal(t, "new name.txt", changes[3].FilePath)
		require.Equal(t, "renamed", changes[3].ChangeType)
		require.NotNil(t, changes[3].OldPath)
		require.Equal(t, "old name.txt", *changes[3].OldPath)
	})

	t.Run("ParseChatGitChangesFromUnifiedDiffQuotedPaths", func(t *testing.T) {
		t.Parallel()

		// Git C-quotes paths when they contain bytes above 0x7f (with
		// the default core.quotepath setting) or control characters.
		diff := strings.Join([]string{
			`diff --git "a/f\303\266\303\266bar.txt" "b/f\303\266\303\266bar.txt"`,
			`--- "a/f\303\266\303\266bar.txt"`,
			`+++ "b/f\303\266\303\266bar.txt"`,
			"@@ -1 +1 @@",
			"-old",
			"+new",
		}, "\n")

		changes := parseChatGitChangesFromUnifiedDiff(codersdk.ChatDiffContents{Diff: diff})
		require.Len(t, changes, 1)
		require.Equal(t, "fööbar.txt", changes[0].FilePath)
		require.Equal(t, "modified", changes[0].ChangeType)
	})

	t.Run("ParseChatGitChangesFromUnifiedDiffQuotedRename", func(t *testing.T) {
		t.Parallel()

		// Git C-quotes `rename from`/`rename to` paths when they contain
		// non-ASCII bytes (like `ä`). The parser should decode them so
		// the diff summary shows a readable file name rather than the
		// raw quoted octal escape.
		diff := strings.Join([]string{
			`diff --git "a/b\303\244r old.txt" "b/b\303\244r new.txt"`,
			"similarity index 100%",
			`rename from "b\303\244r old.txt"`,
			`rename to "b\303\244r new.txt"`,
		}, "\n")

		changes := parseChatGitChangesFromUnifiedDiff(codersdk.ChatDiffContents{Diff: diff})
		require.Len(t, changes, 1)
		require.Equal(t, "renamed", changes[0].ChangeType)
		require.Equal(t, "bär new.txt", changes[0].FilePath)
		require.NotNil(t, changes[0].OldPath)
		require.Equal(t, "bär old.txt", *changes[0].OldPath)
	})

	t.Run("ParseChatGitChangesFromUnifiedDiffRenameWithLiteralAPrefix", func(t *testing.T) {
		t.Parallel()

		// rename from/rename to paths are repository-relative and never
		// carry the a/ or b/ prefix, so real directories named a/ must
		// survive parsing intact.
		diff := strings.Join([]string{
			"diff --git a/a/foo.txt b/a/bar.txt",
			"similarity index 100%",
			"rename from a/foo.txt",
			"rename to a/bar.txt",
		}, "\n")

		changes := parseChatGitChangesFromUnifiedDiff(codersdk.ChatDiffContents{Diff: diff})
		require.Len(t, changes, 1)
		require.Equal(t, "renamed", changes[0].ChangeType)
		require.Equal(t, "a/bar.txt", changes[0].FilePath)
		require.NotNil(t, changes[0].OldPath)
		require.Equal(t, "a/foo.txt", *changes[0].OldPath)
	})

	t.Run("ParseChatGitChangesFromUnifiedDiffIgnoresHunkContentLookalikes", func(t *testing.T) {
		t.Parallel()

		// Added/removed diff lines can legitimately start with `+++ ` or
		// `--- ` (the content happens to begin with `++ ` or `-- `). The
		// parser must treat those as content after the first `@@` hunk
		// header instead of overwriting the already-resolved FilePath
		// and change counts.
		diff := strings.Join([]string{
			"diff --git a/a.txt b/a.txt",
			"--- a/a.txt",
			"+++ b/a.txt",
			"@@ -1,2 +1,2 @@",
			"--- not a header",
			"+++ also not a header",
			"-left",
			"+right",
		}, "\n")

		changes := parseChatGitChangesFromUnifiedDiff(codersdk.ChatDiffContents{Diff: diff})
		require.Len(t, changes, 1)
		require.Equal(t, "a.txt", changes[0].FilePath)
		require.Equal(t, "modified", changes[0].ChangeType)
		require.NotNil(t, changes[0].DiffSummary)
		// Inside the hunk, both "--- not a header" and "-left" are
		// deletion lines, and both "+++ also not a header" and "+right"
		// are addition lines. The header "--- a/a.txt" and "+++ b/a.txt"
		// lines before @@ are not counted.
		require.Equal(t, "+2 -2", *changes[0].DiffSummary)
	})

	t.Run("ParseUnifiedDiffHeaderPaths", func(t *testing.T) {
		t.Parallel()

		for _, tc := range []struct {
			name    string
			line    string
			oldPath string
			newPath string
			ok      bool
		}{
			{
				name:    "Simple",
				line:    "diff --git a/foo.txt b/foo.txt",
				oldPath: "foo.txt", newPath: "foo.txt", ok: true,
			},
			{
				name:    "Rename",
				line:    "diff --git a/old.txt b/new.txt",
				oldPath: "old.txt", newPath: "new.txt", ok: true,
			},
			{
				name:    "SpacesNonRename",
				line:    "diff --git a/foo bar.txt b/foo bar.txt",
				oldPath: "foo bar.txt", newPath: "foo bar.txt", ok: true,
			},
			{
				name: "SpacesRenameIsAmbiguous",
				line: "diff --git a/old name.txt b/new name.txt",
				ok:   false,
			},
			{
				name:    "QuotedTabEscape",
				line:    `diff --git "a/a\tb.txt" "b/a\tb.txt"`,
				oldPath: "a\tb.txt", newPath: "a\tb.txt", ok: true,
			},
			{
				name:    "NestedBPrefix",
				line:    "diff --git a/b/foo.txt b/b/foo.txt",
				oldPath: "b/foo.txt", newPath: "b/foo.txt", ok: true,
			},
			{
				name: "Empty",
				line: "diff --git ",
				ok:   false,
			},
			{
				name: "MissingAPrefix",
				line: "diff --git foo.txt bar.txt",
				ok:   false,
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				gotOld, gotNew, gotOK := parseUnifiedDiffHeaderPaths(tc.line)
				require.Equal(t, tc.ok, gotOK)
				if gotOK {
					require.Equal(t, tc.oldPath, gotOld)
					require.Equal(t, tc.newPath, gotNew)
				}
			})
		}
	})

	t.Run("RenderDiffDrawerSanitizesUntrustedContent", func(t *testing.T) {
		t.Parallel()

		diff := codersdk.ChatDiffContents{Diff: "diff --git a/a.txt b/a.txt\n+safe\x1b]52;c;clipboard\x07line"}
		rawOutput := renderDiffDrawer(styles, diff, renderChatDiffSummary(diff), "", 90, 20)
		output := plainText(rawOutput)

		require.Contains(t, output, "diff --git a/a.txt b/a.txt")
		require.Contains(t, output, "+safeline")
		require.Contains(t, output, "modified a.txt")
		require.NotContains(t, rawOutput, "clipboard")
		require.NotContains(t, rawOutput, "\x1b]52")
	})
	t.Run("RenderModelPicker", func(t *testing.T) {
		t.Parallel()

		catalog := codersdk.ChatModelsResponse{Providers: []codersdk.ChatModelProvider{{
			Provider:  "OpenAI",
			Available: true,
			Models:    []codersdk.ChatModel{{ID: "gpt-4o", Provider: "OpenAI", Model: "gpt-4o", DisplayName: "GPT-4o"}, {ID: "gpt-4.1", Provider: "OpenAI", Model: "gpt-4.1", DisplayName: "GPT-4.1"}},
		}, {
			Provider:          "Anthropic",
			Available:         false,
			UnavailableReason: codersdk.ChatModelProviderUnavailableMissingAPIKey,
		}, {
			Provider:  "Local",
			Available: true,
			Models:    nil,
		}}}
		for _, tt := range []struct {
			name          string
			selectedModel string
			selectedIndex int
			assert        func(t *testing.T, output string)
		}{
			{name: "GroupsModelsByProvider", selectedModel: "gpt-4o", assert: func(t *testing.T, output string) {
				require.Contains(t, output, "OpenAI")
				require.Contains(t, output, "GPT-4o")
				require.Contains(t, output, "GPT-4.1")
			}},
			{name: "ShowsCursorIndicatorOnSelectedPosition", selectedModel: "gpt-4.1", selectedIndex: 1, assert: func(t *testing.T, output string) {
				require.Contains(t, output, "> GPT-4.1")
				require.Contains(t, output, "  GPT-4o")
			}},
			{name: "HidesProvidersWithoutModels", selectedModel: "gpt-4o", assert: func(t *testing.T, output string) {
				require.Contains(t, output, "OpenAI")
				require.NotContains(t, output, "Anthropic")
				require.NotContains(t, output, "Local")
			}},
		} {
			tt := tt
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				var output string
				require.NotPanics(t, func() {
					output = plainText(renderModelPicker(styles, catalog, tt.selectedModel, tt.selectedIndex, 90, 20))
				})
				tt.assert(t, output)
			})
		}

		t.Run("ShowsGlobalEmptyStateWhenNoModelsSelectable", func(t *testing.T) {
			t.Parallel()

			emptyCatalog := codersdk.ChatModelsResponse{Providers: []codersdk.ChatModelProvider{{
				Provider:          "Anthropic",
				Available:         false,
				UnavailableReason: codersdk.ChatModelProviderUnavailableMissingAPIKey,
			}, {
				Provider:  "Local",
				Available: true,
				Models:    nil,
			}}}

			output := plainText(renderModelPicker(styles, emptyCatalog, "", 0, 90, 20))
			require.NotContains(t, output, "Anthropic")
			require.NotContains(t, output, "Local")
			require.Equal(t, 1, strings.Count(output, "No models available."))
		})
	})
	t.Run("KeepsCursorVisibleWithinWindow", func(t *testing.T) {
		t.Parallel()

		models := make([]codersdk.ChatModel, 0, 6)
		for i := 1; i <= 6; i++ {
			models = append(models, codersdk.ChatModel{
				ID:          fmt.Sprintf("provider:model-%d", i),
				Provider:    "provider",
				Model:       fmt.Sprintf("model-%d", i),
				DisplayName: fmt.Sprintf("Model %d", i),
			})
		}
		catalog := codersdk.ChatModelsResponse{Providers: []codersdk.ChatModelProvider{{
			Provider:  "provider",
			Available: true,
			Models:    models,
		}}}

		output := plainText(renderModelPicker(styles, catalog, "provider:model-5", 4, 60, 8))
		require.Contains(t, output, "> Model 5")
		require.NotContains(t, output, "Model 1")
	})

	t.Run("RenderAssistantMarkdown", func(t *testing.T) {
		t.Parallel()

		output := plainText(renderAssistantMarkdown(styles, "- first\n- second", 60, nil))
		require.Contains(t, output, "• first")
		require.Contains(t, output, "• second")
		require.NotContains(t, output, "- first")
	})

	t.Run("SanitizeTerminalRenderableText", func(t *testing.T) {
		t.Parallel()

		output := sanitizeTerminalRenderableText("safe\ttext\n\x1b[31mred\u009b32mgreen\x1b]52;c;clipboard\x07\x1b(Bdone\r\x00")
		require.Equal(t, "safe\ttext\nredgreendone", output)
		require.NotContains(t, output, "\x1b")
		require.NotContains(t, output, "\x07")
		require.NotContains(t, output, "\r")
		require.NotContains(t, output, "\x00")
	})

	t.Run("RenderToolDetailStripsTerminalEscapes", func(t *testing.T) {
		t.Parallel()

		rawOutput := renderToolDetail(styles, "result", "ok\x1b]52;c;clipboard\x07\n\tstill here", 60)
		output := plainText(rawOutput)
		require.Contains(t, output, "result: ok")
		require.Contains(t, output, "still here")
		require.NotContains(t, output, "clipboard")
		require.NotContains(t, output, "\x1b")
		require.NotContains(t, output, "\x07")
	})
	t.Run("UtilityRenderers", func(t *testing.T) {
		t.Parallel()

		for _, tt := range []struct{ name, input, want string }{
			{name: "WrapPreservingNewlines/PreservesExplicitNewlines", input: "line one\nline two", want: "line one\nline two"},
			{name: "WrapPreservingNewlines/EmptyString", input: "", want: ""},
			{name: "WrapPreservingNewlines/OnlyNewlines", input: "\n\n\n", want: "\n\n\n"},
		} {
			require.Equalf(t, tt.want, wrapPreservingNewlines(tt.input, 40), tt.name)
		}
		for _, tt := range []struct {
			name   string
			input  string
			max    int
			assert func(t *testing.T, output string)
		}{
			{name: "ClampLines/AddsEllipsis", input: "line1\nline2\nline3\nline4", max: 3, assert: func(t *testing.T, output string) {
				lines := strings.Split(output, "\n")
				require.Len(t, lines, 3)
				require.Equal(t, "line3…", lines[2])
			}},
			{name: "ClampLines/ZeroMax", input: "line1\nline2", max: 0, assert: func(t *testing.T, output string) { require.Empty(t, output) }},
		} {
			tt.assert(t, clampLines(tt.input, tt.max))
		}
		for _, tt := range []struct {
			name   string
			prefix string
			input  string
			width  int
			assert func(t *testing.T, output string)
		}{
			{name: "RenderPrefixedBlock/IndentsContinuationLines", prefix: "You: ", input: "alpha beta gamma delta", width: 12, assert: func(t *testing.T, output string) {
				lines := strings.Split(output, "\n")
				require.GreaterOrEqual(t, len(lines), 2)
				require.True(t, strings.HasPrefix(lines[1], strings.Repeat(" ", lipgloss.Width("You: "))))
				require.Contains(t, output, "You: ")
			}},
			{name: "RenderPrefixedBlock/EmptyContent", prefix: "You: ", width: 12, assert: func(t *testing.T, output string) { require.Equal(t, "You: ", output) }},
		} {
			tt.assert(t, renderPrefixedBlock(tt.prefix, tt.input, tt.width))
		}
	})

	t.Run("RenderAskUserQuestion", func(t *testing.T) {
		t.Parallel()

		firstQuestion := parsedAskQuestion{
			Header:   "Review plan",
			Question: "Which plan should we use?",
			Options: []parsedAskOption{
				{Label: "Fast path", Value: "fast"},
				{Label: "Safe path", Value: "safe"},
			},
		}
		secondQuestion := parsedAskQuestion{
			Header:   "Risk",
			Question: "How much risk is acceptable?",
			Options:  []parsedAskOption{{Label: "Low", Value: "low"}},
		}
		renderPlain := func(state *askUserQuestionState, width, height int) string {
			return plainText(renderAskUserQuestion(styles, state, width, height))
		}

		t.Run("BasicRenderShowsQuestionOptionsAndHelp", func(t *testing.T) {
			t.Parallel()

			state := newAskUserQuestionState("tool-1", []parsedAskQuestion{firstQuestion})
			output := renderPlain(state, 100, 20)

			require.Contains(t, output, "Plan Question 1/1")
			require.Contains(t, output, firstQuestion.Header)
			require.Contains(t, output, firstQuestion.Question)
			require.Contains(t, output, "Fast path")
			require.Contains(t, output, "Safe path")
			require.Contains(t, output, "Other (type custom answer)")
			require.Contains(t, output, "↑/↓ navigate")
			require.Contains(t, output, "enter select")
		})

		t.Run("SelectedOptionShowsCursor", func(t *testing.T) {
			t.Parallel()

			state := newAskUserQuestionState("tool-1", []parsedAskQuestion{firstQuestion})
			state.OptionCursor = 1
			output := renderPlain(state, 100, 20)

			require.Contains(t, output, "> Safe path")
			require.NotContains(t, output, "> Fast path")
		})

		t.Run("MultipleQuestionsShowProgress", func(t *testing.T) {
			t.Parallel()

			state := newAskUserQuestionState("tool-1", []parsedAskQuestion{firstQuestion, secondQuestion, firstQuestion})
			state.CurrentIndex = 1
			output := renderPlain(state, 100, 20)

			require.Contains(t, output, "Plan Question 2/3")
			require.Contains(t, output, secondQuestion.Header)
			require.Contains(t, output, secondQuestion.Question)
		})

		t.Run("FreeformInputIsVisible", func(t *testing.T) {
			t.Parallel()

			state := newAskUserQuestionState("tool-1", []parsedAskQuestion{firstQuestion})
			state.OptionCursor = len(firstQuestion.Options)
			state.OtherMode = true
			state.OtherInput.Focus()
			state.OtherInput.SetValue("Need a custom plan")
			output := renderPlain(state, 100, 20)

			require.Contains(t, output, "Need a custom plan")
			require.Contains(t, output, "esc cancel input")
		})

		t.Run("NarrowTerminalDoesNotPanic", func(t *testing.T) {
			t.Parallel()

			state := newAskUserQuestionState("tool-1", []parsedAskQuestion{firstQuestion})
			var output string
			require.NotPanics(t, func() {
				output = renderPlain(state, 18, 6)
			})
			require.NotEmpty(t, strings.TrimSpace(output))
		})
	})
}

func plainText(text string) string {
	return ansiRegexp.ReplaceAllString(text, "")
}

func rawJSON(value string) json.RawMessage {
	return json.RawMessage([]byte(value))
}

func int64Ptr(value int64) *int64 {
	return &value
}

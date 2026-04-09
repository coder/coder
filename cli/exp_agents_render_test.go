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

	t.Run("MessagesToBlocks", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name     string
			messages []codersdk.ChatMessage
			assert   func(t *testing.T, blocks []chatBlock)
		}{
			{
				name:     "EmptyMessages",
				messages: nil,
				assert: func(t *testing.T, blocks []chatBlock) {
					t.Helper()
					require.Empty(t, blocks)
				},
			},
			{
				name: "UserText",
				messages: []codersdk.ChatMessage{{
					Role: codersdk.ChatMessageRoleUser,
					Content: []codersdk.ChatMessagePart{{
						Type: codersdk.ChatMessagePartTypeText,
						Text: "hello",
					}},
				}},
				assert: func(t *testing.T, blocks []chatBlock) {
					t.Helper()
					require.Equal(t, []chatBlock{{kind: blockText, role: codersdk.ChatMessageRoleUser, text: "hello"}}, blocks)
				},
			},
			{
				name: "AssistantText",
				messages: []codersdk.ChatMessage{{
					Role: codersdk.ChatMessageRoleAssistant,
					Content: []codersdk.ChatMessagePart{{
						Type: codersdk.ChatMessagePartTypeText,
						Text: "hi there",
					}},
				}},
				assert: func(t *testing.T, blocks []chatBlock) {
					t.Helper()
					require.Equal(t, []chatBlock{{kind: blockText, role: codersdk.ChatMessageRoleAssistant, text: "hi there"}}, blocks)
				},
			},
			{
				name: "ToolCallPart",
				messages: []codersdk.ChatMessage{{
					Role: codersdk.ChatMessageRoleAssistant,
					Content: []codersdk.ChatMessagePart{{
						Type:       codersdk.ChatMessagePartTypeToolCall,
						ToolName:   "weather",
						ToolCallID: "call-1",
						Args: rawJSON(`{
  "city": "SF"
}`),
					}},
				}},
				assert: func(t *testing.T, blocks []chatBlock) {
					t.Helper()
					require.Equal(t, []chatBlock{{
						kind:     blockToolCall,
						role:     codersdk.ChatMessageRoleAssistant,
						toolName: "weather",
						toolID:   "call-1",
						args:     `{"city":"SF"}`,
					}}, blocks)
				},
			},
			{
				name: "ToolResultPart",
				messages: []codersdk.ChatMessage{{
					Role: codersdk.ChatMessageRoleTool,
					Content: []codersdk.ChatMessagePart{{
						Type:       codersdk.ChatMessagePartTypeToolResult,
						ToolName:   "weather",
						ToolCallID: "call-1",
						Result: rawJSON(`{
  "temp": "68F"
}`),
						IsError: true,
					}},
				}},
				assert: func(t *testing.T, blocks []chatBlock) {
					t.Helper()
					require.Equal(t, []chatBlock{{
						kind:     blockToolResult,
						role:     codersdk.ChatMessageRoleTool,
						toolName: "weather",
						toolID:   "call-1",
						result:   `{"temp":"68F"}`,
						isError:  true,
					}}, blocks)
				},
			},
			{
				name: "KeepsToolCallsAndLaterResultsSeparateByToolID",
				messages: []codersdk.ChatMessage{
					{
						Role: codersdk.ChatMessageRoleAssistant,
						Content: []codersdk.ChatMessagePart{
							{Type: codersdk.ChatMessagePartTypeToolCall, ToolName: "github__get_pull_request", ToolCallID: "call-1", Args: rawJSON(`{"owner":"openclaw","repo":"openclaw","pull_number":58036}`)},
							{Type: codersdk.ChatMessagePartTypeToolCall, ToolName: "github__get_pull_request", ToolCallID: "call-2", Args: rawJSON(`{"owner":"openclaw","repo":"openclaw","pull_number":58037}`)},
							{Type: codersdk.ChatMessagePartTypeToolCall, ToolName: "github__get_pull_request", ToolCallID: "call-3", Args: rawJSON(`{"owner":"openclaw","repo":"openclaw","pull_number":58038}`)},
						},
					},
					{
						Role: codersdk.ChatMessageRoleTool,
						Content: []codersdk.ChatMessagePart{
							{Type: codersdk.ChatMessagePartTypeToolResult, ToolName: "github__get_pull_request", ToolCallID: "call-1", Result: rawJSON(`{"base":{"ref":"main"}}`)},
							{Type: codersdk.ChatMessagePartTypeToolResult, ToolName: "github__get_pull_request", ToolCallID: "call-2", Result: rawJSON(`{"base":{"ref":"main"}}`)},
							{Type: codersdk.ChatMessagePartTypeToolResult, ToolName: "github__get_pull_request", ToolCallID: "call-3", Result: rawJSON(`{"base":{"ref":"main"}}`)},
						},
					},
				},
				assert: func(t *testing.T, blocks []chatBlock) {
					t.Helper()
					require.Len(t, blocks, 6)
					require.Equal(t, blockToolCall, blocks[0].kind)
					require.Equal(t, `{"owner":"openclaw","repo":"openclaw","pull_number":58036}`, blocks[0].args)
					require.Equal(t, blockToolCall, blocks[1].kind)
					require.Equal(t, `{"owner":"openclaw","repo":"openclaw","pull_number":58037}`, blocks[1].args)
					require.Equal(t, blockToolCall, blocks[2].kind)
					require.Equal(t, `{"owner":"openclaw","repo":"openclaw","pull_number":58038}`, blocks[2].args)
					require.Equal(t, blockToolResult, blocks[3].kind)
					require.Equal(t, `{"base":{"ref":"main"}}`, blocks[3].result)
					require.Equal(t, blockToolResult, blocks[4].kind)
					require.Equal(t, blockToolResult, blocks[5].kind)
				},
			},
			{
				name: "MultipleMessagesInOrder",
				messages: []codersdk.ChatMessage{
					{
						Role: codersdk.ChatMessageRoleUser,
						Content: []codersdk.ChatMessagePart{{
							Type: codersdk.ChatMessagePartTypeText,
							Text: "question",
						}},
					},
					{
						Role: codersdk.ChatMessageRoleAssistant,
						Content: []codersdk.ChatMessagePart{
							{Type: codersdk.ChatMessagePartTypeReasoning, Text: "thinking"},
							{Type: codersdk.ChatMessagePartTypeToolCall, ToolName: "search", ToolCallID: "call-3", Args: rawJSON(`{"q":"docs"}`)},
							{Type: codersdk.ChatMessagePartTypeText, Text: "answer"},
						},
					},
				},
				assert: func(t *testing.T, blocks []chatBlock) {
					t.Helper()
					require.Equal(t, []chatBlock{
						{kind: blockText, role: codersdk.ChatMessageRoleUser, text: "question"},
						{kind: blockReasoning, role: codersdk.ChatMessageRoleAssistant, text: "thinking"},
						{kind: blockToolCall, role: codersdk.ChatMessageRoleAssistant, toolName: "search", toolID: "call-3", args: `{"q":"docs"}`},
						{kind: blockText, role: codersdk.ChatMessageRoleAssistant, text: "answer"},
					}, blocks)
				},
			},
		}

		for _, tt := range tests {
			tt := tt
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				blocks := messagesToBlocks(tt.messages)
				tt.assert(t, blocks)
			})
		}
	})

	t.Run("MergeConsecutiveToolBlocks", func(t *testing.T) {
		t.Parallel()

		t.Run("MergesAdjacentEmptyToolIDCallAndResult", func(t *testing.T) {
			t.Parallel()

			blocks := []chatBlock{
				{
					kind:     blockToolCall,
					role:     codersdk.ChatMessageRoleAssistant,
					toolName: "read_file",
					args:     `{"path":"main.go"}`,
				},
				{
					kind:     blockToolResult,
					role:     codersdk.ChatMessageRoleTool,
					toolName: "read_file",
					result:   `{"content":"hello"}`,
				},
			}

			merged := mergeConsecutiveToolBlocks(blocks)
			require.Len(t, merged, 1)
			require.Equal(t, chatBlock{
				kind:     blockToolResult,
				role:     codersdk.ChatMessageRoleTool,
				toolName: "read_file",
				args:     `{"path":"main.go"}`,
				result:   `{"content":"hello"}`,
			}, merged[0])
		})

		t.Run("DoesNotMergeDifferentEmptyToolNames", func(t *testing.T) {
			t.Parallel()

			blocks := []chatBlock{
				{kind: blockToolCall, role: codersdk.ChatMessageRoleAssistant, toolName: "read_file", args: `{"path":"main.go"}`},
				{kind: blockToolResult, role: codersdk.ChatMessageRoleTool, toolName: "list_dir", result: `{"entries":[]}`},
			}

			merged := mergeConsecutiveToolBlocks(blocks)
			require.Equal(t, blocks, merged)
		})

		t.Run("DoesNotMergeNonAdjacentEmptyToolID", func(t *testing.T) {
			t.Parallel()

			blocks := []chatBlock{
				{kind: blockToolCall, role: codersdk.ChatMessageRoleAssistant, toolName: "read_file", args: `{"path":"main.go"}`},
				{kind: blockText, role: codersdk.ChatMessageRoleAssistant, text: "still thinking"},
				{kind: blockToolResult, role: codersdk.ChatMessageRoleTool, toolName: "read_file", result: `{"content":"hello"}`},
			}

			merged := mergeConsecutiveToolBlocks(blocks)
			require.Equal(t, blocks, merged)
		})

		t.Run("DoesNotMergeNonAdjacentMatchingToolID", func(t *testing.T) {
			t.Parallel()

			blocks := []chatBlock{
				{kind: blockToolCall, role: codersdk.ChatMessageRoleAssistant, toolName: "read_file", toolID: "call-1", args: `{"path":"main.go"}`},
				{kind: blockText, role: codersdk.ChatMessageRoleAssistant, text: "still thinking"},
				{kind: blockToolResult, role: codersdk.ChatMessageRoleTool, toolName: "read_file", toolID: "call-1", result: `{"content":"hello"}`},
			}

			merged := mergeConsecutiveToolBlocks(blocks)
			require.Equal(t, blocks, merged)
		})

		t.Run("ExistingToolIDMergeStillWorks", func(t *testing.T) {
			t.Parallel()

			blocks := []chatBlock{
				{kind: blockToolCall, role: codersdk.ChatMessageRoleAssistant, toolName: "read_file", toolID: "call-1", args: `{"path":"main.go"}`},
				{kind: blockToolResult, role: codersdk.ChatMessageRoleTool, toolName: "read_file", toolID: "call-1", result: `{"content":"hello"}`},
			}

			merged := mergeConsecutiveToolBlocks(blocks)
			require.Len(t, merged, 1)
			require.Equal(t, chatBlock{
				kind:     blockToolResult,
				role:     codersdk.ChatMessageRoleTool,
				toolName: "read_file",
				toolID:   "call-1",
				args:     `{"path":"main.go"}`,
				result:   `{"content":"hello"}`,
			}, merged[0])
		})

		t.Run("MultiplePairs", func(t *testing.T) {
			t.Parallel()

			blocks := []chatBlock{
				{kind: blockToolCall, role: codersdk.ChatMessageRoleAssistant, toolName: "read_file", toolID: "call-1", args: `{"path":"one.txt"}`},
				{kind: blockToolResult, role: codersdk.ChatMessageRoleTool, toolName: "read_file", toolID: "call-1", result: `{"ok":true}`},
				{kind: blockToolCall, role: codersdk.ChatMessageRoleAssistant, toolName: "list_dir", toolID: "call-2", args: `{"path":"/tmp"}`},
				{kind: blockToolResult, role: codersdk.ChatMessageRoleTool, toolName: "list_dir", toolID: "call-2", result: `{"entries":[]}`},
			}

			merged := mergeConsecutiveToolBlocks(blocks)
			require.Len(t, merged, 2)
			require.Equal(t, `{"path":"one.txt"}`, merged[0].args)
			require.Equal(t, `{"ok":true}`, merged[0].result)
			require.Equal(t, "call-1", merged[0].toolID)
			require.Equal(t, `{"path":"/tmp"}`, merged[1].args)
			require.Equal(t, `{"entries":[]}`, merged[1].result)
			require.Equal(t, "call-2", merged[1].toolID)
		})

		t.Run("OrphanedCall", func(t *testing.T) {
			t.Parallel()

			blocks := []chatBlock{{kind: blockToolCall, role: codersdk.ChatMessageRoleAssistant, toolName: "read_file", toolID: "call-orphan", args: `{"path":"solo.txt"}`}}

			merged := mergeConsecutiveToolBlocks(blocks)
			require.Equal(t, blocks, merged)
		})

		t.Run("OrphanedResult", func(t *testing.T) {
			t.Parallel()

			blocks := []chatBlock{{kind: blockToolResult, role: codersdk.ChatMessageRoleTool, toolName: "read_file", toolID: "call-orphan", result: `{"content":"hello"}`}}

			merged := mergeConsecutiveToolBlocks(blocks)
			require.Equal(t, blocks, merged)
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
	})

	t.Run("ToolResultSummary", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, "(created-ws)", toolResultSummary("coder_create_workspace", "", `{"workspace_name":"created-ws"}`))
	})
	t.Run("RenderToolCall", func(t *testing.T) {
		t.Parallel()

		styles := newTUIStyles()
		for _, tt := range []struct {
			name   string
			part   codersdk.ChatMessagePart
			width  int
			assert func(t *testing.T, output string)
		}{
			{name: "ShowsHumanizedToolNameAndContext", part: codersdk.ChatMessagePart{ToolName: "github__get_pull_request", Args: rawJSON(`{"owner":"openclaw","repo":"openclaw","pull_number":58036}`)}, width: 60, assert: func(t *testing.T, output string) {
				require.Contains(t, output, "  ⏳ get pull request")
				require.Contains(t, output, "(openclaw/openclaw)")
			}},
			{name: "ShowsTruncatedCommandPreview", part: codersdk.ChatMessagePart{ToolName: "coder_execute_command", Args: rawJSON(`{"command":"ls -la /tmp/with/a/very/long/path"}`)}, width: 30, assert: func(t *testing.T, output string) {
				require.Contains(t, output, "⏳ execute command")
				require.Contains(t, output, `"ls -la`)
				require.Contains(t, output, "…")
			}},
			{name: "ContextCompactionRendersBanner", part: codersdk.ChatMessagePart{ToolName: contextCompactionToolName}, width: 40, assert: func(t *testing.T, output string) {
				require.Contains(t, output, "🗜️  Context compacted")
				require.NotContains(t, output, "⏳")
			}},
		} {
			tt := tt
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				var output string
				require.NotPanics(t, func() { output = plainText(renderToolCall(styles, tt.part, tt.width)) })
				tt.assert(t, output)
			})
		}
	})
	t.Run("RenderToolResult", func(t *testing.T) {
		t.Parallel()

		styles := newTUIStyles()
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
				require.NotPanics(t, func() { rawOutput = renderToolResult(styles, tt.part, tt.width) })
				tt.assert(t, rawOutput, plainText(rawOutput))
			})
		}
	})
	t.Run("RenderCompaction", func(t *testing.T) {
		t.Parallel()

		styles := newTUIStyles()
		output := plainText(renderCompaction(styles, 20))
		require.Contains(t, output, "🗜️  Context compacted")
	})
	t.Run("RenderStatusBar", func(t *testing.T) {
		t.Parallel()

		styles := newTUIStyles()
		usage := func(total, limit int64) *codersdk.ChatMessageUsage {
			return &codersdk.ChatMessageUsage{TotalTokens: int64Ptr(total), ContextLimit: int64Ptr(limit)}
		}
		tests := []struct {
			name                 string
			status               codersdk.ChatStatus
			usage                *codersdk.ChatMessageUsage
			queueCount           int
			interrupting         bool
			reconnecting         bool
			width                int
			expectedSubstrings   []string
			unexpectedSubstrings []string
			maxPlainWidth        int
		}{
			{name: "ShowsStatusWithColor", status: codersdk.ChatStatusRunning, width: 80, expectedSubstrings: []string{styles.statusColor(codersdk.ChatStatusRunning).Render(string(codersdk.ChatStatusRunning))}},
			{name: "ShowsTokenUsageWhenPresent", status: codersdk.ChatStatusRunning, usage: usage(50, 100), width: 80, expectedSubstrings: []string{"tokens: 50/100"}},
			{name: "WarnsWhenUsageExceedsEightyPercent", status: codersdk.ChatStatusRunning, usage: usage(81, 100), width: 80, expectedSubstrings: []string{styles.warningText.Render("tokens: 81/100")}},
			{name: "CriticalWhenUsageExceedsNinetyFivePercent", status: codersdk.ChatStatusRunning, usage: usage(96, 100), width: 80, expectedSubstrings: []string{styles.criticalText.Render("tokens: 96/100")}},
			{name: "ShowsQueueCount", status: codersdk.ChatStatusPending, queueCount: 2, width: 80, expectedSubstrings: []string{"queued: 2"}},
			{name: "ShowsInterrupting", status: codersdk.ChatStatusRunning, interrupting: true, width: 80, expectedSubstrings: []string{"interrupting…"}},
			{name: "ShowsReconnecting", status: codersdk.ChatStatusRunning, reconnecting: true, width: 80, expectedSubstrings: []string{"reconnecting…"}},
			{name: "OmitsUsageWhenNil", status: codersdk.ChatStatusRunning, width: 80, unexpectedSubstrings: []string{"tokens:"}},
			{name: "NarrowWidthFits", status: codersdk.ChatStatusRunning, usage: usage(96, 100), queueCount: 2, interrupting: true, reconnecting: true, width: 20, maxPlainWidth: 20},
		}

		for _, tt := range tests {
			tt := tt
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				var output string
				require.NotPanics(t, func() {
					output = renderStatusBar(styles, nil, tt.status, tt.usage, tt.queueCount, tt.interrupting, tt.reconnecting, tt.width)
				})
				plain := plainText(output)
				for _, expected := range tt.expectedSubstrings {
					require.Contains(t, output, expected)
				}
				for _, unexpected := range tt.unexpectedSubstrings {
					require.NotContains(t, plain, unexpected)
				}
				if tt.maxPlainWidth > 0 {
					require.NotEmpty(t, plain)
					require.LessOrEqual(t, lipgloss.Width(plain), tt.maxPlainWidth)
					require.LessOrEqual(t, lipgloss.Width(output), tt.width)
				}
			})
		}
	})
	t.Run("RenderBlock", func(t *testing.T) {
		t.Parallel()

		styles := newTUIStyles()

		t.Run("TextUserIncludesYouPrefix", func(t *testing.T) {
			t.Parallel()

			output := plainText(renderBlock(styles, chatBlock{kind: blockText, role: codersdk.ChatMessageRoleUser, text: "hello"}, false, 40))
			require.Contains(t, output, "You: hello")
		})

		t.Run("TextAssistantRendersMarkdown", func(t *testing.T) {
			t.Parallel()

			output := plainText(renderBlock(styles, chatBlock{kind: blockText, role: codersdk.ChatMessageRoleAssistant, text: "- first\n- second"}, false, 40))
			require.Contains(t, output, "• first")
			require.Contains(t, output, "• second")
			require.NotContains(t, output, "- first")
		})

		t.Run("TextToolRendersDimmed", func(t *testing.T) {
			t.Parallel()

			output := renderBlock(styles, chatBlock{kind: blockText, role: codersdk.ChatMessageRoleTool, text: "tool output"}, false, 40)
			require.Contains(t, output, styles.dimmedText.Render("tool output"))
		})

		type renderBlockPairCase struct {
			name            string
			block           chatBlock
			width           int
			collapsedAssert func(t *testing.T, output string)
			expandedAssert  func(t *testing.T, output string)
		}
		for _, tt := range []renderBlockPairCase{
			{
				name:  "Reasoning",
				block: chatBlock{kind: blockReasoning, role: codersdk.ChatMessageRoleAssistant, text: "line1\nline2\nline3\nline4"},
				width: 40,
				collapsedAssert: func(t *testing.T, output string) {
					lines := strings.Split(output, "\n")
					require.Len(t, lines, 3)
					require.Contains(t, lines[0], "💭 line1")
					require.Equal(t, "line3…", strings.TrimRight(lines[2], " "))
				},
				expandedAssert: func(t *testing.T, output string) {
					lines := strings.Split(output, "\n")
					require.Len(t, lines, 4)
					require.Contains(t, output, "line4")
					require.NotContains(t, output, "line4…")
				},
			},
			{
				name:  "ToolCall",
				block: chatBlock{kind: blockToolCall, toolName: "read_file", args: `{"path":"very/long/path.txt","recursive":true}`},
				width: 60,
				collapsedAssert: func(t *testing.T, output string) {
					require.Contains(t, output, "⏳ read file")
					require.Contains(t, output, "(very/long/path.txt)")
					require.NotContains(t, output, "\n")
				},
				expandedAssert: func(t *testing.T, output string) {
					require.Contains(t, output, "⏳ read file")
					require.Contains(t, output, "args:")
					require.Contains(t, output, `{"path":"very/long/path.txt","recursive":true}`)
					require.Contains(t, output, "\n")
				},
			},
			{
				name:  "ToolResult",
				block: chatBlock{kind: blockToolResult, toolName: "read_file", args: `{"path":"a.txt"}`, result: `{"path":"a.txt","contents":"hello"}`},
				width: 60,
				collapsedAssert: func(t *testing.T, output string) {
					require.Contains(t, output, "✓ read file")
					require.Contains(t, output, "(a.txt)")
					require.NotContains(t, output, "\n")
				},
				expandedAssert: func(t *testing.T, output string) {
					require.Contains(t, output, "✓ read file")
					require.Contains(t, output, "args:")
					require.Contains(t, output, "result:")
					require.Contains(t, output, `{"path":"a.txt","contents":"hello"}`)
					require.Contains(t, output, "\n")
				},
			},
		} {
			tt := tt
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				tt.collapsedAssert(t, plainText(renderBlock(styles, tt.block, false, tt.width)))
				tt.expandedAssert(t, plainText(renderBlock(styles, tt.block, true, tt.width)))
			})
		}

		t.Run("CollapsedToolCallShowsRunCount", func(t *testing.T) {
			t.Parallel()

			output := plainText(renderBlock(styles, chatBlock{kind: blockToolCall, toolName: "github__get_pull_request", args: `{"owner":"openclaw","repo":"openclaw"}`, collapsedCount: 3}, false, 80))
			require.Contains(t, output, "⏳ get pull request...")
		})

		t.Run("CollapsedToolResultShowsRunCount", func(t *testing.T) {
			t.Parallel()

			output := plainText(renderBlock(styles, chatBlock{kind: blockToolResult, toolName: "github__get_pull_request", args: `{"owner":"openclaw","repo":"openclaw"}`, result: `{"ok":true}`, collapsedCount: 10}, false, 80))
			require.Contains(t, output, "✓ get pull request (x10)")
		})
		t.Run("CompactionRendersBanner", func(t *testing.T) {
			t.Parallel()

			output := plainText(renderBlock(styles, chatBlock{kind: blockCompaction}, false, 40))
			require.Contains(t, output, "🗜️  Context compacted")
		})
	})

	t.Run("RenderChatBlocks", func(t *testing.T) {
		t.Parallel()

		styles := newTUIStyles()

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
			require.Contains(t, output, "💭 thinking")
			require.Contains(t, output, "✓ read file")
			require.Contains(t, output, "done")
			require.Less(t, strings.Index(output, "You: hello"), strings.Index(output, "💭 thinking"))
			require.Less(t, strings.Index(output, "💭 thinking"), strings.Index(output, "✓ read file"))
			require.Less(t, strings.Index(output, "✓ read file"), strings.LastIndex(output, "done"))
		})

		t.Run("CollapsesConsecutiveSameNameToolResults", func(t *testing.T) {
			t.Parallel()

			blocks := []chatBlock{
				{kind: blockToolResult, toolName: "github__get_pull_request", args: `{"owner":"openclaw","repo":"openclaw","pull_number":1}`, result: `{"base":{"ref":"main"}}`},
				{kind: blockToolResult, toolName: "github__get_pull_request", args: `{"owner":"openclaw","repo":"openclaw","pull_number":2}`, result: `{"base":{"ref":"main"}}`},
				{kind: blockToolResult, toolName: "github__get_pull_request", args: `{"owner":"openclaw","repo":"openclaw","pull_number":3}`, result: `{"base":{"ref":"main"}}`},
				{kind: blockToolResult, toolName: "create_file", args: `{"path":"main.go"}`, result: `{"ok":true}`},
			}

			output := plainText(renderChatBlocks(styles, blocks, -1, map[int]bool{}, true, 80))
			require.Equal(t, 2, strings.Count(output, "✓"))
			require.Contains(t, output, "get pull request (x3)")
			require.Contains(t, output, "create file")
		})

		t.Run("ExpandedToolBlockPreventsCollapse", func(t *testing.T) {
			t.Parallel()

			blocks := []chatBlock{
				{kind: blockToolResult, toolName: "github__get_pull_request", args: `{"owner":"openclaw","repo":"openclaw","pull_number":1}`, result: `{"base":{"ref":"main"}}`},
				{kind: blockToolResult, toolName: "github__get_pull_request", args: `{"owner":"openclaw","repo":"openclaw","pull_number":2}`, result: `{"base":{"ref":"main"}}`},
			}

			output := plainText(renderChatBlocks(styles, blocks, 1, map[int]bool{1: true}, false, 80))
			require.Equal(t, 2, strings.Count(output, "✓"))
			require.NotContains(t, output, "(x2)")
			require.Contains(t, output, "result:")
		})
	})
	t.Run("RenderDiffDrawer", func(t *testing.T) {
		t.Parallel()

		styles := newTUIStyles()
		branch := "feature/chat-ui"
		prURL := "https://example.com/pulls/123"
		for _, tt := range []struct {
			name    string
			diff    codersdk.ChatDiffContents
			changes []codersdk.ChatGitChange
			assert  func(t *testing.T, output string)
		}{
			{name: "ShowsMetadataWhenPresent", diff: codersdk.ChatDiffContents{Branch: &branch, PullRequestURL: &prURL}, assert: func(t *testing.T, output string) {
				require.Contains(t, output, "Branch: feature/chat-ui")
				require.Contains(t, output, "PR: https://example.com/pulls/123")
			}},
			{name: "ShowsDiffContent", diff: codersdk.ChatDiffContents{Diff: "diff --git a/a.txt b/a.txt\n+added line"}, changes: []codersdk.ChatGitChange{{FilePath: "a.txt", ChangeType: "modified"}}, assert: func(t *testing.T, output string) {
				require.Contains(t, output, "diff --git a/a.txt b/a.txt")
				require.Contains(t, output, "+added line")
			}},
			{name: "ShowsPlaceholderForEmptyDiff", assert: func(t *testing.T, output string) { require.Contains(t, output, "No diff contents.") }},
		} {
			tt := tt
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				var output string
				require.NotPanics(t, func() { output = plainText(renderDiffDrawer(styles, tt.diff, tt.changes, 90, 20)) })
				tt.assert(t, output)
			})
		}
	})
	t.Run("RenderDiffDrawerSanitizesUntrustedContent", func(t *testing.T) {
		t.Parallel()

		styles := newTUIStyles()
		rawOutput := renderDiffDrawer(
			styles,
			codersdk.ChatDiffContents{Diff: "diff --git a/a.txt b/a.txt\n+safe\x1b]52;c;clipboard\x07line"},
			[]codersdk.ChatGitChange{{
				FilePath:   "a.txt\x1b]52;c;clipboard\x07",
				ChangeType: "modified",
			}},
			90,
			20,
		)
		output := plainText(rawOutput)

		require.Contains(t, output, "diff --git a/a.txt b/a.txt")
		require.Contains(t, output, "+safeline")
		require.Contains(t, output, "modified a.txt")
		require.NotContains(t, rawOutput, "clipboard")
		require.NotContains(t, rawOutput, "\x1b]52")
	})
	t.Run("RenderModelPicker", func(t *testing.T) {
		t.Parallel()

		styles := newTUIStyles()
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
	})
	t.Run("KeepsCursorVisibleWithinWindow", func(t *testing.T) {
		t.Parallel()

		styles := newTUIStyles()
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

		styles := newTUIStyles()
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

		styles := newTUIStyles()
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

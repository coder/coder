package cli //nolint:testpackage // Tests unexported chat TUI render helpers.

import (
	"encoding/json"
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
				name: "SkipsSystemMessages",
				messages: []codersdk.ChatMessage{{
					Role: codersdk.ChatMessageRoleSystem,
					Content: []codersdk.ChatMessagePart{{
						Type: codersdk.ChatMessagePartTypeText,
						Text: "internal",
					}},
				}},
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
					require.Len(t, blocks, 1)
					require.Equal(t, chatBlock{kind: blockText, role: codersdk.ChatMessageRoleUser, text: "hello"}, blocks[0])
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
					require.Len(t, blocks, 1)
					require.Equal(t, chatBlock{kind: blockText, role: codersdk.ChatMessageRoleAssistant, text: "hi there"}, blocks[0])
				},
			},
			{
				name: "ReasoningPart",
				messages: []codersdk.ChatMessage{{
					Role: codersdk.ChatMessageRoleAssistant,
					Content: []codersdk.ChatMessagePart{{
						Type: codersdk.ChatMessagePartTypeReasoning,
						Text: "thinking",
					}},
				}},
				assert: func(t *testing.T, blocks []chatBlock) {
					t.Helper()
					require.Len(t, blocks, 1)
					require.Equal(t, chatBlock{kind: blockReasoning, role: codersdk.ChatMessageRoleAssistant, text: "thinking"}, blocks[0])
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
					require.Len(t, blocks, 1)
					require.Equal(t, chatBlock{
						kind:     blockToolCall,
						role:     codersdk.ChatMessageRoleAssistant,
						toolName: "weather",
						toolID:   "call-1",
						args:     `{"city":"SF"}`,
					}, blocks[0])
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
					require.Len(t, blocks, 1)
					require.Equal(t, chatBlock{
						kind:     blockToolResult,
						role:     codersdk.ChatMessageRoleTool,
						toolName: "weather",
						toolID:   "call-1",
						result:   `{"temp":"68F"}`,
						isError:  true,
					}, blocks[0])
				},
			},
			{
				name: "MergesToolCallsWithLaterResultsByToolID",
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
					require.Len(t, blocks, 3)
					require.Equal(t, chatBlock{
						kind:     blockToolResult,
						role:     codersdk.ChatMessageRoleTool,
						toolName: "github__get_pull_request",
						toolID:   "call-1",
						args:     `{"owner":"openclaw","repo":"openclaw","pull_number":58036}`,
						result:   `{"base":{"ref":"main"}}`,
					}, blocks[0])
					require.Equal(t, `{"owner":"openclaw","repo":"openclaw","pull_number":58037}`, blocks[1].args)
					require.Equal(t, `{"owner":"openclaw","repo":"openclaw","pull_number":58038}`, blocks[2].args)
				},
			},
			{
				name: "KeepsStandaloneToolResultWithoutMatchingCall",
				messages: []codersdk.ChatMessage{{
					Role: codersdk.ChatMessageRoleTool,
					Content: []codersdk.ChatMessagePart{{
						Type:       codersdk.ChatMessagePartTypeToolResult,
						ToolName:   "weather",
						ToolCallID: "call-missing",
						Result:     rawJSON(`{"temp":"68F"}`),
					}},
				}},
				assert: func(t *testing.T, blocks []chatBlock) {
					t.Helper()
					require.Len(t, blocks, 1)
					require.Equal(t, blockToolResult, blocks[0].kind)
					require.Equal(t, "call-missing", blocks[0].toolID)
					require.Equal(t, `{"temp":"68F"}`, blocks[0].result)
				},
			},
			{
				name: "ContextCompactionToolCall",
				messages: []codersdk.ChatMessage{{
					Role: codersdk.ChatMessageRoleAssistant,
					Content: []codersdk.ChatMessagePart{{
						Type:       codersdk.ChatMessagePartTypeToolCall,
						ToolName:   contextCompactionToolName,
						ToolCallID: "call-2",
						Args:       rawJSON(`{"summary":"done"}`),
					}},
				}},
				assert: func(t *testing.T, blocks []chatBlock) {
					t.Helper()
					require.Len(t, blocks, 1)
					require.Equal(t, blockCompaction, blocks[0].kind)
					require.NotEqual(t, blockToolCall, blocks[0].kind)
				},
			},
			{
				name: "ContextCompactionToolResult",
				messages: []codersdk.ChatMessage{{
					Role: codersdk.ChatMessageRoleTool,
					Content: []codersdk.ChatMessagePart{{
						Type:       codersdk.ChatMessagePartTypeToolResult,
						ToolName:   contextCompactionToolName,
						ToolCallID: "call-2",
						Result:     rawJSON(`{"summary":"done"}`),
					}},
				}},
				assert: func(t *testing.T, blocks []chatBlock) {
					t.Helper()
					require.Len(t, blocks, 1)
					require.Equal(t, blockCompaction, blocks[0].kind)
					require.NotEqual(t, blockToolResult, blocks[0].kind)
				},
			},
			{
				name: "SourcePart",
				messages: []codersdk.ChatMessage{{
					Role: codersdk.ChatMessageRoleAssistant,
					Content: []codersdk.ChatMessagePart{{
						Type:  codersdk.ChatMessagePartTypeSource,
						Title: "Docs",
						URL:   "https://coder.com/docs",
					}},
				}},
				assert: func(t *testing.T, blocks []chatBlock) {
					t.Helper()
					require.Len(t, blocks, 1)
					require.Equal(t, chatBlock{kind: blockText, role: codersdk.ChatMessageRoleAssistant, text: "[Source: Docs](https://coder.com/docs)"}, blocks[0])
				},
			},
			{
				name: "FilePart",
				messages: []codersdk.ChatMessage{{
					Role: codersdk.ChatMessageRoleAssistant,
					Content: []codersdk.ChatMessagePart{{
						Type:      codersdk.ChatMessagePartTypeFile,
						MediaType: "text/plain",
					}},
				}},
				assert: func(t *testing.T, blocks []chatBlock) {
					t.Helper()
					require.Len(t, blocks, 1)
					require.Equal(t, chatBlock{kind: blockText, role: codersdk.ChatMessageRoleAssistant, text: "[File: text/plain]"}, blocks[0])
				},
			},
			{
				name: "FileReferencePart",
				messages: []codersdk.ChatMessage{{
					Role: codersdk.ChatMessageRoleAssistant,
					Content: []codersdk.ChatMessagePart{{
						Type:      codersdk.ChatMessagePartTypeFileReference,
						FileName:  "main.go",
						StartLine: 1,
						EndLine:   10,
					}},
				}},
				assert: func(t *testing.T, blocks []chatBlock) {
					t.Helper()
					require.Len(t, blocks, 1)
					require.Equal(t, chatBlock{kind: blockText, role: codersdk.ChatMessageRoleAssistant, text: "[main.go L1-10]"}, blocks[0])
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
					require.Len(t, blocks, 4)
					require.Equal(t, blockText, blocks[0].kind)
					require.Equal(t, "question", blocks[0].text)
					require.Equal(t, blockReasoning, blocks[1].kind)
					require.Equal(t, "thinking", blocks[1].text)
					require.Equal(t, blockToolCall, blocks[2].kind)
					require.Equal(t, "search", blocks[2].toolName)
					require.Equal(t, blockText, blocks[3].kind)
					require.Equal(t, "answer", blocks[3].text)
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

		t.Run("CreateWorkspaceUsesNameField", func(t *testing.T) {
			t.Parallel()

			require.Equal(t, "(my-workspace)", toolArgsSummary("coder_create_workspace", `{"name":"my-workspace"}`))
		})

		t.Run("CreateWorkspaceUsesWorkspaceNameField", func(t *testing.T) {
			t.Parallel()

			require.Equal(t, "(my-ws)", toolArgsSummary("coder_create_workspace", `{"workspace_name":"my-ws","template":"docker"}`))
		})

		t.Run("WithUnicodeTruncatesOnRuneBoundary", func(t *testing.T) {
			t.Parallel()

			args := strings.Repeat("こんにちは世界", 10)
			summary := toolArgsSummary("weather", args)
			require.NotEmpty(t, summary)
			require.True(t, utf8.ValidString(summary))
			require.True(t, strings.HasSuffix(summary, "…"))
			require.LessOrEqual(t, len([]rune(summary)), toolSummaryFallbackWidth)
			require.Contains(t, summary, "こんにちは")
		})
	})

	t.Run("ToolResultSummary", func(t *testing.T) {
		t.Parallel()

		t.Run("CreateWorkspaceUsesWorkspaceNameFromResult", func(t *testing.T) {
			t.Parallel()

			require.Equal(t, "(created-ws)", toolResultSummary("coder_create_workspace", "", `{"workspace_name":"created-ws"}`))
		})
	})

	t.Run("RenderToolCall", func(t *testing.T) {
		t.Parallel()

		styles := newTUIStyles()

		t.Run("ShowsHumanizedToolNameAndContext", func(t *testing.T) {
			t.Parallel()

			output := plainText(renderToolCall(styles, codersdk.ChatMessagePart{
				ToolName: "github__get_pull_request",
				Args:     rawJSON(`{"owner":"openclaw","repo":"openclaw","pull_number":58036}`),
			}, 60))
			require.Contains(t, output, "  ⏳ get pull request")
			require.Contains(t, output, "(openclaw/openclaw)")
		})

		t.Run("ShowsTruncatedCommandPreview", func(t *testing.T) {
			t.Parallel()

			output := plainText(renderToolCall(styles, codersdk.ChatMessagePart{
				ToolName: "coder_execute_command",
				Args:     rawJSON(`{"command":"ls -la /tmp/with/a/very/long/path"}`),
			}, 30))
			require.Contains(t, output, "⏳ execute command")
			require.Contains(t, output, `"ls -la`)
			require.Contains(t, output, "…")
		})

		t.Run("VeryLongArgsTruncatePreview", func(t *testing.T) {
			t.Parallel()

			args := rawJSON(`{"payload":"` + strings.Repeat("a", 2000) + `"}`)
			output := plainText(renderToolCall(styles, codersdk.ChatMessagePart{
				ToolName: "weather",
				Args:     args,
			}, 40))
			require.Contains(t, output, "⏳ weather")
			require.Contains(t, output, "…")
			require.NotContains(t, output, strings.Repeat("a", 100))
		})

		t.Run("ContextCompactionRendersBanner", func(t *testing.T) {
			t.Parallel()

			output := plainText(renderToolCall(styles, codersdk.ChatMessagePart{ToolName: contextCompactionToolName}, 40))
			require.Contains(t, output, "🗜️  Context compacted")
			require.NotContains(t, output, "⏳")
		})

		t.Run("EmptyToolNameFallsBackToTool", func(t *testing.T) {
			t.Parallel()

			output := plainText(renderToolCall(styles, codersdk.ChatMessagePart{Args: rawJSON(`{"x":1}`)}, 40))
			require.Contains(t, output, "⏳ tool")
		})

		t.Run("ZeroWidthReturnsJustLabel", func(t *testing.T) {
			t.Parallel()

			output := plainText(renderToolCall(styles, codersdk.ChatMessagePart{ToolName: "weather", Args: rawJSON(`{"x":1}`)}, 0))
			require.Equal(t, "  ⏳ weather", output)
		})

		t.Run("ZeroWidthDoesNotPanic", func(t *testing.T) {
			t.Parallel()

			require.NotPanics(t, func() {
				_ = renderToolCall(styles, codersdk.ChatMessagePart{ToolName: "weather", Args: rawJSON(`{"x":1}`)}, 0)
			})
		})
	})

	t.Run("RenderToolResult", func(t *testing.T) {
		t.Parallel()

		styles := newTUIStyles()

		t.Run("MergedErrorKeepsArgsSummary", func(t *testing.T) {
			t.Parallel()

			output := plainText(renderToolResult(styles, codersdk.ChatMessagePart{
				ToolName: "read_file",
				Args:     rawJSON(`{"path":"secrets.txt"}`),
				Result:   rawJSON(`{"error":"permission denied"}`),
				IsError:  true,
			}, 60))
			require.Contains(t, output, "✗ read file")
			require.Contains(t, output, "(secrets.txt)")
			require.NotContains(t, output, "permission denied")
		})

		t.Run("SuccessShowsCheckPrefixAndArgsContext", func(t *testing.T) {
			t.Parallel()

			output := plainText(renderToolResult(styles, codersdk.ChatMessagePart{
				ToolName: "coder_execute_command",
				Args:     rawJSON(`{"command":"ls -la"}`),
				Result:   rawJSON(`{"ok":true}`),
			}, 40))
			require.Contains(t, output, "✓ execute command")
			require.Contains(t, output, `"ls -la"`)
		})

		t.Run("ErrorShowsErrorStyleAndMessage", func(t *testing.T) {
			t.Parallel()

			output := renderToolResult(styles, codersdk.ChatMessagePart{
				ToolName: "coder_execute_command",
				Result:   rawJSON(`{"error":"command not found"}`),
				IsError:  true,
			}, 40)
			require.Contains(t, output, styles.errorText.Render("✗ execute command"))
			require.Contains(t, plainText(output), `"command not found"`)
		})

		t.Run("ShowsCompactResultPreview", func(t *testing.T) {
			t.Parallel()

			output := plainText(renderToolResult(styles, codersdk.ChatMessagePart{ToolName: "weather", Result: rawJSON(`{"forecast":"sunny and warm all afternoon"}`)}, 26))
			require.Contains(t, output, "✓ weather")
			require.Contains(t, output, "sunny")
			require.Contains(t, output, "…")
			require.NotContains(t, output, "all afternoon")
		})

		t.Run("VeryLongResultTruncatesPreview", func(t *testing.T) {
			t.Parallel()

			result := rawJSON(`{"payload":"` + strings.Repeat("b", 5000) + `"}`)
			output := plainText(renderToolResult(styles, codersdk.ChatMessagePart{ToolName: "weather", Result: result}, 40))
			require.Contains(t, output, "✓ weather")
			require.Contains(t, output, "…")
			require.NotContains(t, output, strings.Repeat("b", 100))
		})

		t.Run("CreateWorkspaceSuccessShowsWorkspaceContext", func(t *testing.T) {
			t.Parallel()

			output := plainText(renderToolResult(styles, codersdk.ChatMessagePart{
				ToolName: "coder_create_workspace",
				Args:     rawJSON(`{"name":"my-workspace"}`),
				Result:   rawJSON(`{"workspace_name":"my-workspace","template":"docker"}`),
			}, 60))
			require.Contains(t, output, "✓ create workspace")
			require.Contains(t, output, "(my-workspace)")
		})

		t.Run("CreateWorkspaceErrorShowsWorkspaceContext", func(t *testing.T) {
			t.Parallel()

			output := plainText(renderToolResult(styles, codersdk.ChatMessagePart{
				ToolName: "coder_create_workspace",
				Args:     rawJSON(`{"name":"my-workspace"}`),
				Result:   rawJSON(`{"error":"template not found"}`),
				IsError:  true,
			}, 60))
			require.Contains(t, output, "✗ create workspace")
			require.Contains(t, output, "(my-workspace)")
		})

		t.Run("MergedCreateWorkspaceResultKeepsArgsSummary", func(t *testing.T) {
			t.Parallel()

			output := plainText(renderToolResult(styles, codersdk.ChatMessagePart{
				ToolName:   "coder_create_workspace",
				ToolCallID: "call-create-workspace",
				Args:       rawJSON(`{"name":"merged-workspace"}`),
				Result:     rawJSON(`{"workspace_name":"merged-workspace","status":"created"}`),
			}, 60))
			require.Contains(t, output, "✓ create workspace")
			require.Contains(t, output, "(merged-workspace)")
		})

		t.Run("ContextCompactionRendersBanner", func(t *testing.T) {
			t.Parallel()

			output := plainText(renderToolResult(styles, codersdk.ChatMessagePart{ToolName: contextCompactionToolName}, 40))
			require.Contains(t, output, "🗜️  Context compacted")
			require.NotContains(t, output, "✓")
		})

		t.Run("EmptyResultRendersNull", func(t *testing.T) {
			t.Parallel()

			output := plainText(renderToolResult(styles, codersdk.ChatMessagePart{ToolName: "weather"}, 40))
			require.Contains(t, output, "null")
		})
	})

	t.Run("RenderCompaction", func(t *testing.T) {
		t.Parallel()

		styles := newTUIStyles()

		t.Run("ContainsIconAndText", func(t *testing.T) {
			t.Parallel()

			output := plainText(renderCompaction(styles, 20))
			require.Contains(t, output, "🗜️  Context compacted")
		})

		t.Run("CentersWithinWidth", func(t *testing.T) {
			t.Parallel()

			output := renderCompaction(styles, 40)
			plain := plainText(output)
			require.Equal(t, 40, lipgloss.Width(output))
			require.True(t, strings.HasPrefix(plain, " "))
			require.Contains(t, plain, "Context compacted")
		})
	})

	t.Run("RenderStatusBar", func(t *testing.T) {
		t.Parallel()

		styles := newTUIStyles()

		t.Run("ShowsStatusWithColor", func(t *testing.T) {
			t.Parallel()

			output := renderStatusBar(styles, nil, codersdk.ChatStatusRunning, nil, 0, false, false, 80)
			require.Contains(t, output, styles.statusColor(codersdk.ChatStatusRunning).Render(string(codersdk.ChatStatusRunning)))
		})

		t.Run("ShowsTokenUsageWhenPresent", func(t *testing.T) {
			t.Parallel()

			usage := &codersdk.ChatMessageUsage{TotalTokens: int64Ptr(50), ContextLimit: int64Ptr(100)}
			output := plainText(renderStatusBar(styles, nil, codersdk.ChatStatusRunning, usage, 0, false, false, 80))
			require.Contains(t, output, "tokens: 50/100")
		})

		t.Run("WarnsWhenUsageExceedsEightyPercent", func(t *testing.T) {
			t.Parallel()

			usage := &codersdk.ChatMessageUsage{TotalTokens: int64Ptr(81), ContextLimit: int64Ptr(100)}
			output := renderStatusBar(styles, nil, codersdk.ChatStatusRunning, usage, 0, false, false, 80)
			require.Contains(t, output, styles.warningText.Render("tokens: 81/100"))
		})

		t.Run("CriticalWhenUsageExceedsNinetyFivePercent", func(t *testing.T) {
			t.Parallel()

			usage := &codersdk.ChatMessageUsage{TotalTokens: int64Ptr(96), ContextLimit: int64Ptr(100)}
			output := renderStatusBar(styles, nil, codersdk.ChatStatusRunning, usage, 0, false, false, 80)
			require.Contains(t, output, styles.criticalText.Render("tokens: 96/100"))
		})

		t.Run("ShowsQueueCount", func(t *testing.T) {
			t.Parallel()

			output := plainText(renderStatusBar(styles, nil, codersdk.ChatStatusPending, nil, 2, false, false, 80))
			require.Contains(t, output, "queued: 2")
		})

		t.Run("ShowsInterrupting", func(t *testing.T) {
			t.Parallel()

			output := plainText(renderStatusBar(styles, nil, codersdk.ChatStatusRunning, nil, 0, true, false, 80))
			require.Contains(t, output, "interrupting…")
		})

		t.Run("ShowsReconnecting", func(t *testing.T) {
			t.Parallel()

			output := plainText(renderStatusBar(styles, nil, codersdk.ChatStatusRunning, nil, 0, false, true, 80))
			require.Contains(t, output, "reconnecting…")
		})

		t.Run("OmitsUsageWhenNil", func(t *testing.T) {
			t.Parallel()

			output := plainText(renderStatusBar(styles, nil, codersdk.ChatStatusRunning, nil, 0, false, false, 80))
			require.NotContains(t, output, "tokens:")
		})

		t.Run("NarrowWidthFits", func(t *testing.T) {
			t.Parallel()

			usage := &codersdk.ChatMessageUsage{TotalTokens: int64Ptr(96), ContextLimit: int64Ptr(100)}
			var output string
			require.NotPanics(t, func() {
				output = renderStatusBar(styles, nil, codersdk.ChatStatusRunning, usage, 2, true, true, 20)
			})
			require.NotEmpty(t, plainText(output))
			require.LessOrEqual(t, lipgloss.Width(output), 20)
		})
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

		t.Run("ReasoningCollapsedClampsToThreeLines", func(t *testing.T) {
			t.Parallel()

			output := plainText(renderBlock(styles, chatBlock{kind: blockReasoning, role: codersdk.ChatMessageRoleAssistant, text: "line1\nline2\nline3\nline4"}, false, 40))
			lines := strings.Split(output, "\n")
			require.Len(t, lines, 3)
			require.Contains(t, lines[0], "💭 line1")
			require.Equal(t, "line3…", strings.TrimRight(lines[2], " "))
		})

		t.Run("ReasoningExpandedShowsFullText", func(t *testing.T) {
			t.Parallel()

			output := plainText(renderBlock(styles, chatBlock{kind: blockReasoning, role: codersdk.ChatMessageRoleAssistant, text: "line1\nline2\nline3\nline4"}, true, 40))
			lines := strings.Split(output, "\n")
			require.Len(t, lines, 4)
			require.Contains(t, output, "line4")
			require.NotContains(t, output, "line4…")
		})

		t.Run("ToolCallCollapsedShowsOneLineSummary", func(t *testing.T) {
			t.Parallel()

			output := plainText(renderBlock(styles, chatBlock{kind: blockToolCall, toolName: "read_file", args: `{"path":"a.txt"}`}, false, 60))
			require.Contains(t, output, "⏳ read file")
			require.Contains(t, output, "(a.txt)")
			require.NotContains(t, output, "\n")
		})

		t.Run("CollapsedToolCallShowsRunCount", func(t *testing.T) {
			t.Parallel()

			output := plainText(renderBlock(styles, chatBlock{kind: blockToolCall, toolName: "github__get_pull_request", args: `{"owner":"openclaw","repo":"openclaw"}`, collapsedCount: 3}, false, 80))
			require.Contains(t, output, "⏳ get pull request... (x3 running)")
		})

		t.Run("ToolCallExpandedShowsFullArgs", func(t *testing.T) {
			t.Parallel()

			output := plainText(renderBlock(styles, chatBlock{kind: blockToolCall, toolName: "read_file", args: `{"path":"very/long/path.txt","recursive":true}`}, true, 60))
			require.Contains(t, output, "⏳ read file")
			require.Contains(t, output, "args:")
			require.Contains(t, output, `{"path":"very/long/path.txt","recursive":true}`)
			require.Contains(t, output, "\n")
		})

		t.Run("ToolResultCollapsedShowsOneLineSummary", func(t *testing.T) {
			t.Parallel()

			output := plainText(renderBlock(styles, chatBlock{kind: blockToolResult, toolName: "read_file", args: `{"path":"a.txt"}`, result: `{"ok":true}`}, false, 60))
			require.Contains(t, output, "✓ read file")
			require.Contains(t, output, "(a.txt)")
			require.NotContains(t, output, "\n")
		})

		t.Run("CollapsedToolResultShowsRunCount", func(t *testing.T) {
			t.Parallel()

			output := plainText(renderBlock(styles, chatBlock{kind: blockToolResult, toolName: "github__get_pull_request", args: `{"owner":"openclaw","repo":"openclaw"}`, result: `{"ok":true}`, collapsedCount: 10}, false, 80))
			require.Contains(t, output, "✓ get pull request (x10)")
		})

		t.Run("ToolResultExpandedShowsFullResult", func(t *testing.T) {
			t.Parallel()

			output := plainText(renderBlock(styles, chatBlock{kind: blockToolResult, toolName: "read_file", args: `{"path":"a.txt"}`, result: `{"path":"a.txt","contents":"hello"}`}, true, 60))
			require.Contains(t, output, "✓ read file")
			require.Contains(t, output, "args:")
			require.Contains(t, output, "result:")
			require.Contains(t, output, `{"path":"a.txt","contents":"hello"}`)
			require.Contains(t, output, "\n")
		})

		t.Run("CompactionRendersBanner", func(t *testing.T) {
			t.Parallel()

			output := plainText(renderBlock(styles, chatBlock{kind: blockCompaction}, false, 40))
			require.Contains(t, output, "🗜️  Context compacted")
		})

		t.Run("ExtremelyNarrow", func(t *testing.T) {
			t.Parallel()

			var output string
			require.NotPanics(t, func() {
				output = plainText(renderBlock(styles, chatBlock{kind: blockText, role: codersdk.ChatMessageRoleUser, text: "narrow terminal rendering still works"}, false, 15))
			})
			require.NotEmpty(t, output)
		})

		t.Run("ExtremelyWide", func(t *testing.T) {
			t.Parallel()

			var output string
			require.NotPanics(t, func() {
				output = plainText(renderBlock(styles, chatBlock{kind: blockText, role: codersdk.ChatMessageRoleUser, text: "wide terminal rendering still works"}, false, 250))
			})
			require.NotEmpty(t, output)
		})

		t.Run("WithEmoji", func(t *testing.T) {
			t.Parallel()

			output := plainText(renderBlock(styles, chatBlock{kind: blockText, role: codersdk.ChatMessageRoleAssistant, text: "Hello 👋 World 🌍"}, false, 40))
			require.Contains(t, output, "Hello 👋 World 🌍")
		})

		t.Run("WithCJKCharacters", func(t *testing.T) {
			t.Parallel()

			output := plainText(renderBlock(styles, chatBlock{kind: blockText, role: codersdk.ChatMessageRoleAssistant, text: "こんにちは世界"}, false, 40))
			require.Contains(t, output, "こんにちは世界")
		})

		t.Run("VeryLongMessage", func(t *testing.T) {
			t.Parallel()

			veryLong := strings.Repeat("abcde", 1000)
			var output string
			require.NotPanics(t, func() {
				output = plainText(renderBlock(styles, chatBlock{kind: blockText, role: codersdk.ChatMessageRoleAssistant, text: veryLong}, false, 60))
			})
			require.NotEmpty(t, output)
		})

		t.Run("VeryLongSingleLineWraps", func(t *testing.T) {
			t.Parallel()

			output := plainText(renderBlock(styles, chatBlock{kind: blockText, role: codersdk.ChatMessageRoleUser, text: strings.Repeat("a", 1000)}, false, 40))
			lines := strings.Split(output, "\n")
			require.Greater(t, len(lines), 1)
			for _, line := range lines {
				require.LessOrEqual(t, lipgloss.Width(line), 40)
			}
		})

		t.Run("EmptyText", func(t *testing.T) {
			t.Parallel()

			var output string
			require.NotPanics(t, func() {
				output = plainText(renderBlock(styles, chatBlock{kind: blockText, role: codersdk.ChatMessageRoleUser, text: ""}, false, 40))
			})
			require.Equal(t, "You: ", output)
		})

		t.Run("NilParts", func(t *testing.T) {
			t.Parallel()

			blocks := messagesToBlocks([]codersdk.ChatMessage{{
				Role: codersdk.ChatMessageRoleAssistant,
				Content: []codersdk.ChatMessagePart{
					{Type: codersdk.ChatMessagePartTypeText},
					{Type: codersdk.ChatMessagePartTypeToolCall},
					{Type: codersdk.ChatMessagePartTypeToolResult},
				},
			}})
			require.Len(t, blocks, 2)

			var output string
			require.NotPanics(t, func() {
				output = plainText(renderChatBlocks(styles, blocks, -1, map[int]bool{}, true, 40))
			})
			require.NotEmpty(t, output)
			require.Contains(t, output, "✓ tool")
		})
	})

	t.Run("RenderChatBlocks", func(t *testing.T) {
		t.Parallel()

		styles := newTUIStyles()
		block := chatBlock{kind: blockText, role: codersdk.ChatMessageRoleUser, text: "hello"}

		t.Run("EmptyBlocksReturnsEmptyString", func(t *testing.T) {
			t.Parallel()

			output := renderChatBlocks(styles, nil, 0, nil, false, 40)
			require.Empty(t, output)
		})

		t.Run("SelectedBlockGetsSelectedStyle", func(t *testing.T) {
			t.Parallel()

			blockView := renderBlock(styles, block, false, 40)
			output := renderChatBlocks(styles, []chatBlock{block}, 0, map[int]bool{}, false, 40)
			require.Equal(t, styles.selectedItem.Render(blockView), output)
		})

		t.Run("ComposerFocusDisablesSelectionHighlight", func(t *testing.T) {
			t.Parallel()

			blockView := renderBlock(styles, block, false, 40)
			output := renderChatBlocks(styles, []chatBlock{block}, 0, map[int]bool{}, true, 40)
			require.Equal(t, blockView, output)
		})

		t.Run("BlockCacheReusedOnSameWidthAndExpansion", func(t *testing.T) {
			t.Parallel()

			blocks := []chatBlock{
				{kind: blockText, role: codersdk.ChatMessageRoleUser, text: "hello"},
				{kind: blockReasoning, role: codersdk.ChatMessageRoleAssistant, text: "thinking through the answer"},
			}
			expandedBlocks := map[int]bool{1: true}

			first := renderChatBlocks(styles, blocks, 0, expandedBlocks, true, 40)
			require.NotEmpty(t, blocks[0].cachedRender)
			require.NotEmpty(t, blocks[1].cachedRender)

			cachedRenders := []string{blocks[0].cachedRender, blocks[1].cachedRender}
			second := renderChatBlocks(styles, blocks, 0, expandedBlocks, true, 40)
			require.Equal(t, first, second)
			require.Equal(t, cachedRenders[0], blocks[0].cachedRender)
			require.Equal(t, cachedRenders[1], blocks[1].cachedRender)
		})

		t.Run("BlockCacheInvalidatedOnWidthChange", func(t *testing.T) {
			t.Parallel()

			blocks := []chatBlock{{
				kind: blockText,
				role: codersdk.ChatMessageRoleUser,
				text: strings.Repeat("cache invalidation ", 8),
			}}

			renderChatBlocks(styles, blocks, 0, map[int]bool{}, true, 60)
			firstCache := blocks[0].cachedRender
			require.NotEmpty(t, firstCache)

			renderChatBlocks(styles, blocks, 0, map[int]bool{}, true, 40)
			require.NotEqual(t, firstCache, blocks[0].cachedRender)
		})

		t.Run("BlockCacheInvalidatedOnExpansionChange", func(t *testing.T) {
			t.Parallel()

			blocks := []chatBlock{{
				kind: blockReasoning,
				role: codersdk.ChatMessageRoleAssistant,
				text: "line one\nline two\nline three\nline four",
			}}
			expandedBlocks := map[int]bool{}

			renderChatBlocks(styles, blocks, 0, expandedBlocks, true, 40)
			firstCache := blocks[0].cachedRender
			require.NotEmpty(t, firstCache)

			expandedBlocks[0] = true
			renderChatBlocks(styles, blocks, 0, expandedBlocks, true, 40)
			require.NotEqual(t, firstCache, blocks[0].cachedRender)
		})

		t.Run("SelectionStylingDoesNotPoisonCache", func(t *testing.T) {
			t.Parallel()

			blocks := []chatBlock{{kind: blockText, role: codersdk.ChatMessageRoleUser, text: "hello"}}

			renderChatBlocks(styles, blocks, 0, map[int]bool{}, false, 40)
			cachedRender := blocks[0].cachedRender
			require.NotEmpty(t, cachedRender)

			renderChatBlocks(styles, blocks, 0, map[int]bool{}, true, 40)
			require.Equal(t, cachedRender, blocks[0].cachedRender)
		})

		t.Run("CollapsesConsecutiveSameNameToolCalls", func(t *testing.T) {
			t.Parallel()

			blocks := []chatBlock{
				{kind: blockToolCall, toolName: "github__get_pull_request", args: `{"owner":"openclaw","repo":"openclaw","pull_number":1}`},
				{kind: blockToolCall, toolName: "github__get_pull_request", args: `{"owner":"openclaw","repo":"openclaw","pull_number":2}`},
				{kind: blockToolCall, toolName: "github__get_pull_request", args: `{"owner":"openclaw","repo":"openclaw","pull_number":3}`},
			}

			output := plainText(renderChatBlocks(styles, blocks, -1, map[int]bool{}, true, 80))
			require.Equal(t, 1, strings.Count(output, "⏳"))
			require.Contains(t, output, "get pull request... (x3 running)")
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

		t.Run("MultipleToolCalls", func(t *testing.T) {
			t.Parallel()

			blocks := []chatBlock{
				{kind: blockToolCall, toolName: "tool_one", args: `{}`},
				{kind: blockToolCall, toolName: "tool_two", args: `{}`},
				{kind: blockToolCall, toolName: "tool_three", args: `{}`},
				{kind: blockToolCall, toolName: "tool_four", args: `{}`},
				{kind: blockToolCall, toolName: "tool_five", args: `{}`},
			}

			output := plainText(renderChatBlocks(styles, blocks, -1, map[int]bool{}, true, 60))
			require.Equal(t, 5, strings.Count(output, "⏳"))
			require.Contains(t, output, "tool one")
			require.Contains(t, output, "tool two")
			require.Contains(t, output, "tool three")
			require.Contains(t, output, "tool four")
			require.Contains(t, output, "tool five")
		})
	})
	t.Run("RenderDiffDrawer", func(t *testing.T) {
		t.Parallel()

		styles := newTUIStyles()
		branch := "feature/chat-ui"
		prURL := "https://example.com/pulls/123"

		t.Run("ShowsBranchInfoWhenPresent", func(t *testing.T) {
			t.Parallel()

			output := plainText(renderDiffDrawer(styles, codersdk.ChatDiffContents{Branch: &branch}, nil, 90, 20))
			require.Contains(t, output, "Branch: feature/chat-ui")
		})

		t.Run("ShowsPRURLWhenPresent", func(t *testing.T) {
			t.Parallel()

			output := plainText(renderDiffDrawer(styles, codersdk.ChatDiffContents{PullRequestURL: &prURL}, nil, 90, 20))
			require.Contains(t, output, "PR: https://example.com/pulls/123")
		})

		t.Run("ShowsDiffContent", func(t *testing.T) {
			t.Parallel()

			diff := codersdk.ChatDiffContents{Diff: "diff --git a/a.txt b/a.txt\n+added line"}
			changes := []codersdk.ChatGitChange{{FilePath: "a.txt", ChangeType: "modified"}}
			output := plainText(renderDiffDrawer(styles, diff, changes, 90, 20))
			require.Contains(t, output, "diff --git a/a.txt b/a.txt")
			require.Contains(t, output, "+added line")
		})

		t.Run("ShowsPlaceholderForEmptyDiff", func(t *testing.T) {
			t.Parallel()

			output := plainText(renderDiffDrawer(styles, codersdk.ChatDiffContents{}, nil, 90, 20))
			require.Contains(t, output, "No diff contents.")
		})

		t.Run("NarrowWidthDoesNotPanic", func(t *testing.T) {
			t.Parallel()

			var output string
			require.NotPanics(t, func() {
				output = plainText(renderDiffDrawer(styles, codersdk.ChatDiffContents{Diff: "diff --git a/a.txt b/a.txt\n+added line"}, []codersdk.ChatGitChange{{FilePath: "a.txt", ChangeType: "modified"}}, 25, 10))
			})
			require.NotEmpty(t, output)
		})
	})

	t.Run("RenderModelPicker", func(t *testing.T) {
		t.Parallel()

		styles := newTUIStyles()
		catalog := codersdk.ChatModelsResponse{
			Providers: []codersdk.ChatModelProvider{
				{
					Provider:  "OpenAI",
					Available: true,
					Models: []codersdk.ChatModel{
						{ID: "gpt-4o", Provider: "OpenAI", Model: "gpt-4o", DisplayName: "GPT-4o"},
						{ID: "gpt-4.1", Provider: "OpenAI", Model: "gpt-4.1", DisplayName: "GPT-4.1"},
					},
				},
				{
					Provider:          "Anthropic",
					Available:         false,
					UnavailableReason: codersdk.ChatModelProviderUnavailableMissingAPIKey,
				},
				{
					Provider:  "Local",
					Available: true,
					Models:    nil,
				},
			},
		}

		t.Run("GroupsModelsByProvider", func(t *testing.T) {
			t.Parallel()

			output := plainText(renderModelPicker(styles, catalog, "gpt-4o", 0, 90, 20))
			require.Contains(t, output, "OpenAI")
			require.Contains(t, output, "GPT-4o")
			require.Contains(t, output, "GPT-4.1")
		})

		t.Run("ShowsCursorIndicatorOnSelectedPosition", func(t *testing.T) {
			t.Parallel()

			output := plainText(renderModelPicker(styles, catalog, "gpt-4.1", 1, 90, 20))
			require.Contains(t, output, "> GPT-4.1")
			require.Contains(t, output, "  GPT-4o")
		})

		t.Run("UnavailableProvidersShowReason", func(t *testing.T) {
			t.Parallel()

			output := plainText(renderModelPicker(styles, catalog, "gpt-4o", 0, 90, 20))
			require.Contains(t, output, "Anthropic")
			require.Contains(t, output, "missing_api_key")
		})

		t.Run("EmptyProvidersShowNoModelsMessage", func(t *testing.T) {
			t.Parallel()

			output := plainText(renderModelPicker(styles, catalog, "gpt-4o", 0, 90, 20))
			require.Contains(t, output, "Local")
			require.Contains(t, output, "No models available.")
		})

		t.Run("NarrowWidthDoesNotPanic", func(t *testing.T) {
			t.Parallel()

			var output string
			require.NotPanics(t, func() {
				output = plainText(renderModelPicker(styles, catalog, "gpt-4o", 0, 25, 10))
			})
			require.NotEmpty(t, output)
		})
	})

	t.Run("RenderAssistantMarkdown", func(t *testing.T) {
		t.Parallel()

		styles := newTUIStyles()

		t.Run("UsesExplicitDarkStyle", func(t *testing.T) {
			t.Parallel()

			output := plainText(renderAssistantMarkdown(styles, "- first\n- second", 60, nil))
			require.Contains(t, output, "• first")
			require.Contains(t, output, "• second")
			require.NotContains(t, output, "- first")
		})
	})
	t.Run("ViewHelpFitsNarrowTerminals", func(t *testing.T) {
		t.Parallel()

		t.Run("ListViewShortensHelpAt80Columns", func(t *testing.T) {
			t.Parallel()

			list := newChatListModel(newTUIStyles())
			list.loading = false
			list.width = 80
			list.chats = []codersdk.Chat{testChat(codersdk.ChatStatusCompleted)}

			output := plainText(list.View())
			lines := strings.Split(output, "\n")
			helpLine := lines[len(lines)-1]
			require.LessOrEqual(t, lipgloss.Width(helpLine), 80)
			require.Contains(t, helpLine, "q quit")
		})

		t.Run("ChatViewShortensHelpAt80Columns", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)
			chat := testChat(codersdk.ChatStatusRunning)
			model.loading = false
			model.width = 80
			model.height = 24
			model.chat = &chat
			model.chatStatus = chat.Status
			model.composerFocused = false

			output := plainText(model.View())
			lines := strings.Split(output, "\n")
			helpLine := lines[len(lines)-1]
			require.LessOrEqual(t, lipgloss.Width(helpLine), 80)
			require.Contains(t, helpLine, "ctrl+p")
			require.Contains(t, helpLine, "ctrl+d")
			require.NotContains(t, helpLine, "enter: expand/collapse")
		})
	})

	t.Run("UtilityRenderers", func(t *testing.T) {
		t.Parallel()

		t.Run("WrapPreservingNewlinesPreservesExplicitNewlines", func(t *testing.T) {
			t.Parallel()

			output := wrapPreservingNewlines("line one\nline two", 40)
			require.Contains(t, output, "line one\nline two")
		})

		t.Run("ClampLinesAddsEllipsis", func(t *testing.T) {
			t.Parallel()

			output := clampLines("line1\nline2\nline3\nline4", 3)
			lines := strings.Split(output, "\n")
			require.Len(t, lines, 3)
			require.Equal(t, "line3…", lines[2])
		})

		t.Run("RenderPrefixedBlockIndentsContinuationLines", func(t *testing.T) {
			t.Parallel()

			prefix := "You: "
			output := renderPrefixedBlock(prefix, "alpha beta gamma delta", 12)
			lines := strings.Split(output, "\n")
			require.GreaterOrEqual(t, len(lines), 2)
			require.True(t, strings.HasPrefix(lines[1], strings.Repeat(" ", lipgloss.Width(prefix))))
			require.Contains(t, output, prefix)
		})

		t.Run("RenderPrefixedBlockEmptyContent", func(t *testing.T) {
			t.Parallel()

			require.Equal(t, "You: ", renderPrefixedBlock("You: ", "", 12))
		})

		t.Run("WrapPreservingNewlinesEmptyString", func(t *testing.T) {
			t.Parallel()

			require.Empty(t, wrapPreservingNewlines("", 40))
		})

		t.Run("WrapPreservingNewlinesOnlyNewlines", func(t *testing.T) {
			t.Parallel()

			require.Equal(t, "\n\n\n", wrapPreservingNewlines("\n\n\n", 40))
		})

		t.Run("ClampLinesZeroMax", func(t *testing.T) {
			t.Parallel()

			require.Empty(t, clampLines("line1\nline2", 0))
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

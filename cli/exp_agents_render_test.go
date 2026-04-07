package cli //nolint:testpackage // Tests unexported chat TUI render helpers.

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

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
				name: "MergesConsecutiveToolCallAndResult",
				messages: []codersdk.ChatMessage{
					{
						Role: codersdk.ChatMessageRoleAssistant,
						Content: []codersdk.ChatMessagePart{{
							Type:       codersdk.ChatMessagePartTypeToolCall,
							ToolName:   "github__get_pull_request",
							ToolCallID: "call-merge",
							Args:       rawJSON(`{"owner":"openclaw","repo":"openclaw","pull_number":58036}`),
						}},
					},
					{
						Role: codersdk.ChatMessageRoleTool,
						Content: []codersdk.ChatMessagePart{{
							Type:       codersdk.ChatMessagePartTypeToolResult,
							ToolName:   "github__get_pull_request",
							ToolCallID: "call-merge",
							Result:     rawJSON(`{"base":{"ref":"main"}}`),
						}},
					},
				},
				assert: func(t *testing.T, blocks []chatBlock) {
					t.Helper()
					require.Len(t, blocks, 1)
					require.Equal(t, blockToolResult, blocks[0].kind)
					require.Equal(t, `{"owner":"openclaw","repo":"openclaw","pull_number":58036}`, blocks[0].args)
					require.Equal(t, `{"base":{"ref":"main"}}`, blocks[0].result)
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
	})

	t.Run("RenderToolResult", func(t *testing.T) {
		t.Parallel()

		styles := newTUIStyles()

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

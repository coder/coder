package fixtures

import (
	_ "embed"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/txtar"
)

var (
	//go:embed anthropic/simple.txtar
	AntSimple []byte

	//go:embed anthropic/single_builtin_tool.txtar
	AntSingleBuiltinTool []byte

	//go:embed anthropic/multi_thinking_builtin_tool.txtar
	AntMultiThinkingBuiltinTool []byte

	//go:embed anthropic/single_builtin_tool_parallel.txtar
	AntSingleBuiltinToolParallel []byte

	//go:embed anthropic/single_injected_tool.txtar
	AntSingleInjectedTool []byte

	//go:embed anthropic/fallthrough.txtar
	AntFallthrough []byte

	//go:embed anthropic/stream_error.txtar
	AntMidStreamError []byte

	//go:embed anthropic/non_stream_error.txtar
	AntNonStreamError []byte

	//go:embed anthropic/simple_bedrock.txtar
	AntSimpleBedrock []byte

	//go:embed anthropic/haiku_simple.txtar
	AntHaikuSimple []byte
)

var (
	//go:embed openai/chatcompletions/simple.txtar
	OaiChatSimple []byte

	//go:embed openai/chatcompletions/single_builtin_tool.txtar
	OaiChatSingleBuiltinTool []byte

	//go:embed openai/chatcompletions/single_injected_tool.txtar
	OaiChatSingleInjectedTool []byte

	//go:embed openai/chatcompletions/fallthrough.txtar
	OaiChatFallthrough []byte

	//go:embed openai/chatcompletions/stream_error.txtar
	OaiChatMidStreamError []byte

	//go:embed openai/chatcompletions/non_stream_error.txtar
	OaiChatNonStreamError []byte

	//go:embed openai/chatcompletions/streaming_injected_tool_no_preamble.txtar
	OaiChatStreamingInjectedToolNoPreamble []byte

	//go:embed openai/chatcompletions/streaming_injected_tool_nonzero_index.txtar
	OaiChatStreamingInjectedToolNonzeroIndex []byte
)

var (
	//go:embed openai/responses/blocking/simple.txtar
	OaiResponsesBlockingSimple []byte

	//go:embed openai/responses/blocking/single_builtin_tool.txtar
	OaiResponsesBlockingSingleBuiltinTool []byte

	//go:embed openai/responses/blocking/multi_reasoning_builtin_tool.txtar
	OaiResponsesBlockingMultiReasoningBuiltinTool []byte

	//go:embed openai/responses/blocking/commentary_builtin_tool.txtar
	OaiResponsesBlockingCommentaryBuiltinTool []byte

	//go:embed openai/responses/blocking/summary_and_commentary_builtin_tool.txtar
	OaiResponsesBlockingSummaryAndCommentaryBuiltinTool []byte

	//go:embed openai/responses/blocking/cached_input_tokens.txtar
	OaiResponsesBlockingCachedInputTokens []byte

	//go:embed openai/responses/blocking/custom_tool.txtar
	OaiResponsesBlockingCustomTool []byte

	//go:embed openai/responses/blocking/conversation.txtar
	OaiResponsesBlockingConversation []byte

	//go:embed openai/responses/blocking/http_error.txtar
	OaiResponsesBlockingHTTPErr []byte

	//go:embed openai/responses/blocking/prev_response_id.txtar
	OaiResponsesBlockingPrevResponseID []byte

	//go:embed openai/responses/blocking/single_builtin_tool_parallel.txtar
	OaiResponsesBlockingSingleBuiltinToolParallel []byte

	//go:embed openai/responses/blocking/single_injected_tool.txtar
	OaiResponsesBlockingSingleInjectedTool []byte

	//go:embed openai/responses/blocking/single_injected_tool_error.txtar
	OaiResponsesBlockingSingleInjectedToolError []byte

	//go:embed openai/responses/blocking/wrong_response_format.txtar
	OaiResponsesBlockingWrongResponseFormat []byte

	//go:embed openai/responses/blocking/web_search.txtar
	OaiResponsesBlockingWebSearch []byte
)

var (
	//go:embed openai/responses/streaming/simple.txtar
	OaiResponsesStreamingSimple []byte

	//go:embed openai/responses/streaming/codex_example.txtar
	OaiResponsesStreamingCodex []byte

	//go:embed openai/responses/streaming/builtin_tool.txtar
	OaiResponsesStreamingBuiltinTool []byte

	//go:embed openai/responses/streaming/multi_reasoning_builtin_tool.txtar
	OaiResponsesStreamingMultiReasoningBuiltinTool []byte

	//go:embed openai/responses/streaming/commentary_builtin_tool.txtar
	OaiResponsesStreamingCommentaryBuiltinTool []byte

	//go:embed openai/responses/streaming/summary_and_commentary_builtin_tool.txtar
	OaiResponsesStreamingSummaryAndCommentaryBuiltinTool []byte

	//go:embed openai/responses/streaming/cached_input_tokens.txtar
	OaiResponsesStreamingCachedInputTokens []byte

	//go:embed openai/responses/streaming/custom_tool.txtar
	OaiResponsesStreamingCustomTool []byte

	//go:embed openai/responses/streaming/conversation.txtar
	OaiResponsesStreamingConversation []byte

	//go:embed openai/responses/streaming/http_error.txtar
	OaiResponsesStreamingHTTPErr []byte

	//go:embed openai/responses/streaming/prev_response_id.txtar
	OaiResponsesStreamingPrevResponseID []byte

	//go:embed openai/responses/streaming/single_builtin_tool_parallel.txtar
	OaiResponsesStreamingSingleBuiltinToolParallel []byte

	//go:embed openai/responses/streaming/single_injected_tool.txtar
	OaiResponsesStreamingSingleInjectedTool []byte

	//go:embed openai/responses/streaming/single_injected_tool_error.txtar
	OaiResponsesStreamingSingleInjectedToolError []byte

	//go:embed openai/responses/streaming/stream_error.txtar
	OaiResponsesStreamingStreamError []byte

	//go:embed openai/responses/streaming/stream_failure.txtar
	OaiResponsesStreamingStreamFailure []byte

	//go:embed openai/responses/streaming/wrong_response_format.txtar
	OaiResponsesStreamingWrongResponseFormat []byte
)

// Section name constants matching the file names used in txtar fixtures.
const (
	fileRequest              = "request"
	fileStreamingResponse    = "streaming"
	fileNonStreamingResponse = "non-streaming"
	fileStreamingToolCall    = "streaming/tool-call"
	fileNonStreamingToolCall = "non-streaming/tool-call"

	// Exported aliases so callers can check [Fixture.Has] before calling a
	// getter that would otherwise fail the test.
	SectionStreaming         = fileStreamingResponse
	SectionNonStreaming      = fileNonStreamingResponse
	SectionStreamingToolCall = fileStreamingToolCall
	SectionNonStreamToolCall = fileNonStreamingToolCall
)

// Fixture holds the named sections of a parsed txtar test fixture.
type Fixture struct {
	sections map[string][]byte
	t        *testing.T
}

// Has reports whether the fixture contains the named section.
func (f Fixture) Has(name string) bool {
	_, ok := f.sections[name]
	return ok
}

func (f Fixture) Request() []byte {
	f.t.Helper()
	v, ok := f.sections[fileRequest]
	require.True(f.t, ok, "fixture archive missing %q section", fileRequest)
	return v
}

func (f Fixture) Streaming() []byte {
	f.t.Helper()
	v, ok := f.sections[fileStreamingResponse]
	require.True(f.t, ok, "fixture archive missing %q section", fileStreamingResponse)
	return v
}

func (f Fixture) NonStreaming() []byte {
	f.t.Helper()
	v, ok := f.sections[fileNonStreamingResponse]
	require.True(f.t, ok, "fixture archive missing %q section", fileNonStreamingResponse)
	return v
}

func (f Fixture) StreamingToolCall() []byte {
	f.t.Helper()
	v, ok := f.sections[fileStreamingToolCall]
	require.True(f.t, ok, "fixture archive missing %q section", fileStreamingToolCall)
	return v
}

func (f Fixture) NonStreamingToolCall() []byte {
	f.t.Helper()
	v, ok := f.sections[fileNonStreamingToolCall]
	require.True(f.t, ok, "fixture archive missing %q section", fileNonStreamingToolCall)
	return v
}

// Parse parses raw txtar data into a [Fixture].
func Parse(t *testing.T, data []byte) Fixture {
	t.Helper()

	archive := txtar.Parse(data)
	require.NotEmpty(t, archive.Files, "fixture archive has no files")

	sections := make(map[string][]byte, len(archive.Files))
	for _, f := range archive.Files {
		sections[f.Name] = f.Data
	}
	return Fixture{sections: sections, t: t}
}

// Request extracts the "request" fixture from raw txtar data.
func Request(t *testing.T, fixture []byte) []byte {
	t.Helper()
	return Parse(t, fixture).Request()
}

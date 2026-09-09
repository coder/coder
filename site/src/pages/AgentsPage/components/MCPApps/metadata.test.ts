import type { ChatMessagePart } from "#/api/typesGenerated";
import { MockChatMCPApp } from "#/testHelpers/chatEntities";
import {
	mergeTools,
	parseMessageContent,
} from "../ChatConversation/messageParsing";
import {
	applyMessagePartToStreamState,
	buildStreamTools,
	createEmptyStreamState,
} from "../ChatConversation/streamState";
import { mcpAppResourceURL } from "./resourceURL";

const parts: ChatMessagePart[] = [
	{
		type: "tool-call",
		tool_call_id: "tool",
		tool_name: "charts",
		args: { metric: "sales" },
	},
	{
		type: "tool-result",
		tool_call_id: "tool",
		tool_name: "charts",
		result: { output: "Sales chart" },
		mcp_app: MockChatMCPApp,
	},
];

it("preserves MCP App metadata through persisted and streaming tool pairing", () => {
	const parsed = parseMessageContent(parts);
	expect(mergeTools(parsed.toolCalls, parsed.toolResults)[0]?.mcpApp).toEqual(
		MockChatMCPApp,
	);
	expect(mergeTools([], parsed.toolResults)[0]?.mcpApp).toEqual(MockChatMCPApp);
	let state = createEmptyStreamState();
	for (const part of parts)
		state =
			applyMessagePartToStreamState(state, part) ?? createEmptyStreamState();
	expect(
		buildStreamTools(state.toolCalls, state.toolResults)[0]?.mcpApp,
	).toEqual(MockChatMCPApp);
	expect(buildStreamTools({}, state.toolResults)[0]?.mcpApp).toEqual(
		MockChatMCPApp,
	);
});

it("encodes the resource binding as a same-origin document URL", () => {
	const url = new URL(
		mcpAppResourceURL("chat/id", "server & x", "ui://chart?x=1&y=2"),
		"https://coder.example",
	);
	expect(url.origin).toBe("https://coder.example");
	expect(url.pathname).toBe("/api/v2/chats/chat%2Fid/mcp-apps/resource");
	expect([...url.searchParams.entries()]).toEqual([
		["server", "server & x"],
		["uri", "ui://chart?x=1&y=2"],
	]);
});

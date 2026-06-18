import { describe, expect, it } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import { MockChatMessage } from "#/testHelpers/chatEntities";
import {
	buildDisplayMessages,
	deriveMessageDisplayState,
} from "./messageHelpers";
import {
	parseMessageContent,
	parseMessagesWithMergedTools,
} from "./messageParsing";
import type {
	MergedTool,
	ParsedMessageContent,
	ParsedMessageEntry,
} from "./types";

const buildMessage = (
	content: TypesGen.ChatMessagePart[],
	role: "user" | "assistant" = "user",
): TypesGen.ChatMessage => ({ ...MockChatMessage, content, role });

const getDisplayState = (
	message: TypesGen.ChatMessage,
	overrides: Partial<Parameters<typeof deriveMessageDisplayState>[0]> = {},
) =>
	deriveMessageDisplayState({
		message,
		parsed: parseMessageContent(message.content),
		hideActions: false,
		hasActiveStream: false,
		...overrides,
	});

const baseMessage = {
	chat_id: "chat",
	created_at: "2026-03-10T00:00:00.000Z",
} as const;

const parsed = (
	overrides: Partial<ParsedMessageContent> = {},
): ParsedMessageContent => ({
	markdown: "",
	reasoning: "",
	toolCalls: [],
	toolResults: [],
	tools: [],
	blocks: [],
	sources: [],
	...overrides,
});

const entry = ({
	messageID,
	role = "assistant",
	content = [],
	parsedOverrides,
}: {
	messageID: number;
	role?: TypesGen.ChatMessageRole;
	content?: TypesGen.ChatMessagePart[];
	parsedOverrides: Partial<ParsedMessageContent>;
}): ParsedMessageEntry => ({
	message: { ...baseMessage, id: messageID, role, content },
	parsed: parsed(parsedOverrides),
});

const readFileArgs = (id: string) => ({ path: `${id}.ts` });

const readFileTool = (id: string): MergedTool => ({
	id,
	name: "read_file",
	args: readFileArgs(id),
	result: { content: id },
	isError: false,
	status: "completed",
});

const readFileToolResult = (id: string) => ({
	id,
	name: "read_file" as const,
	result: { content: id },
	isError: false,
});

const readFileMessage = (
	messageID: number,
	toolID: string,
	parsedOverrides: Partial<ParsedMessageContent> = {},
): ParsedMessageEntry => {
	const args = readFileArgs(toolID);
	return entry({
		messageID,
		parsedOverrides: {
			toolCalls: [{ id: toolID, name: "read_file", args }],
			toolResults: [readFileToolResult(toolID)],
			tools: [readFileTool(toolID)],
			blocks: [{ type: "tool", id: toolID }],
			...parsedOverrides,
		},
	});
};

const hiddenToolResultMessage = (
	messageID: number,
	toolID: string,
): ParsedMessageEntry =>
	entry({
		messageID,
		role: "tool",
		parsedOverrides: {
			toolResults: [readFileToolResult(toolID)],
			tools: [readFileTool(toolID)],
			blocks: [{ type: "tool", id: toolID }],
		},
	});

const textMessage = (
	messageID: number,
	text: string,
	role: TypesGen.ChatMessageRole = "assistant",
): ParsedMessageEntry =>
	entry({
		messageID,
		role,
		content: [{ type: "text", text }],
		parsedOverrides: {
			markdown: text,
			blocks: [{ type: "response", text }],
		},
	});

const executeMessage = (messageID: number): ParsedMessageEntry => {
	const args = { command: "pnpm test" };
	const tool: MergedTool = {
		id: "execute-1",
		name: "execute",
		args,
		isError: false,
		status: "completed",
	};
	return entry({
		messageID,
		parsedOverrides: {
			toolCalls: [{ id: tool.id, name: tool.name, args }],
			tools: [tool],
			blocks: [{ type: "tool", id: tool.id }],
		},
	});
};

const message = ({
	messageID,
	role,
	content,
}: {
	messageID: number;
	role: TypesGen.ChatMessageRole;
	content: TypesGen.ChatMessagePart[];
}): TypesGen.ChatMessage => ({
	...baseMessage,
	id: messageID,
	role,
	content,
});

describe("deriveMessageDisplayState", () => {
	it("marks text-only user messages as copyable", () => {
		const message = buildMessage([{ type: "text", text: "Copy this" }]);

		expect(getDisplayState(message).hasCopyableContent).toBe(true);
	});

	it("marks text-only assistant messages as copyable", () => {
		const message = buildMessage(
			[{ type: "text", text: "Here is my answer." }],
			"assistant",
		);

		expect(getDisplayState(message).hasCopyableContent).toBe(true);
	});

	it("does not mark user messages with file attachments as copyable", () => {
		const message = buildMessage([
			{ type: "text", text: "Copy should not omit this file." },
			{ type: "file", media_type: "text/plain", file_id: "file-1" },
		]);

		expect(getDisplayState(message).hasCopyableContent).toBe(false);
	});

	it("does not mark assistant messages with file attachments as copyable", () => {
		const message = buildMessage(
			[
				{ type: "text", text: "Generated file attached." },
				{ type: "file", media_type: "image/png", file_id: "image-1" },
			],
			"assistant",
		);

		expect(getDisplayState(message).hasCopyableContent).toBe(false);
	});

	it("does not mark assistant messages ending with a tool call as copyable", () => {
		const tool: MergedTool = {
			id: "execute-1",
			name: "execute",
			args: { command: "pnpm test" },
			isError: false,
			status: "completed",
		};
		const message = buildMessage(
			[{ type: "text", text: "Running the tests now." }],
			"assistant",
		);

		const state = getDisplayState(message, {
			parsed: parsed({
				markdown: "Running the tests now.",
				tools: [tool],
				blocks: [
					{ type: "response", text: "Running the tests now." },
					{ type: "tool", id: tool.id },
				],
			}),
		});

		expect(state.hasCopyableContent).toBe(false);
	});

	it("marks assistant messages ending with text after a tool call as copyable", () => {
		const tool: MergedTool = {
			id: "execute-1",
			name: "execute",
			args: { command: "pnpm test" },
			isError: false,
			status: "completed",
		};
		const message = buildMessage(
			[{ type: "text", text: "All tests passed." }],
			"assistant",
		);

		const state = getDisplayState(message, {
			parsed: parsed({
				markdown: "All tests passed.",
				tools: [tool],
				blocks: [
					{ type: "tool", id: tool.id },
					{ type: "response", text: "All tests passed." },
				],
			}),
		});

		expect(state.hasCopyableContent).toBe(true);
	});

	it("does not mark assistant messages ending with a thinking block as copyable", () => {
		// Intended: the action row renders below the whole message, so a
		// trailing thinking disclosure has the same visual problem as a
		// trailing tool call even though copyable markdown exists.
		const message = buildMessage(
			[{ type: "text", text: "Here is my answer." }],
			"assistant",
		);

		const state = getDisplayState(message, {
			parsed: parsed({
				markdown: "Here is my answer.",
				blocks: [
					{ type: "response", text: "Here is my answer." },
					{ type: "thinking", text: "Reconsidering the edge cases." },
				],
			}),
		});

		expect(state.hasCopyableContent).toBe(false);
	});

	it("shows the assistant spacer for reasoning messages when no suppressing flags apply", () => {
		const message = buildMessage(
			[{ type: "reasoning", text: "I should think before answering." }],
			"assistant",
		);

		expect(getDisplayState(message).needsAssistantBottomSpacer).toBe(true);
	});

	it("suppresses the assistant spacer while awaiting the first stream chunk", () => {
		const message = buildMessage(
			[{ type: "reasoning", text: "I should think before answering." }],
			"assistant",
		);

		expect(
			getDisplayState(message, { isAwaitingFirstStreamChunk: true })
				.needsAssistantBottomSpacer,
		).toBe(false);
	});

	it("keeps the assistant spacer hidden when actions are hidden", () => {
		const message = buildMessage(
			[{ type: "reasoning", text: "I should think before answering." }],
			"assistant",
		);

		expect(
			getDisplayState(message, { hideActions: true })
				.needsAssistantBottomSpacer,
		).toBe(false);
	});

	it("keeps the assistant spacer hidden when a stream is active", () => {
		const message = buildMessage(
			[{ type: "reasoning", text: "I should think before answering." }],
			"assistant",
		);

		expect(
			getDisplayState(message, { hasActiveStream: true })
				.needsAssistantBottomSpacer,
		).toBe(false);
	});

	it("never shows the assistant spacer on user messages", () => {
		const message = buildMessage([{ type: "text", text: "Hello" }], "user");

		expect(getDisplayState(message).needsAssistantBottomSpacer).toBe(false);
	});

	it("hides assistant messages with no renderable content", () => {
		const message = buildMessage([], "assistant");

		expect(getDisplayState(message).shouldHide).toBe(true);
	});

	it("hides assistant messages whose execute tool renders nothing", () => {
		const message = buildMessage(
			[
				{
					type: "tool-call",
					tool_call_id: "tool-1",
					tool_name: "execute",
					args: {},
				},
			],
			"assistant",
		);

		expect(getDisplayState(message).shouldHide).toBe(true);
	});

	it("keeps assistant messages visible when execute shows a real command", () => {
		const message = buildMessage(
			[
				{
					type: "tool-call",
					tool_call_id: "tool-1",
					tool_name: "execute",
					args: { command: "pnpm test" },
				},
			],
			"assistant",
		);

		expect(
			getDisplayState(message, {
				parsed: parsed({
					tools: [
						{
							id: "tool-1",
							name: "execute",
							args: { command: "pnpm test" },
							isError: false,
							status: "completed",
						},
					],
					blocks: [{ type: "tool", id: "tool-1" }],
				}),
			}).shouldHide,
		).toBe(false);
	});

	it("hides running wait_agent messages until the chat id is available", () => {
		const message = buildMessage([], "assistant");
		const parsedContent: ParsedMessageContent = {
			...parseMessageContent(message.content),
			blocks: [{ type: "tool", id: "wait-1" }],
			tools: [
				{
					id: "wait-1",
					name: "wait_agent",
					args: {},
					isError: false,
					status: "running",
				},
			],
		};

		expect(
			deriveMessageDisplayState({
				message,
				parsed: parsedContent,
				hideActions: false,
				hasActiveStream: false,
			}).shouldHide,
		).toBe(true);
	});
});

describe("buildDisplayMessages", () => {
	it("keeps durable tool calls visible after parser-level result merging", () => {
		const result = buildDisplayMessages(
			parseMessagesWithMergedTools([
				message({
					messageID: 1,
					role: "assistant",
					content: [
						{
							type: "tool-call",
							tool_call_id: "list-templates-1",
							tool_name: "list_templates",
							args: {},
						},
					],
				}),
				message({
					messageID: 2,
					role: "tool",
					content: [
						{
							type: "tool-result",
							tool_call_id: "list-templates-1",
							tool_name: "list_templates",
							result: {
								count: "1",
								templates: '[{"name":"docker","display_name":"Docker"}]',
							},
						},
					],
				}),
			]),
		);

		expect(result).toHaveLength(1);
		expect(result[0].message.id).toBe(1);
		expect(result[0].parsed.tools).toEqual([
			{
				id: "list-templates-1",
				name: "list_templates",
				args: {},
				result: {
					count: "1",
					templates: '[{"name":"docker","display_name":"Docker"}]',
				},
				isError: false,
				status: "completed",
				mcpServerConfigId: undefined,
				modelIntent: undefined,
				parsedCommands: undefined,
			},
		]);
		expect(result[0].parsed.blocks).toEqual([
			{ type: "tool", id: "list-templates-1" },
		]);
	});

	it("returns a single read_file-only message unchanged", () => {
		const readFile = readFileMessage(1, "read-1");

		const result = buildDisplayMessages([readFile]);

		expect(result).toHaveLength(1);
		expect(result[0]).toBe(readFile);
	});

	it("collapses read_file-only assistant messages across hidden tool results", () => {
		const result = buildDisplayMessages([
			readFileMessage(1, "read-1"),
			hiddenToolResultMessage(2, "read-1"),
			readFileMessage(3, "read-2"),
			hiddenToolResultMessage(4, "read-2"),
		]);

		expect(result).toHaveLength(1);
		expect(result[0].message.id).toBe(1);
		expect(result[0].parsed.toolCalls).toEqual([
			{ id: "read-1", name: "read_file", args: { path: "read-1.ts" } },
			{ id: "read-2", name: "read_file", args: { path: "read-2.ts" } },
		]);
		expect(result[0].parsed.toolResults).toEqual([
			readFileToolResult("read-1"),
			readFileToolResult("read-2"),
		]);
		expect(result[0].parsed.blocks).toEqual([
			{ type: "tool", id: "read-1" },
			{ type: "tool", id: "read-2" },
		]);
		expect(result[0].parsed.tools.map((tool) => tool.id)).toEqual([
			"read-1",
			"read-2",
		]);
	});

	it.each([
		["assistant", textMessage(2, "middle")],
		["user", textMessage(2, "middle", "user")],
	] satisfies Array<
		[string, ParsedMessageEntry]
	>)("does not collapse read_file messages across visible %s content", (_, message) => {
		const result = buildDisplayMessages([
			readFileMessage(1, "read-1"),
			message,
			readFileMessage(3, "read-2"),
		]);

		expect(result.map((entry) => entry.message.id)).toEqual([1, 2, 3]);
		expect(result[0].parsed.blocks).toEqual([{ type: "tool", id: "read-1" }]);
		expect(result[2].parsed.blocks).toEqual([{ type: "tool", id: "read-2" }]);
	});

	it.each([
		["markdown", { markdown: "Visible markdown" }],
		["reasoning", { reasoning: "Visible reasoning" }],
		[
			"sources",
			{
				sources: [
					{ url: "https://example.com/read-2", title: "Read 2 source" },
				],
			},
		],
	] satisfies Array<
		[string, Partial<ParsedMessageContent>]
	>)("does not collapse read_file messages with visible %s", (_, overrides) => {
		const result = buildDisplayMessages([
			readFileMessage(1, "read-1"),
			readFileMessage(2, "read-2", overrides),
			readFileMessage(3, "read-3"),
		]);

		expect(result.map((entry) => entry.message.id)).toEqual([1, 2, 3]);
	});

	it("does not collapse read_file messages across another visible tool", () => {
		const result = buildDisplayMessages([
			readFileMessage(1, "read-1"),
			executeMessage(2),
			readFileMessage(3, "read-2"),
		]);

		expect(result.map((entry) => entry.message.id)).toEqual([1, 2, 3]);
	});
});

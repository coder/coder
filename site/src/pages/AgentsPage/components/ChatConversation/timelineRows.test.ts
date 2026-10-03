import { describe, expect, it } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import { buildDisplayMessages } from "./messageHelpers";
import { assignTimelineRows } from "./timelineRows";
import type {
	MergedTool,
	ParsedMessageContent,
	ParsedMessageEntry,
} from "./types";

const emptyParsed: ParsedMessageContent = {
	markdown: "",
	reasoning: "",
	toolCalls: [],
	toolResults: [],
	tools: [],
	blocks: [],
	sources: [],
	hookNotices: [],
};

const entry = (
	message: TypesGen.ChatMessage,
	text: string,
): ParsedMessageEntry => ({
	message,
	parsed: { ...emptyParsed, markdown: text },
});

const durable = (
	id: number,
	role: TypesGen.ChatMessage["role"],
	text: string,
): ParsedMessageEntry =>
	entry(
		{
			id,
			chat_id: "chat-1",
			role,
			created_at: "2026-08-12T00:00:00Z",
			content: [{ type: "text", text }],
		},
		text,
	);

const keys = (rows: ReturnType<typeof assignTimelineRows>): string[] =>
	rows.map((row) => row.key);

const readFileMessage = (id: number, toolID: string): ParsedMessageEntry => ({
	message: {
		id,
		chat_id: "chat-1",
		role: "assistant",
		created_at: "2026-08-12T00:00:00Z",
		content: [],
	},
	parsed: {
		...emptyParsed,
		toolCalls: [{ id: toolID, name: "read_file", args: { path: toolID } }],
		toolResults: [
			{
				id: toolID,
				name: "read_file",
				result: { content: toolID },
				isError: false,
			},
		],
		tools: [
			{
				id: toolID,
				name: "read_file",
				args: { path: toolID },
				result: { content: toolID },
				isError: false,
				status: "completed",
			} satisfies MergedTool,
		],
		blocks: [{ type: "tool", id: toolID }],
	},
});

describe("assignTimelineRows", () => {
	it("keeps durable row keys stable when an earlier turn is prepended", () => {
		const assistant = durable(2, "assistant", "answer");
		const before = assignTimelineRows([assistant], false);
		const after = assignTimelineRows(
			[durable(1, "user", "prompt"), assistant],
			false,
		);

		expect(keys(before)).toEqual(["message:2"]);
		expect(keys(after)).toEqual(["message:1", "message:2"]);
	});

	it("keys the live row independently of the loaded history", () => {
		const withPrompt = assignTimelineRows(
			[
				durable(1, "user", "prompt"),
				durable(2, "assistant", "first"),
				durable(3, "assistant", "second"),
			],
			true,
		);

		expect(keys(withPrompt)).toEqual([
			"message:1",
			"message:2",
			"message:3",
			"live-assistant",
		]);
		expect(keys(assignTimelineRows([], true))).toEqual(["live-assistant"]);
	});

	it("keeps a merged read_file row's key stable when older reads are prepended", () => {
		const prompt = durable(1, "user", "prompt");
		const loadedFirst = assignTimelineRows(
			[
				prompt,
				buildDisplayMessages([
					readFileMessage(50, "read-50"),
					readFileMessage(51, "read-51"),
				])[0],
			],
			false,
		);
		const afterPrepend = assignTimelineRows(
			[
				prompt,
				buildDisplayMessages([
					readFileMessage(48, "read-48"),
					readFileMessage(49, "read-49"),
					readFileMessage(50, "read-50"),
					readFileMessage(51, "read-51"),
				])[0],
			],
			false,
		);

		expect(keys(loadedFirst)).toEqual([
			"message:1",
			"read-file-group:through:51",
		]);
		expect(keys(afterPrepend)).toEqual(keys(loadedFirst));
	});

	it("keeps a singleton read_file row's key stable when a prepend extends the run", () => {
		const loadedFirst = assignTimelineRows(
			buildDisplayMessages([readFileMessage(50, "read-50")]),
			false,
		);
		const afterPrepend = assignTimelineRows(
			buildDisplayMessages([
				readFileMessage(49, "read-49"),
				readFileMessage(50, "read-50"),
			]),
			false,
		);

		expect(keys(loadedFirst)).toEqual(["read-file-group:through:50"]);
		expect(keys(afterPrepend)).toEqual(keys(loadedFirst));
	});

	it("marks only the last message of an assistant chain", () => {
		const rows = assignTimelineRows(
			[
				durable(1, "user", "prompt"),
				durable(2, "assistant", "first"),
				durable(3, "assistant", "second"),
				durable(4, "user", "follow up"),
			],
			false,
		);

		expect(
			rows.map((row) => row.type === "message" && row.isLastInAssistantChain),
		).toEqual([false, false, true, false]);
		expect(
			rows.map((row) => row.type === "message" && row.isLastMessage),
		).toEqual([false, false, false, true]);
	});
});

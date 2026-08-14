import { describe, expect, it } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import { assignTimelineRows } from "./timelineRows";
import type { ParsedMessageContent, ParsedMessageEntry } from "./types";

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

describe("assignTimelineRows", () => {
	it("keys durable rows by message ID and the live row separately", () => {
		const rows = assignTimelineRows(
			[durable(1, "user", "prompt"), durable(2, "assistant", "answer")],
			true,
		);

		expect(keys(rows)).toEqual(["message:1", "message:2", "live-assistant"]);
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

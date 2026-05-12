import { describe, expect, it } from "vitest";
import type { ChatMessage, ChatMessagePart } from "#/api/typesGenerated";
import { deriveMessageDisplayState } from "./messageHelpers";
import { parseMessageContent } from "./messageParsing";

const buildMessage = (
	content: ChatMessagePart[],
	role: "user" | "assistant" = "user",
): ChatMessage => ({
	id: 1,
	chat_id: "chat-1",
	created_at: "2026-05-11T00:00:00.000Z",
	role,
	content,
});

const getDisplayState = (message: ChatMessage) =>
	deriveMessageDisplayState({
		message,
		parsed: parseMessageContent(message.content),
		hideActions: false,
		hasActiveStream: false,
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
});

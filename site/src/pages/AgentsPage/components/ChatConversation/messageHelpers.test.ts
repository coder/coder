import { describe, expect, it } from "vitest";
import type { ChatMessage, ChatMessagePart } from "#/api/typesGenerated";
import { deriveMessageDisplayState } from "./messageHelpers";
import { parseMessagesWithMergedTools } from "./messageParsing";
import type { ParsedMessageContent } from "./types";

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

const getParsedMessage = (message: ChatMessage) =>
	parseMessagesWithMergedTools([message])[0].parsed;

const getDisplayState = (
	message: ChatMessage,
	overrides: Partial<Parameters<typeof deriveMessageDisplayState>[0]> = {},
) =>
	deriveMessageDisplayState({
		message,
		parsed: getParsedMessage(message),
		hideActions: false,
		hasActiveStream: false,
		...overrides,
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

	it("shows the assistant spacer for thinking-only messages when no suppressing flags apply", () => {
		const message = buildMessage(
			[{ type: "reasoning", text: "I should think before answering." }],
			"assistant",
		);

		expect(getDisplayState(message).needsAssistantBottomSpacer).toBe(true);
	});

	it("hides the assistant spacer when thinking is followed by a tool call", () => {
		const message = buildMessage(
			[
				{ type: "reasoning", text: "I should think before acting." },
				{
					type: "tool-call",
					tool_call_id: "tool-1",
					tool_name: "execute",
					args: { command: "pnpm storybook --no-open" },
				},
			],
			"assistant",
		);

		expect(getDisplayState(message).needsAssistantBottomSpacer).toBe(false);
	});

	it("shows the assistant spacer when thinking is followed by a hidden execute tool", () => {
		const message = buildMessage(
			[
				{ type: "reasoning", text: "I should think before acting." },
				{
					type: "tool-call",
					tool_call_id: "tool-1",
					tool_name: "execute",
					args: {},
				},
			],
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

		expect(getDisplayState(message).shouldHide).toBe(false);
	});

	it("hides running wait_agent messages until the chat id is available", () => {
		const message = buildMessage([], "assistant");
		const parsed: ParsedMessageContent = {
			...getParsedMessage(message),
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
				parsed,
				hideActions: false,
				hasActiveStream: false,
			}).shouldHide,
		).toBe(true);
	});
});

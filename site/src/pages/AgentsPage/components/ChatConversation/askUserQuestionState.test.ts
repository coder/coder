import { describe, expect, it } from "vitest";
import type { ChatMessage, ChatQueuedMessage } from "#/api/typesGenerated";
import {
	MockChatMessage,
	MockChatQueuedMessage,
} from "#/testHelpers/chatEntities";
import { deriveAskUserQuestionState } from "./askUserQuestionState";
import { parseMessagesWithMergedTools } from "./messageParsing";
import type { MergedTool } from "./types";

const mockQuestions = [
	{
		header: "Rollout",
		question: "How should this ship?",
		options: [
			{ label: "Single migration", description: "Ship it at once." },
			{ label: "Phased rollout", description: "Ship it in steps." },
		],
	},
];

let nextMessageId = 1;

const userMessage = (text: string): ChatMessage => ({
	...MockChatMessage,
	id: nextMessageId++,
	role: "user",
	content: [{ type: "text", text }],
});

const questionStep = (
	toolCallId: string,
	result: { async?: boolean; isError?: boolean } = {},
): ChatMessage[] => [
	{
		...MockChatMessage,
		id: nextMessageId++,
		role: "assistant",
		content: [
			{
				type: "tool-call",
				tool_call_id: toolCallId,
				tool_name: "ask_user_question",
				args: {},
			},
		],
	},
	{
		...MockChatMessage,
		id: nextMessageId++,
		role: "tool",
		content: [
			{
				type: "tool-result",
				tool_call_id: toolCallId,
				tool_name: "ask_user_question",
				is_error: result.isError,
				result: result.isError
					? "questions[0].options must contain 2-4 items"
					: {
							output: JSON.stringify({
								questions: mockQuestions,
								async: result.async,
							}),
						},
			},
		],
	},
];

const answerMessage = (toolCallId: string) =>
	`Answer to ask_user_question call ${toolCallId}:\nSingle migration`;

const queued = (text: string): ChatQueuedMessage => ({
	...MockChatQueuedMessage,
	content: [{ type: "text", text }],
});

const derive = (
	messages: ChatMessage[],
	{
		liveTools = [],
		queuedMessages = [],
	}: {
		liveTools?: MergedTool[];
		queuedMessages?: ChatQueuedMessage[];
	} = {},
) =>
	deriveAskUserQuestionState(
		parseMessagesWithMergedTools(messages),
		liveTools,
		queuedMessages,
	);

describe("deriveAskUserQuestionState", () => {
	it("keeps an async question open after an unrelated user message", () => {
		const state = derive([
			...questionStep("async-call", { async: true }),
			userMessage("Also update the docs."),
		]);

		expect(state.latestToolId).toBe("async-call");
		expect(state.isLatestAnswered).toBe(false);
		expect(state.responseTextByToolId.has("async-call")).toBe(false);
	});

	it("answers an async question with the message that names its call", () => {
		const state = derive([
			...questionStep("async-call", { async: true }),
			userMessage("Also update the docs."),
			userMessage(answerMessage("async-call")),
		]);

		expect(state.isLatestAnswered).toBe(true);
		expect(state.responseTextByToolId.get("async-call")).toBe(
			answerMessage("async-call"),
		);
	});

	it("answers a blocking question with the next user message", () => {
		const state = derive([
			...questionStep("blocking-call"),
			userMessage("Phased rollout, please."),
		]);

		expect(state.latestToolId).toBe("blocking-call");
		expect(state.isLatestAnswered).toBe(true);
		expect(state.responseTextByToolId.get("blocking-call")).toBe(
			"Phased rollout, please.",
		);
	});

	it("keeps the question open when a later call fails validation", () => {
		const state = derive([
			...questionStep("async-call", { async: true }),
			...questionStep("invalid-call", { isError: true }),
		]);

		expect(state.latestToolId).toBe("async-call");
		expect(state.isLatestAnswered).toBe(false);
	});

	it("records a queued answer without marking it delivered", () => {
		const state = derive(questionStep("async-call", { async: true }), {
			queuedMessages: [
				queued("Unrelated follow-up"),
				queued(answerMessage("async-call")),
			],
		});

		expect(state.isLatestAnswered).toBe(false);
		expect(state.responseTextByToolId.get("async-call")).toBe(
			answerMessage("async-call"),
		);
	});

	it("lets only a valid finished live question supersede the persisted one", () => {
		const liveQuestion = (
			status: MergedTool["status"],
			isError = false,
		): MergedTool => ({
			id: "live-call",
			name: "ask_user_question",
			status,
			isError,
		});
		const messages = questionStep("async-call", { async: true });

		expect(
			derive(messages, { liveTools: [liveQuestion("running")] }).latestToolId,
		).toBe("async-call");
		expect(
			derive(messages, { liveTools: [liveQuestion("error", true)] })
				.latestToolId,
		).toBe("async-call");
		expect(
			derive(messages, { liveTools: [liveQuestion("completed")] }).latestToolId,
		).toBe("live-call");
	});
});

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

const questionCall = (toolCallId: string, startedAt?: string): ChatMessage => ({
	...MockChatMessage,
	id: nextMessageId++,
	role: "assistant",
	content: [
		{
			type: "tool-call",
			tool_call_id: toolCallId,
			tool_name: "ask_user_question",
			args: {},
			created_at: startedAt,
		},
	],
});

const questionStep = (
	toolCallId: string,
	options: { async?: boolean; isError?: boolean; startedAt?: string } = {},
): ChatMessage[] => [
	questionCall(toolCallId, options.startedAt),
	{
		...MockChatMessage,
		id: nextMessageId++,
		role: "tool",
		content: [
			{
				type: "tool-result",
				tool_call_id: toolCallId,
				tool_name: "ask_user_question",
				is_error: options.isError,
				result: options.isError
					? "questions[0].options must contain 2-4 items"
					: {
							output: JSON.stringify({
								questions: mockQuestions,
								async: options.async,
							}),
						},
			},
		],
	},
];

const answerMessage = (toolCallId: string) =>
	`Answer to ask_user_question call ${toolCallId}:\nSingle migration`;

const queued = (text: string, createdAt?: string): ChatQueuedMessage => ({
	...MockChatQueuedMessage,
	content: [{ type: "text", text }],
	created_at: createdAt ?? MockChatQueuedMessage.created_at,
});

const derive = (
	messages: ChatMessage[],
	{
		liveTools = [],
		queuedMessages = [],
		pendingToolCallIDs,
	}: {
		liveTools?: MergedTool[];
		queuedMessages?: ChatQueuedMessage[];
		pendingToolCallIDs?: ReadonlySet<string>;
	} = {},
) =>
	deriveAskUserQuestionState(
		parseMessagesWithMergedTools(messages, { pendingToolCallIDs }),
		liveTools,
		queuedMessages,
	);

const reusedCallStartedAt = "2026-01-01T00:02:00Z";

const reusingCalls = [
	{
		source: "a persisted call",
		deriveAfter: (
			messages: ChatMessage[],
			queuedMessages: ChatQueuedMessage[] = [],
		) =>
			derive(
				[
					...messages,
					...questionStep("reused-call", {
						async: true,
						startedAt: reusedCallStartedAt,
					}),
				],
				{ queuedMessages },
			),
	},
	{
		source: "a live call",
		deriveAfter: (
			messages: ChatMessage[],
			queuedMessages: ChatQueuedMessage[] = [],
		) =>
			derive(messages, {
				queuedMessages,
				liveTools: [
					{
						id: "reused-call",
						name: "ask_user_question",
						status: "completed",
						isError: false,
						startedAt: reusedCallStartedAt,
					},
				],
			}),
	},
];

describe("deriveAskUserQuestionState", () => {
	it("keeps an async question open after an unrelated user message", () => {
		const state = derive([
			...questionStep("async-call", { async: true }),
			userMessage("Also update the docs."),
		]);

		expect(state.latestTool?.id).toBe("async-call");
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

	it("answers an async question when a chip follows the prefix line", () => {
		const content: ChatMessage["content"] = [
			{ type: "text", text: "Answer to ask_user_question call async-call:" },
			{
				type: "skill-invocation",
				skill_source: "project",
				skill_name: "triage",
			},
			{ type: "text", text: "Single migration" },
		];
		const messages = questionStep("async-call", { async: true });

		const delivered = derive([
			...messages,
			{ ...MockChatMessage, id: nextMessageId++, role: "user", content },
		]);
		const queuedAnswer = derive(messages, {
			queuedMessages: [{ ...MockChatQueuedMessage, content }],
		});

		expect(delivered.isLatestAnswered).toBe(true);
		expect(delivered.responseTextByToolId.get("async-call")).toBe(
			answerMessage("async-call"),
		);
		expect(queuedAnswer.responseTextByToolId.get("async-call")).toBe(
			answerMessage("async-call"),
		);
	});

	it("answers a blocking question with the next user message", () => {
		const state = derive([
			...questionStep("blocking-call"),
			userMessage("Phased rollout, please."),
		]);

		expect(state.latestTool?.id).toBe("blocking-call");
		expect(state.isLatestAnswered).toBe(true);
		expect(state.responseTextByToolId.get("blocking-call")).toBe(
			"Phased rollout, please.",
		);
	});

	// A plan mode question ends the turn, and the turn end promotes a queued
	// answer to an earlier async call.
	it("keeps a blocking question open after an answer to an earlier call", () => {
		const state = derive([
			...questionStep("async-call", { async: true }),
			...questionStep("blocking-call"),
			userMessage(answerMessage("async-call")),
		]);

		expect(state.latestTool?.id).toBe("blocking-call");
		expect(state.isLatestAnswered).toBe(false);
		expect(state.responseTextByToolId.has("blocking-call")).toBe(false);
		expect(state.responseTextByToolId.get("async-call")).toBe(
			answerMessage("async-call"),
		);
	});

	it.each([
		{
			outcome: "fails validation",
			laterCall: questionStep("later-call", { isError: true }),
			pendingToolCallIDs: undefined,
		},
		{
			outcome: "is still running",
			laterCall: [questionCall("later-call")],
			pendingToolCallIDs: new Set(["later-call"]),
		},
	])(
		"keeps the question open when a later call $outcome",
		({ laterCall, pendingToolCallIDs }) => {
			const state = derive(
				[...questionStep("async-call", { async: true }), ...laterCall],
				{ pendingToolCallIDs },
			);

			expect(state.latestTool?.id).toBe("async-call");
			expect(state.isLatestAnswered).toBe(false);
		},
	);

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

	it.each(reusingCalls)(
		"ignores an earlier answer when $source reuses its call ID",
		({ deriveAfter }) => {
			const state = deriveAfter([
				...questionStep("reused-call", { async: true }),
				userMessage(answerMessage("reused-call")),
			]);

			expect(state.latestTool?.startedAt).toBe(reusedCallStartedAt);
			expect(state.isLatestAnswered).toBe(false);
			expect(state.responseTextByToolId.has("reused-call")).toBe(false);
		},
	);

	it.each(reusingCalls)(
		"counts a queued answer only if it is newer than $source that reuses its call ID",
		({ deriveAfter }) => {
			const isAnsweredByAnswerQueuedAt = (createdAt: string) =>
				deriveAfter(questionStep("reused-call", { async: true }), [
					queued(answerMessage("reused-call"), createdAt),
				]).responseTextByToolId.has("reused-call");

			expect(isAnsweredByAnswerQueuedAt("2026-01-01T00:01:00Z")).toBe(false);
			expect(isAnsweredByAnswerQueuedAt("2026-01-01T00:03:00Z")).toBe(true);
		},
	);

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
			derive(messages, { liveTools: [liveQuestion("running")] }).latestTool?.id,
		).toBe("async-call");
		expect(
			derive(messages, { liveTools: [liveQuestion("error", true)] }).latestTool
				?.id,
		).toBe("async-call");
		expect(
			derive(messages, { liveTools: [liveQuestion("completed")] }).latestTool
				?.id,
		).toBe("live-call");
	});
});

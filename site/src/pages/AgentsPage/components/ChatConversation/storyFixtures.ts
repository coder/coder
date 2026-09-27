import type * as TypesGen from "#/api/typesGenerated";
import { MockChatMessage } from "#/testHelpers/chatEntities";
import { MockUserPreferenceSettings } from "#/testHelpers/entities";
import {
	type DeriveLiveStatusParams,
	deriveLiveStatus,
	type LiveStatusModel,
} from "./liveStatusModel";
import { applyMessagePartToStreamState, buildStreamTools } from "./streamState";
import type {
	MergedTool,
	ReconnectState,
	RetryState,
	StreamState,
} from "./types";
import type { WorkingBlock } from "./workingBlockGrouping";

export type StoryStreamRenderState = {
	streamState: StreamState | null;
	streamTools: readonly MergedTool[];
	liveStatus: LiveStatusModel;
};

export const MockCollapsedStepsPreferences: TypesGen.UserPreferenceSettings = {
	...MockUserPreferenceSettings,
	shell_tool_display_mode: "always_collapsed",
	collapse_assistant_steps: true,
};

/**
 * Generate a long conversation so the scroll container overflows in
 * transcript-scrolling stories.
 */
export const buildLongConversation = (
	chatId: string,
	count: number,
): TypesGen.ChatMessage[] => {
	const messages: TypesGen.ChatMessage[] = [];
	for (let i = 1; i <= count; i++) {
		const role: TypesGen.ChatMessageRole = i % 2 === 1 ? "user" : "assistant";
		const turn = Math.ceil(i / 2);
		messages.push({
			id: i,
			chat_id: chatId,
			created_at: new Date(Date.now() - (count - i) * 60_000).toISOString(),
			role,
			content: [
				{
					type: "text",
					text:
						role === "user"
							? `Question ${turn}: Can you explain concept ${turn} in detail?`
							: `Sure! Here is a detailed explanation of concept ${turn}. `.repeat(
									4,
								),
				},
			],
		});
	}
	return messages;
};

const DEFAULT_LIVE_STATUS_PARAMS: DeriveLiveStatusParams = {
	streamState: null,
	retryState: null,
	reconnectState: null,
	streamError: null,
	persistedError: null,
	isAwaitingFirstStreamChunk: false,
	chatStatus: null,
};

export const buildLiveStatus = (
	overrides: Partial<DeriveLiveStatusParams> = {},
): LiveStatusModel =>
	deriveLiveStatus({
		...DEFAULT_LIVE_STATUS_PARAMS,
		...overrides,
	});

export const buildStreamRenderState = (
	parts: readonly TypesGen.ChatMessagePart[],
): StoryStreamRenderState => {
	let streamState: StreamState | null = null;
	for (const part of parts) {
		streamState = applyMessagePartToStreamState(streamState, part);
	}

	return {
		streamState,
		streamTools: buildStreamTools(
			streamState?.toolCalls,
			streamState?.toolResults,
		),
		liveStatus: buildLiveStatus({ streamState }),
	};
};

/**
 * Pinned clock for stories that render countdown timers. Stories
 * should mock `Date.now` to return this value so the countdowns
 * are deterministic across snapshot tests.
 *
 * Set to midnight UTC on the same day as the fixture deadlines,
 * giving reconnect a 1s countdown and retry a 2s countdown.
 */
export const FIXTURE_NOW = new Date("2026-03-10T00:00:00.000Z").getTime();

/**
 * Start of the working-block fixtures. Their tool work begins one second in,
 * so under the pinned clock a live block reads "Working for 12s".
 */
export const WORKING_FIXTURE_START = FIXTURE_NOW - 13_000;
export const workingFixtureTime = (seconds: number) =>
	new Date(WORKING_FIXTURE_START + seconds * 1000).toISOString();

/** Two completed steps that ran for the twelve seconds before the pinned clock. */
export const MockWorkingBlock: WorkingBlock = {
	key: "working:through:message:5",
	liveKey: "working:live:message:1:0",
	rowIndices: [0, 1],
	memberIds: [2, 4],
	startedAt: FIXTURE_NOW - 12_000,
	endedAt: FIXTURE_NOW,
	stepCount: 2,
	failedCount: 0,
	isLive: false,
	isPartial: false,
};

const workingStepMessages = (
	id: number,
	toolCallId: string,
	callAt: number,
	resultAt: number,
): TypesGen.ChatMessage[] => [
	{
		...MockChatMessage,
		id,
		role: "assistant",
		created_at: workingFixtureTime(callAt),
		content: [
			{
				type: "tool-call",
				tool_call_id: toolCallId,
				tool_name: "execute",
				args: { command: `echo ${toolCallId}` },
				created_at: workingFixtureTime(callAt),
			},
		],
	},
	{
		...MockChatMessage,
		id: id + 1,
		role: "tool",
		created_at: workingFixtureTime(resultAt),
		content: [
			{
				type: "tool-result",
				tool_call_id: toolCallId,
				tool_name: "execute",
				result: { output: `${toolCallId} output`, exit_code: "0" },
				created_at: workingFixtureTime(resultAt),
			},
		],
	},
];

/**
 * One completed turn of two shell steps with part timestamps: prompt at 0s,
 * tool work from 1s to 13s, answer at 14s.
 */
export const MockWorkingMessages: TypesGen.ChatMessage[] = [
	{
		...MockChatMessage,
		id: 1,
		created_at: workingFixtureTime(0),
		content: [{ type: "text", text: "Inspect the workspace" }],
	},
	...workingStepMessages(2, "first", 1, 4),
	...workingStepMessages(4, "second", 5, 13),
	{
		...MockChatMessage,
		id: 6,
		role: "assistant",
		created_at: workingFixtureTime(14),
		content: [{ type: "text", text: "Workspace inspection complete." }],
	},
];

/** A question the agent asks after the first step of MockWorkingMessages. */
export const MockQuestionCallMessage: TypesGen.ChatMessage = {
	...MockChatMessage,
	id: 4,
	role: "assistant",
	created_at: workingFixtureTime(5),
	content: [
		{
			type: "tool-call",
			tool_call_id: "question",
			tool_name: "ask_user_question",
			args: {},
			created_at: workingFixtureTime(5),
		},
	],
};

export const buildReconnectState = (
	overrides: Partial<ReconnectState> = {},
): ReconnectState => ({
	attempt: 1,
	delayMs: 1000,
	retryingAt: "2026-03-10T00:00:01.000Z",
	...overrides,
});

const MockLongTurn = Array.from({ length: 60 }, (_, index) =>
	workingStepMessages(100 + index * 2, `step-${index}`, index, index),
).flat();
const MockLongTurnPrompt: TypesGen.ChatMessage = {
	...MockChatMessage,
	id: 99,
	created_at: workingFixtureTime(-1),
	content: [{ type: "text", text: "Run every step" }],
};
/**
 * The loaded transcript of a 60-step turn after each of its three history
 * pages, newest first, so the prompt row only arrives with the final page.
 */
export const MockLongTurnPageLoads = [
	MockLongTurn.slice(60),
	MockLongTurn.slice(30),
	[MockLongTurnPrompt, ...MockLongTurn],
];

export const buildRetryState = (
	overrides: Partial<RetryState> = {},
): RetryState => ({
	attempt: 1,
	error: "Anthropic returned an unexpected error.",
	kind: "generic",
	provider: "anthropic",
	retryingAt: "2026-03-10T00:00:02.000Z",
	...overrides,
});

export const pinFixtureClock = () => {
	const real = Date.now;
	Date.now = () => FIXTURE_NOW;
	return () => {
		Date.now = real;
	};
};

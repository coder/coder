import type * as TypesGen from "#/api/typesGenerated";
import { MockChatMessage } from "#/testHelpers/chatEntities";
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

export type StoryStreamRenderState = {
	streamState: StreamState | null;
	streamTools: readonly MergedTool[];
	liveStatus: LiveStatusModel;
};

export const WORKING_FIXTURE_START = Date.parse("2026-04-01T12:00:00Z");
export const workingFixtureTime = (seconds: number) =>
	new Date(WORKING_FIXTURE_START + seconds * 1000).toISOString();

/**
 * One completed turn of two shell steps with part timestamps: prompt at 0s,
 * tool work from 1s to 13s, answer at 14s.
 */
export const buildWorkingConversation = (
	chatId = MockChatMessage.chat_id,
): TypesGen.ChatMessage[] => {
	const time = workingFixtureTime;
	return [
		{
			...MockChatMessage,
			chat_id: chatId,
			id: 1,
			created_at: time(0),
			content: [{ type: "text", text: "Inspect the workspace" }],
		},
		{
			...MockChatMessage,
			chat_id: chatId,
			id: 2,
			role: "assistant",
			created_at: time(1),
			content: [
				{
					type: "tool-call",
					tool_call_id: "first",
					tool_name: "execute",
					args: { command: "echo first" },
					created_at: time(1),
				},
			],
		},
		{
			...MockChatMessage,
			chat_id: chatId,
			id: 3,
			role: "tool",
			created_at: time(4),
			content: [
				{
					type: "tool-result",
					tool_call_id: "first",
					tool_name: "execute",
					result: { output: "First output", exit_code: "0" },
					created_at: time(4),
				},
			],
		},
		{
			...MockChatMessage,
			chat_id: chatId,
			id: 4,
			role: "assistant",
			created_at: time(5),
			content: [
				{
					type: "tool-call",
					tool_call_id: "second",
					tool_name: "execute",
					args: { command: "echo second" },
					created_at: time(5),
				},
			],
		},
		{
			...MockChatMessage,
			chat_id: chatId,
			id: 5,
			role: "tool",
			created_at: time(13),
			content: [
				{
					type: "tool-result",
					tool_call_id: "second",
					tool_name: "execute",
					result: { output: "Second output", exit_code: "0" },
					created_at: time(13),
				},
			],
		},
		{
			...MockChatMessage,
			chat_id: chatId,
			id: 6,
			role: "assistant",
			created_at: time(14),
			content: [{ type: "text", text: "Workspace inspection complete." }],
		},
	];
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

export const buildReconnectState = (
	overrides: Partial<ReconnectState> = {},
): ReconnectState => ({
	attempt: 1,
	delayMs: 1000,
	retryingAt: "2026-03-10T00:00:01.000Z",
	...overrides,
});

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

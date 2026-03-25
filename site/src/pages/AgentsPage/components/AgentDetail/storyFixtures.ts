import type * as TypesGen from "#/api/typesGenerated";
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

type StoryStreamRenderState = {
	streamState: StreamState | null;
	streamTools: readonly MergedTool[];
	liveStatus: LiveStatusModel;
};

const DEFAULT_LIVE_STATUS_PARAMS: DeriveLiveStatusParams = {
	streamState: null,
	retryState: null,
	reconnectState: null,
	streamError: null,
	persistedError: null,
	isAwaitingFirstStreamChunk: false,
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
		streamTools: buildStreamTools(streamState),
		liveStatus: buildLiveStatus({ streamState }),
	};
};

/**
 * Pinned clock for stories that render countdown timers. Stories
 * should mock `Date.now` to return this value so the countdowns
 * are deterministic across Chromatic snapshots.
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
	error:
		"Anthropic is retrying your request after a transient upstream failure.",
	kind: "generic",
	provider: "anthropic",
	delayMs: 2000,
	retryingAt: "2026-03-10T00:00:02.000Z",
	...overrides,
});

export const textResponseStreamParts = [
	{
		type: "text",
		text: "Storybook streamed answer.",
	},
] satisfies readonly TypesGen.ChatMessagePart[];

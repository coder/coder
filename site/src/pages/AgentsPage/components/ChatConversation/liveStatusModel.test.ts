import { describe, expect, it } from "vitest";
import type { ChatDetailError } from "../../utils/usageLimitMessage";
import { deriveLiveStatus } from "./liveStatusModel";
import { buildReconnectState, buildRetryState } from "./storyFixtures";
import type { StreamState } from "./types";

const buildStreamState = (
	overrides: Partial<StreamState> = {},
): StreamState => ({
	blocks: [],
	toolCalls: {},
	toolResults: {},
	sources: [],
	...overrides,
});

const buildStreamError = (
	overrides: Partial<ChatDetailError> = {},
): ChatDetailError => ({
	kind: "generic",
	message: "Chat processing failed.",
	provider: "anthropic",
	retryable: false,
	statusCode: 500,
	...overrides,
});

const derive = (
	overrides: Partial<Parameters<typeof deriveLiveStatus>[0]> = {},
) =>
	deriveLiveStatus({
		streamState: null,
		retryState: null,
		reconnectState: null,
		streamError: null,
		persistedError: null,
		isAwaitingFirstStreamChunk: false,
		...overrides,
	});

describe("deriveLiveStatus", () => {
	const retryingStatus = {
		phase: "retrying",
		hasAccumulatedOutput: false,
		title: "Retrying request",
		kind: "generic",
		message: "Anthropic returned an unexpected error.",
		attempt: 2,
		provider: "anthropic",
		delayMs: 2000,
		retryingAt: "2026-03-10T00:00:02.000Z",
	};
	const reconnectingStatus = {
		phase: "reconnecting",
		hasAccumulatedOutput: false,
		title: "Reconnecting",
		message: "Chat stream disconnected. Reconnecting…",
		attempt: 1,
		delayMs: 1000,
		retryingAt: "2026-03-10T00:00:01.000Z",
	};
	const failedStatus = {
		phase: "failed",
		hasAccumulatedOutput: false,
		title: "Request failed",
		kind: "generic",
		message: "Chat processing failed.",
		provider: "anthropic",
		statusCode: 500,
	};

	it.each([
		["idle", undefined, { phase: "idle", hasAccumulatedOutput: false }],
		[
			"starting",
			{ isAwaitingFirstStreamChunk: true },
			{ phase: "starting", hasAccumulatedOutput: false },
		],
		[
			"retrying",
			{ retryState: buildRetryState({ attempt: 2 }) },
			retryingStatus,
		],
		[
			"reconnecting",
			{ reconnectState: buildReconnectState() },
			reconnectingStatus,
		],
		["failed", { streamError: buildStreamError() }, failedStatus],
		[
			"streaming",
			{ streamState: buildStreamState() },
			{ phase: "streaming", hasAccumulatedOutput: false },
		],
	])("returns %s", (_phase, overrides, expected) => {
		expect(derive(overrides)).toEqual(expected);
	});

	it("uses the persisted error as the idle fallback", () => {
		expect(derive({ persistedError: buildStreamError() })).toEqual(
			failedStatus,
		);
	});

	it("keeps live stream state ahead of the persisted error fallback", () => {
		expect(
			derive({
				streamState: buildStreamState(),
				persistedError: buildStreamError({ kind: "timeout" }),
			}),
		).toEqual({ phase: "streaming", hasAccumulatedOutput: false });
	});

	it("tracks accumulated output on failed streams", () => {
		expect(
			derive({
				streamState: buildStreamState({
					blocks: [{ type: "response", text: "Partial response" }],
				}),
				streamError: buildStreamError(),
			}),
		).toEqual({
			...failedStatus,
			hasAccumulatedOutput: true,
		});
	});

	it("passes provider detail through failed status", () => {
		expect(
			derive({
				streamError: buildStreamError({
					detail: "Image exceeds 5 MB maximum.",
				}),
			}),
		).toMatchObject({
			phase: "failed",
			detail: "Image exceeds 5 MB maximum.",
		});
	});

	it("tracks accumulated output while reconnecting", () => {
		expect(
			derive({
				streamState: buildStreamState({
					blocks: [{ type: "response", text: "Partial response" }],
				}),
				reconnectState: buildReconnectState(),
			}),
		).toEqual({
			...reconnectingStatus,
			hasAccumulatedOutput: true,
		});
	});

	it("prioritizes retrying over failed and reconnecting", () => {
		expect(
			derive({
				retryState: buildRetryState({ attempt: 2, kind: "rate_limit" }),
				reconnectState: buildReconnectState({ attempt: 3 }),
				streamError: buildStreamError({ kind: "timeout" }),
				persistedError: buildStreamError({ kind: "generic" }),
				isAwaitingFirstStreamChunk: true,
			}),
		).toMatchObject({ phase: "retrying", kind: "rate_limit" });
	});

	it("prioritizes failed over reconnecting and starting", () => {
		expect(
			derive({
				reconnectState: buildReconnectState(),
				streamError: buildStreamError({ kind: "timeout" }),
				persistedError: buildStreamError({ kind: "generic" }),
				isAwaitingFirstStreamChunk: true,
			}),
		).toMatchObject({ phase: "failed", kind: "timeout" });
	});

	it("prioritizes reconnecting over starting", () => {
		expect(
			derive({
				reconnectState: buildReconnectState(),
				isAwaitingFirstStreamChunk: true,
			}),
		).toEqual(reconnectingStatus);
	});

	it("prioritizes starting over streaming", () => {
		expect(
			derive({
				streamState: buildStreamState(),
				isAwaitingFirstStreamChunk: true,
			}),
		).toEqual({ phase: "starting", hasAccumulatedOutput: false });
	});
});

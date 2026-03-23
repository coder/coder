import { describe, expect, it } from "vitest";
import type { ChatDetailError } from "../../utils/usageLimitMessage";
import { deriveLiveStatus } from "./liveStatusModel";
import type { RetryState, StreamState } from "./types";

const makeStreamState = (): StreamState => ({
	blocks: [],
	toolCalls: {},
	toolResults: {},
	sources: [],
});

const makeRetryState = (overrides: Partial<RetryState> = {}): RetryState => ({
	attempt: 2,
	error: "Retrying request shortly.",
	kind: "generic",
	provider: "anthropic",
	delayMs: 2000,
	retryingAt: "2026-03-10T00:00:02.000Z",
	...overrides,
});

const makeStreamError = (
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
		streamError: null,
		isAwaitingFirstStreamChunk: false,
		...overrides,
	});

describe("deriveLiveStatus", () => {
	const retryingStatus = {
		phase: "retrying",
		title: "Retrying request",
		kind: "generic",
		message: "Retrying request shortly.",
		attempt: 2,
		provider: "anthropic",
		delayMs: 2000,
		retryingAt: "2026-03-10T00:00:02.000Z",
	};
	const failedStatus = {
		phase: "failed",
		title: "Request failed",
		kind: "generic",
		message: "Chat processing failed.",
		provider: "anthropic",
		retryable: false,
		statusCode: 500,
	};

	it.each([
		["idle", undefined, { phase: "idle" }],
		["starting", { isAwaitingFirstStreamChunk: true }, { phase: "starting" }],
		["retrying", { retryState: makeRetryState() }, retryingStatus],
		["failed", { streamError: makeStreamError() }, failedStatus],
		["streaming", { streamState: makeStreamState() }, { phase: "streaming" }],
	])("returns %s", (_phase, overrides, expected) => {
		expect(derive(overrides)).toEqual(expected);
	});

	it("prioritizes retrying over failed", () => {
		expect(
			derive({
				retryState: makeRetryState({ kind: "rate_limit" }),
				streamError: makeStreamError({ kind: "timeout" }),
				isAwaitingFirstStreamChunk: true,
			}),
		).toMatchObject({ phase: "retrying", kind: "rate_limit" });
	});

	it("prioritizes failed over starting", () => {
		expect(
			derive({
				streamError: makeStreamError({ kind: "timeout" }),
				isAwaitingFirstStreamChunk: true,
			}),
		).toMatchObject({ phase: "failed", kind: "timeout" });
	});

	it("prioritizes starting over streaming", () => {
		expect(
			derive({
				streamState: makeStreamState(),
				isAwaitingFirstStreamChunk: true,
			}),
		).toEqual({ phase: "starting" });
	});
});

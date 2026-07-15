import { describe, expect, it, vi } from "vitest";
import {
	createChatStreamStore,
	isAwaitingFirstStreamChunk,
} from "./chatStreamStore";

describe("createChatStreamStore", () => {
	it("keeps transient presentation state without durable execution fields", () => {
		const store = createChatStreamStore();
		store.setTransientError({ kind: "generic", message: "request failed" });
		store.setRetryState({
			attempt: 2,
			error: "retrying",
			kind: "generic",
		});

		expect(store.getSnapshot()).toMatchObject({
			transientError: { kind: "generic", message: "request failed" },
			retryState: { attempt: 2, error: "retrying" },
		});
		expect(store.getSnapshot()).not.toHaveProperty("chatStatus");
		expect(store.getSnapshot()).not.toHaveProperty("actionRequiredState");
		expect(store.getSnapshot()).not.toHaveProperty("streamError");
	});

	it("batches subscriber notifications", () => {
		const store = createChatStreamStore();
		const listener = vi.fn();
		store.subscribe(listener);
		store.batch(() => {
			store.setTransientError({ kind: "generic", message: "failed" });
			store.setRetryState({ attempt: 1, error: "retry", kind: "generic" });
		});
		expect(listener).toHaveBeenCalledTimes(1);
	});

	it("resets transport-scoped state", () => {
		const store = createChatStreamStore();
		store.setTransientError({ kind: "generic", message: "failed" });
		store.setRetryState({ attempt: 1, error: "retry", kind: "generic" });
		store.setReconnectState({ attempt: 1, delayMs: 100, retryingAt: "now" });
		store.resetTransportReplayState();
		expect(store.getSnapshot()).toMatchObject({
			transientError: null,
			retryState: null,
			reconnectState: null,
			streamState: null,
		});
	});
});

describe("isAwaitingFirstStreamChunk", () => {
	it("uses canonical status plus transient preview state", () => {
		expect(isAwaitingFirstStreamChunk("running", null, undefined)).toBe(true);
		expect(isAwaitingFirstStreamChunk("waiting", null, undefined)).toBe(false);
		expect(
			isAwaitingFirstStreamChunk("running", null, {
				id: 1,
				chat_id: "chat",
				created_at: "2025-01-01T00:00:00Z",
				role: "assistant",
				content: [],
			}),
		).toBe(false);
	});
});

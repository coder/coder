import { describe, expect, it } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import {
	createChatStore,
	selectHasStreamOverlay,
	selectHasStreamState,
} from "./chatStore";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const makeQueuedMessage = (
	id: number,
	text: string,
	chatID = "chat-1",
): TypesGen.ChatQueuedMessage =>
	({
		id,
		chat_id: chatID,
		created_at: "2025-01-01T00:00:00Z",
		content: [{ type: "text", text }],
	}) as TypesGen.ChatQueuedMessage;

// ---------------------------------------------------------------------------
// setStreamState
// ---------------------------------------------------------------------------

describe("setStreamState", () => {
	it("does not notify when setting the same stream state reference", () => {
		const store = createChatStore();
		store.applyMessagePart({ type: "text", text: "hello" });
		const streamState = store.getSnapshot().streamState;
		expect(streamState).not.toBeNull();

		let notified = false;
		store.subscribe(() => {
			notified = true;
		});

		store.setStreamState(streamState);
		expect(notified).toBe(false);
	});
});

// ---------------------------------------------------------------------------
// setStreamError / clearStreamError
// ---------------------------------------------------------------------------

describe("setStreamError / clearStreamError", () => {
	it("stores and clears a stream error", () => {
		const store = createChatStore();

		store.setStreamError({
			kind: "generic",
			message: "connection lost",
		});
		expect(store.getSnapshot().streamError).toEqual({
			kind: "generic",
			message: "connection lost",
		});

		store.clearStreamError();
		expect(store.getSnapshot().streamError).toBeNull();
	});

	it("does not notify when setting the same error", () => {
		const store = createChatStore();
		store.setStreamError({
			kind: "generic",
			message: "oops",
			detail: "Image exceeds 5 MB maximum.",
		});

		let notified = false;
		store.subscribe(() => {
			notified = true;
		});
		store.setStreamError({
			kind: "generic",
			message: "oops",
			detail: "Image exceeds 5 MB maximum.",
		});

		expect(notified).toBe(false);
	});

	it("clearStreamError is a no-op when already null", () => {
		const store = createChatStore();

		let notified = false;
		store.subscribe(() => {
			notified = true;
		});
		store.clearStreamError();

		expect(notified).toBe(false);
	});
});

// ---------------------------------------------------------------------------
// setRetryState / clearRetryState
// ---------------------------------------------------------------------------

describe("setRetryState / clearRetryState", () => {
	it("stores and clears retry state", () => {
		const store = createChatStore();

		store.setRetryState({
			attempt: 1,
			error: "rate limited",
			kind: "rate_limit",
			provider: "anthropic",
			retryingAt: "2025-01-01T00:00:30.000Z",
		});
		expect(store.getSnapshot().retryState).toEqual({
			attempt: 1,
			error: "rate limited",
			kind: "rate_limit",
			provider: "anthropic",
			retryingAt: "2025-01-01T00:00:30.000Z",
		});

		store.clearRetryState();
		expect(store.getSnapshot().retryState).toBeNull();
	});

	it("clearRetryState is a no-op when already null", () => {
		const store = createChatStore();

		let notified = false;
		store.subscribe(() => {
			notified = true;
		});
		store.clearRetryState();

		expect(notified).toBe(false);
	});
});

// ---------------------------------------------------------------------------
// setReconnectState / clearReconnectState
// ---------------------------------------------------------------------------

describe("setReconnectState / clearReconnectState", () => {
	it("stores and clears reconnect state", () => {
		const store = createChatStore();

		store.setReconnectState({
			attempt: 2,
			delayMs: 3000,
			retryingAt: "2025-01-01T00:00:30.000Z",
		});
		expect(store.getSnapshot().reconnectState).toEqual({
			attempt: 2,
			delayMs: 3000,
			retryingAt: "2025-01-01T00:00:30.000Z",
		});

		store.clearReconnectState();
		expect(store.getSnapshot().reconnectState).toBeNull();
	});

	it("clearReconnectState is a no-op when already null", () => {
		const store = createChatStore();

		let notified = false;
		store.subscribe(() => {
			notified = true;
		});
		store.clearReconnectState();

		expect(notified).toBe(false);
	});
});

// ---------------------------------------------------------------------------
// setSubagentStatusOverride
// ---------------------------------------------------------------------------

describe("setSubagentStatusOverride", () => {
	it("stores per-chatID status overrides", () => {
		const store = createChatStore();

		store.setSubagentStatusOverride("sub-1", "running");
		store.setSubagentStatusOverride("sub-2", "error");

		const overrides = store.getSnapshot().subagentStatusOverrides;
		expect(overrides.get("sub-1")).toBe("running");
		expect(overrides.get("sub-2")).toBe("error");
	});

	it("does not notify when the override is unchanged", () => {
		const store = createChatStore();
		store.setSubagentStatusOverride("sub-1", "running");

		let notified = false;
		store.subscribe(() => {
			notified = true;
		});
		store.setSubagentStatusOverride("sub-1", "running");

		expect(notified).toBe(false);
	});

	it("overwrites an existing override for the same chatID", () => {
		const store = createChatStore();
		store.setSubagentStatusOverride("sub-1", "running");
		store.setSubagentStatusOverride("sub-1", "waiting");

		expect(store.getSnapshot().subagentStatusOverrides.get("sub-1")).toBe(
			"waiting",
		);
	});
});

// ---------------------------------------------------------------------------
// acceptAuthoritativeQueueSnapshot: the accept/reject gate
// ---------------------------------------------------------------------------

describe("acceptAuthoritativeQueueSnapshot", () => {
	it("accepts an ordinary snapshot and keeps unrelated markers", () => {
		const store = createChatStore();
		const a = makeQueuedMessage(1, "A");
		const b = makeQueuedMessage(2, "B");
		const c = makeQueuedMessage(3, "C");

		store.suppressQueuedMessageIDs([b.id]);
		// The running-case promotion only reorders the queue, so the backend
		// still reports the row and the marker has to keep hiding it.
		expect(store.acceptAuthoritativeQueueSnapshot([b, a, c], "socket")).toBe(
			true,
		);
		expect(store.getSnapshot().suppressedQueuedMessageIDs.has(b.id)).toBe(true);
	});

	it("clears a suppression marker the accepted snapshot omits", () => {
		const store = createChatStore();
		const a = makeQueuedMessage(1, "A");
		const b = makeQueuedMessage(2, "B");
		const c = makeQueuedMessage(3, "C");

		store.suppressQueuedMessageIDs([b.id]);
		store.acceptAuthoritativeQueueSnapshot([b, a, c], "socket");

		expect(store.acceptAuthoritativeQueueSnapshot([a, c], "socket")).toBe(true);
		expect(store.getSnapshot().suppressedQueuedMessageIDs.size).toBe(0);
	});

	it("keeps a marker for an id the snapshot never listed", () => {
		const store = createChatStore();
		const a = makeQueuedMessage(1, "A");
		const b = makeQueuedMessage(2, "B");
		const d = makeQueuedMessage(4, "D");

		store.suppressQueuedMessageIDs([a.id]);
		expect(store.acceptAuthoritativeQueueSnapshot([a, b, d], "socket")).toBe(
			true,
		);
		expect(store.getSnapshot().suppressedQueuedMessageIDs.has(a.id)).toBe(true);
	});

	it("rejects a stale snapshot that still lists a promoted message", () => {
		const store = createChatStore();
		const a = makeQueuedMessage(1, "A");
		const b = makeQueuedMessage(2, "B");
		const c = makeQueuedMessage(3, "C");

		store.markQueuedMessagePromoted(a.id);

		expect(store.acceptAuthoritativeQueueSnapshot([a, b], "socket")).toBe(
			false,
		);
		// A rejected snapshot retires nothing.
		expect(store.getSnapshot().promotedQueuedMessageIDs.has(a.id)).toBe(true);

		expect(store.acceptAuthoritativeQueueSnapshot([b, c], "socket")).toBe(true);
		expect(store.getSnapshot().promotedQueuedMessageIDs.size).toBe(0);
		expect(store.getSnapshot().suppressedQueuedMessageIDs.size).toBe(0);
	});

	it("does not treat the cache echo of its own write as a server statement", () => {
		const store = createChatStore();
		const a = makeQueuedMessage(1, "A");
		const tail = makeQueuedMessage(2, "B");

		expect(store.acceptAuthoritativeQueueSnapshot([a], "socket")).toBe(true);

		// Writing the projection to the cache changes the cached value, so the
		// write path arms the echo it is about to produce.
		store.noteLocalQueueProjection([a, tail]);

		// The cache arm observing that write: it passes, but the tail the
		// client invented is not recorded as a row the server listed.
		expect(
			store.acceptAuthoritativeQueueSnapshot([a, { ...tail }], "cache"),
		).toBe(true);
		expect(store.hasObservedQueuedMessageID(tail.id)).toBe(false);
	});

	it("does not arm a cache echo for the snapshot it accepts", () => {
		const store = createChatStore();
		const a = makeQueuedMessage(1, "A");

		// Whether the write that follows changes the cached value is the write
		// path's business, so the gate never arms an expectation of its own. A
		// snapshot equal to what is already cached collapses on write and
		// produces no observation to consume one.
		expect(store.acceptAuthoritativeQueueSnapshot([a], "socket")).toBe(true);
		store.markQueuedMessagePromoted(98);

		expect(store.acceptAuthoritativeQueueSnapshot([{ ...a }], "cache")).toBe(
			true,
		);
		expect(store.getSnapshot().promotedQueuedMessageIDs.has(98)).toBe(false);
	});

	it("does not re-gate a value the client projected into the cache itself", () => {
		const store = createChatStore();
		const a = makeQueuedMessage(1, "A");
		const b = makeQueuedMessage(2, "B");

		store.markQueuedMessagePromoted(a.id);
		store.noteLocalQueueProjection([b]);

		// The cache arm observing the promoted-head projection must not treat it
		// as a server snapshot, or it would retire the marker the send just set.
		expect(store.acceptAuthoritativeQueueSnapshot([b], "cache")).toBe(true);
		expect(store.getSnapshot().promotedQueuedMessageIDs.has(a.id)).toBe(true);
		expect(store.hasObservedQueuedMessageID(b.id)).toBe(false);
	});

	it("gates a socket snapshot that repeats the projected value", () => {
		const store = createChatStore();
		const a = makeQueuedMessage(1, "A");
		const b = makeQueuedMessage(2, "B");

		store.markQueuedMessagePromoted(a.id);
		store.noteLocalQueueProjection([b]);

		// Same value, but the server is the one saying it now: the projection
		// is confirmed, so the promoted marker it protected has to retire.
		expect(store.acceptAuthoritativeQueueSnapshot([{ ...b }], "socket")).toBe(
			true,
		);
		expect(store.getSnapshot().promotedQueuedMessageIDs.size).toBe(0);
		expect(store.hasObservedQueuedMessageID(b.id)).toBe(true);
	});

	it("gates a second cache observation of the projected value", () => {
		const store = createChatStore();
		const a = makeQueuedMessage(1, "A");
		const b = makeQueuedMessage(2, "B");

		store.markQueuedMessagePromoted(a.id);
		store.noteLocalQueueProjection([b]);
		store.acceptAuthoritativeQueueSnapshot([b], "cache");

		// The expectation is consumed by the first observation. A page-0
		// install carrying the same value afterwards is a server statement.
		expect(store.acceptAuthoritativeQueueSnapshot([{ ...b }], "cache")).toBe(
			true,
		);
		expect(store.getSnapshot().promotedQueuedMessageIDs.size).toBe(0);
	});

	it("rejects a promoted-listing snapshot before consuming the echo", () => {
		const store = createChatStore();
		const a = makeQueuedMessage(1, "A");
		const b = makeQueuedMessage(2, "B");

		store.noteLocalQueueProjection([a, b]);
		store.markQueuedMessagePromoted(a.id);

		// The veto outranks the echo: the projection is stale the moment a row
		// it lists is confirmed promoted.
		expect(store.acceptAuthoritativeQueueSnapshot([a, b], "cache")).toBe(false);
		expect(store.getSnapshot().promotedQueuedMessageIDs.has(a.id)).toBe(true);
	});

	it("does not record a client-projected tail as server-observed", () => {
		const store = createChatStore();
		const tail = makeQueuedMessage(7, "tail");

		store.noteLocalQueueProjection([tail]);
		store.acceptAuthoritativeQueueSnapshot([tail], "cache");

		// The send response returned this row; the server has not LISTED it in
		// a queue snapshot, which is the question the reconciliation asks.
		expect(store.hasObservedQueuedMessageID(tail.id)).toBe(false);
	});

	it("records queued IDs the server has reported", () => {
		const store = createChatStore();
		const a = makeQueuedMessage(1, "A");
		const b = makeQueuedMessage(2, "B");
		const c = makeQueuedMessage(3, "C");

		expect(store.hasObservedQueuedMessageID(a.id)).toBe(false);
		store.acceptAuthoritativeQueueSnapshot([a, b], "socket");
		expect(store.hasObservedQueuedMessageID(a.id)).toBe(true);
		expect(store.hasObservedQueuedMessageID(b.id)).toBe(true);

		store.acceptAuthoritativeQueueSnapshot([b], "socket");
		expect(store.hasObservedQueuedMessageID(a.id)).toBe(true);
		expect(store.hasObservedQueuedMessageID(c.id)).toBe(false);

		store.clearSuppressedQueuedMessageIDs();
		expect(store.hasObservedQueuedMessageID(a.id)).toBe(false);
	});
});

// ---------------------------------------------------------------------------
// suppression markers
// ---------------------------------------------------------------------------

describe("suppressQueuedMessageIDs / unsuppressQueuedMessageIDs", () => {
	it("adds and removes IDs from the suppression set", () => {
		const store = createChatStore();
		store.suppressQueuedMessageIDs([42, 43]);
		expect(store.getSnapshot().suppressedQueuedMessageIDs.has(42)).toBe(true);
		expect(store.getSnapshot().suppressedQueuedMessageIDs.has(43)).toBe(true);

		store.unsuppressQueuedMessageIDs([42]);
		expect(store.getSnapshot().suppressedQueuedMessageIDs.has(42)).toBe(false);
		expect(store.getSnapshot().suppressedQueuedMessageIDs.has(43)).toBe(true);
	});

	it("does not notify when every ID is already suppressed", () => {
		const store = createChatStore();
		store.suppressQueuedMessageIDs([42]);

		let notified = false;
		store.subscribe(() => {
			notified = true;
		});

		store.suppressQueuedMessageIDs([42]);
		store.unsuppressQueuedMessageIDs([99]);

		expect(notified).toBe(false);
	});

	it("keeps a promoted marker a failed queue operation does not own", () => {
		const store = createChatStore();
		const a = makeQueuedMessage(1, "A");

		store.markQueuedMessagePromoted(a.id);
		// A delete of the same ID failing after a send promoted it: the send
		// owns the veto, and only a server snapshot omitting the row may
		// retire it.
		store.unsuppressQueuedMessageIDs([a.id]);
		expect(store.getSnapshot().promotedQueuedMessageIDs.has(a.id)).toBe(true);
		expect(store.getSnapshot().suppressedQueuedMessageIDs.has(a.id)).toBe(true);

		// The veto still stands, so the snapshot that still queues A is stale.
		expect(store.acceptAuthoritativeQueueSnapshot([a], "socket")).toBe(false);
	});
});

// ---------------------------------------------------------------------------
// clearStreamState
// ---------------------------------------------------------------------------

describe("clearStreamState", () => {
	it("clears stream state to null", () => {
		const store = createChatStore();
		// Build up some stream state via applyMessagePart.
		store.applyMessagePart({ type: "text", text: "hello" });
		expect(store.getSnapshot().streamState).not.toBeNull();

		store.clearStreamState();

		expect(store.getSnapshot().streamState).toBeNull();
	});

	it("is a no-op when stream state is already null", () => {
		const store = createChatStore();

		let notified = false;
		store.subscribe(() => {
			notified = true;
		});
		store.clearStreamState();

		expect(notified).toBe(false);
	});
});

// ---------------------------------------------------------------------------
// applyMessagePart / applyMessageParts
// ---------------------------------------------------------------------------

describe("applyMessagePart / applyMessageParts", () => {
	it("creates stream state from a text part", () => {
		const store = createChatStore();

		store.applyMessagePart({ type: "text", text: "hello" });

		expect(store.getSnapshot().streamState?.blocks).toEqual([
			{ type: "response", text: "hello" },
		]);
	});

	it("appends to existing stream state", () => {
		const store = createChatStore();
		store.applyMessagePart({ type: "text", text: "hello" });
		store.applyMessagePart({ type: "text", text: " world" });

		expect(store.getSnapshot().streamState?.blocks).toEqual([
			{ type: "response", text: "hello world" },
		]);
	});

	it("applies multiple parts in a single batch", () => {
		const store = createChatStore();

		store.applyMessageParts([
			{ type: "text", text: "one" },
			{ type: "text", text: " two" },
		]);

		expect(store.getSnapshot().streamState?.blocks).toEqual([
			{ type: "response", text: "one two" },
		]);
	});

	it("is a no-op for an empty parts array", () => {
		const store = createChatStore();

		let notified = false;
		store.subscribe(() => {
			notified = true;
		});
		store.applyMessageParts([]);

		expect(notified).toBe(false);
	});
});

// ---------------------------------------------------------------------------
// resetTransientState
// ---------------------------------------------------------------------------

describe("resetTransientState", () => {
	it("clears streamState, streamError, retryState, reconnectState, and subagentOverrides", () => {
		const store = createChatStore();
		store.applyMessagePart({ type: "text", text: "stream" });
		store.setStreamError({
			kind: "generic",
			message: "oops",
		});
		store.setRetryState({
			attempt: 2,
			error: "rate limit",
			kind: "rate_limit",
			provider: "anthropic",
			retryingAt: "2025-01-01T00:01:00.000Z",
		});
		store.setReconnectState({
			attempt: 1,
			delayMs: 1000,
			retryingAt: "2025-01-01T00:00:01.000Z",
		});
		store.setSubagentStatusOverride("sub-1", "error");

		store.resetTransientState();

		const state = store.getSnapshot();
		expect(state.streamState).toBeNull();
		expect(state.streamError).toBeNull();
		expect(state.retryState).toBeNull();
		expect(state.reconnectState).toBeNull();
		expect(state.subagentStatusOverrides.size).toBe(0);
	});

	it("preserves the queue suppression markers", () => {
		const store = createChatStore();
		store.suppressQueuedMessageIDs([10]);
		store.setStreamError({
			kind: "generic",
			message: "oops",
		});

		store.resetTransientState();

		expect(store.getSnapshot().suppressedQueuedMessageIDs.has(10)).toBe(true);
	});

	it("is a no-op when all transient state is already clean", () => {
		const store = createChatStore();

		let notified = false;
		store.subscribe(() => {
			notified = true;
		});
		store.resetTransientState();

		expect(notified).toBe(false);
	});
});

// ---------------------------------------------------------------------------
// beginStreamFinalization
//
// The overlay hands off to the durable message that superseded it. The store
// half is the lifecycle below; the suppression half (exact ID membership in
// the cache-backed list) is covered in chatStore.test.tsx and
// durableChat.test.tsx.
// ---------------------------------------------------------------------------

describe("beginStreamFinalization", () => {
	it("moves the overlay into the finalizing snapshot and records the ID", () => {
		const store = createChatStore();
		store.applyMessagePart({ type: "text", text: "streamed answer" });
		const overlay = store.getSnapshot().streamState;

		store.beginStreamFinalization(42);

		const state = store.getSnapshot();
		expect(state.streamState).toBeNull();
		expect(state.finalizingStreamState).toBe(overlay);
		expect(state.finalizingMessageID).toBe(42);
	});

	it("starts a fresh stream state on the first part of the next turn", () => {
		const store = createChatStore();
		store.applyMessagePart({ type: "text", text: "first turn" });
		store.beginStreamFinalization(42);

		store.applyMessagePart({ type: "text", text: "second turn" });

		const state = store.getSnapshot();
		// The next turn's tokens must not accumulate into the finalized snapshot.
		expect(state.streamState?.blocks).toEqual([
			{ type: "response", text: "second turn" },
		]);
		expect(state.finalizingStreamState).toBeNull();
		expect(state.finalizingMessageID).toBeNull();
	});

	it("keeps the finalizing snapshot when a part carries no renderable output", () => {
		const store = createChatStore();
		store.applyMessagePart({ type: "text", text: "first turn" });
		store.beginStreamFinalization(42);

		store.applyMessagePart({ type: "text", text: "   " });

		const state = store.getSnapshot();
		expect(state.streamState).toBeNull();
		expect(state.finalizingMessageID).toBe(42);
	});

	it("drops a stale snapshot when there is no overlay to hand off", () => {
		const store = createChatStore();
		store.applyMessagePart({ type: "text", text: "first turn" });
		store.beginStreamFinalization(42);

		store.beginStreamFinalization(43);

		const state = store.getSnapshot();
		expect(state.finalizingStreamState).toBeNull();
		expect(state.finalizingMessageID).toBeNull();
	});

	// The timeline reads this flag to decide between its streaming and completed
	// presentations, so it has to stay true through the handoff window while the
	// finalized tail is still rendered by the overlay.
	it("reports a stream overlay while an active stream or a handoff is on screen", () => {
		const store = createChatStore();
		expect(selectHasStreamOverlay(store.getSnapshot())).toBe(false);

		store.applyMessagePart({ type: "text", text: "streamed answer" });
		expect(selectHasStreamOverlay(store.getSnapshot())).toBe(true);
		expect(selectHasStreamState(store.getSnapshot())).toBe(true);

		store.beginStreamFinalization(42);
		// The active stream is gone, but the finalized tail is still on screen.
		expect(selectHasStreamState(store.getSnapshot())).toBe(false);
		expect(selectHasStreamOverlay(store.getSnapshot())).toBe(true);

		store.clearStreamState();
		expect(selectHasStreamOverlay(store.getSnapshot())).toBe(false);
	});

	it.each([
		[
			"clearStreamState",
			(store: ReturnType<typeof createChatStore>) => store.clearStreamState(),
		],
		[
			"resetTransportReplayState",
			(store: ReturnType<typeof createChatStore>) =>
				store.resetTransportReplayState(),
		],
		[
			"resetTransientState",
			(store: ReturnType<typeof createChatStore>) =>
				store.resetTransientState(),
		],
		[
			"setStreamState",
			(store: ReturnType<typeof createChatStore>) => store.setStreamState(null),
		],
	])("%s clears the finalization handoff", (_label, clear) => {
		const store = createChatStore();
		store.applyMessagePart({ type: "text", text: "streamed answer" });
		store.beginStreamFinalization(42);

		clear(store);

		const state = store.getSnapshot();
		expect(state.finalizingStreamState).toBeNull();
		expect(state.finalizingMessageID).toBeNull();
	});
});

// ---------------------------------------------------------------------------
// subscribe
// ---------------------------------------------------------------------------

describe("subscribe", () => {
	it("returns an unsubscribe function that prevents future notifications", () => {
		const store = createChatStore();
		let callCount = 0;
		const unsubscribe = store.subscribe(() => {
			callCount += 1;
		});

		store.setStreamError({ kind: "generic", message: "first" });
		expect(callCount).toBe(1);

		unsubscribe();
		store.setStreamError({ kind: "generic", message: "second" });
		expect(callCount).toBe(1);
	});

	it("supports multiple concurrent subscribers", () => {
		const store = createChatStore();
		let countA = 0;
		let countB = 0;
		store.subscribe(() => {
			countA += 1;
		});
		store.subscribe(() => {
			countB += 1;
		});

		store.setStreamError({ kind: "generic", message: "boom" });

		expect(countA).toBe(1);
		expect(countB).toBe(1);
	});
});

import { describe, expect, it } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import { createChatStore, selectIsAwaitingFirstStreamChunk } from "./chatStore";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Minimal ChatMessage factory. `created_at` is derived from `id` to make
 *  ordering deterministic in tests that care about sort order. */
const makeMessage = (
	id: number,
	role: string,
	text: string,
	chatID = "chat-1",
): TypesGen.ChatMessage =>
	({
		id,
		chat_id: chatID,
		created_at: `2025-01-01T00:00:0${Math.max(Math.abs(id), 0)}.000Z`,
		role,
		content: [{ type: "text", text }],
	}) as TypesGen.ChatMessage;

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
// replaceMessages
// ---------------------------------------------------------------------------

describe("replaceMessages", () => {
	it("populates messagesByID and orderedMessageIDs", () => {
		const store = createChatStore();
		const msg1 = makeMessage(1, "user", "first");
		const msg2 = makeMessage(2, "assistant", "second");

		store.replaceMessages([msg1, msg2]);

		const state = store.getSnapshot();
		expect(state.messagesByID.size).toBe(2);
		expect(state.messagesByID.get(1)).toBe(msg1);
		expect(state.messagesByID.get(2)).toBe(msg2);
		expect(state.orderedMessageIDs).toEqual([1, 2]);
	});

	it("sorts messages by created_at", () => {
		const store = createChatStore();
		const older = {
			...makeMessage(1, "user", "first"),
			created_at: "2025-01-01T00:00:01.000Z",
		} as TypesGen.ChatMessage;
		const newer = {
			...makeMessage(2, "assistant", "second"),
			created_at: "2025-01-01T00:00:05.000Z",
		} as TypesGen.ChatMessage;

		// Insert in reverse order.
		store.replaceMessages([newer, older]);

		expect(store.getSnapshot().orderedMessageIDs).toEqual([1, 2]);
	});

	it("treats undefined as empty array", () => {
		const store = createChatStore();
		store.replaceMessages([makeMessage(1, "user", "hello")]);

		store.replaceMessages(undefined);

		const state = store.getSnapshot();
		expect(state.messagesByID.size).toBe(0);
		expect(state.orderedMessageIDs).toEqual([]);
	});

	it("does not notify subscribers when content is unchanged", () => {
		const store = createChatStore();
		const msg = makeMessage(1, "user", "hello");
		store.replaceMessages([msg]);

		let notified = false;
		store.subscribe(() => {
			notified = true;
		});

		// Same object reference — maps compare equal by ref.
		store.replaceMessages([msg]);

		expect(notified).toBe(false);
	});
});

// ---------------------------------------------------------------------------
// upsertDurableMessage
// ---------------------------------------------------------------------------

describe("upsertDurableMessage", () => {
	it("inserts a new message and reports isDuplicate=false, changed=true", () => {
		const store = createChatStore();
		const msg = makeMessage(1, "user", "hello");

		const result = store.upsertDurableMessage(msg);

		expect(result).toEqual({ isDuplicate: false, changed: true });
		expect(store.getSnapshot().messagesByID.get(1)).toBe(msg);
		expect(store.getSnapshot().orderedMessageIDs).toEqual([1]);
	});

	it("reports isDuplicate=true, changed=false for value-equal duplicate", () => {
		const store = createChatStore();
		const msg = makeMessage(1, "user", "hello");
		store.upsertDurableMessage(msg);

		// Different object reference, same field values.
		const dup = makeMessage(1, "user", "hello");
		const result = store.upsertDurableMessage(dup);

		expect(result).toEqual({ isDuplicate: true, changed: false });
	});

	it("reports isDuplicate=true, changed=true when content differs", () => {
		const store = createChatStore();
		store.upsertDurableMessage(makeMessage(1, "assistant", "draft"));

		const updated = makeMessage(1, "assistant", "final");
		const result = store.upsertDurableMessage(updated);

		expect(result).toEqual({ isDuplicate: true, changed: true });
		expect(store.getSnapshot().messagesByID.get(1)?.content).toEqual(
			updated.content,
		);
	});

	it("does not reorder when updating an existing message in place", () => {
		const store = createChatStore();
		store.upsertDurableMessage(makeMessage(1, "user", "first"));
		store.upsertDurableMessage(makeMessage(2, "assistant", "second"));
		const orderBefore = store.getSnapshot().orderedMessageIDs;

		// Update content of existing message (same ID, same map size).
		store.upsertDurableMessage(makeMessage(2, "assistant", "edited"));

		// Same reference — no reorder needed because the map size
		// didn't change and the ID already existed.
		expect(store.getSnapshot().orderedMessageIDs).toBe(orderBefore);
	});
});

// ---------------------------------------------------------------------------
// setChatStatus
// ---------------------------------------------------------------------------

describe("setChatStatus", () => {
	it("updates chatStatus", () => {
		const store = createChatStore();

		store.setChatStatus("running");

		expect(store.getSnapshot().chatStatus).toBe("running");
	});

	it("accepts null to clear the status", () => {
		const store = createChatStore();
		store.setChatStatus("running");

		store.setChatStatus(null);

		expect(store.getSnapshot().chatStatus).toBeNull();
	});

	it("does not notify when setting the same status", () => {
		const store = createChatStore();
		store.setChatStatus("running");

		let notified = false;
		store.subscribe(() => {
			notified = true;
		});
		store.setChatStatus("running");

		expect(notified).toBe(false);
	});
});

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
		});

		let notified = false;
		store.subscribe(() => {
			notified = true;
		});
		store.setStreamError({
			kind: "generic",
			message: "oops",
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
			delayMs: 3000,
			retryingAt: "2025-01-01T00:00:30.000Z",
		});
		expect(store.getSnapshot().retryState).toEqual({
			attempt: 1,
			error: "rate limited",
			kind: "rate_limit",
			provider: "anthropic",
			delayMs: 3000,
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
		store.setSubagentStatusOverride("sub-1", "completed");

		expect(store.getSnapshot().subagentStatusOverrides.get("sub-1")).toBe(
			"completed",
		);
	});
});

// ---------------------------------------------------------------------------
// setQueuedMessages
// ---------------------------------------------------------------------------

describe("setQueuedMessages", () => {
	it("stores queued messages", () => {
		const store = createChatStore();
		const qm = makeQueuedMessage(10, "queued");

		store.setQueuedMessages([qm]);

		expect(store.getSnapshot().queuedMessages).toEqual([qm]);
	});

	it("treats undefined as empty array", () => {
		const store = createChatStore();
		store.setQueuedMessages([makeQueuedMessage(1, "q")]);

		store.setQueuedMessages(undefined);

		expect(store.getSnapshot().queuedMessages).toEqual([]);
	});

	it("does not notify when queued message IDs are unchanged", () => {
		const store = createChatStore();
		const qm = makeQueuedMessage(10, "queued");
		store.setQueuedMessages([qm]);

		let notified = false;
		store.subscribe(() => {
			notified = true;
		});

		// Different object reference, same ID.
		store.setQueuedMessages([{ ...qm }]);

		expect(notified).toBe(false);
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
			delayMs: 5000,
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

	it("preserves messages and queued messages", () => {
		const store = createChatStore();
		store.replaceMessages([makeMessage(1, "user", "hello")]);
		store.setQueuedMessages([makeQueuedMessage(10, "queued")]);
		store.setStreamError({
			kind: "generic",
			message: "oops",
		});

		store.resetTransientState();

		const state = store.getSnapshot();
		expect(state.messagesByID.size).toBe(1);
		expect(state.queuedMessages).toHaveLength(1);
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
// subscribe
// ---------------------------------------------------------------------------

describe("subscribe", () => {
	it("returns an unsubscribe function that prevents future notifications", () => {
		const store = createChatStore();
		let callCount = 0;
		const unsubscribe = store.subscribe(() => {
			callCount += 1;
		});

		store.setChatStatus("running");
		expect(callCount).toBe(1);

		unsubscribe();
		store.setChatStatus("error");
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

		store.setChatStatus("running");

		expect(countA).toBe(1);
		expect(countB).toBe(1);
	});
});

// ---------------------------------------------------------------------------
// selectIsAwaitingFirstStreamChunk
// ---------------------------------------------------------------------------

describe("selectIsAwaitingFirstStreamChunk", () => {
	it("returns true when running with no stream state and no assistant message", () => {
		const store = createChatStore();
		store.setChatStatus("running");
		store.upsertDurableMessage(makeMessage(1, "user", "hello"));

		expect(selectIsAwaitingFirstStreamChunk(store.getSnapshot())).toBe(true);
	});

	it("returns false when the latest message is from the assistant", () => {
		const store = createChatStore();
		store.setChatStatus("running");
		store.upsertDurableMessage(makeMessage(1, "user", "hello"));
		store.upsertDurableMessage(makeMessage(2, "assistant", "hi there"));

		expect(selectIsAwaitingFirstStreamChunk(store.getSnapshot())).toBe(false);
	});

	it("returns false when stream state is present", () => {
		const store = createChatStore();
		store.setChatStatus("running");
		store.upsertDurableMessage(makeMessage(1, "user", "hello"));
		store.applyMessagePart({ type: "text", text: "response" });

		expect(selectIsAwaitingFirstStreamChunk(store.getSnapshot())).toBe(false);
	});

	it("returns true during pending status when latest message is from user", () => {
		const store = createChatStore();
		store.setChatStatus("pending");
		store.upsertDurableMessage(makeMessage(1, "user", "hello"));

		// "pending" with a user message as latest means the user
		// just submitted and is waiting for the server to start.
		expect(selectIsAwaitingFirstStreamChunk(store.getSnapshot())).toBe(true);
	});

	it("returns false during pending status when latest message is from assistant", () => {
		const store = createChatStore();
		store.setChatStatus("pending");
		store.upsertDurableMessage(makeMessage(1, "user", "hello"));
		store.upsertDurableMessage(makeMessage(2, "assistant", "calling tool"));

		// "pending" with an assistant message as latest means a
		// tool-call cycle is in progress, not a fresh user send.
		expect(selectIsAwaitingFirstStreamChunk(store.getSnapshot())).toBe(false);
	});

	it("returns false when chat status is null", () => {
		const store = createChatStore();
		store.upsertDurableMessage(makeMessage(1, "user", "hello"));

		expect(selectIsAwaitingFirstStreamChunk(store.getSnapshot())).toBe(false);
	});

	it("returns true when latest message is a tool result during running", () => {
		const store = createChatStore();
		store.setChatStatus("running");
		store.upsertDurableMessage(makeMessage(1, "user", "hello"));
		store.upsertDurableMessage(makeMessage(2, "assistant", "calling tool"));
		store.upsertDurableMessage(makeMessage(3, "tool", "tool result"));

		expect(selectIsAwaitingFirstStreamChunk(store.getSnapshot())).toBe(true);
	});

	it("returns false when latest message is a tool result during pending", () => {
		const store = createChatStore();
		store.setChatStatus("pending");
		store.upsertDurableMessage(makeMessage(1, "user", "hello"));
		store.upsertDurableMessage(makeMessage(2, "assistant", "calling tool"));
		store.upsertDurableMessage(makeMessage(3, "tool", "tool result"));

		// During "pending", the transport cannot deliver parts, so
		// we should not be in a "starting" state.
		expect(selectIsAwaitingFirstStreamChunk(store.getSnapshot())).toBe(false);
	});

	it("returns true after optimistic send: clearStreamState + setChatStatus('running') + upsertDurableMessage", () => {
		const store = createChatStore();
		// Simulate a completed previous turn: assistant replied,
		// then server transitioned to "pending".
		store.upsertDurableMessage(makeMessage(1, "user", "first question"));
		store.upsertDurableMessage(makeMessage(2, "assistant", "first answer"));
		store.setChatStatus("pending");

		// Verify baseline: not awaiting during pending.
		expect(selectIsAwaitingFirstStreamChunk(store.getSnapshot())).toBe(false);

		// Simulate handleSend after POST returns (non-queued).
		// This is the exact sequence from AgentChatPage.tsx.
		store.clearStreamState();
		store.setChatStatus("running");
		store.upsertDurableMessage(makeMessage(3, "user", "follow-up"));

		// "Thinking..." should appear immediately.
		expect(selectIsAwaitingFirstStreamChunk(store.getSnapshot())).toBe(true);
	});

	it("returns true when WS delivers user message + status:pending (fresh send)", () => {
		const store = createChatStore();
		// Simulate the WS batch: [message(user), status:pending].
		// This is the exact event order from the server when the
		// user sends a message. "Thinking..." must appear during
		// the pending phase so there is no visual gap before the
		// server transitions to running.
		store.upsertDurableMessage(makeMessage(1, "user", "sweet ty"));
		store.setChatStatus("pending");
		store.clearStreamState();

		expect(selectIsAwaitingFirstStreamChunk(store.getSnapshot())).toBe(true);
	});
});

describe("duplicate message deduplication", () => {
	it("replaceMessages deduplicates orderedMessageIDs when input has duplicate IDs", () => {
		const store = createChatStore();
		const msg1 = makeMessage(1, "user", "hello");
		const msg2 = makeMessage(2, "assistant", "hi");
		// Simulate cross-page duplication: same ID appears twice.
		const msg2Copy = makeMessage(2, "assistant", "hi");

		store.replaceMessages([msg1, msg2, msg2Copy]);

		const state = store.getSnapshot();
		// Map deduplicates by key — only 2 unique entries.
		expect(state.messagesByID.size).toBe(2);
		// orderedMessageIDs MUST also have only 2 entries.
		expect(state.orderedMessageIDs).toEqual([1, 2]);
	});
});

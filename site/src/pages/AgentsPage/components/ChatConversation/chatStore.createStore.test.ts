import { describe, expect, it, vi } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import {
	type ChatStore,
	createChatStore,
	selectChatStatus,
	selectIsAwaitingFirstStreamChunk,
	selectMessagesByID,
	selectOrderedMessageIDs,
	selectQueuedMessages,
	selectStreamError,
} from "./chatStore";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

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

	it("sorts messages by id when created_at disagrees with append order", () => {
		const store = createChatStore();
		const first = {
			...makeMessage(1, "user", "first"),
			created_at: "2025-01-01T00:00:05.000Z",
		} as TypesGen.ChatMessage;
		const second = {
			...makeMessage(2, "assistant", "second"),
			created_at: "2025-01-01T00:00:01.000Z",
		} as TypesGen.ChatMessage;

		// Insert in reverse order.
		store.replaceMessages([second, first]);

		expect(store.getSnapshot().orderedMessageIDs).toEqual([1, 2]);
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

describe("upsertDurableMessages", () => {
	it("orders merged messages by id rather than by arrival", () => {
		const store = createChatStore();
		const sharedCreatedAt = "2025-01-01T00:00:00.000Z";
		const withSharedCreatedAt = (
			id: number,
			role: string,
		): TypesGen.ChatMessage => ({
			...makeMessage(id, role, `message-${id}`),
			created_at: sharedCreatedAt,
		});

		store.replaceMessages([
			withSharedCreatedAt(3, "assistant"),
			withSharedCreatedAt(4, "tool"),
		]);
		store.upsertDurableMessages([
			withSharedCreatedAt(1, "user"),
			withSharedCreatedAt(2, "assistant"),
		]);

		expect(store.getSnapshot().orderedMessageIDs).toEqual([1, 2, 3, 4]);
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
// setQueuedMessages
// ---------------------------------------------------------------------------

describe("setQueuedMessages", () => {
	it("stores queued messages", () => {
		const store = createChatStore();
		const qm = makeQueuedMessage(10, "queued");

		store.setQueuedMessages([qm]);

		expect(store.getSnapshot().queuedMessages).toEqual([qm]);
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
// suppressQueuedMessageID / applyAuthoritativeQueuedMessages
// ---------------------------------------------------------------------------

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

	it("returns false during waiting status when latest message is from user", () => {
		const store = createChatStore();
		store.setChatStatus("waiting");
		store.upsertDurableMessage(makeMessage(1, "user", "hello"));

		// "waiting" means the chat is idle; nothing is generating,
		// so no Thinking indicator should show.
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

	it("returns true after optimistic send: clearStreamState + setChatStatus('running') + upsertDurableMessage", () => {
		const store = createChatStore();
		// Simulate a settled previous turn: assistant replied,
		// then server transitioned to "waiting".
		store.upsertDurableMessage(makeMessage(1, "user", "first question"));
		store.upsertDurableMessage(makeMessage(2, "assistant", "first answer"));
		store.setChatStatus("waiting");

		// Verify baseline: not awaiting while idle.
		expect(selectIsAwaitingFirstStreamChunk(store.getSnapshot())).toBe(false);

		// Simulate handleSend after POST returns (non-queued).
		// This is the exact sequence from AgentChatPage.tsx.
		store.clearStreamState();
		store.setChatStatus("running");
		store.upsertDurableMessage(makeMessage(3, "user", "follow-up"));

		expect(selectIsAwaitingFirstStreamChunk(store.getSnapshot())).toBe(true);
	});

	it("returns true when WS delivers user message + status:running (fresh send)", () => {
		const store = createChatStore();
		// Simulate the WS batch: [message(user), status:running].
		// This is the event order from the server when the user
		// sends a message. The Thinking indicator must appear
		// before the first stream chunk arrives.
		store.upsertDurableMessage(makeMessage(1, "user", "sweet ty"));
		store.setChatStatus("running");
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

// ---------------------------------------------------------------------------
// Pending actions
// ---------------------------------------------------------------------------

const seededStore = (
	messages: readonly TypesGen.ChatMessage[],
	queuedMessages: readonly TypesGen.ChatQueuedMessage[],
	status: TypesGen.ChatStatus,
): ChatStore => {
	const store = createChatStore();
	store.seed({
		messages,
		queued_messages: queuedMessages,
		has_more: false,
		snapshot_version: 1,
	});
	store.applyServerChatStatus(status);
	store.applyCheckpoint(1);
	return store;
};

// What a viewer sees: the transcript texts, the queue IDs and the status.
const visible = (store: ChatStore) => {
	const state = store.getSnapshot();
	const byID = selectMessagesByID(state);
	return {
		transcript: selectOrderedMessageIDs(state).map((id) => {
			const message = byID.get(id);
			const part = message?.content?.[0];
			return `${message?.role}:${part?.type === "text" ? part.text : ""}`;
		}),
		queue: selectQueuedMessages(state).map((message) => message.id),
		status: selectChatStatus(state),
		error: selectStreamError(state)?.message ?? null,
	};
};

const chatError = {
	message: "provider failed",
	kind: "generic",
} as const;

describe("promoteQueued overlay", () => {
	it.each(["error", "requires_action"] as const)(
		"from %s shows the row as the next prompt of a running turn",
		(status) => {
			const store = seededStore(
				[makeMessage(1, "user", "first"), makeMessage(2, "assistant", "a")],
				[makeQueuedMessage(10, "q10"), makeQueuedMessage(11, "q11")],
				status,
			);
			store.setStreamError(chatError);

			store.addPendingAction({ kind: "promoteQueued", queuedMessageID: 11 });

			expect(visible(store)).toEqual({
				transcript: ["user:first", "assistant:a", "user:q11"],
				queue: [10],
				status: "running",
				error: null,
			});
		},
	);

	it.each(["running", "interrupting"] as const)(
		"from %s moves the row to the head and shows the turn stopping",
		(status) => {
			const store = seededStore(
				[makeMessage(1, "user", "first")],
				[makeQueuedMessage(10, "q10"), makeQueuedMessage(11, "q11")],
				status,
			);

			store.addPendingAction({ kind: "promoteQueued", queuedMessageID: 11 });

			expect(visible(store)).toEqual({
				transcript: ["user:first"],
				queue: [11, 10],
				status: "interrupting",
				error: null,
			});
		},
	);

	it("shows the promoted row once when the stream already holds its message", () => {
		const store = seededStore(
			[makeMessage(1, "user", "first")],
			[makeQueuedMessage(11, "q11")],
			"error",
		);
		store.addPendingAction({ kind: "promoteQueued", queuedMessageID: 11 });

		// A sync can span frames: its message arrives before its queue_update.
		store.upsertDurableMessage({
			...makeMessage(2, "user", "q11"),
			queued_message_id: 11,
		});

		expect(visible(store)).toMatchObject({
			transcript: ["user:first", "user:q11"],
			queue: [],
		});
	});
});

// Each scenario runs the same stream events on a store that acted (with its
// overlay) and on one that did not; once a checkpoint reaches the action's
// version, both must show the same chat.
type ConvergenceScenario = {
	name: string;
	messages: readonly TypesGen.ChatMessage[];
	queue: readonly TypesGen.ChatQueuedMessage[];
	status: TypesGen.ChatStatus;
	action: Parameters<ChatStore["addPendingAction"]>[0];
	// The stream's events for the action's commit.
	stream: (store: ChatStore) => void;
	// Another viewer's commit lands first, so the overlay cannot predict it.
	unpredictable?: boolean;
};

const W = 5;

const convergenceScenarios: ConvergenceScenario[] = [
	{
		name: "send to an idle chat",
		messages: [makeMessage(1, "user", "first")],
		queue: [],
		status: "waiting",
		action: { kind: "send", messages: [makeMessage(2, "user", "second")] },
		stream: (store) => {
			store.upsertDurableMessage(makeMessage(2, "user", "second"));
			store.applyServerChatStatus("running");
		},
	},
	{
		name: "queued send",
		messages: [makeMessage(1, "user", "first")],
		queue: [],
		status: "running",
		action: {
			kind: "send",
			messages: [],
			queuedMessage: makeQueuedMessage(10, "q10"),
		},
		stream: (store) => store.setQueuedMessages([makeQueuedMessage(10, "q10")]),
	},
	{
		name: "edit",
		messages: [
			makeMessage(1, "user", "first"),
			makeMessage(2, "assistant", "a"),
			makeMessage(3, "user", "old"),
		],
		queue: [makeQueuedMessage(10, "q10")],
		status: "running",
		action: {
			kind: "edit",
			editedMessageID: 3,
			message: makeMessage(4, "user", "new"),
		},
		stream: (store) => {
			store.replaceMessagesFrom(3, [makeMessage(4, "user", "new")]);
			store.setQueuedMessages([]);
		},
	},
	{
		name: "delete queued",
		messages: [makeMessage(1, "user", "first")],
		queue: [makeQueuedMessage(10, "q10"), makeQueuedMessage(11, "q11")],
		status: "running",
		action: { kind: "deleteQueued", queuedMessageID: 10 },
		stream: (store) => store.setQueuedMessages([makeQueuedMessage(11, "q11")]),
	},
	{
		name: "promote from error",
		messages: [makeMessage(1, "user", "first")],
		queue: [makeQueuedMessage(10, "q10"), makeQueuedMessage(11, "q11")],
		status: "error",
		action: { kind: "promoteQueued", queuedMessageID: 11 },
		stream: (store) => {
			store.upsertDurableMessage({
				...makeMessage(2, "user", "q11"),
				queued_message_id: 11,
			});
			store.setQueuedMessages([makeQueuedMessage(10, "q10")]);
			store.applyServerChatStatus("running");
		},
	},
	{
		name: "promote from running",
		messages: [makeMessage(1, "user", "first")],
		queue: [makeQueuedMessage(10, "q10"), makeQueuedMessage(11, "q11")],
		status: "running",
		action: { kind: "promoteQueued", queuedMessageID: 11 },
		stream: (store) => {
			store.setQueuedMessages([
				makeQueuedMessage(11, "q11"),
				makeQueuedMessage(10, "q10"),
			]);
			store.applyServerChatStatus("interrupting");
		},
	},
	{
		name: "compact",
		messages: [makeMessage(1, "user", "first")],
		queue: [],
		status: "waiting",
		action: { kind: "compact" },
		stream: (store) => store.applyServerChatStatus("running"),
	},
	{
		name: "another tab's edit deletes the pending send",
		messages: [makeMessage(1, "user", "first")],
		queue: [],
		status: "waiting",
		action: { kind: "send", messages: [makeMessage(2, "user", "mine")] },
		stream: (store) => {
			store.replaceMessagesFrom(1, [makeMessage(3, "user", "theirs")]);
			store.applyServerChatStatus("running");
		},
		unpredictable: true,
	},
];

describe("pending actions converge with the stream", () => {
	it.each(convergenceScenarios)("$name", (scenario) => {
		const actor = seededStore(
			scenario.messages,
			scenario.queue,
			scenario.status,
		);
		const observer = seededStore(
			scenario.messages,
			scenario.queue,
			scenario.status,
		);
		const pendingID = actor.addPendingAction(scenario.action);
		actor.updatePendingAction(pendingID, { untilVersion: W });

		// A stream that lags the response keeps the overlay.
		for (const store of [actor, observer]) {
			store.applyCheckpoint(W - 1, 1);
		}
		expect(actor.getSnapshot().pendingActions).toHaveLength(1);
		const shownBeforeCommit = visible(actor);

		for (const store of [actor, observer]) {
			scenario.stream(store);
			store.applyCheckpoint(W, 2);
		}
		expect(actor.getSnapshot().pendingActions).toEqual([]);
		expect(visible(actor)).toEqual(visible(observer));
		// The overlay showed the committed state, so nothing moves when it ends.
		if (!scenario.unpredictable) {
			expect(shownBeforeCommit).toEqual(visible(observer));
		}
	});

	it("adds no overlay for an action the stream already reached", () => {
		const store = seededStore([makeMessage(1, "user", "first")], [], "waiting");
		store.upsertDurableMessage(makeMessage(2, "user", "second"));
		store.applyCheckpoint(W, 2);

		store.addPendingAction(
			{ kind: "send", messages: [makeMessage(2, "user", "second")] },
			W,
		);

		expect(store.getSnapshot().pendingActions).toEqual([]);
	});

	it("removes a failed action's overlay", () => {
		const store = seededStore(
			[makeMessage(1, "user", "first")],
			[makeQueuedMessage(10, "q10")],
			"running",
		);
		const pendingID = store.addPendingAction({
			kind: "deleteQueued",
			queuedMessageID: 10,
		});
		expect(visible(store).queue).toEqual([]);

		store.removePendingAction(pendingID);

		expect(visible(store).queue).toEqual([10]);
	});
});

describe("prependOlderMessages", () => {
	it("prepends an older page", () => {
		const store = createChatStore();
		store.seed({
			messages: [makeMessage(3, "user", "c")],
			queued_messages: [],
			has_more: true,
			snapshot_version: 1,
		});
		const resetCount = store.getHistoryResetCount();

		store.prependOlderMessages(
			[makeMessage(1, "user", "a"), makeMessage(2, "assistant", "b")],
			false,
			resetCount,
		);

		expect(store.getSnapshot().orderedMessageIDs).toEqual([1, 2, 3]);
		expect(store.getSnapshot().hasMoreMessages).toBe(false);
	});

	it("drops a page that loaded across a history reset", () => {
		const store = createChatStore();
		store.seed({
			messages: [makeMessage(3, "user", "c")],
			queued_messages: [],
			has_more: true,
			snapshot_version: 1,
		});
		const resetCount = store.getHistoryResetCount();
		store.replaceMessagesFrom(undefined, [makeMessage(4, "user", "new")]);

		store.prependOlderMessages(
			[makeMessage(1, "user", "a"), makeMessage(2, "assistant", "b")],
			false,
			resetCount,
		);

		expect(store.getSnapshot().orderedMessageIDs).toEqual([4]);
	});
});

describe("load", () => {
	it("fetches the newest page once, however often it is asked", async () => {
		const store = createChatStore();
		const fetchPage = vi.fn(async () => ({
			messages: [makeMessage(2, "assistant", "b"), makeMessage(1, "user", "a")],
			queued_messages: [],
			has_more: true,
			snapshot_version: 4,
		}));

		store.load(fetchPage);
		store.load(fetchPage);
		await vi.waitFor(() => {
			expect(store.getSnapshot().load).toEqual({ status: "loaded" });
		});
		store.load(fetchPage);

		expect(fetchPage).toHaveBeenCalledTimes(1);
		expect(store.getSnapshot().orderedMessageIDs).toEqual([1, 2]);
		expect(store.getSnapshot().hasMoreMessages).toBe(true);
		expect(store.getSyncedSnapshotVersion()).toBe(4);
	});
});

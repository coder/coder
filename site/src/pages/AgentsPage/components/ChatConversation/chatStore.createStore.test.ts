import { describe, expect, it, vi } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import {
	type ChatStore,
	createChatStore,
	isPendingRowID,
	type PendingAction,
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
// applyServerChatStatus
// ---------------------------------------------------------------------------

const retryState = {
	attempt: 2,
	error: "rate limited",
	kind: "rate_limit",
} as const;

describe("applyServerChatStatus", () => {
	it("keeps the turn error while the status is error", () => {
		const store = createChatStore();
		store.applyServerChatStatus("error");
		store.setTurnError({ kind: "generic", message: "provider down" });

		store.applyServerChatStatus("error");
		expect(selectStreamError(store.getSnapshot())?.message).toBe(
			"provider down",
		);

		store.applyServerChatStatus("running");
		expect(selectStreamError(store.getSnapshot())).toBeNull();
	});

	it("keeps the retry while the status is running", () => {
		const store = createChatStore();
		store.applyServerChatStatus("running");
		store.setRetryState(retryState);

		store.applyServerChatStatus("running");
		expect(store.getSnapshot().retryState).toEqual(retryState);

		store.applyServerChatStatus("waiting");
		expect(store.getSnapshot().retryState).toBeNull();
	});

	it("does not notify when the status is unchanged", () => {
		const store = createChatStore();
		store.applyServerChatStatus("running");

		let notified = false;
		store.subscribe(() => {
			notified = true;
		});
		store.applyServerChatStatus("running");

		expect(notified).toBe(false);
	});

	it("keeps a client error that the turn does not report", () => {
		const store = createChatStore();
		store.setStreamError({ kind: "generic", message: "request failed" });

		store.applyServerChatStatus("running");

		expect(selectStreamError(store.getSnapshot())).toMatchObject({
			message: "request failed",
		});
	});
});

describe("advanceSnapshotVersion", () => {
	it("keeps the highest version", () => {
		const store = createChatStore();
		store.seed({
			messages: [],
			queued_messages: [],
			has_more: false,
			snapshot_version: 5,
		});

		store.advanceSnapshotVersion(4);
		expect(store.getSyncedSnapshotVersion()).toBe(5);
		store.advanceSnapshotVersion(7);
		store.advanceSnapshotVersion(6);
		expect(store.getSyncedSnapshotVersion()).toBe(7);
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

		store.applyServerChatStatus("running");
		expect(callCount).toBe(1);

		unsubscribe();
		store.applyServerChatStatus("error");
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

		store.applyServerChatStatus("running");

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
		store.applyServerChatStatus("running");
		store.upsertDurableMessage(makeMessage(1, "user", "hello"));

		expect(selectIsAwaitingFirstStreamChunk(store.getSnapshot())).toBe(true);
	});

	it("returns false when the latest message is from the assistant", () => {
		const store = createChatStore();
		store.applyServerChatStatus("running");
		store.upsertDurableMessage(makeMessage(1, "user", "hello"));
		store.upsertDurableMessage(makeMessage(2, "assistant", "hi there"));

		expect(selectIsAwaitingFirstStreamChunk(store.getSnapshot())).toBe(false);
	});

	it("returns false when stream state is present", () => {
		const store = createChatStore();
		store.applyServerChatStatus("running");
		store.upsertDurableMessage(makeMessage(1, "user", "hello"));
		store.applyMessagePart({ type: "text", text: "response" });

		expect(selectIsAwaitingFirstStreamChunk(store.getSnapshot())).toBe(false);
	});

	it("returns false during waiting status when latest message is from user", () => {
		const store = createChatStore();
		store.applyServerChatStatus("waiting");
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
		store.applyServerChatStatus("running");
		store.upsertDurableMessage(makeMessage(1, "user", "hello"));
		store.upsertDurableMessage(makeMessage(2, "assistant", "calling tool"));
		store.upsertDurableMessage(makeMessage(3, "tool", "tool result"));

		expect(selectIsAwaitingFirstStreamChunk(store.getSnapshot())).toBe(true);
	});

	it("returns true when the stream delivers a new prompt and a running status", () => {
		const store = createChatStore();
		// The Thinking indicator must appear before the first stream chunk.
		store.upsertDurableMessage(makeMessage(1, "user", "first question"));
		store.upsertDurableMessage(makeMessage(2, "assistant", "first answer"));
		store.applyServerChatStatus("waiting");
		expect(selectIsAwaitingFirstStreamChunk(store.getSnapshot())).toBe(false);

		store.upsertDurableMessage(makeMessage(3, "user", "follow-up"));
		store.clearStreamState();
		store.applyServerChatStatus("running");

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
	return store;
};

const textOf = (content: readonly TypesGen.ChatMessagePart[] | undefined) => {
	const part = content?.[0];
	return part?.type === "text" ? part.text : "";
};

// What a viewer sees: the transcript, the queue texts and the status.
const visible = (store: ChatStore) => {
	const state = store.getSnapshot();
	const byID = selectMessagesByID(state);
	return {
		transcript: selectOrderedMessageIDs(state).map((id) => {
			const message = byID.get(id);
			return `${message?.role}:${textOf(message?.content)}`;
		}),
		queue: selectQueuedMessages(state).map((message) =>
			textOf(message.content),
		),
		status: selectChatStatus(state),
		error: selectStreamError(state)?.message ?? null,
	};
};

const chatError = {
	message: "provider failed",
	kind: "generic",
} as const;

// A user message as submitChatTurn builds it at the click.
const eagerSend = (text: string): PendingAction => ({
	kind: "send",
	message: {
		id: 0,
		chat_id: "chat-1",
		created_at: "2025-01-01T00:00:00.000Z",
		role: "user",
		content: [{ type: "text", text }],
	},
});

describe("pending actions", () => {
	it.each(["error", "requires_action"] as const)(
		"promote from %s shows the row as the next prompt of a running turn",
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
				queue: ["q10"],
				status: "running",
				error: null,
			});
		},
	);

	it.each(["running", "interrupting"] as const)(
		"promote from %s moves the row to the head and shows the turn stopping",
		(status) => {
			const store = seededStore(
				[makeMessage(1, "user", "first")],
				[makeQueuedMessage(10, "q10"), makeQueuedMessage(11, "q11")],
				status,
			);

			store.addPendingAction({ kind: "promoteQueued", queuedMessageID: 11 });

			expect(visible(store)).toEqual({
				transcript: ["user:first"],
				queue: ["q11", "q10"],
				status: "interrupting",
				error: null,
			});
		},
	);

	it("places a promote by the status an earlier pending action shows", () => {
		const store = seededStore(
			[makeMessage(1, "user", "first"), makeMessage(2, "assistant", "a")],
			[makeQueuedMessage(10, "q10"), makeQueuedMessage(11, "q11")],
			"error",
		);
		store.addPendingAction({ kind: "compact" });

		// The compaction turn runs when the promote reaches the server.
		store.addPendingAction({ kind: "promoteQueued", queuedMessageID: 11 });

		expect(visible(store)).toMatchObject({
			transcript: ["user:first", "assistant:a"],
			queue: ["q11", "q10"],
			status: "interrupting",
		});
	});

	it("queues a send made while an earlier send is pending", () => {
		const store = seededStore([makeMessage(1, "user", "first")], [], "waiting");

		store.addPendingAction(eagerSend("second"));
		store.addPendingAction(eagerSend("third"));

		expect(visible(store)).toMatchObject({
			transcript: ["user:first", "user:second"],
			queue: ["third"],
		});
	});

	it("keeps a pending message through stream updates without user messages", () => {
		const store = seededStore([makeMessage(1, "user", "first")], [], "waiting");
		store.addPendingAction(eagerSend("second"));

		store.upsertDurableMessages([makeMessage(2, "assistant", "late")]);
		store.applyServerChatStatus("waiting");

		expect(visible(store)).toEqual({
			transcript: ["user:first", "assistant:late", "user:second"],
			queue: [],
			status: "waiting",
			error: null,
		});
	});

	it("gives the rows it draws IDs the server never assigns", () => {
		const store = seededStore(
			[makeMessage(1, "user", "first"), makeMessage(2, "user", "old")],
			[],
			"waiting",
		);
		store.addPendingAction({
			kind: "edit",
			editedMessageID: 2,
			message: makeMessage(2, "user", "new"),
		});
		store.addPendingAction(eagerSend("queued"));

		const state = store.getSnapshot();
		const shown = selectOrderedMessageIDs(state).map((id) => [
			textOf(selectMessagesByID(state).get(id)?.content),
			isPendingRowID(id),
		]);
		expect(shown).toEqual([
			["first", false],
			["new", true],
		]);
		expect(
			selectQueuedMessages(state).map((row) => isPendingRowID(row.id)),
		).toEqual([true]);
	});

	it("shows the turn stopping when a running chat's head is promoted", () => {
		const store = seededStore(
			[makeMessage(1, "user", "first")],
			[makeQueuedMessage(10, "q10"), makeQueuedMessage(11, "q11")],
			"running",
		);

		store.addPendingAction({ kind: "promoteQueued", queuedMessageID: 10 });
		expect(visible(store)).toMatchObject({
			queue: ["q10", "q11"],
			status: "interrupting",
		});

		// The head does not move, so the commit sends only a status.
		store.applyServerChatStatus("interrupting");
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

		expect(visible(store).queue).toEqual(["q10"]);
	});
});

// Each scenario applies the same stream updates to a store that acted (with
// its pending action) and to one that did not; after them both must show the
// same chat.
type ConvergenceScenario = {
	name: string;
	messages: readonly TypesGen.ChatMessage[];
	queue: readonly TypesGen.ChatQueuedMessage[];
	status: TypesGen.ChatStatus;
	action: PendingAction;
	// The stream's updates, then the status it reports.
	stream: (store: ChatStore) => void;
	statusAfter: TypesGen.ChatStatus;
	// Another viewer's commit lands first, so the action cannot predict it.
	unpredictable?: boolean;
};

const convergenceScenarios: ConvergenceScenario[] = [
	{
		name: "send to an idle chat",
		messages: [makeMessage(1, "user", "first")],
		queue: [],
		status: "waiting",
		action: eagerSend("second"),
		stream: (store) => {
			store.upsertDurableMessages([makeMessage(2, "user", "second")]);
		},
		statusAfter: "running",
	},
	{
		name: "send to a running chat",
		messages: [makeMessage(1, "user", "first")],
		queue: [],
		status: "running",
		action: eagerSend("q10"),
		stream: (store) => store.setQueuedMessages([makeQueuedMessage(10, "q10")]),
		statusAfter: "running",
	},
	{
		name: "send to an errored chat with a queue",
		messages: [makeMessage(1, "user", "first")],
		queue: [makeQueuedMessage(10, "q10")],
		status: "error",
		action: eagerSend("q11"),
		stream: (store) => {
			store.upsertDurableMessages([
				{ ...makeMessage(2, "user", "q10"), queued_message_id: 10 },
			]);
			store.setQueuedMessages([makeQueuedMessage(11, "q11")]);
		},
		statusAfter: "running",
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
			message: makeMessage(3, "user", "new"),
		},
		stream: (store) => {
			store.resetHistoryFrom(3);
			store.upsertDurableMessages([makeMessage(4, "user", "new")]);
			store.setQueuedMessages([]);
		},
		statusAfter: "running",
	},
	{
		name: "delete queued",
		messages: [makeMessage(1, "user", "first")],
		queue: [makeQueuedMessage(10, "q10"), makeQueuedMessage(11, "q11")],
		status: "running",
		action: { kind: "deleteQueued", queuedMessageID: 10 },
		stream: (store) => store.setQueuedMessages([makeQueuedMessage(11, "q11")]),
		statusAfter: "running",
	},
	{
		name: "promote from error",
		messages: [makeMessage(1, "user", "first")],
		queue: [makeQueuedMessage(10, "q10"), makeQueuedMessage(11, "q11")],
		status: "error",
		action: { kind: "promoteQueued", queuedMessageID: 11 },
		stream: (store) => {
			store.upsertDurableMessages([
				{ ...makeMessage(2, "user", "q11"), queued_message_id: 11 },
			]);
			store.setQueuedMessages([makeQueuedMessage(10, "q10")]);
		},
		statusAfter: "running",
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
		},
		statusAfter: "interrupting",
	},
	{
		name: "compact",
		messages: [makeMessage(1, "user", "first")],
		queue: [],
		status: "waiting",
		action: { kind: "compact" },
		stream: () => {},
		statusAfter: "running",
	},
	{
		name: "another tab's message replaces the pending one",
		messages: [makeMessage(1, "user", "first")],
		queue: [],
		status: "waiting",
		action: eagerSend("mine"),
		stream: (store) => {
			store.upsertDurableMessages([makeMessage(2, "user", "theirs")]);
		},
		statusAfter: "running",
		unpredictable: true,
	},
	{
		name: "another tab's edit deletes the pending send",
		messages: [makeMessage(1, "user", "first")],
		queue: [],
		status: "waiting",
		action: eagerSend("mine"),
		stream: (store) => {
			store.resetHistoryFrom(1);
			store.upsertDurableMessages([makeMessage(3, "user", "theirs")]);
		},
		statusAfter: "running",
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
		actor.addPendingAction(scenario.action);
		const shownBeforeStream = visible(actor);

		for (const store of [actor, observer]) {
			store.batch(() => {
				scenario.stream(store);
				store.applyServerChatStatus(scenario.statusAfter);
			});
		}

		expect(actor.getSnapshot().pendingActions).toEqual([]);
		expect(visible(actor)).toEqual(visible(observer));
		// The action showed the committed state, so nothing moves when it ends.
		if (!scenario.unpredictable) {
			expect(shownBeforeStream).toEqual(visible(observer));
		}
	});
});

// Each pending part ends at the next stream update of its kind and outlives
// updates of every other kind.
const partEndScenarios: {
	name: string;
	status: TypesGen.ChatStatus;
	action: PendingAction;
	others: (store: ChatStore) => void;
	end: (store: ChatStore) => void;
}[] = [
	{
		name: "an added message at a user message",
		status: "waiting",
		action: eagerSend("second"),
		others: (store) => {
			store.upsertDurableMessages([makeMessage(2, "assistant", "late")]);
			store.resetHistoryFrom(99);
			store.applyServerChatStatus("running");
		},
		end: (store) => {
			store.upsertDurableMessages([makeMessage(3, "user", "second")]);
		},
	},
	{
		name: "an added message at a queue update",
		status: "running",
		action: eagerSend("q10"),
		others: (store) => {
			store.upsertDurableMessages([makeMessage(2, "assistant", "late")]);
			store.resetHistoryFrom(99);
			store.applyServerChatStatus("running");
		},
		end: (store) => store.setQueuedMessages([makeQueuedMessage(10, "q10")]),
	},
	{
		name: "a queue change at a queue update",
		status: "running",
		action: { kind: "deleteQueued", queuedMessageID: 10 },
		others: (store) => {
			store.upsertDurableMessages([makeMessage(2, "user", "theirs")]);
			store.resetHistoryFrom(99);
			store.applyServerChatStatus("running");
		},
		end: (store) => store.setQueuedMessages([]),
	},
	{
		name: "an edit at a history reset",
		status: "waiting",
		action: {
			kind: "edit",
			editedMessageID: 1,
			message: makeMessage(1, "user", "new"),
		},
		others: (store) => {
			store.upsertDurableMessages([makeMessage(2, "user", "theirs")]);
			store.setQueuedMessages([makeQueuedMessage(10, "q10")]);
			store.applyServerChatStatus("running");
		},
		end: (store) => store.resetHistoryFrom(1),
	},
	{
		name: "a status at the next status",
		status: "waiting",
		action: { kind: "compact" },
		others: (store) => {
			store.upsertDurableMessages([makeMessage(2, "user", "theirs")]);
			store.setQueuedMessages([makeQueuedMessage(10, "q10")]);
			store.resetHistoryFrom(99);
		},
		end: (store) => store.applyServerChatStatus("running"),
	},
];

describe("pending part ends", () => {
	it.each(partEndScenarios)("ends $name", (scenario) => {
		const store = seededStore(
			[makeMessage(1, "user", "first")],
			scenario.action.kind === "deleteQueued"
				? [makeQueuedMessage(10, "q10")]
				: [],
			scenario.status,
		);
		store.addPendingAction(scenario.action);

		scenario.others(store);
		expect(store.getSnapshot().pendingActions).toHaveLength(1);

		scenario.end(store);
		expect(store.getSnapshot().pendingActions).toEqual([]);
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
		store.resetHistoryFrom(0);
		store.upsertDurableMessages([makeMessage(4, "user", "new")]);

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

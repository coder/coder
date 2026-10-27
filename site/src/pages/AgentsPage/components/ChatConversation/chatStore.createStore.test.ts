import { describe, expect, it } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import { createDeferred } from "#/testHelpers/deferred";
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
	selectShownError,
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

describe("upsertDurableMessages", () => {
	it("populates messagesByID and orderedMessageIDs", () => {
		const store = createChatStore();
		const msg1 = makeMessage(1, "user", "first");
		const msg2 = makeMessage(2, "assistant", "second");

		store.upsertDurableMessages([msg1, msg2], 2);

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

		store.upsertDurableMessages([second, first], 2);

		expect(store.getSnapshot().orderedMessageIDs).toEqual([1, 2]);
	});

	it("does not notify subscribers when content is unchanged", () => {
		const store = createChatStore();
		store.upsertDurableMessages([makeMessage(1, "user", "hello")], 2);

		let notified = false;
		store.subscribe(() => {
			notified = true;
		});

		store.upsertDurableMessages([makeMessage(1, "user", "hello")], 2);

		expect(notified).toBe(false);
	});

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

		store.upsertDurableMessages(
			[withSharedCreatedAt(3, "assistant"), withSharedCreatedAt(4, "tool")],
			2,
		);
		store.upsertDurableMessages(
			[withSharedCreatedAt(1, "user"), withSharedCreatedAt(2, "assistant")],
			2,
		);

		expect(store.getSnapshot().orderedMessageIDs).toEqual([1, 2, 3, 4]);
	});

	it("does not reorder when updating an existing message in place", () => {
		const store = createChatStore();
		store.upsertDurableMessages(
			[makeMessage(1, "user", "first"), makeMessage(2, "assistant", "second")],
			2,
		);
		const orderBefore = store.getSnapshot().orderedMessageIDs;

		store.upsertDurableMessages([makeMessage(2, "assistant", "edited")], 2);

		expect(store.getSnapshot().messagesByID.get(2)?.content).toEqual([
			{ type: "text", text: "edited" },
		]);
		expect(store.getSnapshot().orderedMessageIDs).toBe(orderBefore);
	});

	it("keeps one ID for a message that appears twice", () => {
		const store = createChatStore();

		store.upsertDurableMessages(
			[
				makeMessage(1, "user", "hello"),
				makeMessage(2, "assistant", "hi"),
				makeMessage(2, "assistant", "hi"),
			],
			2,
		);

		const state = store.getSnapshot();
		expect(state.messagesByID.size).toBe(2);
		expect(state.orderedMessageIDs).toEqual([1, 2]);
	});
});

// ---------------------------------------------------------------------------
// applyServerChatStatus
// ---------------------------------------------------------------------------

describe("applyServerChatStatus", () => {
	it("keeps the turn error while the status is error", () => {
		const store = createChatStore();
		store.applyServerChatStatus("error");
		store.setTurnError({ kind: "generic", message: "provider down" });

		store.applyServerChatStatus("error");
		expect(selectShownError(store.getSnapshot())?.message).toBe(
			"provider down",
		);

		store.applyServerChatStatus("running");
		expect(selectShownError(store.getSnapshot())).toBeNull();
	});

	it("does not notify when setting the same status", () => {
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
		store.setClientError({ kind: "generic", message: "request failed" });

		store.applyServerChatStatus("running");

		expect(selectShownError(store.getSnapshot())).toMatchObject({
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
// setClientError / clearClientError
// ---------------------------------------------------------------------------

describe("setClientError / clearClientError", () => {
	it("stores and clears a stream error", () => {
		const store = createChatStore();

		store.setClientError({
			kind: "generic",
			message: "connection lost",
		});
		expect(store.getSnapshot().clientError).toEqual({
			kind: "generic",
			message: "connection lost",
		});

		store.clearClientError();
		expect(store.getSnapshot().clientError).toBeNull();
	});

	it("does not notify when setting the same error", () => {
		const store = createChatStore();
		store.setClientError({
			kind: "generic",
			message: "oops",
			detail: "Image exceeds 5 MB maximum.",
		});

		let notified = false;
		store.subscribe(() => {
			notified = true;
		});
		store.setClientError({
			kind: "generic",
			message: "oops",
			detail: "Image exceeds 5 MB maximum.",
		});

		expect(notified).toBe(false);
	});

	it("clearClientError is a no-op when already null", () => {
		const store = createChatStore();

		let notified = false;
		store.subscribe(() => {
			notified = true;
		});
		store.clearClientError();

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

		store.setQueuedMessages([qm], 2);

		expect(store.getSnapshot().queuedMessages).toEqual([qm]);
	});

	it("does not notify when queued messages are unchanged", () => {
		const store = createChatStore();
		const qm = makeQueuedMessage(10, "queued");
		store.setQueuedMessages([qm], 2);

		let notified = false;
		store.subscribe(() => {
			notified = true;
		});

		// Different object reference, same ID.
		store.setQueuedMessages([{ ...qm }], 2);

		expect(notified).toBe(false);
	});

	it("applies a snapshot that only changes editing_since or content", () => {
		const store = createChatStore();
		const qm = makeQueuedMessage(10, "queued");
		store.setQueuedMessages([qm]);

		const editing = { ...qm, editing_since: "2025-01-01T00:01:00Z" };
		store.applyAuthoritativeQueuedMessages([editing]);
		expect(store.getSnapshot().queuedMessages).toEqual([editing]);

		const edited: TypesGen.ChatQueuedMessage = {
			...editing,
			content: [{ type: "text", text: "edited" }],
		};
		store.setQueuedMessages([edited]);
		expect(store.getSnapshot().queuedMessages).toEqual([edited]);
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
	it("clears streamState, chatStatus, clientError, retryState, turnError, reconnectState, and subagentOverrides", () => {
		const store = createChatStore();
		store.applyMessagePart({ type: "text", text: "stream" });
		store.applyServerChatStatus("running");
		store.setTurnError({ kind: "generic", message: "turn failed" });
		store.setClientError({
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
		expect(state.chatStatus).toBeNull();
		expect(state.clientError).toBeNull();
		expect(state.retryState).toBeNull();
		expect(state.turnError).toBeNull();
		expect(state.reconnectState).toBeNull();
		expect(state.subagentStatusOverrides.size).toBe(0);
	});

	it("preserves messages and queued messages", () => {
		const store = createChatStore();
		store.upsertDurableMessages([makeMessage(1, "user", "hello")], 2);
		store.setQueuedMessages([makeQueuedMessage(10, "queued")], 2);
		store.setClientError({
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
		store.upsertDurableMessages([makeMessage(1, "user", "hello")], 2);

		expect(selectIsAwaitingFirstStreamChunk(store.getSnapshot())).toBe(true);
	});

	it("returns false when the latest message is from the assistant", () => {
		const store = createChatStore();
		store.applyServerChatStatus("running");
		store.upsertDurableMessages([makeMessage(1, "user", "hello")], 2);
		store.upsertDurableMessages([makeMessage(2, "assistant", "hi there")], 2);

		expect(selectIsAwaitingFirstStreamChunk(store.getSnapshot())).toBe(false);
	});

	it("returns false when stream state is present", () => {
		const store = createChatStore();
		store.applyServerChatStatus("running");
		store.upsertDurableMessages([makeMessage(1, "user", "hello")], 2);
		store.applyMessagePart({ type: "text", text: "response" });

		expect(selectIsAwaitingFirstStreamChunk(store.getSnapshot())).toBe(false);
	});

	it("returns false during waiting status when latest message is from user", () => {
		const store = createChatStore();
		store.applyServerChatStatus("waiting");
		store.upsertDurableMessages([makeMessage(1, "user", "hello")], 2);

		// "waiting" means the chat is idle; nothing is generating,
		// so no Thinking indicator should show.
		expect(selectIsAwaitingFirstStreamChunk(store.getSnapshot())).toBe(false);
	});

	it("returns false when chat status is null", () => {
		const store = createChatStore();
		store.upsertDurableMessages([makeMessage(1, "user", "hello")], 2);

		expect(selectIsAwaitingFirstStreamChunk(store.getSnapshot())).toBe(false);
	});

	it("returns true when latest message is a tool result during running", () => {
		const store = createChatStore();
		store.applyServerChatStatus("running");
		store.upsertDurableMessages([makeMessage(1, "user", "hello")], 2);
		store.upsertDurableMessages(
			[makeMessage(2, "assistant", "calling tool")],
			2,
		);
		store.upsertDurableMessages([makeMessage(3, "tool", "tool result")], 2);

		expect(selectIsAwaitingFirstStreamChunk(store.getSnapshot())).toBe(true);
	});

	it("returns true when WS delivers user message + status:running (fresh send)", () => {
		const store = createChatStore();
		// Simulate the WS batch: [message(user), status:running].
		// This is the event order from the server when the user
		// sends a message. The Thinking indicator must appear
		// before the first stream chunk arrives.
		store.upsertDurableMessages([makeMessage(1, "user", "sweet ty")], 2);
		store.applyServerChatStatus("running");
		store.clearStreamState();

		expect(selectIsAwaitingFirstStreamChunk(store.getSnapshot())).toBe(true);
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

// What a viewer sees: the transcript, the queue texts, the status and the
// error.
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
		error: selectShownError(state)?.message ?? null,
	};
};

// A user message as submitChatTurn builds it at the click.
const pendingSend = (text: string): PendingAction => ({
	kind: "send",
	message: {
		id: 0,
		chat_id: "chat-1",
		created_at: "2025-01-01T00:00:00.000Z",
		role: "user",
		content: [{ type: "text", text }],
	},
});

// Adds the action with the status the store shows, as the page does.
const addPending = (store: ChatStore, action: PendingAction) =>
	store.addPendingAction(action, store.getSnapshot().chatStatus);

describe("pending actions", () => {
	it.each(["error", "requires_action"] as const)(
		"promote from %s shows the row as the next prompt",
		(status) => {
			const store = seededStore(
				[makeMessage(1, "user", "first"), makeMessage(2, "assistant", "a")],
				[makeQueuedMessage(10, "q10"), makeQueuedMessage(11, "q11")],
				status,
			);

			addPending(store, { kind: "promoteQueued", queuedMessageID: 11 });

			expect(visible(store)).toMatchObject({
				transcript: ["user:first", "assistant:a", "user:q11"],
				queue: ["q10"],
			});
		},
	);

	it.each(["running", "interrupting"] as const)(
		"promote from %s moves the row to the head",
		(status) => {
			const store = seededStore(
				[makeMessage(1, "user", "first")],
				[makeQueuedMessage(10, "q10"), makeQueuedMessage(11, "q11")],
				status,
			);

			addPending(store, { kind: "promoteQueued", queuedMessageID: 11 });

			expect(visible(store)).toMatchObject({
				transcript: ["user:first"],
				queue: ["q11", "q10"],
			});
		},
	);

	it("keeps a pending message through stream updates without user messages", () => {
		const store = seededStore([makeMessage(1, "user", "first")], [], "waiting");
		addPending(store, pendingSend("second"));

		store.upsertDurableMessages([makeMessage(2, "assistant", "late")], 2);
		store.applyServerChatStatus("waiting");

		expect(visible(store)).toEqual({
			transcript: ["user:first", "assistant:late", "user:second"],
			queue: [],
			status: "waiting",
			error: null,
		});
	});

	it("gives the rows it shows IDs the server never assigns", () => {
		const store = seededStore(
			[makeMessage(1, "user", "first"), makeMessage(2, "user", "old")],
			[],
			"running",
		);
		addPending(store, {
			kind: "edit",
			editedMessageID: 2,
			message: makeMessage(2, "user", "new"),
		});
		addPending(store, pendingSend("queued"));

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

	it("keeps the queue when a running chat's head is promoted", async () => {
		const store = seededStore(
			[makeMessage(1, "user", "first")],
			[makeQueuedMessage(10, "q10"), makeQueuedMessage(11, "q11")],
			"running",
		);

		const before = store.getSnapshot();
		const failed = store.runPendingAction(
			{ kind: "promoteQueued", queuedMessageID: 10 },
			"running",
			async () => {
				// The head does not move, so the commit sends no queue update.
				expect(store.getSnapshot()).toBe(before);
				throw new Error("failed");
			},
		);

		await expect(failed).rejects.toThrow("failed");
		expect(store.getSnapshot()).toBe(before);
	});

	it("shows nothing for a promoted row the queue no longer holds", () => {
		const store = seededStore(
			[makeMessage(1, "user", "first")],
			[makeQueuedMessage(10, "q10")],
			"error",
		);

		const before = store.getSnapshot();
		addPending(store, { kind: "promoteQueued", queuedMessageID: 11 });

		expect(store.getSnapshot()).toBe(before);
	});

	it("stops showing a failed action", async () => {
		const store = seededStore(
			[makeMessage(1, "user", "first")],
			[makeQueuedMessage(10, "q10")],
			"running",
		);
		const request = createDeferred<undefined>();
		const run = store.runPendingAction(
			{ kind: "deleteQueued", queuedMessageID: 10 },
			"running",
			() => request.promise,
		);
		expect(visible(store).queue).toEqual([]);

		request.reject(new Error("failed"));

		await expect(run).rejects.toThrow("failed");
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
		action: pendingSend("second"),
		stream: (store) => {
			store.upsertDurableMessages([makeMessage(2, "user", "second")], 2);
		},
		statusAfter: "running",
	},
	{
		name: "send to a running chat",
		messages: [makeMessage(1, "user", "first")],
		queue: [],
		status: "running",
		action: pendingSend("q10"),
		stream: (store) =>
			store.setQueuedMessages([makeQueuedMessage(10, "q10")], 2),
		statusAfter: "running",
	},
	...(["requires_action", "interrupting"] as const).map(
		(status): ConvergenceScenario => ({
			name: `send to a ${status} chat`,
			messages: [makeMessage(1, "user", "first")],
			queue: [],
			status,
			action: pendingSend("q10"),
			stream: (store) =>
				store.setQueuedMessages([makeQueuedMessage(10, "q10")], 2),
			statusAfter: status,
		}),
	),
	{
		name: "send to an errored chat with a queue",
		messages: [makeMessage(1, "user", "first")],
		queue: [makeQueuedMessage(10, "q10")],
		status: "error",
		action: pendingSend("q11"),
		stream: (store) => {
			store.upsertDurableMessages(
				[{ ...makeMessage(2, "user", "q10"), queued_message_id: 10 }],
				2,
			);
			store.setQueuedMessages([makeQueuedMessage(11, "q11")], 2);
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
			store.resetHistoryFrom(3, 2);
			store.upsertDurableMessages([makeMessage(4, "user", "new")], 2);
			store.setQueuedMessages([], 2);
		},
		statusAfter: "running",
	},
	{
		name: "delete queued",
		messages: [makeMessage(1, "user", "first")],
		queue: [makeQueuedMessage(10, "q10"), makeQueuedMessage(11, "q11")],
		status: "running",
		action: { kind: "deleteQueued", queuedMessageID: 10 },
		stream: (store) =>
			store.setQueuedMessages([makeQueuedMessage(11, "q11")], 2),
		statusAfter: "running",
	},
	{
		name: "promote from error",
		messages: [makeMessage(1, "user", "first")],
		queue: [makeQueuedMessage(10, "q10"), makeQueuedMessage(11, "q11")],
		status: "error",
		action: { kind: "promoteQueued", queuedMessageID: 11 },
		stream: (store) => {
			store.upsertDurableMessages(
				[{ ...makeMessage(2, "user", "q11"), queued_message_id: 11 }],
				2,
			);
			store.setQueuedMessages([makeQueuedMessage(10, "q10")], 2);
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
			store.setQueuedMessages(
				[makeQueuedMessage(11, "q11"), makeQueuedMessage(10, "q10")],
				2,
			);
		},
		statusAfter: "interrupting",
	},
	{
		name: "another tab's message replaces the pending one",
		messages: [makeMessage(1, "user", "first")],
		queue: [],
		status: "waiting",
		action: pendingSend("mine"),
		stream: (store) => {
			store.upsertDurableMessages([makeMessage(2, "user", "theirs")], 2);
		},
		statusAfter: "running",
		unpredictable: true,
	},
	{
		name: "another tab's edit deletes the pending send",
		messages: [makeMessage(1, "user", "first")],
		queue: [],
		status: "waiting",
		action: pendingSend("mine"),
		stream: (store) => {
			store.resetHistoryFrom(1, 2);
			store.upsertDurableMessages([makeMessage(3, "user", "theirs")], 2);
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
		addPending(actor, scenario.action);
		const shownBeforeStream = visible(actor);

		for (const store of [actor, observer]) {
			store.batch(() => {
				scenario.stream(store);
				store.applyServerChatStatus(scenario.statusAfter);
			});
		}

		expect(actor.getSnapshot().pendingEntries).toEqual([]);
		expect(visible(actor)).toEqual(visible(observer));
		// The action showed the committed messages and queue, so nothing moves
		// when it ends. The status only ever comes from the stream.
		if (!scenario.unpredictable) {
			expect({ ...shownBeforeStream, status: null }).toEqual({
				...visible(observer),
				status: null,
			});
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
	end: (store: ChatStore, version: number) => void;
}[] = [
	{
		name: "an added message at a user message",
		status: "waiting",
		action: pendingSend("second"),
		others: (store) => {
			store.upsertDurableMessages([makeMessage(2, "assistant", "late")], 2);
			store.resetHistoryFrom(99, 2);
			store.applyServerChatStatus("running");
		},
		end: (store, version) => {
			store.upsertDurableMessages([makeMessage(3, "user", "second")], version);
		},
	},
	{
		name: "an added message at a queue update",
		status: "running",
		action: pendingSend("q10"),
		others: (store) => {
			store.upsertDurableMessages([makeMessage(2, "assistant", "late")], 2);
			store.resetHistoryFrom(99, 2);
			store.applyServerChatStatus("running");
		},
		end: (store, version) =>
			store.setQueuedMessages([makeQueuedMessage(10, "q10")], version),
	},
	{
		name: "a queue change at a queue update",
		status: "running",
		action: { kind: "deleteQueued", queuedMessageID: 10 },
		others: (store) => {
			store.upsertDurableMessages([makeMessage(2, "user", "theirs")], 2);
			store.resetHistoryFrom(99, 2);
			store.applyServerChatStatus("running");
		},
		end: (store, version) => store.setQueuedMessages([], version),
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
			store.upsertDurableMessages([makeMessage(2, "user", "theirs")], 2);
			store.setQueuedMessages([makeQueuedMessage(10, "q10")], 2);
			store.applyServerChatStatus("running");
		},
		end: (store, version) => store.resetHistoryFrom(1, version),
	},
];

describe("pending part ends", () => {
	const storeWithAction = (scenario: (typeof partEndScenarios)[number]) => {
		const store = seededStore(
			[makeMessage(1, "user", "first")],
			scenario.action.kind === "deleteQueued"
				? [makeQueuedMessage(10, "q10")]
				: [],
			scenario.status,
		);
		addPending(store, scenario.action);
		return store;
	};

	it.each(partEndScenarios)("ends $name", (scenario) => {
		const store = storeWithAction(scenario);

		scenario.others(store);
		expect(store.getSnapshot().pendingEntries).toHaveLength(1);

		scenario.end(store, 2);
		expect(store.getSnapshot().pendingEntries).toEqual([]);
	});

	// A reconnect resends the events at the synced version, which the click
	// already saw.
	it.each(partEndScenarios)(
		"does not end $name replayed from before the click",
		(scenario) => {
			const store = storeWithAction(scenario);

			scenario.end(store, 1);
			expect(store.getSnapshot().pendingEntries).toHaveLength(1);
		},
	);

	// Both promotes commit before the next sync, so one queue update reports
	// both, and the store then shows only the stream's queue.
	it("shows the stream's queue after two promotes that one update reports", () => {
		const store = seededStore(
			[makeMessage(1, "user", "first")],
			[
				makeQueuedMessage(10, "q10"),
				makeQueuedMessage(11, "q11"),
				makeQueuedMessage(12, "q12"),
			],
			"running",
		);
		addPending(store, { kind: "promoteQueued", queuedMessageID: 11 });
		addPending(store, { kind: "promoteQueued", queuedMessageID: 12 });

		store.setQueuedMessages(
			[
				makeQueuedMessage(12, "q12"),
				makeQueuedMessage(11, "q11"),
				makeQueuedMessage(10, "q10"),
			],
			2,
		);

		expect(visible(store).queue).toEqual(["q12", "q11", "q10"]);
		expect(store.getSnapshot().pendingEntries).toEqual([]);
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
		store.resetHistoryFrom(0, 2);
		store.upsertDurableMessages([makeMessage(4, "user", "new")], 2);

		store.prependOlderMessages(
			[makeMessage(1, "user", "a"), makeMessage(2, "assistant", "b")],
			false,
			resetCount,
		);

		expect(store.getSnapshot().orderedMessageIDs).toEqual([4]);
	});
});

describe("seed", () => {
	it("stores the newest page and its snapshot version", () => {
		const store = createChatStore();

		store.seed({
			messages: [makeMessage(2, "assistant", "b"), makeMessage(1, "user", "a")],
			queued_messages: [],
			has_more: true,
			snapshot_version: 4,
		});

		expect(store.getSnapshot().orderedMessageIDs).toEqual([1, 2]);
		expect(store.getSnapshot().hasMoreMessages).toBe(true);
		expect(store.getSyncedSnapshotVersion()).toBe(4);
	});
});

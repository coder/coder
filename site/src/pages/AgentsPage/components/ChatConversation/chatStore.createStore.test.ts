import { describe, expect, it, vi } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import { createDeferred } from "#/testHelpers/deferred";
import {
	type ChatStore,
	createChatStore,
	isPendingRowID,
	type PendingAction,
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
		busy_behavior: "queue",
		created_at: "2025-01-01T00:00:00Z",
		content: [{ type: "text", text }],
	}) as TypesGen.ChatQueuedMessage;

describe("upsertDurableMessages", () => {
	it("populates messagesByID and orderedMessageIDs", () => {
		const store = createChatStore();
		const msg1 = makeMessage(1, "user", "first");
		const msg2 = makeMessage(2, "assistant", "second");

		store.upsertDurableMessages([msg1, msg2]);

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

		store.upsertDurableMessages([second, first]);

		expect(store.getSnapshot().orderedMessageIDs).toEqual([1, 2]);
	});

	it("does not notify subscribers when content is unchanged", () => {
		const store = createChatStore();
		store.upsertDurableMessages([makeMessage(1, "user", "hello")]);

		let notified = false;
		store.subscribe(() => {
			notified = true;
		});

		store.upsertDurableMessages([makeMessage(1, "user", "hello")]);

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

		store.upsertDurableMessages([
			withSharedCreatedAt(3, "assistant"),
			withSharedCreatedAt(4, "tool"),
		]);
		store.upsertDurableMessages([
			withSharedCreatedAt(1, "user"),
			withSharedCreatedAt(2, "assistant"),
		]);

		expect(store.getSnapshot().orderedMessageIDs).toEqual([1, 2, 3, 4]);
	});

	it("does not reorder when updating an existing message in place", () => {
		const store = createChatStore();
		store.upsertDurableMessages([
			makeMessage(1, "user", "first"),
			makeMessage(2, "assistant", "second"),
		]);
		const orderBefore = store.getSnapshot().orderedMessageIDs;

		store.upsertDurableMessages([makeMessage(2, "assistant", "edited")]);

		expect(store.getSnapshot().messagesByID.get(2)?.content).toEqual([
			{ type: "text", text: "edited" },
		]);
		expect(store.getSnapshot().orderedMessageIDs).toBe(orderBefore);
	});

	it("keeps one ID for a message that appears twice", () => {
		const store = createChatStore();

		store.upsertDurableMessages([
			makeMessage(1, "user", "hello"),
			makeMessage(2, "assistant", "hi"),
			makeMessage(2, "assistant", "hi"),
		]);

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

		store.setQueuedMessages([qm]);

		expect(store.getSnapshot().queuedMessages).toEqual([qm]);
	});

	it("does not notify when queued messages are unchanged", () => {
		const store = createChatStore();
		const qm = makeQueuedMessage(10, "queued");
		store.setQueuedMessages([qm]);

		let notified = false;
		store.subscribe(() => {
			notified = true;
		});

		// Different object reference, same row.
		store.setQueuedMessages([{ ...qm, content: [...qm.content] }]);

		expect(notified).toBe(false);
	});

	it("applies a queue update that only changes editing_since, content or lane", () => {
		const store = createChatStore();
		const qm = makeQueuedMessage(10, "queued");
		store.setQueuedMessages([qm]);

		const editing = { ...qm, editing_since: "2025-01-01T00:01:00Z" };
		store.setQueuedMessages([editing]);
		expect(store.getSnapshot().queuedMessages).toEqual([editing]);

		const edited: TypesGen.ChatQueuedMessage = {
			...editing,
			content: [{ type: "text", text: "edited" }],
		};
		store.setQueuedMessages([edited]);
		expect(store.getSnapshot().queuedMessages).toEqual([edited]);

		const steered: TypesGen.ChatQueuedMessage = {
			...edited,
			busy_behavior: "steer",
		};
		store.setQueuedMessages([steered]);
		expect(store.getSnapshot().queuedMessages).toEqual([steered]);
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
		store.upsertDurableMessages([makeMessage(1, "user", "hello")]);
		store.setQueuedMessages([makeQueuedMessage(10, "queued")]);
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
		store.upsertDurableMessages([makeMessage(1, "user", "hello")]);

		expect(selectIsAwaitingFirstStreamChunk(store.getSnapshot())).toBe(true);
	});

	it("returns false when the latest message is from the assistant", () => {
		const store = createChatStore();
		store.applyServerChatStatus("running");
		store.upsertDurableMessages([makeMessage(1, "user", "hello")]);
		store.upsertDurableMessages([makeMessage(2, "assistant", "hi there")]);

		expect(selectIsAwaitingFirstStreamChunk(store.getSnapshot())).toBe(false);
	});

	it("returns false when stream state is present", () => {
		const store = createChatStore();
		store.applyServerChatStatus("running");
		store.upsertDurableMessages([makeMessage(1, "user", "hello")]);
		store.applyMessagePart({ type: "text", text: "response" });

		expect(selectIsAwaitingFirstStreamChunk(store.getSnapshot())).toBe(false);
	});

	it("returns false during waiting status when latest message is from user", () => {
		const store = createChatStore();
		store.applyServerChatStatus("waiting");
		store.upsertDurableMessages([makeMessage(1, "user", "hello")]);

		// "waiting" means the chat is idle; nothing is generating,
		// so no Thinking indicator should show.
		expect(selectIsAwaitingFirstStreamChunk(store.getSnapshot())).toBe(false);
	});

	it("returns false when chat status is null", () => {
		const store = createChatStore();
		store.upsertDurableMessages([makeMessage(1, "user", "hello")]);

		expect(selectIsAwaitingFirstStreamChunk(store.getSnapshot())).toBe(false);
	});

	it("returns true when latest message is a tool result during running", () => {
		const store = createChatStore();
		store.applyServerChatStatus("running");
		store.upsertDurableMessages([makeMessage(1, "user", "hello")]);
		store.upsertDurableMessages([makeMessage(2, "assistant", "calling tool")]);
		store.upsertDurableMessages([makeMessage(3, "tool", "tool result")]);

		expect(selectIsAwaitingFirstStreamChunk(store.getSnapshot())).toBe(true);
	});

	it("returns true when WS delivers user message + status:running (fresh send)", () => {
		const store = createChatStore();
		// Simulate the WS batch: [message(user), status:running].
		// This is the event order from the server when the user
		// sends a message. The Thinking indicator must appear
		// before the first stream chunk arrives.
		store.upsertDurableMessages([makeMessage(1, "user", "sweet ty")]);
		store.applyServerChatStatus("running");
		store.clearStreamState();

		expect(selectIsAwaitingFirstStreamChunk(store.getSnapshot())).toBe(true);
	});

	it("returns false when the latest shown message is a pending edit", () => {
		const store = seededStore(
			[makeMessage(1, "user", "hello"), makeMessage(2, "assistant", "hi")],
			[],
			"running",
		);
		addPending(store, {
			kind: "edit",
			editedMessageID: 1,
			content: [{ type: "text", text: "edited" }],
		});

		expect(selectIsAwaitingFirstStreamChunk(store.getSnapshot())).toBe(false);
	});
});

// ---------------------------------------------------------------------------
// Pending actions
// ---------------------------------------------------------------------------

const seededStore = (
	messages: readonly TypesGen.ChatMessage[],
	queuedMessages: readonly TypesGen.ChatQueuedMessage[],
	status: TypesGen.ChatStatus | null,
): ChatStore => {
	const store = createChatStore();
	store.seed({
		messages,
		queued_messages: queuedMessages,
		has_more: false,
		snapshot_version: 1,
	});
	if (status) {
		store.applyServerChatStatus(status);
	}
	return store;
};

const textOf = (content: readonly TypesGen.ChatMessagePart[] | undefined) => {
	const part = content?.[0];
	return part?.type === "text" ? part.text : "";
};

const mockFirstMessage = makeMessage(1, "user", "first");
const mockQueuedRow10 = makeQueuedMessage(10, "q10");
const mockQueuedRow11 = makeQueuedMessage(11, "q11");

const steerRow = (id: number, text: string): TypesGen.ChatQueuedMessage => ({
	...makeQueuedMessage(id, text),
	busy_behavior: "steer",
});

// A queued row the owner is editing. It holds the next delivery.
const underEdit = (
	row: TypesGen.ChatQueuedMessage,
): TypesGen.ChatQueuedMessage => ({
	...row,
	editing_since: "2025-01-01T00:01:00Z",
});

// The user message that the server makes from a queued row.
const delivered = (
	id: number,
	text: string,
	queuedMessageID: number,
): TypesGen.ChatMessage => ({
	...makeMessage(id, "user", text),
	queued_message_id: queuedMessageID,
});

// What a viewer sees: the transcript, and the queue with each row's lane
// and edit marker.
const visible = (store: ChatStore) => {
	const state = store.getSnapshot();
	const byID = selectMessagesByID(state);
	return {
		transcript: selectOrderedMessageIDs(state).map((id) => {
			const message = byID.get(id);
			return `${message?.role}:${textOf(message?.content)}`;
		}),
		queue: selectQueuedMessages(state).map(
			(row) =>
				`${row.busy_behavior}:${textOf(row.content)}${row.editing_since ? " (editing)" : ""}`,
		),
	};
};

// A send as submitChatTurn passes it at the click.
const pendingSend = (
	text: string,
	busyBehavior: TypesGen.ChatBusyBehavior = "steer",
): PendingAction => ({
	kind: "send",
	chatID: "chat-1",
	busyBehavior,
	content: [{ type: "text", text }],
});

// The request never settles, so only the stream ends the action.
const addPending = (store: ChatStore, action: PendingAction) =>
	void store.runPendingAction(action, () => new Promise(() => {}));

describe("pending actions", () => {
	it("gives the rows it shows IDs the server never assigns", () => {
		const store = seededStore(
			[mockFirstMessage, makeMessage(2, "user", "old")],
			[],
			"running",
		);
		addPending(store, {
			kind: "edit",
			editedMessageID: 2,
			content: [{ type: "text", text: "new" }],
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

	it("shows nothing for a promoted row the queue no longer holds", () => {
		const store = seededStore([mockFirstMessage], [mockQueuedRow10], "error");
		const before = visible(store);

		addPending(store, { kind: "promoteQueued", queuedMessageID: 11 });

		expect(visible(store)).toEqual(before);
	});

	it("stops showing a failed action", async () => {
		const store = seededStore([mockFirstMessage], [mockQueuedRow10], "running");
		const request = createDeferred<undefined>();
		const run = store.runPendingAction(
			{ kind: "deleteQueued", queuedMessageID: 10 },
			() => request.promise,
		);
		expect(visible(store).queue).toEqual([]);

		request.reject(new Error("failed"));

		await expect(run).rejects.toThrow("failed");
		expect(visible(store).queue).toEqual(["queue:q10"]);
	});

	it("a failed queued edit shows the queue as before and drops no row", async () => {
		const store = seededStore(
			[mockFirstMessage],
			[mockQueuedRow10, underEdit(mockQueuedRow11)],
			"running",
		);
		const request = createDeferred<undefined>();
		const run = store.runPendingAction(
			{ kind: "editQueued", queuedMessageID: 10, editing: true },
			() => request.promise,
		);
		expect(visible(store).queue).toEqual(["queue:q10 (editing)", "queue:q11"]);

		request.reject(new Error("not found"));

		await expect(run).rejects.toThrow("not found");
		expect(visible(store).queue).toEqual(["queue:q10", "queue:q11 (editing)"]);
	});

	it("shows each delete until the stored queue lacks its row", () => {
		const mockQueuedRow12 = makeQueuedMessage(12, "q12");
		const store = seededStore(
			[mockFirstMessage],
			[mockQueuedRow10, mockQueuedRow11, mockQueuedRow12],
			"running",
		);
		addPending(store, { kind: "deleteQueued", queuedMessageID: 10 });
		addPending(store, { kind: "deleteQueued", queuedMessageID: 11 });

		store.setQueuedMessages([mockQueuedRow11, mockQueuedRow12]);
		expect(visible(store).queue).toEqual(["queue:q12"]);

		store.setQueuedMessages([mockQueuedRow12]);
		expect(visible(store).queue).toEqual(["queue:q12"]);
		expect(store.getSnapshot().pendingActions).toEqual([]);
	});

	it("ends pending sends oldest first and runs their requests one at a time", async () => {
		const store = seededStore([mockFirstMessage], [], "waiting");
		const first = createDeferred<undefined>();
		const firstRequest = vi.fn(() => first.promise);
		const secondRequest = vi.fn(() => Promise.resolve());
		const firstRun = store.runPendingAction(pendingSend("one"), firstRequest);
		const secondRun = store.runPendingAction(pendingSend("two"), secondRequest);
		await Promise.resolve();
		expect(firstRequest).toHaveBeenCalledOnce();
		expect(secondRequest).not.toHaveBeenCalled();

		store.upsertDurableMessages([makeMessage(2, "user", "one")]);
		const state = store.getSnapshot();
		expect(
			selectOrderedMessageIDs(state).map((id) => [
				textOf(selectMessagesByID(state).get(id)?.content),
				isPendingRowID(id),
			]),
		).toEqual([
			["first", false],
			["one", false],
			["two", true],
		]);

		first.resolve(undefined);
		await firstRun;
		await secondRun;
		expect(secondRequest).toHaveBeenCalledOnce();
	});

	it("runs the next request after a request rejects", async () => {
		const store = seededStore(
			[mockFirstMessage],
			[mockQueuedRow10, mockQueuedRow11],
			"running",
		);
		const failed = store.runPendingAction(
			{ kind: "deleteQueued", queuedMessageID: 10 },
			() => Promise.reject(new Error("failed")),
		);
		const nextRequest = vi.fn(() => Promise.resolve());
		const nextRun = store.runPendingAction(
			{ kind: "deleteQueued", queuedMessageID: 11 },
			nextRequest,
		);

		await expect(failed).rejects.toThrow("failed");
		await nextRun;
		expect(nextRequest).toHaveBeenCalledOnce();
		expect(visible(store).queue).toEqual(["queue:q10"]);
	});

	it.each<PendingAction>([
		{ kind: "promoteQueued", queuedMessageID: 11 },
		{ kind: "editQueued", queuedMessageID: 11, editing: true },
	])(
		"keeps a pending $kind through a queue update from before it",
		(action) => {
			const store = seededStore(
				[mockFirstMessage],
				[mockQueuedRow10, mockQueuedRow11],
				"running",
			);
			addPending(store, action);
			const shown = visible(store);

			store.setQueuedMessages([mockQueuedRow10, mockQueuedRow11]);

			expect(visible(store)).toEqual(shown);
		},
	);

	it("hides a promoted row once the stream stores its message", () => {
		const store = seededStore(
			[mockFirstMessage],
			[mockQueuedRow10, mockQueuedRow11],
			"error",
		);
		addPending(store, { kind: "promoteQueued", queuedMessageID: 11 });

		store.upsertDurableMessages([delivered(2, "q11", 11)]);

		expect(visible(store)).toEqual({
			transcript: ["user:first", "user:q11"],
			queue: ["queue:q10"],
		});
	});
});

// Each scenario applies the server's change for the action to a store that
// acted and to one that did not. Both show the same chat after it, and the
// acting store shows the whole change from the click unless partial.
type PlacementScenario = {
	name: string;
	messages?: readonly TypesGen.ChatMessage[];
	queue: readonly TypesGen.ChatQueuedMessage[];
	status: TypesGen.ChatStatus | null;
	action: PendingAction;
	// The stream events of the server's change.
	stream: (store: ChatStore) => void;
	// The click shows only the change to the acted-on row or message: the
	// server also changes other rows, or another viewer's change lands first.
	partial?: boolean;
};

const placementScenarios: PlacementScenario[] = [
	...([null, "waiting", "error"] as const).map(
		(status): PlacementScenario => ({
			name: `send with status ${status} and no queue`,
			queue: [],
			status,
			action: pendingSend("second"),
			stream: (store) =>
				store.upsertDurableMessages([makeMessage(2, "user", "second")]),
		}),
	),
	...(["running", "requires_action", "interrupting"] as const).map(
		(status): PlacementScenario => ({
			name: `send with status ${status}`,
			queue: [mockQueuedRow10],
			status,
			action: pendingSend("s11"),
			stream: (store) =>
				store.setQueuedMessages([steerRow(11, "s11"), mockQueuedRow10]),
		}),
	),
	{
		name: "send after turn to a running chat",
		queue: [steerRow(10, "s10")],
		status: "running",
		action: pendingSend("q11", "queue"),
		stream: (store) =>
			store.setQueuedMessages([steerRow(10, "s10"), mockQueuedRow11]),
	},
	{
		name: "send to an errored chat with a queue",
		queue: [mockQueuedRow10],
		status: "error",
		action: pendingSend("s11"),
		stream: (store) => {
			store.upsertDurableMessages([delivered(2, "s11", 11)]);
			store.setQueuedMessages([mockQueuedRow10]);
		},
	},
	{
		name: "send after turn to an errored chat with a queue",
		queue: [mockQueuedRow10],
		status: "error",
		action: pendingSend("q11", "queue"),
		stream: (store) => {
			store.upsertDurableMessages([delivered(2, "q10", 10)]);
			store.setQueuedMessages([mockQueuedRow11]);
		},
		partial: true,
	},
	{
		name: "send to a paused chat",
		queue: [underEdit(mockQueuedRow10)],
		status: "paused",
		action: pendingSend("s11"),
		stream: (store) => store.upsertDurableMessages([delivered(2, "s11", 11)]),
	},
	{
		name: "send after turn to a paused chat",
		queue: [underEdit(mockQueuedRow10)],
		status: "paused",
		action: pendingSend("q11", "queue"),
		stream: (store) =>
			store.setQueuedMessages([underEdit(mockQueuedRow10), mockQueuedRow11]),
	},
	{
		name: "send to an errored chat whose steer row is under edit",
		queue: [underEdit(steerRow(10, "s10"))],
		status: "error",
		action: pendingSend("s11"),
		stream: (store) =>
			store.setQueuedMessages([
				underEdit(steerRow(10, "s10")),
				steerRow(11, "s11"),
			]),
	},
	{
		name: "edit",
		messages: [
			mockFirstMessage,
			makeMessage(2, "assistant", "a"),
			makeMessage(3, "user", "old"),
		],
		queue: [mockQueuedRow10],
		status: "running",
		action: {
			kind: "edit",
			editedMessageID: 3,
			content: [{ type: "text", text: "new" }],
		},
		stream: (store) => {
			store.resetHistoryFrom(3, 2);
			store.upsertDurableMessages([makeMessage(4, "user", "new")]);
			store.setQueuedMessages([]);
		},
		partial: true,
	},
	...(["running", "error"] as const).map(
		(status): PlacementScenario => ({
			name: `delete queued with status ${status}`,
			queue: [mockQueuedRow10, mockQueuedRow11],
			status,
			action: { kind: "deleteQueued", queuedMessageID: 10 },
			stream: (store) => store.setQueuedMessages([mockQueuedRow11]),
		}),
	),
	{
		name: "delete the row that holds a paused chat",
		queue: [underEdit(mockQueuedRow10), mockQueuedRow11],
		status: "paused",
		action: { kind: "deleteQueued", queuedMessageID: 10 },
		stream: (store) => {
			store.upsertDurableMessages([delivered(2, "q11", 11)]);
			store.setQueuedMessages([]);
		},
		partial: true,
	},
	...(["running", "requires_action"] as const).map(
		(status): PlacementScenario => ({
			name: `promote with status ${status}`,
			queue: [steerRow(9, "s9"), mockQueuedRow10, mockQueuedRow11],
			status,
			action: { kind: "promoteQueued", queuedMessageID: 11 },
			stream: (store) =>
				store.setQueuedMessages([
					steerRow(9, "s9"),
					steerRow(11, "q11"),
					mockQueuedRow10,
				]),
		}),
	),
	{
		name: "promote a row under edit in a running chat",
		queue: [mockQueuedRow10, underEdit(mockQueuedRow11)],
		status: "running",
		action: { kind: "promoteQueued", queuedMessageID: 11 },
		stream: (store) =>
			store.setQueuedMessages([steerRow(11, "q11"), mockQueuedRow10]),
	},
	{
		name: "promote in an errored chat",
		queue: [steerRow(9, "s9"), mockQueuedRow10, mockQueuedRow11],
		status: "error",
		action: { kind: "promoteQueued", queuedMessageID: 11 },
		stream: (store) => {
			store.upsertDurableMessages([
				delivered(2, "s9", 9),
				delivered(3, "q11", 11),
			]);
			store.setQueuedMessages([mockQueuedRow10]);
		},
		partial: true,
	},
	{
		name: "promote in a paused chat",
		queue: [underEdit(mockQueuedRow10), mockQueuedRow11],
		status: "paused",
		action: { kind: "promoteQueued", queuedMessageID: 11 },
		stream: (store) => {
			store.upsertDurableMessages([delivered(2, "q11", 11)]);
			store.setQueuedMessages([underEdit(mockQueuedRow10)]);
		},
		partial: true,
	},
	{
		name: "begin a queued edit while another row is under edit",
		queue: [mockQueuedRow10, underEdit(mockQueuedRow11)],
		status: "running",
		action: { kind: "editQueued", queuedMessageID: 10, editing: true },
		stream: (store) =>
			store.setQueuedMessages([underEdit(mockQueuedRow10), mockQueuedRow11]),
	},
	...(["running", "error"] as const).map(
		(status): PlacementScenario => ({
			name: `end a queued edit with status ${status}`,
			queue: [underEdit(mockQueuedRow10)],
			status,
			action: { kind: "editQueued", queuedMessageID: 10, editing: false },
			stream: (store) => store.setQueuedMessages([mockQueuedRow10]),
		}),
	),
	{
		name: "save a queued edit",
		queue: [underEdit(mockQueuedRow10)],
		status: "running",
		action: {
			kind: "editQueued",
			queuedMessageID: 10,
			editing: false,
			content: [{ type: "text", text: "q10 saved" }],
		},
		stream: (store) =>
			store.setQueuedMessages([makeQueuedMessage(10, "q10 saved")]),
	},
	...[undefined, "q10 saved"].map(
		(saved): PlacementScenario => ({
			name: `${saved ? "save" : "end"} the edit that holds a paused chat`,
			queue: [underEdit(mockQueuedRow10), mockQueuedRow11],
			status: "paused",
			action: {
				kind: "editQueued",
				queuedMessageID: 10,
				editing: false,
				content: saved ? [{ type: "text", text: saved }] : undefined,
			},
			stream: (store) => {
				store.upsertDurableMessages([delivered(2, saved ?? "q10", 10)]);
				store.setQueuedMessages([mockQueuedRow11]);
			},
			partial: true,
		}),
	),
	{
		name: "another tab's message replaces the pending one",
		queue: [],
		status: "waiting",
		action: pendingSend("mine"),
		stream: (store) =>
			store.upsertDurableMessages([makeMessage(2, "user", "theirs")]),
		partial: true,
	},
	{
		name: "another tab's edit deletes the pending send",
		queue: [],
		status: "waiting",
		action: pendingSend("mine"),
		stream: (store) => {
			store.resetHistoryFrom(1, 2);
			store.upsertDurableMessages([makeMessage(3, "user", "theirs")]);
		},
		partial: true,
	},
];

describe("pending actions show the server's change", () => {
	it.each(placementScenarios)("$name", (scenario) => {
		const messages = scenario.messages ?? [mockFirstMessage];
		const actor = seededStore(messages, scenario.queue, scenario.status);
		const observer = seededStore(messages, scenario.queue, scenario.status);
		addPending(actor, scenario.action);
		const shownAtClick = visible(actor);

		for (const store of [actor, observer]) {
			store.batch(() => scenario.stream(store));
		}

		expect(visible(actor)).toEqual(visible(observer));
		if (!scenario.partial) {
			expect(shownAtClick).toEqual(visible(observer));
		}
	});
});

// An action outlives the events in others and ends at end.
const endScenarios: {
	name: string;
	queue: readonly TypesGen.ChatQueuedMessage[];
	status: TypesGen.ChatStatus;
	action: PendingAction;
	others: (store: ChatStore) => void;
	end: (store: ChatStore) => void;
}[] = [
	{
		name: "a send at a new user message",
		queue: [],
		status: "waiting",
		action: pendingSend("mine"),
		others: (store) => {
			store.upsertDurableMessages([
				mockFirstMessage,
				makeMessage(2, "assistant", "late"),
			]);
			store.resetHistoryFrom(99, 2);
		},
		end: (store) =>
			store.upsertDurableMessages([makeMessage(3, "user", "mine")]),
	},
	{
		name: "a send at a new queued row",
		queue: [mockQueuedRow10],
		status: "running",
		action: pendingSend("mine"),
		others: (store) => {
			store.setQueuedMessages([mockQueuedRow10]);
			store.upsertDurableMessages([delivered(2, "q10", 10)]);
		},
		end: (store) => store.setQueuedMessages([steerRow(11, "mine")]),
	},
	{
		name: "a promote at the queue update that moves the row to steer",
		queue: [mockQueuedRow10, mockQueuedRow11],
		status: "running",
		action: { kind: "promoteQueued", queuedMessageID: 11 },
		others: (store) => {
			store.upsertDurableMessages([makeMessage(2, "user", "theirs")]);
			store.setQueuedMessages([
				mockQueuedRow10,
				mockQueuedRow11,
				makeQueuedMessage(12, "q12"),
			]);
		},
		end: (store) =>
			store.setQueuedMessages([
				steerRow(11, "q11"),
				mockQueuedRow10,
				makeQueuedMessage(12, "q12"),
			]),
	},
	{
		name: "a delete at the queue update without the row",
		queue: [mockQueuedRow10, mockQueuedRow11],
		status: "running",
		action: { kind: "deleteQueued", queuedMessageID: 10 },
		others: (store) => {
			store.upsertDurableMessages([makeMessage(2, "user", "theirs")]);
			store.setQueuedMessages([mockQueuedRow10, mockQueuedRow11]);
		},
		end: (store) => store.setQueuedMessages([mockQueuedRow11]),
	},
	{
		name: "a queued edit at the queue update that marks the row",
		queue: [mockQueuedRow10],
		status: "running",
		action: { kind: "editQueued", queuedMessageID: 10, editing: true },
		others: (store) =>
			store.setQueuedMessages([mockQueuedRow10, makeQueuedMessage(12, "q12")]),
		end: (store) =>
			store.setQueuedMessages([
				underEdit(mockQueuedRow10),
				makeQueuedMessage(12, "q12"),
			]),
	},
	{
		name: "an edit at its new message",
		queue: [mockQueuedRow10],
		status: "waiting",
		action: {
			kind: "edit",
			editedMessageID: 1,
			content: [{ type: "text", text: "new" }],
		},
		others: (store) => {
			store.resetHistoryFrom(1, 2);
			store.setQueuedMessages([]);
		},
		end: (store) =>
			store.upsertDurableMessages([makeMessage(2, "user", "new")]),
	},
];

describe("pending action ends", () => {
	it.each(endScenarios)("ends $name", (scenario) => {
		const actor = seededStore(
			[mockFirstMessage],
			scenario.queue,
			scenario.status,
		);
		const observer = seededStore(
			[mockFirstMessage],
			scenario.queue,
			scenario.status,
		);
		addPending(actor, scenario.action);

		for (const store of [actor, observer]) {
			scenario.others(store);
		}
		expect(visible(actor)).not.toEqual(visible(observer));

		for (const store of [actor, observer]) {
			scenario.end(store);
		}
		expect(visible(actor)).toEqual(visible(observer));
	});
});

describe("resetHistoryFrom", () => {
	it("ignores a reset that a reconnect resends after the store applied it", () => {
		const store = createChatStore();
		store.seed({
			messages: [makeMessage(3, "user", "c"), makeMessage(10, "user", "old")],
			queued_messages: [],
			has_more: true,
			snapshot_version: 5,
		});
		store.resetHistoryFrom(10, 6);
		store.upsertDurableMessages([makeMessage(11, "user", "edited")]);

		store.resetHistoryFrom(10, 6);

		expect(store.getSnapshot().orderedMessageIDs).toEqual([3, 11]);
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

import { describe, expect, it, vi } from "vitest";
import type { ChatMessage, ChatQueuedMessage } from "#/api/typesGenerated";
import {
	MockChatMessage,
	MockChatQueuedMessage,
} from "#/testHelpers/chatEntities";
import {
	buildInactiveChatQueueReconciliation,
	reconcilePromotedQueueHead,
	restoreOptimisticRequestSnapshot,
	runPromoteQueuedMessage,
	settlePromotedQueueHead,
	submitEdit,
} from "./chatQueueReconciliation";
import { createChatStore } from "./chatStore";

describe("restoreOptimisticRequestSnapshot", () => {
	it("restores queued messages, stream output, status, and stream error", () => {
		const store = createChatStore();
		store.setQueuedMessages([
			{
				id: 9,
				chat_id: "chat-abc-123",
				created_at: "2025-01-01T00:00:00.000Z",
				content: [{ type: "text" as const, text: "queued" }],
			},
		]);
		store.setChatStatus("running");
		store.applyMessagePart({ type: "text", text: "partial response" });
		store.setStreamError({ kind: "generic", message: "old error" });
		const previousSnapshot = store.getSnapshot();

		store.batch(() => {
			store.setQueuedMessages([]);
			store.setChatStatus("waiting");
			store.clearStreamState();
			store.clearStreamError();
		});

		restoreOptimisticRequestSnapshot(store, previousSnapshot);

		const restoredSnapshot = store.getSnapshot();
		expect(restoredSnapshot.queuedMessages).toEqual(
			previousSnapshot.queuedMessages,
		);
		expect(restoredSnapshot.chatStatus).toBe(previousSnapshot.chatStatus);
		expect(restoredSnapshot.streamState).toBe(previousSnapshot.streamState);
		expect(restoredSnapshot.streamError).toEqual(previousSnapshot.streamError);
	});
});

describe("runPromoteQueuedMessage", () => {
	const buildQueuedMessage = (
		id: number,
		text: string,
		chatID = "chat-1",
	): ChatQueuedMessage => ({
		...MockChatQueuedMessage,
		id,
		chat_id: chatID,
		content: [{ type: "text", text }],
	});

	it("suppresses the promoted ID and removes it optimistically", async () => {
		const store = createChatStore();
		const a = buildQueuedMessage(1, "A");
		const b = buildQueuedMessage(2, "B");
		const c = buildQueuedMessage(3, "C");
		store.setQueuedMessages([a, b, c]);
		store.setChatStatus("running");

		const promote = vi.fn(async (_id: number) => undefined);
		const clearChatErrorReason = vi.fn();
		const onError = vi.fn();

		await runPromoteQueuedMessage({
			id: b.id,
			store,
			promoteQueuedMessage: promote,
			agentId: "chat-1",
			clearChatErrorReason,
			onError,
		});

		expect(promote).toHaveBeenCalledWith(b.id);

		const snapshot = store.getSnapshot();
		expect(snapshot.queuedMessages.map((m) => m.id)).toEqual([a.id, c.id]);
		expect(snapshot.suppressedQueuedMessageIDs.has(b.id)).toBe(true);
		expect(snapshot.chatStatus).toBe("running");
	});

	it("rolls back queue and status, clears suppression, and rethrows on API error", async () => {
		const store = createChatStore();
		const a = buildQueuedMessage(1, "A");
		const b = buildQueuedMessage(2, "B");
		store.setQueuedMessages([a, b]);
		store.setChatStatus("waiting");

		const apiError = new Error("boom");
		const promote = vi.fn(async (_id: number) => {
			throw apiError;
		});
		const clearChatErrorReason = vi.fn();
		const onError = vi.fn();

		await expect(
			runPromoteQueuedMessage({
				id: b.id,
				store,
				promoteQueuedMessage: promote,
				agentId: "chat-1",
				clearChatErrorReason,
				onError,
			}),
		).rejects.toBe(apiError);

		expect(onError).toHaveBeenCalledWith(apiError);

		const snapshot = store.getSnapshot();
		expect(snapshot.queuedMessages.map((m) => m.id)).toEqual([a.id, b.id]);
		expect(snapshot.chatStatus).toBe("waiting");
		expect(snapshot.suppressedQueuedMessageIDs.has(b.id)).toBe(false);
	});
});

describe("reconcilePromotedQueueHead", () => {
	const buildQueuedMessage = (id: number, text: string): ChatQueuedMessage => ({
		...MockChatQueuedMessage,
		id,
		content: [{ type: "text", text }],
	});
	const userMessage: ChatMessage = { ...MockChatMessage, id: 10, role: "user" };
	const toolMessage: ChatMessage = { ...MockChatMessage, id: 9, role: "tool" };

	it("suppresses the captured head and appends the queued tail", () => {
		const store = createChatStore();
		const a = buildQueuedMessage(1, "A");
		const b = buildQueuedMessage(2, "B");
		const tail = buildQueuedMessage(3, "C");
		store.setQueuedMessages([a, b]);

		const reconciled = reconcilePromotedQueueHead(
			store,
			[toolMessage, userMessage],
			a.id,
			tail,
		);

		const snapshot = store.getSnapshot();
		expect(snapshot.queuedMessages.map((m) => m.id)).toEqual([b.id, tail.id]);
		expect(snapshot.suppressedQueuedMessageIDs.has(a.id)).toBe(true);
		expect(reconciled?.map((m) => m.id)).toEqual([b.id, tail.id]);
	});

	it("does not suppress the rotated head when a queue_update already applied", () => {
		const store = createChatStore();
		const a = buildQueuedMessage(1, "A");
		const b = buildQueuedMessage(2, "B");
		const c = buildQueuedMessage(3, "C");
		store.setQueuedMessages([b, c]);

		reconcilePromotedQueueHead(store, [userMessage], a.id, c);

		const snapshot = store.getSnapshot();
		expect(snapshot.queuedMessages.map((m) => m.id)).toEqual([b.id, c.id]);
		expect(snapshot.suppressedQueuedMessageIDs.has(a.id)).toBe(true);
		expect(snapshot.suppressedQueuedMessageIDs.has(b.id)).toBe(false);
		expect(snapshot.suppressedQueuedMessageIDs.has(c.id)).toBe(false);

		store.applyAuthoritativeQueuedMessages([a, b, c]);
		expect(store.getSnapshot().queuedMessages.map((m) => m.id)).toEqual([
			b.id,
			c.id,
		]);
		store.applyAuthoritativeQueuedMessages([b, c]);
		expect(store.getSnapshot().suppressedQueuedMessageIDs.size).toBe(0);
	});

	it("keeps the response tail when a stale snapshot arrived mid-request", () => {
		const store = createChatStore();
		const a = buildQueuedMessage(1, "A");
		const b = buildQueuedMessage(2, "B");
		const c = buildQueuedMessage(3, "C");
		// A pre-send snapshot lands while the POST is in flight; it cannot
		// mention the tail the send just created.
		store.setQueuedMessages([a]);
		store.applyAuthoritativeQueuedMessages([a, b]);

		const next = reconcilePromotedQueueHead(store, [userMessage], a.id, c);

		expect(next?.map((m) => m.id)).toEqual([b.id, c.id]);
	});

	it("drops the response tail once the server reported and removed it", () => {
		const store = createChatStore();
		const a = buildQueuedMessage(1, "A");
		const c = buildQueuedMessage(3, "C");
		store.applyAuthoritativeQueuedMessages([a, c]);
		store.applyAuthoritativeQueuedMessages([a]);

		const next = reconcilePromotedQueueHead(store, [userMessage], a.id, c);

		expect(next).toEqual([]);
	});

	it("omits the response tail when a newer queue update was observed", () => {
		const store = createChatStore();
		const a = buildQueuedMessage(1, "A");
		store.setQueuedMessages([a]);

		const next = reconcilePromotedQueueHead(
			store,
			[userMessage],
			a.id,
			undefined,
		);

		expect(next).toEqual([]);
		expect(store.getSnapshot().queuedMessages).toEqual([]);
	});

	it("does nothing when no user row was inserted", () => {
		const store = createChatStore();
		const a = buildQueuedMessage(1, "A");
		store.setQueuedMessages([a]);

		const reconciled = reconcilePromotedQueueHead(
			store,
			[toolMessage],
			a.id,
			buildQueuedMessage(2, "B"),
		);

		const snapshot = store.getSnapshot();
		expect(snapshot.queuedMessages.map((m) => m.id)).toEqual([a.id]);
		expect(snapshot.suppressedQueuedMessageIDs.size).toBe(0);
		expect(reconciled).toBeUndefined();
	});

	it("does nothing when no head was captured before the send", () => {
		const store = createChatStore();

		const reconciled = reconcilePromotedQueueHead(
			store,
			[userMessage],
			undefined,
			buildQueuedMessage(1, "A"),
		);

		const snapshot = store.getSnapshot();
		expect(snapshot.queuedMessages).toEqual([]);
		expect(snapshot.suppressedQueuedMessageIDs.size).toBe(0);
		expect(reconciled).toBeUndefined();
	});
});

describe("buildInactiveChatQueueReconciliation", () => {
	const buildQueuedMessage = (id: number, text: string): ChatQueuedMessage => ({
		...MockChatQueuedMessage,
		id,
		content: [{ type: "text", text }],
	});
	const userMessage: ChatMessage = {
		...MockChatMessage,
		id: 42,
		role: "user",
	};

	it("keeps a message queued while the send was in flight", () => {
		const a = buildQueuedMessage(1, "A");
		const b = buildQueuedMessage(2, "B");
		const c = buildQueuedMessage(3, "C");

		const next = buildInactiveChatQueueReconciliation(
			[a, b, c],
			[a, b],
			[userMessage],
			a.id,
			undefined,
		);

		expect(next?.map((m) => m.id)).toEqual([b.id, c.id]);
	});

	it("falls back to the pre-send queue when nothing is cached", () => {
		const a = buildQueuedMessage(1, "A");
		const b = buildQueuedMessage(2, "B");

		const next = buildInactiveChatQueueReconciliation(
			undefined,
			[a, b],
			[userMessage],
			a.id,
			undefined,
		);

		expect(next?.map((m) => m.id)).toEqual([b.id]);
	});
});

describe("settlePromotedQueueHead", () => {
	const buildQueuedMessage = (id: number, text: string): ChatQueuedMessage => ({
		...MockChatQueuedMessage,
		id,
		content: [{ type: "text", text }],
	});
	const chatID = "chat-abc-123";

	it("restores a head the server still has queued", async () => {
		const store = createChatStore();
		const a = buildQueuedMessage(1, "A");
		const b = buildQueuedMessage(2, "B");
		store.setActiveChatID(chatID);
		store.setQueuedMessages([b]);
		store.markQueuedMessagePromoted(a.id);

		const settled = await settlePromotedQueueHead(
			store,
			chatID,
			a.id,
			async () => ({ messages: [], has_more: false, queued_messages: [a, b] }),
		);

		expect(settled?.map((m) => m.id)).toEqual([a.id, b.id]);
		expect(store.getSnapshot().queuedMessages.map((m) => m.id)).toEqual([
			a.id,
			b.id,
		]);
	});

	it("leaves the queue alone when the fetch fails", async () => {
		const store = createChatStore();
		const a = buildQueuedMessage(1, "A");
		const b = buildQueuedMessage(2, "B");
		store.setActiveChatID(chatID);
		store.setQueuedMessages([b]);
		store.markQueuedMessagePromoted(a.id);

		const settled = await settlePromotedQueueHead(store, chatID, a.id, () =>
			Promise.reject(new Error("offline")),
		);

		expect(settled).toBeUndefined();
		expect(store.getSnapshot().queuedMessages.map((m) => m.id)).toEqual([b.id]);
		expect(store.getSnapshot().promotedQueuedMessageIDs.has(a.id)).toBe(true);
	});

	it("returns the filtered queue the store applied", async () => {
		const store = createChatStore();
		const a = buildQueuedMessage(1, "A");
		const b = buildQueuedMessage(2, "B");
		const c = buildQueuedMessage(3, "C");
		store.setActiveChatID(chatID);
		store.setQueuedMessages([b]);
		store.markQueuedMessagePromoted(a.id);
		// An overlapping explicit promotion suppresses C, which the server has
		// not deleted yet, so the caller must not cache it back.
		store.suppressQueuedMessageID(c.id);

		const settled = await settlePromotedQueueHead(
			store,
			chatID,
			a.id,
			async () => ({
				messages: [],
				has_more: false,
				queued_messages: [a, b, c],
			}),
		);

		expect(settled?.map((m) => m.id)).toEqual([a.id, b.id]);
	});

	it("discards a response that resolves after navigating to another chat", async () => {
		const store = createChatStore();
		const a = buildQueuedMessage(1, "A");
		const b = buildQueuedMessage(2, "B");
		store.setActiveChatID(chatID);
		store.setQueuedMessages([b]);
		store.markQueuedMessagePromoted(a.id);

		const settled = await settlePromotedQueueHead(
			store,
			chatID,
			a.id,
			async () => {
				store.setActiveChatID("chat-other");
				store.setQueuedMessages([]);
				return { messages: [], has_more: false, queued_messages: [a, b] };
			},
		);

		expect(settled).toBeUndefined();
		expect(store.getSnapshot().queuedMessages).toEqual([]);
	});
});

describe("submitEdit", () => {
	const dummyArgs = {
		messageId: 42,
		req: { content: [{ type: "text" as const, text: "edited" }] },
	};

	it("awaits editMessage", async () => {
		const editMessage = vi.fn().mockResolvedValue(undefined);

		await submitEdit({
			editMessage,
			editArgs: dummyArgs,
			onError: vi.fn(),
		});

		expect(editMessage).toHaveBeenCalledWith(dummyArgs);
	});

	it("reports and rethrows an editMessage failure", async () => {
		const onError = vi.fn();
		const editMessage = vi.fn().mockRejectedValue(new Error("boom"));

		await expect(
			submitEdit({
				editMessage,
				editArgs: dummyArgs,
				onError,
			}),
		).rejects.toThrow("boom");

		expect(onError).toHaveBeenCalledWith(
			expect.objectContaining({ message: "boom" }),
		);
	});
});

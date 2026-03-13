import type * as TypesGen from "api/typesGenerated";
import { describe, expect, it } from "vitest";
import { createChatStore } from "./ChatContext";

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
// setStreamError / clearStreamError
// ---------------------------------------------------------------------------

describe("setStreamError / clearStreamError", () => {
	it("stores and clears a stream error", () => {
		const store = createChatStore();

		store.setStreamError("connection lost");
		expect(store.getSnapshot().streamError).toBe("connection lost");

		store.clearStreamError();
		expect(store.getSnapshot().streamError).toBeNull();
	});

	it("does not notify when setting the same error", () => {
		const store = createChatStore();
		store.setStreamError("oops");

		let notified = false;
		store.subscribe(() => {
			notified = true;
		});
		store.setStreamError("oops");

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

		store.setRetryState({ attempt: 1, error: "rate limited" });
		expect(store.getSnapshot().retryState).toEqual({
			attempt: 1,
			error: "rate limited",
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
	it("clears streamState, streamError, retryState, and subagentOverrides", () => {
		const store = createChatStore();
		store.applyMessagePart({ type: "text", text: "stream" });
		store.setStreamError("oops");
		store.setRetryState({ attempt: 2, error: "rate limit" });
		store.setSubagentStatusOverride("sub-1", "error");

		store.resetTransientState();

		const state = store.getSnapshot();
		expect(state.streamState).toBeNull();
		expect(state.streamError).toBeNull();
		expect(state.retryState).toBeNull();
		expect(state.subagentStatusOverrides.size).toBe(0);
	});

	it("preserves messages and queued messages", () => {
		const store = createChatStore();
		store.replaceMessages([makeMessage(1, "user", "hello")]);
		store.setQueuedMessages([makeQueuedMessage(10, "queued")]);
		store.setStreamError("oops");

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
// Bug A: Parts dropped during "pending" status
//
// In production, the useChatStore hook defines a closure called
// shouldApplyMessagePart() (ChatContext.ts lines 623-626) that returns
// false when chatStatus is "pending" or "waiting". When it returns false,
// message_part stream events are silently dropped (line 656-658 `continue`)
// and never queued, buffered, or replayed. There is no recovery path.
// ---------------------------------------------------------------------------

describe("Bug A: parts arriving during pending status are lost", () => {
	it("store.applyMessagePart accepts parts regardless of chatStatus", () => {
		// Documents that the store itself has no status guard.
		// The filtering is solely in the hook's shouldApplyMessagePart
		// closure, not in the store.
		const store = createChatStore();
		store.setChatStatus("pending");

		store.applyMessagePart({ type: "text", text: "during pending" });

		expect(store.getSnapshot().streamState?.blocks).toEqual([
			{ type: "response", text: "during pending" },
		]);
	});

	it("parts skipped by the hook during pending are gone forever", () => {
		// Models the production sequence where shouldApplyMessagePart()
		// drops message_part events during "pending" status.
		const store = createChatStore();
		store.setChatStatus("pending");

		// In production, these parts arrive as message_part stream
		// events. The hook checks shouldApplyMessagePart(), which
		// returns false because chatStatus is "pending". The parts
		// are skipped with `continue` and never forwarded to the
		// store. We model this by not calling applyMessagePart.
		const _droppedByHook = [
			{ type: "text", text: "Hello " },
			{
				type: "tool-call",
				tool_name: "execute",
				tool_call_id: "tc-1",
				args: { command: "ls" },
			},
		];

		// Status transitions to "running".
		store.setChatStatus("running");

		// Parts arriving after the transition are applied normally.
		store.applyMessagePart({ type: "text", text: "world" });

		const state = store.getSnapshot();

		// CORRECT BEHAVIOR: All parts should be present, including
		// those from the pending phase. The text should be
		// "Hello world" and the tool call tc-1 should exist.
		//
		// FAILS: The text is just "world" because the hook dropped
		// "Hello " during pending. The store has no queue or
		// replay mechanism to recover these parts.
		expect(state.streamState?.blocks).toEqual(
			expect.arrayContaining([
				expect.objectContaining({ type: "response", text: "Hello world" }),
			]),
		);
	});
});

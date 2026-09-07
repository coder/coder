import { act, renderHook } from "@testing-library/react";
import { createRef } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type {
	ChatMessage,
	ChatQueuedMessage,
	Workspace,
	WorkspaceApp,
} from "#/api/typesGenerated";
import {
	MockChatMessage,
	MockChatQueuedMessage,
} from "#/testHelpers/chatEntities";
import { createDeferred } from "#/testHelpers/deferred";
import {
	MockUserOwner,
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceApp,
} from "#/testHelpers/entities";
import {
	buildInactiveChatQueueReconciliation,
	getPersistedDraftInputValue,
	getWorkspaceOptionsWithLinkedWorkspace,
	isChatAgentBindingUnresolved,
	isWatchedWorkspaceViewUnchanged,
	reconcilePromotedQueueHead,
	restoreOptimisticRequestSnapshot,
	runPromoteQueuedMessage,
	settlePromotedQueueHead,
	submitEdit,
	useConversationEditingState,
	waitForPendingChatSettingsSyncs,
} from "./AgentChatPage";
import type { ChatMessageInputRef } from "./components/AgentChatInput";
import { createChatStore } from "./components/ChatConversation/chatStore";
import type { PendingAttachment } from "./components/ChatPageContent";
import {
	chatDraftInputStorage,
	chatSidebarTabStorage,
	clearChatStorage,
} from "./storage";

type MockChatInputHandle = {
	handle: ChatMessageInputRef;
	setValue: ReturnType<typeof vi.fn>;
	clear: ReturnType<typeof vi.fn>;
	focus: ReturnType<typeof vi.fn>;
	getValue: ReturnType<typeof vi.fn>;
	currentValue: { value: string };
};

const createMockChatInputHandle = (initialValue = ""): MockChatInputHandle => {
	const currentValue = { value: initialValue };
	const setValue = vi.fn((text: string) => {
		currentValue.value = text;
	});
	const clear = vi.fn(() => {
		currentValue.value = "";
	});
	const focus = vi.fn();
	const getValue = vi.fn(() => currentValue.value);

	return {
		handle: {
			setValue,
			insertText: vi.fn(),
			clear,
			focus,
			getValue,
			addFileReference: vi.fn(),
			getContentParts: vi.fn(() => []),
		},
		setValue,
		clear,
		focus,
		getValue,
		currentValue,
	};
};

const setMobileViewport = (isMobile: boolean) => {
	Object.defineProperty(window, "matchMedia", {
		writable: true,
		value: vi.fn((query: string): MediaQueryList => {
			return {
				matches: query === "(max-width: 639px)" ? isMobile : false,
				media: query,
				onchange: null,
				addEventListener: vi.fn(),
				removeEventListener: vi.fn(),
				dispatchEvent: vi.fn(() => true),
				addListener: vi.fn(),
				removeListener: vi.fn(),
			} as MediaQueryList;
		}),
	});
};

describe("getWorkspaceOptionsWithLinkedWorkspace", () => {
	it("includes a missing linked workspace only when the current user owns it", () => {
		const existingWorkspace = {
			...MockWorkspace,
			id: "existing-workspace",
		};
		const ownerWorkspaceOptions = [existingWorkspace];
		const linkedWorkspace = {
			...MockWorkspace,
			id: "linked-workspace",
			owner_id: MockUserOwner.id,
		};

		expect(
			getWorkspaceOptionsWithLinkedWorkspace(
				ownerWorkspaceOptions,
				linkedWorkspace,
				MockUserOwner.id,
			),
		).toEqual([linkedWorkspace, existingWorkspace]);

		const sharedWorkspace = {
			...linkedWorkspace,
			owner_id: "another-user",
		};

		expect(
			getWorkspaceOptionsWithLinkedWorkspace(
				ownerWorkspaceOptions,
				sharedWorkspace,
				MockUserOwner.id,
			),
		).toBe(ownerWorkspaceOptions);
	});
});

describe("waitForPendingChatSettingsSyncs", () => {
	it("waits for plan-mode and workspace updates before resolving", async () => {
		const planModeUpdate = createDeferred<void>();
		const workspaceUpdate = createDeferred<void>();
		let settled = false;

		const waitPromise = waitForPendingChatSettingsSyncs([
			planModeUpdate.promise,
			workspaceUpdate.promise,
		]).then((result) => {
			settled = true;
			return result;
		});

		await Promise.resolve();
		expect(settled).toBe(false);

		planModeUpdate.resolve(undefined);
		await Promise.resolve();
		expect(settled).toBe(false);

		workspaceUpdate.resolve(undefined);
		await expect(waitPromise).resolves.toBeUndefined();
		expect(settled).toBe(true);
	});

	it("rejects when a chat-setting update fails", async () => {
		const workspaceUpdate = createDeferred<void>();
		const waitPromise = waitForPendingChatSettingsSyncs([
			workspaceUpdate.promise,
		]);

		workspaceUpdate.reject(new Error("boom"));
		await expect(waitPromise).rejects.toThrow("boom");
	});
});

describe("getPersistedDraftInputValue", () => {
	const chatID = "chat-abc-123";
	const expectedKey = `${chatDraftInputStorage.prefix}${chatID}`;

	beforeEach(() => {
		localStorage.clear();
		setMobileViewport(false);
	});

	it("reads the initial value from localStorage for a given chatID", () => {
		localStorage.setItem(expectedKey, "saved draft");

		expect(getPersistedDraftInputValue(chatID)).toBe("saved draft");
	});

	it("returns empty string when localStorage has no draft", () => {
		expect(getPersistedDraftInputValue(chatID)).toBe("");
	});
});

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

describe("useConversationEditingState", () => {
	const chatID = "chat-abc-123";
	const expectedKey = `${chatDraftInputStorage.prefix}${chatID}`;

	beforeEach(() => {
		localStorage.clear();
		setMobileViewport(false);
	});

	const renderEditing = (...args: [] | [string | undefined]) => {
		const onSend = vi.fn().mockResolvedValue(undefined);
		const chatInputRef = createRef<ChatMessageInputRef>();
		const inputValueRef = { current: "" };
		// createRef returns { current: null }, but we need it initialized
		// to "" so the hook sees a string.
		(inputValueRef as { current: string }).current = "";

		const resolvedChatID = args.length === 0 ? chatID : args[0];

		const hook = renderHook(() =>
			useConversationEditingState({
				chatID: resolvedChatID,
				onSend,
				chatInputRef,
				inputValueRef,
			}),
		);

		return { ...hook, onSend, inputValueRef };
	};

	it("persists and removes drafts via handleContentChange", () => {
		const { result, unmount } = renderEditing();

		act(() => {
			result.current.handleContentChange(
				"work in progress",
				"work in progress",
				false,
			);
		});
		expect(chatDraftInputStorage.forId(chatID).get()).toBe("work in progress");
		// handleContentChange persists only; it must not advance the seed.
		expect(result.current.editorInitialValue).toBe("");
		expect(result.current.initialEditorState).toBeUndefined();

		act(() => {
			// Even though the serialized state is non-empty (Lexical always
			// produces a JSON object), the draft is removed when the plain
			// text content is empty.
			result.current.handleContentChange("", '{"root":{"children":[]}}', false);
		});
		expect(localStorage.getItem(expectedKey)).toBeNull();

		unmount();
	});

	it("carries a draft typed during loading into the seed for the loaded editor", () => {
		const { result, unmount } = renderEditing();

		// handleContentChange persists but does not advance the seed.
		const editorState =
			'{"root":{"children":[{"text":"typed while loading"}]}}';
		act(() => {
			result.current.handleLoadingDraftChange(
				"typed while loading",
				editorState,
				false,
			);
		});

		expect(chatDraftInputStorage.forId(chatID).get()).toBe(editorState);
		expect(result.current.editorInitialValue).toBe("typed while loading");
		expect(result.current.initialEditorState).toBe(editorState);

		unmount();
	});

	it("loads edit text into the composer and restores the prior draft on cancel without refocusing", () => {
		const { result, unmount } = renderEditing();

		// Simulate the user typing a draft via handleContentChange.
		act(() => {
			result.current.handleContentChange(
				"work in progress",
				"work in progress",
				false,
			);
		});

		const remountKeyBefore = result.current.remountKey;

		act(() => {
			result.current.handleEditUserMessage(7, "edited message");
		});

		expect(result.current.editingMessageId).toBe(7);
		expect(result.current.editorInitialValue).toBe("edited message");
		expect(result.current.remountKey).toBe(remountKeyBefore + 1);

		const remountKeyAfterEdit = result.current.remountKey;

		act(() => {
			result.current.handleCancelHistoryEdit();
		});

		expect(result.current.editingMessageId).toBeNull();
		expect(result.current.editorInitialValue).toBe("work in progress");
		expect(result.current.remountKey).toBe(remountKeyAfterEdit + 1);
		unmount();
	});

	it("does not force focus when replacing input values on mobile", () => {
		setMobileViewport(true);
		const { result, unmount } = renderEditing();
		const mockInput = createMockChatInputHandle("draft before edit");
		result.current.chatInputRef.current = mockInput.handle;

		// Edit/cancel now drive the editor via editorInitialValue +
		// remountKey, so focus is never called on the mock during
		// edit and cancel flows. handleSendFromInput is the only
		// path that calls focus and it skips on mobile viewports.
		act(() => {
			result.current.handleEditUserMessage(7, "edited message");
		});
		expect(mockInput.focus).not.toHaveBeenCalled();

		act(() => {
			result.current.handleCancelHistoryEdit();
		});
		expect(mockInput.focus).not.toHaveBeenCalled();
		unmount();
	});

	it("falls back to the persisted draft when history edit starts before hydration", () => {
		localStorage.setItem(expectedKey, "persisted draft");
		const { result, unmount } = renderEditing();

		act(() => {
			result.current.handleEditUserMessage(7, "edited message");
		});

		act(() => {
			result.current.handleCancelHistoryEdit();
		});

		// The hook reads the persisted draft from localStorage when
		// inputValueRef hasn't been updated by handleContentChange yet.
		expect(result.current.editorInitialValue).toBe("persisted draft");
		unmount();
	});

	it("prefers the live editor value over stale persisted draft state", () => {
		localStorage.setItem(expectedKey, "stale persisted draft");
		const { result, unmount } = renderEditing();

		// Simulate the editor emitting a content change, which updates
		// inputValueRef to the live value.
		act(() => {
			result.current.handleContentChange("live draft", "live draft", false);
		});

		act(() => {
			result.current.handleEditUserMessage(7, "edited message");
		});

		act(() => {
			result.current.handleCancelHistoryEdit();
		});

		expect(result.current.editorInitialValue).toBe("live draft");
		unmount();
	});

	it("can load the same edit text again after send", async () => {
		const { result, onSend, unmount } = renderEditing();
		const mockInput = createMockChatInputHandle();
		result.current.chatInputRef.current = mockInput.handle;

		const remountKeyBefore = result.current.remountKey;

		act(() => {
			result.current.handleEditUserMessage(7, "hello");
		});

		expect(result.current.remountKey).toBe(remountKeyBefore + 1);

		await act(async () => {
			await result.current.handleSendFromInput("hello");
		});

		const remountKeyAfterSend = result.current.remountKey;

		act(() => {
			result.current.handleEditUserMessage(7, "hello");
		});

		// remountKey increments each time an edit is loaded, even for
		// the same text, so the editor is forced to reinitialize.
		expect(result.current.remountKey).toBe(remountKeyAfterSend + 1);
		expect(result.current.editorInitialValue).toBe("hello");
		expect(onSend).toHaveBeenCalledWith("hello", undefined, 7);
		unmount();
	});

	it("forwards pending attachments through history-edit send", async () => {
		const { result, onSend, unmount } = renderEditing();
		const attachments: PendingAttachment[] = [
			{ fileId: "file-1", mediaType: "image/png" },
		];

		act(() => {
			result.current.handleEditUserMessage(7, "hello");
		});

		await act(async () => {
			await result.current.handleSendFromInput("hello", attachments);
		});

		expect(onSend).toHaveBeenCalledWith("hello", attachments, 7);
		unmount();
	});

	it("restores the edit draft and file-block seed when an edit submission fails", async () => {
		const { result, onSend, unmount } = renderEditing();
		const mockInput = createMockChatInputHandle("edited message");
		const fileBlocks = [
			{ type: "file", file_id: "file-1", media_type: "image/png" },
		] as const;
		result.current.chatInputRef.current = mockInput.handle;
		onSend.mockRejectedValueOnce(new Error("boom"));
		const editorState = JSON.stringify({
			root: {
				children: [
					{
						children: [{ text: "edited message" }],
						type: "paragraph",
					},
				],
				type: "root",
			},
		});

		act(() => {
			result.current.handleEditUserMessage(7, "edited message", fileBlocks);
			result.current.handleContentChange("edited message", editorState, false);
		});

		await act(async () => {
			await expect(
				result.current.handleSendFromInput("edited message"),
			).rejects.toThrow("boom");
		});

		expect(mockInput.clear).toHaveBeenCalled();
		expect(result.current.inputValueRef.current).toBe("edited message");
		expect(result.current.editingMessageId).toBe(7);
		expect(result.current.editingFileBlocks).toEqual(fileBlocks);
		expect(result.current.editorInitialValue).toBe("edited message");
		expect(result.current.initialEditorState).toBe(editorState);
		unmount();
	});

	it("preserves the composer and draft when send fails", async () => {
		const { result, onSend, unmount } = renderEditing();
		const mockInput = createMockChatInputHandle("hello");
		result.current.chatInputRef.current = mockInput.handle;
		onSend.mockRejectedValueOnce(new Error("boom"));

		act(() => {
			result.current.handleContentChange("hello", "hello", false);
		});

		await act(async () => {
			await expect(result.current.handleSendFromInput("hello")).rejects.toThrow(
				"boom",
			);
		});

		expect(mockInput.clear).not.toHaveBeenCalled();
		expect(mockInput.focus).not.toHaveBeenCalled();
		expect(result.current.inputValueRef.current).toBe("hello");
		expect(chatDraftInputStorage.forId(chatID).get()).toBe("hello");
		unmount();
	});

	it("clears the composer and persisted draft after a successful send", async () => {
		localStorage.setItem(expectedKey, "draft to clear");
		const { result, onSend, unmount } = renderEditing();
		const mockInput = createMockChatInputHandle("hello");
		result.current.chatInputRef.current = mockInput.handle;

		await act(async () => {
			await result.current.handleSendFromInput("hello");
		});

		expect(onSend).toHaveBeenCalledWith("hello", undefined, undefined);
		expect(mockInput.clear).toHaveBeenCalled();
		expect(mockInput.focus).toHaveBeenCalled();
		expect(localStorage.getItem(expectedKey)).toBeNull();
		unmount();
	});

	it("does not write a draft key when chatID is undefined", () => {
		const { result, unmount } = renderEditing(undefined);

		act(() => {
			result.current.handleContentChange("should not persist", "{}", false);
		});

		// The ref is still updated even without persistence.
		expect(result.current.inputValueRef.current).toBe("should not persist");
		// No draft for "undefined" chatID should appear.
		expect(
			localStorage.getItem(`${chatDraftInputStorage.prefix}undefined`),
		).toBeNull();
		unmount();
	});

	it("calls focus on the input ref after a successful send", async () => {
		const { result, onSend, unmount } = renderEditing();

		// Attach a mock ChatMessageInputRef to the chatInputRef
		const mockFocus = vi.fn();
		const mockClear = vi.fn();
		const mockInputRef = {
			focus: mockFocus,
			clear: mockClear,
			setValue: vi.fn(),
			insertText: vi.fn(),
			getValue: vi.fn().mockReturnValue(""),
			addFileReference: vi.fn(),
			getContentParts: vi.fn().mockReturnValue([]),
		}; // The hook exposes chatInputRef – assign the mock to it.
		result.current.chatInputRef.current = mockInputRef;

		await act(async () => {
			result.current.handleSendFromInput("hello");
			await vi.waitFor(() => {
				expect(onSend).toHaveBeenCalledWith("hello", undefined, undefined);
			});
		});

		expect(mockClear).toHaveBeenCalled();
		expect(mockFocus).toHaveBeenCalled();
		unmount();
	});

	it("initializes with the correct draft for each chatID", () => {
		const chatA = "chat-aaa";
		const chatB = "chat-bbb";
		localStorage.setItem(`${chatDraftInputStorage.prefix}${chatA}`, "draft A");
		localStorage.setItem(`${chatDraftInputStorage.prefix}${chatB}`, "draft B");

		// Each chatID should initialize with its own draft — this is
		// what the key={agentId} wrapper guarantees at the component
		// level (a new chatID means a full remount).
		const hookA = renderEditing(chatA);
		expect(hookA.result.current.editorInitialValue).toBe("draft A");
		hookA.unmount();

		const hookB = renderEditing(chatB);
		expect(hookB.result.current.editorInitialValue).toBe("draft B");
		hookB.unmount();
	});

	it("clears the draft from localStorage on successful send", async () => {
		localStorage.setItem(expectedKey, "draft to clear");

		const { result, unmount } = renderEditing();

		expect(chatDraftInputStorage.forId(chatID).get()).toBe("draft to clear");

		await act(async () => {
			result.current.handleSendFromInput("hello");
			await vi.waitFor(() => {
				expect(localStorage.getItem(expectedKey)).toBeNull();
			});
		});
		unmount();
	});

	it("persists serialized editor state when provided", () => {
		const { result, unmount } = renderEditing();
		const editorState = JSON.stringify({
			root: {
				children: [
					{
						children: [
							{ text: "review this" },
							{
								type: "file-reference",
								version: 1,
								fileName: "main.go",
								startLine: 1,
								endLine: 10,
								content: "code",
							},
						],
						type: "paragraph",
					},
				],
				type: "root",
			},
		});

		act(() => {
			result.current.handleContentChange("review this", editorState, true);
		});

		// The serialized editor state should be stored, not the plain text.
		expect(chatDraftInputStorage.forId(chatID).get()).toBe(editorState);
		expect(result.current.inputValueRef.current).toBe("review this");
		unmount();
	});

	it("restores editorInitialState from a Lexical JSON draft", () => {
		const editorState = JSON.stringify({
			root: {
				children: [
					{
						children: [{ text: "hello" }],
						type: "paragraph",
					},
				],
				type: "root",
			},
		});
		localStorage.setItem(expectedKey, editorState);

		const { result, unmount } = renderEditing();

		expect(result.current.initialEditorState).toBe(editorState);
		expect(result.current.editorInitialValue).toBe("hello");
		unmount();
	});

	it("falls back to plain text for legacy drafts", () => {
		localStorage.setItem(expectedKey, "legacy plain text");

		const { result, unmount } = renderEditing();

		expect(result.current.initialEditorState).toBeUndefined();
		expect(result.current.editorInitialValue).toBe("legacy plain text");
		unmount();
	});

	it("persists file-reference-only drafts (no text content)", () => {
		const { result, unmount } = renderEditing();
		const editorState = JSON.stringify({
			root: {
				children: [
					{
						children: [
							{
								type: "file-reference",
								version: 1,
								fileName: "main.go",
								startLine: 1,
								endLine: 10,
								content: "code",
							},
						],
						type: "paragraph",
					},
				],
				type: "root",
			},
		});

		act(() => {
			// Empty text but hasFileReferences=true should still persist.
			result.current.handleContentChange("", editorState, true);
		});

		expect(chatDraftInputStorage.forId(chatID).get()).toBe(editorState);
		unmount();
	});

	it("removes draft for whitespace-only content without file references", () => {
		localStorage.setItem(expectedKey, "old draft");
		const { result, unmount } = renderEditing();

		act(() => {
			result.current.handleContentChange("   ", '{"root":{}}', false);
		});

		expect(localStorage.getItem(expectedKey)).toBeNull();
		unmount();
	});

	it("preserves serialized editor state across history edit then cancel", () => {
		const editorState = JSON.stringify({
			root: {
				children: [
					{
						children: [
							{ text: "my draft", type: "text" },
							{
								type: "file-reference",
								version: 1,
								fileName: "main.go",
								startLine: 1,
								endLine: 10,
								content: "code",
							},
						],
						type: "paragraph",
					},
				],
				type: "root",
			},
		});
		localStorage.setItem(expectedKey, editorState);

		const { result, unmount } = renderEditing();

		expect(result.current.initialEditorState).toBe(editorState);
		expect(result.current.editorInitialValue).toBe("my draft");

		// Simulate typing so localStorage reflects the current draft.
		act(() => {
			result.current.handleContentChange("my draft", editorState, true);
		});

		// Start editing a history message.
		act(() => {
			result.current.handleEditUserMessage(42, "old message text");
		});

		expect(result.current.editingMessageId).toBe(42);
		expect(result.current.initialEditorState).toBeUndefined();
		expect(result.current.editorInitialValue).toBe("old message text");

		// Cancel — should restore both plain text and serialized state.
		act(() => {
			result.current.handleCancelHistoryEdit();
		});

		expect(result.current.editingMessageId).toBeNull();
		expect(result.current.initialEditorState).toBe(editorState);
		expect(result.current.editorInitialValue).toBe("my draft");
		unmount();
	});

	it("returns undefined initialEditorState after edit then cancel with plain-text draft", () => {
		localStorage.setItem(expectedKey, "plain text draft");

		const { result, unmount } = renderEditing();

		expect(result.current.initialEditorState).toBeUndefined();

		act(() => {
			result.current.handleContentChange(
				"plain text draft",
				"plain text draft",
				false,
			);
		});

		act(() => {
			result.current.handleEditUserMessage(1, "editing");
		});

		act(() => {
			result.current.handleCancelHistoryEdit();
		});

		expect(result.current.initialEditorState).toBeUndefined();
		expect(result.current.editorInitialValue).toBe("plain text draft");
		unmount();
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

describe("sidebar tab persistence", () => {
	beforeEach(() => {
		localStorage.clear();
	});

	it("stores the tab under agents.last-active-tab.<chatID>", () => {
		chatSidebarTabStorage.forId("chat-1").set("desktop");
		expect(localStorage.getItem("agents.last-active-tab.chat-1")).toBe(
			"desktop",
		);
		expect(chatSidebarTabStorage.forId("chat-1").get()).toBe("desktop");
	});

	it("is removed by chat entity cleanup without touching other chats", () => {
		chatSidebarTabStorage.forId("chat-a").set("git");
		chatSidebarTabStorage.forId("chat-b").set("desktop");
		clearChatStorage("chat-a");
		expect(chatSidebarTabStorage.forId("chat-a").get()).toBeNull();
		expect(chatSidebarTabStorage.forId("chat-b").get()).toBe("desktop");
	});
});

describe("isWatchedWorkspaceViewUnchanged", () => {
	const cloneWithApps = (apps: WorkspaceApp[]): Workspace => ({
		...MockWorkspace,
		latest_build: {
			...MockWorkspace.latest_build,
			resources: MockWorkspace.latest_build.resources.map((resource) => ({
				...resource,
				agents: resource.agents?.map((agent) =>
					agent.id === MockWorkspaceAgent.id ? { ...agent, apps } : agent,
				),
			})),
		},
	});

	it("is true for a fresh payload with only unwatched changes", () => {
		const next: Workspace = {
			...MockWorkspace,
			last_used_at: "2024-01-01T00:00:00Z",
		};

		expect(
			isWatchedWorkspaceViewUnchanged(
				MockWorkspace,
				next,
				MockWorkspaceAgent.id,
			),
		).toBe(true);
	});

	it("is false when a bound-agent app changes health", () => {
		const next = cloneWithApps([{ ...MockWorkspaceApp, health: "healthy" }]);

		expect(
			isWatchedWorkspaceViewUnchanged(
				MockWorkspace,
				next,
				MockWorkspaceAgent.id,
			),
		).toBe(false);
	});

	it("is false when the bound agent gains an app", () => {
		const next = cloneWithApps([
			MockWorkspaceApp,
			{ ...MockWorkspaceApp, id: "second-app", slug: "second-app" },
		]);

		expect(
			isWatchedWorkspaceViewUnchanged(
				MockWorkspace,
				next,
				MockWorkspaceAgent.id,
			),
		).toBe(false);
	});

	it("is false when the latest build changes", () => {
		const next: Workspace = {
			...MockWorkspace,
			latest_build: { ...MockWorkspace.latest_build, id: "new-build-id" },
		};

		expect(
			isWatchedWorkspaceViewUnchanged(
				MockWorkspace,
				next,
				MockWorkspaceAgent.id,
			),
		).toBe(false);
	});
});

describe("isChatAgentBindingUnresolved", () => {
	it("is true when the bound agent is missing from the running build", () => {
		expect(isChatAgentBindingUnresolved(MockWorkspace, "stale-agent-id")).toBe(
			true,
		);
	});

	it("is true when the chat has no binding yet", () => {
		expect(isChatAgentBindingUnresolved(MockWorkspace, undefined)).toBe(true);
	});

	it("is false when the bound agent resolves", () => {
		expect(
			isChatAgentBindingUnresolved(MockWorkspace, MockWorkspaceAgent.id),
		).toBe(false);
	});

	it("is false when the workspace is not running", () => {
		const stopped: Workspace = {
			...MockWorkspace,
			latest_build: { ...MockWorkspace.latest_build, status: "stopped" },
		};

		expect(isChatAgentBindingUnresolved(stopped, "stale-agent-id")).toBe(false);
	});

	it("is false when the running build has no agents", () => {
		const noAgents: Workspace = {
			...MockWorkspace,
			latest_build: {
				...MockWorkspace.latest_build,
				resources: MockWorkspace.latest_build.resources.map((resource) => ({
					...resource,
					agents: [],
				})),
			},
		};

		expect(isChatAgentBindingUnresolved(noAgents, "stale-agent-id")).toBe(
			false,
		);
	});

	it("is false while the workspace is loading", () => {
		expect(isChatAgentBindingUnresolved(undefined, "stale-agent-id")).toBe(
			false,
		);
	});
});

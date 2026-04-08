import { act, renderHook } from "@testing-library/react";
import { createRef } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
	draftInputStorageKeyPrefix,
	getPersistedDraftInputValue,
	useConversationEditingState,
} from "./AgentChatPage";
import type { ChatMessageInputRef } from "./components/AgentChatInput";

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

describe("getPersistedDraftInputValue", () => {
	const chatID = "chat-abc-123";
	const expectedKey = `${draftInputStorageKeyPrefix}${chatID}`;

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

describe("useConversationEditingState", () => {
	const chatID = "chat-abc-123";
	const expectedKey = `${draftInputStorageKeyPrefix}${chatID}`;

	beforeEach(() => {
		localStorage.clear();
		setMobileViewport(false);
	});

	const renderEditing = (...args: [] | [string | undefined]) => {
		const onSend = vi.fn().mockResolvedValue(undefined);
		const onDeleteQueuedMessage = vi.fn().mockResolvedValue(undefined);
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
				onDeleteQueuedMessage,
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
		expect(localStorage.getItem(expectedKey)).toBe("work in progress");

		act(() => {
			// Even though the serialized state is non-empty (Lexical always
			// produces a JSON object), the draft is removed when the plain
			// text content is empty.
			result.current.handleContentChange("", '{"root":{"children":[]}}', false);
		});
		expect(localStorage.getItem(expectedKey)).toBeNull();

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

	it("loads queue edit text into the composer and restores the prior draft on cancel without refocusing", () => {
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
			result.current.handleStartQueueEdit(9, "queued message", []);
		});

		expect(result.current.editingQueuedMessageID).toBe(9);
		expect(result.current.editorInitialValue).toBe("queued message");
		expect(result.current.remountKey).toBe(remountKeyBefore + 1);

		const remountKeyAfterEdit = result.current.remountKey;

		act(() => {
			result.current.handleCancelQueueEdit();
		});

		expect(result.current.editingQueuedMessageID).toBeNull();
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

		act(() => {
			result.current.handleStartQueueEdit(9, "queued message", []);
		});
		expect(mockInput.focus).not.toHaveBeenCalled();

		act(() => {
			result.current.handleCancelQueueEdit();
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

	it("falls back to the persisted draft when queue edit starts before hydration", () => {
		localStorage.setItem(expectedKey, "persisted draft");
		const { result, unmount } = renderEditing();

		act(() => {
			result.current.handleStartQueueEdit(9, "queued message", []);
		});

		act(() => {
			result.current.handleCancelQueueEdit();
		});

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
			localStorage.getItem(`${draftInputStorageKeyPrefix}undefined`),
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
		localStorage.setItem(`${draftInputStorageKeyPrefix}${chatA}`, "draft A");
		localStorage.setItem(`${draftInputStorageKeyPrefix}${chatB}`, "draft B");

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

		expect(localStorage.getItem(expectedKey)).toBe("draft to clear");

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
		expect(localStorage.getItem(expectedKey)).toBe(editorState);
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

		expect(localStorage.getItem(expectedKey)).toBe(editorState);
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

	it("preserves serialized editor state across queue edit then cancel", () => {
		const editorState = JSON.stringify({
			root: {
				children: [
					{
						children: [{ text: "queued draft", type: "text" }],
						type: "paragraph",
					},
				],
				type: "root",
			},
		});
		localStorage.setItem(expectedKey, editorState);

		const { result, unmount } = renderEditing();

		act(() => {
			result.current.handleContentChange("queued draft", editorState, false);
		});

		act(() => {
			result.current.handleStartQueueEdit(99, "queued msg", []);
		});

		expect(result.current.editingQueuedMessageID).toBe(99);
		expect(result.current.initialEditorState).toBeUndefined();

		act(() => {
			result.current.handleCancelQueueEdit();
		});

		expect(result.current.editingQueuedMessageID).toBeNull();
		expect(result.current.initialEditorState).toBe(editorState);
		expect(result.current.editorInitialValue).toBe("queued draft");
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

import { act, renderHook } from "@testing-library/react";
import { createRef } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ChatMessageInputRef } from "../components/AgentChatInput";
import type { PendingAttachment } from "../components/ChatPageContent";
import { draftInputStorageKeyPrefix } from "../utils/draftStorage";
import {
	BuiltInCommandPendingError,
	useConversationEditingState,
} from "./useConversationEditingState";

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
			insertSkill: vi.fn(),
			removeSkills: vi.fn(),
			getEditorState: vi.fn(() => ""),
			restoreEditorState: vi.fn(),
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

describe("useConversationEditingState", () => {
	const chatID = "chat-abc-123";
	const expectedKey = `${draftInputStorageKeyPrefix}${chatID}`;

	beforeEach(() => {
		localStorage.clear();
		setMobileViewport(false);
	});

	const renderEditing = (...args: [] | [string]) => {
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
		expect(localStorage.getItem(expectedKey)).toBe("work in progress");
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

		expect(localStorage.getItem(expectedKey)).toBe(editorState);
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

	it("seeds the composer with the edited message's editor state", () => {
		const { result, unmount } = renderEditing();
		const editorState = JSON.stringify({ root: { type: "root" } });

		act(() => {
			result.current.handleEditUserMessage(7, "", undefined, editorState);
		});

		expect(result.current.initialEditorState).toBe(editorState);
		unmount();
	});

	it("does not force focus when replacing input values on mobile", () => {
		setMobileViewport(true);
		const { result, unmount } = renderEditing();
		const mockInput = createMockChatInputHandle("draft before edit");
		result.current.chatInputRef.current = mockInput.handle;

		// Edit and cancel drive the editor through editorInitialValue and
		// remountKey, so they never call focus.
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
			await result.current.handleSendFromInput({ message: "hello" });
		});

		const remountKeyAfterSend = result.current.remountKey;

		act(() => {
			result.current.handleEditUserMessage(7, "hello");
		});

		// remountKey increments each time an edit is loaded, even for
		// the same text, so the editor is forced to reinitialize.
		expect(result.current.remountKey).toBe(remountKeyAfterSend + 1);
		expect(result.current.editorInitialValue).toBe("hello");
		expect(onSend).toHaveBeenCalledWith({
			message: "hello",
			attachments: undefined,
			workspaceUploads: undefined,
			editedMessageID: 7,
		});
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
			await result.current.handleSendFromInput({
				message: "hello",
				attachments,
			});
		});

		expect(onSend).toHaveBeenCalledWith({
			message: "hello",
			attachments,
			workspaceUploads: undefined,
			editedMessageID: 7,
		});
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
				result.current.handleSendFromInput({ message: "edited message" }),
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
		// The editor reports its clear as a content change.
		mockInput.clear.mockImplementation(() =>
			result.current.handleContentChange("", "", false),
		);
		result.current.chatInputRef.current = mockInput.handle;
		let rejectSend: (error: Error) => void = () => {};
		onSend.mockReturnValueOnce(
			new Promise((_resolve, reject) => {
				rejectSend = reject;
			}),
		);

		act(() => {
			result.current.handleContentChange("hello", "hello", false);
		});

		let sent: Promise<void> = Promise.resolve();
		act(() => {
			sent = result.current.handleSendFromInput({ message: "hello" });
		});
		expect(mockInput.clear).toHaveBeenCalled();
		expect(result.current.inputValueRef.current).toBe("");

		await act(async () => {
			rejectSend(new Error("boom"));
			await expect(sent).rejects.toThrow("boom");
		});

		expect(mockInput.focus).not.toHaveBeenCalled();
		expect(result.current.inputValueRef.current).toBe("hello");
		expect(result.current.editorInitialValue).toBe("hello");
		expect(result.current.initialEditorState).toBe("hello");
		expect(localStorage.getItem(expectedKey)).toBe("hello");

		// A command whose availability is still loading restores it too.
		onSend.mockRejectedValueOnce(new BuiltInCommandPendingError());
		await act(async () => {
			await result.current.handleSendFromInput({ message: "hello" });
		});
		expect(result.current.inputValueRef.current).toBe("hello");
		expect(localStorage.getItem(expectedKey)).toBe("hello");
		unmount();
	});

	it("keeps the saved draft while a send is pending after the chat unmounts", async () => {
		const { result, onSend, unmount } = renderEditing();
		const mockInput = createMockChatInputHandle("hello");
		// The editor reports its clear as a content change.
		mockInput.clear.mockImplementation(() =>
			result.current.handleContentChange("", "", false),
		);
		result.current.chatInputRef.current = mockInput.handle;
		let rejectSend: (error: Error) => void = () => {};
		onSend.mockReturnValueOnce(
			new Promise((_resolve, reject) => {
				rejectSend = reject;
			}),
		);

		act(() => {
			result.current.handleContentChange("hello", "hello", false);
		});
		let sent: Promise<void> = Promise.resolve();
		act(() => {
			sent = result.current.handleSendFromInput({ message: "hello" });
		});
		unmount();

		// A reload now finds the submitted message.
		expect(localStorage.getItem(expectedKey)).toBe("hello");
		rejectSend(new Error("boom"));
		await expect(sent).rejects.toThrow("boom");
	});

	it("keeps what was typed during a send after it succeeds", async () => {
		const { result, onSend, unmount } = renderEditing();
		const mockInput = createMockChatInputHandle("hello");
		// The editor reports its clear as a content change.
		mockInput.clear.mockImplementation(() =>
			result.current.handleContentChange("", "", false),
		);
		result.current.chatInputRef.current = mockInput.handle;
		let resolveSend: () => void = () => {};
		onSend.mockReturnValueOnce(
			new Promise<void>((resolve) => {
				resolveSend = resolve;
			}),
		);

		let sent: Promise<void> = Promise.resolve();
		act(() => {
			sent = result.current.handleSendFromInput({ message: "hello" });
		});
		act(() => {
			result.current.handleContentChange("next", "next state", false);
		});
		await act(async () => {
			resolveSend();
			await sent;
		});

		expect(mockInput.clear).toHaveBeenCalledTimes(1);
		expect(result.current.inputValueRef.current).toBe("next");
		expect(localStorage.getItem(expectedKey)).toBe("next state");
		unmount();
	});

	it("puts a failed message in front of what was typed during it", async () => {
		const { result, onSend, unmount } = renderEditing();
		const mockInput = createMockChatInputHandle("hello");
		result.current.chatInputRef.current = mockInput.handle;
		const paragraph = (text: string) => ({
			type: "paragraph",
			children: [{ type: "text", text }],
		});
		const editorState = (text: string) =>
			JSON.stringify({ root: { type: "root", children: [paragraph(text)] } });
		let rejectSend: (error: Error) => void = () => {};
		onSend.mockReturnValueOnce(
			new Promise((_resolve, reject) => {
				rejectSend = reject;
			}),
		);

		act(() => {
			result.current.handleContentChange("hello", editorState("hello"), false);
		});
		let sent: Promise<void> = Promise.resolve();
		act(() => {
			sent = result.current.handleSendFromInput({ message: "hello" });
		});
		act(() => {
			result.current.handleContentChange("next", editorState("next"), false);
		});
		await act(async () => {
			rejectSend(new Error("boom"));
			await expect(sent).rejects.toThrow("boom");
		});

		expect(result.current.editorInitialValue).toBe("hello\n\nnext");
		expect(result.current.inputValueRef.current).toBe("hello\n\nnext");
		expect(JSON.parse(result.current.initialEditorState ?? "")).toEqual({
			root: {
				type: "root",
				children: [paragraph("hello"), paragraph("next")],
			},
		});
		unmount();
	});

	it("clears the composer and persisted draft after a successful send", async () => {
		localStorage.setItem(expectedKey, "draft to clear");
		const { result, onSend, unmount } = renderEditing();
		const mockInput = createMockChatInputHandle("hello");
		result.current.chatInputRef.current = mockInput.handle;

		await act(async () => {
			await result.current.handleSendFromInput({ message: "hello" });
		});

		expect(onSend).toHaveBeenCalledWith({
			message: "hello",
			attachments: undefined,
			workspaceUploads: undefined,
			editedMessageID: undefined,
		});
		expect(mockInput.clear).toHaveBeenCalled();
		expect(localStorage.getItem(expectedKey)).toBeNull();
		unmount();
	});

	it("initializes with the correct draft for each chatID", () => {
		const chatA = "chat-aaa";
		const chatB = "chat-bbb";
		localStorage.setItem(`${draftInputStorageKeyPrefix}${chatA}`, "draft A");
		localStorage.setItem(`${draftInputStorageKeyPrefix}${chatB}`, "draft B");

		// Each chatID should initialize with its own draft. This is
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
			result.current.handleSendFromInput({ message: "hello" });
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

		// Cancel should restore both plain text and serialized state.
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

import { act, renderHook } from "@testing-library/react";
import { createRef } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ChatMessageInputRef } from "./AgentChatInput";
import {
	draftInputStorageKeyPrefix,
	useConversationEditingState,
} from "./AgentDetail";

describe("useConversationEditingState", () => {
	const chatID = "chat-abc-123";
	const expectedKey = `${draftInputStorageKeyPrefix}${chatID}`;

	beforeEach(() => {
		localStorage.clear();
	});

	const renderEditing = (id: string | undefined = chatID) => {
		const onSend = vi.fn().mockResolvedValue(undefined);
		const onDeleteQueuedMessage = vi.fn().mockResolvedValue(undefined);
		const chatInputRef = createRef<ChatMessageInputRef>();
		const inputValueRef: import("react").RefObject<string> = { current: "" };

		const hook = renderHook(() =>
			useConversationEditingState({
				chatID: id,
				onSend,
				onDeleteQueuedMessage,
				chatInputRef,
				inputValueRef,
			}),
		);

		return { ...hook, onSend, onDeleteQueuedMessage };
	};

	it("reads the initial value from localStorage for a given chatID", () => {
		localStorage.setItem(expectedKey, "saved draft");

		const { result, unmount } = renderEditing();

		expect(result.current.editorInitialValue).toBe("saved draft");
		expect(result.current.inputValueRef.current).toBe("saved draft");
		unmount();
	});

	it("returns empty string when localStorage has no draft", () => {
		const { result, unmount } = renderEditing();

		expect(result.current.editorInitialValue).toBe("");
		expect(result.current.inputValueRef.current).toBe("");
		unmount();
	});

	it("writes content to localStorage via handleContentChange", () => {
		const { result, unmount } = renderEditing();

		act(() => {
			result.current.handleContentChange("work in progress");
		});

		expect(localStorage.getItem(expectedKey)).toBe("work in progress");
		expect(result.current.inputValueRef.current).toBe("work in progress");
		unmount();
	});

	it("removes the draft key when handleContentChange receives empty string", () => {
		localStorage.setItem(expectedKey, "old draft");
		const { result, unmount } = renderEditing();

		act(() => {
			result.current.handleContentChange("");
		});

		expect(localStorage.getItem(expectedKey)).toBeNull();
		unmount();
	});

	it("does not write a draft key when chatID is undefined", () => {
		const { result, unmount } = renderEditing(undefined);

		act(() => {
			result.current.handleContentChange("should not persist");
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

	it("loads the new chat draft and clears edit state when chatID changes", async () => {
		const chatOne = "chat-one";
		const chatTwo = "chat-two";
		localStorage.setItem(
			`${draftInputStorageKeyPrefix}${chatOne}`,
			"draft one",
		);
		localStorage.setItem(
			`${draftInputStorageKeyPrefix}${chatTwo}`,
			"draft two",
		);

		const onSend = vi.fn().mockResolvedValue(undefined);
		const onDeleteQueuedMessage = vi.fn().mockResolvedValue(undefined);
		const chatInputRef = createRef<ChatMessageInputRef>();
		const inputValueRef: import("react").RefObject<string> = { current: "" };

		const { result, rerender } = renderHook(
			({ chatID }: { chatID: string | undefined }) =>
				useConversationEditingState({
					chatID,
					onSend,
					onDeleteQueuedMessage,
					chatInputRef,
					inputValueRef,
				}),
			{
				initialProps: { chatID: chatOne },
			},
		);

		expect(result.current.editorInitialValue).toBe("draft one");
		act(() => {
			result.current.handleEditUserMessage(42, "editing now");
		});
		expect(result.current.editingMessageId).toBe(42);

		rerender({ chatID: chatTwo });

		await vi.waitFor(() => {
			expect(result.current.editorInitialValue).toBe("draft two");
			expect(result.current.inputValueRef.current).toBe("draft two");
			expect(result.current.editingMessageId).toBeNull();
			expect(result.current.editingQueuedMessageID).toBeNull();
		});
	});

	it("clears the input state when switching to an undefined chatID", async () => {
		const chatID = "chat-one";
		localStorage.setItem(`${draftInputStorageKeyPrefix}${chatID}`, "draft one");

		const onSend = vi.fn().mockResolvedValue(undefined);
		const onDeleteQueuedMessage = vi.fn().mockResolvedValue(undefined);
		const chatInputRef = createRef<ChatMessageInputRef>();
		const inputValueRef: import("react").RefObject<string> = { current: "" };
		const initialProps: { nextChatID: string | undefined } = {
			nextChatID: chatID,
		};

		const { result, rerender } = renderHook(
			({ nextChatID }: { nextChatID: string | undefined }) =>
				useConversationEditingState({
					chatID: nextChatID,
					onSend,
					onDeleteQueuedMessage,
					chatInputRef,
					inputValueRef,
				}),
			{ initialProps },
		);

		expect(result.current.editorInitialValue).toBe("draft one");
		rerender({ nextChatID: undefined });

		await vi.waitFor(() => {
			expect(result.current.editorInitialValue).toBe("");
			expect(result.current.inputValueRef.current).toBe("");
		});
	});
});

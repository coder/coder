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

describe("getPersistedDraftInputValue", () => {
	const chatID = "chat-abc-123";
	const expectedKey = `${draftInputStorageKeyPrefix}${chatID}`;

	beforeEach(() => {
		localStorage.clear();
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
	});

	const renderEditing = () => {
		const onSend = vi.fn().mockResolvedValue(undefined);
		const onDeleteQueuedMessage = vi.fn().mockResolvedValue(undefined);
		const chatInputRef = createRef<ChatMessageInputRef>();

		const hook = renderHook(() =>
			useConversationEditingState({
				chatID,
				onSend,
				onDeleteQueuedMessage,
				chatInputRef,
			}),
		);

		return { ...hook, onSend };
	};

	it("persists and removes drafts via handleContentChange", () => {
		const { result, unmount } = renderEditing();

		act(() => {
			result.current.handleContentChange("work in progress");
		});
		expect(localStorage.getItem(expectedKey)).toBe("work in progress");

		act(() => {
			result.current.handleContentChange("");
		});
		expect(localStorage.getItem(expectedKey)).toBeNull();

		unmount();
	});

	it("loads edit text into the composer and restores the prior draft on cancel", () => {
		const { result, unmount } = renderEditing();
		const mockInput = createMockChatInputHandle("work in progress");
		result.current.chatInputRef.current = mockInput.handle;

		act(() => {
			result.current.handleEditUserMessage(7, "edited message");
		});

		expect(result.current.editingMessageId).toBe(7);
		expect(mockInput.setValue).toHaveBeenCalledWith("edited message");

		act(() => {
			result.current.handleCancelHistoryEdit();
		});

		expect(result.current.editingMessageId).toBeNull();
		expect(mockInput.setValue).toHaveBeenLastCalledWith("work in progress");
		expect(mockInput.currentValue.value).toBe("work in progress");
		unmount();
	});

	it("can load the same edit text again after send without relying on a remount", async () => {
		const { result, onSend, unmount } = renderEditing();
		const mockInput = createMockChatInputHandle();
		result.current.chatInputRef.current = mockInput.handle;

		act(() => {
			result.current.handleEditUserMessage(7, "hello");
		});

		await act(async () => {
			await result.current.handleSendFromInput("hello");
		});

		act(() => {
			result.current.handleEditUserMessage(7, "hello");
		});

		expect(onSend).toHaveBeenCalledWith("hello", undefined, 7);
		expect(mockInput.setValue).toHaveBeenNthCalledWith(1, "hello");
		expect(mockInput.setValue).toHaveBeenNthCalledWith(2, "hello");
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
});

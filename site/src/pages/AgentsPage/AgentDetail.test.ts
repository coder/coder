import { createTestQueryClient } from "testHelpers/renderHelpers";
import { act, renderHook } from "@testing-library/react";
import { chatKey, infiniteChats } from "api/queries/chats";
import type * as TypesGen from "api/typesGenerated";
import { createElement, createRef, type ReactNode } from "react";
import { QueryClientProvider } from "react-query";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ChatMessageInputRef } from "./AgentChatInput";
import {
	draftInputStorageKeyPrefix,
	resolveWorkspaceId,
	useCachedWorkspaceId,
	useConversationEditingState,
} from "./AgentDetail";

const makeChat = (chatID: string, workspaceID?: string): TypesGen.Chat => ({
	id: chatID,
	owner_id: "owner-1",
	workspace_id: workspaceID,
	last_model_config_id: "model-1",
	mcp_server_ids: [],
	title: "test",
	status: "running",
	created_at: "2025-01-01T00:00:00.000Z",
	updated_at: "2025-01-01T00:00:00.000Z",
	archived: false,
	last_error: null,
});

const createQueryClientWrapper = (
	queryClient: ReturnType<typeof createTestQueryClient>,
) => {
	return ({ children }: { children: ReactNode }) =>
		createElement(QueryClientProvider, { client: queryClient }, children);
};

describe("useCachedWorkspaceId", () => {
	const chatID = "chat-abc-123";
	const workspaceID = "workspace-abc-123";

	const renderCachedWorkspaceId = (
		queryClient = createTestQueryClient(),
		id: string | undefined = chatID,
	) => {
		return renderHook(() => useCachedWorkspaceId(id), {
			wrapper: createQueryClientWrapper(queryClient),
		});
	};

	it("returns the workspace_id from the per-chat cache", () => {
		const queryClient = createTestQueryClient();
		queryClient.setQueryData(chatKey(chatID), makeChat(chatID, workspaceID));

		const { result } = renderCachedWorkspaceId(queryClient);

		expect(result.current).toBe(workspaceID);
	});

	it("falls back to the infinite chats cache when the per-chat cache is empty", () => {
		const queryClient = createTestQueryClient();
		queryClient.setQueryData(infiniteChats().queryKey, {
			pages: [[makeChat(chatID, workspaceID)]],
			pageParams: [0],
		});

		const { result } = renderCachedWorkspaceId(queryClient);

		expect(result.current).toBe(workspaceID);
	});

	it("returns undefined when the chat is missing from every cache", () => {
		const { result } = renderCachedWorkspaceId();

		expect(result.current).toBeUndefined();
	});

	it("does not leak workspace IDs from other cached chats", () => {
		const queryClient = createTestQueryClient();
		queryClient.setQueryData(
			chatKey("chat-other"),
			makeChat("chat-other", "workspace-other"),
		);
		queryClient.setQueryData(infiniteChats().queryKey, {
			pages: [[makeChat("chat-other", "workspace-other")]],
			pageParams: [0],
		});

		const { result } = renderCachedWorkspaceId(queryClient);

		expect(result.current).toBeUndefined();
	});
});

describe("resolveWorkspaceId", () => {
	it("uses the live chat record once it is available", () => {
		expect(
			resolveWorkspaceId({
				chatRecord: makeChat("chat-1", "workspace-live"),
				cachedWorkspaceId: "workspace-cached",
				hasResolvedChatQuery: true,
			}),
		).toBe("workspace-live");
	});

	it("uses the cached workspace while the chat query is still pending", () => {
		expect(
			resolveWorkspaceId({
				chatRecord: undefined,
				cachedWorkspaceId: "workspace-cached",
				hasResolvedChatQuery: false,
			}),
		).toBe("workspace-cached");
	});

	it("drops the cached workspace once the chat query resolves without a record", () => {
		expect(
			resolveWorkspaceId({
				chatRecord: undefined,
				cachedWorkspaceId: "workspace-cached",
				hasResolvedChatQuery: true,
			}),
		).toBeUndefined();
	});
});

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
});

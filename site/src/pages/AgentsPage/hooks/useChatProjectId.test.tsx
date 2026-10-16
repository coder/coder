import { renderHook, waitFor } from "@testing-library/react";
import { QueryClientProvider } from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type { Chat } from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { useChatProjectId } from "./useChatProjectId";

const mockRootChat: Chat = {
	...MockChat,
	id: "root-chat",
	project_id: "project-1",
};
const mockSubagentChat: Chat = {
	...MockChat,
	id: "subagent-chat",
	parent_chat_id: mockRootChat.id,
	root_chat_id: mockRootChat.id,
};

const renderUseChatProjectId = (chat: Chat, enabled = true) => {
	const queryClient = createTestQueryClient();
	return renderHook(() => useChatProjectId(chat, enabled), {
		wrapper: ({ children }: React.PropsWithChildren) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		),
	});
};

afterEach(() => {
	vi.restoreAllMocks();
});

describe("useChatProjectId", () => {
	it("resolves no project when the root chat fails to load", async () => {
		vi.spyOn(API.experimental, "getChat").mockRejectedValue(new Error("boom"));

		const { result } = renderUseChatProjectId(mockSubagentChat);

		await waitFor(() => {
			expect(result.current).toEqual({
				projectId: undefined,
				isResolved: true,
			});
		});
	});

	it("uses the chat's own project without fetching the root chat", () => {
		const getChat = vi.spyOn(API.experimental, "getChat");

		const { result } = renderUseChatProjectId({
			...mockSubagentChat,
			project_id: "project-2",
		});

		expect(result.current).toEqual({
			projectId: "project-2",
			isResolved: true,
		});
		expect(getChat).not.toHaveBeenCalled();
	});

	it("resolves no project while the chat-projects experiment is off", () => {
		const getChat = vi.spyOn(API.experimental, "getChat");

		const { result } = renderUseChatProjectId(mockSubagentChat, false);

		expect(result.current).toEqual({ projectId: undefined, isResolved: true });
		expect(getChat).not.toHaveBeenCalled();
	});
});

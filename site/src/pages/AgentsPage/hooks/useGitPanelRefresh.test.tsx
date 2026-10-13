import { renderHook, waitFor } from "@testing-library/react";
import { QueryClientProvider, QueryObserver } from "react-query";
import { describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { chat as chatById, chatDiffContents } from "#/api/queries/chats";
import { MockChat } from "#/testHelpers/chatEntities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { useGitPanelRefresh } from "./useGitPanelRefresh";

describe("useGitPanelRefresh", () => {
	it.each([
		{ name: "the git watch socket is connected", sent: true },
		{ name: "the git watch socket is down", sent: false },
	])(
		"refetches the chat and its diff contents when $name",
		async ({ sent }) => {
			const getChat = vi
				.spyOn(API.experimental, "getChat")
				.mockResolvedValue(MockChat);
			const getChatDiffContents = vi
				.spyOn(API.experimental, "getChatDiffContents")
				.mockResolvedValue({ chat_id: MockChat.id });
			const queryClient = createTestQueryClient();
			// Stand in for the chat page and an open diff panel, so the
			// invalidated queries are refetched.
			const unsubscribes = [
				new QueryObserver(queryClient, chatById(MockChat.id)).subscribe(
					() => {},
				),
				new QueryObserver(queryClient, chatDiffContents(MockChat.id)).subscribe(
					() => {},
				),
			];
			try {
				await waitFor(() => expect(getChat).toHaveBeenCalledTimes(1));
				await waitFor(() =>
					expect(getChatDiffContents).toHaveBeenCalledTimes(1),
				);

				const refreshGitWatcher = vi.fn(() => sent);
				const { result } = renderHook(
					() => useGitPanelRefresh(MockChat.id, refreshGitWatcher),
					{
						wrapper: ({ children }) => (
							<QueryClientProvider client={queryClient}>
								{children}
							</QueryClientProvider>
						),
					},
				);

				expect(result.current()).toBe(sent);
				expect(refreshGitWatcher).toHaveBeenCalledTimes(1);
				await waitFor(() => expect(getChat).toHaveBeenCalledTimes(2));
				expect(getChat.mock.lastCall?.[0]).toBe(MockChat.id);
				await waitFor(() =>
					expect(getChatDiffContents).toHaveBeenCalledTimes(2),
				);
				expect(getChatDiffContents.mock.lastCall?.[0]).toBe(MockChat.id);
			} finally {
				for (const unsubscribe of unsubscribes) {
					unsubscribe();
				}
			}
		},
	);
});

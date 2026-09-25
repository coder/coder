import { renderHook, waitFor } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import {
	type InfiniteData,
	QueryClient,
	QueryClientProvider,
	useInfiniteQuery,
} from "react-query";
import { describe, expect, it, vi } from "vitest";
import { chatListFamilyKey } from "#/api/queries/chats";
import type { Chat } from "#/api/typesGenerated";
import { createDeferred, type Deferred } from "#/testHelpers/deferred";
import { boardChats, boardChatsKey } from "./boardChats";
import { refetchChatListUntilLanded } from "./refreshChatList";

// A mounted board list whose first fetch has landed; each later fetch waits
// in `fetches` until the test resolves it.
const renderBoardList = async () => {
	const queryClient = new QueryClient({
		defaultOptions: { queries: { retry: false } },
	});
	const wrapper = ({ children }: PropsWithChildren) => (
		<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
	);
	const fetches: Deferred<Chat[]>[] = [];
	renderHook(
		() =>
			useInfiniteQuery({
				...boardChats(),
				retry: false,
				queryFn: () => {
					const fetch = createDeferred<Chat[]>();
					fetches.push(fetch);
					return fetch.promise;
				},
			}),
		{ wrapper },
	);
	await waitFor(() => expect(fetches).toHaveLength(1));
	fetches[0]?.resolve([]);
	await waitFor(() =>
		expect(queryClient.getQueryState(boardChatsKey)?.fetchStatus).toBe("idle"),
	);
	return { queryClient, fetches };
};

// What AgentsPageLayout does on every chat watch event.
const watchEvent = (queryClient: QueryClient) =>
	queryClient.cancelQueries({ queryKey: chatListFamilyKey });

describe("refetchChatListUntilLanded", () => {
	it("asks again when a watch event cancels the refetch, and stops once data lands", async () => {
		const { queryClient, fetches } = await renderBoardList();

		const stop = refetchChatListUntilLanded(queryClient);
		await waitFor(() => expect(fetches).toHaveLength(2));

		// A cache patch is not the response being waited for.
		queryClient.setQueryData<InfiniteData<Chat[]>>(
			boardChatsKey,
			(old) => old && { ...old },
		);
		await watchEvent(queryClient);
		await waitFor(() => expect(fetches).toHaveLength(3));

		fetches[2]?.resolve([]);
		await waitFor(() =>
			expect(queryClient.getQueryState(boardChatsKey)?.fetchStatus).toBe(
				"idle",
			),
		);
		await watchEvent(queryClient);
		expect(fetches).toHaveLength(3);
		stop();
	});

	it("stops asking after three retries", async () => {
		const { queryClient, fetches } = await renderBoardList();

		const stop = refetchChatListUntilLanded(queryClient);
		for (const pending of [2, 3, 4, 5]) {
			await waitFor(() => expect(fetches).toHaveLength(pending));
			await watchEvent(queryClient);
		}
		expect(fetches).toHaveLength(5);
		stop();
	});

	it("invalidates once and does not listen when nothing observes the board list", () => {
		const queryClient = new QueryClient();
		const invalidate = vi.spyOn(queryClient, "invalidateQueries");
		const subscribe = vi.spyOn(queryClient.getQueryCache(), "subscribe");

		refetchChatListUntilLanded(queryClient);

		expect(invalidate).toHaveBeenCalledTimes(1);
		expect(invalidate).toHaveBeenCalledWith({ queryKey: chatListFamilyKey });
		expect(subscribe).not.toHaveBeenCalled();
	});
});

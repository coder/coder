import { renderHook, waitFor } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import {
	QueryClient,
	QueryClientProvider,
	useInfiniteQuery,
} from "react-query";
import { describe, expect, it } from "vitest";
import { chatListFamilyKey } from "#/api/queries/chats";
import type { Chat } from "#/api/typesGenerated";
import { createDeferred, type Deferred } from "#/testHelpers/deferred";
import { boardChats, boardChatsKey } from "./boardChats";
import { refetchChatListUntilLanded } from "./refreshChatList";

describe("refetchChatListUntilLanded", () => {
	it("asks again when a watch event cancels the refetch, and stops once data lands", async () => {
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
			expect(queryClient.getQueryState(boardChatsKey)?.fetchStatus).toBe(
				"idle",
			),
		);

		const stop = refetchChatListUntilLanded(queryClient);
		await waitFor(() => expect(fetches).toHaveLength(2));

		// What AgentsPageLayout does on every chat watch event.
		await queryClient.cancelQueries({ queryKey: chatListFamilyKey });
		await waitFor(() => expect(fetches).toHaveLength(3));

		fetches[2]?.resolve([]);
		await waitFor(() =>
			expect(queryClient.getQueryState(boardChatsKey)?.fetchStatus).toBe(
				"idle",
			),
		);
		await queryClient.cancelQueries({ queryKey: chatListFamilyKey });
		expect(fetches).toHaveLength(3);
		stop();
	});
});

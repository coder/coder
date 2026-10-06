import { hashKey, type QueryClient } from "react-query";
import { invalidateChatListQueries } from "#/api/queries/chats";
import { boardChatsKey } from "./boardChats";

// Watch events cancel in-flight list refetches without restarting them, so
// one invalidation may never land. Bounded so a busy board cannot loop.
const MAX_RETRIES = 3;

/**
 * Invalidates the chat lists and, while the board's refetch keeps getting
 * cancelled, asks again. Stops listening once a network response lands or
 * the retries run out. Returns a function that stops listening early.
 *
 * With no observer on the board list (the board unmounted), invalidation
 * only marks it stale and no response ever arrives, so it invalidates once
 * and does not subscribe.
 */
export const refetchChatListUntilLanded = (
	queryClient: QueryClient,
): (() => void) => {
	const observers =
		queryClient
			.getQueryCache()
			.find({ queryKey: boardChatsKey })
			?.getObserversCount() ?? 0;
	if (observers === 0) {
		void invalidateChatListQueries(queryClient);
		return () => undefined;
	}
	const boardHash = hashKey(boardChatsKey);
	let retries = 0;
	const unsubscribe = queryClient.getQueryCache().subscribe((event) => {
		if (event.type !== "updated" || event.query.queryHash !== boardHash) {
			return;
		}
		const { action } = event;
		// A manual success is a cache patch (label write, watch event), not
		// the network response this is waiting for.
		if (action.type === "success" && !action.manual) {
			unsubscribe();
		} else if (action.type === "error") {
			// With no observer left (the board unmounted while fetching), an
			// invalidation starts no fetch, so no response would ever end this.
			if (retries >= MAX_RETRIES || event.query.getObserversCount() === 0) {
				unsubscribe();
				return;
			}
			retries += 1;
			void invalidateChatListQueries(queryClient);
		}
	});
	void invalidateChatListQueries(queryClient);
	return unsubscribe;
};

import { hashKey, type InfiniteData, type QueryClient } from "react-query";
import type * as TypesGen from "#/api/typesGenerated";

type MessagesCacheData = InfiniteData<TypesGen.ChatMessagesResponse>;

/**
 * One write against a chat's messages cache.
 *
 * `apply` is an updater over whatever the cache holds when the write lands, not
 * a precomputed snapshot, so a buffered write replayed after a fetch resolved
 * amends the fetched pages instead of overwriting them.
 */
export type MessagesCacheWrite = {
	// Names the write for dev diagnostics. Replay is FIFO regardless of kind.
	kind: string;
	apply: (currentData: MessagesCacheData) => MessagesCacheData;
};

type MessagesCacheCoordinator = {
	buffered: MessagesCacheWrite[];
	unsubscribe: (() => void) | null;
};

/**
 * Per-query-client, per-chat write coordinators. A WeakMap keyed by the query
 * client keeps test clients from leaking into each other and lets a discarded
 * client be collected with its coordinators.
 */
const coordinators = new WeakMap<
	QueryClient,
	Map<string, MessagesCacheCoordinator>
>();

const coordinatorFor = (
	queryClient: QueryClient,
	queryKey: readonly unknown[],
): MessagesCacheCoordinator => {
	let byKey = coordinators.get(queryClient);
	if (!byKey) {
		byKey = new Map();
		coordinators.set(queryClient, byKey);
	}
	const hash = hashKey(queryKey);
	let coordinator = byKey.get(hash);
	if (!coordinator) {
		coordinator = { buffered: [], unsubscribe: null };
		byKey.set(hash, coordinator);
	}
	return coordinator;
};

/**
 * True while a fetch owns the cache entry.
 *
 * Checks `fetchStatus` rather than `isFetching` so a PAUSED fetch counts: the
 * infinite-query behavior captured the existing pages before pausing and still
 * installs them verbatim when it resumes, so a write applied during the pause
 * would be discarded.
 */
const isMessagesFetchInFlight = (
	queryClient: QueryClient,
	queryKey: readonly unknown[],
): boolean => {
	const fetchStatus = queryClient.getQueryState(queryKey)?.fetchStatus;
	return fetchStatus !== undefined && fetchStatus !== "idle";
};

const applyMessagesCacheWrite = (
	queryClient: QueryClient,
	queryKey: readonly unknown[],
	write: MessagesCacheWrite,
): void => {
	queryClient.setQueryData<MessagesCacheData | undefined>(
		queryKey,
		(currentData) => {
			if (!currentData?.pages?.length) {
				// An initialized cache is required. Seeding a synthetic partial page
				// would mark the query successful, and under staleTime: Infinity the
				// first observer would treat that page as fresh and skip the initial
				// history fetch, hiding everything the write did not carry.
				if (process.env.NODE_ENV !== "production") {
					console.warn(
						`[chatMessagesCache] dropped a "${write.kind}" write: ` +
							"the messages cache has no pages yet.",
					);
				}
				return currentData;
			}
			return write.apply(currentData);
		},
	);
};

/**
 * Writes to a chat's messages cache, serialized against fetches on the same
 * cache entry.
 *
 * A fetch captures the pages when it starts and installs its result
 * unconditionally, so anything written in between would be clobbered: a
 * `fetchNextPage`, a deliberate refetch, and an invalidation-driven refetch all
 * behave this way. While a fetch is in flight, writes buffer and replay in
 * arrival order once it settles; the replay lands on top of the fetched pages.
 *
 * Cancelling the fetch instead would lose the write: `cancelQueries` defaults
 * to `{ revert: true }` and restores the pre-fetch snapshot asynchronously.
 *
 * Every writer for a chat has to go through here, otherwise the ordering
 * guarantee only covers part of the writes. Current writers: the per-chat
 * socket (durable messages, history replacement, queue snapshots), the queued
 * message mutations, and the message edit mutation.
 */
export const writeMessagesCache = (
	queryClient: QueryClient,
	queryKey: readonly unknown[],
	write: MessagesCacheWrite,
): void => {
	const coordinator = coordinatorFor(queryClient, queryKey);
	// A non-empty buffer means earlier writes are still waiting, so this one
	// queues behind them even if the fetch already settled. Applying it now
	// would reorder it ahead of them.
	if (
		coordinator.buffered.length === 0 &&
		!isMessagesFetchInFlight(queryClient, queryKey)
	) {
		applyMessagesCacheWrite(queryClient, queryKey, write);
		return;
	}
	coordinator.buffered.push(write);
	if (coordinator.unsubscribe) {
		return;
	}
	// The cache notifies after the fetch installs its data, so by the time this
	// runs the resolved pages are already in place.
	coordinator.unsubscribe = queryClient.getQueryCache().subscribe(() => {
		if (isMessagesFetchInFlight(queryClient, queryKey)) {
			return;
		}
		// Detach first so the replay's own cache events cannot re-enter.
		coordinator.unsubscribe?.();
		coordinator.unsubscribe = null;
		for (const buffered of coordinator.buffered.splice(0)) {
			applyMessagesCacheWrite(queryClient, queryKey, buffered);
		}
	});
};

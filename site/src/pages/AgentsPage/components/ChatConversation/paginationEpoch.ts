// While the user scrolls up, we fetch an older page of messages. A
// WebSocket update can arrive mid-fetch. We write it into the cache
// right away, but when the page fetch finishes, the library overwrites
// the whole cache with what it saw when the fetch started, plus the
// new page, and our update is lost. (fetchNextPage in
// @tanstack/query-core 5.82.0; upstream TanStack/query#3579.)
//
// The fix: while a page fetch is in flight, keep a list of every such
// write, one list per chat. When the fetch finishes, apply the list
// again on top of what the fetch wrote. The list is called an epoch.
//
// Background refetches are handled differently: they get cancelled
// (cancelChatListRefetches). A page fetch is user-requested, so it is
// allowed to finish instead.

type PaginationEpochState<TWrite> = {
	// Page fetches in flight for this chat.
	refCount: number;
	// Identifies this epoch, so a close from an earlier epoch is ignored.
	generation: number;
	// Writes to re-apply after the last fetch finishes.
	buffer: TWrite[];
};

export const createPaginationEpochManager = <TWrite>() => {
	const epochs = new Map<string, PaginationEpochState<TWrite>>();
	let generationCounter = 0;

	return {
		// A page fetch is starting. If one is already in flight for this
		// chat, just count it; otherwise start a fresh list of writes.
		open: (chatID: string): number => {
			const epoch = epochs.get(chatID);
			if (epoch) {
				epoch.refCount += 1;
				return epoch.generation;
			}
			const generation = ++generationCounter;
			epochs.set(chatID, { refCount: 1, generation, buffer: [] });
			return generation;
		},

		// Add a write to the list for the in-flight fetch. If no fetch is
		// in flight, nothing can clobber the write, so this is a no-op.
		record: (chatID: string, write: TWrite): void => {
			epochs.get(chatID)?.buffer.push(write);
		},

		// A page fetch finished. Calling fetchNextPage again mid-fetch
		// cancels the first fetch, but both calls still finish, so only
		// the last fetch standing hands the list back for re-application.
		// A stale generation (an epoch long gone) is ignored.
		close: (
			chatID: string,
			generation: number,
		): readonly TWrite[] | undefined => {
			const epoch = epochs.get(chatID);
			if (!epoch || epoch.generation !== generation) {
				return undefined;
			}
			epoch.refCount -= 1;
			if (epoch.refCount > 0) {
				return undefined;
			}
			epochs.delete(chatID);
			return epoch.buffer;
		},
	};
};

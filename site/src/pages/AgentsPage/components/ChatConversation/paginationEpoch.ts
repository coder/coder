// fetchNextPage in @tanstack/query-core 5.82.0 (upstream
// TanStack/query#3579) snapshots state.data.pages when the fetch
// starts and settles with snapshot + new page, discarding every
// setQueryData made in between. While a per-chat pagination epoch is
// open, cache writes are applied immediately and recorded in the
// epoch buffer; when the epoch closes, the buffered writes replay in
// order against the settled cache. Background refetches instead use
// the cancel convention (cancelChatListRefetches); a pagination fetch
// is user-requested, so it must settle rather than be cancelled.

type PaginationEpochState<TWrite> = {
	refCount: number;
	generation: number;
	buffer: TWrite[];
};

// Refcounted because a cancelled fetchNextPage still settles: only
// the close that drains the refcount replays. The generation token
// makes a stale close a no-op.
export const createPaginationEpochManager = <TWrite>() => {
	const epochs = new Map<string, PaginationEpochState<TWrite>>();
	let generationCounter = 0;
	return {
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
		// No-op when no epoch is open for the chat.
		record: (chatID: string, write: TWrite): void => {
			epochs.get(chatID)?.buffer.push(write);
		},
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

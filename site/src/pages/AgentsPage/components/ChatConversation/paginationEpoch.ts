// fetchNextPage snapshots state.data.pages when the fetch starts and
// settles with snapshot + new page, discarding every setQueryData made
// in between. While a per-chat pagination epoch is open, cache writes
// are applied immediately and recorded in the epoch buffer; when the
// epoch closes, the buffered writes replay in order against the settled
// cache. A replacement supersedes every buffered write before it.
export type PaginationCacheWrite = { kind: string };

export const applySupersededWrites = <TWrite extends PaginationCacheWrite>(
	writes: readonly TWrite[],
): readonly TWrite[] => {
	const lastReplacementIndex = writes.findLastIndex(
		(write) => write.kind === "replace",
	);
	return lastReplacementIndex === -1
		? writes
		: writes.slice(lastReplacementIndex);
};

type PaginationEpochState<TWrite> = {
	refCount: number;
	generation: number;
	buffer: TWrite[];
};

// Keyed by chat ID so concurrent chats never share an epoch. Refcounted
// because a cancelled fetchNextPage still settles: only the call that
// closes the epoch replays. The generation token makes a stale close a
// no-op if it ever lands after the epoch was replaced.
export const createPaginationEpochManager = <
	TWrite extends PaginationCacheWrite,
>() => {
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
			return applySupersededWrites(epoch.buffer);
		},
	};
};

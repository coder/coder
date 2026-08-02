/**
 * Serializes the LOCAL mutations that change a chat's queue positions: send,
 * edit, delete, and promote.
 *
 * A send on an errored chat promotes the server's queue head and reports which
 * row that was, and the client projects the resulting queue into the canonical
 * cache. Letting a delete or a promote commit inside that window interleaves
 * two writers of the same cached queue, so one of them projects a queue the
 * other already changed. Running one queue-position request at a time per chat
 * removes the overlap instead of tracking per-operation ownership.
 *
 * The critical section covers the WHOLE operation, its suppression markers
 * included. A marker placed before the lock belongs to an operation that has
 * not started yet, so its failure path can lift a marker the operation holding
 * the lock is relying on. The cost is that an optimistic removal waits for the
 * request ahead of it; the correctness of the promoted-head veto is worth more
 * than shaving that wait.
 */
const pendingByChatID = new Map<string, Promise<unknown>>();

export const runExclusiveQueueMutation = <T>(
	chatID: string,
	run: () => Promise<T>,
): Promise<T> => {
	const previous = pendingByChatID.get(chatID);
	// Uncontended, which is the normal case: start now rather than a microtask
	// later. Both arms of the chained form run `run`, because a failed
	// predecessor must not strand the queue.
	const result = previous ? previous.then(run, run) : run();
	const settled = result.then(
		() => undefined,
		() => undefined,
	);
	pendingByChatID.set(chatID, settled);
	void settled.then(() => {
		if (pendingByChatID.get(chatID) === settled) {
			pendingByChatID.delete(chatID);
		}
	});
	return result;
};

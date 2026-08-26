import { useCallback, useSyncExternalStore } from "react";
import type { PersistResult, StorageKeyHandle } from "#/storage";

/**
 * Reactive hook for a storage key handle. All consumers of the same
 * key re-render on change, in the same tab and across tabs. The setter accepts a value
 * or an updater function; updaters are evaluated against the storage
 * snapshot, not the render closure, so calls compose before a render
 * commits and never clobber a newer value written by another tab. The
 * setter never writes during render; call it from event handlers.
 * Defaults are never written to storage on mount.
 */
export function useStorage<T>(
	handle: StorageKeyHandle<T>,
): [T, (value: T | ((prev: T) => T)) => PersistResult, () => void] {
	const value = useSyncExternalStore(handle.subscribe, handle.getSnapshot);
	const set = useCallback(
		(next: T | ((prev: T) => T)) =>
			handle.set(
				// SAFETY: stored values are serializable data, never
				// functions, so a function argument is always an updater.
				typeof next === "function"
					? (next as (prev: T) => T)(handle.get())
					: next,
			),
		[handle],
	);
	return [value, set, handle.remove];
}

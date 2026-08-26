import { useCallback, useSyncExternalStore } from "react";
import type { PersistResult, StorageKeyHandle } from "#/storage";

/**
 * Reactive hook for a storage key. All consumers of the key re-render
 * on change, same-tab and cross-tab. Updater functions receive the
 * stored value, not the render closure's. Call the setter from event
 * handlers; it must not run during render.
 */
export function useStorage<T>(
	handle: StorageKeyHandle<T>,
): [T, (value: T | ((prev: T) => T)) => PersistResult, () => void] {
	const value = useSyncExternalStore(handle.subscribe, handle.getSnapshot);
	const set = useCallback(
		(next: T | ((prev: T) => T)) =>
			handle.set(
				// Storage values are plain data, never functions, so a
				// function argument can only be the updater form.
				typeof next === "function"
					? (next as (prev: T) => T)(handle.get())
					: next,
			),
		[handle],
	);
	return [value, set, handle.remove];
}

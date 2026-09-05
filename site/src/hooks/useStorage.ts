import { useCallback, useSyncExternalStore } from "react";
import type { PersistResult, StorageKeyHandle } from "#/storage";

/**
 * Reactive hook for a storage key. All consumers of the key re-render
 * on change, same-tab and cross-tab. Updater functions receive the
 * stored value, not the render closure's. Call the setter from event
 * handlers; it must not run during render.
 */
export function useStorage<T>(
	// Storage holds serialized plain data, so callable types are not
	// valid storage values; rejecting them keeps the setter's updater
	// detection sound.
	handle: StorageKeyHandle<T> &
		(Extract<T, (...args: never[]) => unknown> extends never
			? unknown
			: { storageValuesMustNotBeCallable: never }),
): [T, (value: T | ((prev: T) => T)) => PersistResult, () => PersistResult] {
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

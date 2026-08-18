import { useSyncExternalStore } from "react";
import type { PersistResult, StorageKeyHandle } from "#/utils/storage/storage";

/**
 * Reactive hook for a registered storage key (see
 * utils/storage/keys.ts). All consumers of the same key re-render on
 * change, in the same tab and across tabs. The setter never writes
 * during render; call it from event handlers. Defaults are never
 * written to storage on mount.
 */
export function useStorage<T>(
	handle: StorageKeyHandle<T>,
): [T, (value: T) => PersistResult, () => void] {
	const value = useSyncExternalStore(handle.subscribe, handle.getSnapshot);
	return [value, handle.set, handle.remove];
}

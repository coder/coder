import { useCallback, useSyncExternalStore } from "react";
import type { PersistResult, StorageKeyHandle } from "#/storage";

/** Reads reactive browser storage. Setters must not run during render. */
export function useStorage<T>(
	handle: StorageKeyHandle<T> &
		(Extract<T, (...args: never[]) => unknown> extends never
			? unknown
			: { storageValuesMustNotBeCallable: never }),
): [
	T,
	(value: NonNullable<T> | ((prev: T) => NonNullable<T>)) => PersistResult,
	() => PersistResult,
] {
	const value = useSyncExternalStore(handle.subscribe, handle.getSnapshot);
	const set = useCallback(
		(next: NonNullable<T> | ((prev: T) => NonNullable<T>)) =>
			handle.set(
				typeof next === "function"
					? (next as (prev: T) => NonNullable<T>)(handle.get())
					: next,
			),
		[handle],
	);
	return [value, set, handle.remove];
}

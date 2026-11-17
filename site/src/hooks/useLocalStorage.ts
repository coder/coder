import { useCallback, useEffect, useSyncExternalStore } from "react";
import type * as Yup from "yup";
import {
	type LocalStorageStore,
	useLocalStorageStore,
} from "#/contexts/LocalStorageProvider";

/**
 * Converts between a serialized string and a typed value. `parse` receives
 * `null` when the key is absent. `serialize` returns `null` to remove the key.
 */
export type Codec<T> = {
	parse: (serialized: string | null) => T;
	serialize: (value: T) => string | null;
};

/**
 * Reads and parses a key without subscribing, reflecting unflushed writes. Use
 * it when a component reads once per render and does not need to update.
 */
export function getLocalStorageValue<T>(
	store: LocalStorageStore,
	key: string,
	codec: Codec<T>,
): T {
	return codec.parse(store.readSerialized(key));
}

function isUpdater<T>(next: React.SetStateAction<T>): next is (prev: T) => T {
	return typeof next === "function";
}

/**
 * Reads and writes a localStorage key. Consumers re-render on same-tab writes
 * and on cross-tab storage events. With `debounceMs`, only the storage write
 * is delayed; pending writes flush on `pagehide`, unmount, and key change.
 */
export function useLocalStorage<T>(
	key: string,
	codec: Codec<T>,
	options?: { debounceMs?: number },
): [T, React.Dispatch<React.SetStateAction<T>>] {
	const store = useLocalStorageStore();
	const debounceMs = options?.debounceMs ?? 0;

	// A stable subscribe keeps useSyncExternalStore from resubscribing on every render.
	const subscribeToKey = useCallback(
		(onChange: () => void) => store.subscribe(key, onChange),
		[store, key],
	);
	const serialized = useSyncExternalStore(subscribeToKey, () =>
		store.readSerialized(key),
	);

	useEffect(() => {
		const flushOnPageHide = () => store.flushWrite(key);
		window.addEventListener("pagehide", flushOnPageHide);

		return () => {
			window.removeEventListener("pagehide", flushOnPageHide);
			store.flushWrite(key);
		};
	}, [store, key]);

	const setValue = (next: React.SetStateAction<T>) => {
		const value = isUpdater(next)
			? next(codec.parse(store.readSerialized(key)))
			: next;

		store.commitSerialized(key, codec.serialize(value), debounceMs);
	};

	return [codec.parse(serialized), setValue];
}

/**
 * Codec for a boolean stored as "true" or "false". An absent key reads as
 * `fallback`. Any other stored value reads as `false`.
 */
export function booleanCodec(fallback: boolean): Codec<boolean> {
	return {
		parse: (serialized) =>
			serialized === null ? fallback : serialized === "true",
		serialize: (value) => String(value),
	};
}

/**
 * Codec for a JSON value validated by a Yup schema. Invalid stored values read
 * as `fallback`. Schema defaults fill fields older writers did not store. The
 * memo keeps the returned object stable until the stored value changes.
 */
export function jsonCodec<T>(schema: Yup.Schema<T>, fallback: T): Codec<T> {
	let memo: { serialized: string | null; value: T } | undefined;

	const parseJSON = (serialized: string | null): T => {
		if (serialized === null) {
			return fallback;
		}

		try {
			return schema.validateSync(JSON.parse(serialized));
		} catch {
			return fallback;
		}
	};

	return {
		parse: (serialized) => {
			if (memo === undefined || memo.serialized !== serialized) {
				memo = { serialized, value: parseJSON(serialized) };
			}

			return memo.value;
		},
		serialize: (value) => JSON.stringify(value),
	};
}

import { useCallback, useSyncExternalStore } from "react";
import type * as Yup from "yup";
import { useLocalStorageStore } from "#/contexts/LocalStorageProvider";

/**
 * Converts between a serialized string and a typed value. `parse` receives
 * `null` when the key is absent. `serialize` returns `null` to remove the key.
 */
export type Codec<T> = {
	parse: (serialized: string | null) => T;
	serialize: (value: T) => string | null;
};

function isUpdater<T>(next: React.SetStateAction<T>): next is (prev: T) => T {
	return typeof next === "function";
}

type UseLocalStorageOptions<T> = {
	storageKey: string;
	codec: Codec<T>;
};

/**
 * Identity helper that infers `T` from the codec, so options can be declared
 * once at module scope and shared by every `useLocalStorage` call.
 */
export function localStorageOptions<T>(
	options: UseLocalStorageOptions<T>,
): UseLocalStorageOptions<T> {
	return options;
}

/**
 * Reads and writes a localStorage key. Consumers re-render on same-tab writes
 * and on cross-tab storage events.
 */
export function useLocalStorage<T>({
	storageKey: key,
	codec,
}: UseLocalStorageOptions<T>): [T, React.Dispatch<React.SetStateAction<T>>] {
	const store = useLocalStorageStore();

	// Memoized so useSyncExternalStore does not resubscribe on every render.
	const subscribeToKey = useCallback(
		(onChange: () => void) => store.subscribe(key, onChange),
		[store, key],
	);
	const serialized = useSyncExternalStore(subscribeToKey, () =>
		store.read(key),
	);

	const setValue = useCallback(
		(next: React.SetStateAction<T>) => {
			const value = isUpdater(next) ? next(codec.parse(store.read(key))) : next;

			store.write(key, codec.serialize(value));
		},
		[store, key, codec],
	);

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
 * parsed object keeps its identity until the stored string changes, so it is
 * safe to use in dependency arrays.
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

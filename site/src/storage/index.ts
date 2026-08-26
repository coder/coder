/**
 * Browser storage core.
 *
 * Every persisted UI preference goes through this module. It provides
 * never-throwing primitives around localStorage/sessionStorage, typed
 * key handles with parse-validation, and change notification for
 * same-tab and cross-tab reactivity (the native "storage" event only
 * fires in other tabs).
 *
 * Key handles are defined next to the feature that reads them; this
 * module only provides the building blocks.
 */

import type { Schema } from "yup";

export type PersistResult =
	| { ok: true }
	| { ok: false; reason: "quota" | "unavailable" };

type StorageArea = "local" | "session";

/**
 * Encodes a typed value to the raw string stored in the browser and
 * decodes it back. `decode` returns undefined for invalid or corrupt
 * input so reads fall back to the key's default value. T is the
 * non-null value type; key handles layer null-for-absence on top.
 */
export type StorageCodec<T> = {
	decode: (raw: string) => T | undefined;
	encode: (value: T) => string;
};

export type StorageKeyHandle<T> = {
	readonly key: string;
	/** Read the stored value, falling back to the default when absent or invalid. */
	get: () => T;
	/**
	 * Persist a value. Passing null or undefined removes the key,
	 * matching localStorage semantics where absence is the null state.
	 */
	set: (value: T) => PersistResult;
	remove: () => void;
	/** Subscribe to same-tab and cross-tab changes of this key. */
	subscribe: (listener: () => void) => () => void;
	/**
	 * Referentially stable read for useSyncExternalStore: repeated
	 * calls return the same object until the stored bytes change.
	 */
	getSnapshot: () => T;
};

type EntityStorageKey<T> = {
	readonly prefix: string;
	forId: (...idParts: string[]) => StorageKeyHandle<T>;
	/** Key suffixes (the part after `prefix`) currently stored in localStorage. */
	listStoredSuffixes: () => string[];
	/** Remove every stored key in this family owned by the given entity ID. */
	clear: (id: string) => void;
};

// -- Codecs -----------------------------------------------------------------

export const stringCodec: StorageCodec<string> = {
	decode: (raw) => raw,
	encode: (value) => value,
};

export const booleanCodec: StorageCodec<boolean> = {
	decode: (raw) =>
		raw === "true" ? true : raw === "false" ? false : undefined,
	encode: (value) => String(value),
};

export const integerCodec: StorageCodec<number> = {
	decode: (raw) => {
		const parsed = Number.parseInt(raw, 10);
		return Number.isNaN(parsed) ? undefined : parsed;
	},
	encode: (value) => String(value),
};

/** Codec for a union of string literals; any other stored value decodes to the default. */
export const stringLiteralCodec = <T extends string>(options: {
	oneOf: readonly T[];
}): StorageCodec<T> => ({
	decode: (raw) => options.oneOf.find((option) => option === raw),
	encode: (value) => value,
});

export const jsonCodec = <T>(
	validate: (parsed: unknown) => T | undefined,
): StorageCodec<T> => ({
	decode: (raw) => {
		try {
			return validate(JSON.parse(raw));
		} catch {
			return undefined;
		}
	},
	encode: (value) => JSON.stringify(value),
});

/**
 * JSON codec whose shape is declared as a Yup schema. Validation is
 * strict (no type coercion); the decoded value is rebuilt via cast so
 * unknown properties from older builds never leak through.
 */
export const yupCodec = <T>(schema: Schema<T>): StorageCodec<T> => ({
	decode: (raw) => {
		try {
			const parsed: unknown = JSON.parse(raw);
			return schema.isValidSync(parsed, { strict: true })
				? schema.cast(parsed, { stripUnknown: true })
				: undefined;
		} catch {
			return undefined;
		}
	},
	encode: (value) => JSON.stringify(value),
});

// -- Safe primitives --------------------------------------------------------

const getAreaStorage = (area: StorageArea): Storage | null => {
	// Accessing localStorage itself can throw (sandboxed iframes,
	// disabled cookies), so even the lookup is guarded.
	try {
		return area === "local" ? localStorage : sessionStorage;
	} catch {
		return null;
	}
};

const isQuotaError = (error: unknown): boolean => {
	if (!(error instanceof DOMException)) {
		return false;
	}
	return (
		error.name === "QuotaExceededError" ||
		error.name === "NS_ERROR_DOM_QUOTA_REACHED"
	);
};

const readRaw = (area: StorageArea, key: string): string | null => {
	try {
		return getAreaStorage(area)?.getItem(key) ?? null;
	} catch {
		return null;
	}
};

const writeRaw = (
	area: StorageArea,
	key: string,
	value: string,
): PersistResult => {
	try {
		const storage = getAreaStorage(area);
		if (!storage) {
			return { ok: false, reason: "unavailable" };
		}
		storage.setItem(key, value);
		return { ok: true };
	} catch (error) {
		return { ok: false, reason: isQuotaError(error) ? "quota" : "unavailable" };
	}
};

const removeRaw = (area: StorageArea, key: string): void => {
	try {
		getAreaStorage(area)?.removeItem(key);
	} catch {
		// The value simply stays; readers still work from their defaults.
	}
};

// -- Change notification and snapshot cache ---------------------------------

const cacheKeyFor = (area: StorageArea, key: string): string =>
	`${area}:${key}`;

const keyListeners = new Map<string, Set<() => void>>();

type SnapshotCacheEntry = { raw: string | null; value: unknown };
const snapshotCache = new Map<string, SnapshotCacheEntry>();

const notifyKey = (cacheKey: string): void => {
	const listeners = keyListeners.get(cacheKey);
	if (!listeners) {
		return;
	}
	for (const listener of listeners) {
		listener();
	}
};

const invalidateAndNotify = (area: StorageArea, key: string): void => {
	const cacheKey = cacheKeyFor(area, key);
	snapshotCache.delete(cacheKey);
	notifyKey(cacheKey);
};

addEventListener("storage", (event: StorageEvent) => {
	// sessionStorage is per-tab, so cross-tab events only matter for
	// localStorage.
	let isLocalArea = false;
	try {
		isLocalArea = event.storageArea === localStorage;
	} catch {
		return;
	}
	if (!isLocalArea) {
		return;
	}
	if (event.key === null) {
		// localStorage.clear() in another tab.
		for (const cacheKey of snapshotCache.keys()) {
			if (cacheKey.startsWith("local:")) {
				snapshotCache.delete(cacheKey);
			}
		}
		for (const [cacheKey, listeners] of keyListeners) {
			if (cacheKey.startsWith("local:")) {
				for (const listener of listeners) {
					listener();
				}
			}
		}
		return;
	}
	invalidateAndNotify("local", event.key);
});

// -- Key handles ------------------------------------------------------------

const createHandle = <T>(
	area: StorageArea,
	key: string,
	codec: StorageCodec<NonNullable<T>>,
	defaultValue: T,
): StorageKeyHandle<T> => {
	const cacheKey = cacheKeyFor(area, key);

	const decodeRaw = (raw: string | null): T => {
		if (raw === null) {
			return defaultValue;
		}
		const decoded = codec.decode(raw);
		return decoded === undefined ? defaultValue : decoded;
	};

	const getSnapshot = (): T => {
		const raw = readRaw(area, key);
		const cached = snapshotCache.get(cacheKey);
		if (cached && cached.raw === raw) {
			// SAFETY: each key has a single handle definition, so entries
			// under cacheKey were produced by this handle's codec as a T.
			return cached.value as T;
		}
		const value = decodeRaw(raw);
		snapshotCache.set(cacheKey, { raw, value });
		return value;
	};

	const remove = (): void => {
		// Skip the write and listener notification when there is
		// nothing to remove.
		if (readRaw(area, key) === null) {
			return;
		}
		removeRaw(area, key);
		invalidateAndNotify(area, key);
	};

	const set = (value: T): PersistResult => {
		if (value === null || value === undefined) {
			remove();
			return { ok: true };
		}
		const raw = codec.encode(value);
		const result = writeRaw(area, key, raw);
		if (!result.ok) {
			// Reads keep reflecting what actually persisted; callers can
			// inspect the result for their own failure handling.
			return result;
		}
		// Cache the caller's value directly so getSnapshot hands the
		// exact same reference back.
		snapshotCache.set(cacheKey, { raw, value });
		notifyKey(cacheKey);
		return result;
	};

	const subscribe = (listener: () => void): (() => void) => {
		let listeners = keyListeners.get(cacheKey);
		if (!listeners) {
			listeners = new Set();
			keyListeners.set(cacheKey, listeners);
		}
		listeners.add(listener);
		return () => {
			listeners.delete(listener);
			if (listeners.size === 0) {
				keyListeners.delete(cacheKey);
			}
		};
	};

	return {
		key,
		get: getSnapshot,
		set,
		remove,
		subscribe,
		getSnapshot,
	};
};

export function defineStorageKey<T>(options: {
	key: string;
	codec: StorageCodec<NonNullable<T>>;
	defaultValue: T;
	area?: StorageArea;
}): StorageKeyHandle<T> {
	return createHandle(
		options.area ?? "local",
		options.key,
		options.codec,
		options.defaultValue,
	);
}

// -- Entity-scoped key families ---------------------------------------------

export function defineEntityStorageKey<T>(options: {
	prefix: string;
	codec: StorageCodec<NonNullable<T>>;
	defaultValue: T;
	/**
	 * Extracts the owning entity ID from the key part after `prefix`.
	 * Defaults to the whole suffix; composite-ID families override it.
	 */
	entityIdFromSuffix?: (suffix: string) => string;
}): EntityStorageKey<T> {
	const {
		prefix,
		codec,
		defaultValue,
		entityIdFromSuffix = (suffix) => suffix,
	} = options;

	// Memoized so hooks receive stable subscribe/getSnapshot identities
	// for the same entity ID across renders.
	const handleCache = new Map<string, StorageKeyHandle<T>>();

	const listStoredSuffixes = (): string[] =>
		listLocalKeys()
			.filter((key) => key.startsWith(prefix))
			.map((key) => key.slice(prefix.length));

	return {
		prefix,
		forId: (...idParts) => {
			const key = prefix + idParts.join(".");
			let handle = handleCache.get(key);
			if (!handle) {
				handle = createHandle("local", key, codec, defaultValue);
				handleCache.set(key, handle);
			}
			return handle;
		},
		listStoredSuffixes,
		clear: (id) => {
			if (!id) {
				return;
			}
			// Collect first: removing keys while enumerating by index
			// skips entries.
			const ownedKeys = listLocalKeys().filter(
				(key) =>
					key.startsWith(prefix) &&
					entityIdFromSuffix(key.slice(prefix.length)) === id,
			);
			for (const key of ownedKeys) {
				removeRaw("local", key);
				invalidateAndNotify("local", key);
			}
		},
	};
}

/** Snapshot of localStorage key names; safe to mutate storage afterwards. */
const listLocalKeys = (): string[] => {
	const keys: string[] = [];
	// Enumeration itself can throw under restricted storage access, so
	// this never throws and returns what was collected.
	try {
		const storage = getAreaStorage("local");
		if (!storage) {
			return keys;
		}
		for (let index = 0; index < storage.length; index++) {
			const key = storage.key(index);
			if (key !== null) {
				keys.push(key);
			}
		}
	} catch {
		// Partial or empty list; callers treat it as best-effort.
	}
	return keys;
};

/** @internal Reset cache and listener state between tests. */
export function _resetStorageForTesting(): void {
	snapshotCache.clear();
	keyListeners.clear();
}

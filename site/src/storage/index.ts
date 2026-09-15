/**
 * Browser storage is for device-local state; cross-device preferences belong
 * in server settings. Define key handles next to their consumers.
 */

import type { Schema } from "yup";

export type PersistResult =
	| { ok: true }
	| { ok: false; reason: "quota" | "unavailable" | "invalid" };

type StorageArea = "local" | "session";

type StorageCodec<T> = {
	decode: (raw: string) => T | undefined;
	encode: (value: T) => string;
};

export type StorageKeyHandle<T> = {
	readonly key: string;
	get: () => T;
	set: (value: NonNullable<T>) => PersistResult;
	remove: () => PersistResult;
	subscribe: (listener: () => void) => () => void;
	getSnapshot: () => T;
};

type EntityStorageKey<T> = {
	readonly prefix: string;
	forId: (...idParts: string[]) => StorageKeyHandle<T>;
	listStoredSuffixes: () => string[];
	clear: (id: string) => PersistResult;
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
		// parseInt would accept partial matches like "12px" and huge
		// digit strings overflow to Infinity; require a plain safe
		// integer so corrupt input falls back to the default.
		if (!/^-?\d+$/.test(raw)) {
			return undefined;
		}
		const parsed = Number(raw);
		return Number.isSafeInteger(parsed) ? parsed : undefined;
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

/** JSON codec with strict Yup validation and unknown-field stripping. */
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
	// The storage getter can throw when the browser blocks site data.
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

const removeRaw = (area: StorageArea, key: string): PersistResult => {
	try {
		const storage = getAreaStorage(area);
		if (!storage) {
			return { ok: false, reason: "unavailable" };
		}
		storage.removeItem(key);
		return { ok: true };
	} catch (error) {
		return {
			ok: false,
			reason: isQuotaError(error) ? "quota" : "unavailable",
		};
	}
};

// -- Change notification and snapshot cache ---------------------------------

const listenerKeyFor = (area: StorageArea, key: string): string =>
	`${area}:${key}`;

const keyListeners = new Map<string, Set<() => void>>();

const notifyKey = (listenerKey: string): void => {
	const listeners = keyListeners.get(listenerKey);
	if (!listeners) {
		return;
	}
	for (const listener of listeners) {
		listener();
	}
};

const notifyKeyChanged = (area: StorageArea, key: string): void => {
	notifyKey(listenerKeyFor(area, key));
};

addEventListener("storage", (event: StorageEvent) => {
	let area: StorageArea;

	try {
		if (event.storageArea === localStorage) {
			area = "local";
		} else if (event.storageArea === sessionStorage) {
			area = "session";
		} else {
			return;
		}
	} catch {
		return;
	}

	if (event.key === null) {
		// clear() of the whole area elsewhere in this origin.
		const prefix = `${area}:`;

		for (const [listenerKey, listeners] of keyListeners) {
			if (listenerKey.startsWith(prefix)) {
				for (const listener of listeners) {
					listener();
				}
			}
		}
		return;
	}

	notifyKeyChanged(area, event.key);
});

// -- Key handles ------------------------------------------------------------

const createHandle = <T>(
	area: StorageArea,
	key: string,
	codec: StorageCodec<NonNullable<T>>,
	defaultValue: T,
): StorageKeyHandle<T> => {
	const listenerKey = listenerKeyFor(area, key);

	// Last decode, scoped to this handle so another handle on the same
	// key never sees values produced by a foreign codec or default.
	let cached: { raw: string | null; value: T } | undefined;

	const decodeRaw = (raw: string | null): T => {
		if (raw === null) {
			return defaultValue;
		}
		const decoded = codec.decode(raw);
		return decoded === undefined ? defaultValue : decoded;
	};

	/** Referentially stable across repeated reads. */
	const getSnapshot = (): T => {
		const raw = readRaw(area, key);
		if (cached && cached.raw === raw) {
			return cached.value;
		}
		const value = decodeRaw(raw);
		cached = { raw, value };
		return value;
	};

	const remove = (): PersistResult => {
		try {
			const storage = getAreaStorage(area);
			if (!storage) {
				return { ok: false, reason: "unavailable" };
			}
			// Skip the write and listener notification when there is
			// nothing to remove.
			if (storage.getItem(key) === null) {
				return { ok: true };
			}
			storage.removeItem(key);
		} catch (error) {
			return {
				ok: false,
				reason: isQuotaError(error) ? "quota" : "unavailable",
			};
		}
		notifyKey(listenerKey);
		return { ok: true };
	};

	const set = (value: NonNullable<T>): PersistResult => {
		let raw: string;
		try {
			raw = codec.encode(value);
		} catch {
			// JSON.stringify throws on cyclic values; surface it as a
			// result instead of aborting the caller's event handler.
			return { ok: false, reason: "invalid" };
		}
		// A value whose encoding does not decode back (NaN, unsafe
		// integers) would read as the default after reload; reject it
		// so live and reloaded reads stay consistent.
		const decoded = codec.decode(raw);
		if (decoded === undefined) {
			return { ok: false, reason: "invalid" };
		}
		const result = writeRaw(area, key, raw);
		if (!result.ok) {
			// Reads keep reflecting what actually persisted; callers can
			// inspect the result for their own failure handling.
			return result;
		}
		// Cache the decoded form so the live snapshot always matches a
		// reload (JSON normalizes values like [-0] and drops undefined
		// properties) and so mutating a retained caller reference cannot
		// desync snapshots from the stored bytes.
		cached = { raw, value: decoded };
		notifyKey(listenerKey);
		return result;
	};

	const subscribe = (listener: () => void): (() => void) => {
		let listeners = keyListeners.get(listenerKey);
		if (!listeners) {
			listeners = new Set();
			keyListeners.set(listenerKey, listeners);
		}
		listeners.add(listener);
		return () => {
			listeners.delete(listener);
			if (listeners.size === 0) {
				keyListeners.delete(listenerKey);
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

	// An empty or missing ID part would alias the bare family prefix
	// (which clear() cannot remove), and a part containing the "."
	// delimiter would collide distinct composite identities onto one
	// key. The inert handle keeps such states readable without
	// persisting anything.
	let inertHandle: StorageKeyHandle<T> | undefined;

	const listStoredSuffixes = (): string[] =>
		listLocalKeys()
			.filter((key) => key.startsWith(prefix))
			.map((key) => key.slice(prefix.length));

	return {
		prefix,
		forId: (...idParts) => {
			if (
				idParts.length === 0 ||
				idParts.some((part) => part === "" || part.includes("."))
			) {
				inertHandle ??= {
					key: prefix,
					get: () => defaultValue,
					getSnapshot: () => defaultValue,
					set: () => ({ ok: false, reason: "invalid" }),
					remove: () => ({ ok: true }),
					subscribe: () => () => {},
				};
				return inertHandle;
			}
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
				return { ok: true };
			}
			// Collect first: removing keys while enumerating by index
			// skips entries.
			const localKeys = listLocalKeysOrNull();
			if (localKeys === null) {
				// Matching values may remain; do not report success.
				return { ok: false, reason: "unavailable" };
			}
			const ownedKeys = localKeys.filter(
				(key) =>
					key.startsWith(prefix) &&
					entityIdFromSuffix(key.slice(prefix.length)) === id,
			);
			let failure: PersistResult | undefined;
			for (const key of ownedKeys) {
				const result = removeRaw("local", key);
				if (result.ok) {
					notifyKeyChanged("local", key);
				} else {
					failure ??= result;
				}
			}
			return failure ?? { ok: true };
		},
	};
}

/**
 * Snapshot of localStorage key names, or null when enumeration itself
 * fails under restricted storage access; safe to mutate storage
 * afterwards.
 */
const listLocalKeysOrNull = (): string[] | null => {
	const keys: string[] = [];
	try {
		const storage = getAreaStorage("local");
		if (!storage) {
			return null;
		}
		for (let index = 0; index < storage.length; index++) {
			const key = storage.key(index);
			if (key !== null) {
				keys.push(key);
			}
		}
	} catch {
		return null;
	}
	return keys;
};

const listLocalKeys = (): string[] => listLocalKeysOrNull() ?? [];

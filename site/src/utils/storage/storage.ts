/**
 * Centralized browser storage core.
 *
 * Every persisted UI preference goes through this module. It provides
 * never-throwing primitives around localStorage/sessionStorage, typed
 * key handles with parse-validation, change notification for same-tab
 * and cross-tab reactivity (the native "storage" event only fires in
 * other tabs), and a registry of entity-scoped key families that
 * drives lifecycle cleanup and the expired-key sweep.
 *
 * Key definitions live in `keys.ts`; import handles from there rather
 * than defining keys at call sites.
 */

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

type StorageEntityType = "chat" | "workspace" | "modelConfig";

export type SweepAction = "keep" | "remove" | { rewrite: string };

type EntityStorageKey<T> = {
	readonly prefix: string;
	readonly entity: StorageEntityType;
	forId: (...idParts: string[]) => StorageKeyHandle<T>;
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

export const stringLiteralCodec = <T extends string>(
	values: readonly T[],
): StorageCodec<T> => ({
	decode: (raw) => (values.includes(raw as T) ? (raw as T) : undefined),
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

// -- Safe primitives --------------------------------------------------------

const getAreaStorage = (area: StorageArea): Storage | null => {
	// Accessing window.localStorage itself can throw (sandboxed iframes,
	// disabled cookies), so even the lookup is guarded.
	try {
		if (typeof window === "undefined") {
			return null;
		}
		return area === "local" ? window.localStorage : window.sessionStorage;
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

if (typeof window !== "undefined") {
	window.addEventListener("storage", (event) => {
		// sessionStorage is per-tab, so cross-tab events only matter for
		// localStorage.
		let isLocalArea = false;
		try {
			isLocalArea = event.storageArea === window.localStorage;
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
}

// -- Value envelope for entity-scoped keys ----------------------------------

/**
 * Entity-scoped values are wrapped in a small envelope carrying the
 * last-write time so the sweep can expire orphans whose owning entity
 * was deleted elsewhere. `d` holds the codec-encoded value. Legacy
 * bare values (written before this layer existed) stay readable and
 * get stamped with an envelope by the sweep.
 */
type StoredEnvelope = { v: 1; t: number; d: string };

const decodeEnvelope = (raw: string): StoredEnvelope | null => {
	try {
		const parsed: unknown = JSON.parse(raw);
		if (
			typeof parsed === "object" &&
			parsed !== null &&
			!Array.isArray(parsed)
		) {
			const env = parsed as Record<string, unknown>;
			if (
				env.v === 1 &&
				typeof env.t === "number" &&
				typeof env.d === "string"
			) {
				return { v: 1, t: env.t, d: env.d };
			}
		}
	} catch {
		// Not JSON, so not an envelope.
	}
	return null;
};

const encodeEnvelope = (data: string, nowMs: number): string =>
	JSON.stringify({ v: 1, t: nowMs, d: data } satisfies StoredEnvelope);

// -- Key handles ------------------------------------------------------------

const createHandle = <T>(
	area: StorageArea,
	key: string,
	codec: StorageCodec<NonNullable<T>>,
	defaultValue: T,
	envelope: boolean,
): StorageKeyHandle<T> => {
	const cacheKey = cacheKeyFor(area, key);

	const decodeRaw = (raw: string | null): T => {
		if (raw === null) {
			return defaultValue;
		}
		const data = envelope ? (decodeEnvelope(raw)?.d ?? raw) : raw;
		const decoded = codec.decode(data);
		return decoded === undefined ? defaultValue : decoded;
	};

	const getSnapshot = (): T => {
		const raw = readRaw(area, key);
		const cached = snapshotCache.get(cacheKey);
		if (cached && cached.raw === raw) {
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
		const data = codec.encode(value as NonNullable<T>);
		const raw = envelope ? encodeEnvelope(data, Date.now()) : data;
		const result = writeRaw(area, key, raw);
		if (result.ok) {
			// Cache the caller's value directly so getSnapshot hands the
			// exact same reference back.
			snapshotCache.set(cacheKey, { raw, value });
			notifyKey(cacheKey);
		}
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
		false,
	);
}

// -- Entity-scoped key families ---------------------------------------------

type RegisteredEntityFamily = {
	prefix: string;
	entity: StorageEntityType;
	entityIdFromSuffix: (suffix: string) => string;
	sweepValue: (raw: string, nowMs: number) => SweepAction;
};

const entityFamilies: RegisteredEntityFamily[] = [];

const defaultEnvelopeSweep =
	(ttlMs: number) =>
	(raw: string, nowMs: number): SweepAction => {
		const envelope = decodeEnvelope(raw);
		if (!envelope) {
			// Legacy bare value: stamp an envelope so its TTL clock starts
			// now instead of keeping it forever.
			return { rewrite: encodeEnvelope(raw, nowMs) };
		}
		return nowMs - envelope.t > ttlMs ? "remove" : "keep";
	};

export function defineEntityStorageKey<T>(options: {
	prefix: string;
	entity: StorageEntityType;
	codec: StorageCodec<NonNullable<T>>;
	defaultValue: T;
	ttlMs: number;
	/**
	 * Disable the write-time envelope for families that manage their
	 * own stored format and timestamps. Requires a custom sweepValue.
	 */
	envelope?: boolean;
	/**
	 * Extracts the owning entity ID from the key part after `prefix`.
	 * Defaults to the whole suffix; composite-ID families override it.
	 */
	entityIdFromSuffix?: (suffix: string) => string;
	sweepValue?: (raw: string, nowMs: number) => SweepAction;
}): EntityStorageKey<T> {
	const {
		prefix,
		entity,
		codec,
		defaultValue,
		ttlMs,
		envelope = true,
		entityIdFromSuffix = (suffix) => suffix,
		sweepValue,
	} = options;

	if (entityFamilies.some((family) => family.prefix === prefix)) {
		throw new Error(`Duplicate storage key family prefix: ${prefix}`);
	}
	entityFamilies.push({
		prefix,
		entity,
		entityIdFromSuffix,
		sweepValue: sweepValue ?? defaultEnvelopeSweep(ttlMs),
	});

	// Memoized so hooks receive stable subscribe/getSnapshot identities
	// for the same entity ID across renders.
	const handleCache = new Map<string, StorageKeyHandle<T>>();

	return {
		prefix,
		entity,
		forId: (...idParts) => {
			const key = prefix + idParts.join(".");
			let handle = handleCache.get(key);
			if (!handle) {
				handle = createHandle("local", key, codec, defaultValue, envelope);
				handleCache.set(key, handle);
			}
			return handle;
		},
	};
}

// -- Lifecycle cleanup and expiry sweep -------------------------------------

/**
 * Remove every registered key owned by the given entity. Wire this
 * into the mutation that deletes or archives the entity so per-entity
 * keys cannot leak.
 */
export function clearEntityStorage(
	entity: StorageEntityType,
	id: string,
): void {
	if (!id) {
		return;
	}
	const storage = getAreaStorage("local");
	if (!storage) {
		return;
	}
	try {
		for (let index = storage.length - 1; index >= 0; index--) {
			const key = storage.key(index);
			if (!key) {
				continue;
			}
			const owned = entityFamilies.some(
				(family) =>
					family.entity === entity &&
					key.startsWith(family.prefix) &&
					family.entityIdFromSuffix(key.slice(family.prefix.length)) === id,
			);
			if (owned) {
				removeRaw("local", key);
				invalidateAndNotify("local", key);
			}
		}
	} catch {
		// Enumeration failed; stale keys will be caught by the sweep.
	}
}

const legacyStorageKeys: string[] = [];

/** Keys that are no longer written anywhere and are removed unconditionally by the sweep. */
export function registerLegacyStorageKeys(keys: readonly string[]): void {
	legacyStorageKeys.push(...keys);
}

let sweepHasRun = false;

/**
 * Remove expired entity-scoped values and legacy keys. Explicit
 * cleanup cannot catch entities archived or deleted from another
 * client, so this time-based sweep collects those orphans. Runs once
 * per session; call it from App mount.
 */
export function sweepExpiredStorage(nowMs = Date.now()): void {
	if (sweepHasRun) {
		return;
	}
	sweepHasRun = true;
	const storage = getAreaStorage("local");
	if (!storage) {
		return;
	}
	for (const key of legacyStorageKeys) {
		removeRaw("local", key);
	}
	try {
		for (let index = storage.length - 1; index >= 0; index--) {
			const key = storage.key(index);
			if (!key) {
				continue;
			}
			const family = entityFamilies.find((candidate) =>
				key.startsWith(candidate.prefix),
			);
			if (!family) {
				continue;
			}
			const raw = readRaw("local", key);
			if (raw === null) {
				continue;
			}
			const action = family.sweepValue(raw, nowMs);
			if (action === "keep") {
				continue;
			}
			if (action === "remove") {
				removeRaw("local", key);
			} else {
				writeRaw("local", key, action.rewrite);
			}
			invalidateAndNotify("local", key);
		}
	} catch {
		// Enumeration failed; the sweep retries next session.
	}
}

/** @internal Reset per-session sweep and cache state between tests. */
export function _resetStorageForTesting(): void {
	sweepHasRun = false;
	snapshotCache.clear();
	keyListeners.clear();
}

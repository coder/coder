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

/**
 * Keys whose snapshot holds a value newer than what is persisted
 * because the write failed (quota, unavailable storage). The value
 * stays visible in this tab for the session; any real change to the
 * underlying bytes discards the overlay.
 */
const overlayKeys = new Set<string>();

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
	overlayKeys.delete(cacheKey);
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

// -- Companion timestamps for entity-scoped keys -----------------------------

/**
 * Entity-scoped values keep their legacy raw format so clients from
 * before and after a deploy can read each other's values. The
 * last-write time the sweep needs to expire orphans lives in a
 * companion key under this reserved prefix instead. The companion
 * also stores a fingerprint of the value bytes, so a value updated by
 * an old client (which does not touch companions) is detected and
 * re-stamped rather than expired on a stale timestamp.
 */
const TIMESTAMP_KEY_PREFIX = "coder.storage.timestamps.";

const timestampKeyFor = (key: string): string => TIMESTAMP_KEY_PREFIX + key;

/** FNV-1a 32-bit hash, hex-encoded. Cheap change detection, not crypto. */
const fingerprint = (raw: string): string => {
	let hash = 0x811c9dc5;
	for (let index = 0; index < raw.length; index++) {
		hash ^= raw.charCodeAt(index);
		hash = Math.imul(hash, 0x01000193);
	}
	return (hash >>> 0).toString(16);
};

type StoredTimestamp = { t: number; h: string };

const readTimestamp = (key: string): StoredTimestamp | null => {
	const raw = readRaw("local", timestampKeyFor(key));
	if (raw === null) {
		return null;
	}
	try {
		const parsed: unknown = JSON.parse(raw);
		if (typeof parsed === "object" && parsed !== null) {
			const record = parsed as Record<string, unknown>;
			if (typeof record.t === "number" && typeof record.h === "string") {
				return { t: record.t, h: record.h };
			}
		}
	} catch {
		// Corrupt companion; caller re-stamps.
	}
	return null;
};

const writeTimestamp = (key: string, valueRaw: string, nowMs: number): void => {
	writeRaw(
		"local",
		timestampKeyFor(key),
		JSON.stringify({ t: nowMs, h: fingerprint(valueRaw) }),
	);
};

const removeTimestamp = (key: string): void => {
	removeRaw("local", timestampKeyFor(key));
};

/**
 * Envelope format written by pre-release builds of this module. The
 * sweep migrates any remaining envelopes back to raw values with
 * companion timestamps; regular reads no longer decode it.
 */
const decodeLegacyEnvelope = (raw: string): { t: number; d: string } | null => {
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
				return { t: env.t, d: env.d };
			}
		}
	} catch {
		// Not JSON, so not an envelope.
	}
	return null;
};

// -- Key handles ------------------------------------------------------------

const createHandle = <T>(
	area: StorageArea,
	key: string,
	codec: StorageCodec<NonNullable<T>>,
	defaultValue: T,
	options: { timestamped: boolean; overlay: boolean },
): StorageKeyHandle<T> => {
	const { timestamped, overlay } = options;
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
			return cached.value as T;
		}
		const value = decodeRaw(raw);
		snapshotCache.set(cacheKey, { raw, value });
		return value;
	};

	const remove = (): void => {
		// Skip the write and listener notification when there is
		// nothing to remove, persisted or overlaid.
		if (readRaw(area, key) === null && !overlayKeys.has(cacheKey)) {
			return;
		}
		removeRaw(area, key);
		if (timestamped) {
			removeTimestamp(key);
		}
		invalidateAndNotify(area, key);
	};

	const set = (value: T): PersistResult => {
		if (value === null || value === undefined) {
			remove();
			return { ok: true };
		}
		const raw = codec.encode(value as NonNullable<T>);
		const result = writeRaw(area, key, raw);
		if (result.ok) {
			if (timestamped) {
				// Value first, timestamp second: a failed companion write
				// is stamped by the next sweep.
				writeTimestamp(key, raw, Date.now());
			}
			overlayKeys.delete(cacheKey);
			// Cache the caller's value directly so getSnapshot hands the
			// exact same reference back.
			snapshotCache.set(cacheKey, { raw, value });
		} else if (overlay) {
			// Persistence failed; keep the value visible in this tab by
			// overlaying it on the bytes currently persisted.
			overlayKeys.add(cacheKey);
			snapshotCache.set(cacheKey, { raw: readRaw(area, key), value });
		} else {
			// Callers with their own failure handling need reads to keep
			// reflecting what actually persisted.
			return result;
		}
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
		{ timestamped: false, overlay: true },
	);
}

// -- Entity-scoped key families ---------------------------------------------

type RegisteredEntityFamily = {
	prefix: string;
	entity: StorageEntityType;
	entityIdFromSuffix: (suffix: string) => string;
	ttlMs: number;
	/** Custom expiry for families whose values embed their own timestamps. */
	sweepValue?: (raw: string, nowMs: number) => SweepAction;
};

const entityFamilies: RegisteredEntityFamily[] = [];

export function defineEntityStorageKey<T>(options: {
	prefix: string;
	entity: StorageEntityType;
	codec: StorageCodec<NonNullable<T>>;
	defaultValue: T;
	ttlMs: number;
	/**
	 * Disable the companion timestamp for families whose values embed
	 * their own timestamps. Requires a custom sweepValue.
	 */
	timestamped?: boolean;
	/**
	 * Disable the in-memory fallback for failed writes. Callers that
	 * check PersistResult and run their own quota handling need reads
	 * to reflect what actually persisted.
	 */
	overlay?: boolean;
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
		timestamped = true,
		overlay = true,
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
		ttlMs,
		sweepValue,
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
				handle = createHandle("local", key, codec, defaultValue, {
					timestamped,
					overlay,
				});
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
	// Collect first: removing keys while enumerating by index skips
	// entries. Keys known only in memory (failed-write overlays, and
	// snapshots read before enumeration became restricted) must be
	// cleaned too, so include every locally cached key alongside the
	// localStorage listing. Overlay keys always have a snapshot entry.
	const candidateKeys = new Set<string>(listLocalKeys());
	for (const cacheKey of snapshotCache.keys()) {
		if (cacheKey.startsWith("local:")) {
			candidateKeys.add(cacheKey.slice("local:".length));
		}
	}
	const ownedKeys: string[] = [];
	for (const key of candidateKeys) {
		// Companion timestamps are owned by their value key.
		const valueKey = key.startsWith(TIMESTAMP_KEY_PREFIX)
			? key.slice(TIMESTAMP_KEY_PREFIX.length)
			: key;
		const owned = entityFamilies.some(
			(family) =>
				family.entity === entity &&
				valueKey.startsWith(family.prefix) &&
				family.entityIdFromSuffix(valueKey.slice(family.prefix.length)) === id,
		);
		if (owned) {
			ownedKeys.push(key);
		}
	}
	for (const key of ownedKeys) {
		removeRaw("local", key);
		if (!key.startsWith(TIMESTAMP_KEY_PREFIX)) {
			removeTimestamp(key);
			invalidateAndNotify("local", key);
		}
	}
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
		// Partial or empty list; the next sweep retries.
	}
	return keys;
};

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
 * per page load; call it before rendering the app so readers never
 * hydrate from values the sweep is about to expire.
 */
export function sweepExpiredStorage(nowMs = Date.now()): void {
	if (sweepHasRun) {
		return;
	}
	sweepHasRun = true;
	for (const key of legacyStorageKeys) {
		removeRaw("local", key);
	}
	for (const key of listLocalKeys()) {
		if (key.startsWith(TIMESTAMP_KEY_PREFIX)) {
			// Companion whose value key is gone (removed by a client
			// that does not know about companions).
			if (readRaw("local", key.slice(TIMESTAMP_KEY_PREFIX.length)) === null) {
				removeRaw("local", key);
			}
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
		if (family.sweepValue) {
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
			continue;
		}
		sweepTimestampedKey(family, key, raw, nowMs);
	}
}

const sweepTimestampedKey = (
	family: RegisteredEntityFamily,
	key: string,
	raw: string,
	nowMs: number,
): void => {
	const envelope = decodeLegacyEnvelope(raw);
	if (envelope) {
		if (nowMs - envelope.t > family.ttlMs) {
			removeRaw("local", key);
			removeTimestamp(key);
		} else {
			// Unwrap back to the raw value, keeping the original
			// write time in the companion.
			writeRaw("local", key, envelope.d);
			writeTimestamp(key, envelope.d, envelope.t);
		}
		invalidateAndNotify("local", key);
		return;
	}
	const stamp = readTimestamp(key);
	if (!stamp || stamp.h !== fingerprint(raw)) {
		// No companion (legacy or old-client write) or the value
		// changed since it was stamped: start the TTL clock now.
		writeTimestamp(key, raw, nowMs);
		return;
	}
	if (nowMs - stamp.t > family.ttlMs) {
		removeRaw("local", key);
		removeTimestamp(key);
		invalidateAndNotify("local", key);
	}
};

/** @internal Reset per-session sweep and cache state between tests. */
export function _resetStorageForTesting(): void {
	sweepHasRun = false;
	snapshotCache.clear();
	overlayKeys.clear();
	keyListeners.clear();
}

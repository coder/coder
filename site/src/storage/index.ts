/** Device-local persistence; cross-device preferences belong in server settings. */
import { atom, type WritableAtom } from "jotai";
import { atomWithLazy, RESET } from "jotai/utils";
import type { Schema } from "yup";

export type PersistResult =
	| { ok: true }
	| { ok: false; reason: "quota" | "unavailable" | "invalid" };
type StorageArea = "local" | "session";
type StorageCodec<T> = {
	decode: (raw: string) => T | undefined;
	encode: (value: T) => string;
};
type StorageUpdate<T> =
	| NonNullable<T>
	| typeof RESET
	| ((previous: T) => NonNullable<T>);
const isUpdater = <T>(
	update: StorageUpdate<T>,
): update is (previous: T) => NonNullable<T> => typeof update === "function";
type StorageKeyHandle<T> = WritableAtom<
	T,
	[StorageUpdate<T>],
	PersistResult
> & {
	readonly key: string;
	get: () => T;
	set: (value: NonNullable<T>) => PersistResult;
	remove: () => PersistResult;
};

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

// Imperative writes must also refresh atoms mounted in another Jotai store.
const changes = new EventTarget();

export function defineStorageKey<T>(options: {
	key: string;
	codec: StorageCodec<NonNullable<T>>;
	defaultValue: T;
	area?: StorageArea;
}): StorageKeyHandle<T> {
	const { key, codec, defaultValue, area = "local" } = options;
	const eventName = `${area}:${key}`;
	const decode = (raw: string | null): T => {
		if (raw === null) return defaultValue;
		try {
			return codec.decode(raw) ?? defaultValue;
		} catch {
			return defaultValue;
		}
	};
	const read = () => decode(readRaw(area, key));
	const persist = (value: NonNullable<T> | typeof RESET): PersistResult => {
		let result: PersistResult;
		if (value === RESET) {
			result = removeRaw(area, key);
		} else {
			let raw: string;
			try {
				raw = codec.encode(value);
				// Reject encodings that would fall back to the default after reload.
				if (typeof raw !== "string" || codec.decode(raw) === undefined) {
					return { ok: false, reason: "invalid" };
				}
			} catch {
				return { ok: false, reason: "invalid" };
			}
			result = writeRaw(area, key, raw);
		}
		if (result.ok) changes.dispatchEvent(new Event(eventName));
		return result;
	};
	const valueAtom = atomWithLazy(() => readRaw(area, key));
	valueAtom.onMount = (setValue) => {
		const refresh = () => setValue(readRaw(area, key));
		const onStorage = (event: StorageEvent) => {
			if (
				event.storageArea === getAreaStorage(area) &&
				(event.key === key || event.key === null)
			)
				refresh();
		};
		changes.addEventListener(eventName, refresh);
		addEventListener("storage", onStorage);
		refresh();
		return () => {
			changes.removeEventListener(eventName, refresh);
			removeEventListener("storage", onStorage);
		};
	};
	const storedAtom = atom(
		(get) => decode(get(valueAtom)),
		(_get, set, update: StorageUpdate<T>) => {
			const value = isUpdater(update) ? update(read()) : update;
			// atomWithStorage updates memory before writing; failed writes here are no-ops.
			const result = persist(value);
			if (result.ok) set(valueAtom, readRaw(area, key));
			return result;
		},
	);
	return Object.assign(storedAtom, {
		key,
		get: read,
		set: (value: NonNullable<T>) => persist(value),
		remove: () => persist(RESET),
	});
}

export function defineEntityStorageKey<T>(options: {
	prefix: string;
	codec: StorageCodec<NonNullable<T>>;
	defaultValue: T;
	entityIdFromSuffix?: (suffix: string) => string;
}) {
	const {
		prefix,
		codec,
		defaultValue,
		entityIdFromSuffix = (suffix: string) => suffix,
	} = options;
	const atoms = new Map<string, StorageKeyHandle<T>>();
	const inert = Object.assign(
		atom(defaultValue, () => ({ ok: false, reason: "invalid" }) as const),
		{
			key: prefix,
			get: () => defaultValue,
			set: (): PersistResult => ({ ok: false, reason: "invalid" }),
			remove: (): PersistResult => ({ ok: true }),
		},
	);
	return {
		prefix,
		forId: (...parts: string[]): StorageKeyHandle<T> => {
			if (!parts.length || parts.some((part) => !part || part.includes(".")))
				return inert;
			const key = prefix + parts.join(".");
			let storedAtom = atoms.get(key);
			if (!storedAtom) {
				storedAtom = defineStorageKey({ key, codec, defaultValue });
				atoms.set(key, storedAtom);
			}
			return storedAtom;
		},
		listStoredSuffixes: () =>
			listLocalKeys()
				.filter((key) => key.startsWith(prefix))
				.map((key) => key.slice(prefix.length)),
		clear: (id: string): PersistResult => {
			if (!id) return { ok: true };
			const keys = listLocalKeysOrNull();
			if (!keys) return { ok: false, reason: "unavailable" };
			let failure: PersistResult | undefined;
			for (const key of keys) {
				if (
					!key.startsWith(prefix) ||
					entityIdFromSuffix(key.slice(prefix.length)) !== id
				)
					continue;
				const result = removeRaw("local", key);
				if (result.ok) changes.dispatchEvent(new Event(`local:${key}`));
				else failure ??= result;
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

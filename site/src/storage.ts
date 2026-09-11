import { atom, getDefaultStore, type WritableAtom } from "jotai/vanilla";
import { atomWithStorage, RESET } from "jotai/vanilla/utils";
import type { Schema } from "yup";

type Store = ReturnType<typeof getDefaultStore>;

type SyncStorage<T> = {
	getItem: (key: string, initialValue: T) => T;
	setItem: (key: string, value: T) => void;
	removeItem: (key: string) => void;
	subscribe: (
		key: string,
		callback: (value: T) => void,
		initialValue: T,
	) => () => void;
};

type StorageCodec<T> = {
	decode: (raw: string) => T | undefined;
	encode: (value: T) => string;
};

type StorageUpdate<T> = T | typeof RESET | ((previous: T) => T | typeof RESET);
type StorageAtom<T> = WritableAtom<T, [StorageUpdate<T>], void>;

type StorageAtomFamily<T> = {
	readonly prefix: string;
	forId: (...idParts: string[]) => StorageAtom<T>;
	keyFor: (...idParts: string[]) => string;
	listStoredSuffixes: () => string[];
	clear: (id: string, store?: Store) => void;
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
		if (!/^-?\d+$/.test(raw)) {
			return undefined;
		}
		const parsed = Number(raw);
		return Number.isSafeInteger(parsed) ? parsed : undefined;
	},
	encode: (value) => String(value),
};

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

export const createCodecStorage = <T>(
	getStorage: () => Storage | undefined,
	codec: StorageCodec<NonNullable<T>>,
): SyncStorage<T> => {
	const storage = (): Storage | undefined => {
		try {
			return getStorage();
		} catch {
			return undefined;
		}
	};
	const read = (key: string, initialValue: T): T => {
		try {
			const raw = storage()?.getItem(key);
			if (raw === null || raw === undefined) {
				return initialValue;
			}
			return codec.decode(raw) ?? initialValue;
		} catch {
			return initialValue;
		}
	};

	return {
		getItem: read,
		setItem: (key, value) => {
			try {
				const target = storage();
				if (!target) {
					return;
				}
				if (value === null || value === undefined) {
					target.removeItem(key);
					return;
				}
				const raw = codec.encode(value);
				if (codec.decode(raw) !== undefined) {
					target.setItem(key, raw);
				}
			} catch {
				// Browser storage can be unavailable or full.
			}
		},
		removeItem: (key) => {
			try {
				storage()?.removeItem(key);
			} catch {
				// Browser storage can be unavailable.
			}
		},
		subscribe: (key, callback, initialValue) => {
			const listener = (event: StorageEvent): void => {
				if (
					event.storageArea === storage() &&
					(event.key === null || event.key === key)
				) {
					callback(read(key, initialValue));
				}
			};
			window.addEventListener("storage", listener);
			return () => window.removeEventListener("storage", listener);
		},
	};
};

export function atomFamilyWithStorage<T>(options: {
	prefix: string;
	codec: StorageCodec<NonNullable<T>>;
	initialValue: T;
	entityIdFromSuffix?: (suffix: string) => string;
}): StorageAtomFamily<T> {
	const {
		prefix,
		codec,
		initialValue,
		entityIdFromSuffix = (suffix) => suffix,
	} = options;
	const storage = createCodecStorage<T>(() => localStorage, codec);
	const atoms = new Map<string, StorageAtom<T>>();
	const inertAtom: StorageAtom<T> = atom(
		() => initialValue,
		() => {},
	);
	const keyFor = (...idParts: string[]): string => prefix + idParts.join(".");

	return {
		prefix,
		keyFor,
		forId: (...idParts) => {
			if (
				idParts.length === 0 ||
				idParts.some((part) => part === "" || part.includes("."))
			) {
				return inertAtom;
			}
			const key = keyFor(...idParts);
			let storageAtom = atoms.get(key);
			if (!storageAtom) {
				storageAtom = atomWithStorage(key, initialValue, storage, {
					getOnInit: true,
				});
				atoms.set(key, storageAtom);
			}
			return storageAtom;
		},
		listStoredSuffixes: () =>
			listLocalKeys()
				.filter((key) => key.startsWith(prefix))
				.map((key) => key.slice(prefix.length)),
		clear: (id, store = getDefaultStore()) => {
			if (!id) {
				return;
			}
			for (const key of listLocalKeys()) {
				if (
					!key.startsWith(prefix) ||
					entityIdFromSuffix(key.slice(prefix.length)) !== id
				) {
					continue;
				}
				const storageAtom = atoms.get(key);
				if (storageAtom) {
					store.set(storageAtom, RESET);
				} else {
					try {
						localStorage.removeItem(key);
					} catch {
						// Browser storage can be unavailable.
					}
				}
			}
		},
	};
}

const listLocalKeys = (): string[] => {
	const keys: string[] = [];
	try {
		for (let index = 0; index < localStorage.length; index++) {
			const key = localStorage.key(index);
			if (key !== null) {
				keys.push(key);
			}
		}
	} catch {
		return [];
	}
	return keys;
};

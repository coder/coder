import { createStore } from "jotai";
import { RESET } from "jotai/utils";
import {
	afterEach,
	beforeEach,
	describe,
	expect,
	expectTypeOf,
	it,
	vi,
} from "vitest";
import { array, string } from "yup";
import {
	booleanCodec,
	defineEntityStorageKey,
	defineStorageKey,
	integerCodec,
	jsonCodec,
	stringCodec,
	stringLiteralCodec,
	yupCodec,
} from "./index";

let store = createStore();
beforeEach(() => {
	store = createStore();
});

const stringArraySchema = array(string().defined()).defined();

const boolKey = defineStorageKey<boolean>({
	key: "test.bool",
	codec: booleanCodec,
	defaultValue: false,
});
const numberKey = defineStorageKey<number | null>({
	key: "test.number",
	codec: integerCodec,
	defaultValue: null,
});
const listKey = defineStorageKey<string[] | null>({
	key: "test.list",
	codec: yupCodec(stringArraySchema),
	defaultValue: null,
});
const literalKey = defineStorageKey<"a" | "b">({
	key: "test.literal",
	codec: stringLiteralCodec({ oneOf: ["a", "b"] }),
	defaultValue: "a",
});
const sessionKey = defineStorageKey<string | null>({
	key: "test.session",
	codec: stringCodec,
	defaultValue: null,
	area: "session",
});

const chatNote = defineEntityStorageKey<string | null>({
	prefix: "test.chat-note.",
	codec: stringCodec,
	defaultValue: null,
});
const chatTabs = defineEntityStorageKey<readonly string[]>({
	prefix: "test.chat-tabs.",
	codec: yupCodec<readonly string[]>(stringArraySchema),
	defaultValue: [],
});
const chatComposite = defineEntityStorageKey<string | null>({
	prefix: "test.chat-composite.",
	codec: stringCodec,
	defaultValue: null,
	entityIdFromSuffix: (suffix) => suffix.split(".").at(-1) ?? suffix,
});

afterEach(() => {
	vi.restoreAllMocks();
});

describe("storage core", () => {
	beforeEach(() => {
		localStorage.clear();
		sessionStorage.clear();
	});

	it("returns the default when nothing is stored and never writes it", () => {
		expect(boolKey.get()).toBe(false);
		expect(numberKey.get()).toBeNull();
		expect(localStorage.getItem("test.bool")).toBeNull();
		expect(localStorage.getItem("test.number")).toBeNull();
	});

	it("round-trips values through codecs", () => {
		expect(boolKey.set(true)).toEqual({ ok: true });
		expect(localStorage.getItem("test.bool")).toBe("true");
		expect(boolKey.get()).toBe(true);

		numberKey.set(42);
		expect(localStorage.getItem("test.number")).toBe("42");
		expect(numberKey.get()).toBe(42);

		listKey.set(["x", "y"]);
		expect(listKey.get()).toEqual(["x", "y"]);

		literalKey.set("b");
		expect(literalKey.get()).toBe("b");
	});

	it("falls back to the default for corrupted or invalid values", () => {
		localStorage.setItem("test.list", "{not json");
		expect(listKey.get()).toBeNull();

		localStorage.setItem("test.list", '"a string, not an array"');
		expect(listKey.get()).toBeNull();

		localStorage.setItem("test.number", "not-a-number");
		expect(numberKey.get()).toBeNull();

		localStorage.setItem("test.literal", "c");
		expect(literalKey.get()).toBe("a");
	});

	it("rejects malformed or unsafe integers instead of truncating them", () => {
		localStorage.setItem("test.number", "12px");
		expect(numberKey.get()).toBeNull();

		localStorage.setItem("test.number", "1e3");
		expect(numberKey.get()).toBeNull();

		localStorage.setItem("test.number", "9".repeat(400));
		expect(numberKey.get()).toBeNull();

		localStorage.setItem("test.number", "-7");
		expect(numberKey.get()).toBe(-7);
	});

	it("normalizes signed zero to its persisted form", () => {
		numberKey.set(-0);
		expect(localStorage.getItem("test.number")).toBe("0");
		expect(Object.is(numberKey.get(), 0)).toBe(true);
	});

	it("removes the key explicitly", () => {
		expectTypeOf(numberKey.set).parameter(0).toEqualTypeOf<number>();
		numberKey.set(7);
		expect(localStorage.getItem("test.number")).toBe("7");
		expect(numberKey.remove()).toEqual({ ok: true });
		expect(localStorage.getItem("test.number")).toBeNull();
	});

	it("reports quota errors without throwing", () => {
		vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
			throw new DOMException("full", "QuotaExceededError");
		});
		expect(boolKey.set(true)).toEqual({ ok: false, reason: "quota" });
	});

	it("rejects writes whose encoding cannot decode back", () => {
		numberKey.set(7);
		const listener = vi.fn();
		const unsubscribe = store.sub(numberKey, listener);

		expect(numberKey.set(Number.NaN)).toEqual({ ok: false, reason: "invalid" });
		expect(numberKey.set(479.5)).toEqual({ ok: false, reason: "invalid" });
		expect(numberKey.set(Number.MAX_SAFE_INTEGER + 1)).toEqual({
			ok: false,
			reason: "invalid",
		});

		expect(listener).not.toHaveBeenCalled();
		expect(localStorage.getItem("test.number")).toBe("7");
		expect(numberKey.get()).toBe(7);
		unsubscribe();
	});

	it("reports unserializable values as invalid instead of throwing", () => {
		type Cyclic = Cyclic[];
		// Decode never runs here; encoding the cycle fails first.
		const cyclicKey = defineStorageKey<Cyclic | null>({
			key: "test.cycle",
			codec: jsonCodec<Cyclic>(() => undefined),
			defaultValue: null,
		});
		const cyclic: Cyclic = [];
		cyclic.push(cyclic);
		expect(cyclicKey.set(cyclic)).toEqual({ ok: false, reason: "invalid" });
		expect(localStorage.getItem("test.cycle")).toBeNull();
	});

	it("reports removal failures", () => {
		numberKey.set(7);
		vi.spyOn(Storage.prototype, "removeItem").mockImplementation(() => {
			throw new Error("denied");
		});
		expect(numberKey.remove()).toEqual({ ok: false, reason: "unavailable" });
		expect(localStorage.getItem("test.number")).toBe("7");
	});

	it("keeps reads on persisted bytes when persistence fails", () => {
		boolKey.set(true);
		const listener = vi.fn();
		const unsubscribe = store.sub(boolKey, listener);
		vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
			throw new DOMException("full", "QuotaExceededError");
		});

		expect(boolKey.set(false).ok).toBe(false);

		// Reads keep reflecting what actually persisted; callers can
		// inspect the returned PersistResult for their own handling.
		expect(listener).not.toHaveBeenCalled();
		expect(boolKey.get()).toBe(true);
		expect(localStorage.getItem("test.bool")).toBe("true");
		unsubscribe();
	});

	it("survives unavailable storage reads", () => {
		vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
			throw new Error("denied");
		});
		expect(boolKey.get()).toBe(false);
	});

	it("returns referentially stable snapshots until the bytes change", () => {
		listKey.set(["x"]);
		const unsub = store.sub(listKey, () => {});
		const first = store.get(listKey);
		expect(store.get(listKey)).toBe(first);

		localStorage.setItem("test.list", JSON.stringify(["x", "y"]));
		dispatchEvent(
			new StorageEvent("storage", {
				key: "test.list",
				storageArea: localStorage,
			}),
		);
		const second = store.get(listKey);
		expect(second).not.toBe(first);
		expect(store.get(listKey)).toBe(second);
		unsub();
	});

	it("returns a stable decoded snapshot after set", () => {
		const value = ["x"];
		listKey.set(value);
		const unsub = store.sub(listKey, () => {});
		const first = store.get(listKey);
		expect(first).toEqual(["x"]);
		expect(store.get(listKey)).toBe(first);

		// Mutating the caller's reference cannot change snapshots or
		// desync them from the stored bytes.
		value.push("y");
		expect(store.get(listKey)).toEqual(["x"]);
		unsub();
	});

	it("normalizes composite values to their persisted form", () => {
		const numbersKey = defineStorageKey<number[] | null>({
			key: "test.numbers",
			codec: jsonCodec((parsed) =>
				Array.isArray(parsed) &&
				parsed.every((item): item is number => typeof item === "number")
					? parsed
					: undefined,
			),
			defaultValue: null,
		});
		numbersKey.set([-0]);
		expect(localStorage.getItem("test.numbers")).toBe("[0]");
		expect(Object.is(numbersKey.get()?.[0], 0)).toBe(true);
	});

	it("decodes a shared key independently with each handle codec", () => {
		const asBool = defineStorageKey<boolean>({
			key: "test.shared",
			codec: booleanCodec,
			defaultValue: false,
		});
		const asString = defineStorageKey<string | null>({
			key: "test.shared",
			codec: stringCodec,
			defaultValue: null,
		});
		expect(asBool.get()).toBe(false);
		expect(asString.get()).toBeNull();

		localStorage.setItem("test.shared", "true");
		expect(asBool.get()).toBe(true);
		expect(asString.get()).toBe("true");
	});

	it("supports sessionStorage-backed keys", () => {
		sessionKey.set("once");
		expect(sessionStorage.getItem("test.session")).toBe("once");
		expect(localStorage.getItem("test.session")).toBeNull();
		expect(sessionKey.get()).toBe("once");
	});

	it("notifies subscribers on set and remove", () => {
		const listener = vi.fn();
		const unsubscribe = store.sub(boolKey, listener);
		boolKey.set(true);
		expect(listener).toHaveBeenCalledTimes(1);
		boolKey.remove();
		expect(listener).toHaveBeenCalledTimes(2);
		unsubscribe();
		boolKey.set(false);
		expect(listener).toHaveBeenCalledTimes(2);
	});

	it("invalidates snapshots on cross-tab storage events", () => {
		boolKey.set(false);
		expect(boolKey.get()).toBe(false);
		// Another tab writes the key: no set() runs here, only the event.
		const listener = vi.fn();
		const unsubscribe = store.sub(boolKey, listener);
		localStorage.setItem("test.bool", "true");
		dispatchEvent(
			new StorageEvent("storage", {
				key: "test.bool",
				storageArea: localStorage,
			}),
		);
		expect(listener).toHaveBeenCalledTimes(1);
		expect(boolKey.get()).toBe(true);
		unsubscribe();
	});

	it("notifies session-backed keys on events from same-tab frames", () => {
		sessionKey.set("stale");
		// A same-origin iframe writes the shared session area: no set()
		// runs in this document, only the event.
		const listener = vi.fn();
		const unsubscribe = store.sub(sessionKey, listener);
		sessionStorage.setItem("test.session", "fresh");
		dispatchEvent(
			new StorageEvent("storage", {
				key: "test.session",
				storageArea: sessionStorage,
			}),
		);
		expect(listener).toHaveBeenCalledTimes(1);
		expect(sessionKey.get()).toBe("fresh");
		unsubscribe();
	});

	it("notifies every local key on a cross-tab clear", () => {
		boolKey.set(true);
		const listener = vi.fn();
		const unsubscribe = store.sub(boolKey, listener);
		localStorage.clear();
		dispatchEvent(
			new StorageEvent("storage", { key: null, storageArea: localStorage }),
		);
		expect(listener).toHaveBeenCalledTimes(1);
		expect(boolKey.get()).toBe(false);
		unsubscribe();
	});
});

describe("entity-scoped keys", () => {
	beforeEach(() => {
		localStorage.clear();
	});

	it("memoizes handles per entity ID", () => {
		expect(chatNote.forId("chat-1")).toBe(chatNote.forId("chat-1"));
		expect(chatNote.forId("chat-1")).not.toBe(chatNote.forId("chat-2"));
	});

	it("keeps values in the pre-existing raw formats", () => {
		chatNote.forId("chat-1").set("draft");
		// The value bytes stay in the legacy raw format so clients from
		// before this module can still read them.
		expect(localStorage.getItem("test.chat-note.chat-1")).toBe("draft");
		expect(chatNote.forId("chat-1").get()).toBe("draft");

		localStorage.setItem("test.chat-tabs.chat-1", JSON.stringify(["files"]));
		expect(chatTabs.forId("chat-1").get()).toEqual(["files"]);
	});

	it("clears only the family's keys owned by the given ID", () => {
		chatNote.forId("chat-1").set("draft");
		chatTabs.forId("chat-1").set(["files"]);
		chatComposite.forId("org-1", "chat-1").set("attachment");
		chatNote.forId("chat-2").set("other chat");

		chatNote.clear("chat-1");
		chatComposite.clear("chat-1");

		expect(localStorage.getItem("test.chat-note.chat-1")).toBeNull();
		expect(localStorage.getItem("test.chat-composite.org-1.chat-1")).toBeNull();
		// Other chats and other families are untouched.
		expect(localStorage.getItem("test.chat-note.chat-2")).not.toBeNull();
		expect(localStorage.getItem("test.chat-tabs.chat-1")).not.toBeNull();
	});

	it("lists stored suffixes for the family", () => {
		chatComposite.forId("org-1", "chat-1").set("attachment");
		chatComposite.forId("org-2", "chat-2").set("attachment");
		chatNote.forId("chat-3").set("draft");

		expect(chatComposite.listStoredSuffixes().sort()).toEqual([
			"org-1.chat-1",
			"org-2.chat-2",
		]);
	});

	it("reports unavailable enumeration during entity cleanup", () => {
		const handle = chatNote.forId("chat-1");
		handle.set("draft");
		vi.spyOn(Storage.prototype, "key").mockImplementation(() => {
			throw new Error("denied");
		});

		// The matching value may remain, so this must not claim success.
		expect(chatNote.clear("chat-1")).toEqual({
			ok: false,
			reason: "unavailable",
		});
		expect(localStorage.getItem("test.chat-note.chat-1")).toBe("draft");
	});

	it("notifies subscribers when entity cleanup removes their key", () => {
		const handle = chatNote.forId("chat-1");
		handle.set("draft");
		const listener = vi.fn();
		const unsubscribe = store.sub(handle, listener);
		chatNote.clear("chat-1");
		expect(listener).toHaveBeenCalledTimes(1);
		expect(handle.get()).toBeNull();
		unsubscribe();
	});

	it("ignores empty entity IDs", () => {
		chatNote.forId("chat-1").set("draft");
		expect(chatNote.clear("")).toEqual({ ok: true });
		expect(localStorage.getItem("test.chat-note.chat-1")).not.toBeNull();
	});

	it("reports cleanup failures and keeps unremoved keys unnotified", () => {
		chatNote.forId("chat-1").set("note");
		const listener = vi.fn();
		const unsubscribe = store.sub(chatNote.forId("chat-1"), listener);
		vi.spyOn(Storage.prototype, "removeItem").mockImplementation(() => {
			throw new Error("denied");
		});

		expect(chatNote.clear("chat-1")).toEqual({
			ok: false,
			reason: "unavailable",
		});
		expect(listener).not.toHaveBeenCalled();
		expect(localStorage.getItem("test.chat-note.chat-1")).toBe("note");
		unsubscribe();

		vi.restoreAllMocks();
		expect(chatNote.clear("chat-1")).toEqual({ ok: true });
		expect(localStorage.getItem("test.chat-note.chat-1")).toBeNull();
	});

	it("serves an inert handle for empty entity ID parts", () => {
		for (const handle of [chatNote.forId(""), chatNote.forId()]) {
			expect(handle.get()).toBeNull();
			expect(handle.set("value")).toEqual({ ok: false, reason: "invalid" });
			expect(handle.remove()).toEqual({ ok: true });
		}
		expect(localStorage.getItem("test.chat-note.")).toBeNull();
		expect(chatNote.listStoredSuffixes()).toEqual([]);
	});

	it("serves an inert handle for ID parts containing the delimiter", () => {
		// forId("org.a", "chat") and forId("org", "a.chat") would
		// otherwise collide onto the same key.
		const handle = chatComposite.forId("org.a", "chat");
		expect(handle.set("value")).toEqual({ ok: false, reason: "invalid" });
		expect(chatComposite.listStoredSuffixes()).toEqual([]);
	});
});

describe("Jotai persistence", () => {
	beforeEach(() => {
		localStorage.clear();
		sessionStorage.clear();
	});
	it("hydrates each store lazily without writing defaults", () => {
		expect(store.get(boolKey)).toBe(false);
		expect(localStorage.getItem("test.bool")).toBeNull();
		localStorage.setItem("test.bool", "true");
		expect(createStore().get(boolKey)).toBe(true);
	});
	it("composes functional updates against fresh storage", () => {
		store.set(numberKey, (prev) => (prev ?? 0) + 1);
		localStorage.setItem("test.number", "4");
		store.set(numberKey, (prev) => (prev ?? 0) + 1);
		expect(store.get(numberKey)).toBe(5);
		expect(localStorage.getItem("test.number")).toBe("5");
	});
	it("resets through the atom setter", () => {
		store.set(boolKey, true);
		expect(store.set(boolKey, RESET)).toEqual({ ok: true });
		expect(store.get(boolKey)).toBe(false);
		expect(localStorage.getItem("test.bool")).toBeNull();
	});
	it("does not publish failed writes or removals", () => {
		store.set(boolKey, true);
		const listener = vi.fn();
		const unsub = store.sub(boolKey, listener);
		vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
			throw new DOMException("full", "QuotaExceededError");
		});
		expect(store.set(boolKey, false)).toEqual({ ok: false, reason: "quota" });
		expect(store.get(boolKey)).toBe(true);
		expect(localStorage.getItem("test.bool")).toBe("true");
		vi.spyOn(Storage.prototype, "removeItem").mockImplementation(() => {
			throw new Error("denied");
		});
		expect(store.set(boolKey, RESET)).toEqual({
			ok: false,
			reason: "unavailable",
		});
		expect(store.get(boolKey)).toBe(true);
		expect(listener).not.toHaveBeenCalled();
		unsub();
	});
	it("publishes object bytes once and keeps equal snapshots stable", () => {
		const listener = vi.fn();
		const unsub = store.sub(listKey, listener);
		try {
			store.set(listKey, ["one"]);
			expect(listener).toHaveBeenCalledTimes(1);
			const snapshot = store.get(listKey);
			listKey.set(["one"]);
			expect(store.get(listKey)).toBe(snapshot);
			expect(listener).toHaveBeenCalledTimes(1);
		} finally {
			unsub();
		}
	});
	it("refreshes other mounted stores after atom and imperative writes", () => {
		const other = createStore();
		const unsub = store.sub(boolKey, () => {});
		const unsubOther = other.sub(boolKey, () => {});
		store.set(boolKey, true);
		expect(other.get(boolKey)).toBe(true);
		boolKey.remove();
		expect(other.get(boolKey)).toBe(false);
		expect(store.get(boolKey)).toBe(false);
		unsub();
		unsubOther();
	});
	it("validates cross-tab events and ignores other storage areas", () => {
		store.set(listKey, ["valid"]);
		const unsub = store.sub(listKey, () => {});
		localStorage.setItem("test.list", "123");
		dispatchEvent(
			new StorageEvent("storage", {
				key: "test.list",
				storageArea: sessionStorage,
			}),
		);
		expect(store.get(listKey)).toEqual(["valid"]);
		dispatchEvent(
			new StorageEvent("storage", {
				key: "test.list",
				newValue: "123",
				storageArea: localStorage,
			}),
		);
		expect(store.get(listKey)).toBeNull();
		unsub();
	});
	it("survives a blocked storage getter during creation and mount", () => {
		vi.spyOn(window, "localStorage", "get").mockImplementation(() => {
			throw new DOMException("denied", "SecurityError");
		});
		const preference = defineStorageKey({
			key: "blocked",
			codec: booleanCodec,
			defaultValue: false,
		});
		const unsub = store.sub(preference, () => {});
		expect(store.get(preference)).toBe(false);
		expect(store.set(preference, true)).toEqual({
			ok: false,
			reason: "unavailable",
		});
		expect(store.get(preference)).toBe(false);
		unsub();
	});
});

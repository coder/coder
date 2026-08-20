import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useStorage } from "#/hooks/useStorage";
import {
	_resetStorageForTesting,
	booleanCodec,
	clearEntityStorage,
	defineEntityStorageKey,
	defineStorageKey,
	integerCodec,
	jsonCodec,
	registerLegacyStorageKeys,
	stringCodec,
	stringLiteralCodec,
	sweepExpiredStorage,
} from "./storage";

const isStringArray = (parsed: unknown): string[] | undefined =>
	Array.isArray(parsed) && parsed.every((item) => typeof item === "string")
		? (parsed as string[])
		: undefined;

// Test-only keys: the registry is module-level, so families are
// defined once here rather than importing the app registry.
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
	codec: jsonCodec(isStringArray),
	defaultValue: null,
});
const literalKey = defineStorageKey<"a" | "b">({
	key: "test.literal",
	codec: stringLiteralCodec(["a", "b"]),
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
	entity: "chat",
	codec: stringCodec,
	defaultValue: null,
});
const chatTabs = defineEntityStorageKey<readonly string[]>({
	prefix: "test.chat-tabs.",
	entity: "chat",
	codec: jsonCodec<readonly string[]>(isStringArray),
	defaultValue: [],
});
const chatComposite = defineEntityStorageKey<string | null>({
	prefix: "test.chat-composite.",
	entity: "chat",
	codec: stringCodec,
	defaultValue: null,
	entityIdFromSuffix: (suffix) => suffix.split(".").at(-1) ?? suffix,
});
const workspaceFlag = defineEntityStorageKey<boolean>({
	prefix: "test.workspace-flag.",
	entity: "workspace",
	codec: booleanCodec,
	defaultValue: false,
});
// Registered for its custom sweep behavior only. Mirrors chat draft
// attachments: values carry their own expiry information.
defineEntityStorageKey<string | null>({
	prefix: "test.custom-sweep.",
	entity: "chat",
	codec: stringCodec,
	defaultValue: null,
	sweepValue: (raw) => {
		if (raw === "stale") {
			return "remove";
		}
		if (raw.startsWith("trim:")) {
			return { rewrite: raw.slice("trim:".length) };
		}
		return "keep";
	},
});

registerLegacyStorageKeys(["test.legacy-one", "test.legacy-two"]);

afterEach(() => {
	vi.restoreAllMocks();
});

describe("storage core", () => {
	beforeEach(() => {
		localStorage.clear();
		sessionStorage.clear();
		_resetStorageForTesting();
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

	it("removes the key when setting null", () => {
		numberKey.set(7);
		expect(localStorage.getItem("test.number")).toBe("7");
		expect(numberKey.set(null)).toEqual({ ok: true });
		expect(localStorage.getItem("test.number")).toBeNull();
	});

	it("reports quota errors without throwing", () => {
		vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
			throw new DOMException("full", "QuotaExceededError");
		});
		expect(boolKey.set(true)).toEqual({ ok: false, reason: "quota" });
	});

	it("keeps reads on persisted bytes when persistence fails", () => {
		boolKey.set(true);
		const listener = vi.fn();
		const unsubscribe = boolKey.subscribe(listener);
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
		const first = listKey.getSnapshot();
		expect(listKey.getSnapshot()).toBe(first);

		localStorage.setItem("test.list", JSON.stringify(["x", "y"]));
		const second = listKey.getSnapshot();
		expect(second).not.toBe(first);
		expect(listKey.getSnapshot()).toBe(second);
	});

	it("hands back the exact reference passed to set", () => {
		const value = ["x"];
		listKey.set(value);
		expect(listKey.getSnapshot()).toBe(value);
	});

	it("supports sessionStorage-backed keys", () => {
		sessionKey.set("once");
		expect(sessionStorage.getItem("test.session")).toBe("once");
		expect(localStorage.getItem("test.session")).toBeNull();
		expect(sessionKey.get()).toBe("once");
	});

	it("notifies subscribers on set and remove", () => {
		const listener = vi.fn();
		const unsubscribe = boolKey.subscribe(listener);
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
		localStorage.setItem("test.bool", "true");
		const listener = vi.fn();
		const unsubscribe = boolKey.subscribe(listener);
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

	it("notifies every local key on a cross-tab clear", () => {
		boolKey.set(true);
		const listener = vi.fn();
		const unsubscribe = boolKey.subscribe(listener);
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
		_resetStorageForTesting();
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

	it("clears every key owned by an entity across families", () => {
		chatNote.forId("chat-1").set("draft");
		chatTabs.forId("chat-1").set(["files"]);
		chatComposite.forId("org-1", "chat-1").set("attachment");
		chatNote.forId("chat-2").set("other chat");
		workspaceFlag.forId("chat-1").set(true);

		clearEntityStorage("chat", "chat-1");

		expect(localStorage.getItem("test.chat-note.chat-1")).toBeNull();
		expect(localStorage.getItem("test.chat-tabs.chat-1")).toBeNull();
		expect(localStorage.getItem("test.chat-composite.org-1.chat-1")).toBeNull();
		// Other chats and other entity types are untouched.
		expect(localStorage.getItem("test.chat-note.chat-2")).not.toBeNull();
		expect(localStorage.getItem("test.workspace-flag.chat-1")).not.toBeNull();
	});

	it("survives unavailable enumeration during entity cleanup", () => {
		const handle = chatNote.forId("chat-1");
		handle.set("draft");
		vi.spyOn(Storage.prototype, "key").mockImplementation(() => {
			throw new Error("denied");
		});

		expect(() => clearEntityStorage("chat", "chat-1")).not.toThrow();
	});

	it("notifies subscribers when entity cleanup removes their key", () => {
		const handle = chatNote.forId("chat-1");
		handle.set("draft");
		const listener = vi.fn();
		const unsubscribe = handle.subscribe(listener);
		clearEntityStorage("chat", "chat-1");
		expect(listener).toHaveBeenCalledTimes(1);
		expect(handle.get()).toBeNull();
		unsubscribe();
	});

	it("ignores empty entity IDs", () => {
		chatNote.forId("chat-1").set("draft");
		clearEntityStorage("chat", "");
		expect(localStorage.getItem("test.chat-note.chat-1")).not.toBeNull();
	});
});

describe("sweepExpiredStorage", () => {
	beforeEach(() => {
		localStorage.clear();
		_resetStorageForTesting();
	});

	it("applies custom family sweep logic", () => {
		localStorage.setItem("test.custom-sweep.a", "stale");
		localStorage.setItem("test.custom-sweep.b", "fresh");
		localStorage.setItem("test.custom-sweep.c", "trim:partial");
		// Families without custom sweep logic are untouched.
		localStorage.setItem("test.chat-note.chat-1", "draft");

		sweepExpiredStorage(Date.now());

		expect(localStorage.getItem("test.custom-sweep.a")).toBeNull();
		expect(localStorage.getItem("test.custom-sweep.b")).toBe("fresh");
		expect(localStorage.getItem("test.custom-sweep.c")).toBe("partial");
		expect(localStorage.getItem("test.chat-note.chat-1")).toBe("draft");
	});

	it("removes registered legacy keys unconditionally", () => {
		localStorage.setItem("test.legacy-one", "anything");
		localStorage.setItem("test.legacy-two", "anything");
		localStorage.setItem("test.unrelated", "kept");

		sweepExpiredStorage(Date.now());

		expect(localStorage.getItem("test.legacy-one")).toBeNull();
		expect(localStorage.getItem("test.legacy-two")).toBeNull();
		expect(localStorage.getItem("test.unrelated")).toBe("kept");
	});

	it("runs once per session", () => {
		sweepExpiredStorage(Date.now());
		localStorage.setItem("test.legacy-one", "written after first sweep");
		sweepExpiredStorage(Date.now());
		expect(localStorage.getItem("test.legacy-one")).toBe(
			"written after first sweep",
		);
	});
});

describe("useStorage", () => {
	beforeEach(() => {
		localStorage.clear();
		_resetStorageForTesting();
	});

	it("reads the stored value and never writes the default on mount", () => {
		const { result } = renderHook(() => useStorage(boolKey));
		expect(result.current[0]).toBe(false);
		expect(localStorage.getItem("test.bool")).toBeNull();
	});

	it("updates every hook on the same key in the same tab", () => {
		const first = renderHook(() => useStorage(boolKey));
		const second = renderHook(() => useStorage(boolKey));

		act(() => {
			first.result.current[1](true);
		});

		expect(first.result.current[0]).toBe(true);
		expect(second.result.current[0]).toBe(true);
		expect(localStorage.getItem("test.bool")).toBe("true");
	});

	it("updates on cross-tab storage events", () => {
		const { result } = renderHook(() => useStorage(boolKey));
		expect(result.current[0]).toBe(false);

		act(() => {
			localStorage.setItem("test.bool", "true");
			dispatchEvent(
				new StorageEvent("storage", {
					key: "test.bool",
					storageArea: localStorage,
				}),
			);
		});

		expect(result.current[0]).toBe(true);
	});

	it("removes the value through the remove callback", () => {
		boolKey.set(true);
		const { result } = renderHook(() => useStorage(boolKey));
		expect(result.current[0]).toBe(true);

		act(() => {
			result.current[2]();
		});

		expect(result.current[0]).toBe(false);
		expect(localStorage.getItem("test.bool")).toBeNull();
	});

	it("works with entity-scoped handles", () => {
		const { result } = renderHook(() => useStorage(chatNote.forId("chat-9")));

		act(() => {
			result.current[1]("draft");
		});

		expect(result.current[0]).toBe("draft");
		expect(chatNote.forId("chat-9").get()).toBe("draft");
	});

	it("evaluates updaters against the storage snapshot", () => {
		const { result } = renderHook(() => useStorage(numberKey));

		// Two updater calls before a render commits compose instead of
		// both reading the same stale render closure.
		act(() => {
			result.current[1]((prev) => (prev ?? 0) + 1);
			result.current[1]((prev) => (prev ?? 0) + 1);
		});

		expect(result.current[0]).toBe(2);
		expect(localStorage.getItem("test.number")).toBe("2");
	});
});

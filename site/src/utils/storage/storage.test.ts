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

const dayMs = 24 * 60 * 60 * 1000;
const chatNote = defineEntityStorageKey<string | null>({
	prefix: "test.chat-note.",
	entity: "chat",
	codec: stringCodec,
	defaultValue: null,
	ttlMs: 30 * dayMs,
});
const chatTabs = defineEntityStorageKey<readonly string[]>({
	prefix: "test.chat-tabs.",
	entity: "chat",
	codec: jsonCodec<readonly string[]>(isStringArray),
	defaultValue: [],
	ttlMs: 90 * dayMs,
});
const chatComposite = defineEntityStorageKey<string | null>({
	prefix: "test.chat-composite.",
	entity: "chat",
	codec: stringCodec,
	defaultValue: null,
	ttlMs: 30 * dayMs,
	entityIdFromSuffix: (suffix) => suffix.split(".").at(-1) ?? suffix,
});
const workspaceFlag = defineEntityStorageKey<boolean>({
	prefix: "test.workspace-flag.",
	entity: "workspace",
	codec: booleanCodec,
	defaultValue: false,
	ttlMs: 90 * dayMs,
});
// Registered for its custom sweep behavior only.
defineEntityStorageKey<string | null>({
	prefix: "test.custom-sweep.",
	entity: "chat",
	codec: stringCodec,
	defaultValue: null,
	ttlMs: 30 * dayMs,
	timestamped: false,
	sweepValue: (raw) => (raw === "stale" ? "remove" : "keep"),
});

registerLegacyStorageKeys(["test.legacy-one", "test.legacy-two"]);

const timestampKeyFor = (key: string): string =>
	`coder.storage.timestamps.${key}`;

const envelopeFor = (data: string, atMs: number): string =>
	JSON.stringify({ v: 1, t: atMs, d: data });

const stampFor = (key: string, atMs: number): void => {
	const raw = localStorage.getItem(key);
	if (raw === null) {
		throw new Error(`no value stored at ${key}`);
	}
	// Mirrors the module's FNV-1a fingerprint.
	let hash = 0x811c9dc5;
	for (let index = 0; index < raw.length; index++) {
		hash ^= raw.charCodeAt(index);
		hash = Math.imul(hash, 0x01000193);
	}
	localStorage.setItem(
		timestampKeyFor(key),
		JSON.stringify({ t: atMs, h: (hash >>> 0).toString(16) }),
	);
};

describe("storage core", () => {
	beforeEach(() => {
		localStorage.clear();
		sessionStorage.clear();
		_resetStorageForTesting();
	});

	afterEach(() => {
		vi.restoreAllMocks();
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
		const listener = vi.fn();
		const unsubscribe = boolKey.subscribe(listener);
		vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
			throw new DOMException("full", "QuotaExceededError");
		});
		expect(boolKey.set(true)).toEqual({ ok: false, reason: "quota" });
		expect(listener).not.toHaveBeenCalled();
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

	it("writes the raw value plus a companion timestamp", () => {
		vi.spyOn(Date, "now").mockReturnValue(1000);
		chatNote.forId("chat-1").set("draft");
		// The value bytes stay in the legacy raw format so older
		// clients can still read them.
		expect(localStorage.getItem("test.chat-note.chat-1")).toBe("draft");
		const stamp = JSON.parse(
			localStorage.getItem(timestampKeyFor("test.chat-note.chat-1")) ?? "null",
		);
		expect(stamp.t).toBe(1000);
		expect(typeof stamp.h).toBe("string");
		expect(chatNote.forId("chat-1").get()).toBe("draft");
	});

	it("reads values written before companions existed", () => {
		localStorage.setItem("test.chat-note.chat-1", "old draft");
		expect(chatNote.forId("chat-1").get()).toBe("old draft");

		localStorage.setItem("test.chat-tabs.chat-1", JSON.stringify(["files"]));
		expect(chatTabs.forId("chat-1").get()).toEqual(["files"]);
	});

	it("removes the companion timestamp along with the value", () => {
		const handle = chatNote.forId("chat-1");
		handle.set("draft");
		expect(
			localStorage.getItem(timestampKeyFor("test.chat-note.chat-1")),
		).not.toBeNull();
		handle.remove();
		expect(localStorage.getItem("test.chat-note.chat-1")).toBeNull();
		expect(
			localStorage.getItem(timestampKeyFor("test.chat-note.chat-1")),
		).toBeNull();
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

	it("removes values with expired companions and keeps fresh ones", () => {
		const now = 100 * dayMs;
		localStorage.setItem("test.chat-note.old", "stale draft");
		stampFor("test.chat-note.old", now - 31 * dayMs);
		localStorage.setItem("test.chat-note.new", "fresh draft");
		stampFor("test.chat-note.new", now - dayMs);
		localStorage.setItem("test.chat-tabs.old", JSON.stringify(["files"]));
		stampFor("test.chat-tabs.old", now - 89 * dayMs);

		sweepExpiredStorage(now);

		expect(localStorage.getItem("test.chat-note.old")).toBeNull();
		expect(
			localStorage.getItem(timestampKeyFor("test.chat-note.old")),
		).toBeNull();
		expect(localStorage.getItem("test.chat-note.new")).toBe("fresh draft");
		// 89 days is within the 90 day preference TTL.
		expect(localStorage.getItem("test.chat-tabs.old")).not.toBeNull();
	});

	it("stamps unstamped values so their TTL clock starts", () => {
		const now = 100 * dayMs;
		localStorage.setItem("test.chat-note.legacy", "legacy draft");

		sweepExpiredStorage(now);

		// The value bytes are untouched; only a companion appears.
		expect(localStorage.getItem("test.chat-note.legacy")).toBe("legacy draft");
		const stamp = JSON.parse(
			localStorage.getItem(timestampKeyFor("test.chat-note.legacy")) ?? "null",
		);
		expect(stamp.t).toBe(now);
		expect(chatNote.forId("legacy").get()).toBe("legacy draft");
	});

	it("re-stamps values changed by clients that do not write companions", () => {
		const now = 100 * dayMs;
		localStorage.setItem("test.chat-note.shared", "first");
		stampFor("test.chat-note.shared", now - 60 * dayMs);
		// An old client overwrites the value without updating the stamp.
		localStorage.setItem("test.chat-note.shared", "updated by old client");

		sweepExpiredStorage(now);

		// The stale stamp must not expire the freshly updated value.
		expect(localStorage.getItem("test.chat-note.shared")).toBe(
			"updated by old client",
		);
		const stamp = JSON.parse(
			localStorage.getItem(timestampKeyFor("test.chat-note.shared")) ?? "null",
		);
		expect(stamp.t).toBe(now);
	});

	it("migrates pre-release envelope values back to raw values", () => {
		const now = 100 * dayMs;
		localStorage.setItem(
			"test.chat-note.wrapped",
			envelopeFor("wrapped draft", now - dayMs),
		);
		localStorage.setItem(
			"test.chat-note.expired-wrap",
			envelopeFor("stale draft", now - 31 * dayMs),
		);

		sweepExpiredStorage(now);

		expect(localStorage.getItem("test.chat-note.wrapped")).toBe(
			"wrapped draft",
		);
		const stamp = JSON.parse(
			localStorage.getItem(timestampKeyFor("test.chat-note.wrapped")) ?? "null",
		);
		// The original write time is preserved through the migration.
		expect(stamp.t).toBe(now - dayMs);
		expect(localStorage.getItem("test.chat-note.expired-wrap")).toBeNull();
	});

	it("removes orphaned companion timestamps", () => {
		localStorage.setItem(timestampKeyFor("test.chat-note.gone"), "{}");

		sweepExpiredStorage(Date.now());

		expect(
			localStorage.getItem(timestampKeyFor("test.chat-note.gone")),
		).toBeNull();
	});

	it("applies custom family sweep logic", () => {
		localStorage.setItem("test.custom-sweep.a", "stale");
		localStorage.setItem("test.custom-sweep.b", "fresh");

		sweepExpiredStorage(Date.now());

		expect(localStorage.getItem("test.custom-sweep.a")).toBeNull();
		expect(localStorage.getItem("test.custom-sweep.b")).toBe("fresh");
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
});

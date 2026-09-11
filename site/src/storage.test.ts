import { createStore } from "jotai/vanilla";
import { atomWithStorage, RESET } from "jotai/vanilla/utils";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { array, string } from "yup";
import {
	atomFamilyWithStorage,
	booleanCodec,
	createCodecStorage,
	integerCodec,
	jsonCodec,
	stringCodec,
	stringLiteralCodec,
	yupCodec,
} from "./storage";

const stringArraySchema = array(string().defined()).defined();

const boolAtom = atomWithStorage(
	"test.bool",
	false,
	createCodecStorage(() => localStorage, booleanCodec),
	{ getOnInit: true },
);
const numberAtom = atomWithStorage<number | null>(
	"test.number",
	null,
	createCodecStorage<number | null>(() => localStorage, integerCodec),
	{ getOnInit: true },
);
const listAtom = atomWithStorage<string[] | null>(
	"test.list",
	null,
	createCodecStorage<string[] | null>(
		() => localStorage,
		yupCodec(stringArraySchema),
	),
	{ getOnInit: true },
);
const literalAtom = atomWithStorage<"a" | "b">(
	"test.literal",
	"a",
	createCodecStorage(
		() => localStorage,
		stringLiteralCodec({ oneOf: ["a", "b"] }),
	),
	{ getOnInit: true },
);
const sessionAtom = atomWithStorage<string | null>(
	"test.session",
	null,
	createCodecStorage<string | null>(() => sessionStorage, stringCodec),
	{ getOnInit: true },
);

const chatNoteAtoms = atomFamilyWithStorage<string | null>({
	prefix: "test.chat-note.",
	codec: stringCodec,
	initialValue: null,
});
const chatTabsAtoms = atomFamilyWithStorage<readonly string[]>({
	prefix: "test.chat-tabs.",
	codec: yupCodec<readonly string[]>(stringArraySchema),
	initialValue: [],
});
const chatCompositeAtoms = atomFamilyWithStorage<string | null>({
	prefix: "test.chat-composite.",
	codec: stringCodec,
	initialValue: null,
	entityIdFromSuffix: (suffix) => suffix.split(".").at(-1) ?? suffix,
});

describe("storage atoms", () => {
	beforeEach(() => {
		localStorage.clear();
		sessionStorage.clear();
		vi.restoreAllMocks();
	});

	it("returns defaults without writing them", () => {
		const store = createStore();
		expect(store.get(boolAtom)).toBe(false);
		expect(store.get(numberAtom)).toBeNull();
		expect(localStorage.getItem("test.bool")).toBeNull();
		expect(localStorage.getItem("test.number")).toBeNull();
	});

	it("reads existing values through their codecs", () => {
		localStorage.setItem("test.bool", "true");
		localStorage.setItem("test.number", "42");
		localStorage.setItem("test.list", JSON.stringify(["x", "y"]));
		localStorage.setItem("test.literal", "b");
		const store = createStore();
		const unsubscribers = [boolAtom, numberAtom, listAtom, literalAtom].map(
			(storageAtom) => store.sub(storageAtom, () => {}),
		);

		expect(store.get(boolAtom)).toBe(true);
		expect(store.get(numberAtom)).toBe(42);
		expect(store.get(listAtom)).toEqual(["x", "y"]);
		expect(store.get(literalAtom)).toBe("b");
		for (const unsubscribe of unsubscribers) {
			unsubscribe();
		}
	});

	it("writes values in the pre-existing raw formats", () => {
		const store = createStore();
		const jsonAtom = atomWithStorage<number[] | null>(
			"test.json",
			null,
			createCodecStorage<number[] | null>(
				() => localStorage,
				jsonCodec((parsed) =>
					Array.isArray(parsed) &&
					parsed.every((value): value is number => typeof value === "number")
						? parsed
						: undefined,
				),
			),
			{ getOnInit: true },
		);
		store.set(boolAtom, true);
		store.set(numberAtom, 42);
		store.set(listAtom, ["x", "y"]);
		store.set(literalAtom, "b");
		store.set(jsonAtom, [1, 2]);

		expect(localStorage.getItem("test.bool")).toBe("true");
		expect(localStorage.getItem("test.number")).toBe("42");
		expect(localStorage.getItem("test.list")).toBe('["x","y"]');
		expect(localStorage.getItem("test.literal")).toBe("b");
		expect(localStorage.getItem("test.json")).toBe("[1,2]");
	});

	it("supports functional updates and RESET removal", () => {
		const store = createStore();
		store.set(numberAtom, 1);
		store.set(numberAtom, (previous) => (previous ?? 0) + 1);
		expect(store.get(numberAtom)).toBe(2);
		expect(localStorage.getItem("test.number")).toBe("2");

		store.set(numberAtom, RESET);
		expect(store.get(numberAtom)).toBeNull();
		expect(localStorage.getItem("test.number")).toBeNull();
	});

	it("falls back to defaults for corrupted values", () => {
		localStorage.setItem("test.list", "{not json");
		localStorage.setItem("test.number", "12px");
		localStorage.setItem("test.literal", "c");
		const store = createStore();

		expect(store.get(listAtom)).toBeNull();
		expect(store.get(numberAtom)).toBeNull();
		expect(store.get(literalAtom)).toBe("a");
	});

	it("rejects invalid encoded values without throwing", () => {
		const store = createStore();
		store.set(numberAtom, 7);

		store.set(numberAtom, Number.NaN);
		store.set(numberAtom, 479.5);
		store.set(numberAtom, Number.MAX_SAFE_INTEGER + 1);

		expect(localStorage.getItem("test.number")).toBe("7");
	});

	it("treats unavailable storage as best effort", () => {
		vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
			throw new Error("denied");
		});
		const store = createStore();
		expect(store.get(boolAtom)).toBe(false);

		vi.restoreAllMocks();
		vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
			throw new DOMException("full", "QuotaExceededError");
		});
		expect(() => store.set(boolAtom, true)).not.toThrow();
	});

	it("updates mounted atoms from storage events", () => {
		const store = createStore();
		const listener = vi.fn();
		const unsubscribe = store.sub(boolAtom, listener);
		localStorage.setItem("test.bool", "true");

		dispatchEvent(
			new StorageEvent("storage", {
				key: "test.bool",
				storageArea: localStorage,
			}),
		);

		expect(store.get(boolAtom)).toBe(true);
		expect(listener).toHaveBeenCalledTimes(1);
		unsubscribe();
	});

	it("updates mounted atoms when another tab clears storage", () => {
		const store = createStore();
		store.set(boolAtom, true);
		const unsubscribe = store.sub(boolAtom, () => {});
		localStorage.clear();

		dispatchEvent(
			new StorageEvent("storage", {
				key: null,
				storageArea: localStorage,
			}),
		);

		expect(store.get(boolAtom)).toBe(false);
		unsubscribe();
	});

	it("supports session storage", () => {
		const store = createStore();
		store.set(sessionAtom, "once");
		expect(sessionStorage.getItem("test.session")).toBe("once");
		expect(localStorage.getItem("test.session")).toBeNull();
		expect(store.get(sessionAtom)).toBe("once");
	});
});

describe("entity storage atom families", () => {
	beforeEach(() => {
		localStorage.clear();
		vi.restoreAllMocks();
	});

	it("memoizes atoms and exposes their storage keys", () => {
		expect(chatNoteAtoms.forId("chat-1")).toBe(chatNoteAtoms.forId("chat-1"));
		expect(chatNoteAtoms.forId("chat-1")).not.toBe(
			chatNoteAtoms.forId("chat-2"),
		);
		expect(chatCompositeAtoms.keyFor("org-1", "chat-1")).toBe(
			"test.chat-composite.org-1.chat-1",
		);
	});

	it("reads and writes entity values through atoms", () => {
		const store = createStore();
		const noteAtom = chatNoteAtoms.forId("chat-1");
		store.set(noteAtom, "draft");
		expect(localStorage.getItem("test.chat-note.chat-1")).toBe("draft");
		expect(store.get(noteAtom)).toBe("draft");

		localStorage.setItem("test.chat-tabs.chat-1", JSON.stringify(["files"]));
		expect(store.get(chatTabsAtoms.forId("chat-1"))).toEqual(["files"]);
	});

	it("lists stored suffixes for the family", () => {
		const store = createStore();
		store.set(chatCompositeAtoms.forId("org-1", "chat-1"), "attachment");
		store.set(chatCompositeAtoms.forId("org-2", "chat-2"), "attachment");
		store.set(chatNoteAtoms.forId("chat-3"), "draft");

		expect(chatCompositeAtoms.listStoredSuffixes().sort()).toEqual([
			"org-1.chat-1",
			"org-2.chat-2",
		]);
	});

	it("clears only matching entity keys and resets cached atoms", () => {
		const store = createStore();
		const noteAtom = chatNoteAtoms.forId("chat-1");
		const compositeAtom = chatCompositeAtoms.forId("org-1", "chat-1");
		store.set(noteAtom, "draft");
		store.set(chatTabsAtoms.forId("chat-1"), ["files"]);
		store.set(compositeAtom, "attachment");
		store.set(chatNoteAtoms.forId("chat-2"), "other chat");

		chatNoteAtoms.clear("chat-1", store);
		chatCompositeAtoms.clear("chat-1", store);

		expect(store.get(noteAtom)).toBeNull();
		expect(store.get(compositeAtom)).toBeNull();
		expect(localStorage.getItem("test.chat-note.chat-1")).toBeNull();
		expect(localStorage.getItem("test.chat-composite.org-1.chat-1")).toBeNull();
		expect(localStorage.getItem("test.chat-note.chat-2")).toBe("other chat");
		expect(localStorage.getItem("test.chat-tabs.chat-1")).toBe('["files"]');
	});

	it("clears matching atoms that were not previously created", () => {
		localStorage.setItem("test.chat-note.chat-1", "draft");
		chatNoteAtoms.clear("chat-1", createStore());
		expect(localStorage.getItem("test.chat-note.chat-1")).toBeNull();
	});

	it("ignores invalid entity IDs", () => {
		const store = createStore();
		for (const storageAtom of [
			chatNoteAtoms.forId(),
			chatNoteAtoms.forId(""),
			chatCompositeAtoms.forId("org.one", "chat-1"),
		]) {
			expect(store.get(storageAtom)).toBeNull();
			store.set(storageAtom, "value");
			expect(store.get(storageAtom)).toBeNull();
		}
		expect(chatNoteAtoms.listStoredSuffixes()).toEqual([]);
		expect(chatCompositeAtoms.listStoredSuffixes()).toEqual([]);
	});

	it("ignores empty cleanup IDs", () => {
		const store = createStore();
		store.set(chatNoteAtoms.forId("chat-1"), "draft");
		chatNoteAtoms.clear("", store);
		expect(localStorage.getItem("test.chat-note.chat-1")).toBe("draft");
	});
});

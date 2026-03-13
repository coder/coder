import { afterEach, describe, expect, it, vi } from "vitest";
import { createBrowserChatPreferenceStore } from "./browserChatPreferenceStore";

class MemoryStorage implements Storage {
	#entries = new Map<string, string>();

	get length(): number {
		return this.#entries.size;
	}

	clear(): void {
		this.#entries.clear();
	}

	getItem(key: string): string | null {
		return this.#entries.get(key) ?? null;
	}

	key(index: number): string | null {
		return [...this.#entries.keys()][index] ?? null;
	}

	removeItem(key: string): void {
		this.#entries.delete(key);
	}

	setItem(key: string, value: string): void {
		this.#entries.set(key, value);
	}
}

afterEach(() => {
	vi.restoreAllMocks();
	vi.unstubAllGlobals();
});

describe("createBrowserChatPreferenceStore", () => {
	it("returns the fallback when a key does not exist", () => {
		const storage = new MemoryStorage();
		const store = createBrowserChatPreferenceStore({ storage });

		expect(store.get("missing", "fallback")).toBe("fallback");
	});

	it("returns the stored value after set", () => {
		const storage = new MemoryStorage();
		const store = createBrowserChatPreferenceStore({ storage });

		store.set("foo", { enabled: true, count: 3 });

		expect(store.get("foo", null)).toEqual({ enabled: true, count: 3 });
	});

	it("returns the fallback and removes malformed JSON for regular keys", () => {
		const storage = new MemoryStorage();
		storage.setItem("agents.chat.foo", "not-json");
		const store = createBrowserChatPreferenceStore({ storage });

		expect(store.get("foo", "fallback")).toBe("fallback");
		expect(storage.getItem("agents.chat.foo")).toBeNull();
	});

	it("reads the legacy raw-string selectedModel value", () => {
		const storage = new MemoryStorage();
		storage.setItem("agents.last-model-config-id", "model-legacy");
		const store = createBrowserChatPreferenceStore({ storage });

		expect(store.get("selectedModel", "fallback")).toBe("model-legacy");
	});

	it("returns the fallback when storage reads throw", () => {
		const storage: Storage = {
			length: 0,
			clear: () => undefined,
			getItem: () => {
				throw new Error("storage unavailable");
			},
			key: () => null,
			removeItem: () => undefined,
			setItem: () => undefined,
		};
		const store = createBrowserChatPreferenceStore({ storage });

		expect(store.get("foo", "fallback")).toBe("fallback");
	});

	it("writes JSON-serialized values for non-legacy keys", () => {
		const storage = new MemoryStorage();
		const store = createBrowserChatPreferenceStore({ storage });

		store.set("string", "hello");
		store.set("count", 42);
		store.set("prefs", { theme: "dark" });
		store.set("enabled", true);

		expect(storage.getItem("agents.chat.string")).toBe('"hello"');
		expect(storage.getItem("agents.chat.count")).toBe("42");
		expect(storage.getItem("agents.chat.prefs")).toBe('{"theme":"dark"}');
		expect(storage.getItem("agents.chat.enabled")).toBe("true");
	});

	it("maps selectedModel to the legacy storage key", () => {
		const storage = new MemoryStorage();
		const store = createBrowserChatPreferenceStore({ storage });

		store.set("selectedModel", "model-1");

		expect(storage.getItem("agents.last-model-config-id")).toBe("model-1");
		expect(storage.getItem("agents.chat.selectedModel")).toBeNull();
	});

	it("prefixes other keys under the agents.chat namespace", () => {
		const storage = new MemoryStorage();
		const store = createBrowserChatPreferenceStore({ storage });

		store.set("sidebarWidth", 320);

		expect(storage.getItem("agents.chat.sidebarWidth")).toBe("320");
	});

	it("notifies same-tab subscribers when the same key is set", () => {
		const storage = new MemoryStorage();
		const store = createBrowserChatPreferenceStore({ storage });
		const onFoo = vi.fn();
		const onBar = vi.fn();
		store.subscribe?.("foo", onFoo);
		store.subscribe?.("bar", onBar);

		store.set("foo", "value");

		expect(onFoo).toHaveBeenCalledTimes(1);
		expect(onBar).not.toHaveBeenCalled();
	});

	it("returns an unsubscribe function that stops same-tab notifications", () => {
		const storage = new MemoryStorage();
		const store = createBrowserChatPreferenceStore({ storage });
		const onChange = vi.fn();
		const unsubscribe = store.subscribe?.("foo", onChange) ?? (() => undefined);

		unsubscribe();
		store.set("foo", "value");

		expect(onChange).not.toHaveBeenCalled();
	});

	it("notifies cross-tab subscribers for matching storage keys", () => {
		const storage = new MemoryStorage();
		const store = createBrowserChatPreferenceStore({ storage });
		const onFoo = vi.fn();
		const onSelectedModel = vi.fn();
		store.subscribe?.("foo", onFoo);
		store.subscribe?.("selectedModel", onSelectedModel);

		window.dispatchEvent(
			new StorageEvent("storage", { key: "agents.chat.other-key" }),
		);
		window.dispatchEvent(
			new StorageEvent("storage", { key: "agents.chat.foo" }),
		);
		window.dispatchEvent(
			new StorageEvent("storage", { key: "agents.last-model-config-id" }),
		);

		expect(onFoo).toHaveBeenCalledTimes(1);
		expect(onSelectedModel).toHaveBeenCalledTimes(1);
	});

	it("removes the global storage listener after the last subscriber unsubscribes", () => {
		const storage = new MemoryStorage();
		const addEventListenerSpy = vi.spyOn(window, "addEventListener");
		const removeEventListenerSpy = vi.spyOn(window, "removeEventListener");
		const store = createBrowserChatPreferenceStore({ storage });

		const unsubscribeFoo =
			store.subscribe?.("foo", vi.fn()) ?? (() => undefined);
		const unsubscribeBar =
			store.subscribe?.("bar", vi.fn()) ?? (() => undefined);

		expect(addEventListenerSpy).toHaveBeenCalledWith(
			"storage",
			expect.any(Function),
		);
		removeEventListenerSpy.mockClear();

		unsubscribeFoo();
		expect(removeEventListenerSpy).not.toHaveBeenCalled();

		unsubscribeBar();
		expect(removeEventListenerSpy).toHaveBeenCalledTimes(1);
		expect(removeEventListenerSpy).toHaveBeenCalledWith(
			"storage",
			expect.any(Function),
		);
	});

	it("gracefully degrades when window is unavailable", () => {
		vi.stubGlobal("window", undefined);
		const storage = new MemoryStorage();
		const store = createBrowserChatPreferenceStore({ storage });

		expect(store.get("foo", "fallback")).toBe("fallback");
		expect(() => store.set("foo", "value")).not.toThrow();
		expect(store.subscribe?.("foo", vi.fn()) ?? (() => undefined)).toBeTypeOf(
			"function",
		);
	});
});

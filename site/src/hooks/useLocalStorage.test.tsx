import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import * as Yup from "yup";
import {
	LocalStorageProvider,
	useLocalStorageStore,
} from "#/contexts/LocalStorageProvider";
import {
	booleanCodec,
	type Codec,
	getLocalStorageValue,
	jsonCodec,
	useLocalStorage,
} from "./useLocalStorage";

const counterCodec = jsonCodec(Yup.number().required(), 0);
const stateCodec = jsonCodec(
	Yup.object({ enabled: Yup.boolean().required() }),
	{ enabled: false },
);

const dispatchCrossTabStorageEvent = (key: string | null) => {
	window.dispatchEvent(new StorageEvent("storage", { key }));
};

const renderLocalStorageHook = <T, P>(
	hook: (props: P) => T,
	options?: { initialProps?: P },
) => renderHook(hook, { ...options, wrapper: LocalStorageProvider });

describe("useLocalStorage", () => {
	beforeEach(() => {
		localStorage.clear();
	});

	afterEach(() => {
		vi.restoreAllMocks();
		vi.useRealTimers();
	});

	it("reads the fallback when the key is absent", () => {
		const { result } = renderLocalStorageHook(() =>
			useLocalStorage("test.absent", booleanCodec(true)),
		);
		expect(result.current[0]).toBe(true);
	});

	it("updates every consumer of a key in the same tab", () => {
		const key = "test.same-tab";
		const { result } = renderLocalStorageHook(() => ({
			first: useLocalStorage(key, booleanCodec(false)),
			second: useLocalStorage(key, booleanCodec(false)),
		}));

		act(() => result.current.first[1](true));

		expect(result.current.first[0]).toBe(true);
		expect(result.current.second[0]).toBe(true);
		expect(localStorage.getItem(key)).toBe("true");
	});

	it("updates consumers when another tab writes the key", () => {
		const key = "test.cross-tab";
		const { result } = renderLocalStorageHook(() =>
			useLocalStorage(key, booleanCodec(false)),
		);

		act(() => {
			localStorage.setItem(key, "true");
			dispatchCrossTabStorageEvent(key);
		});

		expect(result.current[0]).toBe(true);
	});

	it("updates consumers when another tab clears storage", () => {
		const key = "test.clear";
		localStorage.setItem(key, "true");
		const { result } = renderLocalStorageHook(() =>
			useLocalStorage(key, booleanCodec(false)),
		);

		act(() => {
			localStorage.clear();
			dispatchCrossTabStorageEvent(null);
		});

		expect(result.current[0]).toBe(false);
	});

	it("ignores storage events for other keys", () => {
		const key = "test.other-key";
		const { result } = renderLocalStorageHook(() =>
			useLocalStorage(key, booleanCodec(false)),
		);

		act(() => {
			localStorage.setItem(key, "true");
			dispatchCrossTabStorageEvent("unrelated");
		});

		expect(result.current[0]).toBe(false);
	});

	it("composes consecutive functional updates", () => {
		const key = "test.consecutive";
		const { result } = renderLocalStorageHook(() =>
			useLocalStorage(key, counterCodec),
		);

		act(() => {
			result.current[1]((prev) => prev + 1);
			result.current[1]((prev) => prev + 1);
		});

		expect(result.current[0]).toBe(2);
		expect(localStorage.getItem(key)).toBe("2");
	});

	it("keeps the in-memory value when a write fails", () => {
		const key = "test.failed-write";
		vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
			throw new DOMException("quota", "QuotaExceededError");
		});
		const { result } = renderLocalStorageHook(() =>
			useLocalStorage(key, booleanCodec(false)),
		);

		act(() => result.current[1](true));

		expect(result.current[0]).toBe(true);
		expect(localStorage.getItem(key)).toBeNull();
	});

	it("adopts another tab's value after a failed write", () => {
		const key = "test.failed-write-cross-tab";
		const setItem = vi
			.spyOn(Storage.prototype, "setItem")
			.mockImplementation(() => {
				throw new DOMException("quota", "QuotaExceededError");
			});
		const { result } = renderLocalStorageHook(() =>
			useLocalStorage(key, booleanCodec(false)),
		);
		act(() => result.current[1](true));

		setItem.mockRestore();
		act(() => {
			localStorage.setItem(key, "false");
			dispatchCrossTabStorageEvent(key);
		});

		expect(result.current[0]).toBe(false);
	});

	it("reads the fallback when storage access throws", () => {
		vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
			throw new DOMException("denied", "SecurityError");
		});
		const { result } = renderLocalStorageHook(() =>
			useLocalStorage("test.access-denied", booleanCodec(true)),
		);
		expect(result.current[0]).toBe(true);
	});

	it("removes the key when the codec serializes null", () => {
		const key = "test.remove";
		const nullableCodec: Codec<string | null> = {
			parse: (serialized) => serialized,
			serialize: (value) => value,
		};
		localStorage.setItem(key, "stored");
		const { result } = renderLocalStorageHook(() =>
			useLocalStorage(key, nullableCodec),
		);

		act(() => result.current[1](null));

		expect(localStorage.getItem(key)).toBeNull();
		expect(result.current[0]).toBeNull();
	});

	it("keeps unknown stored fields when a functional update spreads the previous value", () => {
		const key = "test.unknown-fields";
		localStorage.setItem(key, JSON.stringify({ enabled: true, future: 1 }));
		const { result } = renderLocalStorageHook(() =>
			useLocalStorage(key, stateCodec),
		);

		act(() => result.current[1]((prev) => ({ ...prev, enabled: false })));

		expect(JSON.parse(localStorage.getItem(key) ?? "null")).toEqual({
			enabled: false,
			future: 1,
		});
	});

	it("reads the fallback for stored JSON that fails validation", () => {
		const key = "test.invalid-json";
		localStorage.setItem(key, JSON.stringify({ enabled: "yes" }));
		const { result } = renderLocalStorageHook(() =>
			useLocalStorage(key, stateCodec),
		);
		expect(result.current[0]).toEqual({ enabled: false });
	});

	it("returns the same object until the stored value changes", () => {
		const key = "test.stable-object";
		localStorage.setItem(key, JSON.stringify({ enabled: true }));
		const { result, rerender } = renderLocalStorageHook(() =>
			useLocalStorage(key, stateCodec),
		);
		const firstValue = result.current[0];

		rerender();

		expect(result.current[0]).toBe(firstValue);
	});

	describe("debounceMs", () => {
		beforeEach(() => {
			vi.useFakeTimers();
		});

		it("updates the value immediately and delays the storage write", () => {
			const key = "test.debounce";
			const { result } = renderLocalStorageHook(() =>
				useLocalStorage(key, booleanCodec(false), { debounceMs: 500 }),
			);

			act(() => result.current[1](true));

			expect(result.current[0]).toBe(true);
			expect(localStorage.getItem(key)).toBeNull();

			act(() => {
				vi.advanceTimersByTime(500);
			});

			expect(localStorage.getItem(key)).toBe("true");
		});

		it("flushes a pending write on pagehide", () => {
			const key = "test.debounce-pagehide";
			const { result } = renderLocalStorageHook(() =>
				useLocalStorage(key, booleanCodec(false), { debounceMs: 500 }),
			);

			act(() => result.current[1](true));
			act(() => {
				window.dispatchEvent(new Event("pagehide"));
			});

			expect(localStorage.getItem(key)).toBe("true");
		});

		it("flushes a pending write on unmount", () => {
			const key = "test.debounce-unmount";
			const { result, unmount } = renderLocalStorageHook(() =>
				useLocalStorage(key, booleanCodec(false), { debounceMs: 500 }),
			);

			act(() => result.current[1](true));
			unmount();

			expect(localStorage.getItem(key)).toBe("true");
		});

		it("flushes a pending write when the key changes", () => {
			const { result, rerender } = renderLocalStorageHook(
				({ key }: { key: string }) =>
					useLocalStorage(key, booleanCodec(false), { debounceMs: 500 }),
				{ initialProps: { key: "test.debounce-key-a" } },
			);

			act(() => result.current[1](true));
			rerender({ key: "test.debounce-key-b" });

			expect(localStorage.getItem("test.debounce-key-a")).toBe("true");
		});
	});
});

describe("getLocalStorageValue", () => {
	beforeEach(() => {
		localStorage.clear();
	});

	it("reads the stored value outside React", () => {
		localStorage.setItem("test.getter", "true");
		const { result } = renderLocalStorageHook(() => useLocalStorageStore());

		expect(
			getLocalStorageValue(result.current, "test.getter", booleanCodec(false)),
		).toBe(true);
	});

	it("reflects a debounced write that has not flushed yet", () => {
		const key = "test.getter-pending";
		vi.useFakeTimers();
		const { result, unmount } = renderLocalStorageHook(() => ({
			store: useLocalStorageStore(),
			value: useLocalStorage(key, booleanCodec(false), { debounceMs: 500 }),
		}));

		act(() => result.current.value[1](true));

		expect(
			getLocalStorageValue(result.current.store, key, booleanCodec(false)),
		).toBe(true);
		expect(localStorage.getItem(key)).toBeNull();
		unmount();
		vi.useRealTimers();
	});
});

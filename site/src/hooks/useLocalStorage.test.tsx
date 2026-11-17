import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import * as Yup from "yup";
import { LocalStorageProvider } from "#/contexts/LocalStorageProvider";
import {
	booleanCodec,
	type Codec,
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
			useLocalStorage({ storageKey: "test.absent", codec: booleanCodec(true) }),
		);
		expect(result.current[0]).toBe(true);
	});

	it("updates every consumer of a key in the same tab", () => {
		const key = "test.same-tab";
		const { result } = renderLocalStorageHook(() => ({
			first: useLocalStorage({ storageKey: key, codec: booleanCodec(false) }),
			second: useLocalStorage({ storageKey: key, codec: booleanCodec(false) }),
		}));

		act(() => result.current.first[1](true));

		expect(result.current.first[0]).toBe(true);
		expect(result.current.second[0]).toBe(true);
		expect(localStorage.getItem(key)).toBe("true");
	});

	it("updates consumers when another tab writes the key", () => {
		const key = "test.cross-tab";
		const { result } = renderLocalStorageHook(() =>
			useLocalStorage({ storageKey: key, codec: booleanCodec(false) }),
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
			useLocalStorage({ storageKey: key, codec: booleanCodec(false) }),
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
			useLocalStorage({ storageKey: key, codec: booleanCodec(false) }),
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
			useLocalStorage({ storageKey: key, codec: counterCodec }),
		);

		act(() => {
			result.current[1]((prev) => prev + 1);
			result.current[1]((prev) => prev + 1);
		});

		expect(result.current[0]).toBe(2);
		expect(localStorage.getItem(key)).toBe("2");
	});

	it("keeps the stored value when a write fails", () => {
		const key = "test.failed-write";
		vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
			throw new DOMException("quota", "QuotaExceededError");
		});
		const { result } = renderLocalStorageHook(() =>
			useLocalStorage({ storageKey: key, codec: booleanCodec(false) }),
		);

		act(() => result.current[1](true));

		expect(result.current[0]).toBe(false);
		expect(localStorage.getItem(key)).toBeNull();
	});

	it("reads the fallback when storage access throws", () => {
		vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
			throw new DOMException("denied", "SecurityError");
		});
		const { result } = renderLocalStorageHook(() =>
			useLocalStorage({
				storageKey: "test.access-denied",
				codec: booleanCodec(true),
			}),
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
			useLocalStorage({ storageKey: key, codec: nullableCodec }),
		);

		act(() => result.current[1](null));

		expect(localStorage.getItem(key)).toBeNull();
		expect(result.current[0]).toBeNull();
	});

	it("keeps unknown stored fields when a functional update spreads the previous value", () => {
		const key = "test.unknown-fields";
		localStorage.setItem(key, JSON.stringify({ enabled: true, future: 1 }));
		const { result } = renderLocalStorageHook(() =>
			useLocalStorage({ storageKey: key, codec: stateCodec }),
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
			useLocalStorage({ storageKey: key, codec: stateCodec }),
		);
		expect(result.current[0]).toEqual({ enabled: false });
	});

	it("returns the same object until the stored value changes", () => {
		const key = "test.stable-object";
		localStorage.setItem(key, JSON.stringify({ enabled: true }));
		const { result, rerender } = renderLocalStorageHook(() =>
			useLocalStorage({ storageKey: key, codec: stateCodec }),
		);
		const firstValue = result.current[0];

		rerender();

		expect(result.current[0]).toBe(firstValue);
	});
});

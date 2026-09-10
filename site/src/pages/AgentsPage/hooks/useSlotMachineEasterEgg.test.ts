import { renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
	appendToKeyBuffer,
	isPrintableKeyEvent,
	matchesKeySequence,
	slotMachineKeySequence,
	slotMachineStorageKey,
	useSlotMachineEasterEggListener,
} from "./useSlotMachineEasterEgg";

vi.mock("sonner", () => ({ toast: vi.fn() }));

const keyEvent = (
	key: string,
	modifiers: Partial<{
		ctrlKey: boolean;
		metaKey: boolean;
		altKey: boolean;
		isComposing: boolean;
	}> = {},
) => ({ key, ctrlKey: false, metaKey: false, altKey: false, ...modifiers });

describe("isPrintableKeyEvent", () => {
	it("accepts single printable characters", () => {
		expect(isPrintableKeyEvent(keyEvent("i"))).toBe(true);
		expect(isPrintableKeyEvent(keyEvent("D"))).toBe(true);
		expect(isPrintableKeyEvent(keyEvent(" "))).toBe(true);
	});

	it("rejects named keys", () => {
		expect(isPrintableKeyEvent(keyEvent("Shift"))).toBe(false);
		expect(isPrintableKeyEvent(keyEvent("Enter"))).toBe(false);
		expect(isPrintableKeyEvent(keyEvent("ArrowLeft"))).toBe(false);
	});

	it("rejects ctrl, meta, and alt chords", () => {
		expect(isPrintableKeyEvent(keyEvent("d", { ctrlKey: true }))).toBe(false);
		expect(isPrintableKeyEvent(keyEvent("d", { metaKey: true }))).toBe(false);
		expect(isPrintableKeyEvent(keyEvent("d", { altKey: true }))).toBe(false);
	});

	it("rejects keystrokes that are part of an IME composition", () => {
		expect(isPrintableKeyEvent(keyEvent("d", { isComposing: true }))).toBe(
			false,
		);
	});
});

describe("appendToKeyBuffer", () => {
	it("lowercases and keeps only the trailing characters", () => {
		let buffer = "";
		for (const key of ["x", "I", "d", "D", "q", "d"]) {
			buffer = appendToKeyBuffer(buffer, key, slotMachineKeySequence.length);
		}
		expect(buffer).toBe("iddqd");
	});
});

describe("matchesKeySequence", () => {
	it("matches when the buffer ends with the sequence", () => {
		expect(matchesKeySequence("iddqd", slotMachineKeySequence)).toBe(true);
	});

	it("does not match partial or interrupted sequences", () => {
		expect(matchesKeySequence("iddq", slotMachineKeySequence)).toBe(false);
		expect(matchesKeySequence("idd qd", slotMachineKeySequence)).toBe(false);
		expect(matchesKeySequence("iddqx", slotMachineKeySequence)).toBe(false);
	});

	it("never matches an empty sequence", () => {
		expect(matchesKeySequence("anything", "")).toBe(false);
	});
});

describe("useSlotMachineEasterEggListener", () => {
	afterEach(() => {
		localStorage.removeItem(slotMachineStorageKey);
	});

	const type = (text: string) => {
		for (const key of text) {
			document.dispatchEvent(
				new KeyboardEvent("keydown", { key, bubbles: true }),
			);
		}
	};

	it("persists the toggle on each full sequence", () => {
		renderHook(() => useSlotMachineEasterEggListener());

		type("hello iddqd");
		expect(localStorage.getItem(slotMachineStorageKey)).toBe("true");

		type("iddqd");
		expect(localStorage.getItem(slotMachineStorageKey)).toBe("false");
	});

	it("stops listening after unmount", () => {
		const { unmount } = renderHook(() => useSlotMachineEasterEggListener());
		unmount();

		type("iddqd");
		expect(localStorage.getItem(slotMachineStorageKey)).toBeNull();
	});
});

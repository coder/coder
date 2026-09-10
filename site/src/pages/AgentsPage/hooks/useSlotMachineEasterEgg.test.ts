import { describe, expect, it } from "vitest";
import {
	appendToKeyBuffer,
	isPrintableKeyEvent,
	matchesKeySequence,
	slotMachineKeySequence,
} from "./useSlotMachineEasterEgg";

const keyEvent = (
	key: string,
	modifiers: Partial<{
		ctrlKey: boolean;
		metaKey: boolean;
		altKey: boolean;
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

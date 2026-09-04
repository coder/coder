import { describe, expect, it } from "vitest";
import { isLetterKey } from "./keyboardShortcuts";

const keydown = (init: KeyboardEventInit) => new KeyboardEvent("keydown", init);

describe("isLetterKey", () => {
	it("matches by key on Latin layouts, including shifted letters", () => {
		expect(isLetterKey(keydown({ key: "j", code: "KeyJ" }), "j")).toBe(true);
		expect(isLetterKey(keydown({ key: "J", code: "KeyJ" }), "j")).toBe(true);
		expect(isLetterKey(keydown({ key: "k", code: "KeyK" }), "j")).toBe(false);
	});

	it("prefers the printed letter on remapped Latin layouts", () => {
		// Dvorak: the key at the QWERTY "J" position prints "h".
		expect(isLetterKey(keydown({ key: "h", code: "KeyJ" }), "j")).toBe(false);
		expect(isLetterKey(keydown({ key: "j", code: "KeyC" }), "j")).toBe(true);
	});

	it("falls back to the physical key on non-Latin layouts", () => {
		expect(isLetterKey(keydown({ key: "о", code: "KeyJ" }), "j")).toBe(true);
		expect(isLetterKey(keydown({ key: "ξ", code: "KeyJ" }), "j")).toBe(true);
		expect(isLetterKey(keydown({ key: "о", code: "KeyK" }), "j")).toBe(false);
	});
});

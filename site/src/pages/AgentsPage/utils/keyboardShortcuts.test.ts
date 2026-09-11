import { describe, expect, it, vi } from "vitest";
import { isMac, isWindows } from "#/utils/platform";
import {
	getDefaultVimModifier,
	getModifierKeycap,
	getModifierLabel,
	isLetterKey,
	isModifierPressed,
	isSlashKey,
} from "./keyboardShortcuts";

vi.mock("#/utils/platform", async (importOriginal) => ({
	...(await importOriginal<typeof import("#/utils/platform")>()),
	isMac: vi.fn(),
	isWindows: vi.fn(),
	getOSKey: () => "OSKEY",
}));

const isMacMock = vi.mocked(isMac);
const isWindowsMock = vi.mocked(isWindows);

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

describe("isSlashKey", () => {
	it("matches by key on Latin layouts, including shifted slashes", () => {
		expect(isSlashKey(keydown({ key: "/", code: "Slash" }))).toBe(true);
		expect(
			isSlashKey(keydown({ key: "/", code: "Digit7", shiftKey: true })),
		).toBe(true);
		expect(
			isSlashKey(keydown({ key: "?", code: "Slash", shiftKey: true })),
		).toBe(false);
	});

	it("falls back to the physical key for non-ASCII characters", () => {
		// Option+/ on a macOS US layout reports "÷".
		expect(isSlashKey(keydown({ key: "÷", code: "Slash" }))).toBe(true);
		expect(isSlashKey(keydown({ key: ".", code: "Slash" }))).toBe(false);
	});
});

describe("isModifierPressed", () => {
	it.each([
		["ctrl", { ctrlKey: true }],
		["alt", { altKey: true }],
		["meta", { metaKey: true }],
	] as const)("matches %s alone or with Shift", (modifier, init) => {
		expect(isModifierPressed(keydown(init), modifier)).toBe(true);
		expect(
			isModifierPressed(keydown({ ...init, shiftKey: true }), modifier),
		).toBe(true);
		expect(isModifierPressed(keydown({}), modifier)).toBe(false);
	});

	it("rejects chords with a different or additional modifier", () => {
		expect(isModifierPressed(keydown({ metaKey: true }), "ctrl")).toBe(false);
		expect(isModifierPressed(keydown({ ctrlKey: true }), "alt")).toBe(false);
		expect(isModifierPressed(keydown({ altKey: true }), "meta")).toBe(false);
		// AltGr on Windows reports Ctrl+Alt.
		expect(
			isModifierPressed(keydown({ ctrlKey: true, altKey: true }), "ctrl"),
		).toBe(false);
		expect(
			isModifierPressed(keydown({ ctrlKey: true, altKey: true }), "alt"),
		).toBe(false);
		expect(
			isModifierPressed(keydown({ metaKey: true, ctrlKey: true }), "meta"),
		).toBe(false);
	});
});

describe("getModifierLabel", () => {
	it("uses Cmd and Option on macOS", () => {
		isMacMock.mockReturnValue(true);
		isWindowsMock.mockReturnValue(false);
		expect(getModifierLabel("ctrl")).toBe("Ctrl");
		expect(getModifierLabel("alt")).toBe("Option");
		expect(getModifierLabel("meta")).toBe("Cmd");
	});

	it("uses Win on Windows", () => {
		isMacMock.mockReturnValue(false);
		isWindowsMock.mockReturnValue(true);
		expect(getModifierLabel("alt")).toBe("Alt");
		expect(getModifierLabel("meta")).toBe("Win");
	});

	it("uses Alt and Super elsewhere", () => {
		isMacMock.mockReturnValue(false);
		isWindowsMock.mockReturnValue(false);
		expect(getModifierLabel("ctrl")).toBe("Ctrl");
		expect(getModifierLabel("alt")).toBe("Alt");
		expect(getModifierLabel("meta")).toBe("Super");
	});
});

describe("getModifierKeycap", () => {
	it("uses the OS key glyph for Cmd on macOS and labels otherwise", () => {
		isMacMock.mockReturnValue(true);
		isWindowsMock.mockReturnValue(false);
		expect(getModifierKeycap("meta")).toBe("OSKEY");
		expect(getModifierKeycap("alt")).toBe("Option");
		expect(getModifierKeycap("ctrl")).toBe("Ctrl");

		isMacMock.mockReturnValue(false);
		expect(getModifierKeycap("meta")).toBe("Super");
	});
});

describe("getDefaultVimModifier", () => {
	it("is Cmd on macOS and Ctrl elsewhere", () => {
		isMacMock.mockReturnValue(true);
		expect(getDefaultVimModifier()).toBe("meta");
		isMacMock.mockReturnValue(false);
		expect(getDefaultVimModifier()).toBe("ctrl");
	});
});

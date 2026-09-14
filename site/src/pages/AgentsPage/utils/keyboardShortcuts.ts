import { getOSKey, isMac, isWindows } from "#/utils/platform";

/**
 * Modifier key held for every vim navigation shortcut. `meta` is
 * Cmd on macOS, Win on Windows, and Super elsewhere.
 */
export type VimModifier = "ctrl" | "alt" | "meta";

export const VIM_MODIFIERS: readonly VimModifier[] = ["ctrl", "alt", "meta"];

export const isVimModifier = (value: string | null): value is VimModifier =>
	value !== null && (VIM_MODIFIERS as readonly string[]).includes(value);

/**
 * Modifier used when none has been stored: Cmd on macOS, Ctrl elsewhere.
 */
export function getDefaultVimModifier(): VimModifier {
	return isMac() ? "meta" : "ctrl";
}

/**
 * Reports whether a keydown event is for the given Latin letter,
 * regardless of keyboard layout.
 *
 * `event.key` reflects the active layout and is used when it is an
 * ASCII letter, which also covers remapped Latin layouts such as
 * Dvorak. When it is not (Cyrillic, Greek, Hebrew, and similar
 * layouts report the native character; Option+letter on macOS
 * reports a symbol such as "∆" or "Dead"), the physical `event.code`
 * ("KeyJ") is used instead. `event.code` names the QWERTY position,
 * so on those layouts the chord is matched by physical position.
 */
export const isLetterKey = (event: KeyboardEvent, letter: string): boolean => {
	const lower = letter.toLowerCase();
	const key = event.key.toLowerCase();
	if (/^[a-z]$/.test(key)) {
		return key === lower;
	}
	return event.code === `Key${lower.toUpperCase()}`;
};

/**
 * Reports whether a keydown event is for the slash key. `event.key`
 * is used when it is a printable ASCII character so that layouts
 * where "/" is a shifted key still match; otherwise the physical
 * `event.code` ("Slash") is used, which covers Option+/ on macOS
 * ("÷") and layouts whose slash position emits a non-ASCII character.
 */
export const isSlashKey = (event: KeyboardEvent): boolean => {
	if (/^[\x20-\x7e]$/.test(event.key)) {
		return event.key === "/";
	}
	return event.code === "Slash";
};

/**
 * Reports whether exactly the given modifier is held. The other two
 * modifiers must be released so that chords such as AltGr (Ctrl+Alt
 * on Windows) or Ctrl+Cmd do not match. Shift is not checked.
 *
 * Known conflicts that `preventDefault` cannot suppress: with `meta`,
 * Windows reserves Win+J, Win+K, Win+E, and Win+/ for the OS, and
 * most Linux desktops bind Super+letter to window management; with
 * `alt`, Alt+Shift switches the input language on Windows and on
 * Linux desktops configured with `grp:alt_shift_toggle`, and Firefox
 * on Windows and Linux may open a menu for Alt+Shift+letter chords.
 */
export const isModifierPressed = (
	event: KeyboardEvent,
	modifier: VimModifier,
): boolean => {
	switch (modifier) {
		case "ctrl":
			return event.ctrlKey && !event.altKey && !event.metaKey;
		case "alt":
			return event.altKey && !event.ctrlKey && !event.metaKey;
		case "meta":
			return event.metaKey && !event.ctrlKey && !event.altKey;
	}
};

/**
 * Human-readable name for a modifier on the current platform.
 */
export const getModifierLabel = (modifier: VimModifier): string => {
	switch (modifier) {
		case "ctrl":
			return "Ctrl";
		case "alt":
			return isMac() ? "Option" : "Alt";
		case "meta":
			if (isMac()) {
				return "Cmd";
			}
			return isWindows() ? "Win" : "Super";
	}
};

/**
 * Keycap text for a modifier in a `Kbd` hint. Cmd on macOS uses the
 * same glyph as `getOSKey`; every other modifier uses its text label.
 */
export const getModifierKeycap = (modifier: VimModifier): string => {
	if (modifier === "meta" && isMac()) {
		return getOSKey();
	}
	return getModifierLabel(modifier);
};

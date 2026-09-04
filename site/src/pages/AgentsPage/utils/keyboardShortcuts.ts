/**
 * Reports whether a keydown event is for the given Latin letter,
 * regardless of keyboard layout.
 *
 * `event.key` reflects the active layout and is used when it is an
 * ASCII letter, which also covers remapped Latin layouts such as
 * Dvorak. On Cyrillic, Greek, Hebrew, and similar layouts it reports
 * the native character instead, so the physical `event.code`
 * ("KeyJ") is used for those.
 */
export const isLetterKey = (event: KeyboardEvent, letter: string): boolean => {
	const lower = letter.toLowerCase();
	const key = event.key.toLowerCase();
	if (/^[a-z]$/.test(key)) {
		return key === lower;
	}
	return event.code === `Key${lower.toUpperCase()}`;
};

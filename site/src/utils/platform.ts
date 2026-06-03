/**
 * Returns true if the current platform is macOS.
 */
export function isMac(): boolean {
	return Boolean(navigator.platform.match("Mac"));
}

/**
 * Returns true if the current platform is Windows.
 */
export function isWindows(): boolean {
	return /Win/i.test(navigator.platform);
}

/**
 * Returns the platform-appropriate modifier key label: ⌘ on macOS,
 * Ctrl on everything else.
 */
export function getOSKey(): string {
	return isMac() ? "⌘" : "Ctrl";
}

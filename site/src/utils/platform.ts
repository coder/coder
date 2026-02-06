/**
 * Returns true if the current platform is macOS.
 */
function isMac(): boolean {
	return !!navigator.platform.match("Mac");
}

/**
 * Returns the platform-appropriate modifier key label: ⌘ on macOS,
 * Ctrl on everything else.
 */
export function getOSKey(): string {
	return isMac() ? "⌘" : "Ctrl";
}

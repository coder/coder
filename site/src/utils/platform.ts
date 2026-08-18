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
	return navigator.platform.startsWith("Win");
}

/**
 * Returns true if Coder Desktop is available for the current platform.
 * Coder Desktop currently ships for macOS and Windows only, so we hide the
 * install affordance everywhere else (e.g. Linux).
 */
export function supportsCoderDesktop(): boolean {
	return isMac() || isWindows();
}

/**
 * Returns the platform-appropriate modifier key label: ⌘ on macOS,
 * Ctrl on everything else.
 */
export function getOSKey(): string {
	return isMac() ? "⌘" : "Ctrl";
}

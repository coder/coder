/**
 * Returns true if the current platform is macOS.
 *
 * Note: iPadOS 13+ also reports `navigator.platform === "MacIntel"`. Callers
 * that need to distinguish a real Mac from an iPad should additionally check
 * `navigator.maxTouchPoints` (see `supportsCoderDesktop`).
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
 * Returns true if the current platform is Linux.
 */
export function isLinux(): boolean {
	return navigator.platform.startsWith("Linux");
}

/**
 * Returns true if Coder Desktop is available for the current platform.
 * Coder Desktop ships for macOS, Windows, and Linux (the Linux client is
 * experimental), so we only hide the install affordance on platforms it does
 * not target (e.g. iPadOS, ChromeOS). iPadOS masquerades as macOS via
 * `navigator.platform`, so it is excluded using the touchscreen tell.
 */
export function supportsCoderDesktop(): boolean {
	const isIpadOS = isMac() && navigator.maxTouchPoints > 1;
	return (isMac() || isWindows() || isLinux()) && !isIpadOS;
}

/**
 * Returns the platform-appropriate modifier key label: ⌘ on macOS,
 * Ctrl on everything else.
 */
export function getOSKey(): string {
	return isMac() ? "⌘" : "Ctrl";
}

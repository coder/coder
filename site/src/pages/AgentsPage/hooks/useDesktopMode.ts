import { useCallback, useEffect, useRef, useState } from "react";

/**
 * Try to lock the screen orientation. This only works in certain
 * contexts (PWA standalone mode on Android, fullscreen) and throws
 * or rejects everywhere else. All failures are silently ignored.
 */
async function tryOrientationLock(
	orientation: OrientationLockType,
): Promise<boolean> {
	try {
		await screen.orientation.lock(orientation);
		return true;
	} catch {
		// Expected to fail on iOS Safari, non-fullscreen browsers,
		// and desktop browsers. This is not an error condition.
		return false;
	}
}

function tryOrientationUnlock(): void {
	try {
		screen.orientation.unlock();
	} catch {
		// Same as above — expected to fail in most contexts.
	}
}

/**
 * Detect whether the device is likely a mobile/touch device. We use
 * the coarse pointer media query rather than user-agent sniffing
 * because it reflects actual input capability.
 */
function isTouchDevice(): boolean {
	if (typeof window === "undefined") return false;
	return window.matchMedia("(pointer: coarse)").matches;
}

export interface UseDesktopModeResult {
	/** Whether landscape mode is currently active. */
	isLandscape: boolean;
	/** Enter landscape mode. */
	enterLandscape: () => void;
	/** Exit landscape mode and re-lock portrait. */
	exitLandscape: () => void;
	/**
	 * Whether the device supports the feature at all. False on
	 * desktop browsers and non-touch devices where the button
	 * should be hidden.
	 */
	isSupported: boolean;
}

/**
 * Manages landscape orientation for the Desktop panel on mobile.
 * Follows the YouTube pattern: the app is normally portrait-locked,
 * and this hook provides an escape hatch to landscape specifically
 * for the desktop VNC view.
 *
 * When the caller signals entry (enterLandscape), the hook attempts
 * screen.orientation.lock('landscape'). On exit it re-locks portrait.
 * Unmounting automatically exits landscape.
 */
export function useDesktopMode(): UseDesktopModeResult {
	const [isSupported] = useState(() => isTouchDevice());
	const [isLandscape, setIsLandscape] = useState(false);
	const cleanupRef = useRef(false);

	const enterLandscape = useCallback(() => {
		setIsLandscape(true);
	}, []);

	const exitLandscape = useCallback(() => {
		setIsLandscape(false);
	}, []);

	useEffect(() => {
		if (!isSupported) return;

		if (isLandscape) {
			void tryOrientationLock("landscape");
			cleanupRef.current = true;
		} else if (cleanupRef.current) {
			// Re-lock portrait. If that fails (e.g. iOS), just
			// release whatever lock is active.
			void tryOrientationLock("portrait-primary").then((locked) => {
				if (!locked) {
					tryOrientationUnlock();
				}
			});
			cleanupRef.current = false;
		}

		return () => {
			// Restore portrait on unmount so navigating away from
			// the Desktop panel returns the device to its normal
			// orientation.
			if (cleanupRef.current) {
				void tryOrientationLock("portrait-primary").then((locked) => {
					if (!locked) {
						tryOrientationUnlock();
					}
				});
				cleanupRef.current = false;
			}
		};
	}, [isLandscape, isSupported]);

	return { isLandscape, enterLandscape, exitLandscape, isSupported };
}

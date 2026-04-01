import { useCallback, useEffect, useRef } from "react";

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
 * Manages landscape orientation for the Desktop panel fullscreen
 * mode on mobile. The caller tells the hook whether landscape
 * should be active; the hook handles the Screen Orientation API
 * and cleans up on unmount.
 *
 * Portrait is restored automatically when `isLandscape` flips
 * back to false or when the component unmounts.
 */
export function useDesktopLandscape(isLandscape: boolean): void {
	const wasLandscapeRef = useRef(false);

	const restorePortrait = useCallback(() => {
		void tryOrientationLock("portrait-primary").then((locked) => {
			if (!locked) {
				tryOrientationUnlock();
			}
		});
	}, []);

	useEffect(() => {
		if (isLandscape) {
			void tryOrientationLock("landscape");
			wasLandscapeRef.current = true;
		} else if (wasLandscapeRef.current) {
			restorePortrait();
			wasLandscapeRef.current = false;
		}

		return () => {
			// Restore portrait on unmount so navigating away
			// returns the device to its normal orientation.
			if (wasLandscapeRef.current) {
				restorePortrait();
				wasLandscapeRef.current = false;
			}
		};
	}, [isLandscape, restorePortrait]);
}

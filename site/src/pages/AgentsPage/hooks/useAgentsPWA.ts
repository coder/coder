import { useEffect } from "react";

/**
 * Injects PWA-related <head> tags while the Agents page is mounted
 * (manifest, apple-touch-icon, mobile-web-app metas) and tweaks the
 * viewport to prevent zooming. On mobile it also locks orientation
 * to portrait; the Desktop panel can override this at runtime.
 * Everything is cleaned up on unmount.
 */
export function useAgentsPWA() {
	useEffect(() => {
		// -- Injected elements --------------------------------------------------
		const manifest = document.createElement("link");
		manifest.rel = "manifest";
		manifest.href = "/manifest.json";

		const appleTouchIcon = document.createElement("link");
		appleTouchIcon.rel = "apple-touch-icon";
		appleTouchIcon.href = "/apple-touch-icon.png";

		const mobileWebAppCapable = document.createElement("meta");
		mobileWebAppCapable.name = "mobile-web-app-capable";
		mobileWebAppCapable.content = "yes";

		const appleMobileWebAppCapable = document.createElement("meta");
		appleMobileWebAppCapable.name = "apple-mobile-web-app-capable";
		appleMobileWebAppCapable.content = "yes";

		const appleMobileWebAppStatusBarStyle = document.createElement("meta");
		appleMobileWebAppStatusBarStyle.name =
			"apple-mobile-web-app-status-bar-style";
		appleMobileWebAppStatusBarStyle.content = "black-translucent";

		const appleMobileWebAppTitle = document.createElement("meta");
		appleMobileWebAppTitle.name = "apple-mobile-web-app-title";
		appleMobileWebAppTitle.content = "Agents";

		const injected = [
			manifest,
			appleTouchIcon,
			mobileWebAppCapable,
			appleMobileWebAppCapable,
			appleMobileWebAppStatusBarStyle,
			appleMobileWebAppTitle,
		];

		for (const el of injected) {
			document.head.appendChild(el);
		}

		// -- Viewport override --------------------------------------------------
		const viewport = document.querySelector<HTMLMetaElement>(
			'meta[name="viewport"]',
		);
		const prevViewportContent = viewport?.content ?? "";

		if (viewport) {
			viewport.content =
				"width=device-width, initial-scale=1, maximum-scale=1, user-scalable=no";
		}

		// -- Orientation lock ---------------------------------------------------
		// Lock portrait by default for a native-app feel on mobile.
		// The useDesktopMode hook overrides this when the user opens
		// the Desktop panel in landscape. The lock is best-effort —
		// it only works in PWA standalone mode on Android and is
		// silently ignored everywhere else.
		let orientationLocked = false;
		try {
			screen.orientation
				.lock("portrait-primary")
				.then(() => {
					orientationLocked = true;
				})
				.catch(() => {});
		} catch {
			// screen.orientation.lock may not exist at all.
		}

		// -- Cleanup ------------------------------------------------------------
		return () => {
			for (const el of injected) {
				el.remove();
			}
			if (viewport) {
				viewport.content = prevViewportContent;
			}
			if (orientationLocked) {
				try {
					screen.orientation.unlock();
				} catch {
					// Ignored.
				}
			}
		};
	}, []);
}

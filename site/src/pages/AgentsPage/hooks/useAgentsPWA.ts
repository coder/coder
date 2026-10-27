import { useEffect } from "react";

// Static files are served with immutable caching (ShouldCacheFile in
// site/site.go), so a browser that has fetched a URL never refetches it.
// Bump this when manifest.json or apple-touch-icon.png changes, and bump the
// icon URLs in manifest.json when an icon changes.
const pwaAssetVersion = "4";

/**
 * Injects PWA-related <head> tags while the Agents page is mounted
 * (manifest, apple-touch-icon, mobile-web-app metas) and tweaks the
 * viewport to prevent zooming. Everything is cleaned up on unmount.
 */
export function useAgentsPWA() {
	useEffect(() => {
		// -- Injected elements --------------------------------------------------
		const manifest = document.createElement("link");
		manifest.rel = "manifest";
		manifest.href = `/manifest.json?v=${pwaAssetVersion}`;

		const appleTouchIcon = document.createElement("link");
		appleTouchIcon.rel = "apple-touch-icon";
		appleTouchIcon.href = `/apple-touch-icon.png?v=${pwaAssetVersion}`;

		const mobileWebAppCapable = document.createElement("meta");
		mobileWebAppCapable.name = "mobile-web-app-capable";
		mobileWebAppCapable.content = "yes";

		const appleMobileWebAppCapable = document.createElement("meta");
		appleMobileWebAppCapable.name = "apple-mobile-web-app-capable";
		appleMobileWebAppCapable.content = "yes";

		const appleMobileWebAppTitle = document.createElement("meta");
		appleMobileWebAppTitle.name = "apple-mobile-web-app-title";
		appleMobileWebAppTitle.content = "Agents";

		const injected = [
			manifest,
			appleTouchIcon,
			mobileWebAppCapable,
			appleMobileWebAppCapable,
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

		// -- Cleanup ------------------------------------------------------------
		return () => {
			for (const el of injected) {
				el.remove();
			}
			if (viewport) {
				viewport.content = prevViewportContent;
			}
		};
	}, []);
}

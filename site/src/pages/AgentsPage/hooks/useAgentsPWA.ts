import { useEffect } from "react";

/**
 * Points the document's manifest at the Agents app and injects PWA-related
 * <head> tags while the Agents page is mounted (apple-touch-icon,
 * mobile-web-app metas), and tweaks the viewport to prevent zooming.
 * Everything is restored on unmount.
 */
export function useAgentsPWA() {
	useEffect(() => {
		// -- Manifest override --------------------------------------------------
		// Browsers only honor the first manifest link, so swap its href rather
		// than adding a second one.
		const manifest = document.querySelector<HTMLLinkElement>(
			'link[rel="manifest"]',
		);
		const prevManifestHref = manifest?.getAttribute("href") ?? "";

		if (manifest) {
			manifest.setAttribute("href", "/manifest.json");
		}

		// -- Injected elements --------------------------------------------------
		const appleTouchIcon = document.createElement("link");
		appleTouchIcon.rel = "apple-touch-icon";
		appleTouchIcon.href = "/apple-touch-icon.png";

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
			if (manifest) {
				manifest.setAttribute("href", prevManifestHref);
			}
			if (viewport) {
				viewport.content = prevViewportContent;
			}
		};
	}, []);
}

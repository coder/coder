import { annotatorQueryParam } from "./protocol";

/**
 * Keeps the overlay across full page loads inside the preview. The proxy
 * only injects the script on requests that carry the marker parameter,
 * so a plain link click would land on a page without it. Same-origin link
 * clicks are rewritten at the last moment to carry the marker; the proxy
 * strips it again before the app sees the request.
 *
 * Only plain left clicks on same-origin, same-tab, non-download links are
 * touched. Hash-only links, modified clicks (new tab) and anything the
 * page already cancelled are left alone.
 */
export function carryMarkerAcrossNavigation(
	doc: Document,
	win: Window,
): () => void {
	const onClick = (event: MouseEvent) => {
		if (
			event.defaultPrevented ||
			event.button !== 0 ||
			event.metaKey ||
			event.ctrlKey ||
			event.shiftKey ||
			event.altKey
		) {
			return;
		}
		const anchor = event
			.composedPath()
			.find(
				(node): node is HTMLAnchorElement => node instanceof HTMLAnchorElement,
			);
		if (!anchor || anchor.hasAttribute("download")) {
			return;
		}
		const target = anchor.getAttribute("target");
		if (target && target !== "_self") {
			return;
		}
		const marked = withMarker(anchor.href, win.location);
		if (marked) {
			anchor.href = marked;
		}
	};
	// Bubble phase, after the page's own handlers: a router that called
	// preventDefault to navigate client-side keeps the overlay anyway.
	doc.addEventListener("click", onClick);
	return () => doc.removeEventListener("click", onClick);
}

/**
 * Returns `href` with the marker parameter added when it is a same-origin
 * navigation to another document, or undefined when it should be left as
 * it is.
 */
export function withMarker(
	href: string,
	location: { href: string; origin: string; pathname: string; search: string },
): string | undefined {
	let url: URL;
	try {
		url = new URL(href, location.href);
	} catch {
		return undefined;
	}
	if (url.origin !== location.origin) {
		return undefined;
	}
	if (
		url.hash &&
		url.pathname === location.pathname &&
		url.search === location.search
	) {
		// Same-document fragment navigation; no request is made.
		return undefined;
	}
	if (url.searchParams.get(annotatorQueryParam) === "1") {
		return undefined;
	}
	url.searchParams.set(annotatorQueryParam, "1");
	return url.toString();
}

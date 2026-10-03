/**
 * Classifies image sources rendered from untrusted content (for
 * example LLM-generated chat markdown). Fetching an external source
 * discloses the viewer's IP address to that host (Cure53 CDM-02-006),
 * so callers must not render one without explicit viewer consent.
 */

/**
 * Returns true when loading `src` in an <img> would issue a request
 * to a host other than the current deployment. Unparsable sources are
 * treated as external so the failure mode is "blocked", never
 * "leaked".
 */
export const isExternalImageSource = (src: string): boolean => {
	// Browsers treat backslashes in http(s) URLs as slashes, so
	// "/\evil.com" navigates to "//evil.com". Treat any backslash as
	// external rather than trying to mirror WHATWG parsing quirks.
	if (src.includes("\\")) {
		return true;
	}
	let parsed: URL;
	try {
		parsed = new URL(src, location.origin);
	} catch {
		return true;
	}
	switch (parsed.protocol) {
		case "data:":
		case "blob:":
			return false;
		case "http:":
		case "https:":
			return parsed.origin !== location.origin;
		default:
			// javascript:, file:, ftp:, and anything else is never a
			// safe image source.
			return true;
	}
};

/** Hostname shown in the consent placeholder, if determinable. */
export const externalImageHost = (src: string): string | undefined => {
	try {
		const host = new URL(src, location.origin).hostname;
		return host === "" ? undefined : host;
	} catch {
		return undefined;
	}
};

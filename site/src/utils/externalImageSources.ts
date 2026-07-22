/**
 * Classifies image sources rendered from untrusted content (for
 * example LLM-generated chat markdown). Sources that would cause the
 * viewer's browser to contact a third-party host disclose the
 * viewer's IP address to that host (Cure53 CDM-02-006), so callers
 * must not render them without explicit viewer consent.
 */

/**
 * Returns true when loading `src` in an <img> would issue a request
 * to a host other than the current deployment. Same-origin and
 * relative paths, `data:`, and `blob:` sources are considered safe
 * because they never contact a third-party host. Unparsable sources
 * are treated as external so the failure mode is "blocked", never
 * "leaked".
 */
export const isExternalImageSource = (src: string | undefined): boolean => {
	if (!src) {
		return false;
	}
	const trimmed = src.trim();
	if (trimmed === "") {
		return false;
	}
	// Browsers treat backslashes in http(s) URLs as slashes, so
	// "/\evil.com" navigates to "//evil.com". Treat any backslash as
	// external rather than trying to mirror WHATWG parsing quirks.
	if (trimmed.includes("\\")) {
		return true;
	}
	let parsed: URL;
	try {
		parsed = new URL(trimmed, location.origin);
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

/**
 * Returns true when `value` is an acceptable icon reference: empty or
 * a deployment-relative path such as "/icon/aws.svg". Mirrors the
 * server-side rule (codersdk.IconURLValid) so forms can reject
 * external icon URLs before submitting; the server remains
 * authoritative.
 */
export const isDeploymentIconPath = (value: string): boolean => {
	if (value === "") {
		return true;
	}
	if (
		value.includes("\\") ||
		!value.startsWith("/") ||
		value.startsWith("//")
	) {
		return false;
	}
	try {
		return new URL(value, location.origin).origin === location.origin;
	} catch {
		return false;
	}
};

/**
 * Returns the hostname rendered in the consent placeholder for an
 * external image, or undefined when it cannot be determined.
 */
export const externalImageHost = (src: string): string | undefined => {
	try {
		const host = new URL(src.trim(), location.origin).hostname;
		return host === "" ? undefined : host;
	} catch {
		return undefined;
	}
};

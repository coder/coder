import type { DLPPolicy } from "#/api/typesGenerated";

/**
 * Returns true when the URL has `?dlp_bypass=1` (or any value). This is a
 * prototype-only escape hatch that lets a user click an element the frontend
 * would otherwise render disabled, so they can confirm the backend gate fires.
 */
export function isDLPBypassed(): boolean {
	if (typeof window === "undefined") {
		return false;
	}
	return new URLSearchParams(window.location.search).has("dlp_bypass");
}

type DLPField =
	| "ssh_access"
	| "web_terminal_access"
	| "port_forwarding_access"
	| "desktop_access";

/**
 * Returns the user-facing tooltip reason for a field-gated DLP denial, or
 * null when the field is allowed (or no policy applies).
 */
export function dlpDenialReason(
	dlp: DLPPolicy | null | undefined,
	field: DLPField,
): string | null {
	if (!dlp) {
		return null;
	}
	if (dlp[field]) {
		return null;
	}
	return `Blocked by DLP policy '${dlp.name}' (${field}).`;
}

/**
 * Returns the user-facing tooltip reason for an app slug not appearing in the
 * policy's allowed_applications list, or null when the slug is allowed (or no
 * policy applies).
 */
export function dlpAppDenialReason(
	dlp: DLPPolicy | null | undefined,
	slug: string,
): string | null {
	if (!dlp) {
		return null;
	}
	if (dlp.allowed_applications.includes(slug)) {
		return null;
	}
	return `App '${slug}' is not in DLP policy '${dlp.name}' allowed_applications list.`;
}

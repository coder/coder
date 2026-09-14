/**
 * Creates a url containing a page to navigate to now, and embedding another
 * URL in the query string so you can return to it later.
 * @param navigateTo page to navigate to now (by default, /login)
 * @param returnTo page to redirect to later (for instance, after logging in)
 * @returns URL containing a redirect query parameter
 */
export const embedRedirect = (
	returnTo: string,
	navigateTo = "/login",
): string => `${navigateTo}?redirect=${encodeURIComponent(returnTo)}`;

/**
 * Retrieves a url from the query string of the current URL
 * @param search the query string in the current URL
 * @returns the URL to redirect to
 */
export const retrieveRedirect = (search: string): string => {
	const defaultRedirect = "/";
	const searchParams = new URLSearchParams(search);
	const redirect = searchParams.get("redirect");
	return redirect ? redirect : defaultRedirect;
};

/**
 * Ensures the redirect is not an open redirect, aka it's relative.
 *
 * A parsed URL's pathname can itself start with "//" (via percent-encoded
 * slashes, backslashes, or dot-segment normalization), and a string starting
 * with "//" is a protocol-relative URL when assigned to `location.href`.
 * Building a path is therefore not enough; the candidate is re-parsed
 * against our own origin and rejected if it would resolve anywhere else.
 * See Cure53 CDM-02-001 (coder/security-disclosures#164).
 */
export const sanitizeRedirect = (redirectTo: string): string => {
	const fallbackRedirect = "/";
	try {
		const url = new URL(redirectTo, location.origin);
		const candidate = url.pathname + url.search;
		const resolved = new URL(candidate, location.origin);
		if (resolved.origin !== location.origin) {
			return fallbackRedirect;
		}
		return candidate;
	} catch {
		return fallbackRedirect;
	}
};

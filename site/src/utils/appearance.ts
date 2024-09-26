export const getApplicationName = (): string => {
	const c = document.head
		.querySelector("meta[name=application-name]")
		?.getAttribute("content");
	// Fallback to "Coder" if the application name is not available for some reason.
	// We need to check if the content does not look like `{{ .ApplicationName }}`
	// as it means that Coder is running in development mode.
	return c && !c.startsWith("{{ .") ? c : "Coder";
};

export const getLogoURL = (): string => {
	const c = document.head
		.querySelector("meta[property=logo-url]")
		?.getAttribute("content");
	return c && !c.startsWith("{{ .") ? c : "";
};

/**
 * Exposes an easy way to determine if a given URL is for an emoji hosted on
 * the Coder deployment.
 *
 * Helps when you need to style emojis differently (i.e., not adding rounding to
 * the container so that the emoji doesn't get cut off).
 */
export function isEmojiUrl(url: string | undefined): boolean {
	if (!url) {
		return false;
	}

	return url.startsWith("/emojis/");
}

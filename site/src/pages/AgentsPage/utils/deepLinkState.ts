/**
 * Reads the prompt a `?prompt=` link left in a history entry's state. The
 * prompt moves from the URL into history state on arrival, because the
 * layout's links forward location.search and the next composer must be a
 * plain one. Anything other than a string, including a missing or
 * non-object state, reads as absent.
 */
export const readDeepLinkPrompt = (state: unknown): string | undefined =>
	typeof state === "object" &&
	state !== null &&
	"prompt" in state &&
	typeof state.prompt === "string"
		? state.prompt
		: undefined;

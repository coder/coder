/**
 * Deep link values move from the URL into this entry's history state on
 * arrival, because the layout's links forward location.search and the next
 * composer must be a plain one.
 */
export type DeepLinkState = { debugWorkspaceBuildId?: string; prompt?: string };

/**
 * Reads deep link values from a history entry's state. Returns only the
 * fields that are strings; anything else, including a missing or non-object
 * state, reads as absent.
 */
export const readDeepLinkState = (state: unknown): DeepLinkState => {
	if (typeof state !== "object" || state === null) {
		return {};
	}
	return {
		debugWorkspaceBuildId:
			"debugWorkspaceBuildId" in state &&
			typeof state.debugWorkspaceBuildId === "string"
				? state.debugWorkspaceBuildId
				: undefined,
		prompt:
			"prompt" in state && typeof state.prompt === "string"
				? state.prompt
				: undefined,
	};
};

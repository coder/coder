/**
 * Deep link values move from the URL into this entry's history state on
 * arrival, because the layout's links forward location.search and the next
 * composer must be a plain one.
 */
export type DeepLinkState = { debugWorkspaceBuildId?: string; prompt?: string };

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

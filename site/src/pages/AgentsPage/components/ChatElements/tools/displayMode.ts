import type { AgentDisplayMode } from "#/api/typesGenerated";

export type AgentDisplayState = "collapsed" | "preview" | "expanded";

export const resolveAgentDisplayState = (
	mode: AgentDisplayMode | undefined,
	autoState: AgentDisplayState,
): AgentDisplayState => {
	switch (mode) {
		case undefined:
		case "auto":
			return autoState;
		case "preview":
			return "preview";
		case "always_expanded":
			return "expanded";
		case "always_collapsed":
			return "collapsed";
		default: {
			const _exhaustive: never = mode;
			return _exhaustive;
		}
	}
};

export const isAgentDisplayOpen = (state: AgentDisplayState): boolean => {
	return state !== "collapsed";
};

export const isAgentDisplayFullyExpanded = (
	state: AgentDisplayState,
): boolean => {
	return state === "expanded";
};

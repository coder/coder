import type { AgentDisplayMode } from "#/api/typesGenerated";
import type { ToolCallView } from "./ToolCall";

export type AgentDisplayState = ToolCallView;

export const resolveAgentDisplayState = (
	mode: AgentDisplayMode | undefined,
	autoState: AgentDisplayState,
): AgentDisplayState => {
	switch (mode) {
		case undefined:
		case "auto":
			return autoState;
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

export const isAgentDisplayFullyExpanded = (
	state: AgentDisplayState,
): boolean => {
	return state === "expanded";
};

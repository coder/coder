import { createContext, use } from "react";
import type {
	MCPServerConfig,
	Workspace,
	WorkspaceAgent,
} from "#/api/typesGenerated";
import type { AttachedWorkspaceInfo } from "./AgentComposerBadges";

/** Controlled inputs for independently configurable composer options. */
export type AgentComposerOptionsData = {
	organizationId?: string;

	/**
	 * Shows the current settings without offering changes: the menu trigger
	 * is hidden, the model renders as a static pill, and badges omit their
	 * dismiss actions.
	 */
	isViewOnly?: boolean;

	planning: {
		enabled: boolean;
		onChange: (enabled: boolean) => void;
	};

	mcp?: {
		servers: readonly MCPServerConfig[];
		selectedServerIds: readonly string[];
		/** Absent when the caller does not offer MCP selection changes. */
		onSelectionChange?: (ids: string[]) => void;
		onAuthComplete?: (id: string) => void;
	};

	workspaceSelection?: {
		/** Absent when the caller offers no workspace choices. */
		options?: ReadonlyArray<Pick<Workspace, "id" | "name" | "organization_id">>;
		selectedId?: string | null;
		/** Selects or clears the workspace; absent when changes are not offered. */
		onChange?: (id: string | null) => void;
		isLoading?: boolean;
	};

	linkedWorkspace?: {
		workspace?: Workspace;
		agent?: WorkspaceAgent;
		chatId?: string;
		sshCommand?: string;
		attachedWorkspace?: AttachedWorkspaceInfo;
		folder?: string;
	};
};

export const OptionsContext = createContext<AgentComposerOptionsData | null>(
	null,
);

/** Reads shared tool state inside AgentComposerOptions.Provider. */
export function useAgentComposerOptions() {
	const context = use(OptionsContext);

	if (!context) {
		throw new Error(
			"useAgentComposerOptions must be used inside AgentComposerOptions.Provider",
		);
	}

	return context;
}

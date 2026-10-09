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

	planning: {
		enabled: boolean;
		onChange: (enabled: boolean) => void;
	};

	automations?: {
		enabled: boolean;
		onChange: (enabled: boolean) => void;
	};

	mcp?: {
		servers: readonly MCPServerConfig[];
		selectedServerIds: readonly string[];
		onSelectionChange: (ids: string[]) => void;
		onAuthComplete?: (id: string) => void;
	};

	workspaceSelection?: {
		options: ReadonlyArray<Pick<Workspace, "id" | "name" | "organization_id">>;
		selectedId: string | null;
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

/** Options data plus capabilities the provider derives from it. */
type AgentComposerOptionsValue = AgentComposerOptionsData & {
	/**
	 * Present while a workspace change can start: the caller offers one and
	 * workspace selection is not loading or pending. Composer disablement
	 * does not withdraw it.
	 */
	changeWorkspace?: (id: string | null) => void;
};

export const OptionsContext = createContext<AgentComposerOptionsValue | null>(
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

import { createContext, useContext } from "react";
import type { NewPreviewTab } from "../utils/rightPanelTabs";

type ChatWorkspaceContextValue = {
	workspaceId?: string;
	buildId?: string;
	agentId?: string;
	/** Name of the chat's agent, when the workspace's latest build has it. */
	agentName?: string;
	/**
	 * Opens a preview in the right panel, or says why this viewer cannot.
	 * Unset while the open-preview experiment is off or the chat's workspace
	 * or agent is unavailable.
	 */
	previewAccess?:
		| { open: (preview: NewPreviewTab) => void }
		| { unavailableReason: string };
};

const ChatWorkspaceContext = createContext<ChatWorkspaceContextValue>({});

/**
 * Returns the workspace binding of the current chat. `buildId` and
 * `agentId` can belong to an earlier build than the workspace's latest
 * build.
 */
export const useChatWorkspace = () => useContext(ChatWorkspaceContext);

export { ChatWorkspaceContext };

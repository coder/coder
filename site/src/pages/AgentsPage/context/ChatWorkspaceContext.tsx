import { createContext, useContext } from "react";
import type { NewPreviewTab } from "../utils/rightPanelTabs";

type ChatWorkspaceContextValue = {
	workspaceId?: string;
	buildId?: string;
	agentId?: string;
	/** Name of the chat's agent, when the workspace's latest build has it. */
	agentName?: string;
	/**
	 * Opens a preview in the right panel. Unset while the open-preview
	 * experiment is off.
	 */
	openPreview?: (preview: NewPreviewTab) => void;
};

const ChatWorkspaceContext = createContext<ChatWorkspaceContextValue>({});

/**
 * Returns the workspace binding of the current chat. `buildId` and
 * `agentId` can belong to an earlier build than the workspace's latest
 * build.
 */
export const useChatWorkspace = () => useContext(ChatWorkspaceContext);

export { ChatWorkspaceContext };

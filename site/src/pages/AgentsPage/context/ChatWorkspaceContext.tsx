import { createContext, useContext } from "react";

type ChatWorkspaceContextValue = {
	workspaceId?: string;
	buildId?: string;
	agentId?: string;
};

const ChatWorkspaceContext = createContext<ChatWorkspaceContextValue>({});

/**
 * Returns the workspace binding of the current chat. `buildId` and
 * `agentId` can belong to an earlier build than the workspace's latest
 * build.
 */
export const useChatWorkspace = () => useContext(ChatWorkspaceContext);

export { ChatWorkspaceContext };

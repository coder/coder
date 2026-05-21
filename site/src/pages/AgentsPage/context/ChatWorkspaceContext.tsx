import { createContext, useContext } from "react";

interface ChatWorkspaceContextValue {
	workspaceId?: string;
	buildId?: string;
}

const ChatWorkspaceContext = createContext<ChatWorkspaceContextValue>({});

/**
 * Returns the workspace ID associated with the current chat, if any.
 * Use this in tool renderers that need workspace data during execution.
 */
export const useChatWorkspaceId = () =>
	useContext(ChatWorkspaceContext).workspaceId;

/**
 * Returns the build ID from the chat binding, if any.
 * This is set when create_workspace or start_workspace persists
 * the build ID via UpdateChatWorkspaceBinding, and arrives on the
 * frontend through the chat watch event without polling.
 */
export const useChatBuildId = () => useContext(ChatWorkspaceContext).buildId;

export { ChatWorkspaceContext };

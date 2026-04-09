import { createContext, useContext } from "react";

const ChatWorkspaceContext = createContext<string | undefined>(undefined);

/**
 * Returns the workspace ID associated with the current chat, if any.
 * Use this in tool renderers that need workspace data during execution.
 */
export const useChatWorkspaceId = () => useContext(ChatWorkspaceContext);

export { ChatWorkspaceContext };

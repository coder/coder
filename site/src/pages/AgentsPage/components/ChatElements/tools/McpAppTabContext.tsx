import { createContext, useContext } from "react";
import type { ChatMCPApp } from "#/api/typesGenerated";

type McpAppTabContextValue = {
	/** Opens the MCP Apps view of a tool result in a right panel tab. */
	onOpenMcpApp?: (toolCallId: string, app: ChatMCPApp) => void;
};

export const McpAppTabContext = createContext<McpAppTabContextValue>({});

export const useMcpAppTab = () => useContext(McpAppTabContext);

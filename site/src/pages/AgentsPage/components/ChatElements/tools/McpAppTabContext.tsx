import { createContext } from "react";
import type { ChatMCPApp } from "#/api/typesGenerated";

type McpAppTabContextValue = {
	/** Unset when this user cannot open MCP Apps views in this chat. */
	onOpenMcpApp?: (toolCallId: string, app: ChatMCPApp) => void;
};

export const McpAppTabContext = createContext<McpAppTabContextValue>({});

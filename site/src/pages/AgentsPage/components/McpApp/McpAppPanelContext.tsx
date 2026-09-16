import { createContext, useContext } from "react";

export interface OpenMcpAppRequest {
	mcpServerConfigId: string;
	resourceUri: string;
	toolCallId: string;
}

interface McpAppPanelContextValue {
	/** Opens (or focuses) the right-panel tab for an MCP App. */
	openMcpApp?: (request: OpenMcpAppRequest) => void;
}

export const McpAppPanelContext = createContext<McpAppPanelContextValue>({});

export const useMcpAppPanel = () => useContext(McpAppPanelContext);

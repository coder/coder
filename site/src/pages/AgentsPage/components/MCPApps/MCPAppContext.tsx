import { createContext } from "react";
import type { UserRightPanelTab } from "../../utils/rightPanelTabs";

export const MCPAppContext = createContext<{
	chatId?: string;
	onOpenApp?: (tab: Extract<UserRightPanelTab, { kind: "mcp_app" }>) => void;
}>({});

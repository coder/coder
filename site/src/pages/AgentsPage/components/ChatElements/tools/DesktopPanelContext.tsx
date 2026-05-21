import { createContext, useContext } from "react";
import type * as TypesGen from "#/api/typesGenerated";

interface DesktopPanelContextValue {
	/** The parent chat ID used for the desktop VNC connection. */
	desktopChatId?: string;
	/** Opens the right sidebar panel and switches to the Desktop tab. */
	onOpenDesktop?: () => void;
	/** The workspace agent, when available. */
	agent?: TypesGen.WorkspaceAgent;
	/** The workspace, when available. */
	workspace?: TypesGen.Workspace;
}

export const DesktopPanelContext = createContext<DesktopPanelContextValue>({});

export const useDesktopPanel = () => useContext(DesktopPanelContext);

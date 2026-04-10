import { createContext, useContext } from "react";

interface DesktopPanelContextValue {
	/** The parent chat ID used for the desktop VNC connection. */
	desktopChatId?: string;
	/** Opens the right sidebar panel and switches to the Desktop tab. */
	onOpenDesktop?: () => void;
}

export const DesktopPanelContext = createContext<DesktopPanelContextValue>({});

export const useDesktopPanel = () => useContext(DesktopPanelContext);

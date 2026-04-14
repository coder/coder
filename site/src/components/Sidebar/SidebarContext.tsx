import { createContext, useContext } from "react";

interface SidebarState {
	collapsed: boolean;
}

export const SidebarContext = createContext<SidebarState>({ collapsed: false });

export const useSidebarContext = () => useContext(SidebarContext);

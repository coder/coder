import { createContext, useContext } from "react";

interface SidebarState {
	collapsed: boolean;
	/** Force the sidebar to expand. */
	expand: () => void;
}

export const SidebarContext = createContext<SidebarState>({
	collapsed: false,
	expand: () => {},
});

export const useSidebarContext = () => useContext(SidebarContext);

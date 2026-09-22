import { createContext, useContext } from "react";

interface SidebarState {
	collapsed: boolean;
	/** Force the sidebar to expand. */
	expand: () => void;
	/** Toggle collapsed/expanded state. */
	toggle: () => void;
}

export const SidebarContext = createContext<SidebarState>({
	collapsed: false,
	expand: () => {},
	toggle: () => {},
});

export const useSidebarContext = () => useContext(SidebarContext);

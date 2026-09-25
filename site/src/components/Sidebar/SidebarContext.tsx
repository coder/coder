import { createContext, useContext } from "react";

type SidebarState = {
	collapsed: boolean;
	expand: () => void;
	toggle: () => void;
};

export const SidebarContext = createContext<SidebarState>({
	collapsed: false,
	expand: () => {},
	toggle: () => {},
});

export const useSidebarContext = () => useContext(SidebarContext);

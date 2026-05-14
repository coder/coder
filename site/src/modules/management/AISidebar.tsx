import type { FC } from "react";
import { AISidebarView } from "./AISidebarView";
import { useActiveAISection } from "./useActiveAISection";

export const AISidebar: FC = () => {
	const activeSection = useActiveAISection();
	return <AISidebarView activeSection={activeSection} />;
};

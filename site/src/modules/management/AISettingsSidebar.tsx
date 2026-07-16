import type { FC } from "react";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import AISettingsSidebarView from "#/modules/management/AISettingsSidebarView";
import { useActiveAISection } from "#/modules/management/useActiveAISection";

/**
 * A sidebar for AI settings.
 */
export const AISettingsSidebar: FC = () => {
	const { permissions } = useAuthenticated();
	const activeSection = useActiveAISection();
	return (
		<AISettingsSidebarView
			permissions={permissions}
			activeSection={activeSection}
		/>
	);
};

import type { FC } from "react";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import AISettingsSidebarView from "#/modules/management/AISettingsSidebarView";

/**
 * A sidebar for AI settings.
 */
export const AISettingsSidebar: FC = () => {
	const { permissions } = useAuthenticated();
	return <AISettingsSidebarView permissions={permissions} />;
};

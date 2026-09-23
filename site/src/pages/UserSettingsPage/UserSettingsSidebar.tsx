import type { FC } from "react";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { UserSettingsSidebarView } from "./UserSettingsSidebarView";

/**
 * Wires the user settings sidebar to the signed-in user and the dashboard
 * entitlements and build info that gate optional pages.
 */
export const UserSettingsSidebar: FC = () => {
	const { user } = useAuthenticated();
	const { entitlements, buildInfo } = useDashboard();

	return (
		<UserSettingsSidebarView
			user={user}
			showSchedulePage={
				entitlements.features.advanced_template_scheduling.enabled
			}
			showOAuth2Page={buildInfo.oauth2_provider}
		/>
	);
};

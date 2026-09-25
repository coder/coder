import type { FC } from "react";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { UserSettingsSidebarView } from "./UserSettingsSidebarView";

/** Connects UserSettingsSidebarView to the current user and dashboard. */
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

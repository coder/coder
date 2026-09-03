import type { FC } from "react";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { getPrereleaseFlag } from "#/utils/buildInfo";
import { UserSettingsSidebarView } from "./UserSettingsSidebarView";

/**
 * Wires the user settings sidebar to the dashboard entitlements and
 * experiments that gate optional pages.
 */
export const UserSettingsSidebar: FC = () => {
	const { entitlements, experiments, buildInfo } = useDashboard();

	return (
		<UserSettingsSidebarView
			showSchedulePage={
				entitlements.features.advanced_template_scheduling.enabled
			}
			showOAuth2Page={
				experiments.includes("oauth2") ||
				getPrereleaseFlag(buildInfo) === "devel"
			}
		/>
	);
};
